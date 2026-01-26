package service

import (
	"context"
	"os"
	"testing"

	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/meb/store"
)

func TestResolveVirtualTriples(t *testing.T) {
	// 1. Setup Store
	tmpDir, err := os.MkdirTemp("", "virtual_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := store.DefaultConfig(tmpDir)
	cfg.BypassLockGuard = true // For testing
	s, err := meb.Open(tmpDir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// 2. Add Data
	// Interface: pkg/service:Processor
	// Struct: pkg/impl:ProcessorImpl
	// Struct: pkg/default:DefaultProcessor
	// Struct: pkg/other:RandomStruct

	iName := "pkg/service:Processor"
	sName1 := "pkg/impl:ProcessorImpl"
	sName2 := "pkg/default:DefaultProcessor"
	sName3 := "pkg/other:RandomStruct"

	ctx := context.Background()

	// Add "defines" and "has_kind" facts
	// Interface
	s.AddFact(meb.Fact{Subject: meb.DocumentID(iName), Predicate: "has_kind", Object: "interface"})

	// Structs
	s.AddFact(meb.Fact{Subject: meb.DocumentID(sName1), Predicate: "has_kind", Object: "struct"})
	s.AddFact(meb.Fact{Subject: meb.DocumentID(sName2), Predicate: "has_kind", Object: "struct"})
	s.AddFact(meb.Fact{Subject: meb.DocumentID(sName3), Predicate: "has_kind", Object: "struct"})

	// 3. Setup Service
	svc := NewGraphService(&MockStoreManager{store: s})

	// 4. Resolve Virtual Triples
	graph, err := svc.ResolveVirtualTriples(ctx, "test")
	if err != nil {
		t.Fatalf("ResolveVirtualTriples failed: %v", err)
	}

	// 5. Verify Results
	// We expect 2 links: iName -> sName1, iName -> sName2
	if len(graph.Links) != 2 {
		t.Errorf("Expected 2 virtual links, got %d", len(graph.Links))
	}

	foundImpl := false
	foundDefault := false

	for _, l := range graph.Links {
		if l.Type != "virtual" {
			t.Errorf("Expected virtual link type, got %s", l.Type)
		}
		if l.Relation != "v:wires_to" {
			t.Errorf("Expected v:wires_to relation, got %s", l.Relation)
		}
		if l.Source == iName && l.Target == sName1 {
			foundImpl = true
		}
		if l.Source == iName && l.Target == sName2 {
			foundDefault = true
		}
	}

	if !foundImpl {
		t.Errorf("Missing virtual link for ProcessorImpl")
	}
	if !foundDefault {
		t.Errorf("Missing virtual link for DefaultProcessor")
	}
}
