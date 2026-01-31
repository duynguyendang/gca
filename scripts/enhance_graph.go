package main

import (
	"fmt"
	"os"

	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/meb/store"
)

func main() {
	dataDir := "./data/gca"
	fmt.Printf("Applying Virtual Triples Enhancement to %s...\n", dataDir)

	cfg := store.DefaultConfig(dataDir)
	cfg.ReadOnly = false
	cfg.BypassLockGuard = true // Ensure we can open it

	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		fmt.Printf("Failed to open store: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	if err := ingest.EnhanceVirtualTriples(s); err != nil {
		fmt.Printf("Failed to enhance triples: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Success.")
}
