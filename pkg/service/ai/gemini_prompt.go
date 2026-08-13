package ai

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/logger"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/ooda"
	"github.com/duynguyendang/gca/pkg/promptbuilder"
	"github.com/duynguyendang/meb"
)
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
	entryQuery := common.GetNamedQuery("entry_point")
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
	hubQuery := common.GetNamedQuery("hub_score")
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
	smellQuery := common.GetNamedQuery("smell_type")
	smellResults, err := gcamdb.Query(ctx, analyticalStore, smellQuery)
	if err == nil && len(smellResults) > 0 {
		sb.WriteString("Architectural Issues:\n")
		count := 0
		for _, r := range smellResults {
			subject, _ := r["Subject"].(string)
			smellType, _ := r["Type"].(string)
			if subject != "" && smellType != "" && count < 10 {
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

func (s *AIService) buildTaskPrompt(ctx context.Context, store *meb.MEBStore, req AIRequest) (string, error) {
	messages := make([]interface{}, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = map[string]interface{}{
			"role":    m.Role,
			"content": m.Content,
		}
	}
	data := map[string]interface{}{
		"Query":   req.Query,
		"SymbolID": req.SymbolID,
		"Data":    req.Data,
		"Messages": messages,
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
	sem := make(chan struct{}, 16)

	for i, id := range matchedIDs {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, symID string) {
			defer wg.Done()
			defer func() { <-sem }()
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
	return "", fmt.Errorf("default_context prompt not loaded")
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
