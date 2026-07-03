package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/duynguyendang/gca/pkg/common/errors"
	"github.com/duynguyendang/gca/pkg/datalog"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
)

var queryOptimizer = datalog.NewQueryOptimizer()

// ExecuteQuery executes a Datalog query and returns results.
func (s *GraphService) ExecuteQuery(ctx context.Context, projectID, query string) ([]map[string]any, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	results, err := gcamdb.Query(ctx, store, query)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errors.ErrInvalidInput, err)
	}

	return results, nil
}

// ExecuteQueryOptimized executes a Datalog query with optimization (join reordering and predicate pushdown).
func (s *GraphService) ExecuteQueryOptimized(ctx context.Context, projectID, query string) ([]map[string]any, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	// Parse the query
	atoms, err := datalog.Parse(query)
	if err != nil {
		return nil, err
	}

	// Create optimized execution plan
	plan := queryOptimizer.CreateExecutionPlan(atoms)

	// Reconstruct the optimized query string
	optimizedQuery := reconstructQuery(plan.Atoms)

	// Execute the optimized query
	results, err := gcamdb.Query(ctx, store, optimizedQuery)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errors.ErrInvalidInput, err)
	}

	// Apply any pushed-down predicates as post-processing filters
	if len(plan.PushdownPreds) > 0 {
		results = applyPushdownPredicates(results, plan.PushdownPreds)
	}

	return results, nil
}

// reconstructQuery reconstructs a Datalog query string from a list of atoms.
func reconstructQuery(atoms []datalog.Atom) string {
	var queryParts []string
	for _, atom := range atoms {
		queryParts = append(queryParts, formatAtom(atom))
	}
	return strings.Join(queryParts, ", ")
}

// formatAtom formats an atom back into Datalog syntax.
func formatAtom(atom datalog.Atom) string {
	args := strings.Join(atom.Args, ", ")
	return fmt.Sprintf("%s(%s)", atom.Predicate, args)
}

// applyPushdownPredicates applies pushed-down predicates as post-processing filters.
func applyPushdownPredicates(results []map[string]any, predicates map[string]string) []map[string]any {
	if len(predicates) == 0 {
		return results
	}

	filtered := make([]map[string]any, 0, len(results))

	for _, result := range results {
		if matchesPushdownPredicates(result, predicates) {
			filtered = append(filtered, result)
		}
	}

	return filtered
}

// matchesPushdownPredicates checks if a result matches all pushed-down predicates.
func matchesPushdownPredicates(result map[string]any, predicates map[string]string) bool {
	for varName, constraint := range predicates {
		value, ok := result[varName]
		if !ok {
			return false
		}

		valueStr := fmt.Sprintf("%v", value)

		// Handle different constraint types
		if strings.HasPrefix(constraint, "neq:") {
			// Not equals constraint
			expectedValue := strings.TrimPrefix(constraint, "neq:")
			if valueStr == expectedValue {
				return false
			}
		} else {
			// Equals constraint
			if valueStr != constraint {
				return false
			}
		}
	}
	return true
}
