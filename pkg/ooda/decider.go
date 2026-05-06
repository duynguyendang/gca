package ooda

import (
	"context"
	"fmt"

	"github.com/duynguyendang/gca/pkg/promptbuilder"
	"github.com/duynguyendang/gca/pkg/prompts"
	"github.com/duynguyendang/meb"
)

type PromptLoader interface {
	LoadPrompt(name string) (*prompts.Prompt, error)
}

type GraphDecider struct {
	storeManager StoreManager
	promptLoader PromptLoader
	prompts      *promptbuilder.PromptSet
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

	d.prompts = &promptbuilder.PromptSet{
		Datalog:        loadPrompt("datalog.prompt"),
		Chat:           loadPrompt("chat.prompt"),
		PathNarrative:  loadPrompt("path_narrative.prompt"),
		PathEndpoints:  loadPrompt("path_endpoints.prompt"),
		ResolveSymbol:  loadPrompt("resolve_symbol.prompt"),
		Prune:          loadPrompt("prune.prompt"),
		SmartSearch:    loadPrompt("smart_search.prompt"),
		MultiFile:      loadPrompt("multi_file.prompt"),
		DefaultContext: loadPrompt("default_context.prompt"),
		Insight:        loadPrompt("insight.prompt"),
		Summary:        loadPrompt("summary.prompt"),
		Narrative:      loadPrompt("narrative.prompt"),
	}

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
	data := map[string]interface{}{
		"Query":    frame.Input,
		"SymbolID": frame.SymbolID,
		"Data":     frame.Data,
	}
	return promptbuilder.BuildPrompt(string(frame.Task), ctx, store, d.prompts, data)
}