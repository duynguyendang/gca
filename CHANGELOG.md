# Changelog

All notable changes to the GCA backend are documented in this file.

The format loosely follows [Keep a Changelog](https://keepachangelog.com/).
Releases are tagged with git tags (this project has no CI-automated release
pipeline).

## [0.5.0] - 2026-08-27

### Performance & Memory
- **meb v0.7.0 WriteBatch.** Upgraded `github.com/duynguyendang/meb` to v0.7.0
  and rewired the ingest fact writer to its `BatchWriter` (BadgerDB WriteBatch),
  amortizing fsync/commit across many facts instead of per-file `Update`
  transactions.
- **Reduced ingestion memory.** Removed `IngestState.FileContentCache`; Pass 2
  re-reads files from disk on demand, bounding memory on large codebases.
- **Configurable worker pool.** Replaced the fixed `MaxWorkers=2` cap with
  `config.IngestWorkers()` — `--ingest-workers` flag / `GCA_INGEST_WORKERS` env
  / default `min(4, NumCPU)`. Applies to full and incremental ingest.

### Robustness
- **Cancellable ingestion.** Added `RunWithContext` / `RunIncrementalWithContext`
  that honor context cancellation across Pass 1 and Pass 2; `gca ingest` now
  passes a signal context so Ctrl+C aborts parsing cleanly.
- **Graceful MCP stdio shutdown.** `RunStdio` returns on context cancellation so
  the `gca mcp` command flushes the WAL and closes stores on SIGINT/SIGTERM.

### Observability
- **expvar metrics.** New `pkg/telemetry/metrics.go` publishing GCA-level
  counters (`gca_ingest_files_total`, `gca_query_total`,
  `gca_query_latency_ns`, `gca_query_avg_latency_seconds`, MCP query counters…).
  Wired into ingest, the query layer, and MCP tool handlers.
- **`/api/metrics`** now serves the live expvar registry (was a stub).

### Misc
- Bumped the MCP server version string to `0.5.0`.

## [0.4.0] - 2026-08-15

### Misc
- Test infrastructure: isolated git-diff resolution in a throwaway repo for the
  incremental ingest path.

---

Note: earlier tags (`0.1`, `0.2`, `0.3`) predate this changelog; see `git log`
for their commit history.