package ai

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/logger"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/nlp"
	"github.com/duynguyendang/gca/pkg/ooda"
	"github.com/duynguyendang/meb"
)
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

	response, err := ooda.RunOODATask(ctx, loop, req.ProjectID, req.Query, task, req.SymbolID, req.Data)
	if err != nil {
		return "", err
	}

	if task == ooda.TaskDatalog {
		store, storeErr := s.manager.GetStore(req.ProjectID)
		if storeErr != nil {
			logger.Debug("Datalog execution skipped: store unavailable", "project", req.ProjectID, "err", storeErr)
		} else if store != nil {
			execResp := executeAndFormatDatalogResponse(ctx, response, store)
			if execResp != "" {
				return execResp, nil
			}
		}
	}

	return response, nil
}

func executeAndFormatDatalogResponse(ctx context.Context, response string, store *meb.MEBStore) string {
	response = strings.TrimSpace(response)
	if !strings.HasPrefix(response, "triples(") && !strings.Contains(response, ":-") {
		return ""
	}

	results, err := gcamdb.Query(ctx, store, response)
	if err != nil {
		return fmt.Sprintf("Query generated but execution failed: %v\n\nGenerated query: %s", err, response)
	}

	if len(results) == 0 {
		return "No results found for this query.\n\nGenerated query: " + response
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d result(s):\n\n", len(results)))

	count := 0
	keys := make([]string, 0, len(results))
	for k := range results[0] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, r := range results {
		if count >= 50 {
			sb.WriteString(fmt.Sprintf("... and %d more\n", len(results)-count))
			break
		}
		var parts []string
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%v", k, r[k]))
		}
		if len(parts) > 0 {
			sb.WriteString(fmt.Sprintf("  - %s\n", strings.Join(parts, ", ")))
		}
		count++
	}

	sb.WriteString(fmt.Sprintf("\nQuery: %s", response))
	return sb.String()
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
