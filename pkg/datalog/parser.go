package datalog

import (
	"fmt"
	"strings"
)

// Atom represents a single unit in a Datalog query (e.g., triples(S, P, O) or neq(A, B)).
type Atom struct {
	Predicate string
	Args      []string
}

// Parse parses a Datalog query string which may contain multiple atoms.
// It supports standard predicates like 'triples', constraints like 'regex', and syntactic sugar like '!='.
func Parse(query string) ([]Atom, error) {
	query = strings.TrimSpace(query)
	// Handle "Head :- Body" syntax by taking Body (ignore Head as it's just the Goal)
	if idx := strings.Index(query, ":-"); idx != -1 {
		query = query[idx+2:]
	}
	query = strings.TrimSpace(query)
	// Remove trailing dot
	query = strings.TrimSuffix(query, ".")

	// Remove leading ? if present (common in some Datalog dialects)
	query = strings.TrimPrefix(query, "?")

	// Split by ')' or similar to identify atoms? No, split by typical delimiters.
	// We need a smart splitter that handles:
	// atom1(...), atom2(...)
	// atom1(...) , atom2(...)

	// Use SmartSplit to get top-level atoms considering commas and quotes.
	rawAtoms := SmartSplit(query)
	if len(rawAtoms) == 0 {
		return nil, fmt.Errorf("empty query")
	}

	var parsedAtoms []Atom
	for _, raw := range rawAtoms {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		// Handle syntactic sugar: A != B
		if strings.Contains(raw, "!=") {
			parts := strings.SplitN(raw, "!=", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid inequality format: %s", raw)
			}
			lhs := strings.TrimSpace(parts[0])
			rhs := strings.TrimSpace(parts[1])
			parsedAtoms = append(parsedAtoms, Atom{
				Predicate: "neq",
				Args:      []string{lhs, rhs},
			})
			continue
		}

		// Standard atom: Predicate(Args...)
		pred, args, err := parseAtomString(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse atom '%s': %w", raw, err)
		}
		parsedAtoms = append(parsedAtoms, Atom{
			Predicate: pred,
			Args:      args,
		})
	}

	return parsedAtoms, nil
}

// parseAtomString parses "predicate(arg1, arg2, ...)"
func parseAtomString(s string) (string, []string, error) {
	s = strings.TrimSpace(s)
	start := strings.Index(s, "(")
	end := strings.LastIndex(s, ")")

	if start == -1 || end == -1 || start >= end {
		return "", nil, fmt.Errorf("expected format 'predicate(args...)' but got '%s'", s)
	}

	predicate := strings.TrimSpace(s[:start])
	argsBody := s[start+1 : end]

	args := SmartSplit(argsBody)
	// Trim quotes from args for cleaner usage downstream, OR keep them?
	// The original implementation trimmed them in `parseArg`.
	// Ideally, the parser should keep structure, but for simplicity let's clean them here if they are purely string literals.
	// Actually, let's keep them raw here and let the evaluator decide, OR standardizing on stripping quotes for ease.
	// Given the previous helper `clean`, let's strip quotes to match previous behavior.
	cleanedArgs := make([]string, len(args))
	for i, arg := range args {
		cleanedArgs[i] = strings.TrimSpace(strings.ReplaceAll(arg, "\"", "'")) // normalize to single quotes or just strip?
		// Original 'clean' used ReplaceAll(s, "\"", "") -> stripped double quotes.
		// Let's strip both single and double quotes for consistency.
		cleanedArgs[i] = strings.Trim(cleanedArgs[i], "\"'")
	}

	return predicate, cleanedArgs, nil
}

// SmartSplit splits a string by comma, correctly handling quotes and parentheses.
// e.g. "a, b, 'c,d'" -> ["a", "b", "'c,d'"]
func SmartSplit(s string) []string {
	var results []string
	var current strings.Builder
	depth := 0
	inQuote := false
	var quoteChar rune

	for _, r := range s {
		switch r {
		case '"', '\'':
			if inQuote {
				if r == quoteChar {
					inQuote = false // Close quote
				}
			} else {
				inQuote = true
				quoteChar = r
			}
			current.WriteRune(r)
		case '(':
			if !inQuote {
				depth++
			}
			current.WriteRune(r)
		case ')':
			if !inQuote {
				depth--
			}
			current.WriteRune(r)
		case ',':
			if !inQuote && depth == 0 {
				results = append(results, strings.TrimSpace(current.String()))
				current.Reset()
				continue
			}
			current.WriteRune(r)
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		results = append(results, strings.TrimSpace(current.String()))
	}
	return results
}
