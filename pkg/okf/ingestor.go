package okf

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

// StoreAccessor provides access to the Source and Analytical stores.
type StoreAccessor interface {
	GetSourceStore(projectID string) (*meb.MEBStore, error)
	GetAnalyticalStore(projectID string) (*meb.MEBStore, error)
}

// IngestOptions controls OKF bundle ingestion.
type IngestOptions struct {
	ProjectID string
	BundleDir string
	DryRun    bool // parse + validate, do not write facts
}

// IngestReport summarizes the outcome of an ingestion.
type IngestReport struct {
	Concepts   int
	Links      int
	Bridges    int
	BridgeMiss int           // okf_bridge_miss count
	Errors     []IngestError // per-file parse errors, non-fatal
	Conformant bool          // false if any file was skipped for conformance
	Duration   time.Duration
}

// IngestError records a non-fatal parse/conformance error for a single file.
type IngestError struct {
	File   string
	Reason string
}

func (e IngestError) Error() string {
	return fmt.Sprintf("okf: %s: %s", e.File, e.Reason)
}

// AnalyticsVersionKey is the subject used by the existing analyzer to
// track the analytics version. Deleting this fact forces RunPostIngestAnalysis
// to re-run on the next call (the short-circuit at analyzer.go:388 fails).
const AnalyticsVersionKey = "analytics"

// OKFVersionSubject is the subject storing the OKF bundle version
// (okf_version predicate) per project.
const OKFVersionSubject = "okf_version"

// MaxBodyKB is the max body size stored inline in the okf_body fact.
// Full bodies larger than this are written to disk only.
const MaxBodyKB = 16

// Ingest walks a bundle directory, parses all non-reserved .md files,
// and writes OKF facts into the Source Store and bridges into the
// Analytical Store.
//
// After writing OKF facts, the ingestor deletes the analytics_version
// fact so the next RunPostIngestAnalysis call re-runs computeCentrality
// (which now includes okf_link and bridges_to edges in degree facts).
func Ingest(ctx context.Context, sa StoreAccessor, dataDir string, opts IngestOptions) (*IngestReport, error) {
	start := time.Now()
	report := &IngestReport{Conformant: true}

	if opts.BundleDir == "" {
		return nil, fmt.Errorf("okf: BundleDir is required")
	}

	// 1. Walk bundle directory
	type fileEntry struct {
		relPath string
		content []byte
	}
	var entries []fileEntry
	if err := filepath.WalkDir(opts.BundleDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		relPath, err := filepath.Rel(opts.BundleDir, path)
		if err != nil {
			return err
		}
		// Skip reserved files (index.md, log.md) at root level; always skip log.md
		base := filepath.Base(relPath)
		if base == "log.md" {
			return nil
		}
		entries = append(entries, fileEntry{relPath: relPath, content: nil})
		return nil
	}); err != nil {
		return nil, fmt.Errorf("okf: walk bundle dir: %w", err)
	}

	if len(entries) == 0 {
		report.Duration = time.Since(start)
		return report, nil
	}

	// Read content
	for i := range entries {
		raw, err := os.ReadFile(filepath.Join(opts.BundleDir, entries[i].relPath))
		if err != nil {
			report.Errors = append(report.Errors, IngestError{File: entries[i].relPath, Reason: err.Error()})
			report.Conformant = false
			continue
		}
		entries[i].content = raw
	}

	// 2. Read root index.md for okf_version (OKF §11)
	var okfVersion string
	indexPath := filepath.Join(opts.BundleDir, "index.md")
	if raw, err := os.ReadFile(indexPath); err == nil {
		v, err := ParseRootIndexFrontmatter(raw)
		if err != nil {
			report.Errors = append(report.Errors, IngestError{File: "index.md", Reason: err.Error()})
		} else {
			okfVersion = v
		}
	}

	// 3. Parse all concepts
	concepts := make([]*Concept, 0, len(entries))
	for _, e := range entries {
		c, err := ParseConcept(e.relPath, e.content)
		if err != nil {
			if errors.Is(err, ErrMissingType) || errors.Is(err, ErrInvalidFrontmatter) {
				report.Errors = append(report.Errors, IngestError{File: e.relPath, Reason: err.Error()})
				report.Conformant = false
				continue // skip non-conformant files, but continue the bundle
			}
			report.Errors = append(report.Errors, IngestError{File: e.relPath, Reason: err.Error()})
			continue
		}
		c.ID = ConceptID(opts.ProjectID, e.relPath)
		concepts = append(concepts, c)
	}

	report.Concepts = len(concepts)
	if opts.DryRun {
		report.Duration = time.Since(start)
		return report, nil
	}

	// 4. Get source store (needed for link resolution and dedup)
	sourceStore, err := sa.GetSourceStore(opts.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("okf: get source store: %w", err)
	}

	// 5. Resolve links (second pass, now that all concept IDs are known)
	resolver := NewLinkResolver(opts.ProjectID, sourceStore, concepts)
	resolved := make(map[string][]ResolvedLink, len(concepts))
	for _, c := range concepts {
		fromDir := DirOfConcept(c.SourcePath)
		for _, rawTarget := range c.Links {
			rl := resolver.Resolve(ctx, rawTarget, fromDir)
			resolved[c.ID] = append(resolved[c.ID], rl)
		}
	}

	// 6. Load existing content hashes for dedup
	existingHashes := make(map[string]string)
	for fact, err := range sourceStore.ScanContext(ctx, "", string(PredOKFContentHash), "") {
		if err != nil {
			continue
		}
		if h, ok := fact.Object.(string); ok {
			existingHashes[fact.Subject] = h
		}
	}

	// 7. Write Source Store facts
	//    Delete old OKF concepts whose content hash changed, then re-write.
	//    Unchanged concepts keep their existing facts (the dedup skip below).
	//    This fixes bug: delete-then-skip was causing data loss on re-ingest.
	conceptPrefix := fmt.Sprintf("%s%s/okf/", ConceptIDPrefix, opts.ProjectID)
	changed := make(map[string]bool, len(concepts))
	for _, c := range concepts {
		if prev, ok := existingHashes[c.ID]; !ok || prev != c.ContentHash {
			changed[c.ID] = true
			// Delete old facts for this concept from both stores
			if err := sourceStore.DeleteFactsBySubject(c.ID); err != nil {
				logger.Warn("okf: failed to delete concept facts", "subject", c.ID, "error", err)
			}
		}
	}
	// Also delete stale concepts (in the store but not in the current bundle)
	for subject := range sourceStore.ScanSubjectsByPrefix(ctx, conceptPrefix) {
		if !changed[subject] && !conceptInSlice(subject, concepts) {
			if err := sourceStore.DeleteFactsBySubject(subject); err != nil {
				logger.Warn("okf: failed to delete stale concept", "subject", subject, "error", err)
			}
		}
	}

	// Acquire analytical store once for bridge deletion, writing, and version invalidation.
	analyticalStore, analErr := sa.GetAnalyticalStore(opts.ProjectID)
	if analErr == nil {
		// Delete old bridge facts for changed concepts from Analytical Store
		for _, c := range concepts {
			if changed[c.ID] {
				if err := analyticalStore.DeleteFactsBySubject(c.ID); err != nil {
					logger.Warn("okf: failed to delete analytical facts", "subject", c.ID, "error", err)
				}
			}
		}
	}

	facts := make([]meb.Fact, 0, len(concepts)*15)
	analyticalFacts := make([]meb.Fact, 0, len(concepts)*2) // bridges_to, okf_bridge_miss
	for _, c := range concepts {
		// Dedup: skip unchanged concepts (their facts survive from prior ingest)
		if prev, ok := existingHashes[c.ID]; ok && prev == c.ContentHash && changed[c.ID] == false {
			continue
		}

		sid := c.ID
		facts = append(facts,
			meb.Fact{Subject: sid, Predicate: string(PredOKFConcept), Object: c.Type},
			meb.Fact{Subject: sid, Predicate: string(PredOKFTitle), Object: c.Title},
			meb.Fact{Subject: sid, Predicate: string(PredOKFDescription), Object: c.Description},
			meb.Fact{Subject: sid, Predicate: string(PredOKFContentHash), Object: c.ContentHash},
			meb.Fact{Subject: sid, Predicate: config.PredicateHasRole, Object: config.RoleOKFConcept},
		)

		if c.Resource != "" {
			facts = append(facts, meb.Fact{Subject: sid, Predicate: string(PredOKFResource), Object: c.Resource})
		}
		if c.Timestamp != "" {
			facts = append(facts, meb.Fact{Subject: sid, Predicate: string(PredOKFTimestamp), Object: c.Timestamp})
		}
		for _, tag := range c.Tags {
			facts = append(facts, meb.Fact{Subject: sid, Predicate: string(PredOKFTag), Object: tag})
		}

		// Body: store up to MaxBodyKB inline, full body to disk if larger
		body := c.Body
		if len(body) > MaxBodyKB*1024 {
			// Write full body to disk
			bodyPath := filepath.Join(dataDir, "okf_bodies", opts.ProjectID, c.SourcePath)
			if err := os.MkdirAll(filepath.Dir(bodyPath), 0o755); err == nil {
				os.WriteFile(bodyPath, []byte(body), 0o644) //nolint:errcheck
			}
			body = body[:MaxBodyKB*1024]
		}
		if body != "" {
			facts = append(facts, meb.Fact{Subject: sid, Predicate: string(PredOKFBody), Object: body})
		}

		// Frontmatter extension keys
		if fjson, err := c.SerializeFrontmatter(); err == nil && fjson != "" {
			facts = append(facts, meb.Fact{Subject: sid, Predicate: string(PredOKFFrontmatter), Object: fjson})
		}

		// okf_link and bridges_to (from resolved links)
		for _, rl := range resolved[sid] {
			facts = append(facts, meb.Fact{Subject: sid, Predicate: string(PredOKFLink), Object: rl.Target})
			report.Links++
			if rl.IsBridge {
				analyticalFacts = append(analyticalFacts, meb.Fact{Subject: sid, Predicate: string(PredBridgesTo), Object: rl.SymbolID})
				report.Bridges++
			}
			// Record bridge misses (code-path link that didn't resolve)
			if IsBridgeMissCandidate(rl.Raw) && rl.SymbolID == "" {
				analyticalFacts = append(analyticalFacts, meb.Fact{Subject: sid, Predicate: string(PredOKFBridgeMiss), Object: rl.Target})
				report.BridgeMiss++
			}
		}
	}

	if len(facts) > 0 {
		if err := sourceStore.AddFactBatch(facts); err != nil {
			return nil, fmt.Errorf("okf: write source facts: %w", err)
		}
	}

	// Write bridge facts to Analytical Store (bridges_to, okf_bridge_miss)
	// analyticalStore was acquired above (line ~206). Reuse it here.
	if analErr == nil {
		if len(analyticalFacts) > 0 {
			if err := analyticalStore.AddFactBatch(analyticalFacts); err != nil {
				logger.Warn("okf: failed to write analytical facts", "error", err)
			}
		}
		// Invalidate analytics version to force re-run of computeCentrality
		_ = analyticalStore.DeleteFactsBySubject(AnalyticsVersionKey)
	}

	// 8. Write OKF version
	if okfVersion != "" {
		if err := sourceStore.AddFact(meb.Fact{
			Subject:   OKFVersionSubject,
			Predicate: string(PredOKFVersion),
			Object:    okfVersion,
		}); err != nil {
			logger.Warn("okf: failed to write version", "error", err)
		}
	}

	report.Duration = time.Since(start)
	logger.Info("OKF ingest complete",
		"project", opts.ProjectID,
		"concepts", report.Concepts,
		"links", report.Links,
		"bridges", report.Bridges,
		"bridge_misses", report.BridgeMiss,
		"conformant", report.Conformant,
		"duration", report.Duration,
	)
	return report, nil
}

// conceptInSlice checks if a subject ID belongs to any concept in the slice.
func conceptInSlice(subject string, concepts []*Concept) bool {
	for _, c := range concepts {
		if c.ID == subject {
			return true
		}
	}
	return false
}

// IsBridgeMissCandidate returns true for link targets that look like code-path
// references (i.e. contain a fragment like "foo.go#Bar") but did not resolve
// to a Source Store symbol.
func IsBridgeMissCandidate(raw string) bool {
	if IsExternalURL(raw) || IsBundleAbsolute(raw) || IsRelative(raw) {
		return false
	}
	// Must look like "path#something"
	_, _, ok := SplitPathFragment(raw)
	return ok
}
