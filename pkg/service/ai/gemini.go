package ai

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/gca/pkg/llmconfig"
	"github.com/duynguyendang/gca/pkg/logger"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/nlp"
	"github.com/duynguyendang/gca/pkg/ooda"
	"github.com/duynguyendang/gca/pkg/promptbuilder"
	"github.com/duynguyendang/gca/pkg/prompts"
	"github.com/duynguyendang/meb"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

type ProjectStoreManager interface {
	GetStore(projectID string) (*meb.MEBStore, error)
	GetAnalyticalStore(projectID string) (*meb.MEBStore, error)
}

type AIService struct {
	g              *genkit.Genkit
	manager        ProjectStoreManager
	prompts        *promptbuilder.PromptSet
	defaultModel   string
	embeddingModel string
	provider       string

	// Response caching for AI synthesis (LRU with list for O(1) eviction)
	responseCache        map[string]*cachedResponse
	responseCacheMu      sync.RWMutex
	responseCacheTTL     time.Duration
	responseCacheMaxSize int
	responseCacheList    *list.List
	stopCh               chan struct{}
	cleanupDone          chan struct{}

	// Circuit breaker for AI service resilience
	circuitFailures    atomic.Int32
	circuitOpen        atomic.Bool
	circuitLastFailure atomic.Int64
}

type cachedResponse struct {
	Answer  string
	Summary string
	Time    time.Time
	e       *list.Element
}

func NewAIService(ctx context.Context, manager ProjectStoreManager) (*AIService, error) {
	cfg, err := llmconfig.LoadFromEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to load LLM config: %w", err)
	}

	g := genkit.Init(ctx, genkit.WithPlugins(cfg.Plugins...), genkit.WithDefaultModel(cfg.DefaultModel))

	loadPrompt := func(name string) *prompts.Prompt {
		path, ok := config.PromptPaths[name]
		if !ok {
			logger.Warn("No prompt path configured", "name", name)
			return nil
		}
		p, err := prompts.LoadPrompt(path)
		if err != nil {
			logger.Warn("Failed to load prompt", "name", name, "path", path, "error", err)
			return nil
		}
		return p
	}

	logger.Info("AI Service initialized", "provider", cfg.Provider, "model", cfg.DefaultModel, "embedding", cfg.EmbeddingModel)

	// Initialize cache TTL from config
	cacheTTL := config.QueryCacheTTL
	stopCh := make(chan struct{})
	cleanupDone := make(chan struct{})

	return &AIService{
		g:                    g,
		manager:              manager,
		defaultModel:         cfg.DefaultModel,
		embeddingModel:       cfg.EmbeddingModel,
		provider:             cfg.Provider,
		prompts: &promptbuilder.PromptSet{
			Datalog:        loadPrompt("datalog"),
			Chat:           loadPrompt("chat"),
			PathNarrative:  loadPrompt("path_narrative"),
			PathEndpoints:  loadPrompt("path_endpoints"),
			ResolveSymbol:  loadPrompt("resolve_symbol"),
			Prune:          loadPrompt("prune"),
			SmartSearch:    loadPrompt("smart_search"),
			MultiFile:      loadPrompt("multi_file"),
			DefaultContext: loadPrompt("default_context"),
			Insight:        loadPrompt("insight"),
			Summary:        loadPrompt("summary"),
			Narrative:      loadPrompt("narrative"),
			Refactor:       loadPrompt("refactor"),
			TestGen:        loadPrompt("test_gen"),
			Security:       loadPrompt("security"),
			Performance:    loadPrompt("performance"),
		},
		responseCache:        make(map[string]*cachedResponse),
		responseCacheTTL:     cacheTTL,
		responseCacheMaxSize: 1000,
		responseCacheList:    list.New(),
		stopCh:               stopCh,
		cleanupDone:          cleanupDone,
	}, nil
}

const (
	circuitFailureThreshold = 5
	circuitOpenDuration     = 30 * time.Second
)

func (s *AIService) isCircuitOpen() bool {
	if !s.circuitOpen.Load() {
		return false
	}
	lastFailure := s.circuitLastFailure.Load()
	if time.Since(time.Unix(lastFailure, 0)) > circuitOpenDuration {
		s.circuitOpen.Store(false)
		return false
	}
	return true
}

func (s *AIService) recordFailure() {
	s.circuitFailures.Add(1)
	s.circuitLastFailure.Store(time.Now().Unix())
	if s.circuitFailures.Load() >= circuitFailureThreshold {
		s.circuitOpen.Store(true)
		logger.Warn("AI circuit breaker opened", "failures", s.circuitFailures.Load())
	}
}

func (s *AIService) recordSuccess() {
	s.circuitFailures.Store(0)
	s.circuitOpen.Store(false)
}

func (s *AIService) shouldFailFast() bool {
	return s.isCircuitOpen()
}

func (s *AIService) GenerateText(ctx context.Context, prompt string) (string, error) {
	if s.shouldFailFast() {
		return "", fmt.Errorf("AI service circuit breaker is open, failing fast to prevent cascading failures")
	}

	ctx, cancel := context.WithTimeout(ctx, config.AIRequestTimeout)
	defer cancel()

	logger.Debug("Sending Prompt to LLM", "provider", s.provider, "prompt", prompt)

	resp, err := genkit.Generate(ctx, s.g,
		ai.WithModelName(s.defaultModel),
		ai.WithPrompt(prompt),
	)
	if err != nil {
		s.recordFailure()
		logger.Error("LLM Request Failed", "prompt", prompt, "error", err)
		return "", err
	}

	s.recordSuccess()
	return resp.Text(), nil
}

// GenerateTextWithContext generates text with diagnostic context injected from the Analytical Store.
// This implements the "Virtual Attention Sink" pattern for O(1) LLM context building.
func (s *AIService) GenerateTextWithContext(ctx context.Context, projectID, prompt string) (string, error) {
	// Build diagnostic context from Analytical Store (O(1) lookup)
	diagnosticCtx, err := s.BuildDiagnosticContext(ctx, projectID)
	if err != nil {
		logger.Warn("Failed to build diagnostic context, using prompt without context", "error", err)
		return s.GenerateText(ctx, prompt)
	}

	// Inject diagnostic context into prompt
	enrichedPrompt := diagnosticCtx + "\n" + prompt

	logger.Debug("Sending Prompt with Diagnostic Context", "provider", s.provider, "context_len", len(diagnosticCtx))

	resp, err := genkit.Generate(ctx, s.g,
		ai.WithModelName(s.defaultModel),
		ai.WithPrompt(enrichedPrompt),
	)
	if err != nil {
		logger.Error("LLM Request Failed", "prompt", enrichedPrompt, "error", err)
		return "", err
	}

	return resp.Text(), nil
}

// BuildDiagnosticContext builds a diagnostic context string from the Analytical Store.
// This provides O(1) lookup of pre-computed architectural insights.
func (s *AIService) BuildDiagnosticContext(ctx context.Context, projectID string) (string, error) {
	if projectID == "" {
		return "", fmt.Errorf("projectID is required")
	}

	analyticalStore, err := s.manager.GetAnalyticalStore(projectID)
	if err != nil {
		return "", fmt.Errorf("failed to get analytical store: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("=== DIAGNOSTIC CONTEXT ===\n")
	sb.WriteString(fmt.Sprintf("Project: %s\n", projectID))

	// Query for entry points
	entryQuery := `triples(Subject, "is_entry_point", "true")`
	entryResults, err := gcamdb.Query(ctx, analyticalStore, entryQuery)
	if err == nil && len(entryResults) > 0 {
		sb.WriteString("Entry Points:\n")
		count := 0
		for _, r := range entryResults {
			if subject, ok := r["Subject"].(string); ok && subject != "" && count < 10 {
				sb.WriteString(fmt.Sprintf("- %s\n", subject))
				count++
			}
		}
		if len(entryResults) > 10 {
			sb.WriteString(fmt.Sprintf("- ... and %d more\n", len(entryResults)-10))
		}
	}

	// Query for hub files
	hubQuery := `triples(Subject, "has_hub_score", Score)`
	hubResults, err := gcamdb.Query(ctx, analyticalStore, hubQuery)
	if err == nil && len(hubResults) > 0 {
		sb.WriteString("Hub Files (high connectivity):\n")
		count := 0
		for _, r := range hubResults {
			subject, _ := r["Subject"].(string)
			scoreStr, _ := r["Score"].(string)
			if subject != "" && count < 5 {
				sb.WriteString(fmt.Sprintf("- %s (score: %s)\n", subject, scoreStr))
				count++
			}
		}
		if len(hubResults) > 5 {
			sb.WriteString(fmt.Sprintf("- ... and %d more\n", len(hubResults)-5))
		}
	}

	// Query for smells
	smellQuery := `triples(Subject, "has_smell", Object)`
	smellResults, err := gcamdb.Query(ctx, analyticalStore, smellQuery)
	if err == nil && len(smellResults) > 0 {
		sb.WriteString("Architectural Issues:\n")
		count := 0
		for _, r := range smellResults {
			subject, _ := r["Subject"].(string)
			object, _ := r["Object"].(string)
			if subject != "" && object != "" && count < 10 {
				smellType := object
				if idx := strings.Index(object, ":"); idx > 0 {
					smellType = object[:idx]
				}
				sb.WriteString(fmt.Sprintf("- %s (%s)\n", subject, smellType))
				count++
			}
		}
		if len(smellResults) > 10 {
			sb.WriteString(fmt.Sprintf("- ... and %d more issues\n", len(smellResults)-10))
		}
	}

	sb.WriteString("=============================\n")

	return sb.String(), nil
}

// cacheResponse caches an AI response for a given query (LRU)
func (s *AIService) cacheResponse(cacheKey string, answer, summary string) {
	s.responseCacheMu.Lock()
	defer s.responseCacheMu.Unlock()

	// Evict oldest if at capacity
	if s.responseCacheList.Len() >= s.responseCacheMaxSize {
		if oldest := s.responseCacheList.Back(); oldest != nil {
			delete(s.responseCache, oldest.Value.(string))
			s.responseCacheList.Remove(oldest)
		}
	}

	// Add new entry to front of list
	e := s.responseCacheList.PushFront(cacheKey)
	s.responseCache[cacheKey] = &cachedResponse{
		Answer:  answer,
		Summary: summary,
		Time:    time.Now(),
		e:       e,
	}
}

// getCachedResponse retrieves a cached response if valid (LRU promotion)
func (s *AIService) getCachedResponse(cacheKey string) (string, string, bool) {
	s.responseCacheMu.RLock()
	cached, ok := s.responseCache[cacheKey]
	s.responseCacheMu.RUnlock()

	if !ok {
		return "", "", false
	}

	if time.Since(cached.Time) >= s.responseCacheTTL {
		return "", "", false
	}

	// Promote to front (LRU)
	s.responseCacheMu.Lock()
	if cached.e != nil {
		s.responseCacheList.MoveToFront(cached.e)
	}
	s.responseCacheMu.Unlock()

	return cached.Answer, cached.Summary, true
}

// generateCacheKey creates a deterministic cache key from query + results hash
func (s *AIService) generateCacheKey(query string, intent Intent, results interface{}) string {
	data := fmt.Sprintf("%s|%s|%v", query, intent, results)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// cleanupExpiredCache removes expired cache entries and enforces max size (LRU)
func (s *AIService) cleanupExpiredCache() {
	s.responseCacheMu.Lock()
	defer s.responseCacheMu.Unlock()
	now := time.Now()

	// Remove expired entries (traverse list from back = oldest)
	for e := s.responseCacheList.Back(); e != nil; e = e.Prev() {
		key := e.Value.(string)
		cached, ok := s.responseCache[key]
		if !ok {
			continue
		}
		if now.Sub(cached.Time) >= s.responseCacheTTL {
			delete(s.responseCache, key)
			s.responseCacheList.Remove(e)
		}
	}

	// Enforce max size (remove oldest from back if still over limit)
	for s.responseCacheList.Len() > s.responseCacheMaxSize {
		if oldest := s.responseCacheList.Back(); oldest != nil {
			delete(s.responseCache, oldest.Value.(string))
			s.responseCacheList.Remove(oldest)
		} else {
			break
		}
	}
}

// Close signals the cleanup goroutines to stop and waits for them to complete.
func (s *AIService) Close() {
	close(s.stopCh)
}

// WaitForCleanup waits for any in-flight cleanup goroutines to complete.
// This is optional and mainly for testing.
func (s *AIService) WaitForCleanup() {
	<-s.cleanupDone
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

	logger.Debug("Sending AI Prompt", "task", req.Task, "length", len(prompt))

	return s.GenerateText(ctx, prompt)
}

func (s *AIService) buildTaskPrompt(ctx context.Context, store *meb.MEBStore, req AIRequest) (string, error) {
	data := map[string]interface{}{
		"Query":   req.Query,
		"SymbolID": req.SymbolID,
		"Data":    req.Data,
	}
	return promptbuilder.BuildPrompt(req.Task, ctx, store, s.prompts, data)
}

func (s *AIService) BuildPrompt(ctx context.Context, store *meb.MEBStore, query string, symbolID string) (string, error) {
	startTime := time.Now()
	defer func() {
		logger.Debug("BuildPrompt took", "duration", time.Since(startTime))
	}()

	var contextBuilder strings.Builder
	contextBuilder.WriteString("## Context\n")

	if symbolID != "" {
		if err := s.appendSymbolContext(ctx, store, symbolID, &contextBuilder); err != nil {
			logger.Warn("Failed to fetch symbol context", "symbolID", symbolID, "error", err)
		}
	} else {
		if err := s.buildSemanticContext(ctx, store, query, &contextBuilder); err != nil {
			logger.Warn("Failed to build semantic context", "error", err)
		}
	}

	return s.formatPromptOutput(contextBuilder.String(), query)
}

func (s *AIService) buildSemanticContext(ctx context.Context, store *meb.MEBStore, query string, contextBuilder *strings.Builder) error {
	words := ooda.ExtractPotentialSymbols(query)
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
	if s.prompts.DefaultContext != nil {
		return s.prompts.DefaultContext.Execute(map[string]interface{}{
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
		return fmt.Errorf("failed to get symbol content for %s: %w", symbolID, err)
	}

	inbound, outbound, defines, err := s.querySymbolRelationships(ctx, store, symbolID)
	if err != nil {
		// Log but continue with empty relationships - partial context is better than no context
		logger.Warn("Failed to query symbol relationships", "symbolID", symbolID, "error", err)
		// Initialize empty slices to avoid nil panics
		inbound = nil
		outbound = nil
		defines = nil
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
	var err1, err2, err3 error

	inbound, err1 = gcamdb.Query(ctx, store, fmt.Sprintf(`triples(?s, "%s", "%s")`, config.PredicateCalls, symbolID))
	outbound, err2 = gcamdb.Query(ctx, store, fmt.Sprintf(`triples("%s", "%s", ?o)`, symbolID, config.PredicateCalls))
	defines, err3 = gcamdb.Query(ctx, store, fmt.Sprintf(`triples("%s", "%s", ?o)`, symbolID, config.PredicateDefines))

	// Return the first error encountered, if any
	if err1 != nil {
		return nil, nil, nil, fmt.Errorf("failed to query inbound relationships: %w", err1)
	}
	if err2 != nil {
		return nil, nil, nil, fmt.Errorf("failed to query outbound relationships: %w", err2)
	}
	if err3 != nil {
		return nil, nil, nil, fmt.Errorf("failed to query defines: %w", err3)
	}

	return inbound, outbound, defines, nil
}

func (s *AIService) formatSymbolContext(symbolID string, content string, inbound, outbound, defines []map[string]any, sb *strings.Builder) {
	sb.WriteString(fmt.Sprintf("\n### Symbol: %s\n", symbolID))
	sb.WriteString("```\n")
	sb.WriteString(common.SymbolContext(content))
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

func (m *StoreManagerAdapter) GetAnalyticalStore(projectID string) (*meb.MEBStore, error) {
	return m.service.manager.GetAnalyticalStore(projectID)
}

// analyticalTemplateStore reads query templates from the Analytical Store via Datalog.
// It implements ingest.TemplateStoreInterface for the Neuro-Symbolic Executor.
// Templates are cached for 10 minutes to avoid redundant Datalog queries.
type analyticalTemplateStore struct {
	manager  ProjectStoreManager
	mu       sync.RWMutex
	cache    map[string]*templateCacheEntry
	lastLoad time.Time
}

type templateCacheEntry struct {
	templates []*ingest.TemplateStoreQuery
	loadedAt  time.Time
}

const templateCacheTTL = 10 * time.Minute

func (a *analyticalTemplateStore) GetTemplate(ctx context.Context, projectID, templateID string) (*ingest.TemplateStoreQuery, error) {
	store, err := a.manager.GetAnalyticalStore(projectID)
	if err != nil {
		return nil, fmt.Errorf("get analytical store: %w", err)
	}

	bodyQuery := fmt.Sprintf(`triples("%s", "query_template", Body)`, templateID)
	results, err := gcamdb.Query(ctx, store, bodyQuery)
	if err != nil || len(results) == 0 {
		return nil, fmt.Errorf("template not found: %s", templateID)
	}

	tmpl := &ingest.TemplateStoreQuery{ID: templateID}
	if b, ok := results[0]["Body"].(string); ok {
		tmpl.Body = b
	}

	// Fetch metadata
	catQuery := fmt.Sprintf(`triples("%s", "category", Cat)`, templateID)
	if catResults, err := gcamdb.Query(ctx, store, catQuery); err == nil && len(catResults) > 0 {
		if c, ok := catResults[0]["Cat"].(string); ok {
			tmpl.Category = c
		}
	}
	sevQuery := fmt.Sprintf(`triples("%s", "severity", Sev)`, templateID)
	if sevResults, err := gcamdb.Query(ctx, store, sevQuery); err == nil && len(sevResults) > 0 {
		if s, ok := sevResults[0]["Sev"].(string); ok {
			tmpl.Severity = s
		}
	}
	descQuery := fmt.Sprintf(`triples("%s", "description", Desc)`, templateID)
	if descResults, err := gcamdb.Query(ctx, store, descQuery); err == nil && len(descResults) > 0 {
		if d, ok := descResults[0]["Desc"].(string); ok {
			tmpl.Description = d
		}
	}
	predQuery := fmt.Sprintf(`triples("%s", "Predicate", Pred)`, templateID)
	if predResults, err := gcamdb.Query(ctx, store, predQuery); err == nil && len(predResults) > 0 {
		if p, ok := predResults[0]["Pred"].(string); ok {
			tmpl.Predicate = p
		}
	}
	smellQuery := fmt.Sprintf(`triples("%s", "smell_type", Smell)`, templateID)
	if smellResults, err := gcamdb.Query(ctx, store, smellQuery); err == nil && len(smellResults) > 0 {
		if s, ok := smellResults[0]["Smell"].(string); ok {
			tmpl.SmellType = s
		}
	}

	// Fetch parameters
	paramQuery := fmt.Sprintf(`triples("%s", "param", Param)`, templateID)
	if paramResults, err := gcamdb.Query(ctx, store, paramQuery); err == nil && len(paramResults) > 0 {
		for _, pr := range paramResults {
			if paramStr, ok := pr["Param"].(string); ok && paramStr != "" {
				parts := strings.SplitN(paramStr, "|", 2)
				if len(parts) == 2 {
					tmpl.Parameters = append(tmpl.Parameters, ingest.TemplateParam{
						Name: parts[0],
						Type: parts[1],
					})
				}
			}
		}
	}

	return tmpl, nil
}

func (a *analyticalTemplateStore) ListTemplates(ctx context.Context, projectID, category string) ([]*ingest.TemplateStoreQuery, error) {
	cacheKey := category
	a.mu.RLock()
	if entry, ok := a.cache[cacheKey]; ok && time.Since(entry.loadedAt) < templateCacheTTL {
		templates := entry.templates
		a.mu.RUnlock()
		return templates, nil
	}
	a.mu.RUnlock()

	store, err := a.manager.GetAnalyticalStore(projectID)
	if err != nil {
		return nil, fmt.Errorf("get analytical store: %w", err)
	}

	var query string
	if category != "" {
		query = fmt.Sprintf(`triples(ID, "category", "%s"), triples(ID, "query_template", Body)`, category)
	} else {
		query = `triples(ID, "query_template", Body)`
	}

	results, err := gcamdb.Query(ctx, store, query)
	if err != nil {
		return nil, err
	}

	var templates []*ingest.TemplateStoreQuery
	seen := make(map[string]int)

	for _, r := range results {
		id, _ := r["ID"].(string)
		if id == "" {
			continue
		}
		if existingIdx, ok := seen[id]; ok {
			if b, ok := r["Body"].(string); ok && b != "" && templates[existingIdx].Body == "" {
				templates[existingIdx].Body = b
			}
			continue
		}
		seen[id] = len(templates)

		tmpl := &ingest.TemplateStoreQuery{ID: id}
		if b, ok := r["Body"].(string); ok {
			tmpl.Body = b
		}

		// Fetch metadata per template
		catQ := fmt.Sprintf(`triples("%s", "category", Cat)`, id)
		if catR, err := gcamdb.Query(ctx, store, catQ); err == nil && len(catR) > 0 {
			if c, ok := catR[0]["Cat"].(string); ok {
				tmpl.Category = c
			}
		}
		sevQ := fmt.Sprintf(`triples("%s", "severity", Sev)`, id)
		if sevR, err := gcamdb.Query(ctx, store, sevQ); err == nil && len(sevR) > 0 {
			if s, ok := sevR[0]["Sev"].(string); ok {
				tmpl.Severity = s
			}
		}
		descQ := fmt.Sprintf(`triples("%s", "description", Desc)`, id)
		if descR, err := gcamdb.Query(ctx, store, descQ); err == nil && len(descR) > 0 {
			if d, ok := descR[0]["Desc"].(string); ok {
				tmpl.Description = d
			}
		}
		predQ := fmt.Sprintf(`triples("%s", "Predicate", Pred)`, id)
		if predR, err := gcamdb.Query(ctx, store, predQ); err == nil && len(predR) > 0 {
			if p, ok := predR[0]["Pred"].(string); ok {
				tmpl.Predicate = p
			}
		}
		smellQ := fmt.Sprintf(`triples("%s", "smell_type", Smell)`, id)
		if smellR, err := gcamdb.Query(ctx, store, smellQ); err == nil && len(smellR) > 0 {
			if s, ok := smellR[0]["Smell"].(string); ok {
				tmpl.SmellType = s
			}
		}

		// Fetch parameters
		paramQ := fmt.Sprintf(`triples("%s", "param", Param)`, id)
		if paramR, err := gcamdb.Query(ctx, store, paramQ); err == nil && len(paramR) > 0 {
			for _, pr := range paramR {
				if paramStr, ok := pr["Param"].(string); ok && paramStr != "" {
					parts := strings.SplitN(paramStr, "|", 2)
					if len(parts) == 2 {
						tmpl.Parameters = append(tmpl.Parameters, ingest.TemplateParam{
							Name: parts[0],
							Type: parts[1],
						})
					}
				}
			}
		}

		templates = append(templates, tmpl)
	}

	a.mu.Lock()
	a.cache[cacheKey] = &templateCacheEntry{templates: templates, loadedAt: time.Now()}
	a.mu.Unlock()

	return templates, nil
}

func (s *AIService) HandleRequestOODA(ctx context.Context, req AIRequest) (string, error) {
	task := ooda.GCATask(req.Task)
	if task == "" {
		task = ooda.TaskChat
	}

	if task == ooda.TaskTestGeneration {
		store, err := s.manager.GetStore(req.ProjectID)
		if err != nil {
			return "", fmt.Errorf("failed to get store: %w", err)
		}

		depth := 3
		if req.Data != nil {
			if m, ok := req.Data.(map[string]any); ok {
				if d, ok := m["depth"].(int); ok && d > 0 {
					depth = d
				}
			}
		}

		model := &AIServiceModelAdapter{service: s}
		config := &ooda.MultiRoundConfig{
			Store: store,
			Model: model,
			Depth: depth,
		}

		return ooda.RunMultiRoundTestGen(ctx, config, req.SymbolID)
	}

	storeManager := &StoreManagerAdapter{service: s}
	promptLoader := &PromptLoaderAdapter{service: s}
	model := &AIServiceModelAdapter{service: s}

	config := ooda.NewOODAConfig(storeManager, promptLoader, model)
	config.TemplateStore = &analyticalTemplateStore{manager: s.manager, cache: make(map[string]*templateCacheEntry)}
	loop := ooda.NewOODALoopFromConfig(config)

	return ooda.RunOODATask(ctx, loop, req.ProjectID, req.Query, task, req.SymbolID, req.Data)
}

type AskRequest struct {
	ProjectID           string             `json:"project_id"`
	Query               string             `json:"query"`
	SymbolID            string             `json:"symbol_id,omitempty"`
	Depth               int                `json:"depth,omitempty"`
	Context             string             `json:"context,omitempty"`
	ConversationHistory []ConversationTurn `json:"conversation_history,omitempty"`
}

type ConversationTurn struct {
	UserInput    string `json:"user_input"`
	Intent       string `json:"intent"`
	DatalogQuery string `json:"datalog_query"`
	ResultCount  int    `json:"result_count"`
	Summary      string `json:"summary"`
	Timestamp    int64  `json:"timestamp"`
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

	// Full NLP pipeline: pronoun expansion → entity resolution → intent classification
	nlpHistory := convertToNLPHistory(req.ConversationHistory)
	nlpSvc := nlp.NewNLPService(newNLPEntityStore(store), nil)
	nlpResult := nlpSvc.ProcessQuery(req.Query, nlpHistory)

	resp.Intent = string(nlpResult.Intent)
	resp.Confidence = nlpResult.Confidence

	target := ""
	if len(nlpResult.Entities) > 0 {
		target = nlpResult.Entities[0].Name
	}
	if target == "" && req.SymbolID != "" {
		target = req.SymbolID
	}

	// Build conversation context for query generation
	conversationContext := buildConversationContext(req.ConversationHistory)
	queryResult, err := GenerateDatalogWithContext(ctx, nlpResult.ExpandedQuery, Intent(nlpResult.Intent), target, store, conversationContext)
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

	// Check cache before AI synthesis
	cacheKey := s.generateCacheKey(req.Query, Intent(nlpResult.Intent), results)
	if cachedAnswer, cachedSummary, found := s.getCachedResponse(cacheKey); found {
		logger.Debug("AI response cache hit", "query", req.Query)
		resp.Answer = cachedAnswer
		resp.Summary = cachedSummary
		return resp, nil
	}

	// For intents that benefit from architectural context, use LLM with diagnostic context
	// This implements the "Virtual Attention Sink" pattern
	useContextIntents := map[Intent]bool{
		IntentChat:        true,
		IntentFind:        true,
		IntentSummarize:   true,
		IntentExplain:     true,
		IntentRefactor:    true,
		IntentSecurity:    true,
		IntentPerformance: true,
	}

	var synthResult *SynthesisResult
	if useContextIntents[Intent(nlpResult.Intent)] {
		// Use LLM with diagnostic context injection
		llmPrompt := fmt.Sprintf("Based on the following query results for project %s:\n\nQuery: %s\nResults: %v\n\nUser Question: %s\n\nPlease provide a concise summary of the architectural health and any problematic files.",
			req.ProjectID, resp.Query, formatResultsForLLM(results), nlpResult.ExpandedQuery)
		answer, err := s.GenerateTextWithContext(ctx, req.ProjectID, llmPrompt)
		if err == nil {
			synthResult = &SynthesisResult{
				Answer:  answer,
				Summary: fmt.Sprintf("Found %v", results),
			}
		}
	}

	// Fallback to heuristic synthesis if LLM with context failed or not applicable
	if synthResult == nil {
		synthResult, _ = SynthesizeAnswer(ctx, Intent(nlpResult.Intent), nlpResult.ExpandedQuery, resp.Query, results, store)
	}

	if synthResult != nil {
		resp.Answer = synthResult.Answer
		resp.Summary = synthResult.Summary
		// Cache the successful response
		s.cacheResponse(cacheKey, synthResult.Answer, synthResult.Summary)
	} else {
		resp.Answer = fmt.Sprintf("Found results but had trouble generating explanation: %v", err)
		resp.Summary = fmt.Sprintf("Found %v", results)
	}

	// Periodic cleanup of expired cache entries (every 100 requests)
	if len(s.responseCache) > common.MaxExecutorCacheCleanup {
		go func() {
			select {
			case <-s.stopCh:
				return
			default:
				s.cleanupExpiredCache()
			}
		}()
	}

	return resp, nil
}

func convertToNLPHistory(history []ConversationTurn) []*nlp.ConversationTurn {
	result := make([]*nlp.ConversationTurn, len(history))
	for i := range history {
		result[i] = &nlp.ConversationTurn{
			UserInput:    history[i].UserInput,
			Intent:       history[i].Intent,
			DatalogQuery: history[i].DatalogQuery,
			ResultCount:  history[i].ResultCount,
			Summary:      history[i].Summary,
			Timestamp:    history[i].Timestamp,
		}
	}
	return result
}
