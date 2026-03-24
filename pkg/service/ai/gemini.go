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
	"github.com/duynguyendang/gca/pkg/ooda"
	"github.com/duynguyendang/gca/pkg/prompts"
	"github.com/duynguyendang/meb"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// ProjectStoreManager interface abstraction to avoid circular dependency if possible,
// or just use the one from service package if it's exported.
// Since we are in `pkg/service/ai`, we can't import `pkg/service`.
// We will define a local interface or rely on `meb.MEBStore`.
type ProjectStoreManager interface {
	GetStore(projectID string) (*meb.MEBStore, error)
}

type GeminiService struct {
	client         *genai.Client
	embeddingModel *genai.EmbeddingModel
	manager        ProjectStoreManager

	// Loaded Prompts - All AI tasks use external prompt files
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

// NewGeminiService creates a new Gemini AI service with the given API key and store manager.
// It loads all required prompt files from the prompts directory.
// Returns a configured GeminiService or an error if initialization fails.
func NewGeminiService(ctx context.Context, apiKey string, manager ProjectStoreManager) (*GeminiService, error) {
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not found")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}

	// Load all prompts from external files
	loadPrompt := func(name string) *prompts.Prompt {
		p, err := prompts.LoadPrompt("prompts/" + name)
		if err != nil {
			log.Printf("Warning: Failed to load %s: %v", name, err)
			return nil
		}
		return p
	}

	return &GeminiService{
		client:               client,
		embeddingModel:       client.EmbeddingModel("gemini-embedding-001"),
		manager:              manager,
		DatalogPrompt:        loadPrompt("datalog.prompt"),
		ChatPrompt:           loadPrompt("chat.prompt"),
		PathNarrativePrompt:  loadPrompt("path_narrative.prompt"),
		PathEndpointsPrompt:  loadPrompt("path_endpoints.prompt"),
		ResolveSymbolPrompt:  loadPrompt("resolve_symbol.prompt"),
		PrunePrompt:          loadPrompt("prune.prompt"),
		SmartSearchPrompt:    loadPrompt("smart_search.prompt"),
		MultiFilePrompt:      loadPrompt("multi_file.prompt"),
		DefaultContextPrompt: loadPrompt("default_context.prompt"),
	}, nil
}

// getModel fetches the GenerativeModel dynamically so changes to GEMINI_MODEL
// take effect without needing to restart the application.
func (s *GeminiService) getModel() *genai.GenerativeModel {
	modelName := os.Getenv("GEMINI_MODEL")
	if modelName == "" {
		modelName = "gemini-3-flash-preview"
	}
	model := s.client.GenerativeModel(modelName)
	model.SetTemperature(0.2) // Low temperature for technical accuracy
	return model
}

// GetEmbedding generates an embedding vector for the given text using Gemini's embedding model.
// The embedding can be used for semantic search and similarity comparisons.
// Returns a 768-dimensional float32 vector or an error if generation fails.
func (s *GeminiService) GetEmbedding(ctx context.Context, text string) ([]float32, error) {
	if s.embeddingModel == nil {
		return nil, fmt.Errorf("embedding model not initialized")
	}
	if text == "" {
		return nil, fmt.Errorf("empty text for embedding")
	}

	res, err := s.embeddingModel.EmbedContent(ctx, genai.Text(text))
	if err != nil {
		return nil, fmt.Errorf("embedding generation failed: %w", err)
	}

	if res.Embedding == nil || len(res.Embedding.Values) == 0 {
		return nil, fmt.Errorf("no embedding values returned")
	}

	return res.Embedding.Values, nil
}

// Ask processes a user query, optionally focusing on a specific symbol.
// AIRequest defines the structure for AI operations
type AIRequest struct {
	ProjectID string      `json:"project_id"`
	Task      string      `json:"task"` // "chat", "insight", "prune", "summary", "path", etc.
	Query     string      `json:"query"`
	SymbolID  string      `json:"symbol_id"`
	Data      interface{} `json:"data"` // Flexible data payload (node list, etc.)
}

// HandleRequest dispatchs the AI request based on the Task type
func (s *GeminiService) HandleRequest(ctx context.Context, req AIRequest) (string, error) {
	store, err := s.manager.GetStore(req.ProjectID)
	if err != nil {
		return "", fmt.Errorf("failed to get store: %w", err)
	}

	prompt, err := s.buildTaskPrompt(ctx, store, req)
	if err != nil {
		return "", fmt.Errorf("failed to build prompt: %w", err)
	}

	// Log prompt length for debugging
	log.Printf("Sending AI Prompt (Task: %s, Length: %d chars)", req.Task, len(prompt))

	// Add timeout to prevent hanging, extended for Gemini 3
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	log.Printf("Sending Prompt to Gemini (%s):\n%s", req.Task, prompt)

	resp, err := s.getModel().GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		log.Printf("Gemini Request Failed:\n%s\nError: %v", prompt, err)
		return "", err
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		log.Printf("Gemini returned empty candidates")
		return "No response from AI.", nil
	}

	var sb strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			sb.WriteString(string(txt))
		}
	}

	return sb.String(), nil
}

func (s *GeminiService) buildTaskPrompt(ctx context.Context, store *meb.MEBStore, req AIRequest) (string, error) {
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
	default:
		return s.BuildPrompt(ctx, store, req.Query, req.SymbolID)
	}
}

// buildInsightPrompt builds prompt for node insight analysis.
func (s *GeminiService) buildInsightPrompt(ctx context.Context, store *meb.MEBStore, req AIRequest) (string, error) {
	return s.BuildPrompt(ctx, store, fmt.Sprintf("Analyze the architectural role of component %s. Provide a comprehensive analysis including role, interactions, and design patterns.", req.SymbolID), req.SymbolID)
}

// buildChatPrompt builds prompt for general code analysis.
func (s *GeminiService) buildChatPrompt(req AIRequest) (string, error) {
	context := formatNodesWithCode(req.Data, 20)
	if s.ChatPrompt != nil {
		return s.ChatPrompt.Execute(map[string]interface{}{
			"Query":   req.Query,
			"Context": context,
		})
	}
	return fmt.Sprintf("%s\n\n%s", req.Query, context), nil
}

// buildPrunePrompt builds prompt for selecting top architectural nodes.
func (s *GeminiService) buildPrunePrompt(req AIRequest) (string, error) {
	nodes := formatNodeList(req.Data)
	if s.PrunePrompt != nil {
		return s.PrunePrompt.Execute(map[string]interface{}{
			"Nodes": nodes,
		})
	}
	return "", fmt.Errorf("prune.prompt not loaded")
}

// buildSummaryPrompt builds prompt for architecture summary.
func (s *GeminiService) buildSummaryPrompt(ctx context.Context, store *meb.MEBStore, req AIRequest) (string, error) {
	nodes := formatNodesSimple(req.Data, 15)
	return s.BuildPrompt(ctx, store, fmt.Sprintf("Provide a 2-3 sentence architectural summary for file \"%s\".\nSymbols:\n%s", req.Query, nodes), "")
}

// buildNarrativePrompt builds prompt for architecture flow narrative.
func (s *GeminiService) buildNarrativePrompt(ctx context.Context, store *meb.MEBStore, req AIRequest) (string, error) {
	names := extractNodeNames(req.Data)
	return s.BuildPrompt(ctx, store, fmt.Sprintf("Explain the high-level logic flow for these components: %s. Keep it concise.", names), "")
}

// buildResolveSymbolPrompt builds prompt for symbol resolution.
func (s *GeminiService) buildResolveSymbolPrompt(req AIRequest) (string, error) {
	candidates := extractStringList(req.Data, 30)
	if s.ResolveSymbolPrompt != nil {
		return s.ResolveSymbolPrompt.Execute(map[string]interface{}{
			"Query":      req.Query,
			"Candidates": candidates,
		})
	}
	return "", fmt.Errorf("resolve_symbol.prompt not loaded")
}

// buildPathEndpointsPrompt builds prompt for path endpoints.
func (s *GeminiService) buildPathEndpointsPrompt(req AIRequest) (string, error) {
	candidates := extractStringList(req.Data, 50)
	if s.PathEndpointsPrompt != nil {
		return s.PathEndpointsPrompt.Execute(map[string]interface{}{
			"Query":      req.Query,
			"Candidates": candidates,
		})
	}
	return "", fmt.Errorf("path_endpoints.prompt not loaded")
}

// buildDatalogPrompt builds prompt for datalog queries.
func (s *GeminiService) buildDatalogPrompt(req AIRequest) (string, error) {
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

// buildPathNarrativePrompt builds prompt for path narrative.
func (s *GeminiService) buildPathNarrativePrompt(ctx context.Context, store *meb.MEBStore, req AIRequest) (string, error) {
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

// buildSmartSearchPrompt builds prompt for smart search analysis.
func (s *GeminiService) buildSmartSearchPrompt(req AIRequest) (string, error) {
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

// buildMultiFileSummaryPrompt builds prompt for multi-file summary.
func (s *GeminiService) buildMultiFileSummaryPrompt(ctx context.Context, store *meb.MEBStore, req AIRequest) (string, error) {
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

	for _, fileID := range fileIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
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

// Helper: Format nodes with full code for chat context
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

// Helper: Format nodes as simple list (name + kind)
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

// Helper: Format predicates list for datalog prompt
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

// Helper: Format nodes for prune task
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

// Helper: Format graph results (nodes or links) for smart_search_analysis
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

// Helper: Extract node names as comma-separated string
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

// Helper: Extract string list with limit
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

// Helper: Extract path string from node list
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

// BuildPrompt constructs the prompt with context.
// Requirement: < 50ms
func (s *GeminiService) BuildPrompt(ctx context.Context, store *meb.MEBStore, query string, symbolID string) (string, error) {
	startTime := time.Now()
	defer func() {
		fmt.Printf("BuildPrompt took %v\n", time.Since(startTime))
	}()

	var contextBuilder strings.Builder
	contextBuilder.WriteString("## Context\n")

	// 1. Direct Symbol Context (if provided)
	if symbolID != "" {
		if err := s.appendSymbolContext(ctx, store, symbolID, &contextBuilder); err != nil {
			log.Printf("Failed to fetch symbol context for %s: %v", symbolID, err)
		}
	} else {
		// 2. Semantic Context Discovery (from query)
		if err := s.buildSemanticContext(ctx, store, query, &contextBuilder); err != nil {
			log.Printf("Failed to build semantic context: %v", err)
		}
	}

	return s.formatPromptOutput(contextBuilder.String(), query)
}

// buildSemanticContext finds potential symbols in the query and fetches their context.
func (s *GeminiService) buildSemanticContext(ctx context.Context, store *meb.MEBStore, query string, contextBuilder *strings.Builder) error {
	words := extractPotentialSymbols(query)
	if len(words) == 0 {
		return nil
	}

	// Limit to top 3 unique matches
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

		// Fast Is-It-A-Symbol Check
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

// fetchMatchedSymbolContexts fetches context for matched symbols in parallel.
func (s *GeminiService) fetchMatchedSymbolContexts(ctx context.Context, store *meb.MEBStore, matchedIDs []string, contextBuilder *strings.Builder) error {
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

// formatPromptOutput formats the final prompt with the context and query.
func (s *GeminiService) formatPromptOutput(context string, query string) (string, error) {
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

func (s *GeminiService) appendSymbolContext(ctx context.Context, store *meb.MEBStore, symbolID string, sb *strings.Builder) error {
	// 1. Fetch Symbol Content
	content, err := s.getSymbolContent(store, symbolID)
	if err != nil {
		return err
	}

	// 2. Query symbol relationships
	inbound, outbound, defines, err := s.querySymbolRelationships(ctx, store, symbolID)
	if err != nil {
		log.Printf("failed to query symbol relationships for %s: %v", symbolID, err)
	}

	// 3. Format symbol context
	s.formatSymbolContext(symbolID, content, inbound, outbound, defines, sb)
	return nil
}

// getSymbolContent retrieves the content for a given symbol ID.
func (s *GeminiService) getSymbolContent(store *meb.MEBStore, symbolID string) (string, error) {
	contentBytes, err := store.GetContentByKey(string(symbolID))
	if err != nil {
		return "", err
	}
	return string(contentBytes), nil
}

// querySymbolRelationships queries the graph for symbol relationships.
func (s *GeminiService) querySymbolRelationships(ctx context.Context, store *meb.MEBStore, symbolID string) (inbound, outbound, defines []map[string]any, err error) {
	// Inbound: Who calls me?
	inbound, _ = store.Query(ctx, fmt.Sprintf(`triples(?s, "%s", "%s")`, config.PredicateCalls, symbolID))

	// Outbound: Who do I call?
	outbound, _ = store.Query(ctx, fmt.Sprintf(`triples("%s", "%s", ?o)`, symbolID, config.PredicateCalls))

	// Defines: What do I define? (For files)
	defines, _ = store.Query(ctx, fmt.Sprintf(`triples("%s", "%s", ?o)`, symbolID, config.PredicateDefines))

	return inbound, outbound, defines, nil
}

// formatSymbolContext formats the symbol context into the provided builder.
func (s *GeminiService) formatSymbolContext(symbolID string, content string, inbound, outbound, defines []map[string]any, sb *strings.Builder) {
	sb.WriteString(fmt.Sprintf("\n### Symbol: %s\n", symbolID))
	sb.WriteString("```go\n")
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

var symbolRegex = regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9_.\/]{3,}\b`)

func extractPotentialSymbols(query string) []string {
	// Simple extraction: words >= 4 chars to avoid "are", "the", "any"
	// Prefer CamelCase or underscores/dots
	return symbolRegex.FindAllString(query, -1)
}

type GeminiModelAdapter struct {
	service *GeminiService
}

// NewGeminiModelAdapter creates an adapter wrapping the given service.
func NewGeminiModelAdapter(svc *GeminiService) *GeminiModelAdapter {
	return &GeminiModelAdapter{service: svc}
}

func (m *GeminiModelAdapter) GenerateContent(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	log.Printf("Sending Prompt to Gemini:\n%s", prompt)

	resp, err := m.service.getModel().GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		log.Printf("Gemini Request Failed:\n%s\nError: %v", prompt, err)
		return "", err
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return "No response from AI.", nil
	}

	var sb strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			sb.WriteString(string(txt))
		}
	}

	return sb.String(), nil
}

type PromptLoaderAdapter struct {
	service *GeminiService
}

func (l *PromptLoaderAdapter) LoadPrompt(name string) (*prompts.Prompt, error) {
	p, err := prompts.LoadPrompt(name)
	if err != nil {
		return nil, err
	}
	return p, nil
}

type StoreManagerAdapter struct {
	service *GeminiService
}

func (m *StoreManagerAdapter) GetStore(projectID string) (*meb.MEBStore, error) {
	return m.service.manager.GetStore(projectID)
}

func (s *GeminiService) HandleRequestOODA(ctx context.Context, req AIRequest) (string, error) {
	storeManager := &StoreManagerAdapter{service: s}
	promptLoader := &PromptLoaderAdapter{service: s}
	model := &GeminiModelAdapter{service: s}

	config := ooda.NewOODAConfig(storeManager, promptLoader, model)
	loop := ooda.NewOODALoopFromConfig(config)

	task := ooda.GCATask(req.Task)
	if task == "" {
		task = ooda.TaskChat
	}

	return ooda.RunOODATask(ctx, loop, req.ProjectID, req.Query, task, req.SymbolID, req.Data)
}
