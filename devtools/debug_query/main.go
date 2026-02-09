package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

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
	// Use meb.NewMEBStore with correct config struct
	cfg := store.DefaultConfig(dataDir)
	cfg.ReadOnly = true // Use read-only mode to avoid locking issues if server is running (though it's killed now)

	db, err := meb.NewMEBStore(cfg)
	if err != nil {
		log.Fatalf("Failed to open store: %v", err)
	}
	defer db.Close()

	// 1. Check if the file exists
	targetFile := "langgraph-fixed/libs/checkpoint/langgraph/checkpoint/serde/base.py"
	doc, err := db.GetDocument(meb.DocumentID(targetFile))
	if err != nil {
		fmt.Printf("File %s NOT FOUND: %v\n", targetFile, err)
	} else {
		fmt.Printf("File %s FOUND. Content length: %d\n", targetFile, len(doc.Content))
	}

	// 2. Check for specific symbol content
	// The user reported "langgraph.checkpoint.serde.base"
	// This maps to the file content itself often in our schema for modules
	targetSymbol := "langgraph-fixed/libs/checkpoint/langgraph/checkpoint/serde/base.py"

	var symDoc *meb.Document
	symDoc, err = db.GetDocument(meb.DocumentID(targetSymbol))
	if err != nil {
		fmt.Printf("Symbol %s NOT FOUND: %v\n", targetSymbol, err)
	} else {
		fmt.Printf("Symbol %s FOUND. Content length: %d\n", targetSymbol, len(symDoc.Content))
		if len(symDoc.Content) > 0 {
			fmt.Printf("Sample: %.50s...\n", string(symDoc.Content))
		}
	}

	// 3. Check imports of a specific file
	importingFile := "langgraph-fixed/libs/checkpoint/langgraph/cache/base/__init__.py"
	expectedImport := "langgraph-fixed/libs/checkpoint/langgraph/checkpoint/serde/base.py"

	fmt.Printf("Imports for %s:\n", importingFile)
	found := false
	for fact, err := range db.Scan(importingFile, meb.PredImports, "", "") {
		if err != nil {
			fmt.Printf("Error scanning imports: %v\n", err)
			continue
		}
		obj, ok := fact.Object.(string)
		if ok {
			fmt.Printf("  -> %s\n", obj)
			if obj == expectedImport {
				found = true
			}
		}
	}

	if found {
		fmt.Printf("SUCCESS: Import %s resolved correctly!\n", expectedImport)
	} else {
		fmt.Printf("FAILURE: Import %s NOT found in imports list.\n", expectedImport)
	}
}
