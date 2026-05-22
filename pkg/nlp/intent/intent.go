package intent

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/duynguyendang/gca/pkg/nlp/types"
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
	Intent        Intent
	Confidence    float64
	ResolvedEntity *types.Entity
	Entities      []*types.Entity
	Extracted     map[string]string
	Features      IntentFeatures
}

type IntentFeatures struct {
	HasQuestionWord   bool
	HasSymbol         bool
	QueryLength       int
	StructuralScore   float64
	DomainSpecificity float64
	CoOccurrenceBonus float64
}

type IntentClassifier struct {
	compiledPatterns     map[Intent][]*regexp.Regexp
	strongIndicators     map[Intent][]string
	questionWords        []string
	structuralIndicators []string
	symbolPattern        *regexp.Regexp
}

var IntentPatterns = []struct {
	Intent     Intent
	Patterns   []string
	Confidence float64
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

var strongIntentIndicators = map[Intent][]string{
	IntentSecurity:    {"sql injection", "xss", "csrf", "authentication", "authorization", "sql-inject", "sanitiz", "vulnerabilit", "audit"},
	IntentRefactor:    {"refactor", "technical debt", "code smell", "cyclic complexity", "coupling"},
	IntentTestGen:     {"unit test", "integration test", "test coverage", "write test", "generate test", "jest", "pytest", "go test"},
	IntentPerformance: {"performance", "bottleneck", "optimize", "memory leak", "cpu", "latency", "slow query"},
}

var questionWords = []string{"who", "what", "where", "when", "why", "how", "which", "whose", "whom"}

var structuralIndicators = []string{"?", "how do", "how does", "how can", "what is", "what are", "show me", "find all", "list"}

var symbolPattern = regexp.MustCompile(`([A-Z][a-zA-Z0-9]*\.[A-Z][a-zA-Z0-9]*|[a-zA-Z0-9_]+/[a-zA-Z0-9_./]+|[A-Z][a-z]+[A-Z][a-zA-Z0-9]*)`)

var followUpPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^(and|but|so|then)\s+`),
	regexp.MustCompile(`^(why|how|what)\s+`),
	regexp.MustCompile(`^show\s+(me\s+)?(more|others?|another)`),
	regexp.MustCompile(`^(just|only)\s+(one|more|a\s+few)`),
}

var pronounPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^(it|this|that|them|those)\s+(is|was|are|were|can|does|doesn|should|would)`),
	regexp.MustCompile(`^(it|this|that)\s+(call|use|invoke|have|has|contain)`),
	regexp.MustCompile(`^(what|where)\s+(is|are|was)\s+(it|this|that)`),
}

var historyFileRE = regexp.MustCompile(`[a-zA-Z0-9_./\\-]+\.go\b`)
var historySymRE = regexp.MustCompile(`[A-Z][a-zA-Z0-9]+(?:Service|Handler|Controller|Module|Manager|Client)`)
var historyPkgRE = regexp.MustCompile(`[a-z][a-zA-Z0-9]+/[a-z][a-zA-Z0-9]+`)

func NewClassifier() *IntentClassifier {
	return &IntentClassifier{
		compiledPatterns:     compilePatterns(),
		strongIndicators:     strongIntentIndicators,
		questionWords:        questionWords,
		structuralIndicators: structuralIndicators,
		symbolPattern:        symbolPattern,
	}
}

func compilePatterns() map[Intent][]*regexp.Regexp {
	result := make(map[Intent][]*regexp.Regexp)
	for _, p := range IntentPatterns {
		for _, pattern := range p.Patterns {
			re := regexp.MustCompile(pattern)
			result[p.Intent] = append(result[p.Intent], re)
		}
	}
	return result
}

func (ic *IntentClassifier) Classify(query string) IntentResult {
	queryLower := strings.ToLower(query)
	features := ic.extractIntentFeatures(query, queryLower)

	bestResult := IntentResult{
		Intent:     IntentChat,
		Confidence: 0.3,
		Extracted:  make(map[string]string),
		Features:   features,
	}

	score := ic.checkStrongIntentIndicators(queryLower, ic.strongIndicators)
	if score.Confidence > bestResult.Confidence {
		bestResult.Intent = score.Intent
		bestResult.Confidence = score.Confidence
		bestResult.Extracted = score.Extracted
	}

	for _, ip := range IntentPatterns {
		compiled := ic.compiledPatterns[ip.Intent]
		for _, re := range compiled {
			matches := ic.findSubstringMatch(queryLower, re)
			if matches != nil {
				confidence := ip.Confidence

				if features.StructuralScore > 0 {
					confidence += features.StructuralScore * 0.1
				}

				if features.QueryLength > 10 && features.QueryLength < 100 {
					confidence += 0.05
				}

				target := ""
				if len(matches) > 1 && matches[1] != "" {
					confidence += 0.1
					target = matches[1]
				}

				if confidence > bestResult.Confidence {
					bestResult.Intent = ip.Intent
					bestResult.Confidence = confidence
					bestResult.Extracted = make(map[string]string)
					if target != "" {
						bestResult.Extracted["target"] = target
					}
				}
			}
		}
	}

	if features.HasSymbol && features.QueryLength < 50 {
		bestResult.Confidence += 0.1
		if bestResult.Intent == IntentChat {
			bestResult.Intent = IntentFind
		}
	}

	if features.HasQuestionWord {
		if bestResult.Intent == IntentChat && features.QueryLength < 80 {
			bestResult.Confidence = minf(bestResult.Confidence+0.15, 0.85)
		}
	}

	if features.CoOccurrenceBonus > 0 {
		bestResult.Confidence += features.CoOccurrenceBonus
	}

	if bestResult.Confidence < 0.5 {
		bestResult.Intent = IntentChat
		bestResult.Confidence = maxf(0.5, bestResult.Confidence)
	}

	return bestResult
}

func (ic *IntentClassifier) ClassifyWithContext(query string, history []*types.ConversationTurn) IntentResult {
	result := ic.Classify(query)

	if len(history) > 0 {
		result = ic.refineFromHistory(result, query, history)
	}

	return result
}

func (ic *IntentClassifier) extractIntentFeatures(query, queryLower string) IntentFeatures {
	f := IntentFeatures{
		QueryLength: len(query),
	}

	for _, qw := range ic.questionWords {
		if strings.HasPrefix(queryLower, qw) || strings.Contains(queryLower, " "+qw+" ") {
			f.HasQuestionWord = true
			break
		}
	}

	if ic.symbolPattern.MatchString(query) {
		f.HasSymbol = true
	}

	structuralMatches := 0
	for _, indicator := range ic.structuralIndicators {
		if strings.Contains(queryLower, indicator) {
			structuralMatches++
		}
	}
	f.StructuralScore = float64(structuralMatches) / float64(len(ic.structuralIndicators))

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
	f.DomainSpecificity = float64(technicalTerms) / maxf(float64(f.QueryLength)/50.0, 1.0)

	return f
}

func (ic *IntentClassifier) checkStrongIntentIndicators(query string, indicators map[Intent][]string) IntentResult {
	intentChecks := []struct {
		intent       Intent
		minTerms     int
		confHigh     float64
		confLow      float64
		additional   func(string) bool
	}{
		{IntentSecurity, 1, 0.95, 0.85, nil},
		{IntentRefactor, 1, 0.90, 0, nil},
		{IntentTestGen, 2, 0.93, 0.88, func(q string) bool {
			return strings.Contains(q, "test") || strings.Contains(q, "coverage")
		}},
		{IntentPerformance, 2, 0.92, 0.85, nil},
	}

	bestResult := IntentResult{Confidence: -1}
	bestConf := -1.0

	for _, check := range intentChecks {
		matchedTerms := ic.countMatchingTerms(query, indicators[check.intent])
		conf := -1.0

		if matchedTerms >= check.minTerms {
			conf = check.confHigh
		} else if check.additional != nil && check.additional(query) && matchedTerms == 1 {
			conf = check.confLow
		}

		if conf > bestConf {
			bestConf = conf
			bestResult = IntentResult{
				Intent:     check.intent,
				Confidence: conf,
				Extracted:  map[string]string{strings.ToLower(string(check.intent)) + "_terms": query},
			}
		}
	}

	return bestResult
}

func (ic *IntentClassifier) countMatchingTerms(query string, terms []string) int {
	count := 0
	for _, term := range terms {
		if strings.Contains(query, term) {
			count++
		}
	}
	return count
}

func (ic *IntentClassifier) refineFromHistory(result IntentResult, query string, history []*types.ConversationTurn) IntentResult {
	lastTurn := history[len(history)-1]

	if ic.isFollowUp(query) {
		if pronounRef, entity := ic.detectPronounReferenceWithEntity(query, history); pronounRef != "" {
			result.ResolvedEntity = entity
			result.Extracted["pronoun_target"] = pronounRef
			result.Confidence += 0.1
			if result.Confidence < 0.7 && lastTurn.Intent != "" {
				result.Intent = Intent(lastTurn.Intent)
				result.Confidence = minf(0.75, result.Confidence+0.15)
			}
		} else if string(result.Intent) == lastTurn.Intent {
			result.Confidence = minf(result.Confidence+0.15, 0.95)
		}
	}

	if len(history) >= 2 {
		sequentialBonus := ic.calculateSequentialBonus(result.Intent, history)
		result.Confidence += sequentialBonus
		result.Features.CoOccurrenceBonus = sequentialBonus
	}

	if ic.canInferTransition(result.Intent, query, history) {
		result.Intent = IntentFind
		result.Confidence = 0.75
	}

	return result
}

func (ic *IntentClassifier) isFollowUp(query string) bool {
	shortFollowUp := []string{"it", "that", "them", "this", "there", "and then", "also", "another", "more", "else", "tell", "show"}
	queryLower := strings.ToLower(query)
	trimmed := strings.TrimSpace(query)

	if len(trimmed) < 30 {
		for _, phrase := range shortFollowUp {
			if strings.HasPrefix(queryLower, phrase) {
				return true
			}
		}
	}

	for _, pattern := range followUpPatterns {
		if pattern.MatchString(queryLower) {
			return true
		}
	}

	return false
}

func (ic *IntentClassifier) detectPronounReferenceWithEntity(query string, history []*types.ConversationTurn) (string, *types.Entity) {
	queryLower := strings.ToLower(query)

	for _, pattern := range pronounPatterns {
		if pattern.MatchString(queryLower) {
			if len(history) == 0 {
				return "previous_target", nil
			}

			last := history[len(history)-1]
			entity := extractEntityFromTurn(last)
			if entity != nil {
				return entity.Name, entity
			}

			return "previous_target", nil
		}
	}

	return "", nil
}

func extractEntityFromTurn(turn *types.ConversationTurn) *types.Entity {
	if turn == nil || turn.UserInput == "" {
		return nil
	}

	input := turn.UserInput

	if match := historyFileRE.FindString(input); match != "" {
		return &types.Entity{
			Name:       match,
			EntityType: "file",
			Source:     "history",
		}
	}

	if match := historySymRE.FindString(input); match != "" {
		return &types.Entity{
			Name:       match,
			EntityType: "symbol",
			Source:     "history",
		}
	}

	if match := historyPkgRE.FindString(input); match != "" {
		return &types.Entity{
			Name:       match,
			EntityType: "package",
			Source:     "history",
		}
	}

	return nil
}

func (ic *IntentClassifier) calculateSequentialBonus(current Intent, history []*types.ConversationTurn) float64 {
	if len(history) < 2 {
		return 0
	}

	sameIntentCount := 0
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Intent == string(current) {
			sameIntentCount++
		} else {
			break
		}
	}

	if sameIntentCount >= 2 {
		return 0.1 * float64(sameIntentCount)
	}

	prev := Intent(history[len(history)-1].Intent)
	if ic.isComplementaryIntent(prev, current) {
		return 0.08
	}

	return 0
}

func (ic *IntentClassifier) isComplementaryIntent(prev, curr Intent) bool {
	transitions := map[Intent][]Intent{
		IntentExplain:   {IntentFind, IntentWhoCalls, IntentWhatCalls},
		IntentSummarize: {IntentFind, IntentExplain},
		IntentFind:      {IntentExplain, IntentWhoCalls},
		IntentWhoCalls:  {IntentFind, IntentExplain},
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

func (ic *IntentClassifier) canInferTransition(current Intent, query string, history []*types.ConversationTurn) bool {
	queryLower := strings.ToLower(query)

	if current == IntentExplain && strings.Contains(queryLower, "where") {
		return true
	}

	if current == IntentExplain && strings.Contains(queryLower, "list all") {
		return true
	}

	return false
}

func (ic *IntentClassifier) findSubstringMatch(s string, re *regexp.Regexp) []string {
	matches := re.FindStringSubmatch(s)
	if len(matches) > 1 {
		return matches
	}

	singleWord := ic.extractSingleWord(s)
	if singleWord != "" && ic.isLikelyIntent(singleWord) {
		return []string{"", singleWord}
	}

	for i := 0; i <= len(s)-3; i++ {
		end := i + 3
		for end <= len(s) && !ic.isWordBoundary(s[end-1]) {
			end++
		}
		if end > i+3 {
			word := s[i : end-1]
			if ic.isLikelySymbol(word) {
				return []string{"", word}
			}
		}
	}

	return nil
}

func (ic *IntentClassifier) extractSingleWord(s string) string {
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

func (ic *IntentClassifier) isLikelyIntent(word string) bool {
	intentWords := []string{"what", "who", "how", "where", "find", "show", "explain", "describe", "tell"}
	wordLower := strings.ToLower(word)
	for _, iw := range intentWords {
		if wordLower == iw {
			return true
		}
	}
	return false
}

func (ic *IntentClassifier) isLikelySymbol(word string) bool {
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

func (ic *IntentClassifier) isWordBoundary(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '/'
}

func (ic *IntentClassifier) BuildConversationContext(history []*types.ConversationTurn) string {
	if len(history) == 0 {
		return ""
	}

	var ctx strings.Builder
	ctx.WriteString("Previous conversation:\n")

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

func (ic *IntentClassifier) TemplateFor(intent Intent, target string) string {
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

func (r IntentResult) String() string {
	return string(r.Intent)
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}