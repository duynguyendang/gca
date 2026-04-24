package ingest

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

// safeAddFact checks if a fact exists before adding to avoid duplicates on re-run.
// Returns true if fact was added, false if it already existed.
func safeAddFact(s *meb.MEBStore, subj, pred string, obj any) bool {
	ctx := context.Background()
	for item, err := range s.ScanContext(ctx, subj, pred, "") {
		if err != nil {
			continue
		}
		if item.Subject == subj && item.Predicate == pred {
			return false // already exists
		}
	}
	s.AddFact(meb.Fact{Subject: subj, Predicate: pred, Object: obj})
	return true
}

// upsertFact adds a fact, replacing any existing fact with same subject/predicate.
// For route->handler facts that should be deterministic on re-run.
func upsertFact(s *meb.MEBStore, subj, pred string, obj any) {
	s.DeleteFactsBySubject(subj)
	s.AddFact(meb.Fact{Subject: subj, Predicate: pred, Object: obj})
}

// getTagConfig returns the active tagging rules for this project.
// Falls back to default rules if no project-specific config is available.
func getTagConfig() *config.ProjectTagConfig {
	// Future: load from gca.yaml per project via LoadTagRulesFromYAML
	rules := config.DefaultTagRules()
	return &config.ProjectTagConfig{Rules: rules}
}

// shouldInjectTag checks if a file already has the given tag.
func shouldInjectTag(s *meb.MEBStore, fileID, tag string) bool {
	for fact, err := range s.Scan(fileID, config.PredicateHasTag, tag) {
		if err != nil {
			continue
		}
		_ = fact
		return false // already tagged
	}
	return true
}

// configDrivenTagMatcher iterates over the regex rules and injects matching tags.
func configDrivenTagMatcher(s *meb.MEBStore, fileID string, tagCfg *config.ProjectTagConfig) {
	if strings.Contains(fileID, ":") {
		return // Skip symbol-level IDs, only tag files
	}

	for _, tag := range tagCfg.MatchingTags(fileID) {
		if shouldInjectTag(s, fileID, tag) {
			s.AddFact(meb.Fact{Subject: fileID, Predicate: config.PredicateHasTag, Object: tag})
			logger.Debug("Config-driven tag injection", "file", fileID, "tag", tag)
		}
	}
}

func EnhanceVirtualTriples(s *meb.MEBStore) error {
	tagCfg := getTagConfig()

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

	// Route detection via regex
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
				upsertFact(s, string(route), config.PredicateHandledBy, targetID)
				upsertFact(s, string(targetID), config.PredicateHasRole, config.RoleAPIHandler)
			} else {
				logger.Warn("Failed to link route to handler", "route", route, "handler", rawHandler, "token", handlerToken)
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
			targetID := routeMap[cleanRef]
			safeAddFact(s, string(sID), config.PredicateCallsAPI, cleanRef)
			safeAddFact(s, string(sID), config.PredicateCalls, targetID)
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

	logger.Info("Scanning internal BE calls")
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
						safeAddFact(s, f.ID, config.PredicateCalls, svcID)
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
	logger.Info("Scanning for Data Lineage")
	for _, f := range files {
		for modelName, targets := range contractMap {
			if strings.Contains(f.Content, modelName) {
				for _, tID := range targets {
					if f.ID != tID {
						safeAddFact(s, f.ID, config.PredicateExposesModel, tID)
					}
				}
			}
		}
	}

	for id := range feSet {
		if strings.Contains(id, ":") {
			continue
		}
		base := strings.TrimSuffix(common.ExtractSymbolName(id), filepath.Ext(id))
		for fact, err := range s.Scan(id, config.PredicateDefines, "") {
			if err != nil {
				continue
			}
			sID, ok := fact.Object.(string)
			if !ok {
				continue
			}
			if strings.EqualFold(common.ExtractSymbolName(sID), base) {
				safeAddFact(s, string(id), config.PredicateExports, sID)
			}
		}
	}

	logger.Info("Injecting architectural tags for security smell detection")
	for id := range beSet {
		configDrivenTagMatcher(s, id, tagCfg)
	}
	for id := range feSet {
		configDrivenTagMatcher(s, id, tagCfg)
	}

	return nil
}