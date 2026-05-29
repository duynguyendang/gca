package ooda

import (
	"context"
	"fmt"
	"strings"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/meb"
)

const (
	maxRounds         = 5
	maxSymbolsToFetch = 10
)

type MultiRoundConfig struct {
	Store   *meb.MEBStore
	Model   common.LLMClient
	Depth   int
}

func RunMultiRoundTestGen(ctx context.Context, config *MultiRoundConfig, target string) (string, error) {
	if config.Depth <= 0 {
		config.Depth = 3
	}

	symbolID, err := resolveTarget(ctx, config.Store, target)
	if err != nil {
		return "", fmt.Errorf("resolve target: %w", err)
	}

	downstream := bfsCallees(ctx, config.Store, symbolID, config.Depth)

	authDeps, dbDeps, otherDeps := classifyDeps(ctx, config.Store, downstream)
	targetMeta := getSymbolMeta(ctx, config.Store, symbolID)
	framework := detectFramework(ctx, config.Store)
	routeConfig := getRouteConfig(ctx, config.Store, symbolID)

	allDeps := make([]DepMeta, 0, len(authDeps)+len(dbDeps)+len(otherDeps))
	allDeps = append(allDeps, authDeps...)
	allDeps = append(allDeps, dbDeps...)
	allDeps = append(allDeps, otherDeps...)

	fetchedCode := make(map[string]string)

	analyzePrompt := buildAnalyzePrompt(symbolID, targetMeta, routeConfig, allDeps, framework)

	for round := 0; round < maxRounds; round++ {
		response, err := config.Model.GenerateContent(ctx, analyzePrompt)
		if err != nil {
			return "", fmt.Errorf("llm call failed: %w", err)
		}

		response = strings.TrimSpace(response)

		if strings.HasPrefix(response, "GENERATE:") {
			code := strings.TrimPrefix(response, "GENERATE:")
			return strings.TrimSpace(code), nil
		}

		if strings.HasPrefix(response, "FETCH:") {
			fetchLine := strings.TrimPrefix(response, "FETCH:")
			fetchLine = strings.TrimSpace(fetchLine)

			symbolsToFetch := parseSymbolList(fetchLine)
			if len(symbolsToFetch) > maxSymbolsToFetch {
				symbolsToFetch = symbolsToFetch[:maxSymbolsToFetch]
			}

			fetchCount := 0
			for _, sym := range symbolsToFetch {
				if _, alreadyHave := fetchedCode[sym]; alreadyHave {
					continue
				}
				if content, err := config.Store.GetContentByKey(sym); err == nil && len(content) > 0 {
					fetchedCode[sym] = string(content)
					fetchCount++
				}
			}

			if fetchCount == 0 && len(symbolsToFetch) > 0 {
				return "", fmt.Errorf("none of the requested symbols could be fetched: %s", symbolsToFetch)
			}

			generatePrompt := buildGeneratePrompt(symbolID, targetMeta, routeConfig, allDeps, fetchedCode, framework)
			response, err = config.Model.GenerateContent(ctx, generatePrompt)
			if err != nil {
				return "", fmt.Errorf("llm call failed: %w", err)
			}

			response = strings.TrimSpace(response)

			if strings.HasPrefix(response, "GENERATE:") {
				code := strings.TrimPrefix(response, "GENERATE:")
				return strings.TrimSpace(code), nil
			}

			if strings.HasPrefix(response, "FETCH:") {
				analyzePrompt = buildAnalyzePromptWithCode(symbolID, targetMeta, routeConfig, allDeps, fetchedCode, framework)
				continue
			}

			if isLikelyTestCode(response) {
				return response, nil
			}
			return response, fmt.Errorf("unrecognized response in round %d: %s", round+1, response[:min(50, len(response))])
		}

		if isLikelyTestCode(response) {
			return response, nil
		}

		return response, fmt.Errorf("unrecognized response format in round %d: %s", round+1, response[:min(50, len(response))])
	}

	return "", fmt.Errorf("exceeded maximum rounds (%d)", maxRounds)
}

func parseSymbolList(line string) []string {
	var result []string
	parts := strings.Split(line, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func isLikelyTestCode(response string) bool {
	responseLower := strings.ToLower(response)
	hasTestImport := strings.Contains(responseLower, "testing") ||
		strings.Contains(responseLower, "httptest") ||
		strings.Contains(responseLower, "supertest") ||
		strings.Contains(responseLower, "pytest")
	hasFuncTest := strings.Contains(responseLower, "func test") ||
		strings.Contains(responseLower, "def test_") ||
		strings.Contains(responseLower, "describe(") ||
		strings.Contains(responseLower, "it(")
	return hasTestImport && hasFuncTest
}

func buildAnalyzePrompt(symbolID string, targetMeta SymbolMeta, routeConfig *RouteConfig, deps []DepMeta, framework string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Test Generation — Call Chain Analysis\n\n## Target Handler: %s\n", symbolID))
	sb.WriteString(fmt.Sprintf("Kind: %s\n", targetMeta.Kind))
	sb.WriteString(fmt.Sprintf("Package: %s\n", targetMeta.Package))
	if targetMeta.Role != "" {
		sb.WriteString(fmt.Sprintf("Role: %s\n", targetMeta.Role))
	}
	sb.WriteString(fmt.Sprintf("Framework: %s\n\n", framework))

	if routeConfig != nil {
		sb.WriteString("## Route Config\n")
		sb.WriteString(fmt.Sprintf("Method: %s\n", routeConfig.Method))
		sb.WriteString(fmt.Sprintf("Path: %s\n", routeConfig.Path))
		if len(routeConfig.Params) > 0 {
			sb.WriteString(fmt.Sprintf("Params: %s\n", strings.Join(routeConfig.Params, ", ")))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Target Handler Code\n```go\n")
	sb.WriteString(targetMeta.Content)
	sb.WriteString("\n```\n\n")

	if targetMeta.Signature != "" {
		sb.WriteString(fmt.Sprintf("## Target Signature\n`%s`\n\n", targetMeta.Signature))
	}

	sb.WriteString("## Downstream Call Chain\n")
	sb.WriteString(fmt.Sprintf("Total callees: %d\n\n", len(deps)))

	if len(deps) > 0 {
		sb.WriteString("| Symbol | Kind | Role |\n")
		sb.WriteString("|--------|------|------|\n")
		for _, dep := range deps {
			sb.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", dep.ID, dep.Kind, dep.Role))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Framework Guidelines\n")
	sb.WriteString(getFrameworkGuidelines(framework))

	sb.WriteString("\n\n## Your Task\n")
	sb.WriteString("Analyze the handler and its downstream call chain. Identify which downstream functions you need FULL CODE for to write accurate integration tests. Focus on functions that perform auth checks, DB access, or complex business logic.\n\n")
	sb.WriteString("Output one of:\n")
	sb.WriteString("- `FETCH: handlers.go:GetUser, db.go:CreateSession` — use the symbol ID exactly as shown in the table\n")
	sb.WriteString("- `GENERATE:` followed by your integration test code — if you have enough context\n\n")
	sb.WriteString("Be selective. Only request functions critical for understanding the handler's behavior.\n")

	return sb.String()
}

func buildAnalyzePromptWithCode(symbolID string, targetMeta SymbolMeta, routeConfig *RouteConfig, deps []DepMeta, fetchedCode map[string]string, framework string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Test Generation — Call Chain Analysis (Round 2+)\n\n## Target Handler: %s\n", symbolID))
	sb.WriteString(fmt.Sprintf("Kind: %s\n", targetMeta.Kind))
	sb.WriteString(fmt.Sprintf("Package: %s\n", targetMeta.Package))
	if targetMeta.Role != "" {
		sb.WriteString(fmt.Sprintf("Role: %s\n", targetMeta.Role))
	}
	sb.WriteString(fmt.Sprintf("Framework: %s\n\n", framework))

	if routeConfig != nil {
		sb.WriteString("## Route Config\n")
		sb.WriteString(fmt.Sprintf("Method: %s\n", routeConfig.Method))
		sb.WriteString(fmt.Sprintf("Path: %s\n", routeConfig.Path))
		if len(routeConfig.Params) > 0 {
			sb.WriteString(fmt.Sprintf("Params: %s\n", strings.Join(routeConfig.Params, ", ")))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Target Handler Code\n```go\n")
	sb.WriteString(targetMeta.Content)
	sb.WriteString("\n```\n\n")

	if targetMeta.Signature != "" {
		sb.WriteString(fmt.Sprintf("## Target Signature\n`%s`\n\n", targetMeta.Signature))
	}

	if len(fetchedCode) > 0 {
		sb.WriteString("## Fetched Downstream Function Code\n")
		for sym, code := range fetchedCode {
			sb.WriteString(fmt.Sprintf("\n### %s\n```go\n%s\n```\n\n", sym, code))
		}
	}

	unfetchedCount := 0
	for _, dep := range deps {
		if _, ok := fetchedCode[dep.ID]; !ok {
			unfetchedCount++
		}
	}
	sb.WriteString(fmt.Sprintf("## Unfetched Call Chain (%d remaining)\n", unfetchedCount))
	if unfetchedCount > 0 {
		for _, dep := range deps {
			if _, ok := fetchedCode[dep.ID]; ok {
				continue
			}
			sb.WriteString(fmt.Sprintf("- `%s` (%s, %s)\n", dep.ID, dep.Kind, dep.Role))
		}
		sb.WriteString("\n(Fetched code shown above)\n")
	}

	sb.WriteString("## Framework Guidelines\n")
	sb.WriteString(getFrameworkGuidelines(framework))

	sb.WriteString("\n\n## Your Task\n")
	sb.WriteString("You now have code for the critical downstream functions. You may:\n")
		sb.WriteString("- Request more code: `FETCH: handlers.go:FuncX, db.go:FuncY` — use the symbol ID exactly as shown in the table\n")
	sb.WriteString("- Generate tests now: `GENERATE:` followed by your complete integration test code\n\n")
	sb.WriteString("Be specific about which functions you need.\n")

	return sb.String()
}

func buildGeneratePrompt(symbolID string, targetMeta SymbolMeta, routeConfig *RouteConfig, deps []DepMeta, fetchedCode map[string]string, framework string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Test Generation — Integration Tests\n\n## Target Handler: %s\n", symbolID))
	sb.WriteString(fmt.Sprintf("Kind: %s\n", targetMeta.Kind))
	sb.WriteString(fmt.Sprintf("Package: %s\n", targetMeta.Package))
	if targetMeta.Role != "" {
		sb.WriteString(fmt.Sprintf("Role: %s\n", targetMeta.Role))
	}
	sb.WriteString(fmt.Sprintf("Framework: %s\n\n", framework))

	if routeConfig != nil {
		sb.WriteString("## Route Config\n")
		sb.WriteString(fmt.Sprintf("Method: %s\n", routeConfig.Method))
		sb.WriteString(fmt.Sprintf("Path: %s\n", routeConfig.Path))
		if len(routeConfig.Params) > 0 {
			sb.WriteString(fmt.Sprintf("Params: %s\n", strings.Join(routeConfig.Params, ", ")))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Target Handler Code\n```go\n")
	sb.WriteString(targetMeta.Content)
	sb.WriteString("\n```\n\n")

	if targetMeta.Signature != "" {
		sb.WriteString(fmt.Sprintf("## Target Signature\n`%s`\n\n", targetMeta.Signature))
	}

	if len(fetchedCode) > 0 {
		sb.WriteString("## Downstream Function Code (Requested)\n")
		for sym, code := range fetchedCode {
			sb.WriteString(fmt.Sprintf("\n### %s\n```go\n%s\n```\n\n", sym, code))
		}
	}

	if len(deps) > 0 {
		var auth, db, other []DepMeta
		for _, dep := range deps {
			if _, fetched := fetchedCode[dep.ID]; fetched {
				continue
			}
			if dep.Role == "api_handler" || dep.Role == "sanitizer" || dep.Role == "public_api" {
				auth = append(auth, dep)
			} else if dep.Role == "data_contract" || dep.Role == "database" {
				db = append(db, dep)
			} else {
				other = append(other, dep)
			}
		}

		if len(auth) > 0 {
			sb.WriteString("## Auth Dependencies (mock these)\n")
			for _, dep := range auth {
				sb.WriteString(fmt.Sprintf("- `%s` — %s\n", dep.Name, dep.Role))
			}
			sb.WriteString("\n")
		}
		if len(db) > 0 {
			sb.WriteString("## DB Dependencies (mock these)\n")
			for _, dep := range db {
				sb.WriteString(fmt.Sprintf("- `%s` — %s\n", dep.Name, dep.Role))
			}
			sb.WriteString("\n")
		}
		if len(other) > 0 {
			sb.WriteString("## Other Dependencies\n")
			for _, dep := range other {
				sb.WriteString(fmt.Sprintf("- `%s`\n", dep.Name))
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("## Framework Guidelines\n")
	sb.WriteString(getFrameworkGuidelines(framework))

	sb.WriteString("\n\n## Task\n")
	sb.WriteString("Generate comprehensive integration tests for the target handler. Write real HTTP-level tests that:\n")
	sb.WriteString("- Use httptest.NewServer / supertest / TestClient\n")
	sb.WriteString("- Test path params, query params, JSON body\n")
	sb.WriteString("- Cover happy path, validation errors, auth failures, edge cases\n")
	sb.WriteString("- Mock auth and DB dependencies\n\n")
	sb.WriteString("Output: `GENERATE:` followed by your complete test code.\n")

	return sb.String()
}