package ephemeral

import (
	"context"
	"testing"
	"time"

	"github.com/duynguyendang/meb"
)

// mockStoreAccessor returns nil for source/analytical stores.
// This isolates federation tests to the ephemeral-only path.
type mockStoreAccessor struct{}

func (m *mockStoreAccessor) GetSourceStore(projectID string) (*meb.MEBStore, error) {
	return nil, nil
}

func (m *mockStoreAccessor) GetAnalyticalStore(projectID string) (*meb.MEBStore, error) {
	return nil, nil
}

func TestFederatedQuery_SessionNotFound(t *testing.T) {
	es := NewEphemeralStore(defaultSessionTTL)
	defer es.Close()

	_, err := FederatedQuery(context.Background(), FederatedQueryRequest{
		SessionID: "nonexistent",
		ProjectID: "test",
		Query:     `triples(S, P, O)`,
	}, es, &mockStoreAccessor{})

	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestFederatedQuery_EphemeralOnly(t *testing.T) {
	es := NewEphemeralStore(defaultSessionTTL)
	defer es.Close()

	session, err := es.NewSession("test")
	if err != nil {
		t.Fatal(err)
	}

	// Add a fact directly
	fact := meb.Fact{Subject: "f.go", Predicate: "diff_added", Object: "1:new"}
	if err := session.Facts.AddFactBatch([]meb.Fact{fact}); err != nil {
		t.Fatalf("AddFactBatch: %v", err)
	}

	result, err := FederatedQuery(context.Background(), FederatedQueryRequest{
		SessionID: session.ID,
		ProjectID: "test",
		Query:     `triples(S, "diff_added", O), eq(S, "f.go")`,
	}, es, &mockStoreAccessor{})

	if err != nil {
		t.Fatalf("FederatedQuery() error = %v", err)
	}

	if result.TotalFacts != 1 {
		t.Errorf("TotalFacts = %d, want 1", result.TotalFacts)
	}
	if len(result.Ephemeral) != 1 {
		t.Errorf("len(Ephemeral) = %d, want 1", len(result.Ephemeral))
	}
	if len(result.Source) != 0 {
		t.Errorf("len(Source) = %d, want 0 (no source store)", len(result.Source))
	}
	if len(result.Analytical) != 0 {
		t.Errorf("len(Analytical) = %d, want 0 (no analytical store)", len(result.Analytical))
	}

	// Verify returned data
	row := result.Ephemeral[0]
	if s, ok := row["S"].(string); !ok || s != "f.go" {
		t.Errorf("unexpected subject: %v", row["S"])
	}
	if o, ok := row["O"].(string); !ok || o != "1:new" {
		t.Errorf("unexpected object: %v", row["O"])
	}
}

func TestFederatedQuery_QueryError(t *testing.T) {
	es := NewEphemeralStore(defaultSessionTTL)
	defer es.Close()

	session, err := es.NewSession("test")
	if err != nil {
		t.Fatal(err)
	}

	fact := meb.Fact{Subject: "f.go", Predicate: "diff_added", Object: "1:new"}
	if err := session.Facts.AddFactBatch([]meb.Fact{fact}); err != nil {
		t.Fatalf("AddFactBatch: %v", err)
	}

	// Malformed Datalog query: ephemeral query fails, other stores unavailable
	// → FederatedQuery returns error
	_, err = FederatedQuery(context.Background(), FederatedQueryRequest{
		SessionID: session.ID,
		ProjectID: "test",
		Query:     "not_valid_datalog(",
	}, es, &mockStoreAccessor{})

	if err == nil {
		t.Fatal("expected error when ephemeral query fails and other stores unavailable")
	}
}

func TestFederatedQuery_ExtendsTTL(t *testing.T) {
	es := NewEphemeralStore(30 * time.Minute)
	defer es.Close()

	session, err := es.NewSession("test")
	if err != nil {
		t.Fatal(err)
	}

	originalExpiry := session.ExpiresAt

	_, err = FederatedQuery(context.Background(), FederatedQueryRequest{
		SessionID: session.ID,
		ProjectID: "test",
		Query:     `triples(S, P, O)`,
	}, es, &mockStoreAccessor{})

	if err != nil {
		t.Fatalf("FederatedQuery() error = %v", err)
	}

	if !session.ExpiresAt.After(originalExpiry) {
		t.Error("session expiry was not extended after query")
	}
}
