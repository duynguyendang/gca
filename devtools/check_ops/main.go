package main

import (
	"fmt"
	"log"
	
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/keys"
	"github.com/duynguyendang/meb/store"
)

func main() {
	cfg := store.DefaultConfig("./data/gca-v2-fresh")
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	
	fmt.Printf("Current topic ID: %d\n", s.TopicID())
	
	// Get dictionary IDs
	execID, _ := s.Dict().GetID("Execute")
	hasNameID, _ := s.Dict().GetID("has_name")
	
	fmt.Printf("'Execute' dict ID: %d\n", execID)
	fmt.Printf("'has_name' dict ID: %d\n", hasNameID)
	
	// Pack with topic ID 1 (current)
	topicID := s.TopicID()
	oPacked := keys.PackID(topicID, keys.UnpackLocalID(execID))
	pPacked := keys.PackID(topicID, keys.UnpackLocalID(hasNameID))
	
	fmt.Printf("\nTopic ID: %d\n", topicID)
	fmt.Printf("Object packed: %d (topic=%d, local=%d)\n", oPacked, topicID, keys.UnpackLocalID(execID))
	fmt.Printf("Predicate packed: %d (topic=%d, local=%d)\n", pPacked, topicID, keys.UnpackLocalID(hasNameID))
	
	// Build OPS key prefix
	opsPrefix := keys.EncodeTripleOPSPrefix(oPacked, pPacked, 0)
	fmt.Printf("\nOPS prefix to scan: %v\n", opsPrefix)
	
	// Now let's look at what's actually in the OPS index
	// We need to access the badger DB directly
	fmt.Println("\n=== Checking OPS index directly ===")
	
	// Try scanning with topic ID 1
	opsKey1 := keys.EncodeTripleOPSPrefix(keys.PackID(1, keys.UnpackLocalID(execID)), keys.PackID(1, keys.UnpackLocalID(hasNameID)), 0)
	fmt.Printf("OPS key (topic=1): %v\n", opsKey1)
	
	// The issue: when we stored with topic 6611809, the object was packed with THAT topic
	// So we need to scan without topic packing, or scan all topics
	
	// Let's try a broader scan - just the object ID without topic packing
	fmt.Println("\n=== Scanning OPS index for Execute ===")
	
	// Access internal DB - we need to do this through the exported interface
	// Since we can't access s.db directly, let's try different approaches
	
	// Try with Scan using empty subject
	fmt.Println("Using Scan('', 'has_name', 'Execute'):")
	count := 0
	for fact, err := range s.Scan("", "has_name", "Execute") {
		if err != nil {
			fmt.Printf("  Error: %v\n", err)
			break
		}
		fmt.Printf("  %s has_name %v\n", fact.Subject, fact.Object)
		count++
	}
	fmt.Printf("Total: %d\n", count)
}
