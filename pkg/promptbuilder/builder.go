package promptbuilder

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

func BuildDatalogPrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.Datalog != nil {
		return ps.Datalog.Execute(data)
	}
	var sb strings.Builder
	sb.WriteString("Analyze the following code relationship data:\n\n")
	sb.WriteString(FormatGraphResults(data, "nodes"))
	sb.WriteString("\n")
	sb.WriteString(FormatGraphResults(data, "links"))
	return sb.String(), nil
}

func BuildDatalogPromptWithSchema(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.Datalog != nil {
		return ps.Datalog.Execute(data)
	}
	predicates, _ := data.(map[string]interface{})["Predicates"].(string)
	factExamples, _ := data.(map[string]interface{})["FactExamples"].(string)
	constraintDoc, _ := data.(map[string]interface{})["ConstraintDoc"].(string)
	query, _ := data.(map[string]interface{})["Query"].(string)

	var sb strings.Builder
	sb.WriteString("## Available Predicates\n")
	sb.WriteString(predicates)
	sb.WriteString("\n\n## Example Facts (from store)\n")
	sb.WriteString(factExamples)
	sb.WriteString("\n\n")
	sb.WriteString(constraintDoc)
	sb.WriteString("\n\n## User Query\n")
	sb.WriteString(query)
	sb.WriteString("\n\nOutput:")
	return sb.String(), nil
}

func BuildChatPrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	var m map[string]interface{}
	if md, ok := data.(map[string]interface{}); ok {
		m = md
	}

	if ps.Chat != nil {
		var contextBuilder strings.Builder
		contextBuilder.WriteString("## Context\n")

		if symbolID, ok := m["SymbolID"].(string); ok && symbolID != "" {
			if err := AppendSymbolContext(ctx, store, symbolID, &contextBuilder); err != nil {
				logger.Warn("Failed to fetch symbol context", "symbolID", symbolID, "error", err)
			}
		}

		if nodes, ok := m["Data"].([]interface{}); ok && len(nodes) > 0 {
			contextBuilder.WriteString("\n### Related Nodes\n")
			for i, node := range nodes {
				if i >= 10 {
					break
				}
				if nodeMap, ok := node.(map[string]interface{}); ok {
					name, _ := nodeMap["name"].(string)
					kind, _ := nodeMap["kind"].(string)
					code, _ := nodeMap["code"].(string)
					contextBuilder.WriteString(fmt.Sprintf("\n#### %s (%s)\n", name, kind))
					if code != "" {
						contextBuilder.WriteString("```\n")
						contextBuilder.WriteString(code)
						contextBuilder.WriteString("\n```\n")
					}
				}
			}
		}

		var query string
		if q := m["Query"]; q != nil {
			query = fmt.Sprintf("%v", q)
		} else if q := m["input"]; q != nil {
			query = fmt.Sprintf("%v", q)
		}

		var historyBuilder strings.Builder
		if rawMessages, ok := m["Messages"].([]interface{}); ok && len(rawMessages) > 0 {
			historyBuilder.WriteString("## Conversation History\n")
			for _, raw := range rawMessages {
				if msg, ok := raw.(map[string]interface{}); ok {
					role, _ := msg["role"].(string)
					content, _ := msg["content"].(string)
					if role == "user" {
						historyBuilder.WriteString(fmt.Sprintf("**User:** %s\n\n", content))
					} else if role == "ai" || role == "assistant" {
						historyBuilder.WriteString(fmt.Sprintf("**AI:** %s\n\n", content))
					}
				}
			}
			historyBuilder.WriteString("---\n\n")
		}

		if isRouteQuery(query) {
			appendRouteContext(store, &contextBuilder)
		}

		templateData := map[string]interface{}{
			"Query":   query,
			"Context": contextBuilder.String(),
			"History": historyBuilder.String(),
		}
		return ps.Chat.Execute(templateData)
	}

	var sb strings.Builder
	sb.WriteString("Chat analysis:\n")
	if m != nil {
		var query string
		if q := m["Query"]; q != nil {
			query = fmt.Sprintf("%v", q)
		} else if q := m["input"]; q != nil {
			query = fmt.Sprintf("%v", q)
		}
		if query != "" {
			sb.WriteString(fmt.Sprintf("Query: %s\n\n", query))
		}
	}
	sb.WriteString(FormatNodesSimple(data, 20))
	return sb.String(), nil
}

func BuildPathNarrativePrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.PathNarrative != nil {
		m, ok := data.(map[string]interface{})
		if !ok {
			return ps.PathNarrative.Execute(data)
		}

		var pathBuilder strings.Builder
		if nodes, ok := m["Data"].([]interface{}); ok {
			for i, node := range nodes {
				if i >= 20 {
					break
				}
				if nodeMap, ok := node.(map[string]interface{}); ok {
					if name, ok := nodeMap["name"].(string); ok {
						pathBuilder.WriteString(fmt.Sprintf("%d. %s\n", i+1, name))
					}
				}
			}
		}

		templateData := map[string]interface{}{
			"Query": m["Query"],
			"Path":  pathBuilder.String(),
		}
		return ps.PathNarrative.Execute(templateData)
	}
	var sb strings.Builder
	sb.WriteString("Path analysis:\n")
	sb.WriteString(ExtractPathString(data))
	return sb.String(), nil
}

func BuildPathEndpointsPrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.PathEndpoints != nil {
		m, ok := data.(map[string]interface{})
		if !ok {
			return ps.PathEndpoints.Execute(data)
		}

		var candidatesBuilder strings.Builder
		if candidates, ok := m["Data"].([]interface{}); ok {
			for i, c := range candidates {
				if i >= 20 {
					break
				}
				if s, ok := c.(string); ok {
					candidatesBuilder.WriteString(fmt.Sprintf("- %s\n", s))
				}
			}
		}

		templateData := map[string]interface{}{
			"Query":      m["Query"],
			"Candidates": candidatesBuilder.String(),
		}
		return ps.PathEndpoints.Execute(templateData)
	}
	var sb strings.Builder
	sb.WriteString("Endpoint analysis:\n")
	sb.WriteString(FormatNodeList(data))
	return sb.String(), nil
}

func BuildResolveSymbolPrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, symbolID string) (string, error) {
	if ps.ResolveSymbol != nil {
		return ps.ResolveSymbol.Execute(map[string]interface{}{"symbol_id": symbolID})
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Resolve symbol: %s\n", symbolID))
	return sb.String(), nil
}

func BuildPrunePrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.Prune != nil {
		m, ok := data.(map[string]interface{})
		if !ok {
			return ps.Prune.Execute(data)
		}

		var nodesBuilder strings.Builder
		if nodes, ok := m["Data"].([]interface{}); ok {
			for i, node := range nodes {
				if i >= 50 {
					break
				}
				if nodeMap, ok := node.(map[string]interface{}); ok {
					name, _ := nodeMap["name"].(string)
					kind, _ := nodeMap["kind"].(string)
					id, _ := nodeMap["id"].(string)
					nodesBuilder.WriteString(fmt.Sprintf("- %s (%s) [%s]\n", name, kind, id))
				}
			}
		}

		templateData := map[string]interface{}{
			"Nodes": nodesBuilder.String(),
		}
		return ps.Prune.Execute(templateData)
	}
	var sb strings.Builder
	sb.WriteString("Prune analysis:\n")
	sb.WriteString(FormatNodesSimple(data, 50))
	return sb.String(), nil
}

func BuildSmartSearchPrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.SmartSearch != nil {
		return ps.SmartSearch.Execute(data)
	}
	var sb strings.Builder
	sb.WriteString("Smart search analysis:\n")
	sb.WriteString(FormatNodesWithCode(data, 10))
	return sb.String(), nil
}

func BuildMultiFilePrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.MultiFile != nil {
		return ps.MultiFile.Execute(data)
	}

	files, ok := data.([]map[string]interface{})
	if !ok {
		return "Multi-file analysis: insufficient data", nil
	}

	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup
	results := make([]string, len(files))
	var mu sync.Mutex

	for i, file := range files {
		wg.Add(1)
		go func(idx int, f map[string]interface{}) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var localSb strings.Builder
			if name, ok := f["name"].(string); ok {
				localSb.WriteString(fmt.Sprintf("## File: %s\n", name))
			}
			if content, ok := f["content"].(string); ok {
				localSb.WriteString("```\n")
				localSb.WriteString(content)
				localSb.WriteString("\n```\n")
			}
			if summary, ok := f["summary"].(string); ok {
				localSb.WriteString(fmt.Sprintf("Summary: %s\n", summary))
			}

			mu.Lock()
			results[idx] = localSb.String()
			mu.Unlock()
		}(i, file)
	}
	wg.Wait()

	var sb strings.Builder
	sb.WriteString("## Multi-File Analysis\n\n")
	for _, r := range results {
		sb.WriteString(r)
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func BuildDefaultContextPrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.DefaultContext != nil {
		return ps.DefaultContext.Execute(data)
	}
	var sb strings.Builder
	sb.WriteString("Default context analysis:\n")
	sb.WriteString(FormatNodesWithCode(data, 10))
	return sb.String(), nil
}

func BuildInsightPrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.Insight != nil {
		m, ok := data.(map[string]interface{})
		if !ok {
			return ps.Insight.Execute(data)
		}

		var contextBuilder strings.Builder
		if symbolID, ok := m["SymbolID"].(string); ok && symbolID != "" {
			if err := AppendSymbolContext(ctx, store, symbolID, &contextBuilder); err != nil {
				logger.Warn("Failed to fetch symbol context", "symbolID", symbolID, "error", err)
			}
		}

		templateData := map[string]interface{}{
			"SymbolID": m["SymbolID"],
			"Context":  contextBuilder.String(),
		}
		return ps.Insight.Execute(templateData)
	}
	var sb strings.Builder
	sb.WriteString("Insight analysis:\n")
	sb.WriteString(FormatNodesWithCode(data, 10))
	return sb.String(), nil
}

func BuildSummaryPrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.Summary != nil {
		m, ok := data.(map[string]interface{})
		if !ok {
			return ps.Summary.Execute(data)
		}

		var symbolsBuilder strings.Builder
		if nodes, ok := m["Data"].([]interface{}); ok {
			for i, node := range nodes {
				if i >= 20 {
					break
				}
				if nodeMap, ok := node.(map[string]interface{}); ok {
					name, _ := nodeMap["name"].(string)
					kind, _ := nodeMap["kind"].(string)
					symbolsBuilder.WriteString(fmt.Sprintf("- %s (%s)\n", name, kind))
				}
			}
		}

		templateData := map[string]interface{}{
			"Query":   m["Query"],
			"Symbols": symbolsBuilder.String(),
		}
		return ps.Summary.Execute(templateData)
	}
	var sb strings.Builder
	sb.WriteString("Summary analysis:\n")
	sb.WriteString(FormatNodesSimple(data, 20))
	return sb.String(), nil
}

func BuildNarrativePrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.Narrative != nil {
		m, ok := data.(map[string]interface{})
		if !ok {
			return ps.Narrative.Execute(data)
		}

		var componentsBuilder strings.Builder
		if nodes, ok := m["Data"].([]interface{}); ok {
			for i, node := range nodes {
				if i >= 30 {
					break
				}
				if nodeMap, ok := node.(map[string]interface{}); ok {
					if name, ok := nodeMap["name"].(string); ok {
						componentsBuilder.WriteString(fmt.Sprintf("- %s\n", name))
					}
				}
			}
		}

		templateData := map[string]interface{}{
			"Query":      m["Query"],
			"Components": componentsBuilder.String(),
		}
		return ps.Narrative.Execute(templateData)
	}
	var sb strings.Builder
	sb.WriteString("Narrative analysis:\n")
	sb.WriteString(FormatNodesSimple(data, 30))
	return sb.String(), nil
}

func BuildRefactorPrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.Refactor != nil {
		m, ok := data.(map[string]interface{})
		if !ok {
			return ps.Refactor.Execute(data)
		}

		var contextBuilder strings.Builder
		if nodes, ok := m["Data"].([]interface{}); ok && len(nodes) > 0 {
			for i, node := range nodes {
				if i >= 10 {
					break
				}
				if nodeMap, ok := node.(map[string]interface{}); ok {
					name, _ := nodeMap["name"].(string)
					kind, _ := nodeMap["kind"].(string)
					code, _ := nodeMap["code"].(string)
					contextBuilder.WriteString(fmt.Sprintf("\n### %s (%s)\n", name, kind))
					if code != "" {
						contextBuilder.WriteString("```\n" + code + "\n```\n")
					}
				}
			}
		}

		templateData := map[string]interface{}{
			"Query":   m["Query"],
			"SymbolID": m["SymbolID"],
			"Context":  contextBuilder.String(),
		}
		return ps.Refactor.Execute(templateData)
	}
	var sb strings.Builder
	sb.WriteString("Refactoring analysis:\n")
	sb.WriteString(FormatNodesWithCode(data, 10))
	return sb.String(), nil
}

func BuildTestGenPrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.TestGen != nil {
		m, ok := data.(map[string]interface{})
		if !ok {
			return ps.TestGen.Execute(data)
		}

		var contextBuilder strings.Builder
		if nodes, ok := m["Data"].([]interface{}); ok && len(nodes) > 0 {
			for i, node := range nodes {
				if i >= 10 {
					break
				}
				if nodeMap, ok := node.(map[string]interface{}); ok {
					name, _ := nodeMap["name"].(string)
					kind, _ := nodeMap["kind"].(string)
					code, _ := nodeMap["code"].(string)
					contextBuilder.WriteString(fmt.Sprintf("\n### %s (%s)\n", name, kind))
					if code != "" {
						contextBuilder.WriteString("```\n" + code + "\n```\n")
					}
				}
			}
		}

		templateData := map[string]interface{}{
			"Query":   m["Query"],
			"SymbolID": m["SymbolID"],
			"Context":  contextBuilder.String(),
		}
		return ps.TestGen.Execute(templateData)
	}
	var sb strings.Builder
	sb.WriteString("Test generation analysis:\n")
	sb.WriteString(FormatNodesWithCode(data, 10))
	return sb.String(), nil
}

func BuildSecurityPrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.Security != nil {
		m, ok := data.(map[string]interface{})
		if !ok {
			return ps.Security.Execute(data)
		}

		var contextBuilder strings.Builder
		if nodes, ok := m["Data"].([]interface{}); ok && len(nodes) > 0 {
			for i, node := range nodes {
				if i >= 10 {
					break
				}
				if nodeMap, ok := node.(map[string]interface{}); ok {
					name, _ := nodeMap["name"].(string)
					kind, _ := nodeMap["kind"].(string)
					code, _ := nodeMap["code"].(string)
					contextBuilder.WriteString(fmt.Sprintf("\n### %s (%s)\n", name, kind))
					if code != "" {
						contextBuilder.WriteString("```\n" + code + "\n```\n")
					}
				}
			}
		}

		templateData := map[string]interface{}{
			"Query":   m["Query"],
			"SymbolID": m["SymbolID"],
			"Context":  contextBuilder.String(),
		}
		return ps.Security.Execute(templateData)
	}
	var sb strings.Builder
	sb.WriteString("Security audit:\n")
	sb.WriteString(FormatNodesWithCode(data, 10))
	return sb.String(), nil
}

func BuildPerformancePrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.Performance != nil {
		m, ok := data.(map[string]interface{})
		if !ok {
			return ps.Performance.Execute(data)
		}

		var contextBuilder strings.Builder
		if nodes, ok := m["Data"].([]interface{}); ok && len(nodes) > 0 {
			for i, node := range nodes {
				if i >= 10 {
					break
				}
				if nodeMap, ok := node.(map[string]interface{}); ok {
					name, _ := nodeMap["name"].(string)
					kind, _ := nodeMap["kind"].(string)
					code, _ := nodeMap["code"].(string)
					contextBuilder.WriteString(fmt.Sprintf("\n### %s (%s)\n", name, kind))
					if code != "" {
						contextBuilder.WriteString("```\n" + code + "\n```\n")
					}
				}
			}
		}

		templateData := map[string]interface{}{
			"Query":   m["Query"],
			"SymbolID": m["SymbolID"],
			"Context":  contextBuilder.String(),
		}
		return ps.Performance.Execute(templateData)
	}
	var sb strings.Builder
	sb.WriteString("Performance analysis:\n")
	sb.WriteString(FormatNodesWithCode(data, 10))
	return sb.String(), nil
}

func BuildPrompt(task string, ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps == nil {
		return buildFallbackPrompt(task, ctx, store, data)
	}
	switch task {
	case "datalog":
		return BuildDatalogPrompt(ctx, store, ps, data)
	case "chat":
		return BuildChatPrompt(ctx, store, ps, data)
	case "path_narrative":
		return BuildPathNarrativePrompt(ctx, store, ps, data)
	case "path_endpoints":
		return BuildPathEndpointsPrompt(ctx, store, ps, data)
	case "resolve_symbol":
		symbolID, _ := data.(string)
		return BuildResolveSymbolPrompt(ctx, store, ps, symbolID)
	case "prune":
		return BuildPrunePrompt(ctx, store, ps, data)
	case "smart_search_analysis":
		return BuildSmartSearchPrompt(ctx, store, ps, data)
	case "multi_file_summary":
		return BuildMultiFilePrompt(ctx, store, ps, data)
	case "default_context":
		return BuildDefaultContextPrompt(ctx, store, ps, data)
	case "insight":
		return BuildInsightPrompt(ctx, store, ps, data)
	case "summary":
		return BuildSummaryPrompt(ctx, store, ps, data)
	case "narrative":
		return BuildNarrativePrompt(ctx, store, ps, data)
	case "refactor":
		return BuildRefactorPrompt(ctx, store, ps, data)
	case "test_generation":
		return BuildTestGenPrompt(ctx, store, ps, data)
	case "security_audit":
		return BuildSecurityPrompt(ctx, store, ps, data)
	case "performance":
		return BuildPerformancePrompt(ctx, store, ps, data)
	default:
		return BuildDefaultContextPrompt(ctx, store, ps, data)
	}
}

func buildFallbackPrompt(task string, ctx context.Context, store *meb.MEBStore, data interface{}) (string, error) {
	var query string
	var symbolContext string
	if m, ok := data.(map[string]interface{}); ok {
		query, _ = m["Query"].(string)
		symbolContext, _ = m["SymbolContext"].(string)
	}
	if symbolContext != "" {
		return fmt.Sprintf("Context:\n%s\n\nUser Question: %s\n\nProvide a clear, helpful answer based on the available code context.", symbolContext, query), nil
	}
	return fmt.Sprintf("User Question: %s\n\nProvide a clear, helpful answer based on the available code context.", query), nil
}