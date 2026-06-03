package promptbuilder

import (
	"fmt"
	"strings"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
)

func FormatNodesWithCode(data interface{}, limit int) string {
	return common.FormatNodesWithCode(data, limit)
}

func FormatNodesSimple(data interface{}, limit int) string {
	return common.FormatNodesSimple(data, limit)
}

func FormatPredicatesList(data interface{}) string {
	return common.FormatPredicatesList(data)
}

func FormatNodeList(data interface{}) string {
	return common.FormatNodeList(data)
}

func FormatGraphResults(data interface{}, key string) string {
	m, ok := data.(map[string]interface{})
	if !ok {
		return ""
	}
	list, ok := m[key].([]interface{})
	if !ok {
		return ""
	}
	var sb strings.Builder
	if key == "nodes" {
		for i, item := range list {
			if node, ok := item.(map[string]interface{}); ok {
				name, _ := node["name"].(string)
				kind, _ := node["kind"].(string)
				id, _ := node["id"].(string)
				sb.WriteString(fmt.Sprintf("%d. **%s** (Type: %s)\n   ID: `%s`\n", i+1, name, kind, id))
			}
		}
	} else if key == "links" {
		for i, item := range list {
			if link, ok := item.(map[string]interface{}); ok {
				source, _ := link["source"].(string)
				target, _ := link["target"].(string)
				relation, _ := link["relation"].(string)
				if relation == "" {
					relation = config.PredicateCalls
				}
				sb.WriteString(fmt.Sprintf("%d. `%s` **%s** `%s`\n", i+1, source, relation, target))
			}
		}
	}
	return sb.String()
}

func ExtractNodeNames(data interface{}) string {
	return common.ExtractNodeNames(data)
}

func ExtractStringList(data interface{}, limit int) string {
	return common.ExtractStringList(data, limit)
}

func ExtractPathString(data interface{}) string {
	return common.ExtractPathString(data)
}

func AppendSymbolContext(ctx interface{}, store *meb.MEBStore, symbolID string, sb *strings.Builder) error {
	contentBytes, err := store.GetContentByKey(symbolID)
	if err != nil {
		return err
	}
	content := string(contentBytes)

	sb.WriteString(fmt.Sprintf("\n### Symbol: %s\n", symbolID))
	sb.WriteString("```go\n")
	sb.WriteString(common.SymbolContext(content))
	sb.WriteString("\n```\n")

	return nil
}

var routeKeywords = []string{
	"api endpoint", "http route", "route handler", "list route", "list endpoint",
	"list api", "list all api", "list all endpoint", "list all route",
	"which endpoint", "what endpoint", "all routes", "all endpoints",
	"show routes", "show endpoints", "show api",
}

func isRouteQuery(query string) bool {
	q := strings.ToLower(query)
	for _, kw := range routeKeywords {
		if strings.Contains(q, kw) {
			return true
		}
	}
	return false
}

func appendRouteContext(store *meb.MEBStore, sb *strings.Builder) {
	routes := make(map[string]string) // route -> method
	handlers := make(map[string]string) // route -> handler

	for fact, err := range store.Scan("", config.PredicateHandledBy, "") {
		if err != nil {
			continue
		}
		route := fact.Subject
		if handler, ok := fact.Object.(string); ok {
			handlers[route] = handler
		}
	}

	for fact, err := range store.Scan("", "http_method", "") {
		if err != nil {
			continue
		}
		route := fact.Subject
		if method, ok := fact.Object.(string); ok {
			routes[route] = method
		}
	}

	if len(handlers) == 0 {
		sb.WriteString("\n### API Routes\nNo API routes detected in the codebase. Try re-ingesting the project with route detection.\n")
		return
	}

	sb.WriteString("\n### API Routes (from codebase analysis)\n")
	sb.WriteString("```")
	sb.WriteString(fmt.Sprintf("%-8s %-40s %s\n", "METHOD", "ROUTE", "HANDLER"))
	sb.WriteString(fmt.Sprintf("%-8s %-40s %s\n", "------", "-----", "-------"))
	for route, handler := range handlers {
		method := routes[route]
		if method == "" {
			method = "ANY"
		}
		sb.WriteString(fmt.Sprintf("%-8s %-40s %s\n", method, route, handler))
	}
	sb.WriteString("```\n")
}