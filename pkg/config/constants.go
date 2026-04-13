package config

import "time"

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
	PredicateHasDoc      = "has_doc"
	PredicateHasComment  = "has_comment"
	PredicateHasRole     = "has_role"
	PredicateHasTag      = "has_tag"
	PredicateKind        = "kind"
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

// Graph constants
const (
	DefaultGraph = "default"
)

// Note: File extensions are defined in config.go as SourceFileExtensions

// Policy and GenePool paths
const (
	GenePoolPath = "policies/queries.dl"
	PolicyPath   = "policies"
)

// Role predicates for semantic classification
const (
	RoleDataContract = "data_contract"
	RoleAPIHandler   = "api_handler"
	RoleUtility      = "utility"
)

// Additional predicates
const (
	PredicateName       = "name"
	PredicateReferences = "references"
)

// Special values
const (
	DefaultPackageRoot = "root"
	TypeDocument       = "document"
)

// Additional predicates for pathfinder and virtual relations
const (
	PredicateCallsAPI      = "calls_api"
	PredicateHandledBy     = "handled_by"
	PredicateExports       = "exports"
	PredicateParentDefines = "parent_defines"
	PredicateExposesModel  = "exposes_model"
	PredicateCalledBy      = "called_by"
	PredicateHasName       = "has_name"
)

// Centrality configuration
const (
	CentralityEnabled        = true
	CentralityCacheTTL       = 5 * time.Minute
	CentralityBoostIn        = 1.0 // Weight for in-degree (incoming calls)
	CentralityBoostOut       = 1.0 // Weight for out-degree (outgoing calls)
	CentralityBoostMain      = 2.5 // Boost for main/init entry points
	CentralityBoostEntry     = 2.0 // Boost for entry point symbols
	CentralityBoostHub       = 1.5 // Boost for hub nodes (high in+out degree)
	CentralityBoostInterface = 1.3 // Boost for interface-like patterns
)
