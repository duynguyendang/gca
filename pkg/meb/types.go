package meb

import (
	"fmt"
)

// DocumentID is a unique identifier for a document in the knowledge base.
// Format: "pkg/auth:Login" (package/path:Symbol)
type DocumentID string

// String returns the string representation of the DocumentID.
func (d DocumentID) String() string {
	return string(d)
}

// Document represents a raw content unit with semantic embeddings.
// It separates the "what" (content) from the "how" (relational facts).
type Document struct {
	ID        DocumentID     `json:"id"`
	Content   []byte         `json:"content,omitempty"`
	Embedding []float32      `json:"embedding,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// Fact represents a single relational logic unit (Quad) for the Datalog engine.
// It separates structural relationships from content.
type Fact struct {
	Subject   DocumentID // The entity being described
	Predicate string     // The relationship type
	Object    any        // The target value (can be another DocumentID or literal)
	Graph     string     // Context/Tenant ID (defaults to "default")
}

// String returns a human-readable representation of the Fact.
func (f Fact) String() string {
	graph := f.Graph
	if graph == "" {
		graph = "default"
	}
	return fmt.Sprintf("<%s, %s, %v> @%s", f.Subject, f.Predicate, f.Object, graph)
}

// IsValid checks if the fact has all required fields.
func (f Fact) IsValid() bool {
	return f.Subject != "" && f.Predicate != "" && f.Object != nil
}

// WithGraph returns a new Fact with the specified graph.
func (f Fact) WithGraph(graph string) Fact {
	f.Graph = graph
	return f
}

// NewFact creates a new Fact with the given subject, predicate, and object.
func NewFact(subject DocumentID, predicate string, object any) Fact {
	return Fact{
		Subject:   subject,
		Predicate: predicate,
		Object:    object,
		Graph:     "default",
	}
}

// NewFactInGraph creates a new Fact in a specific graph.
func NewFactInGraph(subject DocumentID, predicate string, object any, graph string) Fact {
	return Fact{
		Subject:   subject,
		Predicate: predicate,
		Object:    object,
		Graph:     graph,
	}
}

// SymbolStat represents a symbol and its frequency.
type SymbolStat struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}
