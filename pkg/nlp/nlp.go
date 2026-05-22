package nlp

import (
	"github.com/duynguyendang/gca/pkg/nlp/coref"
	"github.com/duynguyendang/gca/pkg/nlp/entity"
	"github.com/duynguyendang/gca/pkg/nlp/intent"
	"github.com/duynguyendang/gca/pkg/nlp/types"
)

type Config struct {
	EnableEntityResolution bool
	EnableCoreference      bool
}

func NewConfig() *Config {
	return &Config{
		EnableEntityResolution: true,
		EnableCoreference:     true,
	}
}

type NLPService struct {
	Classifier *intent.IntentClassifier
	Resolver   *entity.Resolver
	Expander   *coref.PronounExpander
}

type EntityStore interface {
	LookupID(key string) (uint64, bool)
	ScanFacts(subject, predicate, object string) [][3]string
	GetContentByKey(key string) ([]byte, error)
}

func NewNLPService(store EntityStore, cfg *Config) *NLPService {
	if cfg == nil {
		cfg = NewConfig()
	}

	resolver := entity.NewResolver(store)
	classifier := intent.NewClassifier()
	expander := coref.NewExpander(nil, nil)

	return &NLPService{
		Classifier: classifier,
		Resolver:   resolver,
		Expander:   expander,
	}
}

var DefaultConfig = NewConfig()

type Intent = types.Intent
type Entity = types.Entity
type IntentResult = types.IntentResult
type ConversationTurn = types.ConversationTurn
type Expansion = types.Expansion
type IntentFeatures = types.IntentFeatures

type QueryResult struct {
	OriginalQuery string
	ExpandedQuery string
	Intent        Intent
	Confidence    float64
	Entities      []*Entity
	Expansions    []Expansion
	Features      IntentFeatures
}

func (s *NLPService) ProcessQuery(query string, history []*ConversationTurn) *QueryResult {
	result := &QueryResult{
		OriginalQuery: query,
		ExpandedQuery: query,
	}

	if len(history) > 0 {
		s.Expander.SetHistory(history)
		expanded, corefExpansions := s.Expander.ExpandPronouns(query)
		if len(corefExpansions) > 0 {
			result.ExpandedQuery = expanded
			result.Expansions = make([]Expansion, len(corefExpansions))
			for i, e := range corefExpansions {
				result.Expansions[i] = Expansion(e)
			}
		}
	}

	if s.Resolver != nil {
		entities, err := s.Resolver.Resolve(result.ExpandedQuery, nil)
		if err == nil && len(entities) > 0 {
			result.Entities = entities
			s.Expander.SetEntities(entities)
		}
	}

	var features IntentFeatures
	if len(history) > 0 {
		classified := s.Classifier.ClassifyWithContext(result.ExpandedQuery, history)
		result.Intent = Intent(classified.Intent)
		result.Confidence = classified.Confidence
		features = intentFeaturesToTypes(classified.Features)
		if classified.ResolvedEntity != nil {
			result.Entities = append(result.Entities, classified.ResolvedEntity)
		}
	} else {
		classified := s.Classifier.Classify(result.ExpandedQuery)
		result.Intent = Intent(classified.Intent)
		result.Confidence = classified.Confidence
		features = intentFeaturesToTypes(classified.Features)
	}
	result.Features = features

	return result
}

func intentFeaturesToTypes(f intent.IntentFeatures) IntentFeatures {
	return IntentFeatures{
		HasQuestionWord:   f.HasQuestionWord,
		HasSymbol:         f.HasSymbol,
		QueryLength:       f.QueryLength,
		StructuralScore:   f.StructuralScore,
		DomainSpecificity: f.DomainSpecificity,
		CoOccurrenceBonus: f.CoOccurrenceBonus,
	}
}