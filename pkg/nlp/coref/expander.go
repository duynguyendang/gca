package coref

import (
	"regexp"
	"strings"

	"github.com/duynguyendang/gca/pkg/nlp/types"
)

var (
	pronounRE1 = regexp.MustCompile(`^(it|this|that|them|those)\s+(is|was|are|were|can|does|doesn|should|would)`)
	pronounRE2 = regexp.MustCompile(`^(it|this|that)\s+(call|use|invoke|have|has|contain)`)
	pronounRE3 = regexp.MustCompile(`^(what|where)\s+(is|are|was)\s+(it|this|that)`)
	pronounRE4 = regexp.MustCompile(`^(they|them|those)\s+(are|were|can|do)`)
	pronounRE5 = regexp.MustCompile(`^(he|she)\s+(is|was|can|does)`)
	pronounRE6 = regexp.MustCompile(`^(we)\s+(are|can|should|need)`)

	pronounPatterns = []*regexp.Regexp{
		pronounRE1, pronounRE2, pronounRE3, pronounRE4, pronounRE5, pronounRE6,
	}

	historyFileRE = regexp.MustCompile(`[a-zA-Z0-9_/]+\.go\b`)
	historySymRE  = regexp.MustCompile(`[A-Z][a-zA-Z0-9]+(?:Service|Handler|Controller|Module)`)

	pronounWordREs map[string]*regexp.Regexp
)

func init() {
	pronounWordREs = make(map[string]*regexp.Regexp)
	for _, p := range []string{"it", "this", "that", "them", "those", "he", "she", "we", "us"} {
		pronounWordREs[p] = regexp.MustCompile(`\b` + regexp.QuoteMeta(p) + `\b`)
	}
}

type PronounExpander struct {
	history  []*types.ConversationTurn
	entities []*types.Entity

	pronounPatterns []*regexp.Regexp
	demonstratives  []string
	personalPronouns []string
}

func NewExpander(history []*types.ConversationTurn, entities []*types.Entity) *PronounExpander {
	return &PronounExpander{
		history:          history,
		entities:         entities,
		pronounPatterns:  pronounPatterns,
		demonstratives:   []string{"this", "that", "these", "those"},
		personalPronouns: []string{"it", "he", "she", "they", "them", "we", "us"},
	}
}

type Expansion struct {
	Pronoun    string
	Resolved   string
	Confidence float64
	Entity     *types.Entity
}

func (e *PronounExpander) ExpandPronouns(query string) (string, []Expansion) {
	if query == "" {
		return query, nil
	}

	expansions := []Expansion{}

	for _, pattern := range e.pronounPatterns {
		if pattern.MatchString(strings.ToLower(query)) {
			if expansion := e.resolveToEntity(query); expansion != nil {
				expansions = append(expansions, *expansion)
				query = e.replacePronounWord(query, expansion.Pronoun, expansion.Resolved)
			}
		}
	}

	return query, expansions
}

func (e *PronounExpander) resolveToEntity(query string) *Expansion {
	queryLower := strings.ToLower(query)
	words := strings.Fields(queryLower)

	if len(words) == 0 {
		return nil
	}

	firstWord := words[0]

	if !e.isPronoun(firstWord) {
		return nil
	}

	if entity := e.findInHistory(); entity != nil {
		return &Expansion{
			Pronoun:    firstWord,
			Resolved:   entity.Name,
			Confidence: 0.85,
			Entity:     entity,
		}
	}

	if entity := e.findInEntities(); entity != nil {
		return &Expansion{
			Pronoun:    firstWord,
			Resolved:   entity.Name,
			Confidence: 0.9,
			Entity:     entity,
		}
	}

	return nil
}

func (e *PronounExpander) isPronoun(word string) bool {
	for _, p := range e.personalPronouns {
		if p == word {
			return true
		}
	}
	for _, d := range e.demonstratives {
		if d == word {
			return true
		}
	}
	return false
}

func (e *PronounExpander) findInHistory() *types.Entity {
	if len(e.history) == 0 {
		return nil
	}

	last := e.history[len(e.history)-1]

	patterns := []struct {
		pattern *regexp.Regexp
		resolve func(string) *types.Entity
	}{
		{
			historyFileRE,
			func(s string) *types.Entity { return &types.Entity{Name: s, EntityType: "file", Source: "history"} },
		},
		{
			historySymRE,
			func(s string) *types.Entity { return &types.Entity{Name: s, EntityType: "symbol", Source: "history"} },
		},
	}

	for _, p := range patterns {
		if matches := p.pattern.FindString(last.UserInput); matches != "" {
			return p.resolve(matches)
		}
	}

	return nil
}

func (e *PronounExpander) findInEntities() *types.Entity {
	if len(e.entities) == 0 {
		return nil
	}

	for _, entity := range e.entities {
		if entity != nil && entity.Name != "" {
			return entity
		}
	}

	return nil
}

func (e *PronounExpander) SetHistory(history []*types.ConversationTurn) {
	e.history = history
}

func (e *PronounExpander) SetEntities(entities []*types.Entity) {
	e.entities = entities
}

func (e *PronounExpander) replacePronounWord(query, pronoun, resolved string) string {
	if re, ok := pronounWordREs[pronoun]; ok {
		return re.ReplaceAllString(query, resolved)
	}
	return query
}