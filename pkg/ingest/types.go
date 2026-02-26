package ingest

import (
	"context"

	"github.com/duynguyendang/meb"
)

// AnalysisBundle holds the results of extracting a file.
// It separates raw documents from relational facts.
type Document struct {
	ID       string
	Content  []byte
	Metadata map[string]any
}

// AnalysisBundle holds the results of extracting a file.
// It separates raw documents from relational facts.
type AnalysisBundle struct {
	Documents []Document
	Facts     []meb.Fact
}

// Extractor is the interface for language-specific content extraction.
type Extractor interface {
	// Extract analyzes the content and returns a bundle of documents and facts.
	Extract(ctx context.Context, path string, content []byte) (*AnalysisBundle, error)
}
