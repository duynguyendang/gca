package main

import (
	"context"
	"fmt"
	"log"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func main() {
	// Create a test store
	cfg := &store.Config{
		InMemory:       true,
		BlockCacheSize: 1 << 20, // 1MB
		IndexCacheSize: 1 << 20, // 1MB
	}

	store, err := meb.NewMEBStore(cfg)
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Set topic ID (simulate project ingestion)
	store.SetTopicID(12345)
	fmt.Println("Topic ID set to:", store.TopicID())

	// Simulate what extractor.processSymbols does
	symbols := []ingest.Symbol{
		{
			ID:        "test/file.go:MyFunction",
			Name:      "MyFunction",
			Type:      "function",
			Package:   "test",
			StartLine: 10,
			EndLine:   20,
		},
		{
			ID:        "test/file.go:MyStruct",
			Name:      "MyStruct",
			Type:      "struct",
			Package:   "test",
			StartLine: 25,
			EndLine:   35,
		},
	}

	relPath := "myproject/test/file.go"
	filePackage := "test"
	tags := []string{"backend"}

	bundle := &ingest.AnalysisBundle{
		Documents: make([]ingest.Document, 0, len(symbols)),
		Facts:     make([]meb.Fact, 0, len(symbols)*5),
	}

	// Add file-level facts
	bundle.Facts = append(bundle.Facts, meb.Fact{
		Subject:   relPath,
		Predicate: config.PredicateInPackage,
		Object:    filePackage,
	})

	// Process symbols (mimicking extractor.go line 271-310)
	for _, sym := range symbols {
		doc := ingest.Document{
			ID:      string(sym.ID),
			Content: []byte(fmt.Sprintf("func %s() {}", sym.Name)),
			Metadata: map[string]any{
				"file":       relPath,
				"start_line": int32(sym.StartLine),
				"end_line":   int32(sym.EndLine),
				"package":    filePackage,
				"tags":       tags,
			},
		}
		bundle.Documents = append(bundle.Documents, doc)

		// These are the exact facts created in processSymbols (line 294-299)
		bundle.Facts = append(bundle.Facts,
			meb.Fact{Subject: string(sym.ID), Predicate: config.PredicateType, Object: sym.Type},
			meb.Fact{Subject: relPath, Predicate: config.PredicateDefines, Object: sym.ID},
			meb.Fact{Subject: string(sym.ID), Predicate: config.PredicateInPackage, Object: filePackage},
			meb.Fact{Subject: string(sym.ID), Predicate: config.PredicateName, Object: sym.Name},
			meb.Fact{Subject: string(sym.ID), Predicate: config.PredicateHasName, Object: sym.Name},
		)

		fmt.Printf("Created has_name fact: Subject=%s, Predicate=%s, Object=%s\n",
			sym.ID, config.PredicateHasName, sym.Name)
	}

	fmt.Printf("\nTotal facts in bundle: %d\n", len(bundle.Facts))
	fmt.Println("Facts breakdown:")
	factCounts := make(map[string]int)
	for _, f := range bundle.Facts {
		factCounts[f.Predicate]++
	}
	for pred, count := range factCounts {
		fmt.Printf("  %s: %d\n", pred, count)
	}

	// Add the facts to the store
	fmt.Println("\nAdding fact batch to store...")
	err = store.AddFactBatch(bundle.Facts)
	if err != nil {
		log.Fatalf("Failed to add fact batch: %v", err)
	}
	fmt.Println("Fact batch added successfully")

	// Now query for has_name facts
	fmt.Println("\n=== Querying has_name facts ===")
	ctx := context.Background()
	hasNameCount := 0
	for fact, err := range store.Scan("", config.PredicateHasName, "") {
		if err != nil {
			fmt.Printf("  Error: %v\n", err)
			continue
		}
		fmt.Printf("  Found: %s %s %v\n", fact.Subject, fact.Predicate, fact.Object)
		hasNameCount++
	}
	fmt.Printf("Total has_name facts via Scan: %d\n", hasNameCount)

	// Try FindSubjectsByObject
	fmt.Println("\n=== Using FindSubjectsByObject for 'MyFunction' ===")
	foundCount := 0
	for subject := range store.FindSubjectsByObject(ctx, config.PredicateHasName, "MyFunction") {
		fmt.Printf("  Subject: %s\n", subject)
		foundCount++
	}
	fmt.Printf("Total subjects via FindSubjectsByObject: %d\n", foundCount)

	// Check ALL facts
	fmt.Println("\n=== All facts in store ===")
	totalFacts := 0
	for fact, err := range store.Scan("", "", "") {
		if err != nil {
			continue
		}
		fmt.Printf("  %s %s %v\n", fact.Subject, fact.Predicate, fact.Object)
		totalFacts++
	}
	fmt.Printf("Total facts: %d\n", totalFacts)

	// Check defines facts
	fmt.Println("\n=== defines facts ===")
	definesCount := 0
	for fact, err := range store.Scan("", config.PredicateDefines, "") {
		if err != nil {
			continue
		}
		fmt.Printf("  %s %s %v\n", fact.Subject, fact.Predicate, fact.Object)
		definesCount++
	}
	fmt.Printf("Total defines facts: %d\n", definesCount)
}
