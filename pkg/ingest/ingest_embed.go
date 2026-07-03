package ingest

import (
	"strings"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
)

// symbolEmbedTarget holds a symbol ID and text to embed
type symbolEmbedTarget struct {
	symbolID string
	text     string
}

// buildEmbedText constructs embedding text for re-embedding.
// Uses has_name (symbol name), has_doc (doc comment), and content from the bundle.
// The symbolID is used to look up related facts in the bundle.
func buildEmbedText(symbolID string, bundleFacts []meb.Fact, content []byte) string {
	var parts []string

	// Look up name and doc from facts
	var name, doc string
	for _, fact := range bundleFacts {
		if string(fact.Subject) == symbolID {
			if fact.Predicate == config.PredicateHasName {
				if n, ok := fact.Object.(string); ok {
					name = n
				}
			} else if fact.Predicate == config.PredicateHasDoc {
				if d, ok := fact.Object.(string); ok {
					doc = d
				}
			}
		}
	}

	if name != "" {
		parts = append(parts, name)
	}
	if doc != "" {
		parts = append(parts, doc)
	}
	// Add content preview (truncated to avoid bloat)
	if len(content) > 0 {
		contentStr := common.ContentPreview(string(content))
		parts = append(parts, contentStr)
	}

	return strings.Join(parts, "\n---\n")
}
