package ingest

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
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

	// Also include the files themselves in the sets
	resFF, _ := s.Query(ctx, `triples(?f, "has_tag", "frontend")`)
	for _, r := range resFF {
		feSet[r["?f"].(string)] = true
	}
	resBF, _ := s.Query(ctx, `triples(?f, "has_tag", "backend")`)
	for _, r := range resBF {
		beSet[r["?f"].(string)] = true
	}

	// 2. Discover Route Mappings
	routeMap := make(map[string]string)
	symbolLookup := make(map[string]string)

	isTagged := func(id string, set map[string]bool) bool {
		if set[id] {
			return true
		}
		parts := strings.Split(id, ":")
		if len(parts) > 1 {
			return set[parts[0]]
		}
		return false
	}

	// Build symbol lookup for BE
	for id := range beSet {
		if strings.Contains(id, ":") {
			continue
		}
		qDef := fmt.Sprintf(`triples("%s", "defines", ?s)`, id)
		resDef, _ := s.Query(ctx, qDef)
		for _, r := range resDef {
			sID := r["?s"].(string)
			parts := strings.Split(sID, ":")
			if len(parts) > 1 {
				name := parts[len(parts)-1]
				if idx := strings.LastIndex(name, "."); idx != -1 {
					name = name[idx+1:]
				}
				symbolLookup[name] = sID
			}
		}
	}

	// Scan Router Files
	// Regex for: s.GET("/path", handler) or group.POST("/path", handler)
	// Supports: GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD
	// Captures: 1=Method, 2=Path, 3=Handler match (simple), 4=Handler token
	// Note: Go syntax is flexible, this captures common patterns.
	// We need to capture the handler symbol name.
	// Example: s.GET("/v1/projects", s.handleProjects)
	// Match: .GET, "/v1/projects", s.handleProjects
	// Scan Router Files
	// Regex for: s.GET("/path", handler) or group.POST("/path", handler)
	// Supports: GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD
	// Captures: 1=Method, 2=Path, 3=Handler match (simple)
	// Note: Go syntax is flexible, this captures common patterns.
	// We need to capture the handler symbol name.
	// Example: s.GET("/v1/projects", s.handleProjects)
	// Match: .GET, "/v1/projects", s.handleProjects
	routeRegex := regexp.MustCompile(`\.(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)\(\s*"([^"]+)"\s*,\s*([^,\)]+)`)

	for id := range beSet {
		if strings.Contains(id, ":") {
			continue
		}
		doc, err := s.GetDocument(meb.DocumentID(id))
		if err != nil {
			continue
		}
		content := string(doc.Content)
		// Basic heuristic: check if file looks like a router setup
		if !strings.Contains(content, "gin.Default") && !strings.Contains(content, "gin.New") && !strings.Contains(content, ".Group") && !strings.Contains(content, "Router") {
			continue
		}

		matches := routeRegex.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			// method := match[1] // Unused for now
			route := match[2]
			rawHandler := strings.TrimSpace(match[3])

			// Extract simple function name from regex match
			// e.g. "s.handleProjects" -> "handleProjects"
			// e.g. "handlers.GetUser" -> "GetUser"
			handlerToken := rawHandler
			if idx := strings.LastIndex(rawHandler, "."); idx != -1 {
				handlerToken = rawHandler[idx+1:]
			}

			// Clean up token (remove closing parens if regex greediness caught them, though strict regex shouldn't)
			handlerToken = strings.Trim(handlerToken, " ),;")

			if targetID, ok := symbolLookup[handlerToken]; ok {
				routeMap[route] = targetID
				s.AddFact(meb.Fact{Subject: meb.DocumentID(route), Predicate: "handled_by", Object: targetID, Graph: "virtual"})
				// REL-02: Tag the handler as an api_handler
				s.AddFact(meb.Fact{Subject: meb.DocumentID(targetID), Predicate: "has_role", Object: "api_handler", Graph: "virtual"})
			}
		}
	}

	// 3. Scan FE References for calls_api
	qRefs := `triples(?s, "references", ?ref)`
	resRefs, _ := s.Query(ctx, qRefs)
	for _, res := range resRefs {
		sID, _ := res["?s"].(string)
		ref, _ := res["?ref"].(string)
		cleanRef := ref
		if idx := strings.Index(ref, "?"); idx != -1 {
			cleanRef = ref[:idx]
		}
		if _, exists := routeMap[cleanRef]; exists {
			s.AddFact(meb.Fact{Subject: meb.DocumentID(sID), Predicate: "calls_api", Object: cleanRef, Graph: "virtual"})
			// Also add direct 'calls' link to the backend handler to enable standard pathfinding
			targetID := routeMap[cleanRef]
			s.AddFact(meb.Fact{Subject: meb.DocumentID(sID), Predicate: "calls", Object: targetID, Graph: "virtual"})
		}
	}

	// 4. Internal Service Linker (Optimized)
	type HandlerInfo struct {
		ID      string
		Content string
	}
	var handlers []HandlerInfo
	for id := range beSet {
		if strings.Contains(id, ":") {
			continue
		}
		doc, err := s.GetDocument(meb.DocumentID(id))
		if err == nil {
			content := string(doc.Content)
			qDef := fmt.Sprintf(`triples("%s", "defines", ?s)`, id)
			resDef, _ := s.Query(ctx, qDef)
			for _, r := range resDef {
				handlers = append(handlers, HandlerInfo{ID: r["?s"].(string), Content: content})
			}
		}
	}

	methodIndex := make(map[string][]string)
	qMethods := `triples(?s, "type", "method")`
	resMethods, _ := s.Query(ctx, qMethods)
	for _, r := range resMethods {
		id := r["?s"].(string)
		if isTagged(id, beSet) {
			parts := strings.Split(id, ":")
			if len(parts) > 1 {
				name := parts[1]
				if idx := strings.LastIndex(name, "."); idx != -1 {
					name = name[idx+1:]
				}
				methodIndex[name] = append(methodIndex[name], id)
			}
		}
	}

	fmt.Println("[Virtual] Scanning internal BE calls...")
	for _, h := range handlers {
		for methodName, svcIDs := range methodIndex {
			if strings.Contains(h.Content, "."+methodName+"(") {
				for _, svcID := range svcIDs {
					if h.ID != svcID {
						s.AddFact(meb.Fact{Subject: meb.DocumentID(h.ID), Predicate: "calls", Object: svcID, Graph: "virtual"})
					}
				}
			}
		}
	}

	// 5. Data Lineage (exposes_model)
	contractMap := make(map[string][]string)
	resContracts, _ := s.Query(ctx, `triples(?s, "has_role", "data_contract")`)
	for _, r := range resContracts {
		sID := r["?s"].(string)
		parts := strings.Split(sID, ":")
		if len(parts) > 1 {
			name := parts[len(parts)-1]
			contractMap[name] = append(contractMap[name], sID)
		}
	}
	fmt.Println("[Virtual] Scanning for Data Lineage...")
	for _, h := range handlers {
		for modelName, targets := range contractMap {
			if strings.Contains(h.Content, modelName) {
				for _, tID := range targets {
					s.AddFact(meb.Fact{Subject: meb.DocumentID(h.ID), Predicate: "exposes_model", Object: tID, Graph: "virtual"})
				}
			}
		}
	}

	// 6. Logical Ownership (exports)
	for id := range feSet {
		if strings.Contains(id, ":") {
			continue
		}
		base := strings.TrimSuffix(filepath.Base(id), filepath.Ext(id))
		qDef := fmt.Sprintf(`triples("%s", "defines", ?s)`, id)
		resDef, _ := s.Query(ctx, qDef)
		for _, r := range resDef {
			sID := r["?s"].(string)
			if strings.EqualFold(filepath.Base(strings.Split(sID, ":")[1]), base) {
				s.AddFact(meb.Fact{Subject: meb.DocumentID(id), Predicate: "exports", Object: sID, Graph: "virtual"})
			}
		}
	}

	return nil
}
