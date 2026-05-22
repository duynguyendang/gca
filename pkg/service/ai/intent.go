package ai

import (
	"github.com/duynguyendang/gca/pkg/nlp/intent"
	nlptypes "github.com/duynguyendang/gca/pkg/nlp/types"
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

type IntentFeatures struct {
	HasQuestionWord   bool
	HasSymbol         bool
	QueryLength       int
	StructuralScore   float64
	DomainSpecificity float64
	CoOccurrenceBonus float64
}

type IntentResult struct {
	Intent     Intent
	Confidence float64
	Target     string
	Extracted  map[string]string
	Features   IntentFeatures
}

var defaultClassifier = intent.NewClassifier()

type nlpIntent = intent.Intent
type nlpFeatures = intent.IntentFeatures

func ClassifyIntent(query string) IntentResult {
	result := defaultClassifier.Classify(query)
	return toAIIntentResult(result)
}

func ClassifyIntentWithContext(query string, history []ConversationTurn) IntentResult {
	historyTypes := make([]*nlptypes.ConversationTurn, len(history))
	for i := range history {
		historyTypes[i] = &nlptypes.ConversationTurn{
			UserInput:    history[i].UserInput,
			Intent:       history[i].Intent,
			DatalogQuery: history[i].DatalogQuery,
			ResultCount:  history[i].ResultCount,
			Summary:      history[i].Summary,
			Timestamp:    history[i].Timestamp,
		}
	}
	result := defaultClassifier.ClassifyWithContext(query, historyTypes)
	return toAIIntentResult(result)
}

func GetDatalogTemplateForIntent(intent Intent, target string) string {
	return defaultClassifier.TemplateFor(nlpIntent(intent), target)
}

func toAIIntentResult(result intent.IntentResult) IntentResult {
	extracted := make(map[string]string)
	for k, v := range result.Extracted {
		extracted[k] = v
	}
	var targetStr string
	if result.ResolvedEntity != nil {
		targetStr = result.ResolvedEntity.Name
	}
	return IntentResult{
		Intent:     Intent(result.Intent),
		Confidence: result.Confidence,
		Target:     targetStr,
		Extracted:  extracted,
		Features:   IntentFeatures(result.Features),
	}
}

func buildConversationContext(history []ConversationTurn) string {
	historyTypes := make([]*nlptypes.ConversationTurn, len(history))
	for i := range history {
		historyTypes[i] = &nlptypes.ConversationTurn{
			UserInput:    history[i].UserInput,
			Intent:       history[i].Intent,
			DatalogQuery: history[i].DatalogQuery,
			ResultCount:  history[i].ResultCount,
			Summary:      history[i].Summary,
			Timestamp:    history[i].Timestamp,
		}
	}
	return defaultClassifier.BuildConversationContext(historyTypes)
}

func (r IntentResult) String() string {
	return string(r.Intent)
}