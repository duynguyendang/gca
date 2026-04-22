package ai

import (
	"fmt"
	"regexp"
	"strings"
)

type Intent string

const (
	IntentWhoCalls    Intent = "who_calls"
	IntentWhatCalls   Intent = "what_calls"
	IntentHowReaches  Intent = "how_reaches"
	IntentSummarize   Intent = "summarize"
	IntentExplain     Intent = "explain"
	IntentFind        Intent = "find"
	IntentSecurity    Intent = "security_audit"
	IntentRefactor    Intent = "refactor"
	IntentTestGen     Intent = "test_generation"
	IntentPerformance Intent = "performance"
	IntentChat        Intent = "chat"
)

type IntentResult struct {
	Intent     Intent
	Confidence float64
	Target     string
	Extracted  map[string]string
	Features   IntentFeatures
}

type IntentFeatures struct {
	HasQuestionWord   bool
	HasSymbol          bool
	QueryLength        int
	StructuralScore    float64
	DomainSpecificity   float64
	CoOccurrenceBonus  float64
}

// intentPatternWithFeatures extends basic pattern with metadata for scoring
type intentPatternWithFeatures struct {
	intent            Intent
	patterns          []string
	confidence        float64
	specificityWeight float64 // 0-1, higher = more specific intent indicator
	domainIndicators  []string
}

// strongIntentIndicators are keywords that strongly indicate a specific intent
var strongIntentIndicators = map[Intent][]string{
	IntentSecurity:    {"sql injection", "xss", "csrf", "authentication", "authorization", "sql-inject", "sanitiz", "vulnerabilit", "audit"},
	IntentRefactor:    {"refactor", "technical debt", "code smell", "cyclic complexity", "coupling"},
	IntentTestGen:     {"unit test", "integration test", "test coverage", "write test", "generate test", "jest", "pytest", "go test"},
	IntentPerformance:  {"performance", "bottleneck", "optimize", "memory leak", "cpu", "latency", "slow query"},
}

// questionWords that indicate interrogative intent
var questionWords = []string{"who", "what", "where", "when", "why", "how", "which", "whose", "whom"}

// structuralIndicators for computing structural score
var structuralIndicators = []string{"?", "how do", "how does", "how can", "what is", "what are", "show me", "find all", "list"}

// symbolPattern matches common symbol formats (Package.Type, file/path, CamelCase)
var symbolPattern = regexp.MustCompile(`([A-Z][a-zA-Z0-9]*\.[A-Z][a-zA-Z0-9]*|[a-zA-Z0-9_]+/[a-zA-Z0-9_./]+|[A-Z][a-z]+[A-Z][a-zA-Z0-9]*)`)

var intentPatterns = []struct {
	intent     Intent
	patterns   []string
	confidence float64
}{
	{
		IntentWhoCalls,
		[]string{
			`who calls?\s+([\w.]+)`,
			`who invok(?:es|ed|ing)\s+([\w.]+)`,
			`who us(?:es|ed|ing)\s+([\w.]+)`,
			`callers? of\s+([\w.]+)`,
			`find (?:all |the )?callers? (?:of |for )?([\w.]+)`,
			`all (?:functions |methods )?that call\s+([\w.]+)`,
			`which (?:functions |methods )?call\s+([\w.]+)`,
			`who (?:can |could )?call\s+([\w.]+)`,
		},
		0.9,
	},
	{
		IntentWhatCalls,
		[]string{
			`what does\s+([\w.]+)\s+(?:call|invoke)`,
			`what (?:functions?|methods?|apis?)?\s+([\w.]+)\s+(?:call|use|invoke)`,
			`callees? of\s+([\w.]+)`,
			`(\w+)\s+calls?\s+(?:what|which)`,
			`what (?:does |is )?([\w.]+) (?:call|calls|invoke|invokes)`,
			`which (?:functions |methods )?(?:does |is )?([\w.]+) (?:call|calls|invoke)`,
		},
		0.85,
	},
	{
		IntentHowReaches,
		[]string{
			`how does\s+(\w+)\s+reach`,
			`how (?:can|could)\s+(\w+)\s+(?:get|call|reach)`,
			`path from\s+(\w+)\s+to\s+(\w+)`,
			`connection between\s+(\w+)\s+and\s+(\w+)`,
			`(?:find|show|trace) (?:the )?path`,
		},
		0.8,
	},
	{
		IntentSummarize,
		[]string{
			`summarize\s+(\w+)`,
			`summary of\s+(\w+)`,
			`overview of\s+(\w+)`,
			`(?:what is|what's)\s+(\w+)`,
			`describe\s+(\w+)`,
		},
		0.75,
	},
	{
		IntentExplain,
		[]string{
			`explain\s+([\w./]+)`,
			`how (?:does|do|is|was)\s+([\w./]+)`,
			`(?:tell|give) me (?:about|more info(?:rmation)? about)\s+([\w./]+)`,
			`(?:tell|show) me (?:how|what|why)`,
			`how (?:does |do |can )?([\w.]+) (?:work|works|function)`,
			`what (?:does |do )?([\w.]+) (?:do |does )`,
			`walk me through\s+([\w./]+)`,
			`tell me more about\s+([\w./]+)`,
		},
		0.7,
	},
	{
		IntentFind,
		[]string{
			`find\s+([\w./]+)`,
			`where is\s+([\w./]+)`,
			`where (?:does|do|is|are|was|were)\s+([\w./]+)`,
			`where (?:does|do|is)\s+([\w./]+)\s+(?:defined|located)`,
			`locate\s+([\w./]+)`,
			`search for\s+([\w./]+)`,
			`which (?:file |function |class )?(?:defines |contains |has )?([\w./]+)`,
			`find (?:where |all )?(?:the )?([\w./]+)`,
			`where can i find\s+([\w./]+)`,
		},
		0.75,
	},
	{
		IntentSecurity,
		[]string{
			`security`,
			`vulnerabilit`,
			`audit`,
			`injection`,
			`authent`,
			`authoriz`,
			`permission`,
			`access control`,
			`sanitiz`,
			`sql.?inject`,
			`xss`,
			`csrf`,
			`crypto`,
			`password`,
			`secret`,
			`api.?key`,
		},
		0.9,
	},
	{
		IntentRefactor,
		[]string{
			`refactor`,
			`improve`,
			`reorganiz`,
			`restructure`,
			`technical debt`,
			`simplif`,
		},
		0.85,
	},
	{
		IntentTestGen,
		[]string{
			`test`,
			`unit test`,
			`coverage`,
			`write.*test`,
			`generat.*test`,
		},
		0.85,
	},
	{
		IntentPerformance,
		[]string{
			`performance`,
			`speed`,
			`bottleneck`,
			`optimi`,
			`slow`,
			`memory leak`,
			`complexity`,
		},
		0.85,
	},
}

func ClassifyIntent(query string) IntentResult {
	queryLower := strings.ToLower(query)
	features := extractIntentFeatures(query, queryLower)

	bestResult := IntentResult{
		Intent:     IntentChat,
		Confidence: 0.3,
		Extracted:  make(map[string]string),
		Features:   features,
	}

	// Phase 1: Strong domain-specific indicators (highest priority)
	score := checkStrongIntentIndicators(queryLower, strongIntentIndicators)
	if score.Confidence > 0 {
		bestResult.Intent = score.Intent
		bestResult.Confidence = score.Confidence
		return bestResult
	}

	// Phase 2: Pattern matching with contextual boosting
	for _, ip := range intentPatterns {
		for _, pattern := range ip.patterns {
			matches := findSubstringMatch(query, pattern)
			if matches != nil {
				confidence := ip.confidence

				// Boost confidence if structural indicators present
				if features.StructuralScore > 0 {
					confidence += features.StructuralScore * 0.1
				}

				// Boost if query length is appropriate for the intent
				if features.QueryLength > 10 && features.QueryLength < 100 {
					confidence += 0.05
				}

				if len(matches) > 1 && matches[1] != "" {
					confidence += 0.1
					bestResult.Target = matches[1]
				}

				if confidence > bestResult.Confidence {
					bestResult.Intent = ip.intent
					bestResult.Confidence = confidence
					bestResult.Extracted = make(map[string]string)
					for i, m := range matches {
						if i > 0 {
							bestResult.Extracted[string(rune('a'+i-1))] = m
						}
					}
				}
			}
		}
	}

	// Phase 3: Symbol-based resolution boost
	if features.HasSymbol && features.QueryLength < 50 {
		bestResult.Confidence += 0.1
		if bestResult.Intent == IntentChat {
			bestResult.Intent = IntentFind
		}
	}

	// Phase 4: Question word bonus
	if features.HasQuestionWord {
		if bestResult.Intent == IntentChat && features.QueryLength < 80 {
			bestResult.Confidence = min(bestResult.Confidence+0.15, 0.85)
		}
	}

	// Phase 5: Co-occurrence bonus for intent switching based on context
	if features.CoOccurrenceBonus > 0 {
		bestResult.Confidence += features.CoOccurrenceBonus
	}

	// Fallback threshold
	if bestResult.Confidence < 0.5 {
		bestResult.Intent = IntentChat
		bestResult.Confidence = max(0.5, bestResult.Confidence)
	}

	return bestResult
}

func extractIntentFeatures(query, queryLower string) IntentFeatures {
	f := IntentFeatures{
		QueryLength: len(query),
	}

	// Check for question words
	for _, qw := range questionWords {
		if strings.HasPrefix(queryLower, qw) || strings.Contains(queryLower, " "+qw+" ") {
			f.HasQuestionWord = true
			break
		}
	}

	// Check for symbols
	if symbolPattern.MatchString(query) {
		f.HasSymbol = true
	}

	// Structural score: count structural indicators
	structuralMatches := 0
	for _, indicator := range structuralIndicators {
		if strings.Contains(queryLower, indicator) {
			structuralMatches++
		}
	}
	f.StructuralScore = float64(structuralMatches) / float64(len(structuralIndicators))

	// Domain specificity: shorter queries with technical terms = higher specificity
	technicalTerms := 0
	technicalCount := map[string]int{
		"function": 1, "method": 1, "class": 1, "interface": 1,
		"api": 1, "endpoint": 1, "route": 1, "handler": 1,
		"module": 1, "package": 1, "struct": 1, "enum": 1,
		"database": 1, "query": 1, "cache": 1, "middleware": 1,
	}
	for term, weight := range technicalCount {
		if strings.Contains(queryLower, term) {
			technicalTerms += weight
		}
	}
	f.DomainSpecificity = float64(technicalTerms) / max(float64(f.QueryLength)/50.0, 1.0)

	return f
}

func checkStrongIntentIndicators(query string, indicators map[Intent][]string) IntentResult {
	// Security check (highest priority for security-related queries)
	securityTerms := 0
	for _, term := range indicators[IntentSecurity] {
		if strings.Contains(query, term) {
			securityTerms++
		}
	}
	if securityTerms >= 2 {
		return IntentResult{
			Intent:     IntentSecurity,
			Confidence: 0.95,
			Extracted:  map[string]string{"security_terms": query},
		}
	}
	if securityTerms == 1 {
		return IntentResult{
			Intent:     IntentSecurity,
			Confidence: 0.85,
			Extracted:  map[string]string{"security_term": query},
		}
	}

	// Refactor check
	refactorTerms := 0
	for _, term := range indicators[IntentRefactor] {
		if strings.Contains(query, term) {
			refactorTerms++
		}
	}
	if refactorTerms >= 1 {
		return IntentResult{
			Intent:     IntentRefactor,
			Confidence: 0.90,
			Extracted:  map[string]string{"refactor_terms": query},
		}
	}

	// TestGen check
	testTerms := 0
	for _, term := range indicators[IntentTestGen] {
		if strings.Contains(query, term) {
			testTerms++
		}
	}
	if testTerms >= 2 {
		return IntentResult{
			Intent:     IntentTestGen,
			Confidence: 0.93,
			Extracted:  map[string]string{"test_terms": query},
		}
	}
	if testTerms == 1 && (strings.Contains(query, "test") || strings.Contains(query, "coverage")) {
		return IntentResult{
			Intent:     IntentTestGen,
			Confidence: 0.88,
		}
	}

	// Performance check
	perfTerms := 0
	for _, term := range indicators[IntentPerformance] {
		if strings.Contains(query, term) {
			perfTerms++
		}
	}
	if perfTerms >= 2 {
		return IntentResult{
			Intent:     IntentPerformance,
			Confidence: 0.92,
		}
	}
	if perfTerms == 1 {
		return IntentResult{
			Intent:     IntentPerformance,
			Confidence: 0.85,
		}
	}

	return IntentResult{Confidence: -1}
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// ClassifyIntentWithContext classifies intent while considering conversation history
func ClassifyIntentWithContext(query string, history []ConversationTurn) IntentResult {
	// Base classification
	result := ClassifyIntent(query)

	// Apply context-based refinements from conversation history
	if len(history) > 0 {
		result = refineFromHistory(result, query, history)
	}

	return result
}

// refineFromHistory applies contextual refinements based on conversation flow
func refineFromHistory(result IntentResult, query string, history []ConversationTurn) IntentResult {
	lastTurn := history[len(history)-1]

	// If user is following up (short query with pronouns), maintain context
	if isFollowUp(query) {
		// Boost confidence if we're continuing in same intent
		if string(result.Intent) == lastTurn.Intent {
			result.Confidence = min(result.Confidence+0.15, 0.95)
		}

		// Detect pronoun references to previous targets
		if pronounRef := detectPronounReference(query); pronounRef != "" {
			result.Target = pronounRef
			result.Extracted["pronoun_target"] = pronounRef
			result.Confidence += 0.1
		}
	}

	// Context continuity bonus: if we're doing sequential exploration
	if len(history) >= 2 {
		sequentialBonus := calculateSequentialBonus(result.Intent, history)
		result.Confidence += sequentialBonus
		result.Features.CoOccurrenceBonus = sequentialBonus
	}

	// Intent transition detection: user moving from explain to find
	if canInferTransition(result.Intent, query, history) {
		result.Intent = IntentFind
		result.Confidence = 0.75
	}

	return result
}

// isFollowUp detects if the query is a follow-up (short with contextual references)
func isFollowUp(query string) bool {
	shortFollowUp := []string{"it", "that", "them", "this", "there", "why", "how", "what", "and then", "also", "another", "more", "else"}
	queryLower := strings.ToLower(query)
	trimmed := strings.TrimSpace(query)

	// Short queries that likely reference previous context
	if len(trimmed) < 30 {
		for _, phrase := range shortFollowUp {
			if strings.HasPrefix(queryLower, phrase) {
				return true
			}
		}
	}

	// Check for implicit follow-up patterns
	followUpPatterns := []string{
		`^(and|but|so|then)\s+`,
		`^(why|how|what)\s+`,
		`^show\s+(me\s+)?(more|others?|another)`,
		`^(just|only)\s+(one|more|a\s+few)`,
	}
	for _, pattern := range followUpPatterns {
		if regexp.MustCompile(pattern).MatchString(queryLower) {
			return true
		}
	}

	return false
}

// detectPronounReference resolves pronouns to their antecedent from history
func detectPronounReference(query string) string {
	queryLower := strings.ToLower(query)

	// Pattern: query starts with pronoun or demonstrative
	pronounPatterns := []string{
		`^(it|this|that|them|those)\s+(is|was|are|were|can|does|doesn|should|would)`,
		`^(it|this|that)\s+(call|use|invoke|have|has|contain)`,
		`^(what|where)\s+(is|are|was)\s+(it|this|that)`,
	}

	for _, pattern := range pronounPatterns {
		if regexp.MustCompile(pattern).MatchString(queryLower) {
			return "previous_target"
		}
	}

	return ""
}

// calculateSequentialBonus detects if the current query continues a pattern
func calculateSequentialBonus(current Intent, history []ConversationTurn) float64 {
	if len(history) < 2 {
		return 0
	}

	// Count consecutive same-intent queries
	sameIntentCount := 0
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Intent == string(current) {
			sameIntentCount++
		} else {
			break
		}
	}

	// Sequential exploration bonus
	if sameIntentCount >= 2 {
		return 0.1 * float64(sameIntentCount)
	}

	// Complementary intent bonus (e.g., explain -> find)
	prev := Intent(history[len(history)-1].Intent)
	if isComplementaryIntent(prev, current) {
		return 0.08
	}

	return 0
}

// isComplementaryIntent detects natural intent transitions
func isComplementaryIntent(prev, curr Intent) bool {
	transitions := map[Intent][]Intent{
		IntentExplain:   {IntentFind, IntentWhoCalls, IntentWhatCalls},
		IntentSummarize: {IntentFind, IntentExplain},
		IntentFind:     {IntentExplain, IntentWhoCalls},
		IntentWhoCalls: {IntentFind, IntentExplain},
	}

	if candidates, ok := transitions[prev]; ok {
		for _, c := range candidates {
			if c == curr {
				return true
			}
		}
	}
	return false
}

// canInferTransition detects when user intent shifts based on query patterns
func canInferTransition(current Intent, query string, history []ConversationTurn) bool {
	queryLower := strings.ToLower(query)

	// "show me where" after "explain" -> likely wants to find
	if current == IntentExplain && strings.Contains(queryLower, "where") {
		return true
	}

	// "list all" after "what is" -> switch to find
	if current == IntentExplain && strings.Contains(queryLower, "list all") {
		return true
	}

	return false
}

// buildConversationContext creates a context summary for query generation
func buildConversationContext(history []ConversationTurn) string {
	if len(history) == 0 {
		return ""
	}

	var ctx strings.Builder
	ctx.WriteString("Previous conversation:\n")

	// Show last 3 turns max
	start := 0
	if len(history) > 3 {
		start = len(history) - 3
	}

	for i := start; i < len(history); i++ {
		turn := history[i]
		ctx.WriteString(fmt.Sprintf("- Q%d: %s (intent: %s, results: %d)\n",
			i+1, turn.UserInput, turn.Intent, turn.ResultCount))
	}

	return ctx.String()
}

func findSubstringMatch(s, pattern string) []string {
	singleWord := extractSingleWord(s)
	if singleWord != "" && containsWord(s, singleWord) {
		if isLikelyIntent(s, singleWord) {
			return []string{"", singleWord}
		}
	}

	for i := 0; i <= len(s)-3; i++ {
		end := i + 3
		for end <= len(s) && !isWordBoundary(s[end-1]) {
			end++
		}
		if end > i+3 {
			word := s[i : end-1]
			if isLikelySymbol(word) {
				return []string{"", word}
			}
		}
	}

	return nil
}

func extractSingleWord(s string) string {
	words := strings.Fields(s)
	if len(words) >= 2 {
		last := words[len(words)-1]
		last = strings.Trim(last, "?.!,:;")
		if len(last) > 2 && len(last) < 50 {
			return last
		}
	}
	return ""
}

func containsWord(s, word string) bool {
	return strings.Contains(s, word) || strings.Contains(s, strings.ToUpper(word))
}

func isLikelyIntent(s, word string) bool {
	intentWords := []string{"what", "who", "how", "where", "find", "show", "explain", "describe", "tell"}
	for _, iw := range intentWords {
		if strings.Contains(s, iw) {
			return true
		}
	}
	return len(word) > 4
}

func isLikelySymbol(word string) bool {
	if len(word) < 3 {
		return false
	}
	upperCount := 0
	for _, c := range word {
		if c >= 'A' && c <= 'Z' {
			upperCount++
		}
	}
	if upperCount > 1 {
		return true
	}
	if upperCount == 1 && len(word) > 5 {
		return true
	}
	return strings.Contains(word, "_") || strings.Contains(word, "/")
}

func isWordBoundary(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '/'
}

func (r IntentResult) String() string {
	return string(r.Intent)
}

func GetDatalogTemplateForIntent(intent Intent, target string) string {
	switch intent {
	case IntentWhoCalls:
		if target != "" {
			return `triples(?caller, "calls", "` + target + `")`
		}
		return `triples(?caller, "calls", ?callee)`
	case IntentWhatCalls:
		if target != "" {
			return `triples("` + target + `", "calls", ?callee)`
		}
		return `triples(?caller, "calls", ?callee)`
	case IntentHowReaches:
		return `{"tool": "find_path", "source": "?source", "target": "?target"}`
	case IntentSummarize:
		return `triples("?target", "defines", ?sym), triples("?target", "has_doc", ?doc)`
	case IntentExplain:
		return `triples("?target", "?pred", ?obj)`
	case IntentFind:
		return `triples(?s, "defines", ?sym), regex(?sym, "?target")`
	case IntentSecurity:
		return `triples(?s, "references", ?ref), regex(?ref, "password|token|secret|key")`
	case IntentRefactor:
		return `triples(?f, "defines", ?sym), triples(?sym, "has_doc", ?doc)`
	case IntentTestGen:
		return `triples(?f, "defines", ?sym)`
	case IntentPerformance:
		return `triples(?f, "defines", ?sym)`
	default:
		return `triples(?s, ?p, ?o)`
	}
}
