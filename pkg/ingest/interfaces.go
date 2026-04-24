package ingest

import (
	"context"
	"iter"

	"github.com/duynguyendang/meb"
)

type Store interface {
	AddFact(fact meb.Fact) error
	AddFactBatch(facts []meb.Fact) error
	DeleteFactsBySubject(subject string) error
	Scan(subject, predicate, object string) iter.Seq2[meb.Fact, error]
	ScanContext(ctx context.Context, subject, predicate, object string) iter.Seq2[meb.Fact, error]
	GetContentByKey(key string) ([]byte, error)
	SetTopicID(id uint32)
}

type StoreManager interface {
	GetSourceStore(projectID string) (*meb.MEBStore, error)
	GetAnalyticalStore(projectID string) (*meb.MEBStore, error)
}

type TemplateStore interface {
	ListTemplates(ctx context.Context, projectID, category string) ([]*TemplateStoreQuery, error)
	GetTemplate(ctx context.Context, projectID, templateID string) (*TemplateStoreQuery, error)
}

type FactScanner interface {
	Scan(subject, predicate, object string) iter.Seq2[meb.Fact, error]
	ScanContext(ctx context.Context, subject, predicate, object string) iter.Seq2[meb.Fact, error]
}
