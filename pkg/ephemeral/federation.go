package ephemeral

import (
	"context"
	"errors"
	"fmt"

	gcamdb "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/meb"

	"github.com/duynguyendang/gca/pkg/logger"
)

// StoreAccessor provides access to the Source and Analytical stores for federated queries.
type StoreAccessor interface {
	GetSourceStore(projectID string) (*meb.MEBStore, error)
	GetAnalyticalStore(projectID string) (*meb.MEBStore, error)
}

// FederatedQueryRequest describes a query across ephemeral + source + analytical stores.
type FederatedQueryRequest struct {
	SessionID string `json:"session_id"`
	ProjectID string `json:"project_id"`
	Query     string `json:"query"`
}

// IsNotFoundError returns true if err indicates a missing or expired session.
func IsNotFoundError(err error) bool {
	return errors.Is(err, ErrSessionNotFound) || errors.Is(err, ErrSessionExpired)
}

// FederatedQueryResult holds results from each store.
type FederatedQueryResult struct {
	Ephemeral  []map[string]any `json:"ephemeral"`
	Source     []map[string]any `json:"source"`
	Analytical []map[string]any `json:"analytical"`
	TotalFacts int              `json:"total_facts"`
}

// FederatedQuery executes a Datalog query across all three stores
// (ephemeral diff facts, source store, analytical store) and merges the results.
func FederatedQuery(
	ctx context.Context,
	req FederatedQueryRequest,
	es *EphemeralStore,
	sa StoreAccessor,
) (*FederatedQueryResult, error) {
	session, err := es.GetSession(req.SessionID)
	if err != nil {
		if errors.Is(err, ErrSessionExpired) {
			return nil, err
		}
		return nil, ErrSessionNotFound
	}
	session.ExtendTTL(defaultSessionTTL)

	result := &FederatedQueryResult{}

	// 1. Query Ephemeral Store (diff facts)
	ephemeralResults, ephemeralErr := gcamdb.Query(ctx, session.Facts, req.Query)
	if ephemeralErr != nil {
		logger.Warn("ephemeral store query failed", "session_id", req.SessionID, "error", ephemeralErr)
	}

	// 2. Query Source Store
	sourceStore, srcErr := sa.GetSourceStore(req.ProjectID)
	if srcErr != nil {
		logger.Warn("source store not available", "project", req.ProjectID, "error", srcErr)
	}

	// 3. Query Analytical Store
	analyticalStore, anaErr := sa.GetAnalyticalStore(req.ProjectID)
	if anaErr != nil {
		logger.Warn("analytical store not available", "project", req.ProjectID, "error", anaErr)
	}

	// Only return results if at least one store returned data.
	// If ephemeral query itself failed, propagate that error.
	if ephemeralErr != nil && (srcErr != nil || analyticalStore == nil) && (anaErr != nil || sourceStore == nil) {
		return nil, fmt.Errorf("all stores unavailable (ephemeral query error: %w)", ephemeralErr)
	}

	if len(ephemeralResults) > 0 {
		result.Ephemeral = ephemeralResults
		result.TotalFacts += len(ephemeralResults)
	}

	// Source store
	if sourceStore != nil && srcErr == nil {
		sourceResults, srcQErr := gcamdb.Query(ctx, sourceStore, req.Query)
		if srcQErr != nil {
			logger.Warn("source store query failed", "project", req.ProjectID, "error", srcQErr)
		} else if len(sourceResults) > 0 {
			result.Source = sourceResults
			result.TotalFacts += len(sourceResults)
		}
	}

	// Analytical store
	if analyticalStore != nil && anaErr == nil {
		analyticalResults, anaQErr := gcamdb.Query(ctx, analyticalStore, req.Query)
		if anaQErr != nil {
			logger.Warn("analytical store query failed", "project", req.ProjectID, "error", anaQErr)
		} else if len(analyticalResults) > 0 {
			result.Analytical = analyticalResults
			result.TotalFacts += len(analyticalResults)
		}
	}

	return result, nil
}
