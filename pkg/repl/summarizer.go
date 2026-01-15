package repl

import (
	"fmt"
	"sort"
)

const (
	// MaxSampleResults limits the number of sample results sent to Gemini
	MaxSampleResults = 20

	// MaxFrequentItems limits the number of top predicates/subjects tracked
	MaxFrequentItems = 10

	// MaxResultStringLength truncates individual result strings to save tokens
	MaxResultStringLength = 100

	// LargeResultThreshold determines when to summarize vs send all results
	LargeResultThreshold = 50
)

// ResultSummary provides a structured summary of query results.
type ResultSummary struct {
	TotalCount         int
	SampleResults      []string
	FrequentPredicates map[string]int
	FrequentSubjects   map[string]int
	IsTruncated        bool
}

// SummarizeResults creates an intelligent summary of query results.
func SummarizeResults(results []map[string]any) *ResultSummary {
	summary := &ResultSummary{
		TotalCount:         len(results),
		FrequentPredicates: make(map[string]int),
		FrequentSubjects:   make(map[string]int),
		IsTruncated:        len(results) >= LargeResultThreshold,
	}

	// Determine how many results to sample
	sampleSize := len(results)
	if summary.IsTruncated {
		sampleSize = MaxSampleResults
	}

	// Extract samples and count frequencies
	summary.SampleResults = make([]string, 0, sampleSize)

	for i := 0; i < sampleSize && i < len(results); i++ {
		result := results[i]

		// Format result as a readable string
		resultStr := formatResult(result)
		if len(resultStr) > MaxResultStringLength {
			resultStr = resultStr[:MaxResultStringLength] + "..."
		}
		summary.SampleResults = append(summary.SampleResults, resultStr)

		// Count predicates and subjects for structural analysis
		if pred, ok := result["P"]; ok {
			if predStr, isStr := pred.(string); isStr {
				summary.FrequentPredicates[predStr]++
			}
		}
		if subj, ok := result["S"]; ok {
			if subjStr, isStr := subj.(string); isStr {
				summary.FrequentSubjects[subjStr]++
			}
		}
	}

	// For large result sets, analyze all results for frequency patterns
	if summary.IsTruncated {
		for i := sampleSize; i < len(results); i++ {
			result := results[i]
			if pred, ok := result["P"]; ok {
				if predStr, isStr := pred.(string); isStr {
					summary.FrequentPredicates[predStr]++
				}
			}
			if subj, ok := result["S"]; ok {
				if subjStr, isStr := subj.(string); isStr {
					summary.FrequentSubjects[subjStr]++
				}
			}
		}

		// Keep only top N most frequent items
		summary.FrequentPredicates = topN(summary.FrequentPredicates, MaxFrequentItems)
		summary.FrequentSubjects = topN(summary.FrequentSubjects, MaxFrequentItems)
	}

	return summary
}

// formatResult converts a result map into a readable string.
func formatResult(result map[string]any) string {
	// Try to format as triple if S, P, O are present
	if s, hasS := result["S"]; hasS {
		if p, hasP := result["P"]; hasP {
			if o, hasO := result["O"]; hasO {
				return fmt.Sprintf("(%s, %s, %s)", s, p, o)
			}
		}
	}

	// Otherwise, format as key-value pairs
	str := "{"
	first := true
	for k, v := range result {
		if !first {
			str += ", "
		}
		str += fmt.Sprintf("%s: %s", k, v)
		first = false
	}
	str += "}"
	return str
}

// topN returns the top N items by count from a frequency map.
func topN(freq map[string]int, n int) map[string]int {
	type kv struct {
		Key   string
		Value int
	}

	// Convert to slice for sorting
	var pairs []kv
	for k, v := range freq {
		pairs = append(pairs, kv{k, v})
	}

	// Sort by count descending
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].Value > pairs[j].Value
	})

	// Take top N
	result := make(map[string]int)
	for i := 0; i < n && i < len(pairs); i++ {
		result[pairs[i].Key] = pairs[i].Value
	}

	return result
}
