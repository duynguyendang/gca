package ingest

import (
	"context"
	"iter"

	"github.com/duynguyendang/meb"
)

type MockStore struct {
	Facts     []meb.Fact
	Documents map[string][]byte
	Topics    map[uint32]bool
}

func NewMockStore() *MockStore {
	return &MockStore{
		Facts:     make([]meb.Fact, 0),
		Documents: make(map[string][]byte),
		Topics:    make(map[uint32]bool),
	}
}

func (m *MockStore) AddFact(fact meb.Fact) error {
	m.Facts = append(m.Facts, fact)
	return nil
}

func (m *MockStore) AddFactBatch(facts []meb.Fact) error {
	m.Facts = append(m.Facts, facts...)
	return nil
}

func (m *MockStore) DeleteFactsBySubject(subject string) error {
	newFacts := make([]meb.Fact, 0)
	for _, f := range m.Facts {
		if f.Subject != subject {
			newFacts = append(newFacts, f)
		}
	}
	m.Facts = newFacts
	return nil
}

func (m *MockStore) Scan(subject, predicate, object string) iter.Seq2[meb.Fact, error] {
	return func(yield func(meb.Fact, error) bool) {
		for _, f := range m.Facts {
			if subject != "" && f.Subject != subject {
				continue
			}
			if predicate != "" && f.Predicate != predicate {
				continue
			}
			if object != "" {
				if objStr, ok := f.Object.(string); ok {
					if objStr != object {
						continue
					}
				} else {
					continue
				}
			}
			if !yield(f, nil) {
				return
			}
		}
	}
}

func (m *MockStore) ScanContext(ctx context.Context, subject, predicate, object string) iter.Seq2[meb.Fact, error] {
	return m.Scan(subject, predicate, object)
}

func (m *MockStore) GetContentByKey(key string) ([]byte, error) {
	if content, ok := m.Documents[key]; ok {
		return content, nil
	}
	return nil, nil
}

func (m *MockStore) SetTopicID(id uint32) {
	m.Topics[id] = true
}

type MockStoreManager struct {
	SourceStore     *MockStore
	AnalyticalStore *MockStore
	SourceErr       error
	AnalyticalErr   error
}

func NewMockStoreManager() *MockStoreManager {
	return &MockStoreManager{
		SourceStore:     NewMockStore(),
		AnalyticalStore: NewMockStore(),
	}
}

func (m *MockStoreManager) GetSourceStore(projectID string) (*meb.MEBStore, error) {
	if m.SourceErr != nil {
		return nil, m.SourceErr
	}
	return nil, nil
}

func (m *MockStoreManager) GetAnalyticalStore(projectID string) (*meb.MEBStore, error) {
	if m.AnalyticalErr != nil {
		return nil, m.AnalyticalErr
	}
	return nil, nil
}

type MockTemplateStore struct {
	Templates []*TemplateStoreQuery
	Err       error
}

func NewMockTemplateStore() *MockTemplateStore {
	return &MockTemplateStore{
		Templates: make([]*TemplateStoreQuery, 0),
	}
}

func (m *MockTemplateStore) ListTemplates(ctx context.Context, projectID, category string) ([]*TemplateStoreQuery, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Templates, nil
}

func (m *MockTemplateStore) GetTemplate(ctx context.Context, projectID, templateID string) (*TemplateStoreQuery, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	for _, t := range m.Templates {
		if t.ID == templateID {
			return t, nil
		}
	}
	return nil, nil
}
