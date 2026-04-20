package ingest

import (
	"context"
	"fmt"
	"log"
	"strings"

	mebpkg "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/meb"
)

// Analyzer performs post-ingestion analysis on the Source Store
// and writes results (smells, centrality) to the Analytical Store.
type Analyzer struct {
	storeManager  StoreManagerInterface
	templateStore TemplateStoreInterface
}

// StoreManagerInterface defines the interface for store operations.
type StoreManagerInterface interface {
	GetSourceStore(projectID string) (*meb.MEBStore, error)
	GetAnalyticalStore(projectID string) (*meb.MEBStore, error)
}

// TemplateStoreInterface defines the interface for template operations.
type TemplateStoreInterface interface {
	GetTemplate(ctx context.Context, projectID, templateID string) (*TemplateStoreQuery, error)
	ListTemplates(ctx context.Context, projectID, category string) ([]*TemplateStoreQuery, error)
}

// TemplateStoreQuery represents a query template from the TemplateStore.
type TemplateStoreQuery struct {
	ID          string
	Body        string
	Category    string
	Severity    string
	Description string
	Parameters  []TemplateParam
}

// TemplateParam represents a parameter in a query template.
type TemplateParam struct {
	Name        string
	Type        string
	Description string
}

// NewAnalyzer creates a new Analyzer.
func NewAnalyzer(sm StoreManagerInterface, ts TemplateStoreInterface) *Analyzer {
	return &Analyzer{
		storeManager:  sm,
		templateStore: ts,
	}
}

// RunStaticAnalysis executes the full static analysis pipeline:
// 1. Compute centrality and entry points -> Analytical Store
// 2. Execute smell detection templates -> Analytical Store
func (a *Analyzer) RunStaticAnalysis(ctx context.Context, projectID string) error {
	log.Printf("Starting static analysis for project: %s", projectID)

	// Step 1: Compute centrality and entry points
	if err := a.computeCentrality(ctx, projectID); err != nil {
		log.Printf("Warning: centrality computation failed: %v", err)
	}

	// Step 2: Detect smells using hardcoded queries
	if err := a.detectSmells(ctx, projectID); err != nil {
		log.Printf("Warning: smell detection failed: %v", err)
	}

	// Step 3: Execute template-based queries if template store is available
	if a.templateStore != nil {
		if err := a.executeTemplateQueries(ctx, projectID); err != nil {
			log.Printf("Warning: template query execution failed: %v", err)
		}
	}

	log.Printf("Static analysis completed for project: %s", projectID)
	return nil
}

// executeTemplateQueries executes smell detection templates from the TemplateStore.
func (a *Analyzer) executeTemplateQueries(ctx context.Context, projectID string) error {
	if a.templateStore == nil {
		return nil
	}

	// Get all smell templates
	templates, err := a.templateStore.ListTemplates(ctx, projectID, "smell")
	if err != nil {
		return fmt.Errorf("failed to list smell templates: %w", err)
	}

	sourceStore, err := a.storeManager.GetSourceStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get source store: %w", err)
	}

	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	for _, tmpl := range templates {
		if tmpl.Body == "" {
			continue
		}

		// Execute template query against source store
		results, err := mebpkg.Query(ctx, sourceStore, tmpl.Body)
		if err != nil {
			log.Printf("Warning: template query %s failed: %v", tmpl.ID, err)
			continue
		}

		// Write results to analytical store
		for _, r := range results {
			subject := extractString(r, "Subject")
			if subject == "" {
				continue
			}

			fact := meb.Fact{
				Subject:   subject,
				Predicate: "has_smell",
				Object:    tmpl.ID + ":" + tmpl.Category,
			}
			if err := analyticalStore.AddFact(fact); err != nil {
				log.Printf("Warning: failed to add smell fact: %v", err)
			}
		}
	}

	return nil
}

// extractString extracts a string value from a map.
func extractString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// RunPostIngestAnalysis runs all post-ingestion analysis for a project.
// This should be called after the main ingestion is complete.
// It runs asynchronously to avoid blocking the ingestion process.
func (a *Analyzer) RunPostIngestAnalysis(ctx context.Context, projectID string) error {
	log.Printf("Starting post-ingest analysis for project: %s", projectID)

	// Run centrality computation
	if err := a.computeCentrality(ctx, projectID); err != nil {
		log.Printf("Warning: centrality computation failed: %v", err)
	}

	// Run smell detection
	if err := a.detectSmells(ctx, projectID); err != nil {
		log.Printf("Warning: smell detection failed: %v", err)
	}

	log.Printf("Post-ingest analysis complete for project: %s", projectID)
	return nil
}

// RunPostIngestAnalysisAsync runs analysis asynchronously.
func (a *Analyzer) RunPostIngestAnalysisAsync(ctx context.Context, projectID string) {
	go func() {
		if err := a.RunPostIngestAnalysis(ctx, projectID); err != nil {
			log.Printf("Async post-ingest analysis failed: %v", err)
		}
	}()
}

// computeCentrality calculates in/out degree for files and functions,
// then writes centrality facts to the Analytical Store.
func (a *Analyzer) computeCentrality(ctx context.Context, projectID string) error {
	sourceStore, err := a.storeManager.GetSourceStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get source store: %w", err)
	}

	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	// Compute hub scores - files that are called by many others
	hubQuery := `triples(File, "calls", _), not contains(File, ":")`
	hubResults, err := mebpkg.Query(ctx, sourceStore, hubQuery)
	if err != nil {
		return fmt.Errorf("hub query failed: %w", err)
	}

	// Count callers per file
	callerCounts := make(map[string]int)
	for _, r := range hubResults {
		if file, ok := r["File"].(string); ok {
			callerCounts[file]++
		}
	}

	// Write hub score facts for files with multiple callers
	for file, count := range callerCounts {
		if count > 5 { // Threshold for hub classification
			fact := meb.Fact{
				Subject:   file,
				Predicate: "has_hub_score",
				Object:    fmt.Sprintf("%d", count),
			}
			if err := analyticalStore.AddFact(fact); err != nil {
				log.Printf("Warning: failed to add hub score for %s: %v", file, err)
			}
		}
	}

	// Compute entry points - files that define main, init, or have many callees
	entryQuery := `triples(File, "defines", Symbol), or(contains(Symbol, "main"), contains(Symbol, "init"))`
	entryResults, err := mebpkg.Query(ctx, sourceStore, entryQuery)
	if err != nil {
		log.Printf("Warning: entry point query failed: %v", err)
	} else {
		for _, r := range entryResults {
			if file, ok := r["File"].(string); ok {
				fact := meb.Fact{
					Subject:   file,
					Predicate: "is_entry_point",
					Object:    "true",
				}
				if err := analyticalStore.AddFact(fact); err != nil {
					log.Printf("Warning: failed to add entry point for %s: %v", file, err)
				}
			}
		}
	}

	// Compute centrality for symbols (functions/methods)
	symbolCentralityQuery := `triples(File, "defines", Symbol), triples(Symbol, "calls", Target)`
	symbolResults, err := mebpkg.Query(ctx, sourceStore, symbolCentralityQuery)
	if err != nil {
		log.Printf("Warning: symbol centrality query failed: %v", err)
	} else {
		// Count calls per symbol
		symbolCalls := make(map[string]int)
		for _, r := range symbolResults {
			if symbol, ok := r["Symbol"].(string); ok {
				symbolCalls[symbol]++
			}
		}

		for symbol, count := range symbolCalls {
			if count > 10 { // High connectivity threshold
				fact := meb.Fact{
					Subject:   symbol,
					Predicate: "has_centrality",
					Object:    fmt.Sprintf("%d", count),
				}
				if err := analyticalStore.AddFact(fact); err != nil {
					log.Printf("Warning: failed to add centrality for %s: %v", symbol, err)
				}
			}
		}
	}

	log.Printf("Centrality analysis complete: %d hub files, %d entry points",
		len(callerCounts), len(entryResults))

	return nil
}

// detectSmells runs smell detection queries and writes results to Analytical Store.
func (a *Analyzer) detectSmells(ctx context.Context, projectID string) error {
	sourceStore, err := a.storeManager.GetSourceStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get source store: %w", err)
	}

	analyticalStore, err := a.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return fmt.Errorf("failed to get analytical store: %w", err)
	}

	smellResults := 0

	// Detect circular dependencies
	circularQuery := `triples(A, "calls", B), triples(B, "calls", A), A != B`
	circularResults, err := mebpkg.Query(ctx, sourceStore, circularQuery)
	if err != nil {
		log.Printf("Warning: circular dependency query failed: %v", err)
	} else {
		for _, r := range circularResults {
			aStr, _ := r["A"].(string)
			bStr, _ := r["B"].(string)
			fact := meb.Fact{
				Subject:   aStr,
				Predicate: "has_smell",
				Object:    "circular_dependency:" + bStr,
			}
			if err := analyticalStore.AddFact(fact); err != nil {
				log.Printf("Warning: failed to add circular smell: %v", err)
			} else {
				smellResults++
			}
		}
	}

	// Detect hub files (God files) - files with excessive imports
	// Query all imports and count in Go - use named variable instead of _ for wildcard
	importQuery := `triples(File, "imports", P)`
	importResults, err := mebpkg.Query(ctx, sourceStore, importQuery)
	if err != nil {
		log.Printf("Warning: import query failed: %v", err)
	} else {
		importCounts := make(map[string]int)
		for _, r := range importResults {
			if file, ok := r["File"].(string); ok {
				importCounts[file]++
			}
		}
		for file, count := range importCounts {
			if count > 50 && !strings.Contains(file, ":") {
				fact := meb.Fact{
					Subject:   file,
					Predicate: "has_smell",
					Object:    fmt.Sprintf("god_file:imports:%d", count),
				}
				if err := analyticalStore.AddFact(fact); err != nil {
					log.Printf("Warning: failed to add hub smell: %v", err)
				} else {
					smellResults++
				}
			}
		}
	}

	// Detect files with excessive definitions
	// Query all defines and count in Go - use named variable instead of _ for wildcard
	definesQuery := `triples(File, "defines", S)`
	definesResults, err := mebpkg.Query(ctx, sourceStore, definesQuery)
	if err != nil {
		log.Printf("Warning: defines query failed: %v", err)
	} else {
		definesCounts := make(map[string]int)
		for _, r := range definesResults {
			if file, ok := r["File"].(string); ok {
				definesCounts[file]++
			}
		}
		for file, count := range definesCounts {
			if count > 30 && !strings.Contains(file, ":") {
				fact := meb.Fact{
					Subject:   file,
					Predicate: "has_smell",
					Object:    fmt.Sprintf("god_file:defines:%d", count),
				}
				if err := analyticalStore.AddFact(fact); err != nil {
					log.Printf("Warning: failed to add god file smell: %v", err)
				} else {
					smellResults++
				}
			}
		}
	}

	// Detect layer violations
	layerQuery := `triples(File, "imports", Target), triples(File, "has_tag", LayerTag), triples(Target, "has_tag", "backend"), LayerTag != "backend"`
	layerResults, err := mebpkg.Query(ctx, sourceStore, layerQuery)
	if err != nil {
		log.Printf("Warning: layer violation query failed: %v", err)
	} else {
		for _, r := range layerResults {
			file, _ := r["File"].(string)
			target, _ := r["Target"].(string)
			layer, _ := r["LayerTag"].(string)
			fact := meb.Fact{
				Subject:   file,
				Predicate: "has_smell",
				Object:    "layer_violation:" + target + ":" + layer,
			}
			if err := analyticalStore.AddFact(fact); err != nil {
				log.Printf("Warning: failed to add layer violation: %v", err)
			} else {
				smellResults++
			}
		}
	}

	log.Printf("Smell detection complete: %d smells found", smellResults)
	return nil
}

// contains is a helper to check if a string contains a substring.
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
