package ai

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/datalog"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

type QueryGenResult struct {
	Query     string
	Intent    Intent
	Validated bool
	Error     string
	Results   interface{}
	Context   map[string]interface{}
}

var (
	predicateCache     []string
	predicateCacheTime time.Time
	predicateCacheMu   sync.RWMutex
	predicateCacheTTL  = 5 * time.Minute
)

func GenerateDatalog(ctx context.Context, nlQuery string, intent Intent, target string, store *meb.MEBStore) (*QueryGenResult, error) {
	result := &QueryGenResult{
		Intent:  intent,
		Context: make(map[string]interface{}),
	}

	predicates := getAvailablePredicates(store)
	result.Context["predicates"] = predicates

	baseQuery := GetDatalogTemplateForIntent(intent, target)
	result.Query = baseQuery

	enrichedQuery, err := enrichQueryWithContext(ctx, nlQuery, intent, target, store, baseQuery)
	if err == nil && enrichedQuery != "" {
		result.Query = enrichedQuery
	}

	validated, err := ValidateDatalog(result.Query)
	if !validated {
		result.Validated = false
		result.Error = err.Error()
		result.Query = baseQuery
		result.Validated = true
	} else {
		result.Validated = true
	}

	return result, nil
}

func getAvailablePredicates(store *meb.MEBStore) []string {
	predicateCacheMu.RLock()
	if len(predicateCache) > 0 && time.Since(predicateCacheTime) < predicateCacheTTL {
		defer predicateCacheMu.RUnlock()
		return predicateCache
	}
	predicateCacheMu.RUnlock()

	predicates := []string{
		"calls",
		"defines",
		"imports",
		"references",
		"in_package",
		"has_doc",
		"has_role",
		"has_tag",
		"type",
	}

	if store != nil {
		if preds := store.ListPredicates(); len(preds) > 0 {
			predicates = make([]string, 0, len(preds))
			for _, p := range preds {
				predicates = append(predicates, string(p.Symbol))
			}
		}
	}

	predicateCacheMu.Lock()
	predicateCache = predicates
	predicateCacheTime = time.Now()
	predicateCacheMu.Unlock()

	return predicates
}

func enrichQueryWithContext(ctx context.Context, nlQuery string, intent Intent, target string, store *meb.MEBStore, baseQuery string) (string, error) {
	if store == nil || target == "" {
		return baseQuery, nil
	}

	target = strings.Trim(target, "\"' ")

	exactMatchID, exists := store.LookupID(target)
	if exists {
		exactMatch := fmt.Sprintf("%d", exactMatchID)
		if intent == IntentWhoCalls {
			return fmt.Sprintf(`triples(?caller, "calls", "%s")`, exactMatch), nil
		}
		if intent == IntentWhatCalls {
			return fmt.Sprintf(`triples("%s", "calls", ?callee)`, exactMatch), nil
		}
	}

	symbols := searchSymbols(store, target)
	if len(symbols) > 0 {
		result := buildQueryFromSymbols(symbols, intent, target)
		if result != "" {
			return result, nil
		}
	}

	return baseQuery, nil
}

func searchSymbols(store *meb.MEBStore, query string) []string {
	var results []string

	if query == "" {
		return results
	}

	upperQuery := strings.ToUpper(query)
	lowerQuery := strings.ToLower(query)

	var scanErrors int
	for fact, err := range store.Scan("", config.PredicateDefines, "") {
		if err != nil {
			scanErrors++
			continue
		}
		symID, ok := fact.Object.(string)
		if !ok {
			continue
		}

		symName := extractSymbolName(symID)
		symNameUpper := strings.ToUpper(symName)
		symNameLower := strings.ToLower(symName)

		if strings.Contains(symNameUpper, upperQuery) ||
			strings.Contains(symNameLower, lowerQuery) ||
			symNameUpper == upperQuery ||
			symNameLower == lowerQuery {
			results = append(results, symID)
			if len(results) >= 10 {
				break
			}
		}
	}

	if scanErrors > 0 {
		logger.Warn("Scan errors while searching symbols", "errors", scanErrors, "query", query)
	}

	return results
}

func extractSymbolName(symID string) string {
	if idx := strings.LastIndex(symID, ":"); idx >= 0 && idx < len(symID)-1 {
		return symID[idx+1:]
	}
	return symID
}

func buildQueryFromSymbols(symbols []string, intent Intent, original string) string {
	if len(symbols) == 0 {
		return ""
	}

	if len(symbols) == 1 {
		sym := symbols[0]
		switch intent {
		case IntentWhoCalls:
			return fmt.Sprintf(`triples(?caller, "calls", "%s")`, sym)
		case IntentWhatCalls:
			return fmt.Sprintf(`triples("%s", "calls", ?callee)`, sym)
		case IntentFind:
			return fmt.Sprintf(`triples("%s", ?pred, ?obj)`, sym)
		case IntentSummarize:
			return fmt.Sprintf(`triples("%s", ?pred, ?obj)`, sym)
		}
	}

	var conditions []string
	for _, sym := range symbols {
		switch intent {
		case IntentWhoCalls:
			conditions = append(conditions, fmt.Sprintf(`triples(?caller, "calls", "%s")`, sym))
		case IntentWhatCalls:
			conditions = append(conditions, fmt.Sprintf(`triples("%s", "calls", ?callee)`, sym))
		case IntentFind, IntentSummarize:
			conditions = append(conditions, fmt.Sprintf(`triples("%s", ?pred, ?obj)`, sym))
		}
	}

	return strings.Join(conditions, ", ")
}

func ValidateDatalog(query string) (bool, error) {
	if query == "" {
		return false, fmt.Errorf("empty query")
	}

	if strings.HasPrefix(query, "{") {
		return true, nil
	}

	atoms, err := datalog.Parse(query)
	if err != nil {
		return false, fmt.Errorf("parse error: %w", err)
	}

	if len(atoms) == 0 {
		return false, fmt.Errorf("no atoms in query")
	}

	validPredicates := map[string]bool{
		"triples":      true,
		"eq":           true,
		"neq":          true,
		"=":            true,
		"!=":           true,
		"regex":        true,
		"contains":     true,
		"starts_with":  true,
		"calls":        true,
		"defines":      true,
		"imports":      true,
		"references":   true,
		"in_package":   true,
		"has_doc":      true,
		"has_role":     true,
		"has_tag":      true,
		"type":         true,
		"has_kind":     true,
		"has_language": true,
	}

	for _, atom := range atoms {
		if !validPredicates[atom.Predicate] && !strings.HasPrefix(atom.Predicate, "?") {
			return false, fmt.Errorf("unknown predicate: %s", atom.Predicate)
		}
	}

	return true, nil
}

func ExecuteQuery(ctx context.Context, store *meb.MEBStore, query string) (interface{}, error) {
	if query == "" {
		return nil, fmt.Errorf("empty query")
	}

	if strings.HasPrefix(query, "{") {
		return nil, nil
	}

	results, err := gcamdb.Query(ctx, store, query)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	return results, nil
}

func BuildGraphContext(ctx context.Context, store *meb.MEBStore, symbolID string) (map[string]interface{}, error) {
	context := make(map[string]interface{})

	if symbolID == "" || store == nil {
		return context, nil
	}

	symbolID = strings.Trim(symbolID, "\"' ")

	content, err := store.GetContentByKey(symbolID)
	if err == nil {
		context["content"] = string(content)
	}

	inbound, _ := gcamdb.Query(ctx, store, fmt.Sprintf(`triples(?s, "calls", "%s")`, symbolID))
	outbound, _ := gcamdb.Query(ctx, store, fmt.Sprintf(`triples("%s", "calls", ?o)`, symbolID))
	defines, _ := gcamdb.Query(ctx, store, fmt.Sprintf(`triples("%s", "defines", ?o)`, symbolID))

	if len(inbound) > 0 {
		context["inbound"] = inbound
	}
	if len(outbound) > 0 {
		context["outbound"] = outbound
	}
	if len(defines) > 0 {
		context["defines"] = defines
	}

	return context, nil
}
