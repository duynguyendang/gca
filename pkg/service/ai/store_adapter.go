package ai

import (
	"fmt"

	"github.com/duynguyendang/gca/pkg/nlp"
	meblib "github.com/duynguyendang/meb"
)

type nlpStoreAdapter struct {
	store *meblib.MEBStore
}

func newNLPEntityStore(store *meblib.MEBStore) nlp.EntityStore {
	return &nlpStoreAdapter{store: store}
}

func (a *nlpStoreAdapter) LookupID(key string) (uint64, bool) {
	return a.store.LookupID(key)
}

func (a *nlpStoreAdapter) ScanFacts(subject, predicate, object string) [][3]string {
	var results [][3]string
	for fact, err := range a.store.Scan(subject, predicate, object) {
		if err != nil {
			continue
		}
		objStr := fmt.Sprintf("%v", fact.Object)
		results = append(results, [3]string{fact.Subject, fact.Predicate, objStr})
	}
	return results
}

func (a *nlpStoreAdapter) GetContentByKey(key string) ([]byte, error) {
	return a.store.GetContentByKey(key)
}
