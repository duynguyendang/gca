package repl

import (
	"context"
	"fmt"

	"github.com/duynguyendang/gca/pkg/meb"
)

// HandleShow executes the "show" command to display source code of a symbol.
func HandleShow(ctx context.Context, s *meb.MEBStore, arg string) {
	if arg == "" {
		fmt.Println("Usage: show <symbol_id>")
		return
	}

	targetID := meb.DocumentID(arg)

	// Fetch document from DocStore
	doc, err := s.GetDocument(targetID)
	if err != nil {
		fmt.Printf("❌ Failed to get document: %v\n", err)
		return
	}

	// Display metadata
	fmt.Printf("📄 Document: %s\n", doc.ID)
	if len(doc.Metadata) > 0 {
		fmt.Println("Metadata:")
		for k, v := range doc.Metadata {
			fmt.Printf("  - %s: %v\n", k, v)
		}
	} else {
		fmt.Println("Metadata: [None]")
	}

	// Display content
	if len(doc.Content) > 0 {
		fmt.Println("\n--- Source Code ---")
		fmt.Println(string(doc.Content))
		fmt.Println("-------------------")
	} else {
		fmt.Println("\n[No content available]")
	}
}
