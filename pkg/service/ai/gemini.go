package ai

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/duynguyendang/gca/pkg/config"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/ooda"
	"github.com/duynguyendang/gca/pkg/prompts"
	"github.com/duynguyendang/meb"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core/api"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/anthropic"
	"github.com/firebase/genkit/go/plugins/compat_oai/openai"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	"github.com/firebase/genkit/go/plugins/ollama"
)

type ProjectStoreManager interface {
	GetStore(projectID string) (*meb.MEBStore, error)
}

type AIService struct {
	g              *genkit.Genkit
	manager        ProjectStoreManager
	defaultModel   string
	embeddingModel string
	provider       string

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

func NewAIService(ctx context.Context, manager ProjectStoreManager) (*AIService, error) {
	provider := os.Getenv("LLM_PROVIDER")
	if provider == "" {
		provider = "googleai"
	}

	apiKey := os.Getenv("LLM_API_KEY")
	if apiKey == "" && provider != "ollama" {
		return nil, fmt.Errorf("LLM_API_KEY not found")
	}

	var plugins []api.Plugin

	switch provider {
	case "googleai", "gemini":
		plugins = append(plugins, &googlegenai.GoogleAI{APIKey: apiKey})
	case "openai":
		plugins = append(plugins, &openai.OpenAI{APIKey: apiKey})
	case "anthropic":
		plugins = append(plugins, &anthropic.Anthropic{APIKey: apiKey})
	case "ollama":
		addr := os.Getenv("OLLAMA_ADDRESS")
		if addr == "" {
			addr = "http://localhost:11434"
		}
		plugins = append(plugins, &ollama.Ollama{ServerAddress: addr})
	default:
		plugins = append(plugins, &googlegenai.GoogleAI{APIKey: apiKey})
	}

	defaultModel := os.Getenv("LLM_MODEL")
	if defaultModel == "" {
		switch provider {
		case "googleai", "gemini":
			defaultModel = "googleai/gemini-2.5-flash"
		case "openai":
			defaultModel = "openai/gpt-4o"
		case "anthropic":
			defaultModel = "anthropic/claude-3-5-sonnet-20241022"
		case "ollama":
			defaultModel = "ollama/llama3.2"
		default:
			defaultModel = "googleai/gemini-2.5-flash"
		}
	} else if !strings.Contains(defaultModel, "/") {
		defaultModel = provider + "/" + defaultModel
	}

	embeddingModel := os.Getenv("EMBEDDING_MODEL")
	if embeddingModel == "" {
		switch provider {
		case "googleai", "gemini":
			embeddingModel = "googleai/text-embedding-004"
		case "openai":
			embeddingModel = "openai/text-embedding-3-large"
		case "anthropic":
			embeddingModel = ""
		case "ollama":
			embeddingModel = "ollama/nomic-embed-text"
		default:
			embeddingModel = "googleai/text-embedding-004"
		}
	} else if !strings.Contains(embeddingModel, "/") {
		embeddingModel = provider + "/" + embeddingModel
	}

	g := genkit.Init(ctx, genkit.WithPlugins(plugins...), genkit.WithDefaultModel(defaultModel))

	loadPrompt := func(name string) *prompts.Prompt {
		path, ok := config.PromptPaths[name]
		if !ok {
			log.Printf("Warning: No prompt path configured for %s", name)
			return nil
		}
		p, err := prompts.LoadPrompt(path)
		if err != nil {
			log.Printf("Warning: Failed to load %s from %s: %v", name, path, err)
			return nil
		}
		return p
	}

	log.Printf("AI Service initialized: provider=%s, model=%s, embedding=%s", provider, defaultModel, embeddingModel)

	return &AIService{
		g:                    g,
		manager:              manager,
		defaultModel:         defaultModel,
		embeddingModel:       embeddingModel,
		provider:             provider,
		DatalogPrompt:        loadPrompt("datalog"),
		ChatPrompt:           loadPrompt("chat"),
		PathNarrativePrompt:  loadPrompt("path_narrative"),
		PathEndpointsPrompt:  loadPrompt("path_endpoints"),
		ResolveSymbolPrompt:  loadPrompt("resolve_symbol"),
		PrunePrompt:          loadPrompt("prune"),
		SmartSearchPrompt:    loadPrompt("smart_search"),
		MultiFilePrompt:      loadPrompt("multi_file"),
		DefaultContextPrompt: loadPrompt("default_context"),
	}, nil
}

func (s *AIService) GenerateText(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	log.Printf("Sending Prompt to LLM (%s):\n%s", s.provider, prompt)

	resp, err := genkit.Generate(ctx, s.g,
		ai.WithModelName(s.defaultModel),
		ai.WithPrompt(prompt),
	)
	if err != nil {
		log.Printf("LLM Request Failed:\n%s\nError: %v", prompt, err)
		return "", err
	}

	return resp.Text(), nil
}

func (s *AIService) GetEmbedding(ctx context.Context, text string) ([]float32, error) {
	if s.embeddingModel == "" {
		return nil, fmt.Errorf("embedding model not configured for provider %s", s.provider)
	}
	if text == "" {
		return nil, fmt.Errorf("empty text for embedding")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := genkit.Embed(ctx, s.g,
		ai.WithEmbedderName(s.embeddingModel),
		ai.WithTextDocs(text),
	)
	if err != nil {
		return nil, fmt.Errorf("embedding generation failed: %w", err)
	}

	if len(resp.Embeddings) == 0 || len(resp.Embeddings[0].Embedding) == 0 {
		return nil, fmt.Errorf("no embedding values returned")
	}

	values := resp.Embeddings[0].Embedding
	result := make([]float32, len(values))
	for i, v := range values {
		result[i] = float32(v)
	}
	return result, nil
}

type AIRequest struct {
	ProjectID        string      `json:"project_id"`
	Task             string      `json:"task"`
	Query            string      `json:"query"`
	SymbolID         string      `json:"symbol_id"`
	Data             interface{} `json:"data"`
	ContextMode      string      `json:"context_mode,omitempty"`
	QueryInstruction string      `json:"query_instruction,omitempty"`
}

func (s *AIService) HandleRequest(ctx context.Context, req AIRequest) (string, error) {
	store, err := s.manager.GetStore(req.ProjectID)
	if err != nil {
		return "", fmt.Errorf("failed to get store: %w", err)
	}

	prompt, err := s.buildTaskPrompt(ctx, store, req)
	if err != nil {
		return "", fmt.Errorf("failed to build prompt: %w", err)
	}

	log.Printf("Sending AI Prompt (Task: %s, Length: %d chars)", req.Task, len(prompt))

	return s.GenerateText(ctx, prompt)
}

func (s *AIService) buildTaskPrompt(ctx context.Context, store *meb.MEBStore, req AIRequest) (string, error) {
	switch req.Task {
	case "insight":
		return s.buildInsightPrompt(ctx, store, req)
	case "chat":
		return s.buildChatPrompt(req)
	case "prune":
		return s.buildPrunePrompt(req)
	case "summary":
		return s.buildSummaryPrompt(ctx, store, req)
	case "narrative":
		return s.buildNarrativePrompt(ctx, store, req)
	case "resolve_symbol":
		return s.buildResolveSymbolPrompt(req)
	case "path_endpoints":
		return s.buildPathEndpointsPrompt(req)
	case "datalog":
		return s.buildDatalogPrompt(req)
	case "path_narrative":
		return s.buildPathNarrativePrompt(ctx, store, req)
	case "smart_search_analysis":
		return s.buildSmartSearchPrompt(req)
	case "multi_file_summary":
		return s.buildMultiFileSummaryPrompt(ctx, store, req)
	case "refactor":
		return s.buildRefactorPrompt(ctx, store, req)
	case "test_generation":
		return s.buildTestGenerationPrompt(ctx, store, req)
	case "security_audit":
		return s.buildSecurityAuditPrompt(ctx, store, req)
	case "performance":
		return s.buildPerformancePrompt(ctx, store, req)
	default:
		return s.BuildPrompt(ctx, store, req.Query, req.SymbolID)
	}
}

func (s *AIService) buildInsightPrompt(ctx context.Context, store *meb.MEBStore, req AIRequest) (string, error) {
	return s.BuildPrompt(ctx, store, fmt.Sprintf("Analyze the architectural role of component %s. Provide a comprehensive analysis including role, interactions, and design patterns.", req.SymbolID), req.SymbolID)
}

func (s *AIService) buildChatPrompt(req AIRequest) (string, error) {
	context := formatNodesWithCode(req.Data, 20)
	if s.ChatPrompt != nil {
		return s.ChatPrompt.Execute(map[string]interface{}{
			"Query":   req.Query,
			"Context": context,
		})
	}
	return fmt.Sprintf("%s\n\n%s", req.Query, context), nil
}

func (s *AIService) buildPrunePrompt(req AIRequest) (string, error) {
	nodes := formatNodeList(req.Data)
	if s.PrunePrompt != nil {
		return s.PrunePrompt.Execute(map[string]interface{}{
			"Nodes": nodes,
		})
	}
	return "", fmt.Errorf("prune.prompt not loaded")
}

func (s *AIService) buildSummaryPrompt(ctx context.Context, store *meb.MEBStore, req AIRequest) (string, error) {
	nodes := formatNodesSimple(req.Data, 15)
	return s.BuildPrompt(ctx, store, fmt.Sprintf("Provide a 2-3 sentence architectural summary for file \"%s\".\nSymbols:\n%s", req.Query, nodes), "")
}

func (s *AIService) buildNarrativePrompt(ctx context.Context, store *meb.MEBStore, req AIRequest) (string, error) {
	names := extractNodeNames(req.Data)
	return s.BuildPrompt(ctx, store, fmt.Sprintf("Explain the high-level logic flow for these components: %s. Keep it concise.", names), "")
}

func (s *AIService) buildResolveSymbolPrompt(req AIRequest) (string, error) {
	candidates := extractStringList(req.Data, 30)
	if s.ResolveSymbolPrompt != nil {
		return s.ResolveSymbolPrompt.Execute(map[string]interface{}{
			"Query":      req.Query,
			"Candidates": candidates,
		})
	}
	return "", fmt.Errorf("resolve_symbol.prompt not loaded")
}

func (s *AIService) buildPathEndpointsPrompt(req AIRequest) (string, error) {
	candidates := extractStringList(req.Data, 50)
	if s.PathEndpointsPrompt != nil {
		return s.PathEndpointsPrompt.Execute(map[string]interface{}{
			"Query":      req.Query,
			"Candidates": candidates,
		})
	}
	return "", fmt.Errorf("path_endpoints.prompt not loaded")
}

func (s *AIService) buildDatalogPrompt(req AIRequest) (string, error) {
	predicatesList := formatPredicatesList(req.Data)
	if s.DatalogPrompt != nil {
		return s.DatalogPrompt.Execute(map[string]interface{}{
			"Query":      req.Query,
			"SymbolID":   req.SymbolID,
			"Predicates": predicatesList,
		})
	}
	return "", fmt.Errorf("datalog.prompt not loaded")
}

func (s *AIService) buildPathNarrativePrompt(ctx context.Context, store *meb.MEBStore, req AIRequest) (string, error) {
	pathStr := extractPathString(req.Data)
	if s.PathNarrativePrompt != nil {
		promptStr, err := s.PathNarrativePrompt.Execute(map[string]interface{}{
			"Query": req.Query,
			"Path":  pathStr,
		})
		if err == nil {
			return s.BuildPrompt(ctx, store, promptStr, "")
		}
	}
	return s.BuildPrompt(ctx, store, fmt.Sprintf("Explain flow: %s. Path: %s", req.Query, pathStr), "")
}

func (s *AIService) buildSmartSearchPrompt(req AIRequest) (string, error) {
	nodes := formatGraphResults(req.Data, "nodes")
	links := formatGraphResults(req.Data, "links")

	if s.SmartSearchPrompt != nil {
		return s.SmartSearchPrompt.Execute(map[string]interface{}{
			"Nodes": nodes,
			"Links": links,
			"Query": req.Query,
		})
	}
	return "", fmt.Errorf("smart_search.prompt not loaded")
}

func (s *AIService) buildMultiFileSummaryPrompt(ctx context.Context, store *meb.MEBStore, req AIRequest) (string, error) {
	fileIDs := make([]string, 0)
	if list, ok := req.Data.([]interface{}); ok {
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
	sem := make(chan struct{}, 10) // limit concurrent goroutines

	for _, fileID := range fileIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			var localSb strings.Builder
			localCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			if err := s.appendSymbolContext(localCtx, store, id, &localSb); err == nil {
				mu.Lock()
				contextBuilder.WriteString(localSb.String())
				mu.Unlock()
			}
		}(fileID)
	}
	wg.Wait()

	if s.MultiFilePrompt != nil {
		return s.MultiFilePrompt.Execute(map[string]interface{}{
			"Context": contextBuilder.String(),
			"Query":   req.Query,
		})
	}
	return "", fmt.Errorf("multi_file.prompt not loaded")
}

func (s *AIService) buildRefactorPrompt(ctx context.Context, store *meb.MEBStore, req AIRequest) (string, error) {
	var sb strings.Builder
	sb.WriteString("You are an expert Software Architect. Analyze the following code and suggest refactoring improvements.\n\n")
	sb.WriteString(fmt.Sprintf("## Query: %s\n\n", req.Query))
	if err := s.appendSymbolContext(ctx, store, req.SymbolID, &sb); err != nil {
		sb.WriteString("No code context available.\n")
	}
	sb.WriteString("\nProvide specific, actionable refactoring suggestions with code examples.")
	return sb.String(), nil
}

func (s *AIService) buildTestGenerationPrompt(ctx context.Context, store *meb.MEBStore, req AIRequest) (string, error) {
	var sb strings.Builder
	sb.WriteString("You are an expert Software Engineer. Generate comprehensive unit tests for the following code.\n\n")
	sb.WriteString(fmt.Sprintf("## Query: %s\n\n", req.Query))
	if err := s.appendSymbolContext(ctx, store, req.SymbolID, &sb); err != nil {
		sb.WriteString("No code context available.\n")
	}
	sb.WriteString("\nGenerate tests covering: normal cases, edge cases, and error conditions.")
	return sb.String(), nil
}

func (s *AIService) buildSecurityAuditPrompt(ctx context.Context, store *meb.MEBStore, req AIRequest) (string, error) {
	var sb strings.Builder
	sb.WriteString("You are a Security Expert. Perform a security audit on the following code.\n\n")
	sb.WriteString(fmt.Sprintf("## Query: %s\n\n", req.Query))
	if err := s.appendSymbolContext(ctx, store, req.SymbolID, &sb); err != nil {
		sb.WriteString("No code context available.\n")
	}
	sb.WriteString("\nIdentify potential vulnerabilities including: injection, authentication, authorization, data exposure, and configuration issues.")
	return sb.String(), nil
}

func (s *AIService) buildPerformancePrompt(ctx context.Context, store *meb.MEBStore, req AIRequest) (string, error) {
	var sb strings.Builder
	sb.WriteString("You are a Performance Engineer. Analyze the following code for performance issues.\n\n")
	sb.WriteString(fmt.Sprintf("## Query: %s\n\n", req.Query))
	if err := s.appendSymbolContext(ctx, store, req.SymbolID, &sb); err != nil {
		sb.WriteString("No code context available.\n")
	}
	sb.WriteString("\nIdentify bottlenecks, unnecessary allocations, inefficient algorithms, and suggest optimizations.")
	return sb.String(), nil
}

func formatNodesWithCode(data interface{}, limit int) string {
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

var symbolRegex = regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9_.\/]{3,}\b`)

func extractPotentialSymbols(query string) []string {
	return symbolRegex.FindAllString(query, -1)
}

func (s *AIService) BuildPrompt(ctx context.Context, store *meb.MEBStore, query string, symbolID string) (string, error) {
	startTime := time.Now()
	defer func() {
		log.Printf("BuildPrompt took %v\n", time.Since(startTime))
	}()

	var contextBuilder strings.Builder
	contextBuilder.WriteString("## Context\n")

	if symbolID != "" {
		if err := s.appendSymbolContext(ctx, store, symbolID, &contextBuilder); err != nil {
			log.Printf("Failed to fetch symbol context for %s: %v", symbolID, err)
		}
	} else {
		if err := s.buildSemanticContext(ctx, store, query, &contextBuilder); err != nil {
			log.Printf("Failed to build semantic context: %v", err)
		}
	}

	return s.formatPromptOutput(contextBuilder.String(), query)
}

func (s *AIService) buildSemanticContext(ctx context.Context, store *meb.MEBStore, query string, contextBuilder *strings.Builder) error {
	words := extractPotentialSymbols(query)
	if len(words) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var matchedIDs []string

	for _, word := range words {
		if len(matchedIDs) >= 3 {
			break
		}
		if seen[word] {
			continue
		}
		seen[word] = true

		_, exists := store.LookupID(word)
		if exists {
			matchedIDs = append(matchedIDs, word)
		}
	}

	if len(matchedIDs) == 0 {
		return nil
	}

	return s.fetchMatchedSymbolContexts(ctx, store, matchedIDs, contextBuilder)
}

func (s *AIService) fetchMatchedSymbolContexts(ctx context.Context, store *meb.MEBStore, matchedIDs []string, contextBuilder *strings.Builder) error {
	results := make([]string, len(matchedIDs))
	var wg sync.WaitGroup

	for i, id := range matchedIDs {
		wg.Add(1)
		go func(idx int, symID string) {
			defer wg.Done()
			var localSb strings.Builder
			localCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			if err := s.appendSymbolContext(localCtx, store, symID, &localSb); err == nil {
				results[idx] = localSb.String()
			}
		}(i, id)
	}
	wg.Wait()

	for _, result := range results {
		contextBuilder.WriteString(result)
	}
	return nil
}

func (s *AIService) formatPromptOutput(context string, query string) (string, error) {
	if s.DefaultContextPrompt != nil {
		return s.DefaultContextPrompt.Execute(map[string]interface{}{
			"Context": context,
			"Query":   query,
		})
	}

	prompt := fmt.Sprintf(`You are an expert Software Architect assistant.
Assign context to the user's question using the provided Code and Graph information.

%s

## User Question
%s

Answer concisely and accurately based on the code provided.`, context, query)

	return prompt, nil
}

func (s *AIService) appendSymbolContext(ctx context.Context, store *meb.MEBStore, symbolID string, sb *strings.Builder) error {
	content, err := s.getSymbolContent(store, symbolID)
	if err != nil {
		return err
	}

	inbound, outbound, defines, err := s.querySymbolRelationships(ctx, store, symbolID)
	if err != nil {
		log.Printf("failed to query symbol relationships for %s: %v", symbolID, err)
	}

	s.formatSymbolContext(symbolID, content, inbound, outbound, defines, sb)
	return nil
}

func (s *AIService) getSymbolContent(store *meb.MEBStore, symbolID string) (string, error) {
	contentBytes, err := store.GetContentByKey(string(symbolID))
	if err != nil {
		return "", err
	}
	return string(contentBytes), nil
}

func (s *AIService) querySymbolRelationships(ctx context.Context, store *meb.MEBStore, symbolID string) (inbound, outbound, defines []map[string]any, err error) {
	inbound, _ = gcamdb.Query(ctx, store, fmt.Sprintf(`triples(?s, "%s", "%s")`, config.PredicateCalls, symbolID))
	outbound, _ = gcamdb.Query(ctx, store, fmt.Sprintf(`triples("%s", "%s", ?o)`, symbolID, config.PredicateCalls))
	defines, _ = gcamdb.Query(ctx, store, fmt.Sprintf(`triples("%s", "%s", ?o)`, symbolID, config.PredicateDefines))
	return inbound, outbound, defines, nil
}

func (s *AIService) formatSymbolContext(symbolID string, content string, inbound, outbound, defines []map[string]any, sb *strings.Builder) {
	sb.WriteString(fmt.Sprintf("\n### Symbol: %s\n", symbolID))
	sb.WriteString("```\n")
	if len(content) > 2000 {
		sb.WriteString(content[:2000] + "\n... (truncated)")
	} else {
		sb.WriteString(content)
	}
	sb.WriteString("\n```\n")

	if len(defines) > 0 {
		sb.WriteString("**Defines:**\n")
		for i, row := range defines {
			if i >= 5 {
				break
			}
			if obj, ok := row["?o"].(string); ok {
				sb.WriteString(fmt.Sprintf("- %s\n", obj))
			}
		}
	}

	if len(inbound) > 0 {
		sb.WriteString("**Called By:**\n")
		for i, row := range inbound {
			if i >= 5 {
				break
			}
			if subj, ok := row["?s"].(string); ok {
				sb.WriteString(fmt.Sprintf("- %s\n", subj))
			}
		}
	}

	if len(outbound) > 0 {
		sb.WriteString("**Calls:**\n")
		for i, row := range outbound {
			if i >= 5 {
				break
			}
			if obj, ok := row["?o"].(string); ok {
				sb.WriteString(fmt.Sprintf("- %s\n", obj))
			}
		}
	}
	sb.WriteString("\n")
}

type AIServiceModelAdapter struct {
	service *AIService
}

func NewAIServiceModelAdapter(svc *AIService) *AIServiceModelAdapter {
	return &AIServiceModelAdapter{service: svc}
}

func (m *AIServiceModelAdapter) GenerateContent(ctx context.Context, prompt string) (string, error) {
	return m.service.GenerateText(ctx, prompt)
}

type PromptLoaderAdapter struct {
	service *AIService
}

func (l *PromptLoaderAdapter) LoadPrompt(name string) (*prompts.Prompt, error) {
	p, err := prompts.LoadPrompt(name)
	if err != nil {
		return nil, err
	}
	return p, nil
}

type StoreManagerAdapter struct {
	service *AIService
}

func (m *StoreManagerAdapter) GetStore(projectID string) (*meb.MEBStore, error) {
	return m.service.manager.GetStore(projectID)
}

func (s *AIService) HandleRequestOODA(ctx context.Context, req AIRequest) (string, error) {
	storeManager := &StoreManagerAdapter{service: s}
	promptLoader := &PromptLoaderAdapter{service: s}
	model := &AIServiceModelAdapter{service: s}

	config := ooda.NewOODAConfig(storeManager, promptLoader, model)
	loop := ooda.NewOODALoopFromConfig(config)

	task := ooda.GCATask(req.Task)
	if task == "" {
		task = ooda.TaskChat
	}

	return ooda.RunOODATask(ctx, loop, req.ProjectID, req.Query, task, req.SymbolID, req.Data)
}

type AskRequest struct {
	ProjectID string `json:"project_id"`
	Query     string `json:"query"`
	SymbolID  string `json:"symbol_id,omitempty"`
	Depth     int    `json:"depth,omitempty"`
	Context   string `json:"context,omitempty"`
}

type AskResponse struct {
	Answer     string      `json:"answer"`
	Query      string      `json:"query"`
	Intent     string      `json:"intent"`
	Confidence float64     `json:"confidence"`
	Results    interface{} `json:"results"`
	Summary    string      `json:"summary"`
	Error      string      `json:"error,omitempty"`
}

func (s *AIService) HandleAsk(ctx context.Context, req AskRequest) (*AskResponse, error) {
	resp := &AskResponse{
		Query: req.Query,
	}

	if req.ProjectID == "" {
		resp.Error = "project_id is required"
		return resp, fmt.Errorf("project_id is required")
	}
	if req.Query == "" {
		resp.Error = "query is required"
		return resp, fmt.Errorf("query is required")
	}

	store, err := s.manager.GetStore(req.ProjectID)
	if err != nil {
		resp.Error = fmt.Sprintf("failed to get store: %v", err)
		return resp, fmt.Errorf("failed to get store: %w", err)
	}

	intentResult := ClassifyIntent(req.Query)
	resp.Intent = string(intentResult.Intent)
	resp.Confidence = intentResult.Confidence

	target := intentResult.Target
	if target == "" && req.SymbolID != "" {
		target = req.SymbolID
	}

	queryResult, err := GenerateDatalog(ctx, req.Query, intentResult.Intent, target, store)
	if err != nil {
		resp.Query = queryResult.Query
		resp.Error = fmt.Sprintf("query generation failed: %v", err)
		resp.Answer = "I had trouble understanding your question. Could you rephrase it?"
		return resp, nil
	}

	resp.Query = queryResult.Query

	pathTool := parsePathTool(resp.Query)
	var results interface{}
	if pathTool != nil {
		results, err = ExecutePathQuery(ctx, store, pathTool.Source, pathTool.Target)
	} else {
		results, err = ExecuteQuery(ctx, store, resp.Query)
	}

	if err != nil {
		resp.Error = fmt.Sprintf("query execution failed: %v", err)
		resp.Summary = "0 results"
		resp.Answer = "I couldn't find any matching results for your query."
		return resp, nil
	}

	resp.Results = results

	synthResult, err := SynthesizeAnswer(ctx, intentResult.Intent, req.Query, resp.Query, results, store)
	if err == nil {
		resp.Answer = synthResult.Answer
		resp.Summary = synthResult.Summary
	} else {
		resp.Answer = fmt.Sprintf("Found results but had trouble generating explanation: %v", err)
		resp.Summary = fmt.Sprintf("Found %v", results)
	}

	return resp, nil
}
