package meb

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/duynguyendang/gca/pkg/meb/store"
)

func TestAnalysis_ResolveDependencies(t *testing.T) {
	// Setup Store
	tmpDir := t.TempDir()
	cfg := store.DefaultConfig(tmpDir)
	cfg.DictDir = filepath.Join(tmpDir, "dict")
	cfg.InMemory = true

	s, err := NewMEBStore(cfg)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// --- Scenario 1: Interface Inference ---
	// 1. Define Interface
	ifaceID := DocumentID("pkg/repo:Repository")
	s.AddFact(NewFact(ifaceID, "kind", "interface"))

	// 2. Define Struct (Implementation) - Not explicitly linked, but "implements" helps
	structID := DocumentID("pkg/repo:PostgresRepo")
	// Add "implements" fact manually for test (ingestion normally handles this)
	s.AddFact(NewFact(structID, "implements", string(ifaceID)))

	// 3. Define Caller (Service)
	serviceID := DocumentID("pkg/svc:UserService")
	// Service calls Interface method
	// For ResolveDependencies, we look for "calls" to Interface
	s.AddFact(NewFact(serviceID, "calls", string(ifaceID)))

	// --- Scenario 2: DI Wire Inference ---
	// 1. File A with metadata wire: "MyDatabase"
	fileAID := DocumentID("pkg/app:main.go")
	s.AddDocument(fileAID, []byte("..."), nil, map[string]any{"wire": "MyDatabase"})
	s.AddFact(NewFact(fileAID, "has_hash", "hash1")) // Needed for iteration scan in Analysis

	// 2. File B defines "MyDatabase"
	fileBID := DocumentID("pkg/db:MyDatabase")                                 // This handles the prefix check logic: ends with :MyDatabase
	s.AddFact(NewFact(DocumentID("pkg/db:db.go"), "defines", string(fileBID))) // fileB defines the symbol

	// --- Execute Analysis ---
	if err := s.ResolveDependencies(ctx); err != nil {
		t.Fatalf("ResolveDependencies failed: %v", err)
	}

	// --- Verify Results ---

	// Verify Scenario 1: v:potentially_calls(Service, Struct)
	foundCall := false
	for fact := range s.ScanContext(ctx, string(serviceID), "v:potentially_calls", string(structID), "") {
		foundCall = true
		if fact.Weight != 0.8 {
			t.Errorf("Expected weight 0.8, got %f", fact.Weight)
		}
		if fact.Source != "virtual" {
			t.Errorf("Expected source 'virtual', got %s", fact.Source)
		}
	}
	if !foundCall {
		t.Errorf("Failed to find inferred v:potentially_calls link")
	}

	// Verify Scenario 2: v:wires_to(FileA, Symbol)
	foundWire := false
	for fact := range s.ScanContext(ctx, string(fileAID), "v:wires_to", string(fileBID), "") {
		foundWire = true
		if fact.Weight != 0.5 {
			t.Errorf("Expected weight 0.5, got %f", fact.Weight)
		}
		if fact.Source != "virtual" {
			t.Errorf("Expected source 'virtual', got %s", fact.Source)
		}
	}
	if !foundWire {
		t.Errorf("Failed to find inferred v:wires_to link")
	}
}

func TestFactMetadataEncoding(t *testing.T) {
	f := Fact{
		Subject:   "S",
		Predicate: "P",
		Object:    "O",
		Weight:    0.75,
		Source:    "inference",
	}

	data := EncodeFactMetadata(f)

	w, src := DecodeFactMetadata(data)
	if w != 0.75 {
		t.Errorf("Expected weight 0.75, got %f", w)
	}
	if src != "inference" {
		t.Errorf("Expected source 'inference', got %s", src)
	}

	// Test Default
	w, src = DecodeFactMetadata(nil)
	if w != 1.0 {
		t.Errorf("Default weight should be 1.0, got %f", w)
	}
}
