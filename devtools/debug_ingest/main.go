package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/meb/store"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Usage: %s <project_id>", os.Args[0])
	}
	projectID := os.Args[1]

	cwd, _ := os.Getwd()
	dataDir := filepath.Join(cwd, "data", projectID)
	cfg := store.DefaultConfig(dataDir)
	// We need write access
	cfg.ReadOnly = false

	db, err := meb.NewMEBStore(cfg)
	if err != nil {
		log.Fatalf("Failed to open store: %v", err)
	}
	defer db.Close()

	// Re-ingest ONLY math.go
	path := filepath.Join(cwd, "pkg/meb/vector/math.go")
	relPath := "pkg/meb/vector/math.go"

	// Create extractor
	ext := ingest.NewTreeSitterExtractor()

	// Read file
	content, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read file: %v", err)
	}

	// Extract
	ctx := context.Background()
	bundle, err := ext.Extract(ctx, relPath, content)
	if err != nil {
		log.Fatalf("Failed to extract: %v", err)
	}

	// Add File Doc
	if err := db.AddDocument(meb.DocumentID(relPath), content, nil, map[string]any{"project": projectID}); err != nil {
		log.Printf("Failed to add file doc: %v", err)
	}

	// Add Symbol Docs (This is what we fixed)
	for _, doc := range bundle.Documents {
		if err := db.AddDocument(doc.ID, doc.Content, nil, doc.Metadata); err != nil {
			log.Printf("Failed to add symbol doc: %v", err)
		} else {
			log.Printf("Added symbol doc: %s (len=%d)", doc.ID, len(doc.Content))
		}
	}

	log.Println("Re-ingestion complete for math.go")
}
