package config

// Predicate constants used throughout the codebase
const (
	PredicateDefines     = "defines"
	PredicateCalls       = "calls"
	PredicateImports     = "imports"
	PredicateType        = "type"
	PredicateHasKind     = "has_kind"
	PredicateHasLanguage = "has_language"
	PredicateStartLine   = "start_line"
	PredicateEndLine     = "end_line"
	PredicateInPackage   = "in_package"
)

// File depth limits
const (
	DefaultFileDepthLimit = 2
	MaxFileDepthLimit     = 2
)

// Cache configuration
const (
	CacheTTLSeconds = 300 // 5 minutes default cache TTL
)

// Graph clustering thresholds
const (
	MinNodesForClustering = 500
)

// File path validation
const (
	MaxPackageFilesToResolve = 10
)

// Virtual relation types
const (
	VirtualRelationWiresTo          = "v:wires_to"
	VirtualRelationPotentiallyCalls = "v:potentially_calls"
)

// File type constants
const (
	FileTypeFile = "file"
)

// Symbol kind constants
const (
	SymbolKindFunc      = "func"
	SymbolKindMethod    = "method"
	SymbolKindStruct    = "struct"
	SymbolKindInterface = "interface"
	SymbolKindFile      = "file"
	SymbolKindCluster   = "cluster"
	SymbolKindGateway   = "gateway"
	SymbolKindSymbol    = "symbol"
)

// Relation types
const (
	RelationCalls      = "calls"
	RelationCallsFile  = "calls_file"
	RelationAggregated = "aggregated"
	RelationImports    = "imports"
	RelationDefines    = "defines"
)

// Default limits
const (
	DefaultSearchLimit       = 50
	DefaultVectorSearchLimit = 10
)

// Note: File extensions are defined in config.go as SourceFileExtensions
