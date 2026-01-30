package ingest

import (
	"context"
	"fmt"
	"strings"

	"github.com/duynguyendang/gca/pkg/meb"
)

// EnhanceVirtualTriples injects "virtual" edges to bridge the gap between
// decoupled components (e.g. Frontend -> Backend via HTTP).
func EnhanceVirtualTriples(s *meb.MEBStore) error {
	ctx := context.Background()

	// 1. Discover Route Mappings dynamically from server.go
	routeMap := make(map[string]string)

	// Pre-index symbols in the server package for fast lookup
	symbolLookup := make(map[string]string)
	// Query all defined symbols in server package, not just "type" which might be missing
	// Or better: iterate all symbols in pkg/server
	// But "triples" with wildcard predicate?
	// Let's use the query that worked for Defines in Internal Linker
	// Actually, just listing all functions/methods is safer if we fix the type issue.
	// But since type might be missing, let's look at "defines" in server files if possible.
	// For now, let's stick to generic query but handle dot.

	// Issue: handleGraphPath missed "type" predicate.
	// So iteration via "type" missed it.
	// We MUST iterate via "defines" for robust discovery.

	// Helper to scan a file's definitions
	scanFileDefs := func(fileID string) {
		qDef := fmt.Sprintf(`triples("%s", "defines", ?s)`, fileID)
		res, err := s.Query(ctx, qDef)
		if err == nil {
			for _, r := range res {
				id, _ := r["?s"].(string)
				// Extract name
				parts := strings.Split(id, ":")
				if len(parts) > 1 {
					name := parts[len(parts)-1]
					// Handle Receiver.Method or .Method
					if idx := strings.LastIndex(name, "."); idx != -1 {
						name = name[idx+1:]
					}
					symbolLookup[name] = id
				}
			}
		}
	}

	// Scan key server files
	scanFileDefs("gca/pkg/server/server.go")
	scanFileDefs("gca/pkg/server/handlers.go")

	serverDoc, err := s.GetDocument("gca/pkg/server/server.go")
	if err == nil {
		content := string(serverDoc.Content)
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if !strings.Contains(line, "s.router.") {
				continue
			}
			// Extract route: between the first dual-quotes
			quoteIdx := strings.Index(line, "\"")
			if quoteIdx == -1 {
				continue
			}
			rest := line[quoteIdx+1:]
			endQuoteIdx := strings.Index(rest, "\"")
			if endQuoteIdx == -1 {
				continue
			}
			route := rest[:endQuoteIdx]

			// Extract handler: after the last comma and "s."
			sIdx := strings.LastIndex(line, "s.")
			if sIdx == -1 {
				continue
			}
			handler := line[sIdx+2:]
			// Strip trailing characters like )
			handlerParts := strings.FieldsFunc(handler, func(r rune) bool {
				return r == ')' || r == ',' || r == ' ' || r == ';'
			})
			if len(handlerParts) == 0 {
				continue
			}
			handler = handlerParts[0]

			if id, ok := symbolLookup[handler]; ok {
				routeMap[route] = id
			}
		}
	}

	// Fallback/Hardcoded defaults if discovery failed
	if len(routeMap) == 0 {
		routeMap = map[string]string{
			"/v1/graph/path": "gca/pkg/server/handlers.go:handleGraphPath",
			"/v1/query":      "gca/pkg/server/handlers.go:handleQuery",
		}
	}

	fmt.Printf("[Virtual] Discovered %d routes from server.go\n", len(routeMap))

	// 2. Scan Frontend Services for fetch calls
	// We look for any symbol in 'gca-fe/services' that contains 'fetch' keyworks
	q := `triples(?s, "type", "function")`
	results, err := s.Query(ctx, q)
	if err != nil {
		return err
	}

	fmt.Printf("[Virtual] Scanning %d functions for API calls...\n", len(results))
	count := 0

	for _, res := range results {
		sID, ok := res["?s"].(string)
		if !ok || !strings.Contains(sID, "gca-fe/services") {
			continue
		}

		// Get Content
		doc, err := s.GetDocument(meb.DocumentID(sID))
		if err != nil {
			continue
		}

		// Regex to find url patterns
		// Look for: url = `${cleanBase}/v1/graph/path...`
		// or fetch(..., "/v1/...")
		content := string(doc.Content)

		for route, handlerID := range routeMap {
			// Check if content mentions specific route
			// We check for the precise string path "/v1/..."
			if strings.Contains(content, route) {
				// Create Virtual Edge
				fmt.Printf("[Virtual] Linked %s -> %s (via %s)\n", sID, handlerID, route)

				// Add 'calls' edge
				s.AddFact(meb.Fact{
					Subject:   meb.DocumentID(sID), // meb.DocumentID is alias for string? No, Fact.Subject is DocumentID.
					Predicate: "calls",
					Object:    handlerID,
					Graph:     "virtual",
				})

				// Add 'http_route' edge for metadata?
				// Maybe later.
				count++
			}
		}
	}

	// 3. Internal Service Linker (Backend -> Backend)
	// Heuristic: Link `s.graphService.Method` in handlers to `pkg/service` methods

	// A. Gather all Handler functions in pkg/server/handlers.go
	type HandlerInfo struct {
		ID      string
		Content string
	}
	var handlers []HandlerInfo

	// Use "defines" to find symbols in handlers.go
	// Note: We need the full ID "gca/pkg/server/handlers.go"
	qHandlersDef := `triples("gca/pkg/server/handlers.go", "defines", ?s)`
	resHandlersDef, err := s.Query(ctx, qHandlersDef)
	if err == nil {
		for _, r := range resHandlersDef {
			id, _ := r["?s"].(string)
			// Optional: verify it's a function or method?
			// We can just try to get content. faster.
			doc, err := s.GetDocument(meb.DocumentID(id))
			if err == nil {
				handlers = append(handlers, HandlerInfo{
					ID:      id,
					Content: string(doc.Content),
				})
			}
		}
	}
	fmt.Printf("[Virtual] Found %d handlers to scan via defines.\n", len(handlers))

	// B. Gather all Service method candidates (Functions AND Methods)
	var serviceMethods []string

	// Query Methods
	qMethods := `triples(?s, "type", "method")`
	resMethods, err := s.Query(ctx, qMethods)
	if err == nil {
		for _, r := range resMethods {
			id, _ := r["?s"].(string)
			if strings.Contains(id, "pkg/service/") {
				serviceMethods = append(serviceMethods, id)
			}
		}
	}
	// Query Functions
	qFuncs := `triples(?s, "type", "function")`
	resFuncs, err := s.Query(ctx, qFuncs)
	if err == nil {
		for _, r := range resFuncs {
			id, _ := r["?s"].(string)
			if strings.Contains(id, "pkg/service/") {
				serviceMethods = append(serviceMethods, id)
			}
		}
	}

	for _, svcID := range serviceMethods {
		// Extract method name
		parts := strings.Split(svcID, ":")
		if len(parts) < 2 {
			continue
		}
		methodName := parts[1]
		// Remove receiver part if present (e.g. *GraphService.FindShortestPath)
		// Format could be: file.go:Receiver.Method or file.go:.Method
		if idx := strings.LastIndex(methodName, "."); idx != -1 {
			methodName = methodName[idx+1:]
		}

		// Check against all handlers
		callPattern := "s.graphService." + methodName

		for _, h := range handlers {
			if strings.Contains(h.Content, callPattern) {
				fmt.Printf("[Virtual] Internal Link: %s -> %s\n", h.ID, svcID)
				s.AddFact(meb.Fact{
					Subject:   meb.DocumentID(h.ID),
					Predicate: "calls",
					Object:    svcID,
					Graph:     "virtual",
				})
			}
		}
	}

	fmt.Printf("[Virtual] Injected internal service links.\n")
	return nil
}
