package meb

import (
	"context"
	"fmt"

	"github.com/duynguyendang/gca/pkg/datalog"
	"github.com/duynguyendang/meb"
)

type Store struct {
	*meb.MEBStore
}

func NewStore(db *meb.MEBStore) *Store {
	return &Store{db}
}

func Query(ctx context.Context, store *meb.MEBStore, query string) ([]map[string]any, error) {
	atoms, err := datalog.Parse(query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	if len(atoms) == 0 {
		return nil, fmt.Errorf("empty query")
	}

	var results []map[string]any

	switch atoms[0].Predicate {
	case "triples":
		results = executeTriplesQuery(ctx, store, atoms)
	default:
		results = executeTriplesQuery(ctx, store, atoms)
	}

	return results, nil
}

func (s *Store) Query(ctx context.Context, query string) ([]map[string]any, error) {
	return Query(ctx, s.MEBStore, query)
}

func executeTriplesQuery(ctx context.Context, store *meb.MEBStore, atoms []datalog.Atom) []map[string]any {
	var results []map[string]any

	for _, atom := range atoms {
		if atom.Predicate != "triples" {
			continue
		}

		if len(atom.Args) < 3 {
			continue
		}

		subj := resolveArg(atom.Args[0])
		pred := resolveArg(atom.Args[1])
		obj := resolveArg(atom.Args[2])

		for fact, err := range store.Scan(subj, pred, obj) {
			if err != nil {
				continue
			}

			result := make(map[string]any)
			if atom.Args[0][0] == '?' {
				result[atom.Args[0]] = fact.Subject
			}
			if atom.Args[1][0] == '?' {
				result[atom.Args[1]] = fact.Predicate
			}
			if atom.Args[2][0] == '?' {
				result[atom.Args[2]] = fact.Object
			}

			if len(result) > 0 {
				results = append(results, result)
			}
		}
	}

	return results
}

func resolveArg(arg string) string {
	if len(arg) >= 2 && arg[0] == '"' && arg[len(arg)-1] == '"' {
		return arg[1 : len(arg)-1]
	}
	if len(arg) >= 2 && arg[0] == '?' {
		return ""
	}
	return arg
}
