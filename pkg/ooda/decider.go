package ooda

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/prompts"
	"github.com/duynguyendang/meb"
)

type PromptLoader interface {
	LoadPrompt(name string) (*prompts.Prompt, error)
}

type GraphDecider struct {
	storeManager         StoreManager
	promptLoader         PromptLoader
	DatalogPrompt        *prompts.Prompt
	ChatPrompt           *prompts.Prompt
	PathNarrativePrompt  *prompts.Prompt
	PathEndpointsPrompt  *prompts.Prompt
	ResolveSymbolPrompt  *prompts.Prompt
	PrunePrompt          *prompts.Prompt
	SmartSearchPrompt    *prompts.Prompt
	MultiFilePrompt      *prompts.Prompt
	DefaultContextPrompt *prompts.Prompt
}

func NewGraphDecider(storeManager StoreManager, promptLoader PromptLoader) *GraphDecider {
	d := &GraphDecider{
		storeManager: storeManager,
		promptLoader: promptLoader,
	}

	loadPrompt := func(name string) *prompts.Prompt {
		p, _ := promptLoader.LoadPrompt("prompts/" + name)
		return p
	}

	d.DatalogPrompt = loadPrompt("datalog.prompt")
	d.ChatPrompt = loadPrompt("chat.prompt")
	d.PathNarrativePrompt = loadPrompt("path_narrative.prompt")
	d.PathEndpointsPrompt = loadPrompt("path_endpoints.prompt")
	d.ResolveSymbolPrompt = loadPrompt("resolve_symbol.prompt")
	d.PrunePrompt = loadPrompt("prune.prompt")
	d.SmartSearchPrompt = loadPrompt("smart_search.prompt")
	d.MultiFilePrompt = loadPrompt("multi_file.prompt")
	d.DefaultContextPrompt = loadPrompt("default_context.prompt")

	return d
}

func (d *GraphDecider) Decide(ctx context.Context, frame *GCAFrame) error {
	frame.Phase = PhaseDecide

	store, err := d.storeManager.GetStore(frame.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get store: %w", err)
	}

	prompt, err := d.buildPrompt(ctx, store, frame)
	if err != nil {
		return fmt.Errorf("failed to build prompt: %w", err)
	}

	frame.Prompt = prompt

	frame.Context = append(frame.Context, Atom{
		Predicate: "prompt_built",
		Subject:   frame.ID.String(),
		Object:    fmt.Sprintf("length:%d", len(prompt)),
		Weight:    0.9,
	})

	return nil
}

func (d *GraphDecider) buildPrompt(ctx context.Context, store *meb.MEBStore, frame *GCAFrame) (string, error) {
	switch frame.Task {
	case TaskInsight:
		return d.buildInsightPrompt(ctx, store, frame)

	case TaskChat:
		return d.buildChatPrompt(frame)

	case TaskPrune:
		return d.buildPrunePrompt(frame)

	case TaskSummary:
		return d.buildSummaryPrompt(ctx, store, frame)

	case TaskNarrative:
		return d.buildNarrativePrompt(ctx, store, frame)

	case TaskResolveSymbol:
		return d.buildResolveSymbolPrompt(frame)

	case TaskPathEndpoints:
		return d.buildPathEndpointsPrompt(frame)

	case TaskDatalog:
		return d.buildDatalogPrompt(frame)

	case TaskPathNarrative:
		return d.buildPathNarrativePrompt(frame)

	case TaskSmartSearchAnalysis:
		return d.buildSmartSearchPrompt(frame)

	case TaskMultiFileSummary:
		return d.buildMultiFilePrompt(ctx, store, frame)

	case TaskRefactor:
		return d.buildRefactorPrompt(ctx, store, frame)

	case TaskTestGeneration:
		return d.buildTestGenerationPrompt(ctx, store, frame)

	case TaskSecurityAudit:
		return d.buildSecurityAuditPrompt(ctx, store, frame)

	case TaskPerformance:
		return d.buildPerformancePrompt(ctx, store, frame)

	default:
		return d.buildDefaultPrompt(ctx, store, frame)
	}
}

func (d *GraphDecider) buildInsightPrompt(ctx context.Context, store *meb.MEBStore, frame *GCAFrame) (string, error) {
	symbolID := frame.SymbolID
	if symbolID == "" {
		symbols := ExtractPotentialSymbols(frame.Input)
		if len(symbols) > 0 {
			symbolID = symbols[0]
		}
	}

	if symbolID == "" {
		return "", fmt.Errorf("no symbol ID available for insight task")
	}

	return fmt.Sprintf("Analyze the architectural role of component %s. Provide a comprehensive analysis including role, interactions, and design patterns.", symbolID), nil
}

func (d *GraphDecider) buildChatPrompt(frame *GCAFrame) (string, error) {
	context := formatNodesWithCode(frame.Data, 20)
	if d.ChatPrompt != nil {
		return d.ChatPrompt.Execute(map[string]interface{}{
			"Query":   frame.Input,
			"Context": context,
		})
	}
	return fmt.Sprintf("%s\n\n%s", frame.Input, context), nil
}

func (d *GraphDecider) buildPrunePrompt(frame *GCAFrame) (string, error) {
	nodes := formatNodeList(frame.Data)
	if d.PrunePrompt != nil {
		return d.PrunePrompt.Execute(map[string]interface{}{
			"Nodes": nodes,
		})
	}
	return "", fmt.Errorf("prune.prompt not loaded")
}

func (d *GraphDecider) buildSummaryPrompt(ctx context.Context, store *meb.MEBStore, frame *GCAFrame) (string, error) {
	nodes := formatNodesSimple(frame.Data, 15)
	query := frame.Input
	if query == "" {
		query = frame.SymbolID
	}
	return fmt.Sprintf("Provide a 2-3 sentence architectural summary for file \"%s\".\nSymbols:\n%s", query, nodes), nil
}

func (d *GraphDecider) buildNarrativePrompt(ctx context.Context, store *meb.MEBStore, frame *GCAFrame) (string, error) {
	names := extractNodeNames(frame.Data)
	return fmt.Sprintf("Explain the high-level logic flow for these components: %s. Keep it concise.", names), nil
}

func (d *GraphDecider) buildResolveSymbolPrompt(frame *GCAFrame) (string, error) {
	candidates := extractStringList(frame.Data, 30)
	if d.ResolveSymbolPrompt != nil {
		return d.ResolveSymbolPrompt.Execute(map[string]interface{}{
			"Query":      frame.Input,
			"Candidates": candidates,
		})
	}
	return "", fmt.Errorf("resolve_symbol.prompt not loaded")
}

func (d *GraphDecider) buildPathEndpointsPrompt(frame *GCAFrame) (string, error) {
	candidates := extractStringList(frame.Data, 50)
	if d.PathEndpointsPrompt != nil {
		return d.PathEndpointsPrompt.Execute(map[string]interface{}{
			"Query":      frame.Input,
			"Candidates": candidates,
		})
	}
	return "", fmt.Errorf("path_endpoints.prompt not loaded")
}

func (d *GraphDecider) buildDatalogPrompt(frame *GCAFrame) (string, error) {
	predicatesList := formatPredicatesList(frame.Data)
	if d.DatalogPrompt != nil {
		return d.DatalogPrompt.Execute(map[string]interface{}{
			"Query":      frame.Input,
			"SymbolID":   frame.SymbolID,
			"Predicates": predicatesList,
		})
	}
	return "", fmt.Errorf("datalog.prompt not loaded")
}

func (d *GraphDecider) buildPathNarrativePrompt(frame *GCAFrame) (string, error) {
	pathStr := extractPathString(frame.Data)
	if d.PathNarrativePrompt != nil {
		promptStr, err := d.PathNarrativePrompt.Execute(map[string]interface{}{
			"Query": frame.Input,
			"Path":  pathStr,
		})
		if err == nil {
			return promptStr, nil
		}
	}
	return fmt.Sprintf("Explain flow: %s. Path: %s", frame.Input, pathStr), nil
}

func (d *GraphDecider) buildSmartSearchPrompt(frame *GCAFrame) (string, error) {
	nodes := formatGraphResults(frame.Data, "nodes")
	links := formatGraphResults(frame.Data, "links")

	if d.SmartSearchPrompt != nil {
		return d.SmartSearchPrompt.Execute(map[string]interface{}{
			"Nodes": nodes,
			"Links": links,
			"Query": frame.Input,
		})
	}
	return "", fmt.Errorf("smart_search.prompt not loaded")
}

func (d *GraphDecider) buildMultiFilePrompt(ctx context.Context, store *meb.MEBStore, frame *GCAFrame) (string, error) {
	fileIDs := make([]string, 0)
	if list, ok := frame.Data.([]interface{}); ok {
		for _, item := range list {
			if s, ok := item.(string); ok {
				fileIDs = append(fileIDs, s)
			}
		}
	}

	if len(fileIDs) > 20 {
		fileIDs = fileIDs[:20]
	}

	var contextBuilder strings.Builder
	contextBuilder.WriteString("## Context\n")

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, fileID := range fileIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			var localSb strings.Builder
			localCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			if err := appendSymbolContext(localCtx, store, id, &localSb); err == nil {
				mu.Lock()
				contextBuilder.WriteString(localSb.String())
				mu.Unlock()
			}
		}(fileID)
	}
	wg.Wait()

	if d.MultiFilePrompt != nil {
		return d.MultiFilePrompt.Execute(map[string]interface{}{
			"Context": contextBuilder.String(),
			"Query":   frame.Input,
		})
	}
	return "", fmt.Errorf("multi_file.prompt not loaded")
}

func (d *GraphDecider) buildDefaultPrompt(ctx context.Context, store *meb.MEBStore, frame *GCAFrame) (string, error) {
	var contextBuilder strings.Builder
	contextBuilder.WriteString("## Context\n")

	if frame.SymbolID != "" {
		appendSymbolContext(ctx, store, frame.SymbolID, &contextBuilder)
	} else {
		words := ExtractPotentialSymbols(frame.Input)
		count := 0
		seen := make(map[string]bool)
		for _, word := range words {
			if count >= 3 {
				break
			}
			if seen[word] {
				continue
			}
			seen[word] = true

			_, exists := store.LookupID(word)
			if exists {
				count++
				appendSymbolContext(ctx, store, word, &contextBuilder)
			}
		}
	}

	if d.DefaultContextPrompt != nil {
		return d.DefaultContextPrompt.Execute(map[string]interface{}{
			"Context": contextBuilder.String(),
			"Query":   frame.Input,
		})
	}

	return fmt.Sprintf(`You are an expert Software Architect assistant.
Assign context to the user's question using the provided Code and Graph information.

%s

## User Question
%s

Answer concisely and accurately based on the code provided.`, contextBuilder.String(), frame.Input), nil
}

func formatNodesWithCode(data interface{}, limit int) string {
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

func formatNodesSimple(data interface{}, limit int) string {
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

func formatPredicatesList(data interface{}) string {
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

func formatNodeList(data interface{}) string {
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

func formatGraphResults(data interface{}, key string) string {
	if data == nil {
		return ""
	}
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

func extractNodeNames(data interface{}) string {
	if data == nil {
		return ""
	}
	list, ok := data.([]interface{})
	if !ok {
		return ""
	}
	names := make([]string, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]interface{}); ok {
			if name, ok := m["name"].(string); ok {
				names = append(names, name)
			}
		}
	}
	return strings.Join(names, ", ")
}

func extractStringList(data interface{}, limit int) string {
	if data == nil {
		return ""
	}
	list, ok := data.([]interface{})
	if !ok {
		return ""
	}
	items := make([]string, 0)
	for i, item := range list {
		if i >= limit {
			break
		}
		if str, ok := item.(string); ok {
			items = append(items, str)
		}
	}
	return strings.Join(items, "\n")
}

func extractPathString(data interface{}) string {
	if data == nil {
		return ""
	}
	list, ok := data.([]interface{})
	if !ok {
		return ""
	}
	names := make([]string, 0)
	for _, item := range list {
		if m, ok := item.(map[string]interface{}); ok {
			if name, ok := m["name"].(string); ok {
				names = append(names, name)
			}
		}
	}
	return strings.Join(names, " -> ")
}

func appendSymbolContext(ctx context.Context, store *meb.MEBStore, symbolID string, sb *strings.Builder) error {
	contentBytes, err := store.GetContentByKey(symbolID)
	if err != nil {
		return err
	}
	content := string(contentBytes)

	sb.WriteString(fmt.Sprintf("\n### Symbol: %s\n", symbolID))
	sb.WriteString("```go\n")
	if len(content) > 2000 {
		sb.WriteString(content[:2000] + "\n... (truncated)")
	} else {
		sb.WriteString(content)
	}
	sb.WriteString("\n```\n")

	return nil
}

func (d *GraphDecider) buildRefactorPrompt(ctx context.Context, store *meb.MEBStore, frame *GCAFrame) (string, error) {
	var sb strings.Builder

	sb.WriteString("# Code Refactoring Analysis\n\n")
	sb.WriteString(fmt.Sprintf("## User Request\n%s\n\n", frame.Input))

	symbols := ExtractPotentialSymbols(frame.Input)
	if len(symbols) > 0 {
		sb.WriteString("## Target Symbols\n")
		for _, sym := range symbols {
			sb.WriteString(fmt.Sprintf("- %s\n", sym))
			_ = appendSymbolContext(ctx, store, sym, &sb)
		}
	}

	sb.WriteString("## Analysis Guidelines\n")
	sb.WriteString("Analyze the code for:\n")
	sb.WriteString("1. Code smells and duplication\n")
	sb.WriteString("2. Opportunities for method extraction\n")
	sb.WriteString("3. Simplification opportunities\n")
	sb.WriteString("4. Suggested refactoring steps\n")
	sb.WriteString("5. Estimated impact (low/medium/high)\n")

	return sb.String(), nil
}

func (d *GraphDecider) buildTestGenerationPrompt(ctx context.Context, store *meb.MEBStore, frame *GCAFrame) (string, error) {
	var sb strings.Builder

	sb.WriteString("# Test Generation Request\n\n")
	sb.WriteString(fmt.Sprintf("## User Request\n%s\n\n", frame.Input))

	symbols := ExtractPotentialSymbols(frame.Input)
	if len(symbols) > 0 {
		sb.WriteString("## Target Symbols\n")
		for _, sym := range symbols {
			sb.WriteString(fmt.Sprintf("- %s\n", sym))
			_ = appendSymbolContext(ctx, store, sym, &sb)
		}
	}

	sb.WriteString("## Test Generation Guidelines\n")
	sb.WriteString("Generate appropriate tests:\n")
	sb.WriteString("1. Identify test type (unit/integration/e2e)\n")
	sb.WriteString("2. Generate test cases for edge cases\n")
	sb.WriteString("3. Include mock/stub suggestions\n")
	sb.WriteString("4. Show expected assertions\n")

	return sb.String(), nil
}

func (d *GraphDecider) buildSecurityAuditPrompt(ctx context.Context, store *meb.MEBStore, frame *GCAFrame) (string, error) {
	var sb strings.Builder

	sb.WriteString("# Security Audit Request\n\n")
	sb.WriteString(fmt.Sprintf("## User Request\n%s\n\n", frame.Input))

	symbols := ExtractPotentialSymbols(frame.Input)
	if len(symbols) > 0 {
		sb.WriteString("## Target Symbols\n")
		for _, sym := range symbols {
			sb.WriteString(fmt.Sprintf("- %s\n", sym))
			_ = appendSymbolContext(ctx, store, sym, &sb)
		}
	}

	sb.WriteString("## Security Audit Guidelines\n")
	sb.WriteString("Analyze for:\n")
	sb.WriteString("1. Input validation issues\n")
	sb.WriteString("2. Authentication/authorization concerns\n")
	sb.WriteString("3. Injection vulnerabilities\n")
	sb.WriteString("4. Data exposure risks\n")
	sb.WriteString("5. Recommended fixes with severity (critical/high/medium/low)\n")

	return sb.String(), nil
}

func (d *GraphDecider) buildPerformancePrompt(ctx context.Context, store *meb.MEBStore, frame *GCAFrame) (string, error) {
	var sb strings.Builder

	sb.WriteString("# Performance Analysis Request\n\n")
	sb.WriteString(fmt.Sprintf("## User Request\n%s\n\n", frame.Input))

	symbols := ExtractPotentialSymbols(frame.Input)
	if len(symbols) > 0 {
		sb.WriteString("## Target Symbols\n")
		for _, sym := range symbols {
			sb.WriteString(fmt.Sprintf("- %s\n", sym))
			_ = appendSymbolContext(ctx, store, sym, &sb)
		}
	}

	sb.WriteString("## Performance Analysis Guidelines\n")
	sb.WriteString("Analyze for:\n")
	sb.WriteString("1. Time complexity issues\n")
	sb.WriteString("2. Memory allocation problems\n")
	sb.WriteString("3. Bottleneck identification\n")
	sb.WriteString("4. Optimization suggestions\n")
	sb.WriteString("5. Expected improvement impact\n")

	return sb.String(), nil
}
