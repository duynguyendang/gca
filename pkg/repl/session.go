package repl

// SessionContext maintains conversational state across queries.
type SessionContext struct {
	// The original natural language question from the user
	LastNLQuery string

	// The generated Datalog query
	LastDatalog string

	// Raw results from MEB (may be truncated for large sets)
	LastResults []map[string]any

	// Structured summary for sending to Gemini
	ResultSummary *ResultSummary

	// Multi-turn conversation history for advanced refinement
	ConversationHistory []ConversationTurn
}

// ConversationTurn represents a single query-response cycle.
type ConversationTurn struct {
	UserInput        string
	NLQuery          string // Empty if direct Datalog
	DatalogQuery     string
	ResultCount      int
	Explanation      string // AI-generated explanation
	SuggestedQueries string // AI-generated follow-up suggestions
}

// NewSessionContext creates a new session context.
func NewSessionContext() *SessionContext {
	return &SessionContext{
		ConversationHistory: make([]ConversationTurn, 0),
	}
}

// UpdateContext updates the session with the latest query results.
func (s *SessionContext) UpdateContext(nlQuery, datalog string, results []map[string]any, summary *ResultSummary) {
	s.LastNLQuery = nlQuery
	s.LastDatalog = datalog
	s.LastResults = results
	s.ResultSummary = summary
}

// AddTurn appends a conversation turn to the history.
func (s *SessionContext) AddTurn(turn ConversationTurn) {
	s.ConversationHistory = append(s.ConversationHistory, turn)

	// Keep only last 5 turns to manage memory and token costs
	if len(s.ConversationHistory) > 5 {
		s.ConversationHistory = s.ConversationHistory[len(s.ConversationHistory)-5:]
	}
}

// HasContext returns true if there is previous query context.
func (s *SessionContext) HasContext() bool {
	return s.LastNLQuery != "" || s.LastDatalog != ""
}

// GetLastSuggestions returns the suggested queries from the most recent turn.
func (s *SessionContext) GetLastSuggestions() string {
	if len(s.ConversationHistory) == 0 {
		return ""
	}
	return s.ConversationHistory[len(s.ConversationHistory)-1].SuggestedQueries
}
