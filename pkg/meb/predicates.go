package meb

// Core Predicates Constants
const (
	PredDefines    = "defines"    // Ownership (File->Symbol)
	PredCalls      = "calls"      // Execution flow
	PredImports    = "imports"    // Dependency
	PredType       = "type"       // Categorization
	PredImplements = "implements" // Interface fulfillment
	PredHasDoc     = "has_doc"    // Documentation
)

// System/Whitelisted Predicates
const (
	PredHasSourceCode = "has_source_code"
	PredHash          = "hash_sha256"
)

// PredicateMetadata describes a system predicate for documentation.
type PredicateMetadata struct {
	Description string
	Example     string
}

// SystemPredicates maps predicate names to their metadata.
var SystemPredicates = map[string]PredicateMetadata{
	PredDefines:       {"Ownership of a symbol", "triples('parser.go', 'defines', 'ParseFunc')"},
	PredCalls:         {"Execution flow / Invocation", "triples('ParseFunc', 'calls', 'LexerNext')"},
	PredImports:       {"File/Module dependency", "triples('main.go', 'imports', 'pkg/auth')"},
	PredType:          {"Categorization of a node", "triples('UserStruct', 'type', 'struct')"},
	PredImplements:    {"Interface/Contract fulfillment", "triples('MyStore', 'implements', 'IStore')"},
	PredHasDoc:        {"Association with documentation", "triples('AddDocument', 'has_doc', 'Adds a doc...')"},
	PredHasSourceCode: {"Symbol contains raw code", "triples(S, 'has_source_code', C)"},
	PredHash:          {"File content hash", "triples(F, 'hash_sha256', H)"},
}
