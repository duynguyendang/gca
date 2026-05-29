package ooda

import (
	"context"
	"fmt"
	"strings"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/gca/pkg/promptbuilder"
	"github.com/duynguyendang/gca/pkg/prompts"
	"github.com/duynguyendang/meb"
)

type PromptLoader interface {
	LoadPrompt(name string) (*prompts.Prompt, error)
}

type GraphDecider struct {
	storeManager   StoreManager
	promptLoader   PromptLoader
	prompts        *promptbuilder.PromptSet
	templateLister ingest.TemplateStoreInterface
	llmClient      common.LLMClient
	synthesizePrompt *prompts.Prompt
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
		TestGen:        loadPrompt("test_gen.prompt"),
	}

	d.synthesizePrompt = loadPrompt("neuro_symbolic_synthesize.prompt")

	return d
}

func (d *GraphDecider) SetTemplateStore(ts ingest.TemplateStoreInterface) {
	d.templateLister = ts
}

func (d *GraphDecider) SetLLMClient(client common.LLMClient) {
	d.llmClient = client
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
	if frame.Task == TaskTestGeneration {
		tgc, err := buildTestGenContext(ctx, store, frame)
		if err != nil {
			return "", fmt.Errorf("build test gen context: %w", err)
		}
		return buildTestGenerationPrompt(d.prompts.TestGen, tgc)
	}

	if d.templateLister != nil && d.llmClient != nil {
		nsResult, attentionSink, err := d.buildNeuroSymbolicPrompt(ctx, store, frame)
		if err == nil && nsResult != nil {
			if nsResult.SkipSynthesis {
				frame.Context = append(frame.Context, Atom{
					Predicate: "neuro_symbolic_skipped",
					Subject:   frame.ID.String(),
					Object:    nsResult.Reasoning,
					Weight:    0.5,
				})
				return d.buildFallbackPrompt(ctx, store, frame)
			}

			synthesisCtx := BuildSynthesisContext(frame.Input, nsResult, attentionSink)

			var synthesisPrompt string
			if d.synthesizePrompt != nil {
				data := map[string]interface{}{
					"DiagnosticContext": attentionSink,
					"QueryResults":      formatResults(nsResult),
					"UserQuery":         frame.Input,
				}
				synthesisPrompt, _ = d.synthesizePrompt.Execute(data)
			} else {
				synthesisPrompt = synthesisCtx + "\n## User Question\n" + frame.Input + "\n\nProvide a clear, grounded answer based ONLY on the query results above."
			}

			frame.Context = append(frame.Context, Atom{
				Predicate: "neuro_symbolic_used",
				Subject:   frame.ID.String(),
				Object:    nsResult.TemplateID,
				Weight:    1.0,
			})
			frame.Context = append(frame.Context, Atom{
				Predicate: "template_executed",
				Subject:   frame.ID.String(),
				Object:    fmt.Sprintf("results:%d", len(nsResult.Results)),
				Weight:    0.9,
			})

			return synthesisPrompt, nil
		}
	}

	return d.buildFallbackPrompt(ctx, store, frame)
}

func formatResults(nsResult *NeuroSymbolicResult) string {
	if len(nsResult.Results) == 0 {
		return "No results found."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d result(s):\n", len(nsResult.Results)))
	for i, r := range nsResult.Results {
		if i >= 20 {
			sb.WriteString(fmt.Sprintf("  ... and %d more\n", len(nsResult.Results)-20))
			break
		}
		parts := []string{}
		for k, v := range r {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
		if len(parts) > 0 {
			sb.WriteString(fmt.Sprintf("  - %s\n", strings.Join(parts, ", ")))
		}
	}
	return sb.String()
}

func (d *GraphDecider) buildFallbackPrompt(ctx context.Context, store *meb.MEBStore, frame *GCAFrame) (string, error) {
	data := map[string]interface{}{
		"Query":    frame.Input,
		"SymbolID": frame.SymbolID,
		"Data":     frame.Data,
	}

	if frame.Task == TaskDatalog {
		schema, err := BuildDatalogSchemaContext(ctx, store)
		if err != nil {
			return "", fmt.Errorf("build datalog schema: %w", err)
		}
		data["Predicates"] = schema.Predicates
		data["FactExamples"] = schema.FactExamples
		data["ConstraintDoc"] = schema.ConstraintDoc
		return promptbuilder.BuildDatalogPromptWithSchema(ctx, store, d.prompts, data)
	}

	return promptbuilder.BuildPrompt(string(frame.Task), ctx, store, d.prompts, data)
}

func (d *GraphDecider) buildNeuroSymbolicPrompt(ctx context.Context, store *meb.MEBStore, frame *GCAFrame) (*NeuroSymbolicResult, string, error) {
	analyticalStore, err := d.storeManager.GetAnalyticalStore(frame.ProjectID)
	if err != nil {
		return nil, "", err
	}

	nsConfig := &NeuroSymbolicConfig{
		SourceStore:     store,
		AnalyticalStore: analyticalStore,
		TemplateLister:  d.templateLister,
		ProjectID:       frame.ProjectID,
		LLMClient:      d.llmClient,
	}

	nsResult, err := nsConfig.ExecuteTemplateQuery(ctx, frame.Task, frame.Input)
	if err != nil {
		return nil, nsResult.AttentionSink, err
	}

	return nsResult, nsResult.AttentionSink, nil
}