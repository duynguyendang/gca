package promptbuilder

import (
	"context"
	"fmt"
	"strings"
	"sync"

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

func BuildChatPrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.Chat != nil {
		return ps.Chat.Execute(data)
	}
	var sb strings.Builder
	sb.WriteString("Chat analysis:\n")
	sb.WriteString(FormatNodesSimple(data, 20))
	return sb.String(), nil
}

func BuildPathNarrativePrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.PathNarrative != nil {
		return ps.PathNarrative.Execute(data)
	}
	var sb strings.Builder
	sb.WriteString("Path analysis:\n")
	sb.WriteString(ExtractPathString(data))
	return sb.String(), nil
}

func BuildPathEndpointsPrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.PathEndpoints != nil {
		return ps.PathEndpoints.Execute(data)
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
		return ps.Prune.Execute(data)
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
		return ps.Insight.Execute(data)
	}
	var sb strings.Builder
	sb.WriteString("Insight analysis:\n")
	sb.WriteString(FormatNodesWithCode(data, 10))
	return sb.String(), nil
}

func BuildSummaryPrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.Summary != nil {
		return ps.Summary.Execute(data)
	}
	var sb strings.Builder
	sb.WriteString("Summary analysis:\n")
	sb.WriteString(FormatNodesSimple(data, 20))
	return sb.String(), nil
}

func BuildNarrativePrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.Narrative != nil {
		return ps.Narrative.Execute(data)
	}
	var sb strings.Builder
	sb.WriteString("Narrative analysis:\n")
	sb.WriteString(FormatNodesSimple(data, 30))
	return sb.String(), nil
}

func BuildRefactorPrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.Refactor != nil {
		return ps.Refactor.Execute(data)
	}
	var sb strings.Builder
	sb.WriteString("Refactoring analysis:\n")
	sb.WriteString(FormatNodesWithCode(data, 10))
	return sb.String(), nil
}

func BuildTestGenPrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.TestGen != nil {
		return ps.TestGen.Execute(data)
	}
	var sb strings.Builder
	sb.WriteString("Test generation analysis:\n")
	sb.WriteString(FormatNodesWithCode(data, 10))
	return sb.String(), nil
}

func BuildSecurityPrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.Security != nil {
		return ps.Security.Execute(data)
	}
	var sb strings.Builder
	sb.WriteString("Security audit:\n")
	sb.WriteString(FormatNodesWithCode(data, 10))
	return sb.String(), nil
}

func BuildPerformancePrompt(ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
	if ps.Performance != nil {
		return ps.Performance.Execute(data)
	}
	var sb strings.Builder
	sb.WriteString("Performance analysis:\n")
	sb.WriteString(FormatNodesWithCode(data, 10))
	return sb.String(), nil
}

func BuildPrompt(task string, ctx context.Context, store *meb.MEBStore, ps *PromptSet, data interface{}) (string, error) {
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