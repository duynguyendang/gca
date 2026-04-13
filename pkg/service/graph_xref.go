package service

import (
	"context"
	"fmt"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/export"
	"github.com/duynguyendang/gca/pkg/ingest"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
)

func (s *GraphService) GetCallers(ctx context.Context, projectID, symbolID string, maxDepth int) ([]string, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	symbolID = symbolID

	resolver := ingest.NewSymbolResolver(store)
	cg, err := resolver.BuildCallGraph(store)
	if err != nil {
		return nil, fmt.Errorf("failed to build call graph: %w", err)
	}

	if maxDepth <= 0 {
		maxDepth = 3
	}
	if maxDepth > 10 {
		maxDepth = 10
	}

	return cg.GetCallersRecursive(symbolID, maxDepth), nil
}

func (s *GraphService) GetCallees(ctx context.Context, projectID, symbolID string, maxDepth int) ([]string, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	symbolID = symbolID

	resolver := ingest.NewSymbolResolver(store)
	cg, err := resolver.BuildCallGraph(store)
	if err != nil {
		return nil, fmt.Errorf("failed to build call graph: %w", err)
	}

	if maxDepth <= 0 {
		maxDepth = 3
	}
	if maxDepth > 10 {
		maxDepth = 10
	}

	return cg.GetCalleesRecursive(symbolID, maxDepth), nil
}

func (s *GraphService) GetWhoCalls(ctx context.Context, projectID, symbolID string, maxDepth int) (*export.D3Graph, error) {
	callers, err := s.GetCallers(ctx, projectID, symbolID, maxDepth)
	if err != nil {
		return nil, err
	}

	graph := &export.D3Graph{
		Nodes: []export.D3Node{},
		Links: []export.D3Link{},
	}
	nodeSet := make(map[string]bool)

	for _, caller := range callers {
		if !nodeSet[caller] {
			parts := splitSymbolID(caller)
			kind := config.SymbolKindSymbol
			parentID := ""
			if len(parts) >= 2 {
				parentID = parts[0]
				kind = guessKind(parts[1])
			}
			graph.Nodes = append(graph.Nodes, export.D3Node{
				ID:       caller,
				Name:     extractName(caller),
				Kind:     kind,
				ParentID: parentID,
			})
			nodeSet[caller] = true
		}

		if !nodeSet[symbolID] {
			parts := splitSymbolID(symbolID)
			kind := config.SymbolKindSymbol
			parentID := ""
			if len(parts) >= 2 {
				parentID = parts[0]
				kind = guessKind(parts[1])
			}
			graph.Nodes = append(graph.Nodes, export.D3Node{
				ID:       symbolID,
				Name:     extractName(symbolID),
				Kind:     kind,
				ParentID: parentID,
			})
			nodeSet[symbolID] = true
		}

		graph.Links = append(graph.Links, export.D3Link{
			Source:   caller,
			Target:   symbolID,
			Relation: config.PredicateCalledBy,
			Type:     "backward",
		})
	}

	return graph, nil
}

func (s *GraphService) GetWhatCalls(ctx context.Context, projectID, symbolID string, maxDepth int) (*export.D3Graph, error) {
	callees, err := s.GetCallees(ctx, projectID, symbolID, maxDepth)
	if err != nil {
		return nil, err
	}

	graph := &export.D3Graph{
		Nodes: []export.D3Node{},
		Links: []export.D3Link{},
	}
	nodeSet := make(map[string]bool)

	if !nodeSet[symbolID] {
		parts := splitSymbolID(symbolID)
		kind := config.SymbolKindSymbol
		parentID := ""
		if len(parts) >= 2 {
			parentID = parts[0]
			kind = guessKind(parts[1])
		}
		graph.Nodes = append(graph.Nodes, export.D3Node{
			ID:       symbolID,
			Name:     extractName(symbolID),
			Kind:     kind,
			ParentID: parentID,
		})
		nodeSet[symbolID] = true
	}

	for _, callee := range callees {
		if !nodeSet[callee] {
			parts := splitSymbolID(callee)
			kind := config.SymbolKindSymbol
			parentID := ""
			if len(parts) >= 2 {
				parentID = parts[0]
				kind = guessKind(parts[1])
			}
			graph.Nodes = append(graph.Nodes, export.D3Node{
				ID:       callee,
				Name:     extractName(callee),
				Kind:     kind,
				ParentID: parentID,
			})
			nodeSet[callee] = true
		}

		graph.Links = append(graph.Links, export.D3Link{
			Source:   symbolID,
			Target:   callee,
			Relation: config.PredicateCalls,
			Type:     "forward",
		})
	}

	return graph, nil
}

func (s *GraphService) CheckReachability(ctx context.Context, projectID, fromID, toID string, maxDepth int) (bool, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return false, err
	}

	resolver := ingest.NewSymbolResolver(store)
	cg, err := resolver.BuildCallGraph(store)
	if err != nil {
		return false, fmt.Errorf("failed to build call graph: %w", err)
	}

	if maxDepth <= 0 {
		maxDepth = 5
	}
	if maxDepth > 20 {
		maxDepth = 20
	}

	return cg.FindReachable(fromID, toID, maxDepth), nil
}

func (s *GraphService) DetectCycles(ctx context.Context, projectID string) ([][]string, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	resolver := ingest.NewSymbolResolver(store)
	cg, err := resolver.BuildCallGraph(store)
	if err != nil {
		return nil, fmt.Errorf("failed to build call graph: %w", err)
	}

	return cg.DetectCycles(), nil
}

func (s *GraphService) FindLCA(ctx context.Context, projectID, symbolA, symbolB string, maxDepth int) (string, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return "", err
	}

	resolver := ingest.NewSymbolResolver(store)
	cg, err := resolver.BuildCallGraph(store)
	if err != nil {
		return "", fmt.Errorf("failed to build call graph: %w", err)
	}

	if maxDepth <= 0 {
		maxDepth = 10
	}
	if maxDepth > 30 {
		maxDepth = 30
	}

	return cg.LeastCommonAncestor(symbolA, symbolB, maxDepth), nil
}

func (s *GraphService) EnrichWithCalledBy(ctx context.Context, projectID string) error {
	store, err := s.getStore(projectID)
	if err != nil {
		return err
	}

	resolver := ingest.NewSymbolResolver(store)
	cg, err := resolver.BuildCallGraph(store)
	if err != nil {
		return fmt.Errorf("failed to build call graph: %w", err)
	}

	return ingest.AddResolvedCallsAsCalledBy(store, cg)
}

func splitSymbolID(id string) []string {
	var parts []string
	for i := 0; i < len(id); i++ {
		if id[i] == ':' {
			parts = append(parts, id[:i])
			parts = append(parts, id[i+1:])
			return parts
		}
	}
	return []string{id}
}

func extractName(id string) string {
	parts := splitSymbolID(id)
	if len(parts) >= 2 {
		name := parts[1]
		if idx := len(name) - 1; idx > 0 && name[idx] == ')' {
			if openIdx := stringsIndex(name, "("); openIdx >= 0 {
				return name[:openIdx]
			}
		}
		return name
	}
	return id
}

func stringsIndex(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func guessKind(symbolName string) string {
	if stringsHasPrefix(symbolName, "New") ||
		stringsHasPrefix(symbolName, "Create") ||
		stringsHasPrefix(symbolName, "Get") ||
		stringsHasPrefix(symbolName, "Load") {
		return config.SymbolKindFunc
	}
	if idx := stringsIndex(symbolName, "("); idx > 0 {
		return config.SymbolKindFunc
	}
	if len(symbolName) > 0 && (symbolName[0] >= 'A' && symbolName[0] <= 'Z') {
		return config.SymbolKindStruct
	}
	return config.SymbolKindSymbol
}

func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func (s *GraphService) QueryCalledBy(ctx context.Context, projectID, symbolID string) ([]map[string]any, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`triples(?caller, "%s", "%s")`, config.PredicateCalledBy, symbolID)
	return gcamdb.Query(ctx, store, query)
}

func (s *GraphService) QueryCalls(ctx context.Context, projectID, symbolID string) ([]map[string]any, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`triples("%s", "%s", ?callee)`, symbolID, config.PredicateCalls)
	return gcamdb.Query(ctx, store, query)
}
