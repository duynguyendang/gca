package neuro

import (
	"context"
	"fmt"
	"strings"

	"github.com/duynguyendang/gca/internal/manager"
	mebpkg "github.com/duynguyendang/gca/pkg/meb"
)

// DiagnosticContext represents the parsed diagnostic context from Analytical Store.
type DiagnosticContext struct {
	EntryPoints     []string
	HubFiles        []HubFile
	SmellFlags      []SmellFlag
	QueryTemplates  []QueryTemplate
	TopSymbols      []string
}

// HubFile represents a file with hub characteristics.
type HubFile struct {
	File  string
	Score int
}

// SmellFlag represents a detected architectural smell.
type SmellFlag struct {
	File   string
	Smell  string
	Detail string
}

// QueryTemplate represents an available query template.
type QueryTemplate struct {
	ID          string
	Description string
	Category    string
}

// ContextBuilder builds diagnostic context from the Analytical Store.
type ContextBuilder struct {
	storeManager *manager.StoreManager
}

// NewContextBuilder creates a new ContextBuilder.
func NewContextBuilder(storeManager *manager.StoreManager) *ContextBuilder {
	return &ContextBuilder{
		storeManager: storeManager,
	}
}

// BuildDiagnosticContext queries the Analytical Store and builds a formatted
// diagnostic context string for inclusion in AI prompts.
func (cb *ContextBuilder) BuildDiagnosticContext(ctx context.Context, projectID string) (string, error) {
	analyticalStore, err := cb.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		return "", fmt.Errorf("failed to get analytical store: %w", err)
	}

	dc := &DiagnosticContext{}

	// Query for entry points
	entryQuery := `triples(Subject, "is_entry_point", "true")`
	entryResults, err := mebpkg.Query(ctx, analyticalStore, entryQuery)
	if err == nil {
		for _, r := range entryResults {
			if subject, ok := r["Subject"].(string); ok && subject != "" {
				dc.EntryPoints = append(dc.EntryPoints, subject)
			}
		}
	}

	// Query for hub scores
	hubQuery := `triples(Subject, "has_hub_score", Score)`
	hubResults, err := mebpkg.Query(ctx, analyticalStore, hubQuery)
	if err == nil {
		for _, r := range hubResults {
			subject, _ := r["Subject"].(string)
			scoreStr, _ := r["Score"].(string)
			if subject == "" {
				continue
			}
			score := 0
			if s, err := parseInt(scoreStr); err == nil {
				score = s
			}
			dc.HubFiles = append(dc.HubFiles, HubFile{File: subject, Score: score})
		}
	}

	// Query for smells
	smellQuery := `triples(Subject, "has_smell", Object)`
	smellResults, err := mebpkg.Query(ctx, analyticalStore, smellQuery)
	if err == nil {
		for _, r := range smellResults {
			subject, _ := r["Subject"].(string)
			object, _ := r["Object"].(string)
			if subject == "" || object == "" {
				continue
			}
			detail := ""
			if strings.Contains(object, ":") {
				parts := strings.SplitN(object, ":", 2)
				object = parts[0]
				detail = parts[1]
			}
			dc.SmellFlags = append(dc.SmellFlags, SmellFlag{
				File:   subject,
				Smell:  object,
				Detail: detail,
			})
		}
	}

	// Query for top centrality symbols
	centralityQuery := `triples(Subject, "has_centrality", Score)`
	centralityResults, err := mebpkg.Query(ctx, analyticalStore, centralityQuery)
	if err == nil {
		for _, r := range centralityResults {
			if subject, ok := r["Subject"].(string); ok && subject != "" {
				dc.TopSymbols = append(dc.TopSymbols, subject)
			}
		}
	}

	// Query for available query templates
	templateQuery := `triples(ID, "query_template", Body)`
	templateResults, err := mebpkg.Query(ctx, analyticalStore, templateQuery)
	if err == nil {
		seen := make(map[string]bool)
		for _, r := range templateResults {
			id, _ := r["ID"].(string)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			dc.QueryTemplates = append(dc.QueryTemplates, QueryTemplate{ID: id})

			// Get category for this template
			catQuery := fmt.Sprintf(`triples("%s", "category", Cat)`, id)
			if catResults, err := mebpkg.Query(ctx, analyticalStore, catQuery); err == nil && len(catResults) > 0 {
				if cat, ok := catResults[0]["Cat"].(string); ok {
					dc.QueryTemplates[len(dc.QueryTemplates)-1].Category = cat
				}
			}
		}
	}

	return cb.formatContext(dc), nil
}

// formatContext formats the diagnostic context as a string block.
func (cb *ContextBuilder) formatContext(dc *DiagnosticContext) string {
	var sb strings.Builder

	sb.WriteString("=== DIAGNOSTIC CONTEXT ===\n")

	if len(dc.EntryPoints) > 0 {
		sb.WriteString("Entry Points: ")
		sb.WriteString(strings.Join(dc.EntryPoints, ", "))
		sb.WriteString("\n")
	}

	if len(dc.HubFiles) > 0 {
		sb.WriteString("Hub Files: ")
		var hubs []string
		for _, h := range dc.HubFiles {
			hubs = append(hubs, fmt.Sprintf("%s(score:%d)", h.File, h.Score))
		}
		sb.WriteString(strings.Join(hubs, ", "))
		sb.WriteString("\n")
	}

	if len(dc.SmellFlags) > 0 {
		sb.WriteString("Smell Flags: ")
		var smells []string
		for _, s := range dc.SmellFlags {
			if s.Detail != "" {
				smells = append(smells, fmt.Sprintf("%s(%s:%s)", s.File, s.Smell, s.Detail))
			} else {
				smells = append(smells, fmt.Sprintf("%s(%s)", s.File, s.Smell))
			}
		}
		sb.WriteString(strings.Join(smells, ", "))
		sb.WriteString("\n")
	}

	if len(dc.TopSymbols) > 0 {
		sb.WriteString("High-Centrality Symbols: ")
		sb.WriteString(strings.Join(dc.TopSymbols, ", "))
		sb.WriteString("\n")
	}

	if len(dc.QueryTemplates) > 0 {
		sb.WriteString("Available Query Templates: ")
		var templates []string
		for _, t := range dc.QueryTemplates {
			templates = append(templates, t.ID)
		}
		sb.WriteString(strings.Join(templates, ", "))
		sb.WriteString("\n")
	}

	sb.WriteString("=============================\n")

	return sb.String()
}

// parseInt parses a string to int.
func parseInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			break
		}
	}
	return n, nil
}
