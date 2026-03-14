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

var PromptPaths = map[string]string{
	"datalog":      "prompts/datalog.prompt",
	"explain":      "prompts/explain_results.prompt",
	"planner":      "prompts/planner.prompt",
	"find_similar": "prompts/find_similar.prompt",
	"extract_code": "prompts/extract_code.prompt",
}
