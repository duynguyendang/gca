package ingest

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
)

func EnhanceVirtualTriples(s *meb.MEBStore) error {
	feSet := make(map[string]bool)
	beSet := make(map[string]bool)

	for fact, err := range s.Scan("", config.PredicateHasTag, "frontend") {
		if err != nil {
			continue
		}
		feSet[fact.Subject] = true
	}

	for fact, err := range s.Scan("", config.PredicateDefines, "") {
		if err != nil {
			continue
		}
		obj, ok := fact.Object.(string)
		if !ok {
			continue
		}
		if feSet[fact.Subject] {
			feSet[obj] = true
		}
	}

	for fact, err := range s.Scan("", config.PredicateHasTag, "backend") {
		if err != nil {
			continue
		}
		beSet[fact.Subject] = true
	}

	for fact, err := range s.Scan("", config.PredicateDefines, "") {
		if err != nil {
			continue
		}
		obj, ok := fact.Object.(string)
		if !ok {
			continue
		}
		if beSet[fact.Subject] {
			beSet[obj] = true
		}
	}

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

	for id := range beSet {
		if strings.Contains(id, ":") {
			continue
		}
		for fact, err := range s.Scan(id, config.PredicateDefines, "") {
			if err != nil {
				continue
			}
			sID, ok := fact.Object.(string)
			if !ok {
				continue
			}
			name := common.ExtractSymbolName(sID)
			symbolLookup[name] = sID
		}
	}

	routeRegex := regexp.MustCompile(`\.(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)\(\s*"([^"]+)"\s*,\s*([^,\)]+)`)

	for id := range beSet {
		if strings.Contains(id, ":") {
			continue
		}
		doc, err := s.GetContentByKey(string(id))
		if err != nil {
			continue
		}
		content := string(doc)
		if !strings.Contains(content, "gin.Default") && !strings.Contains(content, "gin.New") && !strings.Contains(content, ".Group") && !strings.Contains(content, "Router") {
			continue
		}

		matches := routeRegex.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			route := match[2]
			rawHandler := strings.TrimSpace(match[3])

			handlerToken := rawHandler
			if idx := strings.LastIndex(rawHandler, "."); idx != -1 {
				handlerToken = rawHandler[idx+1:]
			}

			handlerToken = strings.Trim(handlerToken, " ),;")

			if targetID, ok := symbolLookup[handlerToken]; ok {
				routeMap[route] = targetID
				s.AddFact(meb.Fact{Subject: string(route), Predicate: config.PredicateHandledBy, Object: targetID})
				s.AddFact(meb.Fact{Subject: string(targetID), Predicate: config.PredicateHasRole, Object: config.RoleAPIHandler})
			} else {
				fmt.Printf("[Virtual] Failed to link route %s to handler %s (token: %s). Symbol not found.\n", route, rawHandler, handlerToken)
			}
		}
	}

	for fact, err := range s.Scan("", config.PredicateReferences, "") {
		if err != nil {
			continue
		}
		sID := fact.Subject
		ref, ok := fact.Object.(string)
		if !ok {
			continue
		}
		cleanRef := ref
		if idx := strings.Index(ref, "?"); idx != -1 {
			cleanRef = ref[:idx]
		}
		if _, exists := routeMap[cleanRef]; exists {
			s.AddFact(meb.Fact{Subject: string(sID), Predicate: config.PredicateCallsAPI, Object: cleanRef})
			targetID := routeMap[cleanRef]
			s.AddFact(meb.Fact{Subject: string(sID), Predicate: config.PredicateCalls, Object: targetID})
		}
	}

	type FileInfo struct {
		ID      string
		Content string
		Symbols []string
	}
	var files []FileInfo
	for id := range beSet {
		if strings.Contains(id, ":") {
			continue
		}
		doc, err := s.GetContentByKey(string(id))
		if err == nil {
			content := string(doc)
			var symbols []string
			for fact, err := range s.Scan(id, config.PredicateDefines, "") {
				if err != nil {
					continue
				}
				obj, ok := fact.Object.(string)
				if ok {
					symbols = append(symbols, obj)
				}
			}
			if len(symbols) > 0 {
				files = append(files, FileInfo{ID: id, Content: content, Symbols: symbols})
			}
		}
	}

	methodIndex := make(map[string][]string)
	for fact, err := range s.Scan("", config.PredicateType, "method") {
		if err != nil {
			continue
		}
		id := fact.Subject
		if beSet[id] || isTagged(id, beSet) {
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
	methodCallRegex := regexp.MustCompile(`\.([A-Za-z0-9_]+)\(`)

	for _, f := range files {
		calledMethods := make(map[string]bool)
		matches := methodCallRegex.FindAllStringSubmatch(f.Content, -1)
		for _, m := range matches {
			if len(m) > 1 {
				calledMethods[m[1]] = true
			}
		}

		for methodName, svcIDs := range methodIndex {
			if calledMethods[methodName] {
				for _, svcID := range svcIDs {
					if f.ID != svcID {
						s.AddFact(meb.Fact{Subject: f.ID, Predicate: config.PredicateCalls, Object: svcID})
					}
				}
			}
		}
	}

	contractMap := make(map[string][]string)
	for fact, err := range s.Scan("", config.PredicateHasRole, config.RoleDataContract) {
		if err != nil {
			continue
		}
		sID := fact.Subject
		name := common.ExtractSymbolName(sID)
		contractMap[name] = append(contractMap[name], sID)
	}
	fmt.Println("[Virtual] Scanning for Data Lineage...")
	for _, f := range files {
		for modelName, targets := range contractMap {
			if strings.Contains(f.Content, modelName) {
				for _, tID := range targets {
					if f.ID != tID {
						s.AddFact(meb.Fact{Subject: f.ID, Predicate: config.PredicateExposesModel, Object: tID})
					}
				}
			}
		}
	}

	for id := range feSet {
		if strings.Contains(id, ":") {
			continue
		}
		base := strings.TrimSuffix(filepath.Base(id), filepath.Ext(id))
		for fact, err := range s.Scan(id, config.PredicateDefines, "") {
			if err != nil {
				continue
			}
			sID, ok := fact.Object.(string)
			if !ok {
				continue
			}
			if strings.EqualFold(filepath.Base(strings.Split(sID, ":")[1]), base) {
				s.AddFact(meb.Fact{Subject: string(id), Predicate: config.PredicateExports, Object: sID})
			}
		}
	}

	return nil
}
