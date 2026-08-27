package telemetry

import (
	"expvar"
	"time"
)

// GCA-level application metrics exposed via the standard library expvar registry
// (served at /debug/vars). The existing LoggerSink/MetricsSink in this package
// track meb-internal events (circuit breaker, WAL, GC); these counters cover GCA's
// own ingest, query, and MCP activity.
var (
	ingestFilesTotal   = expvar.NewInt("gca_ingest_files_total")
	ingestFilesFailed  = expvar.NewInt("gca_ingest_files_failed")
	queryTotal         = expvar.NewInt("gca_query_total")
	queryErrorsTotal   = expvar.NewInt("gca_query_errors_total")
	queryLatencyNanos  = expvar.NewInt("gca_query_latency_ns")
	mcpQueriesTotal    = expvar.NewInt("gca_mcp_queries_total")
	mcpQueriesErrors   = expvar.NewInt("gca_mcp_query_errors_total")
	mcpInvalidRequests = expvar.NewInt("gca_mcp_invalid_requests_total")
)

func init() {
	// Resolved per-dump so the value stays in sync with the underlying counters.
	expvar.Publish("gca_query_avg_latency_seconds", expvar.Func(func() any {
		if n := queryTotal.Value(); n > 0 {
			return float64(queryLatencyNanos.Value()) / 1e9 / float64(n)
		}
		return 0.0
	}))
}

// IngestFileProcessed records one source file successfully ingested.
func IngestFileProcessed() { ingestFilesTotal.Add(1) }

// IngestFileFailed records one source file that failed to ingest.
func IngestFileFailed() { ingestFilesFailed.Add(1) }

// QueryStarted records a graph/query execution attempt.
func QueryStarted() { queryTotal.Add(1) }

// QueryError records a query execution that returned an error.
func QueryError() { queryErrorsTotal.Add(1) }

// RecordQueryLatency records query execution time.
func RecordQueryLatency(d time.Duration) { queryLatencyNanos.Add(int64(d)) }

// MCPQuery records an MCP tool invocation.
func MCPQuery() { mcpQueriesTotal.Add(1) }

// MCPQueryError records an MCP tool invocation that errored.
func MCPQueryError() { mcpQueriesErrors.Add(1) }

// MCPInvalidRequest records a malformed/blocked MCP request.
func MCPInvalidRequest() { mcpInvalidRequests.Add(1) }
