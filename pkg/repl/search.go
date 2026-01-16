package repl

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/agext/levenshtein"
)

// MatchResult represents a single search match result.
type MatchResult struct {
	Symbol string
	Score  float64
}

// FindNodesBySimilarity searches for nodes that are similar to the query string.
// It uses a combination of Levenshtein distance and Jaccard similarity.
func FindNodesBySimilarity(query string, symbols []string) []string {
	if query == "" || len(symbols) == 0 {
		return nil
	}

	queryLower := strings.ToLower(query)
	queryTokens := tokenize(queryLower)

	var results []MatchResult

	for _, symbol := range symbols {
		// Skip empty symbols
		if symbol == "" {
			continue
		}

		// Calculate similarity score
		score := calculateScore(queryLower, queryTokens, symbol)

		if score > 0.3 { // Threshold to filter out irrelevant results
			results = append(results, MatchResult{Symbol: symbol, Score: score})
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Return top 10
	limit := 10
	if len(results) < limit {
		limit = len(results)
	}

	topStrings := make([]string, limit)
	for i := 0; i < limit; i++ {
		topStrings[i] = results[i].Symbol
	}

	return topStrings
}

// calculateScore returns a similarity score between 0 and 1.
// It combines exact match, Levenshtein distance, and Jaccard similarity.
func calculateScore(queryLower string, queryTokens map[string]bool, symbol string) float64 {
	symbolLower := strings.ToLower(symbol)

	// 1. Exact match bonus
	if queryLower == symbolLower {
		return 1.0
	}
	if strings.Contains(symbolLower, queryLower) {
		return 0.95 // Substring match is very strong
	}

	// 2. Levenshtein Similarity (Global)
	// Good for when the user types the full path or near full path
	levDist := levenshtein.Distance(queryLower, symbolLower, nil)
	maxLen := float64(len(queryLower))
	if len(symbolLower) > int(maxLen) {
		maxLen = float64(len(symbolLower))
	}
	globalLevScore := 1.0 - (float64(levDist) / maxLen)
	if globalLevScore < 0 {
		globalLevScore = 0
	}

	// 3. Jaccard Similarity & Token-wise Levenshtein
	// This helps when the user types keywords "meb store" vs "pkg/meb/store.go"
	// or makes a typo in a keyword "storag" vs "store".
	symbolTokens := tokenize(symbolLower)

	// Calculate the best match for each query token against symbol tokens
	totalTokenScore := 0.0
	for qToken := range queryTokens {
		bestTokenScore := 0.0
		// Exact token match?
		if symbolTokens[qToken] {
			bestTokenScore = 1.0
		} else {
			// Find best fuzzy match for this query token among symbol tokens
			for sToken := range symbolTokens {
				// Normalized Levenshtein for tokens
				dist := levenshtein.Distance(qToken, sToken, nil)
				tMax := float64(len(qToken))
				if len(sToken) > int(tMax) {
					tMax = float64(len(sToken))
				}
				score := 1.0 - (float64(dist) / tMax)
				if score > bestTokenScore {
					bestTokenScore = score
				}
			}
		}
		totalTokenScore += bestTokenScore
	}

	// Average token score
	tokenScore := 0.0
	if len(queryTokens) > 0 {
		tokenScore = totalTokenScore / float64(len(queryTokens))
	}

	// Weighted Average
	// We prioritize the Token Score because it handles both "bag of words" (Jaccard-like)
	// and "fuzzy keywords" (Token-wise Levenshtein).
	// Global Levenshtein is mainly for "almost exact path" matches.

	finalScore := math.Max(globalLevScore, tokenScore)

	return finalScore
}

// tokenize splits a string into unique tokens.
// It handles camelCase, snake_case, and standard separators.
func tokenize(s string) map[string]bool {
	tokens := make(map[string]bool)
	var currentToken strings.Builder

	// Tokenize by non-alphanumeric chars
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) {
			if currentToken.Len() > 0 {
				token := strings.ToLower(currentToken.String())
				if len(token) > 2 { // Filter out very short noise tokens unless query is short
					tokens[token] = true
				} else if len(s) < 10 { // Keep short tokens for short strings
					tokens[token] = true
				}
				currentToken.Reset()
			}
		} else {
			// Handle camelCase: separate if uppercase
			if unicode.IsUpper(r) && currentToken.Len() > 0 {
				token := strings.ToLower(currentToken.String())
				tokens[token] = true
				currentToken.Reset()
			}
			currentToken.WriteRune(r)
		}
	}
	if currentToken.Len() > 0 {
		tokens[strings.ToLower(currentToken.String())] = true
	}
	return tokens
}

// calculateJaccard calculates Jaccard index between two sets of tokens.
// J(A, B) = |A ∩ B| / |A ∪ B|
func calculateJaccard(setA, setB map[string]bool) float64 {
	intersection := 0
	union := len(setA)

	for token := range setB {
		if setA[token] {
			intersection++
		} else {
			union++
		}
	}

	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}
