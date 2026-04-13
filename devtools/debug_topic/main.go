package main

import (
	"context"
	"fmt"
	"log"
	
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func main() {
	cfg := store.DefaultConfig("./data/gca-v2-fresh")
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	
	ctx := context.Background()
	
	fmt.Printf("Current topic ID: %d\n", s.TopicID())
	
	// Check if "Execute" exists in dictionary
	fmt.Println("\n=== Checking dictionary ===")
	execID, err := s.Dict().GetID("Execute")
	if err != nil {
		fmt.Printf("  'Execute' not in dictionary\n")
	} else {
		fmt.Printf("  'Execute' ID: %d\n", execID)
	}
	
	hasNameID, err := s.Dict().GetID("has_name")
	if err != nil {
		fmt.Printf("  'has_name' not in dictionary\n")
	} else {
		fmt.Printf("  'has_name' ID: %d\n", hasNameID)
	}
	
	// Try raw Scan
	fmt.Println("\n=== Raw Scan for has_name ===")
	count := 0
	for fact, err := range s.Scan("", "has_name", "") {
		if err != nil {
			fmt.Printf("  Error: %v\n", err)
			break
		}
		objStr, _ := fact.Object.(string)
		if objStr == "Execute" {
			fmt.Printf("  Found Execute: %s has_name %s\n", fact.Subject, objStr)
		}
		count++
		if count <= 3 {
			fmt.Printf("  %s has_name %v\n", fact.Subject, fact.Object)
		}
	}
	fmt.Printf("Total has_name facts via Scan: %d\n", count)
	
	// Check OPS index directly
	fmt.Println("\n=== Checking OPS index structure ===")
	// The issue might be topic ID mismatch in OPS lookup
	
	// Set topic ID to 1 (default) and try
	s.SetTopicID(1)
	fmt.Printf("Set topic ID to: %d\n", s.TopicID())
	
	count2 := 0
	for subject := range s.FindSubjectsByObject(ctx, "has_name", "Execute") {
		fmt.Printf("  Found via FindSubjectsByObject: %s\n", subject)
		count2++
	}
	fmt.Printf("Total via FindSubjectsByObject: %d\n", count2)
	
	// Try with ScanContext
	fmt.Println("\n=== ScanContext test ===")
	count3 := 0
	for fact, err := range s.ScanContext(ctx, "", "has_name", "Execute") {
		if err != nil {
			continue
		}
		fmt.Printf("  %s has_name %v\n", fact.Subject, fact.Object)
		count3++
	}
	fmt.Printf("Total via ScanContext: %d\n", count3)
}
