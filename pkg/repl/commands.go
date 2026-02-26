package repl

import (
	"context"
	"fmt"

	"github.com/duynguyendang/meb"
)

// HandleShow executes the "show" command to display source code of a symbol.
func HandleShow(ctx context.Context, s *meb.MEBStore, arg string) {
	if arg == "" {
		fmt.Println("Usage: show <symbol_id>")
		return
	}

	targetID := string(arg)

	// Fetch document from DocStore
	content, err := s.GetContentByKey(targetID)
	if err != nil {
		fmt.Printf("❌ Failed to get document: %v\n", err)
		return
	}

	metadata, _ := s.GetDocumentMetadata(targetID)

	// Display metadata
	fmt.Printf("📄 Document: %s\n", targetID)
	if len(metadata) > 0 {
		fmt.Println("Metadata:")
		for k, v := range metadata {
			fmt.Printf("  - %s: %v\n", k, v)
		}
	} else {
		fmt.Println("Metadata: [None]")
	}

	// Display content
	if len(content) > 0 {
		fmt.Println("\n--- Source Code ---")
		fmt.Println(string(content))
		fmt.Println("-------------------")
	} else {
		fmt.Println("\n[No content available]")
	}
}
