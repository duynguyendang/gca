package service

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/common/errors"
	"github.com/duynguyendang/gca/pkg/export"
	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/repl"
)

// ProjectStoreManager interface abstraction
type ProjectStoreManager interface {
	GetStore(projectID string) (*meb.MEBStore, error)
	ListProjects() ([]manager.ProjectMetadata, error)
}

// GraphService handles graph query and enrichment operations.
type GraphService struct {
	manager ProjectStoreManager
}

// NewGraphService creates a new GraphService.
func NewGraphService(manager ProjectStoreManager) *GraphService {
	return &GraphService{manager: manager}
}

// ListProjects returns a list of available projects.
func (s *GraphService) ListProjects() ([]manager.ProjectMetadata, error) {
	return s.manager.ListProjects()
}

// ExecuteQuery executes a Datalog query for a specific project.
func (s *GraphService) ExecuteQuery(ctx context.Context, projectID, query string) ([]map[string]any, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	results, err := store.Query(ctx, query)
	if err != nil {
		// Can we differentiate invalid query vs internal error here?
		// Assuming Query returns standard errors, we might wrap them.
		return nil, fmt.Errorf("%w: %v", errors.ErrInvalidInput, err)
	}

	return results, nil
}

// ExportGraph executes a query and transforms the results into a D3 graph JSON.
// It also optionally hydrates the nodes with source code.
func (s *GraphService) ExportGraph(ctx context.Context, projectID, query string, hydrate bool, lazy bool) (*export.D3Graph, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	// 1. Execute Query
	results, err := store.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errors.ErrInvalidInput, err)
	}

	// 2. Transform to D3
	transformer := export.NewD3Transformer(store)
	graph, err := transformer.Transform(ctx, query, results)
	if err != nil {
		return nil, fmt.Errorf("%w: transformer failed: %v", errors.ErrInternal, err)
	}

	// 3. Hydrate if requested
	if hydrate && len(graph.Nodes) > 0 {
		if err := s.enrichNodes(ctx, store, graph, lazy); err != nil {
			// Log warning but return graph?
			// For service layer, we should return error or handle partials.
			// Let's return error to be explicit.
			return nil, fmt.Errorf("%w: hydration failed: %v", errors.ErrInternal, err)
		}
	}

	return graph, nil
}

// enrichNodes populates node content and kind from the store.
func (s *GraphService) enrichNodes(ctx context.Context, store *meb.MEBStore, graph *export.D3Graph, lazy bool) error {
	ids := make([]meb.DocumentID, len(graph.Nodes))
	for i, n := range graph.Nodes {
		ids[i] = meb.DocumentID(n.ID)
	}

	// Use HydrateShallow if lazy is true, assuming lazy implies we just want metadata
	// and for backbone we definitely want shallow (no children).
	// Actually, lazy=true is used for GetFileGraph too, where we might want children structure?
	// GetFileGraph calls with lazy=false for content? No, GetFileGraph uses explicit calls.
	// Let's check call sites:
	// 1. ExportGraph with hydrate=true -> calls enrichNodes(..., lazy)
	// 2. GetBackboneGraph -> calls enrichNodes(..., true)

	// If lazy is true, we assume shallow is also acceptable for performance context unless specified.
	// To be safe, let's strictly use HydrateShallow ONLY if lazy is true because
	// typically if we are lazy-loading content, we likely don't need deep children tree either
	// (e.g. Backbone or List views).
	// But `GetFileDetails` uses `ExportGraph(..., true)` (lazy) but might want children?
	// `GetFileDetails` returns internal structure, so it uses `triples(defines)` query explicitly.
	// The explicit query returns nodes. The hydration just fills metadata.
	// If the hydration *also* brings children, it duplicates the graph structure in `n.Children` which D3Transformer might not use.
	// D3Transformer builds graph from Query results. `enrichNodes` populates `n.Children` from hydration.
	// `n.Children` in D3Node is often used for hierarchical views.
	// For Backbone, we rely on DAGRE and manual clustering, not `n.Children`.
	// For FileDetails, we rely on Query nodes.
	// So `n.Children` from hydration is likely redundant or used only for specific "Expand" actions in old logic.
	// Safe to use HydrateShallow if lazy is true.

	var hydrated []meb.HydratedSymbol
	var err error

	if lazy {
		hydrated, err = store.HydrateShallow(ctx, ids, true)
	} else {
		hydrated, err = store.Hydrate(ctx, ids, false)
	}

	if err != nil {
		return err
	}

	hMap := make(map[meb.DocumentID]meb.HydratedSymbol)
	for _, h := range hydrated {
		hMap[h.ID] = h
	}

	for i := range graph.Nodes {
		n := &graph.Nodes[i]
		if h, ok := hMap[meb.DocumentID(n.ID)]; ok {
			n.Code = h.Content
			if h.Kind != "" {
				n.Kind = h.Kind
			}
			// Only map children if we have them (which HydrateShallow won't return)
			if len(h.Children) > 0 {
				n.Children = s.mapChildren(h.Children)
			}
		}
	}
	return nil
}

func (s *GraphService) mapChildren(hydrated []meb.HydratedSymbol) []export.D3Node {
	if len(hydrated) == 0 {
		return nil
	}
	nodes := make([]export.D3Node, len(hydrated))
	for i, h := range hydrated {
		// Fix Name generation first
		parts := strings.Split(string(h.ID), "/")
		name := parts[len(parts)-1]

		nodes[i] = export.D3Node{
			ID:       string(h.ID),
			Name:     name,
			Kind:     h.Kind,
			Code:     h.Content,
			Children: s.mapChildren(h.Children),
		}

		// Map Metadata if needed (Language/Group) - HydratedSymbol has Metadata map.
		if lang, ok := h.Metadata["language"].(string); ok {
			nodes[i].Language = lang
			nodes[i].Group = lang
		}
	}
	return nodes
}

// GetSource returns the content of a specific file/symbol.
func (s *GraphService) GetSource(projectID, docID string) (string, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return "", err
	}

	doc, err := store.GetDocument(meb.DocumentID(docID))
	if err != nil {
		// Assumptions regarding GetDocument error
		return "", fmt.Errorf("%w: document not found", errors.ErrNotFound)
	}

	return string(doc.Content), nil
}

// GetSymbol retrieves the full hydrated symbol (content + metadata) for a given ID.
func (s *GraphService) GetSymbol(ctx context.Context, projectID, docID string) (*meb.HydratedSymbol, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	ids := []meb.DocumentID{meb.DocumentID(docID)}
	hydrated, err := store.Hydrate(ctx, ids, false) // lazy=false to fetch content
	if err != nil {
		return nil, err
	}
	if len(hydrated) == 0 {
		return nil, fmt.Errorf("%w: symbol not found", errors.ErrNotFound)
	}

	return &hydrated[0], nil
}

// Helper to get store with error mapping
func (s *GraphService) getStore(projectID string) (*meb.MEBStore, error) {
	if projectID == "" {
		return nil, fmt.Errorf("%w: missing project ID", errors.ErrInvalidInput)
	}
	store, err := s.manager.GetStore(projectID)
	if err != nil {
		// Heuristic to detect "not found" vs internal error
		sErr := err.Error()
		if os.IsNotExist(err) || sErr == fmt.Sprintf("project not found: %s", projectID) || strings.Contains(sErr, "not found") {
			return nil, fmt.Errorf("%w: %v", errors.ErrNotFound, err)
		}
		return nil, fmt.Errorf("%w: %v", errors.ErrInternal, err)
	}
	return store, nil
}

// GenerateSummary generates a project summary.
func (s *GraphService) GenerateSummary(projectID string) (*repl.ProjectSummary, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}
	// Recalculate stats on demand? Optional.
	return repl.GenerateProjectSummary(store)
}

// GetPredicates returns known predicates.
func (s *GraphService) GetPredicates(projectID string) ([]map[string]string, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	preds, err := store.GetAllPredicates()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errors.ErrInternal, err)
	}

	var results []map[string]string
	for _, p := range preds {
		if m, ok := meb.SystemPredicates[p]; ok {
			results = append(results, map[string]string{
				"name":        p,
				"description": m.Description,
				"example":     m.Example,
			})
		}
	}
	return results, nil
}

// SearchSymbols performs symbol search.
func (s *GraphService) SearchSymbols(projectID, query, predicate string, limit int) ([]string, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 50
	}

	return store.SearchSymbols(query, limit, predicate)
}

// ListFiles returns all ingested file paths for a project.
func (s *GraphService) ListFiles(projectID string) ([]string, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	// Query for all files that have a hash
	// Use explicit predicate string to avoid depending on meb package details here if possible,
	// but meb.PredHash is cleaner.
	q := fmt.Sprintf("triples(?f, \"%s\", ?h)", meb.PredHash)
	results, err := store.Query(context.Background(), q)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errors.ErrInternal, err)
	}

	files := make([]string, 0, len(results))
	for _, res := range results {
		if f, ok := res["?f"].(string); ok {
			// clean quotes if any (datalog sometimes returns quoted strings if not careful,
			// but here they should be DocumentIDs which are strings)
			f = strings.Trim(f, "\"")
			files = append(files, f)
		}
	}
	// Sort for consistent output
	// Not strictly necessary but good for UI
	return files, nil
}

// GetFileGraph returns a composite graph for a specific file (Defines + Imports + Calls).
func (s *GraphService) GetFileGraph(ctx context.Context, projectID, fileID string) (*export.D3Graph, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	// Clean fileID (remove quotes if present, though handler should pass clean string)
	cleanFileID := strings.Trim(fileID, "\"")
	quotedFileID := fmt.Sprintf("\"%s\"", cleanFileID)

	// We will collect all results and then transform.
	// Actually, D3Transformer expects a single query string for parsing "triples" atom.
	// But here we present a unified view.
	// Hack: We can construct a synthetic result set and a dummy query for the transformer,
	// OR we can manually merge D3Graphs. Merging D3Graphs is safer.

	var mergedGraph *export.D3Graph = &export.D3Graph{
		Nodes: []export.D3Node{},
		Links: []export.D3Link{},
	}

	// Map for link deduplication
	linkMap := make(map[string]bool)

	// Helper to merge
	merge := func(query string) error {
		results, err := store.Query(ctx, query)
		if err != nil {
			return err
		}
		if len(results) == 0 {
			return nil
		}

		subGraph, err := export.ExportD3(ctx, store, query, results)
		if err != nil {
			return err
		}

		// Merge Nodes (deduplicate by ID)
		nodeMap := make(map[string]export.D3Node)
		for _, n := range mergedGraph.Nodes {
			nodeMap[n.ID] = n
		}
		for _, n := range subGraph.Nodes {
			if _, exists := nodeMap[n.ID]; !exists {
				nodeMap[n.ID] = n
				mergedGraph.Nodes = append(mergedGraph.Nodes, n)
			}
		}

		// Merge Links with Deduplication
		for _, l := range subGraph.Links {
			// Create a unique key for the link
			key := fmt.Sprintf("%s-%s-%s", l.Source, l.Relation, l.Target)
			if _, exists := linkMap[key]; !exists {
				linkMap[key] = true
				mergedGraph.Links = append(mergedGraph.Links, l)
			}
		}
		return nil
	}

	// 1. Defines: triples("file", "defines", ?s)
	q1 := fmt.Sprintf("triples(%s, \"defines\", ?s)", quotedFileID)
	if err := merge(q1); err != nil {
		return nil, fmt.Errorf("failed to get definitions: %w", err)
	}

	// 2. Imports: triples("file", "imports", ?t)
	q2 := fmt.Sprintf("triples(%s, \"imports\", ?t)", quotedFileID)
	if err := merge(q2); err != nil {
		return nil, fmt.Errorf("failed to get imports: %w", err)
	}

	// 3. All Calls from file symbols: triples(?s, "calls", ?t), triples("file", "defines", ?s)
	// This finds ALL calls originating from symbols defined in this file.
	// Includes both internal calls and calls to imported/external symbols.
	q3 := fmt.Sprintf("triples(?s, \"calls\", ?t), triples(%s, \"defines\", ?s)", quotedFileID)
	if err := merge(q3); err != nil {
		return nil, fmt.Errorf("failed to get calls: %w", err)
	}

	// 4. Hydrate
	if len(mergedGraph.Nodes) > 0 {
		// GetFileGraph usually implies full content detail, so lazy=false by default?
		// Or should we update GetFileGraph signature?
		// For now, let's keep GetFileGraph strict (lazy=false) to match current behavior.
		if err := s.enrichNodes(ctx, store, mergedGraph, false); err != nil {
			return nil, fmt.Errorf("hydration failed: %w", err)
		}
	}

	return mergedGraph, nil
}

// GetProjectMap returns a high-level view of file dependencies (imports only).
func (s *GraphService) GetProjectMap(ctx context.Context, projectID string) (*export.D3Graph, error) {
	// Query only "imports" relationships between files
	// We want triples(?s, "imports", ?o) where ?s is a file?
	// Actually typical pattern: triples(?file, "imports", ?importedPackage)
	// But usually we want file-to-file or package-to-package.
	// Task says: "Query only imports predicates between files."
	// Let's assume standard query: triples(?s, "imports", ?o)
	query := `triples(?s, "imports", ?o)`

	// Execute with hydrate=false (or lazy=true implicitly if we used hydration, but here we skip it entirely?)
	// Task says: "No Hydration: Do not fetch source code or heavy metadata."
	// So hydrate=false.
	return s.ExportGraph(ctx, projectID, query, false, false)
}

// GetFileDetails returns detailed internal structure of a file.
func (s *GraphService) GetFileDetails(ctx context.Context, projectID, fileID string) (*export.D3Graph, error) {
	cleanFileID := strings.Trim(fileID, "\"")
	quotedFileID := fmt.Sprintf("\"%s\"", cleanFileID)

	// Logic:
	// 1. defines: triples(file, "defines", ?s)
	// 2. internal calls: triples(?s, "calls", ?o) WHERE ?s defined in file AND ?o defined in file
	//    Datalog: triples(?file, "defines", ?s), triples(?s, "calls", ?o), triples(?file, "defines", ?o)
	//    (assuming ?file matches fileID)

	// We can construct a combined query or merge graphs.
	// Merging is safer for complex joins if datalog engine doesn't support complex joins well.

	// Query 1: Defines
	q1 := fmt.Sprintf(`triples(%s, "defines", ?s)`, quotedFileID)

	// Query 2: Internal Calls
	// triples(quotedFileID, "defines", ?s)
	// triples(quotedFileID, "defines", ?o)
	// triples(?s, "calls", ?o)
	// This might be too complex for simple parser?
	// Our Parse logic handles list of atoms. Store.Query handles efficient join.
	// So we can try one query:
	// triples(File, "defines", ?s), triples(File, "defines", ?o), triples(?s, "calls", ?o)
	q2 := fmt.Sprintf(`triples(%s, "defines", ?s), triples(%s, "defines", ?o), triples(?s, "calls", ?o)`, quotedFileID, quotedFileID)

	// Also maybe "type" relationships?
	// triples(?s, "type", ?o) ...

	// Let's use the helper we used in GetFileGraph to merge results if needed,
	// but GetFileGraph helper was internal variable.
	// Let's just create a new helper here or manual merge.

	mergedGraph := &export.D3Graph{Nodes: []export.D3Node{}, Links: []export.D3Link{}}

	// Run q1
	g1, err := s.ExportGraph(ctx, projectID, q1, true, true) // Lazy hydration
	if err == nil {
		mergedGraph.Nodes = append(mergedGraph.Nodes, g1.Nodes...)
		mergedGraph.Links = append(mergedGraph.Links, g1.Links...)
	}

	// Run q2
	// ExportGraph executes query then transforms.
	g2, err := s.ExportGraph(ctx, projectID, q2, false, true) // No need to hydrate here if nodes match q1?
	// Actually we should just merge results.
	if err == nil {
		// Dedup nodes
		nodeMap := make(map[string]bool)
		for _, n := range mergedGraph.Nodes {
			nodeMap[n.ID] = true
		}
		for _, n := range g2.Nodes {
			if !nodeMap[n.ID] {
				mergedGraph.Nodes = append(mergedGraph.Nodes, n)
				nodeMap[n.ID] = true
			}
		}
		mergedGraph.Links = append(mergedGraph.Links, g2.Links...)
	}

	// Post-processing: Set ParentID for all nodes in this detailed view
	for i := range mergedGraph.Nodes {
		mergedGraph.Nodes[i].ParentID = cleanFileID
	}

	return mergedGraph, nil
}

// GetBackboneGraph returns a graph containing only cross-file dependencies.
func (s *GraphService) GetBackboneGraph(ctx context.Context, projectID string) (*export.D3Graph, error) {
	// Query all calls
	query := `triples(?s, "calls", ?o)`
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	results, err := store.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errors.ErrInvalidInput, err)
	}

	// Filter for cross-file calls
	backbone := &export.D3Graph{
		Nodes: []export.D3Node{},
		Links: []export.D3Link{},
	}
	nodeSet := make(map[string]bool)

	for _, r := range results {
		srcID, ok1 := r["?s"].(string)
		tgtID, ok2 := r["?o"].(string)
		if !ok1 || !ok2 {
			continue
		}

		// IDs are typically "path/to/file:Symbol" or "path/to/file:Type.Method"
		// We split by first colon to get file path.
		srcParts := strings.SplitN(srcID, ":", 2)
		tgtParts := strings.SplitN(tgtID, ":", 2)

		if len(srcParts) < 2 || len(tgtParts) < 2 {
			continue
		}

		srcFile := srcParts[0]
		tgtFile := tgtParts[0]

		if srcFile != tgtFile {
			// This is a cross-file call
			backbone.Links = append(backbone.Links, export.D3Link{
				Source:   srcID,
				Target:   tgtID,
				Relation: "calls",
			})

			// Add nodes if not present
			if !nodeSet[srcID] {
				backbone.Nodes = append(backbone.Nodes, export.D3Node{
					ID:       srcID,
					Name:     srcParts[1], // Ideally we'd parse name better but this is fine for ID strategy
					Kind:     "gateway",   // Temporary kind, or we could hydrate to get real kind
					ParentID: srcFile,
				})
				nodeSet[srcID] = true
			}
			if !nodeSet[tgtID] {
				backbone.Nodes = append(backbone.Nodes, export.D3Node{
					ID:       tgtID,
					Name:     tgtParts[1],
					Kind:     "gateway",
					ParentID: tgtFile,
				})
				nodeSet[tgtID] = true
			}
		}
	}

	// Optional: Hydrate these nodes to get their real kind (func, method)?
	// The requirement just says "Include Entry/Exit Points".
	// Maybe we can enrich them if performance allows.
	// For now, let's try to enrich them with lazy=true to get metadata/kind if possible,
	// but purely from ID we might not know the kind.
	// Let's call enrichNodes which does hydrate(lazy=true).
	// But first we need valid DocumentIDs.
	if len(backbone.Nodes) > 0 {
		if err := s.enrichNodes(ctx, store, backbone, true); err != nil {
			// If enrichment fails, we still have the structure
			fmt.Printf("Backbone enrichment warning: %v\n", err)
		}
	}

	return backbone, nil
}
