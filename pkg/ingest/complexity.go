package ingest

import (
	"strings"

	"github.com/duynguyendang/gca/pkg/config"
)

// countBranchKeywords counts cyclomatic complexity by scanning for control-flow
// keywords in source content. This is a heuristic (not AST-accurate) but works
// when the AST node isn't available. Keyword counts include:
// if, for, switch, select, defer, case, elif, while, match, &&, ||, and, or.
func countBranchKeywords(content string) int {
	count := 0
	for _, kw := range []string{
		"if ", "if(", "if\t",
		"for ", "for(", "for\t",
		"switch ", "switch(",
		"select ",
		"defer ",
		"case ",
		"elif ",
		"while ",
		"match ",
	} {
		count += strings.Count(content, kw)
	}
	// Count logical operators (&&, ||, and, or) — they add to cyclomatic complexity.
	count += strings.Count(content, "&&")
	count += strings.Count(content, "||")
	count += strings.Count(content, " and ")
	count += strings.Count(content, " or ")
	return count
}

// complexityRole maps high complexity to a smell type for the role tag.
func complexityRole(complexity int) string {
	if complexity >= config.ComplexityVeryHigh {
		return "very_high_complexity"
	}
	if complexity >= config.ComplexityHigh {
		return "high_complexity"
	}
	return ""
}
