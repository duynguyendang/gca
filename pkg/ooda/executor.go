package ooda

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/gca/pkg/prompts"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/meb"
)

type TemplateLister interface {
	ListTemplates(ctx context.Context, projectID, category string) ([]*ingest.TemplateStoreQuery, error)
}

type NeuroSymbolicConfig struct {
	SourceStore     *meb.MEBStore
	AnalyticalStore *meb.MEBStore
	TemplateLister  TemplateLister
	ProjectID       string
	LLMClient       common.LLMClient
}

type NeuroSymbolicResult struct {
	Query         string
	Results       []map[string]any
	TemplateID    string
	Entity        string
	Reasoning     string
	ExecError     error
	SkipSynthesis bool
	AttentionSink string
}

type TemplateSelection struct {
	TemplateID string
	Params     map[string]string
	Skip       bool
	Reason     string
}

var neuroSymPromptTemplate *prompts.Prompt

var templateCategories = []string{"smell", "analysis", "datalog", "test", "performance", "security", "refactor"}

var attentionSinkQueries = map[string]string{
	"entry_points": `triples(Subject, "is_entry_point", "true")`,
	"hub_scores":   `triples(Subject, "has_hub_score", Score)`,
	"smells":       `triples(Subject, "has_smell", Object)`,
}

func init() {
	var err error
	neuroSymPromptTemplate, err = prompts.LoadPrompt("prompts/neuro_symbolic.prompt")
	if err != nil {
		neuroSymPromptTemplate = nil
	}
}

func formatTemplatesForAI(templates []*ingest.TemplateStoreQuery) string {
	if len(templates) == 0 {
		return "No templates available."
	}
	var sb strings.Builder
	for _, t := range templates {
		sb.WriteString(fmt.Sprintf("- %s(%s): %s\n",
			t.ID, formatParams(t.Parameters), t.Description))
	}
	return sb.String()
}

func formatParams(params []ingest.TemplateParam) string {
	if len(params) == 0 {
		return ""
	}
	var parts []string
	for _, p := range params {
		parts = append(parts, fmt.Sprintf("%s (%s)", p.Name, p.Type))
	}
	return strings.Join(parts, ", ")
}

func parseTemplateResponse(response string) *TemplateSelection {
	result := &TemplateSelection{
		Params: make(map[string]string),
	}

	upperResp := strings.ToUpper(response)
	if strings.Contains(upperResp, "TEMPLATE: NONE") {
		result.Skip = true
		if idx := strings.Index(upperResp, "REASON:"); idx >= 0 {
			result.Reason = strings.TrimSpace(response[idx+8:])
		}
		return result
	}

	templateRE := regexp.MustCompile(`(?i)TEMPLATE:\s*(\S+)`)
	if match := templateRE.FindStringSubmatch(response); len(match) > 1 {
		result.TemplateID = strings.TrimSpace(match[1])
	}

	paramsRE := regexp.MustCompile(`(?i)PARAMS:\s*(.+?)(?:\n|$)`)
	if match := paramsRE.FindStringSubmatch(response); len(match) > 1 {
		paramsStr := strings.TrimSpace(match[1])
		if paramsStr != "" && paramsStr != "none" {
			for _, pair := range strings.Split(paramsStr, ",") {
				pair = strings.TrimSpace(pair)
				if idx := strings.Index(pair, "="); idx > 0 {
					key := strings.TrimSpace(pair[:idx])
					val := strings.TrimSpace(pair[idx+1:])
					if key != "" {
						result.Params[key] = val
					}
				}
			}
		}
	}

	return result
}

func selectTemplateWithAI(ctx context.Context, cfg *NeuroSymbolicConfig, attentionSink, userQuery string) (*TemplateSelection, []*ingest.TemplateStoreQuery, error) {
	selection := &TemplateSelection{
		Params: make(map[string]string),
	}

	categories := templateCategories
	var allTemplates []*ingest.TemplateStoreQuery
	for _, cat := range categories {
		templates, err := cfg.TemplateLister.ListTemplates(ctx, cfg.ProjectID, cat)
		if err == nil && len(templates) > 0 {
			allTemplates = append(allTemplates, templates...)
		}
	}

	if len(allTemplates) == 0 {
		selection.Skip = true
		selection.Reason = "No templates available in template store"
		return selection, nil, nil
	}

	templatesStr := formatTemplatesForAI(allTemplates)

	data := map[string]interface{}{
		"DiagnosticContext": attentionSink,
		"AvailableTemplates": templatesStr,
		"UserQuery":         userQuery,
	}

	var promptText string
	if neuroSymPromptTemplate != nil {
		promptText, _ = neuroSymPromptTemplate.Execute(data)
	} else {
		selection.Skip = true
		selection.Reason = "neuro_symbolic prompt not loaded"
		return selection, allTemplates, nil
	}

	llmCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	llmResp, err := cfg.LLMClient.GenerateContent(llmCtx, promptText)
	if err != nil {
		return nil, nil, fmt.Errorf("AI template selection failed: %w", err)
	}

	return parseTemplateResponse(llmResp), allTemplates, nil
}

func parameterizeTemplate(templateBody string, params map[string]string) string {
	result := templateBody
	for key, val := range params {
		placeholder := "{{" + key + "}}"
		result = strings.Replace(result, placeholder, val, -1)
	}
	result = strings.Replace(result, "{{target}}", "", -1)
	result = strings.Replace(result, "{{File}}", "", -1)
	result = strings.Replace(result, "{{Symbol}}", "", -1)
	return result
}

func (c *NeuroSymbolicConfig) ExecuteTemplateQuery(ctx context.Context, task GCATask, query string) (*NeuroSymbolicResult, error) {
	result := &NeuroSymbolicResult{}

	attentionSink, err := c.GetAttentionSink(ctx)
	if err != nil {
		result.Reasoning = fmt.Sprintf("Failed to get attention sink: %v", err)
		return result, nil
	}

	result.AttentionSink = attentionSink

	selection, allTemplates, err := selectTemplateWithAI(ctx, c, attentionSink, query)
	if err != nil {
		result.Reasoning = fmt.Sprintf("Template selection failed: %v", err)
		return result, nil
	}

	if selection.Skip {
		result.SkipSynthesis = true
		result.Reasoning = selection.Reason
		result.AttentionSink = attentionSink
		return result, nil
	}

	if selection.TemplateID == "" {
		result.SkipSynthesis = true
		result.Reasoning = "No template selected by AI"
		result.AttentionSink = attentionSink
		return result, nil
	}

	result.TemplateID = selection.TemplateID

	var selectedTemplate *ingest.TemplateStoreQuery
	for _, t := range allTemplates {
		if t.ID == selection.TemplateID {
			selectedTemplate = t
			break
		}
	}

	if selectedTemplate == nil {
		result.SkipSynthesis = true
		result.Reasoning = fmt.Sprintf("Template '%s' not found in template store", selection.TemplateID)
		result.AttentionSink = attentionSink
		return result, nil
	}

	parametrized := parameterizeTemplate(selectedTemplate.Body, selection.Params)
	if parametrized == selectedTemplate.Body && len(selection.Params) > 0 {
		parametrized = parameterizeWithFallback(selectedTemplate.Body, query)
	}

	result.Query = parametrized
	if len(selection.Params) > 0 {
		result.Entity = selection.Params["File"]
		if result.Entity == "" {
			result.Entity = selection.Params["Symbol"]
		}
		if result.Entity == "" {
			for _, v := range selection.Params {
				if v != "" {
					result.Entity = v
					break
				}
			}
		}
	}

	if parametrized == "" {
		result.SkipSynthesis = true
		result.Reasoning = "Failed to parameterize template"
		return result, nil
	}

	results, err := gcamdb.Query(ctx, c.SourceStore, parametrized)
	if err != nil {
		result.ExecError = fmt.Errorf("template query execution failed: %w", err)
		result.Reasoning = fmt.Sprintf("Template '%s' selected but execution failed: %v", selectedTemplate.ID, err)
		return result, nil
	}

	result.Results = results
	result.Reasoning = fmt.Sprintf("Template '%s' selected and executed. Found %d results.",
		selectedTemplate.ID, len(results))

	return result, nil
}

func parameterizeWithFallback(templateBody, query string) string {
	result := templateBody

	symbolRE := regexp.MustCompile(`([A-Z][a-zA-Z0-9]+(?:\.[a-zA-Z0-9]+)+)|"([^"]+)"|([a-zA-Z_][a-zA-Z0-9_]*(?:/[a-zA-Z0-9_]+)*)`)
	matches := symbolRE.FindAllString(query, -1)

	if len(matches) > 0 {
		target := strings.Trim(matches[0], "\"' ")
		result = strings.Replace(result, "{{target}}", target, -1)
		result = strings.Replace(result, "{{File}}", target, -1)
		result = strings.Replace(result, "{{Symbol}}", target, -1)
	}

	result = regexp.MustCompile(`\?(?:\s|$)`).ReplaceAllString(result, "?target")

	result = strings.Replace(result, "{{target}}", "", -1)
	result = strings.Replace(result, "{{File}}", "", -1)
	result = strings.Replace(result, "{{Symbol}}", "", -1)

	return result
}

func (c *NeuroSymbolicConfig) GetAttentionSink(ctx context.Context) (string, error) {
	if c.AnalyticalStore == nil {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("=== ATTENTION SINK (Pre-computed Architectural Facts) ===\n")

	if entries, err := gcamdb.Query(ctx, c.AnalyticalStore, attentionSinkQueries["entry_points"]); err == nil && len(entries) > 0 {
		sb.WriteString("\nEntry Points:\n")
		count := 0
		for _, r := range entries {
			if subj, ok := r["Subject"].(string); ok && subj != "" && count < 10 {
				sb.WriteString(fmt.Sprintf("  - %s\n", subj))
				count++
			}
		}
		if len(entries) > 10 {
			sb.WriteString(fmt.Sprintf("  ... and %d more\n", len(entries)-10))
		}
	}

	if hubs, err := gcamdb.Query(ctx, c.AnalyticalStore, attentionSinkQueries["hub_scores"]); err == nil && len(hubs) > 0 {
		sb.WriteString("\nHigh-Centrality Files (Hub):\n")
		count := 0
		for _, r := range hubs {
			if subj, ok := r["Subject"].(string); ok && subj != "" {
				scoreStr := ""
				if s, ok := r["Score"].(string); ok {
					scoreStr = s
				}
				if scoreStr != "" && count < 5 {
					sb.WriteString(fmt.Sprintf("  - %s (hub_score: %s)\n", subj, scoreStr))
					count++
				}
			}
		}
	}

	if smells, err := gcamdb.Query(ctx, c.AnalyticalStore, attentionSinkQueries["smells"]); err == nil && len(smells) > 0 {
		sb.WriteString("\nArchitectural Smells:\n")
		count := 0
		for _, r := range smells {
			if subj, ok := r["Subject"].(string); ok && subj != "" {
				obj := ""
				if o, ok := r["Object"].(string); ok {
					obj = o
				}
				if subj != "" && obj != "" && count < 10 {
					smellType := obj
					if idx := strings.Index(obj, ":"); idx > 0 {
						smellType = obj[:idx]
					}
					sb.WriteString(fmt.Sprintf("  - %s (%s)\n", subj, smellType))
					count++
				}
			}
		}
		if len(smells) > 10 {
			sb.WriteString(fmt.Sprintf("  ... and %d more smells\n", len(smells)-10))
		}
	}

	sb.WriteString("=================================================\n")

	return sb.String(), nil
}

func BuildNeuroSymbolicContext(query string, nsResult *NeuroSymbolicResult, attentionSink string) string {
	var sb strings.Builder

	sb.WriteString(attentionSink)

	sb.WriteString("\n=== QUERY RESULT ===\n")
	sb.WriteString(fmt.Sprintf("User Query: %s\n", query))
	if nsResult.Entity != "" {
		sb.WriteString(fmt.Sprintf("Target Entity: %s\n", nsResult.Entity))
	}
	if nsResult.TemplateID != "" {
		sb.WriteString(fmt.Sprintf("Template Used: %s\n", nsResult.TemplateID))
		sb.WriteString(fmt.Sprintf("Datalog Query: %s\n", nsResult.Query))
	}
	if nsResult.Reasoning != "" {
		sb.WriteString(fmt.Sprintf("Reasoning: %s\n", nsResult.Reasoning))
	}
	if len(nsResult.Results) > 0 {
		sb.WriteString(fmt.Sprintf("Results Found: %d\n", len(nsResult.Results)))
		for i, r := range nsResult.Results {
			if i >= 5 {
				sb.WriteString(fmt.Sprintf("  ... and %d more\n", len(nsResult.Results)-5))
				break
			}
			for k, v := range r {
				sb.WriteString(fmt.Sprintf("  %s = %v\n", k, v))
			}
		}
	}

	return sb.String()
}

func BuildSynthesisContext(query string, nsResult *NeuroSymbolicResult, attentionSink string) string {
	var sb strings.Builder

	sb.WriteString(attentionSink)

	if nsResult.TemplateID != "" {
		sb.WriteString(fmt.Sprintf("\nSelected Template: %s\n", nsResult.TemplateID))
		sb.WriteString(fmt.Sprintf("Datalog Query: %s\n", nsResult.Query))
	}

	if len(nsResult.Results) > 0 {
		sb.WriteString(fmt.Sprintf("\nQuery Results (%d items):\n", len(nsResult.Results)))
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
	} else {
		sb.WriteString("\nQuery Results: No results found.\n")
	}

	return sb.String()
}
