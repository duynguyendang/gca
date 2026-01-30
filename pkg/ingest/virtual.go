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

	// 1. Discover BE/FE Sets
	feSet := make(map[string]bool)
	beSet := make(map[string]bool)

	resFE, _ := s.Query(ctx, `triples(?f, "has_tag", "frontend"), triples(?f, "defines", ?s)`)
	for _, r := range resFE {
		id, _ := r["?s"].(string)
		feSet[id] = true
	}
	resBE, _ := s.Query(ctx, `triples(?f, "has_tag", "backend"), triples(?f, "defines", ?s)`)
	for _, r := range resBE {
		id, _ := r["?s"].(string)
		beSet[id] = true
	}

	// Also include the files themselves in the sets for content scanning
	resFEFiles, _ := s.Query(ctx, `triples(?f, "has_tag", "frontend")`)
	for _, r := range resFEFiles {
		id, _ := r["?f"].(string)
		feSet[id] = true
	}
	resBEFiles, _ := s.Query(ctx, `triples(?f, "has_tag", "backend")`)
	for _, r := range resBEFiles {
		id, _ := r["?f"].(string)
		beSet[id] = true
	}
	// 2. Discover Route Mappings dynamically from router files
	routeMap := make(map[string]string)
	symbolLookup := make(map[string]string)

	isTagged := func(id string, set map[string]bool) bool {
		if set[id] {
			return true // Direct match (symbol or file)
		}
		// Fallback: Check file part
		parts := strings.Split(id, ":")
		if len(parts) > 1 {
			return set[parts[0]]
		}
		return false
	}

	fmt.Printf("[Virtual] feSet size: %d, beSet size: %d\n", len(feSet), len(beSet))

	scanFileDefs := func(fileID string) {
		qDef := fmt.Sprintf(`triples("%s", "defines", ?s)`, fileID)
		res, err := s.Query(ctx, qDef)
		if err == nil {
			for _, r := range res {
				id, _ := r["?s"].(string)
				parts := strings.Split(id, ":")
				if len(parts) > 1 {
					name := parts[len(parts)-1]
					if idx := strings.LastIndex(name, "."); idx != -1 {
						name = name[idx+1:]
					}
					symbolLookup[name] = id
				}
			}
		}
	}

	for id := range beSet {
		// Only scan symbol definitions, not the files themselves
		if !strings.Contains(id, ":") {
			scanFileDefs(id)
		}
	}

	for id := range beSet {
		// Only scan files for router logic
		if strings.Contains(id, ":") {
			continue
		}
		doc, err := s.GetDocument(meb.DocumentID(id))
		if err != nil {
			continue
		}
		content := string(doc.Content)
		if !strings.Contains(content, ".router.") {
			continue
		}

		fmt.Printf("[Virtual] Scanning router file: %s\n", id)

		lines := strings.Split(content, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if !strings.Contains(line, ".router.") {
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

			// Extract handler: after the last comma or "s."
			var handler string
			sIdx := strings.LastIndex(line, "s.")
			if sIdx != -1 {
				handler = line[sIdx+2:]
			} else {
				// Fallback to last segment after comma
				commaIdx := strings.LastIndex(line, ",")
				if commaIdx != -1 {
					handler = strings.TrimSpace(line[commaIdx+1:])
				}
			}

			if handler == "" {
				continue
			}

			// Strip trailing characters like )
			handlerParts := strings.FieldsFunc(handler, func(r rune) bool {
				return r == ')' || r == ',' || r == ' ' || r == ';'
			})
			if len(handlerParts) == 0 {
				continue
			}
			handlerToken := handlerParts[0]

			// Try to find the handler symbol
			if targetID, ok := symbolLookup[handlerToken]; ok {
				routeMap[route] = targetID
				fmt.Printf("[Virtual] Linked %s --(handled_by)--> %s\n", route, targetID)
				s.AddFact(meb.Fact{
					Subject:   meb.DocumentID(route),
					Predicate: meb.PredHandledBy,
					Object:    targetID,
					Graph:     "virtual",
				})
				s.AddFact(meb.Fact{
					Subject:   meb.DocumentID(route),
					Predicate: meb.PredType,
					Object:    "api_route",
					Graph:     "virtual",
				})
			}
		}
	}

	if len(routeMap) == 0 {
		fmt.Printf("[Virtual] WARNING: No routes discovered. FE-BE bridging will be limited.\n")
	}

	fmt.Printf("[Virtual] Discovered %d routes.\n", len(routeMap))

	// 2. Scan Frontend Services for fetch calls
	// We look for any symbol in frontend files that contains 'fetch' keywords
	q := `triples(?s, "type", "function")`
	results, err := s.Query(ctx, q)
	if err != nil {
		return err
	}

	fmt.Printf("[Virtual] Scanning %d functions for API calls...\n", len(results))
	count := 0

	for _, res := range results {
		sID, ok := res["?s"].(string)
		if !ok || !feSet[sID] {
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

		for route := range routeMap {
			// Check if content mentions specific route
			// We check for the precise string path "/v1/..."
			if strings.Contains(content, route) {
				// Create Virtual Edge
				fmt.Printf("[Virtual] Linked %s --(calls_api)--> %s\n", sID, route)

				// Add 'calls_api' edge
				s.AddFact(meb.Fact{
					Subject:   meb.DocumentID(sID),
					Predicate: meb.PredCallsAPI,
					Object:    route,
					Graph:     "virtual",
				})

				count++
			}
		}
	}

	// 3. Internal Service Linker (Backend -> Backend)
	// Heuristic: Link `s.graphService.Method` in handlers to `pkg/service` methods

	// A. Gather all Handler candidates from backend files
	type HandlerInfo struct {
		ID      string
		Content string
	}
	var handlers []HandlerInfo

	for id := range beSet {
		// Only scan files, not symbols
		if strings.Contains(id, ":") {
			continue
		}

		doc, err := s.GetDocument(meb.DocumentID(id))
		if err != nil {
			continue
		}
		content := string(doc.Content)

		// If it looks like it contains handler-to-service calls
		if strings.Contains(content, "s.graphService.") {
			// Find all symbols defined in this file
			qDef := fmt.Sprintf(`triples("%s", "defines", ?s)`, id)
			res, err := s.Query(ctx, qDef)
			if err == nil {
				for _, r := range res {
					sID, _ := r["?s"].(string)
					// We use the same content for all symbols in the file for linking
					handlers = append(handlers, HandlerInfo{
						ID:      sID,
						Content: content,
					})
				}
			}
		}
	}
	fmt.Printf("[Virtual] Found %d handler symbols to scan across backend files.\n", len(handlers))

	// B. Gather all Service method candidates (Functions AND Methods)
	var serviceMethods []string

	// Query Methods
	qMethods := `triples(?s, "type", "method")`
	resMethods, err := s.Query(ctx, qMethods)
	if err == nil {
		for _, r := range resMethods {
			id, _ := r["?s"].(string)
			if isTagged(id, beSet) {
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
			if isTagged(id, beSet) {
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

	// 4. Scan for internal FE calls (Symbol to Symbol)
	fmt.Printf("[Virtual] Scanning for internal FE-FE calls (Optimized)...\n")
	feIndex := make(map[string][]string) // ShortName -> []FullID

	// Build index locally to handle potential name collisions
	for _, res := range results {
		sID, ok := res["?s"].(string)
		if !ok || !isTagged(sID, feSet) {
			continue
		}
		parts := strings.Split(sID, ":")
		if len(parts) > 1 {
			short := parts[len(parts)-1]
			feIndex[short] = append(feIndex[short], sID)
		}
	}

	// Helper to check byte for symbol character (A-Z, a-z, 0-9, _, $)
	isSymbolChar := func(b byte) bool {
		return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' || b == '$'
	}

	for _, res := range results {
		sID, ok := res["?s"].(string)
		if !ok || !isTagged(sID, feSet) {
			continue
		}

		doc, err := s.GetDocument(meb.DocumentID(sID))
		if err != nil {
			continue
		}

		content := string(doc.Content)
		n := len(content)

		// Scan content once
		for i := 0; i < n; i++ {
			// Find start of symbol
			if !isSymbolChar(content[i]) {
				continue
			}
			start := i
			// Consume symbol
			for i < n && isSymbolChar(content[i]) {
				i++
			}
			token := content[start:i]

			// Check index
			if targets, exists := feIndex[token]; exists {
				// Check for call pattern: token followed by '(' (ignoring whitespace?)
				// Original logic was strict short+"("
				// We'll stick to strict for now to match behavior, but token scanning naturally isolates token.
				// Actually, strict short+"(" implies token IS followed by (.

				// Peek ahead for (
				if i < n && content[i] == '(' {
					for _, targetID := range targets {
						if sID == targetID {
							continue
						}
						s.AddFact(meb.Fact{
							Subject:   meb.DocumentID(sID),
							Predicate: "calls",
							Object:    targetID,
							Graph:     "virtual",
						})
						count++
					}
				}
			}
			// Don't skip back, i is at end of token. Loop will increment i?
			// Wait, loop has i++. I need to decrement i or change loop.
			i-- // Adjust for loop increment
		}
	}

	fmt.Printf("[Virtual] Added %d total virtual edges.\n", count)
	return nil
}
