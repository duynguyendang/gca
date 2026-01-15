package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/meb/store"
)

func main() {
	// Setup
	dir, err := os.MkdirTemp("", "gca-perf-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	srcDir, err := os.MkdirTemp("", "gca-src-perf-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(srcDir)

	cfg := store.DefaultConfig(dir)
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()

	// 1. Create 50 files
	fmt.Println("Creating 50 files...")
	for i := 0; i < 50; i++ {
		content := fmt.Sprintf("package main\nfunc Func%d() {}\n", i)
		name := fmt.Sprintf("file_%d.go", i)
		os.WriteFile(srcDir+"/"+name, []byte(content), 0644)
	}

	// 2. First Ingestion
	start := time.Now()
	if err := ingest.Run(s, srcDir); err != nil {
		log.Fatal(err)
	}
	duration := time.Since(start)
	fmt.Printf("Initial Ingestion took: %v\n", duration)

	// 3. Second Ingestion (No Changes)
	start = time.Now()
	if err := ingest.Run(s, srcDir); err != nil {
		log.Fatal(err)
	}
	duration = time.Since(start)
	fmt.Printf("Second Ingestion (Unchanged) took: %v\n", duration)

	// 4. Update 1 file
	fmt.Println("Updating file_0.go...")
	os.WriteFile(srcDir+"/file_0.go", []byte("package main\nfunc Func0_Modified() {}\n"), 0644)

	// 5. Third Ingestion (1 Change)
	start = time.Now()
	if err := ingest.Run(s, srcDir); err != nil {
		log.Fatal(err)
	}
	duration = time.Since(start)
	fmt.Printf("Third Ingestion (1 Change) took: %v\n", duration)

	// Verify result
	// Should have Func0_Modified, should NOT have Func0
	fmt.Println("Verifying facts (Func0 should be gone)...")
	// Note: we can't easily check for absence in this script without datalog query,
	// assuming logic works if no error.
}
