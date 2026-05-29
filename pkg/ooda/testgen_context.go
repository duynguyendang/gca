package ooda

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/duynguyendang/gca/pkg/prompts"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/meb"
)

type TestGenContext struct {
	Target     string
	TargetMeta SymbolMeta
	Framework  string
	RouteConfig *RouteConfig
	AuthDeps   []DepMeta
	DbDeps     []DepMeta
	OtherDeps  []DepMeta
}

type RouteConfig struct {
	Method string
	Path   string
	Params []string
}

type SymbolMeta struct {
	Name      string
	Kind      string
	Package   string
	Role      string
	Tags      []string
	Content   string
	Signature string
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

	depth := 3
	if frame.Data != nil {
		if m, ok := frame.Data.(map[string]any); ok {
			if d, ok := m["depth"].(int); ok && d > 0 {
				depth = d
			}
		}
	}

	deps := bfsCallees(ctx, store, symbolID, depth)

	authDeps, dbDeps, otherDeps := classifyDeps(ctx, store, deps)

	targetMeta := getSymbolMeta(ctx, store, symbolID)

	framework := detectFramework(ctx, store)

	routeConfig := getRouteConfig(ctx, store, symbolID)

	return &TestGenContext{
		Target:     symbolID,
		TargetMeta: targetMeta,
		Framework:  framework,
		RouteConfig: routeConfig,
		AuthDeps:   authDeps,
		DbDeps:     dbDeps,
		OtherDeps:  otherDeps,
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
		if len(current) == 0 {
			break
		}

		callees := queryCalleesAtDepth(ctx, store, current)
		var next []string
		for _, callee := range callees {
			if !visited[callee] {
				visited[callee] = true
				result = append(result, callee)
				next = append(next, callee)
			}
		}
		current = next
	}
	return result
}

func queryCalleesAtDepth(ctx context.Context, store *meb.MEBStore, symbols []string) []string {
	if len(symbols) == 0 {
		return nil
	}

	var orClauses []string
	for _, sym := range symbols {
		orClauses = append(orClauses, fmt.Sprintf(`triples("%s", "calls", Callee)`, sym))
	}
	query := strings.Join(orClauses, ", ")

		results, err := gcamdb.Query(ctx, store, query)
	if err != nil || len(results) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var callees []string
	for _, row := range results {
		if callee, ok := row["Callee"].(string); ok && !seen[callee] {
			seen[callee] = true
			callees = append(callees, callee)
		}
	}
	return callees
}

func classifyDeps(ctx context.Context, store *meb.MEBStore, symbolIDs []string) (auth, db, other []DepMeta) {
	for _, id := range symbolIDs {
		meta := getSymbolMeta(ctx, store, id)
		dep := DepMeta{ID: id, Name: meta.Name, Kind: meta.Kind, Role: meta.Role}

		isAuth := meta.Role == "api_handler"
		isDb := meta.Role == "data_contract"

		if !isAuth && !isDb {
			fileID := filePrefix(id)
			for fact := range store.ScanContext(ctx, fileID, "has_tag", "") {
				if tag, ok := fact.Object.(string); ok {
					if tag == "sanitizer" || tag == "public_api" {
						isAuth = true
					}
					if tag == "database" {
						isDb = true
					}
				}
			}
		}

		if isAuth {
			auth = append(auth, dep)
		} else if isDb {
			db = append(db, dep)
		} else {
			other = append(other, dep)
		}
	}
	return
}

func filePrefix(symbolID string) string {
	if idx := strings.LastIndex(symbolID, ":"); idx >= 0 {
		return symbolID[:idx]
	}
	return symbolID
}

func getSymbolMeta(ctx context.Context, store *meb.MEBStore, symbolID string) SymbolMeta {
	meta := SymbolMeta{Name: extractSymbolName(symbolID)}

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
		meta.Signature = extractSignature(string(content), meta.Name)
	}

	meta.Kind = kind
	meta.Package = pkg
	meta.Role = role
	meta.Tags = tags
	return meta
}

func extractSignature(content, funcName string) string {
	if funcName == "" || content == "" {
		return ""
	}

	funcRegex := regexp.MustCompile(`(?m)^func\s+\([^)]+\)\s*` + regexp.QuoteMeta(funcName) + `\s*\([^)]*\)`)
	matches := funcRegex.FindStringSubmatch(content)
	if len(matches) > 0 {
		sig := strings.TrimSpace(matches[0])
		sig = regexp.MustCompile(`\s+`).ReplaceAllString(sig, " ")
		return sig
	}

	funcRegex2 := regexp.MustCompile(`(?m)^func\s+` + regexp.QuoteMeta(funcName) + `\s*\([^)]*\)`)
	matches2 := funcRegex2.FindStringSubmatch(content)
	if len(matches2) > 0 {
		sig := strings.TrimSpace(matches2[0])
		sig = regexp.MustCompile(`\s+`).ReplaceAllString(sig, " ")
		return sig
	}

	return ""
}

func extractSymbolName(symbolID string) string {
	if idx := strings.LastIndex(symbolID, ":"); idx >= 0 && idx < len(symbolID)-1 {
		return symbolID[idx+1:]
	}
	return symbolID
}

func getRouteConfig(ctx context.Context, store *meb.MEBStore, symbolID string) *RouteConfig {
	var route, method, path string
	var pathParams []string

	for fact := range store.ScanContext(ctx, "", "handled_by", symbolID) {
		route = fact.Subject
		break
	}
	if route == "" {
		return nil
	}

	if idx := strings.Index(route, ":"); idx >= 0 {
		method = route[:idx]
		path = route[idx+1:]
	} else {
		method = "GET"
		path = route
	}

	methodUpper := strings.ToUpper(method)
	if methodUpper != "GET" && methodUpper != "POST" && methodUpper != "PUT" &&
		methodUpper != "DELETE" && methodUpper != "PATCH" && methodUpper != "OPTIONS" {
		method = "GET"
	}

	paramRegex := regexp.MustCompile(`:([a-zA-Z_][a-zA-Z0-9_]*)`)
	matches := paramRegex.FindAllStringSubmatch(path, -1)
	for _, m := range matches {
		pathParams = append(pathParams, m[1])
	}

	return &RouteConfig{
		Method: method,
		Path:   path,
		Params: pathParams,
	}
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

func buildTestGenerationPrompt(template *prompts.Prompt, tgc *TestGenContext) (string, error) {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Test Generation Request\n\n## Target: %s\n", tgc.Target))
	sb.WriteString(fmt.Sprintf("Kind: %s\n", tgc.TargetMeta.Kind))
	sb.WriteString(fmt.Sprintf("Package: %s\n", tgc.TargetMeta.Package))
	if tgc.TargetMeta.Role != "" {
		sb.WriteString(fmt.Sprintf("Role: %s\n", tgc.TargetMeta.Role))
	}
	sb.WriteString(fmt.Sprintf("Framework: %s\n\n", tgc.Framework))

	if tgc.RouteConfig != nil {
		sb.WriteString("## Route Config\n")
		sb.WriteString(fmt.Sprintf("Method: %s\n", tgc.RouteConfig.Method))
		sb.WriteString(fmt.Sprintf("Path: %s\n", tgc.RouteConfig.Path))
		if len(tgc.RouteConfig.Params) > 0 {
			sb.WriteString(fmt.Sprintf("Params: %s\n", strings.Join(tgc.RouteConfig.Params, ", ")))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Target Code\n```go\n")
	sb.WriteString(tgc.TargetMeta.Content)
	sb.WriteString("\n```\n\n")

	if tgc.TargetMeta.Signature != "" {
		sb.WriteString(fmt.Sprintf("## Target Signature\n`%s`\n\n", tgc.TargetMeta.Signature))
	}

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
		return `## Go Integration Test Guidelines
- Use httptest.NewServer to create a test HTTP server from your router
- Use httptest.NewRequest to craft HTTP requests with proper method/URL/headers
- Assert status codes: if w.Code != http.StatusOK { t.Errorf(...) }
- Test route path params via URL construction: /api/users/123
- Test query params: URL + "?page=1&limit=10"
- Mock auth by injecting a test middleware or interface
- Mock DB with test fakes, sqlmock, or in-memory implementations
- Follow naming: func TestIntegration_<Handler>_<Scenario>
- Cover: happy path, validation errors, auth failures, edge cases`
	case "jest":
		return `## Jest Integration Test Guidelines
- Use supertest (for Express/Fastify) or node-fetch/jest-fetch-mock for HTTP testing
- Create a test server instance in beforeAll, close it in afterAll
- Use request(object) or fetch(URL) to make HTTP calls
- Test route parameters: /api/users/:id → /api/users/123
- Test query parameters: ?name=test&role=admin
- Mock external dependencies with jest.mock()
- Assert: status codes (expect(res.status).toBe(200)), response body shape
- Cover: happy path, validation errors, auth failures, edge cases`
	case "pytest":
		return `## Pytest Integration Test Guidelines
- Use your framework's TestClient (Django TestCase, FastAPI TestClient, Flask test_client)
- pytest fixtures for test server setup/teardown
- @pytest.mark.parametrize for multiple request variants
- Assert: status codes (assert response.status_code == 200), JSON body keys
- Test: path params, query params, JSON body, auth headers
- Mock external services or databases with fakes/mocks
- Cover: happy path, validation errors, auth failures, edge cases`
	default:
		return `## Integration Test Guidelines
Generate tests that make real HTTP requests to the endpoint.
Cover: normal cases, edge cases, error conditions.`
	}
}