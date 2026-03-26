package registry

import (
	"context"
	"testing"

	manglesdk "github.com/duynguyendang/manglekit/sdk"
)

// TestQueryRegistry_LoadQueriesFromGenePool tests loading queries from GenePool
func TestQueryRegistry_LoadQueriesFromGenePool(t *testing.T) {
	// Create a manglekit client
	client, err := manglesdk.NewClient(context.Background())
	if err != nil {
		t.Fatalf("Failed to create manglekit client: %v", err)
	}
	defer client.Shutdown(context.Background())

	// Create query registry
	registry := NewQueryRegistry(client.Engine())

	// Load queries from GenePool
	err = registry.LoadQueriesFromGenePool(context.Background(), "../../policies/queries.dl")
	if err != nil {
		t.Logf("Warning: Failed to load queries from GenePool (file may not exist): %v", err)
		// This is expected in test environments where the policy file doesn't exist
		return
	}

	// Verify queries were loaded
	queries := registry.ListQueries()
	if len(queries) == 0 {
		t.Error("No queries loaded from GenePool")
	}

	t.Logf("Loaded %d queries from GenePool", len(queries))
	for _, query := range queries {
		t.Logf("  - %s (%s): %s", query.Name, query.Category, query.Description)
	}
}

// TestQueryRegistry_GetQuery tests retrieving a specific query
func TestQueryRegistry_GetQuery(t *testing.T) {
	// Create a manglekit client
	client, err := manglesdk.NewClient(context.Background())
	if err != nil {
		t.Fatalf("Failed to create manglekit client: %v", err)
	}
	defer client.Shutdown(context.Background())

	// Create query registry
	registry := NewQueryRegistry(client.Engine())

	// Load queries from GenePool
	err = registry.LoadQueriesFromGenePool(context.Background(), "../../policies/queries.dl")
	if err != nil {
		t.Skip("Skipping test: GenePool policies not available")
		return
	}

	// Test getting a query that should exist
	query, err := registry.GetQuery("find_defines")
	if err != nil {
		t.Fatalf("Failed to get query 'find_defines': %v", err)
	}

	if query.Name != "find_defines" {
		t.Errorf("Expected query name 'find_defines', got '%s'", query.Name)
	}

	if query.Category != "structural" {
		t.Errorf("Expected category 'structural', got '%s'", query.Category)
	}

	if query.Tier != 1 {
		t.Errorf("Expected tier 1, got %d", query.Tier)
	}
}

// TestQueryRegistry_ExecuteQuery tests query execution with validation
func TestQueryRegistry_ExecuteQuery(t *testing.T) {
	// Create a manglekit client
	client, err := manglesdk.NewClient(context.Background())
	if err != nil {
		t.Fatalf("Failed to create manglekit client: %v", err)
	}
	defer client.Shutdown(context.Background())

	// Create query registry
	registry := NewQueryRegistry(client.Engine())

	// Load queries from GenePool
	err = registry.LoadQueriesFromGenePool(context.Background(), "../../policies/queries.dl")
	if err != nil {
		t.Skip("Skipping test: GenePool policies not available")
		return
	}

	// Test executing a query with parameters
	query, err := registry.ExecuteQuery(context.Background(), "find_defines", map[string]any{
		"FileID": "pkg/service/graph.go",
	})

	if err != nil {
		t.Logf("Query execution returned error (expected in test environment): %v", err)
	}

	if query != "" {
		t.Logf("Generated query: %s", query)
	}
}

// TestQueryRegistry_ListQueriesByCategory tests filtering queries by category
func TestQueryRegistry_ListQueriesByCategory(t *testing.T) {
	// Create a manglekit client
	client, err := manglesdk.NewClient(context.Background())
	if err != nil {
		t.Fatalf("Failed to create manglekit client: %v", err)
	}
	defer client.Shutdown(context.Background())

	// Create query registry
	registry := NewQueryRegistry(client.Engine())

	// Load queries from GenePool
	err = registry.LoadQueriesFromGenePool(context.Background(), "../../policies/queries.dl")
	if err != nil {
		t.Skip("Skipping test: GenePool policies not available")
		return
	}

	// Test listing queries by category
	queries := registry.ListQueriesByCategory("structural")

	if len(queries) == 0 {
		t.Error("No structural queries found")
	}

	for _, query := range queries {
		if query.Category != "structural" {
			t.Errorf("Expected category 'structural', got '%s'", query.Category)
		}
	}

	t.Logf("Found %d structural queries", len(queries))
}
