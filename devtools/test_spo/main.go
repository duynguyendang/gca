package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/duynguyendang/gca/internal/manager"
)

func main() {
	// Use the existing data directory
	dataDir := "../data"
	projectID := "genkit-go"

	if len(os.Args) > 1 {
		dataDir = os.Args[1]
	}
	if len(os.Args) > 2 {
		projectID = os.Args[2]
	}

	fmt.Printf("Testing SPO index with project: %s (data dir: %s)\n", projectID, dataDir)

	// Create store manager
	sm := manager.NewStoreManager(dataDir, manager.MemoryProfile{}, false)

	// Get store (this should set topicID)
	store, err := sm.GetStore(projectID)
	if err != nil {
		log.Fatalf("Failed to get store: %v", err)
	}
	defer store.Close()

	fmt.Printf("Topic ID: %d\n", store.TopicID())

	ctx := context.Background()

	// Count total facts
	totalFacts := 0
	for range store.ScanContext(ctx, "", "", "") {
		totalFacts++
	}
	fmt.Printf("Total facts: %d\n", totalFacts)

	// Test 1: Scan all facts (no bounds)
	fmt.Println("\n=== Test 1: Scan all facts (no bounds) ===")
	countAll := 0
	for _, err := range store.ScanContext(ctx, "", "", "") {
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		countAll++
		if countAll <= 5 {
			// We'll get the fact details in the next scan
		}
	}
	fmt.Printf("Total facts scanned: %d\n", countAll)

	// Test 2: Scan with bound subject
	fmt.Println("\n=== Test 2: Scan with bound subject ===")
	// First, let's find a subject that exists
	fmt.Println("Finding first few facts to get subject names...")
	subjectCount := 0
	for fact, err := range store.ScanContext(ctx, "", "", "") {
		if err != nil {
			continue
		}
		fmt.Printf("  Fact %d: subject=%q, predicate=%q, object=%v\n", subjectCount+1, fact.Subject, fact.Predicate, fact.Object)
		subjectCount++
		if subjectCount >= 3 {
			break
		}
	}

	// Now let's try scanning for a specific subject
	// We'll use one from the facts we just saw, or try a common pattern
	testSubjects := []string{}
	if subjectCount > 0 {
		// Re-scan to get actual subjects
		for fact, err := range store.ScanContext(ctx, "", "defines", "") {
			if err != nil {
				continue
			}
			testSubjects = append(testSubjects, fact.Subject)
			if len(testSubjects) >= 2 {
				break
			}
		}
	}

	for _, subj := range testSubjects {
		fmt.Printf("\nScanning for subject=%q:\n", subj)
		count := 0
		for fact, err := range store.ScanContext(ctx, subj, "", "") {
			if err != nil {
				fmt.Printf("  Error: %v\n", err)
				continue
			}
			fmt.Printf("  Found: <%s, %s, %v>\n", fact.Subject, fact.Predicate, fact.Object)
			count++
		}
		fmt.Printf("  Total: %d facts\n", count)
	}

	// Test 3: Scan with bound predicate only
	fmt.Println("\n=== Test 3: Scan with bound predicate='defines' ===")
	countDefines := 0
	for fact, err := range store.ScanContext(ctx, "", "defines", "") {
		if err != nil {
			continue
		}
		countDefines++
		if countDefines <= 3 {
			fmt.Printf("  Found: <%s, %s, %v>\n", fact.Subject, fact.Predicate, fact.Object)
		}
	}
	fmt.Printf("Total 'defines' facts: %d\n", countDefines)

	// Test 4: Scan with bound object
	fmt.Println("\n=== Test 4: Scan with bound object ===")
	// Find an object to search for
	for fact, err := range store.ScanContext(ctx, "", "calls", "") {
		if err != nil {
			continue
		}
		objStr, ok := fact.Object.(string)
		if !ok {
			continue
		}
		fmt.Printf("Scanning for object=%q:\n", objStr)
		count := 0
		for fact2, err2 := range store.ScanContext(ctx, "", "calls", objStr) {
			if err2 != nil {
				continue
			}
			fmt.Printf("  Found: <%s, %s, %v>\n", fact2.Subject, fact2.Predicate, fact2.Object)
			count++
		}
		fmt.Printf("  Total: %d facts\n", count)
		break
	}

	// Test 5: Use Query function (datalog)
	fmt.Println("\n=== Test 5: Datalog query ===")
	// Import the query function
	// We'll need to test this separately since it's in a different package
	fmt.Println("(Datalog query test requires separate execution)")
}
