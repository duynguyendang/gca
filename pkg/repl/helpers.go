package repl

import (
	"fmt"
	"strings"
)

// Helper: Format predicates list for prompt
func formatPredicatesListSection(facts []string) string {
	var sb strings.Builder
	for _, p := range facts {
		sb.WriteString(fmt.Sprintf("- %s\n", p))
	}
	return sb.String()
}
