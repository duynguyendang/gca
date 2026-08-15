package ai

import (
	"container/list"
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/gca/pkg/llmconfig"
	"github.com/duynguyendang/gca/pkg/logger"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
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
	stopCh      chan struct{}
	cleanupDone chan struct{}
	stopOnce    sync.Once

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
	MaxPromptLen            = 128000
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

func truncatePrompt(prompt string) string {
	if utf8.RuneCountInString(prompt) > MaxPromptLen {
		logger.Warn("Truncating oversized prompt", "original_len", len(prompt), "cap", MaxPromptLen)
		runes := []rune(prompt)
		return string(runes[:MaxPromptLen]) + "\n\n[Context truncated to fit model limits]"
	}
	return prompt
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

// GenerateTextStream generates text via streaming, calling onChunk with each token delta.
func (s *AIService) GenerateTextStream(ctx context.Context, prompt string, onChunk func(string) error) error {
	if s.shouldFailFast() {
		return fmt.Errorf("AI service circuit breaker is open, failing fast to prevent cascading failures")
	}

	ctx, cancel := context.WithTimeout(ctx, config.AIRequestTimeout)
	defer cancel()

	logger.Debug("Sending Streaming Prompt to LLM", "provider", s.provider, "prompt", prompt)

	stream := genkit.GenerateStream(ctx, s.g,
		ai.WithModelName(s.defaultModel),
		ai.WithPrompt(prompt),
	)
	for result, err := range stream {
		if err != nil {
			s.recordFailure()
			logger.Error("LLM Stream Failed", "error", err)
			return err
		}
		if result.Done {
			s.recordSuccess()
			return nil
		}
		if chunk := result.Chunk; chunk != nil {
			if err := onChunk(chunk.Text()); err != nil {
				logger.Debug("Stream onChunk returned error (client disconnect)", "error", err)
				s.recordSuccess()
				return err
			}
		}
	}
	s.recordSuccess()
	return nil
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



// Close signals the cleanup goroutines to stop and waits for them to complete.
func (s *AIService) Close() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		close(s.cleanupDone)
	})
}

// WaitForCleanup waits for any in-flight cleanup goroutines to complete.
// This is optional and mainly for testing.
func (s *AIService) WaitForCleanup() {
	select {
	case <-s.cleanupDone:
	case <-time.After(5 * time.Second):
	}
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
	seen := make(map[string]bool)

	for _, r := range results {
		id, _ := r["ID"].(string)
		if id == "" {
			continue
		}
		body, _ := r["Body"].(string)
		// Distinct rules may share an ID; key by id+body so each survives.
		key := id + "\x00" + body
		if seen[key] {
			continue
		}
		seen[key] = true

		tmpl := &ingest.TemplateStoreQuery{ID: id, Body: body}

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

