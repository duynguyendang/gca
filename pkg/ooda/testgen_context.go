package ooda

import (
	"context"
	"fmt"
	"strings"

	"github.com/duynguyendang/gca/pkg/prompts"
	"github.com/duynguyendang/meb"
)

type TestGenContext struct {
	Target         string
	TargetMeta     SymbolMeta
	Framework      string
	AuthDeps       []DepMeta
	DbDeps         []DepMeta
	OtherDeps      []DepMeta
	ProjectHeaders []string
}

type SymbolMeta struct {
	Name    string
	Kind    string
	Package string
	Role    string
	Tags    []string
	Content string
}

type DepMeta struct {
	ID   string
	Name string
	Kind string
	Role string
}

func buildTestGenContext(ctx context.Context, store *meb.MEBStore, frame *GCAFrame) (*TestGenContext, error) {
	target := frame.SymbolID
	if target == "" {
		target = frame.Input
	}

	symbolID, err := resolveTarget(ctx, store, target)
	if err != nil {
		return nil, fmt.Errorf("resolve target: %w", err)
	}

	deps := bfsCallees(ctx, store, symbolID, 3)

	authDeps, dbDeps, otherDeps := classifyDeps(ctx, store, deps)

	targetMeta := getSymbolMeta(ctx, store, symbolID)

	framework := detectFramework(ctx, store)

	headers := getProjectHeaders(ctx, store, symbolID)

	return &TestGenContext{
		Target:         symbolID,
		TargetMeta:     targetMeta,
		Framework:      framework,
		AuthDeps:       authDeps,
		DbDeps:         dbDeps,
		OtherDeps:      otherDeps,
		ProjectHeaders: headers,
	}, nil
}

func resolveTarget(ctx context.Context, store *meb.MEBStore, target string) (string, error) {
	if !strings.HasPrefix(target, "/") {
		return target, nil
	}

	var result string
	for fact := range store.ScanContext(ctx, target, "handled_by", "") {
		if h, ok := fact.Object.(string); ok {
			result = h
			break
		}
	}
	if result != "" {
		return result, nil
	}
	return target, fmt.Errorf("no handler for route: %s", target)
}

func bfsCallees(ctx context.Context, store *meb.MEBStore, symbolID string, maxDepth int) []string {
	visited := make(map[string]bool)
	visited[symbolID] = true
	current := []string{symbolID}
	var result []string

	for depth := 0; depth < maxDepth; depth++ {
		var next []string
		for _, sym := range current {
			for fact := range store.ScanContext(ctx, sym, "calls", "") {
				if callee, ok := fact.Object.(string); ok && !visited[callee] {
					visited[callee] = true
					result = append(result, callee)
					next = append(next, callee)
				}
			}
		}
		current = next
		if len(current) == 0 {
			break
		}
	}
	return result
}

func classifyDeps(ctx context.Context, store *meb.MEBStore, symbolIDs []string) (auth, db, other []DepMeta) {
	for _, id := range symbolIDs {
		meta := getSymbolMeta(ctx, store, id)
		dep := DepMeta{ID: id, Name: meta.Name, Kind: meta.Kind, Role: meta.Role}

		switch meta.Role {
		case "sanitizer", "auth":
			auth = append(auth, dep)
		case "database":
			db = append(db, dep)
		default:
			other = append(other, dep)
		}
	}
	return
}

func getSymbolMeta(ctx context.Context, store *meb.MEBStore, symbolID string) SymbolMeta {
	meta := SymbolMeta{}

	var kind, pkg, role string
	var tags []string

	for fact := range store.ScanContext(ctx, symbolID, "has_kind", "") {
		if s, ok := fact.Object.(string); ok {
			kind = s
			break
		}
	}
	for fact := range store.ScanContext(ctx, symbolID, "in_package", "") {
		if s, ok := fact.Object.(string); ok {
			pkg = s
			break
		}
	}
	for fact := range store.ScanContext(ctx, symbolID, "has_role", "") {
		if s, ok := fact.Object.(string); ok {
			role = s
			break
		}
	}
	for fact := range store.ScanContext(ctx, symbolID, "has_tag", "") {
		if s, ok := fact.Object.(string); ok {
			tags = append(tags, s)
		}
	}

	if content, err := store.GetContentByKey(symbolID); err == nil && len(content) > 0 {
		meta.Content = string(content)
	}

	meta.Kind = kind
	meta.Package = pkg
	meta.Role = role
	meta.Tags = tags
	return meta
}

func detectFramework(ctx context.Context, store *meb.MEBStore) string {
	count := 0
	for fact := range store.ScanContext(ctx, "", "defines", "") {
		file := fact.Subject
		switch {
		case strings.HasSuffix(file, "_test.go"):
			return "go"
		case strings.HasSuffix(file, ".test.ts"), strings.HasSuffix(file, ".test.js"):
			return "jest"
		case strings.HasSuffix(file, "_test.py"):
			return "pytest"
		}
		count++
		if count > 100 {
			break
		}
	}
	return "go"
}

func getProjectHeaders(ctx context.Context, store *meb.MEBStore, symbolID string) []string {
	headerSet := make(map[string]bool)

	for fact := range store.ScanContext(ctx, symbolID, "documented_by", "") {
		if mdFile, ok := fact.Object.(string); ok {
			for headerFact := range store.ScanContext(ctx, mdFile, "documented_header", "") {
				if headerText, ok := headerFact.Object.(string); ok {
					headerSet[headerText] = true
				}
			}
		}
	}

	result := make([]string, 0, len(headerSet))
	for h := range headerSet {
		result = append(result, h)
	}
	return result
}

func buildTestGenerationPrompt(template *prompts.Prompt, tgc *TestGenContext) (string, error) {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Test Generation Request\n\n## Target: %s\n", tgc.Target))
	sb.WriteString(fmt.Sprintf("Kind: %s\n", tgc.TargetMeta.Kind))
	sb.WriteString(fmt.Sprintf("Package: %s\n", tgc.TargetMeta.Package))
	if tgc.TargetMeta.Role != "" {
		sb.WriteString(fmt.Sprintf("Role: %s\n", tgc.TargetMeta.Role))
	}
	sb.WriteString(fmt.Sprintf("Framework: %s\n\n", tgc.Framework))

	sb.WriteString("## Target Code\n```go\n")
	sb.WriteString(tgc.TargetMeta.Content)
	sb.WriteString("\n```\n\n")

	if len(tgc.AuthDeps) > 0 {
		sb.WriteString("## Auth Dependencies (mock these)\n")
		for _, dep := range tgc.AuthDeps {
			sb.WriteString(fmt.Sprintf("- `%s` — %s\n", dep.Name, dep.Role))
		}
		sb.WriteString("\n")
	}

	if len(tgc.DbDeps) > 0 {
		sb.WriteString("## DB Dependencies (mock these)\n")
		for _, dep := range tgc.DbDeps {
			sb.WriteString(fmt.Sprintf("- `%s` — %s\n", dep.Name, dep.Role))
		}
		sb.WriteString("\n")
	}

	if len(tgc.OtherDeps) > 0 {
		sb.WriteString("## Other Dependencies\n")
		for _, dep := range tgc.OtherDeps {
			sb.WriteString(fmt.Sprintf("- `%s`\n", dep.Name))
		}
		sb.WriteString("\n")
	}

	if len(tgc.ProjectHeaders) > 0 {
		sb.WriteString("## Design Headers (business context)\n")
		for _, h := range tgc.ProjectHeaders {
			sb.WriteString(fmt.Sprintf("- %s\n", h))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(getFrameworkGuidelines(tgc.Framework))

	contextStr := sb.String()

	templateData := map[string]interface{}{
		"Query":    tgc.Target,
		"SymbolID": tgc.Target,
		"Context":  contextStr,
	}

	return template.Execute(templateData)
}

func getFrameworkGuidelines(f string) string {
	switch f {
	case "go":
		return `## Go Test Guidelines
- Use standard testing.T with table-driven tests
- Mock auth with interfaces (define AuthService interface)
- Mock DB with mock interface or sqlmock
- Follow naming: func Test<Name>_<Scenario>
- Test: happy path, error cases, edge cases`
	case "jest":
		return `## Jest Guidelines
- Use describe()/it() or test() blocks
- jest.mock() for external modules
- beforeEach/afterEach for setup/teardown
- Test rendering, state changes, edge cases`
	case "pytest":
		return `## Pytest Guidelines
- pytest fixtures for setup/teardown
- @pytest.mark.parametrize for multiple test cases
- pytest.raises() for exception testing
- test_<function>_<scenario> naming`
	default:
		return `## Guidelines
Generate tests for: normal cases, edge cases, error conditions.`
	}
}