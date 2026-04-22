package common

import (
	"fmt"
	"strings"
)

// formatNodesWithCode formats a list of nodes with their source code for AI prompts.
func FormatNodesWithCode(data interface{}, limit int) string {
	if data == nil {
		return ""
	}
	list, ok := data.([]interface{})
	if !ok {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Query Results:\n\n")
	for i, item := range list {
		if i >= limit {
			break
		}
		if m, ok := item.(map[string]interface{}); ok {
			id, _ := m["id"].(string)
			name, _ := m["name"].(string)
			kind, _ := m["kind"].(string)
			code, _ := m["code"].(string)

			sb.WriteString(fmt.Sprintf("### %d. %s\n", i+1, id))
			if name != "" && name != id {
				sb.WriteString(fmt.Sprintf("Name: %s\n", name))
			}
			if kind != "" {
				sb.WriteString(fmt.Sprintf("Type: %s\n", kind))
			}
			if code != "" {
				sb.WriteString(fmt.Sprintf("```\n%s\n```\n", code))
			}
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// formatNodesSimple formats a list of nodes without code (simple bullet list).
func FormatNodesSimple(data interface{}, limit int) string {
	if data == nil {
		return ""
	}
	list, ok := data.([]interface{})
	if !ok {
		return ""
	}
	var sb strings.Builder
	for i, item := range list {
		if i >= limit {
			break
		}
		if m, ok := item.(map[string]interface{}); ok {
			name, _ := m["name"].(string)
			kind, _ := m["kind"].(string)
			sb.WriteString(fmt.Sprintf("- %s (%s)\n", name, kind))
		}
	}
	return sb.String()
}

// FormatPredicatesList formats a list of predicates as markdown bullet items.
func FormatPredicatesList(data interface{}) string {
	if str, ok := data.(string); ok {
		return str
	}
	list, ok := data.([]interface{})
	if !ok {
		return ""
	}
	var sb strings.Builder
	for _, item := range list {
		if predicate, ok := item.(string); ok {
			sb.WriteString(fmt.Sprintf("- `%s`\n", predicate))
		}
	}
	return sb.String()
}

// FormatNodeList formats a list of nodes with name, kind, and ID.
func FormatNodeList(data interface{}) string {
	if data == nil {
		return ""
	}
	if str, ok := data.(string); ok {
		return str
	}
	list, ok := data.([]interface{})
	if !ok {
		return ""
	}
	var sb strings.Builder
	for _, item := range list {
		if m, ok := item.(map[string]interface{}); ok {
			name, _ := m["name"].(string)
			kind, _ := m["kind"].(string)
			id, _ := m["id"].(string)
			sb.WriteString(fmt.Sprintf("- %s (Kind: %s, ID: %s)\n", name, kind, id))
		}
	}
	return sb.String()
}

// FormatGraphResults formats results from a graph query by key.
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
	for _, item := range list {
		if s, ok := item.(string); ok {
			sb.WriteString(s)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// ExtractNodeNames extracts just the node names from query results.
func ExtractNodeNames(data interface{}) string {
	if data == nil {
		return ""
	}
	list, ok := data.([]interface{})
	if !ok {
		return ""
	}
	var sb strings.Builder
	for _, item := range list {
		if m, ok := item.(map[string]interface{}); ok {
			if name, ok := m["name"].(string); ok {
				sb.WriteString(name)
				sb.WriteString(", ")
			}
		}
	}
	return strings.TrimSuffix(sb.String(), ", ")
}

// ExtractStringList extracts a list of strings from query results, limited to N items.
func ExtractStringList(data interface{}, limit int) string {
	if data == nil {
		return ""
	}
	list, ok := data.([]interface{})
	if !ok {
		return ""
	}
	var sb strings.Builder
	count := 0
	for _, item := range list {
		if count >= limit {
			break
		}
		if s, ok := item.(string); ok {
			sb.WriteString(s)
			sb.WriteString(", ")
			count++
		}
	}
	return strings.TrimSuffix(sb.String(), ", ")
}

// ExtractPathString extracts a formatted path string from query results.
func ExtractPathString(data interface{}) string {
	if data == nil {
		return ""
	}
	list, ok := data.([]interface{})
	if !ok {
		return ""
	}
	var sb strings.Builder
	for _, item := range list {
		if m, ok := item.(map[string]interface{}); ok {
			if id, ok := m["id"].(string); ok {
				sb.WriteString(" -> ")
				sb.WriteString(id)
			}
		}
	}
	return sb.String()
}
