package okf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

// StoreAccessor grants the ingestor access to the Source and Analytical stores
// for a project. *manager.StoreManager satisfies this interface.
type StoreAccessor interface {
	GetSourceStore(projectID string) (*meb.MEBStore, error)
	GetAnalyticalStore(projectID string) (*meb.MEBStore, error)
}

// IngestOptions controls an OKF bundle ingestion run.
type IngestOptions struct {
	ProjectID string
	BundleDir string
	DryRun    bool // parse + validate, do not write facts
}

// IngestReport summarizes an OKF bundle ingestion run.
type IngestReport struct {
	Concepts   int           `json:"concepts"`
	Links      int           `json:"links"`
	Bridges    int           `json:"bridges"`
	BridgeMiss int           `json:"bridge_misses"`
	Errors     []IngestError `json:"errors"`
	Conformant bool          `json:"conformant"`
	Duration   string        `json:"duration"`
}

// IngestError records a non-fatal per-file conformance error.
type IngestError struct {
	File   string `json:"file"`
	Reason string `json:"reason"`
}

// maxInlineBodyBytes is the maximum OKF body stored inline as a fact. Larger
// bodies are persisted to disk under <dataDir>/okf_bodies and truncated inline.
const maxInlineBodyBytes = 16 * 1024

// IsOKFBundledURL reports whether a resolved link target is a code-path URI
// (gca://project/<id>/file/<path>#<symbol>) that could bridge to a source symbol.
func IsOKFBundledURL(target string) bool {
	return strings.Contains(target, "/file/") && strings.Contains(target, "#")
}

// parsedConcept is a parsed OKF concept paired with its bundle-relative path.
type parsedConcept struct {
	concept *Concept
	relPath string
}

// Ingest walks an OKF bundle directory, parses each concept, and writes facts
// into the project's Source and Analytical stores. Follows docs/designs/okf-support.md
// §Ingest Flow. dataDir is the store data directory (used to persist full bodies).
func Ingest(ctx context.Context, sa StoreAccessor, dataDir string, opts IngestOptions) (*IngestReport, error) {
	start := time.Now()
	report := &IngestReport{Conformant: true}

	// Phase 1: walk the bundle.
	files, err := walkBundle(opts.BundleDir)
	if err != nil {
		return nil, err
	}

	// Phase 2: root index.md version (OKF §11).
	version := ""
	if raw, err := os.ReadFile(filepath.Join(opts.BundleDir, "index.md")); err == nil {
		if v, verr := ParseRootIndexFrontmatter(raw); verr == nil {
			version = v
		}
	}

	// Phase 3: parse every concept file.
	concepts := make([]*parsedConcept, 0, len(files))
	existing := make(map[string]bool) // concept ID → present in this bundle
	for rel, content := range files {
		concept, perr := ParseConcept(rel, content)
		if perr != nil {
			report.Conformant = false
			report.Errors = append(report.Errors, IngestError{File: rel, Reason: perr.Error()})
			continue
		}
		concept.ID = ConceptID(opts.ProjectID, rel)
		concepts = append(concepts, &parsedConcept{concept: concept, relPath: rel})
		existing[concept.ID] = true
	}

	report.Concepts = len(concepts)
	if opts.DryRun {
		return report, nil
	}

	source, err := sa.GetSourceStore(opts.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("okf ingest: source store: %w", err)
	}
	analytical, err := sa.GetAnalyticalStore(opts.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("okf ingest: analytical store: %w", err)
	}

	// Phase 4: content-hash dedup + stale cleanup.
	hashes := loadContentHashes(ctx, source)
	stored := loadConceptIDs(ctx, source)
	bodyDir := filepath.Join(dataDir, "okf_bodies", opts.ProjectID)

	for _, pc := range concepts {
		c := pc.concept
		if storedHash, ok := hashes[c.ID]; ok && storedHash == c.ContentHash {
			continue // unchanged — existing facts survive
		}
		if err := source.DeleteFactsBySubject(c.ID); err != nil {
			logger.Warn("okf ingest: failed to clear changed concept", "id", c.ID, "error", err)
		}
		if err := analytical.DeleteFactsBySubject(c.ID); err != nil {
			logger.Warn("okf ingest: failed to clear changed concept", "id", c.ID, "error", err)
		}
	}

	// Stale concepts present in the store but not in the current bundle.
	for id := range stored {
		if !existing[id] {
			if err := source.DeleteFactsBySubject(id); err != nil {
				logger.Warn("okf ingest: failed to clear stale concept", "id", id, "error", err)
			}
			if err := analytical.DeleteFactsBySubject(id); err != nil {
				logger.Warn("okf ingest: failed to clear stale concept", "id", id, "error", err)
			}
		}
	}

	// Phase 5: emit facts for new/changed concepts.
	var sourceFacts []meb.Fact
	for _, pc := range concepts {
		c := pc.concept
		if storedHash, ok := hashes[c.ID]; ok && storedHash == c.ContentHash {
			continue
		}
		facts := conceptFacts(c, config.RoleOKFConcept)
		sourceFacts = append(sourceFacts, facts...)
		if len(c.Body) > maxInlineBodyBytes {
			writeBodyToDisk(bodyDir, c, []byte(c.Body))
		}
	}
	if len(sourceFacts) > 0 {
		if err := source.AddFactBatch(sourceFacts); err != nil {
			logger.Warn("okf ingest: failed to write concept facts", "error", err)
		}
	}

	// Phase 6: two-pass cross-link resolution.
	resolver := NewLinkResolver(opts.ProjectID, source, conceptList(concepts))
	srcLinks, analyticalFacts := resolveAllLinks(ctx, resolver, concepts, opts.ProjectID)
	report.Links = len(srcLinks)
	report.Bridges = countBridges(analyticalFacts)
	report.BridgeMiss = countBridgeMisses(analyticalFacts)
	if len(srcLinks) > 0 {
		if err := source.AddFactBatch(srcLinks); err != nil {
			logger.Warn("okf ingest: failed to write okf_link facts", "error", err)
		}
	}
	if len(analyticalFacts) > 0 {
		if err := analytical.AddFactBatch(analyticalFacts); err != nil {
			logger.Warn("okf ingest: failed to write analytical facts", "error", err)
		}
	}

	// Phase 7: okf_age_days (analytical).
	writeAgeFacts(ctx, analytical, concepts)

	// Phase 8: invalidate analytics version so centrality recomputes.
	if err := analytical.DeleteFactsBySubject("analytics"); err != nil {
		logger.Warn("okf ingest: failed to invalidate analytics version", "error", err)
	}

	// Phase 9: write okf_version.
	if version != "" {
		if err := source.AddFact(meb.Fact{Subject: "analytics", Predicate: PredOKFVersion.String(), Object: version}); err != nil {
			logger.Warn("okf ingest: failed to write okf_version", "error", err)
		}
	}

	report.Duration = time.Since(start).String()
	return report, nil
}

// walkBundle returns the bundle-relative path → content for every .md file,
// skipping reserved files (log.md always, index.md handled separately).
func walkBundle(bundleDir string) (map[string][]byte, error) {
	info, err := os.Stat(bundleDir)
	if err != nil {
		return nil, fmt.Errorf("okf ingest: bundle dir: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("okf ingest: bundle path is not a directory: %s", bundleDir)
	}

	files := make(map[string][]byte)
	err = filepath.WalkDir(bundleDir, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(bundleDir, path)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if !strings.HasSuffix(strings.ToLower(rel), ".md") {
			return nil
		}
		if IsReservedFile(rel) {
			return nil
		}
		content, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		files[rel] = content
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("okf ingest: walk bundle: %w", err)
	}
	return files, nil
}

// conceptFacts builds the Source Store facts for a single concept.
func conceptFacts(c *Concept, role string) []meb.Fact {
	facts := []meb.Fact{
		{Subject: c.ID, Predicate: PredOKFConcept.String(), Object: c.Type},
		{Subject: c.ID, Predicate: PredOKFTitle.String(), Object: c.Title},
		{Subject: c.ID, Predicate: PredOKFDescription.String(), Object: c.Description},
		{Subject: c.ID, Predicate: PredOKFContentHash.String(), Object: c.ContentHash},
		{Subject: c.ID, Predicate: config.PredicateHasRole, Object: role},
		{Subject: c.ID, Predicate: config.PredicateType, Object: role},
		{Subject: c.ID, Predicate: config.PredicateInPackage, Object: DirOfConcept(c.SourcePath)},
	}
	if c.Timestamp != "" {
		facts = append(facts, meb.Fact{Subject: c.ID, Predicate: PredOKFTimestamp.String(), Object: c.Timestamp})
	}
	if c.Resource != "" {
		facts = append(facts, meb.Fact{Subject: c.ID, Predicate: PredOKFResource.String(), Object: c.Resource})
	}
	for _, tag := range c.Tags {
		facts = append(facts, meb.Fact{Subject: c.ID, Predicate: PredOKFTag.String(), Object: tag})
	}
	body := c.Body
	if len(body) > maxInlineBodyBytes {
		body = body[:maxInlineBodyBytes]
	}
	if body != "" {
		facts = append(facts, meb.Fact{Subject: c.ID, Predicate: PredOKFBody.String(), Object: body})
	}
	if fjson, err := c.SerializeFrontmatter(); err == nil && fjson != "" {
		facts = append(facts, meb.Fact{Subject: c.ID, Predicate: PredOKFFrontmatter.String(), Object: fjson})
	}
	return facts
}

// resolveAllLinks runs the two-pass link resolution for all concepts.
func resolveAllLinks(ctx context.Context, resolver *LinkResolver, concepts []*parsedConcept, projectID string) ([]meb.Fact, []meb.Fact) {
	var srcLinks []meb.Fact
	var analyticalFacts []meb.Fact
	for _, pc := range concepts {
		for _, raw := range pc.concept.Links {
			resolved := resolver.Resolve(ctx, raw, DirOfConcept(pc.concept.SourcePath))
			srcLinks = append(srcLinks, meb.Fact{
				Subject:   pc.concept.ID,
				Predicate: PredOKFLink.String(),
				Object:    resolved.Target,
			})
			if resolved.IsBridge && resolved.SymbolID != "" {
				analyticalFacts = append(analyticalFacts, meb.Fact{
					Subject:   pc.concept.ID,
					Predicate: PredBridgesTo.String(),
					Object:    resolved.SymbolID,
				})
			} else if IsOKFBundledURL(resolved.Target) {
				analyticalFacts = append(analyticalFacts, meb.Fact{
					Subject:   pc.concept.ID,
					Predicate: PredOKFBridgeMiss.String(),
					Object:    resolved.Target,
				})
			}
		}
	}
	return srcLinks, analyticalFacts
}

// writeAgeFacts computes okf_age_days for concepts with a timestamp.
func writeAgeFacts(ctx context.Context, analytical *meb.MEBStore, concepts []*parsedConcept) {
	now := time.Now()
	var facts []meb.Fact
	for _, pc := range concepts {
		if pc.concept.Timestamp == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, pc.concept.Timestamp)
		if err != nil {
			continue
		}
		days := now.Sub(t).Hours() / 24
		facts = append(facts, meb.Fact{Subject: pc.concept.ID, Predicate: "okf_age_days", Object: days})
	}
	if len(facts) > 0 {
		if err := analytical.AddFactBatch(facts); err != nil {
			logger.Warn("okf ingest: failed to write okf_age_days", "error", err)
		}
	}
}

// loadContentHashes reads all existing okf_content_hash facts.
func loadContentHashes(ctx context.Context, source *meb.MEBStore) map[string]string {
	hashes := make(map[string]string)
	for fact, err := range source.ScanContext(ctx, "", PredOKFContentHash.String(), "") {
		if err != nil {
			continue
		}
		if h, ok := fact.Object.(string); ok {
			hashes[fact.Subject] = h
		}
	}
	return hashes
}

// loadConceptIDs reads all existing okf_concept subjects.
func loadConceptIDs(ctx context.Context, source *meb.MEBStore) map[string]bool {
	ids := make(map[string]bool)
	for fact, err := range source.ScanContext(ctx, "", PredOKFConcept.String(), "") {
		if err != nil {
			continue
		}
		ids[fact.Subject] = true
	}
	return ids
}

// conceptList flattens parsed concepts into *Concept for the LinkResolver.
func conceptList(pcs []*parsedConcept) []*Concept {
	out := make([]*Concept, 0, len(pcs))
	for _, pc := range pcs {
		out = append(out, pc.concept)
	}
	return out
}

func countBridges(facts []meb.Fact) int {
	n := 0
	for _, f := range facts {
		if f.Predicate == PredBridgesTo.String() {
			n++
		}
	}
	return n
}

func countBridgeMisses(facts []meb.Fact) int {
	n := 0
	for _, f := range facts {
		if f.Predicate == PredOKFBridgeMiss.String() {
			n++
		}
	}
	return n
}

// writeBodyToDisk persists a full OKF body when it is too large to store inline.
func writeBodyToDisk(bodyDir string, c *Concept, content []byte) {
	path := filepath.Join(bodyDir, c.SourcePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		logger.Warn("okf ingest: failed to create body dir", "error", err)
		return
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		logger.Warn("okf ingest: failed to write body", "path", path, "error", err)
	}
}