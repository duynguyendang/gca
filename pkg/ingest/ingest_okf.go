
package ingest

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/gca/pkg/okf"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/keys"
)

func TagRoles(ctx context.Context, s *meb.MEBStore) error {
	for fact, err := range s.ScanWithPruning(ctx, "", config.PredicateHandledBy, "", keys.EntityFunc, false) {
		if err != nil {
			continue
		}
		h, ok := fact.Object.(string)
		if !ok {
			continue
		}
		s.AddFact(meb.Fact{Subject: string(h), Predicate: config.PredicateHasRole, Object: config.RoleAPIHandler})
	}
	for fact, err := range s.Scan("", config.PredicateInPackage, "") {
		if err != nil {
			continue
		}
		p, ok := fact.Object.(string)
		if !ok {
			continue
		}
		if strings.Contains(p, "types") || strings.Contains(p, "models") || strings.Contains(p, "meb") || strings.Contains(p, "ast") {
			s.AddFact(meb.Fact{Subject: fact.Subject, Predicate: config.PredicateHasRole, Object: config.RoleDataContract})
		}
	}
	return nil
}

// resolveOKFLinks resolves raw OKF link targets into okf_link and bridges_to facts.
// This runs after all files are processed so all concept IDs are registered.
func resolveOKFLinks(ctx context.Context, s *meb.MEBStore, projectName, sourceDir string) error {
	// 1. Collect all OKF concepts from Source Store
	type conceptInfo struct {
		id          string
		sourcePath  string
		fromDir     string
		rawLinks    []string
	}
	concepts := make(map[string]*conceptInfo)

	// Find all okf_concept facts
	for fact := range s.ScanContext(ctx, "", "okf_concept", "") {
		conceptID := fact.Subject
	 ci := &conceptInfo{id: conceptID}
		// Get source path from the document metadata (stored as the document ID)
		// The source path is the relPath used during ingest
	 concepts[conceptID] = ci
	}

	// 2. Collect raw links for each concept
	for fact := range s.ScanContext(ctx, "", "okf_raw_link", "") {
		if ci, ok := concepts[fact.Subject]; ok {
			if link, ok := fact.Object.(string); ok {
				ci.rawLinks = append(ci.rawLinks, link)
			}
		}
	}

	if len(concepts) == 0 {
		return nil
	}

	// 3. Build concept map for link resolution: bundleRelPath → conceptID
	conceptMap := make(map[string]string)
	for _, ci := range concepts {
		// Extract bundle-relative path from concept ID
		// Format: gca://project/<projectID>/okf/<bundleRelPath>
		prefix := fmt.Sprintf("%s%s/okf/", okf.ConceptIDPrefix, projectName)
		if strings.HasPrefix(ci.id, prefix) {
			bundleRelPath := strings.TrimPrefix(ci.id, prefix)
			conceptMap[bundleRelPath] = ci.id
		}
	}

	// 4. Resolve links and write facts
	var sourceFacts, analyticalFacts []meb.Fact

	for _, ci := range concepts {
		for _, rawLink := range ci.rawLinks {
			resolved := resolveOKFLink(rawLink, ci.fromDir, conceptMap, projectName)

			// Write okf_link fact
			sourceFacts = append(sourceFacts, meb.Fact{
				Subject:   ci.id,
				Predicate: "okf_link",
				Object:    resolved.Target,
			})

			// Write bridges_to fact if resolved to a code symbol
			if resolved.IsBridge && resolved.SymbolID != "" {
				analyticalFacts = append(analyticalFacts, meb.Fact{
					Subject:   ci.id,
					Predicate: "bridges_to",
					Object:    resolved.SymbolID,
				})
			}

			// Write okf_bridge_miss for unresolvable code links
			if resolved.IsBridgeMiss {
				analyticalFacts = append(analyticalFacts, meb.Fact{
					Subject:   ci.id,
					Predicate: "okf_bridge_miss",
					Object:    resolved.Target,
				})
			}
		}
	}

	// 5. Write facts
	if len(sourceFacts) > 0 {
		if err := s.AddFactBatch(sourceFacts); err != nil {
			logger.Warn("Failed to write okf_link facts", "error", err)
		}
	}
	if len(analyticalFacts) > 0 {
		// Write analytical facts to a separate store if available
		// For now, write to the same store with a different topic
		for _, fact := range analyticalFacts {
			if err := s.AddFact(fact); err != nil {
				logger.Warn("Failed to write analytical OKF fact", "predicate", fact.Predicate, "error", err)
			}
		}
	}

	logger.Info("OKF link resolution complete",
		"concepts", len(concepts),
		"source_facts", len(sourceFacts),
		"analytical_facts", len(analyticalFacts),
	)
	return nil
}

// okfResolvedLink holds the result of resolving an OKF link.
type okfResolvedLink struct {
	Target      string
	SymbolID    string
	IsBridge    bool
	IsBridgeMiss bool
}

// resolveOKFLink resolves a single OKF raw link target.
func resolveOKFLink(raw, fromDir string, conceptMap map[string]string, projectName string) okfResolvedLink {
	raw = strings.TrimSpace(raw)

	// 1. Bundle-absolute: "/tables/orders.md"
	if strings.HasPrefix(raw, "/") {
		target := strings.TrimPrefix(raw, "/")
		target = strings.TrimSuffix(target, ".md")
		if conceptID, ok := conceptMap[target]; ok {
			return okfResolvedLink{Target: conceptID}
		}
		return okfResolvedLink{Target: raw}
	}

	// 2. External URL
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return okfResolvedLink{Target: raw}
	}

	// 3. Relative: "./foo.md", "../bar.md", or "other.md"
	if strings.HasSuffix(raw, ".md") || strings.HasPrefix(raw, "./") || strings.HasPrefix(raw, "../") {
		target := strings.TrimSuffix(raw, ".md")
		resolved := target
		if fromDir != "" {
			resolved = filepath.ToSlash(filepath.Join(fromDir, target))
		}
		if conceptID, ok := conceptMap[resolved]; ok {
			return okfResolvedLink{Target: conceptID}
		}
		return okfResolvedLink{Target: "/" + resolved + ".md"}
	}

	// 4. Code-path link: "path/to/file.go#Symbol" or "gca://project/.../file/...#Symbol"
	// For now, store as-is — full resolution requires the Source Store
	if strings.Contains(raw, "#") {
		return okfResolvedLink{Target: raw, IsBridgeMiss: true}
	}

	// 5. Unknown format — store as-is
	return okfResolvedLink{Target: raw}
}
