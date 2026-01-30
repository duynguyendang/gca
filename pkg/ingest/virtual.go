package ingest

import (
	"context"
	"fmt"
	"path/filepath"
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
	for id := range beSet {
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

		lines := strings.Split(content, "\n")
		for _, line := range lines {
			if !strings.Contains(line, ".router.") {
				continue
			}
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

			var handler string
			if sIdx := strings.LastIndex(line, "s."); sIdx != -1 {
				handler = line[sIdx+2:]
			} else if commaIdx := strings.LastIndex(line, ","); commaIdx != -1 {
				handler = strings.TrimSpace(line[commaIdx+1:])
			}

			if handler == "" {
				continue
			}
			handlerToken := strings.FieldsFunc(handler, func(r rune) bool {
				return r == ')' || r == ',' || r == ' ' || r == ';'
			})[0]

			if targetID, ok := symbolLookup[handlerToken]; ok {
				routeMap[route] = targetID
				s.AddFact(meb.Fact{Subject: meb.DocumentID(route), Predicate: "handled_by", Object: targetID, Graph: "virtual"})
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
