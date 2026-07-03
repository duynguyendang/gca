package ingest

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

var virtualFactMu sync.Mutex

// safeAddFact checks if a fact exists before adding to avoid duplicates on re-run.
// Returns true if fact was added, false if it already existed.
func safeAddFact(s Store, subj, pred string, obj any) bool {
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
// Protected by mutex to ensure atomic operations.
func upsertFact(s Store, subj, pred string, obj any) {
	virtualFactMu.Lock()
	defer virtualFactMu.Unlock()
	// Check if fact with same subject+predicate already exists
	existing := false
	for f, err := range s.Scan(subj, pred, "") {
		if err != nil {
			continue
		}
		_ = f
		existing = true
		break
	}
	if !existing {
		s.AddFact(meb.Fact{Subject: subj, Predicate: pred, Object: obj})
	}
}

// getTagConfig returns the active tagging rules for this project.
// Falls back to default rules if no project-specific config is available.
func getTagConfig() *config.ProjectTagConfig {
	// Future: load from gca.yaml per project via LoadTagRulesFromYAML
	rules := config.DefaultTagRules()
	return &config.ProjectTagConfig{Rules: rules}
}

// shouldInjectTag checks if a file already has the given tag.
func shouldInjectTag(s Store, fileID, tag string) bool {
	for fact, err := range s.Scan(fileID, config.PredicateHasTag, tag) {
		if err != nil {
			continue
		}
		_ = fact
		return false // already tagged
	}
	return true
}

// shouldInjectFact checks if a fact already exists before adding (idempotent write).
func shouldInjectFact(s Store, subj, pred string, obj any) bool {
	objStr, ok := obj.(string)
	if !ok {
		return true // non-string objects are always "new"
	}
	for fact, err := range s.Scan(subj, pred, objStr) {
		if err != nil {
			continue
		}
		_ = fact
		return false // already exists
	}
	return true
}

// configDrivenTagMatcher iterates over the regex rules and injects matching tags.
func configDrivenTagMatcher(s Store, fileID string, tagCfg *config.ProjectTagConfig) {
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

// extractHandler extracts the handler function name from a raw handler string.
// Handles: "s.handleTest" → "handleTest", "wrapReflectionHandler(handleListActions(g))" → "handleListActions",
// "func(...) {...}" → "" (anonymous, caller should create synthetic ID).
func extractHandler(raw string) string {
	raw = strings.TrimSpace(raw)
	// Anonymous function literals → return empty (caller creates synthetic ID)
	if strings.HasPrefix(raw, "func") {
		return ""
	}
	// Strip receiver prefix: "s.handleTest" → "handleTest"
	if idx := strings.LastIndex(raw, "."); idx != -1 {
		raw = raw[idx+1:]
	}
	raw = strings.Trim(raw, " ),;")
	// Handle wrapped functions: "wrapHandler(innerFunc(args))" → "innerFunc"
	if idx := strings.Index(raw, "("); idx != -1 {
		inner := raw[:idx]
		after := strings.TrimSpace(raw[idx+1:])
		if j := strings.Index(after, "("); j != -1 {
			inner = after[:j]
		}
		inner = strings.Trim(inner, " ),;")
		if inner != "" && inner != "func" {
			return inner
		}
		return ""
	}
	return raw
}

func collectTaggedDefines(s Store, tag string) map[string]bool {
	set := make(map[string]bool)
	for fact, err := range s.Scan("", config.PredicateHasTag, tag) {
		if err != nil {
			continue
		}
		set[fact.Subject] = true
	}
	for fact, err := range s.Scan("", config.PredicateDefines, "") {
		if err != nil {
			continue
		}
		obj, ok := fact.Object.(string)
		if !ok {
			continue
		}
		if set[fact.Subject] {
			set[obj] = true
		}
	}
	return set
}

func isIDInSet(id string, set map[string]bool) bool {
	if set[id] {
		return true
	}
	parts := strings.Split(id, ":")
	if len(parts) > 1 {
		return set[parts[0]]
	}
	return false
}

func buildSymbolIndex(s Store, beSet map[string]bool) map[string]string {
	lookup := make(map[string]string)
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
			lookup[name] = sID
		}
	}
	return lookup
}

func detectRoutes(s Store, beSet map[string]bool, symbolLookup map[string]string) map[string]string {
	ginRouteRegex := regexp.MustCompile(`\.(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)\(\s*"([^"]+)"\s*,\s*([^,\)]+)`)
	go122MuxRegex := regexp.MustCompile(`\.(HandleFunc|Handle)\s*\(\s*"(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)\s+([^"]+)"\s*,\s*(.+)`)
	paramRegex := regexp.MustCompile(`:([a-zA-Z_][a-zA-Z0-9_]*)`)

	routeMap := make(map[string]string)
	for id := range beSet {
		if strings.Contains(id, ":") {
			continue
		}
		doc, err := s.GetContentByKey(string(id))
		if err != nil {
			continue
		}
		content := string(doc)

		if strings.Contains(content, "gin.Default") || strings.Contains(content, "gin.New") ||
			strings.Contains(content, ".Group") || strings.Contains(content, "Router") {
			for _, match := range ginRouteRegex.FindAllStringSubmatch(content, -1) {
				method := strings.ToUpper(match[1])
				route := match[2]
				rawHandler := strings.TrimSpace(match[3])
				handlerToken := extractHandler(rawHandler)

				if targetID, ok := symbolLookup[handlerToken]; ok {
					routeMap[route] = targetID
					upsertFact(s, route, config.PredicateHandledBy, targetID)
					upsertFact(s, route, "http_method", method)
					for _, m := range paramRegex.FindAllStringSubmatch(route, -1) {
						upsertFact(s, route, "path_param", m[1])
					}
					upsertFact(s, targetID, config.PredicateHasRole, config.RoleAPIHandler)
				} else {
					logger.Warn("Failed to link route to handler", "route", route, "handler", rawHandler, "token", handlerToken)
				}
			}
		}

		if strings.Contains(content, "http.NewServeMux") || strings.Contains(content, "HandleFunc(") || strings.Contains(content, "mux.Handle(") {
			for _, match := range go122MuxRegex.FindAllStringSubmatch(content, -1) {
				method := strings.ToUpper(match[2])
				route := match[3]
				rawHandler := strings.TrimSpace(match[4])
				handlerToken := extractHandler(rawHandler)

				if targetID, ok := symbolLookup[handlerToken]; ok {
					routeMap[route] = targetID
					upsertFact(s, route, config.PredicateHandledBy, targetID)
					upsertFact(s, route, "http_method", method)
					for _, m := range paramRegex.FindAllStringSubmatch(route, -1) {
						upsertFact(s, route, "path_param", m[1])
					}
					upsertFact(s, targetID, config.PredicateHasRole, config.RoleAPIHandler)
				} else {
					syntheticID := id + ":handler:" + method + "_" + route
					routeMap[route] = syntheticID
					upsertFact(s, route, config.PredicateHandledBy, syntheticID)
					upsertFact(s, route, "http_method", method)
					safeAddFact(s, syntheticID, config.PredicateDefines, id)
					for _, m := range paramRegex.FindAllStringSubmatch(route, -1) {
						upsertFact(s, route, "path_param", m[1])
					}
					upsertFact(s, syntheticID, config.PredicateHasRole, config.RoleAPIHandler)
				}
			}
		}
	}
	return routeMap
}

func linkAPICalls(s Store, routeMap map[string]string) {
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
}

type fileInfo struct {
	ID      string
	Content string
	Symbols []string
}

func buildFileList(s Store, beSet map[string]bool) []fileInfo {
	var files []fileInfo
	for id := range beSet {
		if strings.Contains(id, ":") {
			continue
		}
		doc, err := s.GetContentByKey(string(id))
		if err != nil {
			continue
		}
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
			files = append(files, fileInfo{ID: id, Content: content, Symbols: symbols})
		}
	}
	return files
}

func buildMethodIndex(s Store, beSet map[string]bool) map[string][]string {
	methodIndex := make(map[string][]string)
	for fact, err := range s.Scan("", config.PredicateType, "method") {
		if err != nil {
			continue
		}
		id := fact.Subject
		if beSet[id] || isIDInSet(id, beSet) {
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
	return methodIndex
}

func linkMethodCalls(s Store, files []fileInfo, methodIndex map[string][]string) {
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
}

func linkDataLineage(s Store, files []fileInfo) {
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
}

func linkFrontendExports(s Store, feSet map[string]bool) {
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
}

func injectFileMetaFacts(s Store, tagCfg *config.ProjectTagConfig) {
	logger.Info("Injecting test file tags and in_file facts")
	testSymbolRegex := regexp.MustCompile(`^(Test|Benchmark|Example)[A-Z].*`)
	for fact, err := range s.Scan("", config.PredicateDefines, "") {
		if err != nil {
			continue
		}
		fileID := fact.Subject
		if strings.Contains(fileID, ":") {
			continue
		}

		symbolID, ok := fact.Object.(string)
		if ok && shouldInjectFact(s, symbolID, "in_file", fileID) {
			s.AddFact(meb.Fact{Subject: symbolID, Predicate: "in_file", Object: fileID})
		}

		if tagCfg.MatchingTags(fileID) != nil {
			for _, tag := range tagCfg.MatchingTags(fileID) {
				if tag == config.TagTestFile {
					if shouldInjectTag(s, fileID, config.TagTestFile) {
						s.AddFact(meb.Fact{Subject: fileID, Predicate: config.PredicateHasTag, Object: config.TagTestFile})
						logger.Debug("Test file tagged", "file", fileID)
					}
				}
			}
		}
	}

	for fact, err := range s.Scan("", config.PredicateDefines, "") {
		if err != nil {
			continue
		}
		sID, ok := fact.Object.(string)
		if !ok {
			continue
		}
		name := common.ExtractSymbolName(sID)
		if testSymbolRegex.MatchString(name) {
			s.AddFact(meb.Fact{Subject: sID, Predicate: config.PredicateHasTag, Object: config.TagTestSymbol})
			s.AddFact(meb.Fact{Subject: sID, Predicate: "is_test_symbol", Object: "true"})
		}
	}
}

func injectArchitecturalTags(s Store, feSet map[string]bool, beSet map[string]bool, tagCfg *config.ProjectTagConfig) {
	logger.Info("Injecting architectural tags for security smell detection")
	for id := range beSet {
		configDrivenTagMatcher(s, id, tagCfg)
	}
	for id := range feSet {
		configDrivenTagMatcher(s, id, tagCfg)
	}
}

func EnhanceVirtualTriples(s Store) error {
	tagCfg := getTagConfig()
	feSet := collectTaggedDefines(s, "frontend")
	beSet := collectTaggedDefines(s, "backend")

	symbolIdx := buildSymbolIndex(s, beSet)
	routes := detectRoutes(s, beSet, symbolIdx)
	linkAPICalls(s, routes)

	files := buildFileList(s, beSet)
	methodIdx := buildMethodIndex(s, beSet)
	linkMethodCalls(s, files, methodIdx)
	linkDataLineage(s, files)

	linkFrontendExports(s, feSet)
	injectFileMetaFacts(s, tagCfg)
	injectArchitecturalTags(s, feSet, beSet, tagCfg)
	return nil
}
