package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/meb"
)

type SynthesisResult struct {
	Answer  string
	Query   string
	Intent  Intent
	Results interface{}
	Summary string
	Errors  []string
}

func SynthesizeAnswer(ctx context.Context, intent Intent, nlQuery string, query string, results interface{}, store *meb.MEBStore) (*SynthesisResult, error) {
	synth := &SynthesisResult{
		Query:   query,
		Intent:  intent,
		Results: results,
	}

	synth.Answer = synthesizeWithRegistry(intent, results, ctx, store)
	synth.Summary = generateSummary(intent, results)

	return synth, nil
}

type synthesisFunc func(results interface{}, ctx context.Context, store *meb.MEBStore) string

var synthesizeRegistry = map[Intent]synthesisFunc{
	IntentWhoCalls:   func(r interface{}, _ context.Context, _ *meb.MEBStore) string { return summarizeCallers(r) },
	IntentWhatCalls:  func(r interface{}, _ context.Context, _ *meb.MEBStore) string { return summarizeCallees(r) },
	IntentHowReaches: func(r interface{}, _ context.Context, _ *meb.MEBStore) string { return summarizePath(r) },
	IntentSummarize:   func(r interface{}, ctx context.Context, store *meb.MEBStore) string { return summarizeEntity(ctx, r, store) },
	IntentFind:       func(r interface{}, _ context.Context, _ *meb.MEBStore) string { return summarizeFind(r) },
	IntentChat:       func(r interface{}, _ context.Context, _ *meb.MEBStore) string { return summarizeGeneral(r) },
}

func synthesizeWithRegistry(intent Intent, results interface{}, ctx context.Context, store *meb.MEBStore) string {
	if handler, ok := synthesizeRegistry[intent]; ok {
		return handler(results, ctx, store)
	}
	return summarizeGeneral(results)
}

func extractStringsFromResults(results interface{}, keys []string) []string {
	var found []string
	switch r := results.(type) {
	case []map[string]any:
		for _, row := range r {
			for _, key := range keys {
				if val, ok := row[key].(string); ok && val != "" {
					found = append(found, val)
				}
			}
		}
	case map[string]interface{}:
		if nodes, ok := r["nodes"].([]interface{}); ok {
			for _, n := range nodes {
				if m, ok := n.(map[string]interface{}); ok {
					if id, ok := m["id"].(string); ok {
						found = append(found, id)
					}
				}
			}
		}
	}
	return found
}

func summarizeCallers(results interface{}) string {
	callers := extractStringsFromResults(results, []string{"?caller"})
	if len(callers) == 0 {
		return "No callers found."
	}
	unique := removeDuplicates(callers)
	if len(unique) == 1 {
		return fmt.Sprintf("**%s** is called by: %s", extractNameFromID(unique[0]), strings.Join(unique, ", "))
	}
	return fmt.Sprintf("**%d callers** found:\n%s", len(unique), formatList(unique))
}

func summarizeCallees(results interface{}) string {
	callees := extractStringsFromResults(results, []string{"?callee", "?o"})
	if len(callees) == 0 {
		return "No callees found."
	}
	unique := removeDuplicates(callees)
	if len(unique) == 1 {
		return fmt.Sprintf("**%s** calls: %s", extractNameFromID(unique[0]), strings.Join(unique, ", "))
	}
	return fmt.Sprintf("**%d functions/methods** called:\n%s", len(unique), formatList(unique))
}

func summarizePath(results interface{}) string {
	path := extractStringsFromResults(results, []string{"node"})

	if pathNodes := extractNodesFromResults(results); len(pathNodes) > 0 && len(path) == 0 {
		path = pathNodes
	}

	if len(path) == 0 {
		return "No path found between the specified symbols."
	}

	unique := removeDuplicates(path)
	return fmt.Sprintf("**Path found** (%d hops):\n```\n%s\n```",
		len(unique)-1, strings.Join(unique, " → "))
}

func extractNodesFromResults(results interface{}) []string {
	var nodes []string
	if r, ok := results.(map[string]interface{}); ok {
		if nds, ok := r["nodes"].([]interface{}); ok {
			for _, n := range nds {
				if m, ok := n.(map[string]interface{}); ok {
					if id, ok := m["id"].(string); ok {
						nodes = append(nodes, id)
					}
				}
			}
		}
	}
	return nodes
}

func summarizeEntity(ctx context.Context, results interface{}, store *meb.MEBStore) string {
	entities := extractStringsFromResults(results, []string{"?s", "?sym", "?obj", "?name"})
	docs := extractStringsFromResults(results, []string{"?doc"})

	if len(entities) == 0 {
		return "No matching entities found."
	}

	unique := removeDuplicates(entities)
	var summary strings.Builder

	summary.WriteString(fmt.Sprintf("**Found %d matching entities:**\n", len(unique)))
	summary.WriteString(formatList(unique))

	if len(docs) > 0 && docs[0] != "" {
		summary.WriteString("\n**Documentation:**\n")
		for i, doc := range docs {
			if i >= 3 {
				break
			}
			doc = common.DocPreview(doc)
			summary.WriteString(fmt.Sprintf("- %s\n", doc))
		}
	}

	return summary.String()
}

func summarizeFind(results interface{}) string {
	found := extractStringsFromResults(results, []string{"?s", "?sym", "?obj", "?id"})
	if len(found) == 0 {
		return "No matches found."
	}
	unique := removeDuplicates(found)
	return fmt.Sprintf("**Found %d matches:**\n%s", len(unique), formatList(unique))
}

func summarizeGeneral(results interface{}) string {
	switch r := results.(type) {
	case []map[string]any:
		count := len(r)
		if count == 0 {
			return "No results found."
		}
		var sample []string
		for _, row := range r {
			for _, val := range row {
				if str, ok := val.(string); ok && str != "" && len(str) < 50 {
					sample = append(sample, str)
					if len(sample) >= 5 {
						break
					}
				}
			}
			if len(sample) >= 5 {
				break
			}
		}
		return fmt.Sprintf("**%d results** found.\nSample: %s", count, strings.Join(sample, ", "))

	case map[string]interface{}:
		if nodes, ok := r["nodes"].([]interface{}); ok {
			return fmt.Sprintf("**%d nodes** found.", len(nodes))
		}
		if links, ok := r["links"].([]interface{}); ok {
			return fmt.Sprintf("**%d links** found.", len(links))
		}
		return "Results found but unable to format."

	default:
		if results == nil {
			return "No results."
		}
		return fmt.Sprintf("Results type: %T", results)
	}
}

func generateSummary(intent Intent, results interface{}) string {
	switch intent {
	case IntentWhoCalls:
		return countResults(results, "caller")
	case IntentWhatCalls:
		return countResults(results, "callee")
	case IntentFind:
		return countResults(results, "match")
	default:
		return countResults(results, "result")
	}
}

func countResults(results interface{}, label string) string {
	var count int
	switch r := results.(type) {
	case []map[string]any:
		count = len(r)
	case map[string]interface{}:
		if nodes, ok := r["nodes"].([]interface{}); ok {
			count = len(nodes)
		} else if links, ok := r["links"].([]interface{}); ok {
			count = len(links)
		}
	}

	if count == 0 {
		return fmt.Sprintf("No %ss found.", label)
	}
	return fmt.Sprintf("%d %ss found.", count, label)
}

func removeDuplicates(list []string) []string {
	seen := make(map[string]bool)
	result := []string{}
	for _, s := range list {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

func formatList(items []string) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) <= 10 {
		var lines []string
		for _, item := range items {
			name := extractNameFromID(item)
			lines = append(lines, fmt.Sprintf("- %s (`%s`)", name, item))
		}
		return strings.Join(lines, "\n")
	}

	var lines []string
	for _, item := range items[:7] {
		name := extractNameFromID(item)
		lines = append(lines, fmt.Sprintf("- %s (`%s`)", name, item))
	}
	lines = append(lines, fmt.Sprintf("- _... and %d more_", len(items)-7))
	return strings.Join(lines, "\n")
}

func extractNameFromID(id string) string {
	if idx := strings.LastIndex(id, ":"); idx >= 0 && idx < len(id)-1 {
		name := id[idx+1:]
		if idx := strings.Index(name, "("); idx >= 0 {
			name = name[:idx]
		}
		return name
	}
	if idx := strings.LastIndex(id, "/"); idx >= 0 && idx < len(id)-1 {
		return id[idx+1:]
	}
	return id
}

type PathTool struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

func parsePathTool(query string) *PathTool {
	if !strings.HasPrefix(query, "{") {
		return nil
	}

	var tool struct {
		Tool   string `json:"tool"`
		Source string `json:"source"`
		Target string `json:"target"`
	}

	if err := json.Unmarshal([]byte(query), &tool); err != nil {
		return nil
	}

	if tool.Tool == "find_path" || tool.Tool == "find_connection" {
		return &PathTool{
			Source: tool.Source,
			Target: tool.Target,
		}
	}

	return nil
}

func ExecutePathQuery(ctx context.Context, store *meb.MEBStore, source, target string) (interface{}, error) {
	path, err := findPathBetween(ctx, store, source, target)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"nodes": path,
		"links": buildLinksFromPath(path),
	}, nil
}

func findPathBetween(ctx context.Context, store *meb.MEBStore, source, target string) ([]string, error) {
	if source == "" || target == "" {
		return nil, fmt.Errorf("source and target required")
	}

	if source == target {
		return []string{source}, nil
	}

	visited := make(map[string]bool)
	queue := [][]string{{source}}
	visited[source] = true

	maxNodes := config.PathFindingMaxNodes
	nodesVisited := 0

	for len(queue) > 0 {
		currentPath := queue[0]
		queue = queue[1:]
		current := currentPath[len(currentPath)-1]

		edges := getEdgesForNode(ctx, store, current)

		for _, next := range edges.forward {
			if next == target {
				return append(currentPath, target), nil
			}
			if !visited[next] {
				visited[next] = true
				newPath := make([]string, len(currentPath))
				copy(newPath, currentPath)
				queue = append(queue, append(newPath, next))
			}
		}

		for _, prev := range edges.backward {
			if prev == target {
				return append([]string{target}, currentPath...), nil
			}
			if !visited[prev] {
				visited[prev] = true
				newPath := make([]string, len(currentPath))
				copy(newPath, currentPath)
				queue = append(queue, append([]string{prev}, newPath...))
			}
		}

		nodesVisited++
		if nodesVisited >= maxNodes {
			return nil, fmt.Errorf("path search exceeded max nodes limit (%d)", maxNodes)
		}
	}

	return nil, fmt.Errorf("no path found between %s and %s", source, target)
}

type nodeEdges struct {
	forward  []string
	backward []string
}

func getEdgesForNode(ctx context.Context, store *meb.MEBStore, node string) nodeEdges {
	edges := nodeEdges{
		forward:  make([]string, 0),
		backward: make([]string, 0),
	}

	forwardEdges, _ := gcamdb.Query(ctx, store, fmt.Sprintf(`triples("%s", "calls", ?next)`, node))
	for _, edge := range forwardEdges {
		if next, ok := edge["?next"].(string); ok {
			edges.forward = append(edges.forward, next)
		}
	}

	backwardEdges, _ := gcamdb.Query(ctx, store, fmt.Sprintf(`triples(?prev, "calls", "%s")`, node))
	for _, edge := range backwardEdges {
		if prev, ok := edge["?prev"].(string); ok {
			edges.backward = append(edges.backward, prev)
		}
	}

	return edges
}

func buildLinksFromPath(path []string) []map[string]string {
	var links []map[string]string
	for i := 0; i < len(path)-1; i++ {
		links = append(links, map[string]string{
			"source":   path[i],
			"target":   path[i+1],
			"relation": "calls",
		})
	}
	return links
}

// formatResultsForLLM formats query results into a readable string for LLM prompts.
func formatResultsForLLM(results interface{}) string {
	if results == nil {
		return "No results"
	}

	switch r := results.(type) {
	case []map[string]any:
		if len(r) == 0 {
			return "No results"
		}
		var lines []string
		for i, row := range r {
			if i >= 10 {
				lines = append(lines, fmt.Sprintf("... and %d more results", len(r)-10))
				break
			}
			var parts []string
			for _, v := range row {
				if str, ok := v.(string); ok && str != "" {
					parts = append(parts, str)
				}
			}
			if len(parts) > 0 {
				lines = append(lines, strings.Join(parts, ", "))
			}
		}
		return strings.Join(lines, "\n")

	case map[string]interface{}:
		if nodes, ok := r["nodes"].([]interface{}); ok {
			return fmt.Sprintf("%d nodes found", len(nodes))
		}
		if links, ok := r["links"].([]interface{}); ok {
			return fmt.Sprintf("%d links found", len(links))
		}
		return "Complex result structure"

	default:
		return fmt.Sprintf("%v", results)
	}
}
