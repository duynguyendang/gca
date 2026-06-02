package ai

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/datalog"
	"github.com/duynguyendang/gca/pkg/logger"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
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
	} else {
		result.Validated = true
	}

	return result, nil
}

// GenerateDatalogWithContext generates Datalog queries with multi-turn conversation awareness
func GenerateDatalogWithContext(ctx context.Context, nlQuery string, intent Intent, target string, store *meb.MEBStore, conversationCtx string) (*QueryGenResult, error) {
	result := &QueryGenResult{
		Intent:  intent,
		Context: make(map[string]interface{}),
	}

	predicates := getAvailablePredicates(store)
	result.Context["predicates"] = predicates

	// If we have conversation context, try to resolve implicit references
	resolvedTarget := resolveTargetFromContext(target, conversationCtx)
	if resolvedTarget != "" {
		target = resolvedTarget
	}

	baseQuery := GetDatalogTemplateForIntent(intent, target)
	result.Query = baseQuery

	// Enrich with both current context and conversation history
	enrichedQuery, err := enrichQueryWithContext(ctx, nlQuery, intent, target, store, baseQuery)
	if err == nil && enrichedQuery != "" {
		result.Query = enrichedQuery
	}

	// Apply conversation-based refinements
	result.Query = refineQueryFromConversation(result.Query, conversationCtx, intent)

	validated, err := ValidateDatalog(result.Query)
	if !validated {
		result.Validated = false
		result.Error = err.Error()
		result.Query = baseQuery
	} else {
		result.Validated = true
	}

	return result, nil
}

// resolveTargetFromContext resolves target from conversation context when target is empty
func resolveTargetFromContext(target, conversationCtx string) string {
	if target != "" || conversationCtx == "" {
		return target
	}

	// Look for the last discussed target in conversation history
	// Conversation format from buildConversationContext:
	// "- Q1: <user input> (intent: <intent>, results: <count>)"
	lines := strings.Split(conversationCtx, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "- Q") {
			continue
		}

		// Extract intent from the parenthetical: (intent: <intent>, results: N)
		intentStart := strings.Index(line, "(intent:")
		if intentStart < 0 {
			continue
		}
		intentEnd := intentStart + strings.Index(line[intentStart+1:], ",")
		if intentEnd < 0 {
			continue
		}
		intentVal := strings.TrimSpace(line[intentStart+8 : intentEnd])

		// Only use find/explain intents as they reference a target
		if intentVal != "find" && intentVal != "explain" && intentVal != "what_calls" && intentVal != "who_calls" {
			continue
		}

		// Extract the user query portion: "- Q1: <query> (intent:"
		queryStart := strings.Index(line, ": ")
		intentParen := strings.Index(line, " (intent:")
		if queryStart < 0 || intentParen < 0 {
			continue
		}
		query := line[queryStart+2 : intentParen]

		// Look for file/symbol patterns in the query (e.g., "auth.go", "service/auth", "AuthService")
		targetPatterns := []string{
			`[a-zA-Z0-9_/]+\.go\b`,
			`[A-Z][a-zA-Z0-9]+Service`,
			`[a-z][a-zA-Z0-9]+/[a-z][a-zA-Z0-9]+`,
		}
		for _, pattern := range targetPatterns {
			if match := regexp.MustCompile(pattern).FindString(query); match != "" {
				return match
			}
		}
	}

	return target
}

// refineQueryFromConversation applies refinements based on conversation flow
func refineQueryFromConversation(query, conversationCtx string, intent Intent) string {
	if conversationCtx == "" || intent == IntentChat {
		return query
	}

	historyLower := strings.ToLower(conversationCtx)

	// If user asks for "more" or "another", add distinct modifier to avoid duplicate results
	if strings.Contains(historyLower, "more") || strings.Contains(historyLower, "another") {
		if !strings.Contains(query, "distinct") && !strings.Contains(query, "limit") {
			if intent == IntentFind || intent == IntentExplain {
				// Append distinct to existing predicates to deduplicate
				if strings.HasSuffix(query, ").") {
					query = strings.TrimSuffix(query, ")") + ", distinct)"
				} else if strings.HasSuffix(query, ")") {
					query = query[:len(query)-1] + ", distinct)"
				}
			}
		}
	}

	// If conversation shows counting behavior (e.g., "how many", "count", "total"),
	// add aggregation hint to the query
	if strings.Contains(historyLower, "how many") || strings.Contains(historyLower, "count") || strings.Contains(historyLower, "total") {
		if intent == IntentExplain || intent == IntentSummarize {
			if !strings.Contains(query, "findall") && !strings.Contains(query, "list.length") {
				// Wrap query to also return count
				query = fmt.Sprintf("findall(X, %s, L), list.length(L, Count)", query)
			}
		}
	}

	// If user is following up on a specific file/module, broaden scope to include related symbols
	if strings.Contains(historyLower, "related") || strings.Contains(historyLower, "everything about") {
		if intent == IntentFind && !strings.Contains(query, "references") {
			// Extend to also include references predicate
			if strings.Contains(query, "triples(") {
				parts := strings.Split(query, ",")
				if len(parts) >= 1 {
					base := strings.TrimSpace(parts[0])
					query = base + fmt.Sprintf(", triples(?s, \"references\", %s)", extractTargetFromQuery(base))
				}
			}
		}
	}

	return query
}

// extractTargetFromQuery pulls a symbol ID out of a triples predicate for use in extensions
func extractTargetFromQuery(query string) string {
	// Extract the quoted target from patterns like: triples("foo.go", "calls", ?x)
	re := regexp.MustCompile(`triples\([^,]+,\s*"[^"]+",\s*"?([^")]+)"?\)`)
	if match := re.FindStringSubmatch(query); len(match) > 1 {
		return match[1]
	}
	// Fallback: extract first quoted string
	re2 := regexp.MustCompile(`"([^"]+)"`)
	if matches := re2.FindAllStringSubmatch(query, -1); len(matches) > 0 {
		return matches[0][1]
	}
	return "?"
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
		sym := common.EscapeDatalogValue(symbols[0])
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
		sym = common.EscapeDatalogValue(sym)
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
