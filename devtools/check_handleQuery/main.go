package main

import (
	"fmt"
	"log"
	
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func main() {
	cfg := store.DefaultConfig("./data/gca-v2")
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	
	// Check what defines facts look like for handlers.go
	fmt.Println("=== defines facts for handlers.go ===")
	count := 0
	for fact, err := range s.Scan("gca-v2/pkg/server/handlers.go", "defines", "") {
		if err != nil {
			continue
		}
		symID, _ := fact.Object.(string)
		fmt.Printf("  defines: %s\n", symID)
		
		// Check has_name for this symbol
		for hn, hnErr := range s.Scan(symID, "has_name", "") {
			if hnErr != nil {
				continue
			}
			fmt.Printf("    -> has_name: %v\n", hn.Object)
		}
		
		count++
		if count >= 5 {
			fmt.Println("  ... (truncated)")
			break
		}
	}
}
