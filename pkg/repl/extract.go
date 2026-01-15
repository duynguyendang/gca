package repl

import (
	"regexp"
	"strings"
)

// extractSuggestedQueries extracts suggested follow-up queries from an AI explanation.
// It looks for common patterns like "Suggested Follow-up Queries:" or numbered lists after
// certain keywords.
func extractSuggestedQueries(explanation string) string {
	// Pattern 1: Look for "Suggested" section followed by numbered or bulleted lists
	suggestedPattern := regexp.MustCompile(`(?i)### Suggested.*?(?:Queries|Follow.*?up).*?\n((?:.*?\n)*?)(?:\n\n|$)`)
	matches := suggestedPattern.FindStringSubmatch(explanation)

	if len(matches) > 1 {
		// Clean up the extracted section
		section := matches[1]
		lines := strings.Split(section, "\n")
		var queries []string

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Extract the actual query text
			// Remove numbered/bulleted prefixes like "1. ", "- ", "* "
			line = regexp.MustCompile(`^[\d]+\.?\s*`).ReplaceAllString(line, "")
			line = regexp.MustCompile(`^[-*•]\s*`).ReplaceAllString(line, "")

			// Remove markdown code blocks
			line = strings.ReplaceAll(line, "```", "")
			line = strings.ReplaceAll(line, "`", "")

			// Skip lines that are just explanations (contain "Goal:", "Datalog:", etc.)
			if strings.Contains(line, "Goal:") || strings.Contains(line, "Datalog:") {
				continue
			}

			// Extract quoted suggestions
			quotePattern := regexp.MustCompile(`"([^"]+)"`)
			quoteMatches := quotePattern.FindAllStringSubmatch(line, -1)
			for _, qm := range quoteMatches {
				if len(qm) > 1 {
					queries = append(queries, qm[1])
				}
			}
		}

		if len(queries) > 0 {
			return strings.Join(queries, "\n")
		}
	}

	// Pattern 2: Look for any quoted suggestions throughout the text
	quotePattern := regexp.MustCompile(`(?:try|suggest|could|filter|isolate|trace|identify).*?"([^"]+)"`)
	allMatches := quotePattern.FindAllStringSubmatch(explanation, -1)

	var queries []string
	for _, match := range allMatches {
		if len(match) > 1 {
			queries = append(queries, match[1])
		}
	}

	if len(queries) > 0 {
		return strings.Join(queries, "\n")
	}

	return ""
}
