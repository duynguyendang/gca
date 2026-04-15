package meb

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

// TestSPOIndexBasic tests basic SPO index lookups with bound subject
func TestSPOIndexBasic(t *testing.T) {
	// Setup
	tmpDir, err := os.MkdirTemp("", "spo_basic_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := store.DefaultConfig(tmpDir)
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Set topic ID
	s.SetTopicID(1)
	t.Logf("Topic ID set to: %d", s.TopicID())

	ctx := context.Background()

	// Add test facts
	facts := []meb.Fact{
		{Subject: "main.go", Predicate: "defines", Object: "main"},
		{Subject: "main.go", Predicate: "has_kind", Object: "file"},
		{Subject: "main.go", Predicate: "imports", Object: "fmt"},
		{Subject: "utils.go", Predicate: "defines", Object: "helper"},
	}

	for _, fact := range facts {
		if err := s.AddFact(fact); err != nil {
			t.Fatalf("Failed to add fact %+v: %v", fact, err)
		}
	}

	t.Logf("Added %d facts", len(facts))

	// Test 1: Scan with bound subject only
	t.Run("BoundSubjectOnly", func(t *testing.T) {
		count := 0
		t.Logf("Scanning for subject='main.go', predicate='', object=''")
		
		for fact, err := range s.ScanContext(ctx, "main.go", "", "") {
			if err != nil {
				t.Errorf("Scan error: %v", err)
				continue
			}
			t.Logf("Found fact: %+v", fact)
			count++
		}

		if count != 3 {
			t.Errorf("Expected 3 facts for subject 'main.go', got %d", count)
		}
	})

	// Test 2: Scan with bound subject and predicate
	t.Run("BoundSubjectAndPredicate", func(t *testing.T) {
		count := 0
		t.Logf("Scanning for subject='main.go', predicate='defines', object=''")
		
		for fact, err := range s.ScanContext(ctx, "main.go", "defines", "") {
			if err != nil {
				t.Errorf("Scan error: %v", err)
				continue
			}
			t.Logf("Found fact: %+v", fact)
			count++
		}

		if count != 1 {
			t.Errorf("Expected 1 fact, got %d", count)
		}
	})

	// Test 3: Scan with all three bound
	t.Run("AllBound", func(t *testing.T) {
		count := 0
		t.Logf("Scanning for subject='main.go', predicate='defines', object='main'")
		
		for fact, err := range s.ScanContext(ctx, "main.go", "defines", "main") {
			if err != nil {
				t.Errorf("Scan error: %v", err)
				continue
			}
			t.Logf("Found fact: %+v", fact)
			count++
		}

		if count != 1 {
			t.Errorf("Expected 1 fact, got %d", count)
		}
	})

	// Test 4: Scan with no bounds (should return all facts)
	t.Run("NoBounds", func(t *testing.T) {
		count := 0
		t.Logf("Scanning for all facts (no bounds)")
		
		for fact, err := range s.ScanContext(ctx, "", "", "") {
			if err != nil {
				t.Errorf("Scan error: %v", err)
				continue
			}
			t.Logf("Found fact: %+v", fact)
			count++
		}

		if count != 4 {
			t.Errorf("Expected 4 facts total, got %d", count)
		}
	})
}

// TestSPOIndexWithDifferentTopicIDs tests that topic ID isolation works correctly
func TestSPOIndexWithDifferentTopicIDs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "spo_topic_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := store.DefaultConfig(tmpDir)
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx := context.Background()

	// Add facts with topic ID 1
	s.SetTopicID(1)
	s.AddFact(meb.Fact{Subject: "file1.go", Predicate: "defines", Object: "func1"})
	s.AddFact(meb.Fact{Subject: "file1.go", Predicate: "has_kind", Object: "file"})

	// Add facts with topic ID 2
	s.SetTopicID(2)
	s.AddFact(meb.Fact{Subject: "file2.go", Predicate: "defines", Object: "func2"})
	s.AddFact(meb.Fact{Subject: "file2.go", Predicate: "has_kind", Object: "file"})

	// Scan with topic ID 1
	s.SetTopicID(1)
	t.Log("Scanning with topic ID 1:")
	count1 := 0
	for fact, err := range s.ScanContext(ctx, "file1.go", "", "") {
		if err != nil {
			t.Errorf("Scan error: %v", err)
			continue
		}
		t.Logf("  Found: %+v", fact)
		count1++
	}
	if count1 != 2 {
		t.Errorf("Expected 2 facts for topic 1, got %d", count1)
	}

	// Scan with topic ID 2
	s.SetTopicID(2)
	t.Log("Scanning with topic ID 2:")
	count2 := 0
	for fact, err := range s.ScanContext(ctx, "file2.go", "", "") {
		if err != nil {
			t.Errorf("Scan error: %v", err)
			continue
		}
		t.Logf("  Found: %+v", fact)
		count2++
	}
	if count2 != 2 {
		t.Errorf("Expected 2 facts for topic 2, got %d", count2)
	}

	// Verify topic isolation: scanning for file1.go with topic 2 should return 0
	t.Log("Scanning for file1.go with topic ID 2 (should return 0):")
	countIsolated := 0
	for fact, err := range s.ScanContext(ctx, "file1.go", "", "") {
		if err != nil {
			t.Errorf("Scan error: %v", err)
			continue
		}
		t.Logf("  Found (unexpected): %+v", fact)
		countIsolated++
	}
	if countIsolated != 0 {
		t.Errorf("Expected 0 facts (topic isolation), got %d", countIsolated)
	}
}

// TestSPOIndexOPS tests the OPS (Object-Predicate-Subject) index path
func TestSPOIndexOPS(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "spo_ops_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := store.DefaultConfig(tmpDir)
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.SetTopicID(1)
	ctx := context.Background()

	// Add facts
	s.AddFact(meb.Fact{Subject: "a.go", Predicate: "calls", Object: "b.go:foo"})
	s.AddFact(meb.Fact{Subject: "c.go", Predicate: "calls", Object: "b.go:foo"})
	s.AddFact(meb.Fact{Subject: "d.go", Predicate: "calls", Object: "b.go:bar"})

	// Test: Find all callers of "b.go:foo" (bound object)
	t.Log("Scanning for object='b.go:foo':")
	count := 0
	for fact, err := range s.ScanContext(ctx, "", "calls", "b.go:foo") {
		if err != nil {
			t.Errorf("Scan error: %v", err)
			continue
		}
		t.Logf("  Found: %+v", fact)
		count++
	}
	if count != 2 {
		t.Errorf("Expected 2 facts calling 'b.go:foo', got %d", count)
	}
}

// TestSPOIndexDictionary tests that dictionary lookups work correctly
func TestSPOIndexDictionary(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "spo_dict_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := store.DefaultConfig(tmpDir)
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.SetTopicID(1)
	ctx := context.Background()

	// Check dictionary IDs before adding facts
	t.Log("Checking dictionary ID resolution:")
	
	subjID, subjFound := s.LookupID("main.go")
	t.Logf("  Subject 'main.go' ID: %d, found: %v", subjID, subjFound)
	
	predID, predFound := s.LookupID("defines")
	t.Logf("  Predicate 'defines' ID: %d, found: %v", predID, predFound)

	// Add fact
	s.AddFact(meb.Fact{Subject: "main.go", Predicate: "defines", Object: "main"})

	// Check dictionary IDs after adding facts
	subjID2, subjFound2 := s.LookupID("main.go")
	t.Logf("  After add - Subject 'main.go' ID: %d, found: %v", subjID2, subjFound2)
	
	predID2, predFound2 := s.LookupID("defines")
	t.Logf("  After add - Predicate 'defines' ID: %d, found: %v", predID2, predFound2)

	// Test scan
	t.Log("Scanning for subject='main.go':")
	count := 0
	for fact, err := range s.ScanContext(ctx, "main.go", "", "") {
		if err != nil {
			t.Errorf("Scan error: %v", err)
			continue
		}
		t.Logf("  Found: %+v", fact)
		count++
	}
	if count != 1 {
		t.Errorf("Expected 1 fact, got %d", count)
	}
}

// TestSPOIndexLargeDataset tests with a larger dataset to ensure scalability
func TestSPOIndexLargeDataset(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "spo_large_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := store.DefaultConfig(tmpDir)
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.SetTopicID(1)
	ctx := context.Background()

	// Add 100 facts
	numFacts := 100
	for i := 0; i < numFacts; i++ {
		subject := fmt.Sprintf("file%d.go", i%10) // 10 unique subjects, 10 facts each
		s.AddFact(meb.Fact{
			Subject:   subject,
			Predicate: "has_line",
			Object:    fmt.Sprintf("%d", i),
		})
	}

	t.Logf("Added %d facts", numFacts)

	// Test: Scan for one subject (should return 10 facts)
	t.Log("Scanning for subject='file0.go':")
	count := 0
	for _, err := range s.ScanContext(ctx, "file0.go", "", "") {
		if err != nil {
			t.Errorf("Scan error: %v", err)
			continue
		}
		count++
	}
	if count != 10 {
		t.Errorf("Expected 10 facts for 'file0.go', got %d", count)
	}

	// Test: Scan all facts
	t.Log("Scanning all facts:")
	totalCount := 0
	for _, err := range s.ScanContext(ctx, "", "", "") {
		if err != nil {
			t.Errorf("Scan error: %v", err)
			continue
		}
		totalCount++
	}
	if totalCount != numFacts {
		t.Errorf("Expected %d total facts, got %d", numFacts, totalCount)
	}
}
