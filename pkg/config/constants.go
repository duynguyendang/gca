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
	GenePoolPath = "policies/init.mg" // Seed manifest — single source of truth
	PolicyPath   = "policies"
)

// Role predicates for semantic classification
const (
	RoleDataContract = "data_contract"
	RoleAPIHandler   = "api_handler"
	RoleUtility      = "utility"
	RoleOKFConcept   = "okf_concept"
)

// Architectural tag constants for security smell detection
const (
	TagPublicAPI  = "public_api"
	TagSanitizer  = "sanitizer"
	TagDatabase   = "database"
	TagTestFile   = "test_file"
	TagTestSymbol = "test_symbol"
)

// Additional predicates
const (
	PredicateName             = "name"
	PredicateReferences       = "references"
	PredicateDocumentedBy     = "documented_by"
	PredicateDocumentedHeader = "documented_header"
	PredicateCallsLine        = "calls_line"
)

// Special values
const (
	DefaultPackageRoot = "root"
	TypeDocument       = "document"
)

// Additional predicates for pathfinder and virtual relations
const (
	PredicateCallsAPI        = "calls_api"
	PredicateHandledBy       = "handled_by"
	PredicateExports         = "exports"
	PredicateParentDefines   = "parent_defines"
	PredicateExposesModel    = "exposes_model"
	PredicateCalledBy        = "called_by"
	PredicateHasName         = "has_name"
	PredicateHasSecurityRisk = "has_security_risk"
	PredicateHasHealthScore  = "has_health_score"
	PredicateHasHealthDebt   = "has_health_debt"
	PredicateLastCommitSHA   = "last_commit_sha"
	PredicateSchemaVersion   = "schema_version"
	PredicateKPISnapshot     = "kpi_snapshot"
	PredicateKPISnapshotKey  = "kpi_snapshot_key" // id → timestamp ordering key
)

// SchemaVersion is the current version of the knowledge schema. It is written
// to the store at ingest time and checked by ingest_status to detect stores
// produced by an older version of GCA.
const SchemaVersion = "2.0"

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

// Hub and analysis thresholds
const (
	HubClassificationThreshold     = 5  // Min callers to classify as hub
	ComplexityHigh                 = 15 // Cyclomatic complexity threshold for high
	ComplexityVeryHigh             = 25 // Cyclomatic complexity threshold for very high
	CentralityHighConnectThreshold = 10 // Min calls to flag high-connectivity symbol
	GodFileImportThreshold         = 50 // Min imports to flag god file (excessive imports)
	GodFileDefineThreshold         = 30 // Min definitions to flag god file (excessive defines)
)

// Virtual Attention Sink configuration
const (
	VirtualAttentionThreshold = 0.05  // Minimum centrality score (0-1) to include symbol
	MaxAttentionSymbols       = 8     // Maximum symbols to include in prompt context
	StickyOnlyMode            = false // If true, query only GlobalTopicID (skip Window)
)

// KPI snapshot configuration (F2 health-over-time trends)
const (
	// KPISnapshotRetention is the number of recent KPI snapshots to keep per
	// project. Older snapshots are pruned on each analysis cycle so the
	// Analytical Store stays bounded.
	KPISnapshotRetention = 52
)

// SkippedDirectories lists directory names excluded from filesystem walks during ingestion.
var SkippedDirectories = []string{
	"node_modules", ".git", "dist", "build", ".next",
	"doc-snippets", "docs", "examples", "example",
	"testdata", "__tests__", "__mocks__", "vendor", "coverage", ".cache",
}

// IsSkippedDir returns true if the directory name should be skipped during walks.
func IsSkippedDir(name string) bool {
	for _, d := range SkippedDirectories {
		if name == d {
			return true
		}
	}
	return false
}
