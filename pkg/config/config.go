package config

import (
	"os"
	"runtime"
	"strconv"
	"time"
)

const (
	DefaultPort = "8080"
	DefaultHost = "0.0.0.0"
)

const (
	DefaultModel          = "gemini-1.5-flash"
	DefaultEmbeddingModel = "gemini-embedding-001"
	DefaultTemperature    = 0.2
	DefaultMaxTokens      = 8192
)

const (
	QueryTimeout     = 30 * time.Second
	AIRequestTimeout = 30 * time.Second // Must be < frontend timeout (35s) and < Cloud Run proxy disconnect (~49s)
	EmbeddingTimeout = 10 * time.Second
)

const (
	AutoClusterThreshold = 500
	ResultCapLimit       = 50
	MaxPathDepth         = 10
	MaxProcessedNodes    = 10000
	MaxBranching         = 50
	SimilarityThreshold  = 0.3
	TopResultsLimit      = 10
	DisplayLimitSmall    = 10
	DisplayLimitMedium   = 15
	// IngestBatchFacts is the number of facts to accumulate before flushing a
	// cross-file batched MEB transaction during ingestion.
	IngestBatchFacts = 20000
)

// ingestWorkersOverride lets a --ingest-workers flag force the ingest worker
// count ahead of the GCA_INGEST_WORKERS env var. 0 = no override.
var ingestWorkersOverride int

// SetIngestWorkers hard-sets the ingest worker count (e.g. from a CLI flag).
// A value <= 0 restores automatic resolution.
func SetIngestWorkers(n int) {
	ingestWorkersOverride = n
}

// IngestWorkers returns the number of concurrent ingest worker goroutines:
// CLI flag override -> GCA_INGEST_WORKERS env var -> default min(4, NumCPU).
func IngestWorkers() int {
	if ingestWorkersOverride > 0 {
		return ingestWorkersOverride
	}
	if s := os.Getenv("GCA_INGEST_WORKERS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	n := runtime.NumCPU()
	if n > 4 {
		return 4
	}
	return n
}

// Query result cache settings
const (
	QueryCacheEnabled      = false // Disabled: cache key doesn't include TopicID
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
	"insight":         "prompts/insight.prompt",
	"summary":         "prompts/summary.prompt",
	"narrative":       "prompts/narrative.prompt",
	"agent_planner":   "prompts/agent_planner.prompt",
	"reflect":         "prompts/reflect.prompt",
	"refactor":        "prompts/refactor.prompt",
	"test_gen":        "prompts/test_gen.prompt",
	"security":        "prompts/security.prompt",
	"performance":     "prompts/performance.prompt",
}
