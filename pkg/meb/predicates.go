package meb

// System Predicates Constants
const (
	PredCalls         = "calls"
	PredCallsAt       = "calls_at"
	PredDefines       = "defines"
	PredDefinesSymbol = "defines_symbol"
	PredEndLine       = "end_line"
	PredFile          = "file"
	PredHasSourceCode = "has_source_code"
	PredHash          = "hash_sha256"
	PredImports       = "imports"
	PredKind          = "kind"
	PredPackage       = "package"
	PredStartLine     = "start_line"
	PredType          = "type"
)

// PredicateMetadata describes a system predicate for documentation.
type PredicateMetadata struct {
	Description string
	Example     string
}

// SystemPredicates maps predicate names to their metadata.
var SystemPredicates = map[string]PredicateMetadata{
	PredCalls:         {"X calls Y", "triples(X, 'calls', Y)"},
	PredCallsAt:       {"Call site line number", "triples(S, 'calls_at', Line)"},
	PredDefines:       {"Custom predicate (legacy/internal)", "triples(S, 'defines', O)"},
	PredDefinesSymbol: {"File defines Symbol", "triples(F, 'defines_symbol', S)"},
	PredEndLine:       {"Symbol end line", "triples(S, 'end_line', L)"},
	PredFile:          {"Symbol defined in File", "triples(S, 'file', F)"},
	PredHasSourceCode: {"Symbol contains raw code", "triples(S, 'has_source_code', C)"},
	PredHash:          {"File content hash", "triples(F, 'hash_sha256', H)"},
	PredImports:       {"File imports Package", "triples(F, 'imports', P)"},
	PredKind:          {"Symbol kind (func, struct, etc.)", "triples(S, 'kind', 'func')"},
	PredPackage:       {"Symbol defined in Package", "triples(S, 'package', P)"},
	PredStartLine:     {"Symbol start line", "triples(S, 'start_line', L)"},
	PredType:          {"Symbol has Type", "triples(S, 'type', T)"},
}
