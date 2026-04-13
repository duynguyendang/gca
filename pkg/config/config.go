package config

import "time"

const (
	DefaultPort     = "8080"
	DefaultHost     = "0.0.0.0"
	DefaultGRPCPort = "50051"
)

const (
	DefaultModel          = "gemini-1.5-flash"
	DefaultEmbeddingModel = "gemini-embedding-001"
	DefaultVisionModel    = "gemini-1.5-flash"
	DefaultTemperature    = 0.2
	DefaultMaxTokens      = 2048
)

const (
	QueryTimeout     = 30 * time.Second
	AIRequestTimeout = 120 * time.Second
	EmbeddingTimeout = 10 * time.Second
)

const (
	MaxWorkers           = 2
	AutoClusterThreshold = 500
	ResultCapLimit       = 50
	MaxPathDepth         = 10
	MaxProcessedNodes    = 10000
	MaxBranching         = 50
	SimilarityThreshold  = 0.3
	TopResultsLimit      = 10
	DisplayLimitSmall    = 10
	DisplayLimitMedium   = 15
)

// Query result cache settings
const (
	QueryCacheEnabled      = true
	QueryCacheTTL          = 5 * time.Minute
	QueryCacheMaxSize      = 1000
	QueryResultLimit       = 1000 // Default limit for query results
	QuerySymbolSearchLimit = 100  // Limit for symbol search
	PathFindingMaxNodes    = 500  // Max nodes to visit in path finding
)

const (
	PathfinderEdgeWeightFile     = 1
	PathfinderEdgeWeightDir      = 10
	PathfinderEdgeWeightFunction = 5
	PathfinderDepthLimit         = 3
)

const (
	ClusteringResolution = 0.1
	ClusteringRandomness = 0.01
	ClusteringMaxPasses  = 10
)

const (
	InMemoryCacheSize = 128 << 20 // 128 MB
)

const (
	RetryCount = 3
)

// Validation constants - centralized limits for input validation
const (
	MaxQueryLength       = 200000
	MaxProjectIDLength   = 255
	MaxSymbolIDLength    = 1000
	MaxIDsCount          = 1000
	MaxEmbeddingDim      = 10000
	MaxLimit             = 1000
	MaxOffset            = 1000000
	MaxCursorLength      = 1000
	MaxDepth             = 10
	MaxClusters          = 100
	MaxSearchQueryLength = 500
	MaxPredicateLength   = 100
	MaxPrefixLength      = 500
)

// Supported source file extensions for validation
var SourceFileExtensions = []string{
	".go", ".ts", ".js", ".jsx", ".tsx",
	".py", ".java", ".cpp", ".c", ".rs",
	".swift", ".kt", ".scala", ".rb", ".php",
	".cs", ".vue", ".svelte", ".html", ".css",
}

var PromptPaths = map[string]string{
	"datalog":         "prompts/datalog.prompt",
	"chat":            "prompts/chat.prompt",
	"path_narrative":  "prompts/path_narrative.prompt",
	"path_endpoints":  "prompts/path_endpoints.prompt",
	"resolve_symbol":  "prompts/resolve_symbol.prompt",
	"prune":           "prompts/prune.prompt",
	"smart_search":    "prompts/smart_search.prompt",
	"multi_file":      "prompts/multi_file.prompt",
	"default_context": "prompts/default_context.prompt",
	"explain":         "prompts/explain_results.prompt",
	"planner":         "prompts/planner.prompt",
}
