package ai

import (
	"context"
	"fmt"

	"github.com/duynguyendang/gca/pkg/logger"
)
type AIRequest struct {
	ProjectID        string      `json:"project_id"`
	Task             string      `json:"task"`
	Query            string      `json:"query"`
	SymbolID         string      `json:"symbol_id"`
	Data             interface{} `json:"data"`
	ContextMode      string      `json:"context_mode,omitempty"`
	QueryInstruction string      `json:"query_instruction,omitempty"`
	Messages         []ChatMessage `json:"messages,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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

	prompt = truncatePrompt(prompt)

	logger.Debug("Sending AI Prompt", "task", req.Task, "length", len(prompt))

	return s.GenerateText(ctx, prompt)
}

// HandleRequestStream builds a prompt and streams the LLM response via onChunk.
func (s *AIService) HandleRequestStream(ctx context.Context, req AIRequest, onChunk func(string) error) error {
	store, err := s.manager.GetStore(req.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get store: %w", err)
	}

	prompt, err := s.buildTaskPrompt(ctx, store, req)
	if err != nil {
		return fmt.Errorf("failed to build prompt: %w", err)
	}

	prompt = truncatePrompt(prompt)

	return s.GenerateTextStream(ctx, prompt, onChunk)
}
