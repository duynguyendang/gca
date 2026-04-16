package ai

import (
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
}

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
	query = strings.ToLower(query)
	bestResult := IntentResult{
		Intent:     IntentChat,
		Confidence: 0.3,
		Extracted:  make(map[string]string),
	}

	for _, ip := range intentPatterns {
		for _, pattern := range ip.patterns {
			matches := findSubstringMatch(query, pattern)
			if matches != nil {
				confidence := ip.confidence
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

	if bestResult.Confidence < 0.5 {
		bestResult.Intent = IntentChat
		bestResult.Confidence = 0.5
	}

	return bestResult
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
