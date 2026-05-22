package types

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
	ResolvedEntity *Entity
	Entities      []*Entity
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

type Entity struct {
	ID         string
	Name       string
	EntityType string
	Source     string
}

type ConversationTurn struct {
	UserInput    string
	Intent       string
	DatalogQuery string
	ResultCount  int
	Summary      string
	Timestamp    int64
}

type Expansion struct {
	Pronoun    string
	Resolved   string
	Confidence float64
	Entity     *Entity
}

func (e *Entity) String() string {
	if e == nil {
		return ""
	}
	return e.Name
}