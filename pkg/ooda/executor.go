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

// attentionSinkQueries provides named queries for the Virtual Attention Sink.
// Query strings are defined in policies/queries.mg and loaded via common.GetNamedQuery.
func attentionSinkQuery(name string) string {
	switch name {
	case "entry_points":
		return common.GetNamedQuery("entry_point")
	case "hub_scores":
		return common.GetNamedQuery("hub_score")
	case "smells":
		return common.GetNamedQuery("smell")
	}
	return ""
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

type attentionSection struct {
	queryName      string
	header         string
	limit          int
	overflowSuffix string
	formatFn       func(r map[string]any) string
}

func formatAttentionSection(ctx context.Context, store *meb.MEBStore, section attentionSection) string {
	results, err := gcamdb.Query(ctx, store, attentionSinkQuery(section.queryName))
	if err != nil || len(results) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n" + section.header + ":\n")
	count := 0
	for _, r := range results {
		line := section.formatFn(r)
		if line == "" {
			continue
		}
		if count >= section.limit {
			break
		}
		sb.WriteString(fmt.Sprintf("  - %s\n", line))
		count++
	}
	if len(results) > section.limit && section.overflowSuffix != "" {
		sb.WriteString(fmt.Sprintf("  ... and %d more %s\n", len(results)-section.limit, section.overflowSuffix))
	}
	return sb.String()
}

func (c *NeuroSymbolicConfig) GetAttentionSink(ctx context.Context) (string, error) {
	if c.AnalyticalStore == nil {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("=== ATTENTION SINK (Pre-computed Architectural Facts) ===\n")

	sb.WriteString(formatAttentionSection(ctx, c.AnalyticalStore, attentionSection{
		queryName:      "entry_points",
		header:         "Entry Points",
		limit:          10,
		overflowSuffix: "more",
		formatFn: func(r map[string]any) string {
			if subj, ok := r["Subject"].(string); ok && subj != "" {
				return subj
			}
			return ""
		},
	}))

	sb.WriteString(formatAttentionSection(ctx, c.AnalyticalStore, attentionSection{
		queryName:      "hub_scores",
		header:         "High-Centrality Files (Hub)",
		limit:          5,
		overflowSuffix: "more",
		formatFn: func(r map[string]any) string {
			subj, ok := r["Subject"].(string)
			if !ok || subj == "" {
				return ""
			}
			if s, ok := r["Score"].(string); ok && s != "" {
				return fmt.Sprintf("%s (hub_score: %s)", subj, s)
			}
			return ""
		},
	}))

	sb.WriteString(formatAttentionSection(ctx, c.AnalyticalStore, attentionSection{
		queryName:      "smells",
		header:         "Architectural Smells",
		limit:          10,
		overflowSuffix: "more smells",
		formatFn: func(r map[string]any) string {
			subj, ok := r["Subject"].(string)
			if !ok || subj == "" {
				return ""
			}
			obj, ok := r["Object"].(string)
			if !ok || obj == "" {
				return ""
			}
			smellType := obj
			if idx := strings.Index(obj, ":"); idx > 0 {
				smellType = obj[:idx]
			}
			return fmt.Sprintf("%s (%s)", subj, smellType)
		},
	}))

	sb.WriteString("=================================================\n")

	return sb.String(), nil
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
