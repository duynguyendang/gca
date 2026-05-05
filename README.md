# GCA (Gem Code Analysis)

**The Shared Brain for Code** — One knowledge graph. Two interfaces. Infinite clarity.

GCA ingests your codebase into a queryable knowledge graph, then exposes it through:
- 🧑‍💻 **GCA Explorer** — Interactive frontend for human exploration (graph visualization, AI chat, health dashboard)
- 🤖 **MCP Server** — Model Context Protocol interface for structured code queries (compatible with Cursor, Claude Desktop, and other MCP clients)

## Your Codebase. Your Rules.

Most tools ship with hardcoded rules. Your architecture? Ignored. Your conventions? Ignored. Your team? Forced to adopt someone else's opinions.

**GCA treats rules as data.** Architecture smells, health scoring, query templates, even how AI understands your code — all defined in plain Datalog files. Add a new smell for your monorepo? Write a `.mg` file. No recompile. No SDK. No Go code.

```prolog
% policies/smells/api_without_auth.mg
query_metadata("smell_unauth_api", "Detect API handlers without authentication").
query_metadata("smell_unauth_api", "severity", "high").

query("smell_unauth_api", Handler, Callee) :-
    triples(Handler, "has_tag", "public_api"),
    triples(Handler, "calls", Callee),
    Callee != "auth.Authenticate".
```
Restart `./gca server`. The smell auto-appears in the dashboard and API.

## Define Your Rules in Datalog

GCA is powered by **Google Mangle** — a Datalog engine that evaluates rules at query time. Every rule is just a `.mg` file. The rules are the product.

### Add a Smell Detection Rule

```prolog
% policies/smells/my_smell.mg
query_metadata("smell_my_pattern", "Detect X in your codebase").
query_metadata("smell_my_pattern", "severity", "high").

query("smell_my_pattern", Subject, Object) :-
    triples(Subject, "calls", Object),
    triples(Object, "has_kind", "database").
```

### Adjust Health Scoring

```prolog
% policies/smells/scoring.mg
% Change penalty weights — no rebuild needed
smell_weight("circular_dependency", 25).  % bump from 10 to 25
smell_weight("layer_violation", 12).       % bump from 8 to 12
```

### Teach AI a New Query Pattern

```prolog
% policies/intent_templates.mg
intent_template("find_dead_code", "default",
    `triples(?s, "has_kind", "func"), not triples(_, "calls", ?s)`).
```

## The Rule Stack

Every layer of GCA's intelligence is a Datalog policy you can edit:

| Layer | Files | What You Control |
|-------|-------|-----------------|
| **Architecture Smells** | `policies/smells/*.mg` (17 total) | Circular deps, god files, hub anomalies, layer violations, security risks, surprise scoring, knowledge gaps |
| **Health Scoring** | `policies/smells/scoring.mg` | Penalty weights per smell, hub thresholds, composite health score (0–100) |
| **Query Registry** | `policies/queries.mg` | Pre-defined queries auto-exposed as REST endpoints |
| **Intent Templates** | `policies/intent_templates.mg` | Natural language → Datalog mapping for AI |
| **Memory Rules** | `policies/memory/*.mg` | Fact promotion/eviction policies |

## Use Cases

- **Auto-scan architecture smells** — 17 smell policies run on every ingest; results surface in the health dashboard
- **Onboard to any codebase** — AI narrates the architecture using centrality data and smell reports
- **Enforce team coding rules** — define layer boundaries, API contracts, auth requirements in `.mg` files
- **Validate PRs** — ephemeral store ingests diffs; smell policies run only on changed files
- **Custom health metrics** — adjust scoring weights to match your team's priorities
- **Teach AI new queries** — add intent templates without touching Go code
- **Query code via MCP** — connect MCP-compatible tools to explore codebase structure

## Neuro-Symbolic AI

GCA combines the **rigorous logic** of Datalog (via the Mangle engine) with the **intuitive reasoning** of modern LLMs. This "Neuro-Symbolic" approach ensures that AI insights are grounded in the actual facts of your code's structure — not hallucinations.

The AI reads diagnostic reports from the Analytical Store, then executes precise Datalog queries against the Source Store. Every answer traces back to actual code.

## Built for Production

GCA runs efficiently on modest hardware — no external databases or services required:

| Capability | Details |
| --------- | --------- |
| **Low Memory Mode** | `LOW_MEM=true` ingests large projects on limited RAM |
| **Single Binary** | Graph, vector, and source content — all in one MEB instance |
| **Zero External Dependencies** | No Elasticsearch, no Neo4j, no Redis — just Go and BadgerDB |
| **Disk Persistence** | Facts and vectors survive restarts |
| **Efficient Storage** | Dictionary compression reduces memory 10x |
| **Vector Compression** | 1536d → int8 hybrid quantization with SIMD acceleration |

## Features

### Multi-Modal Search

GCA offers three complementary ways to query your codebase:

#### Datalog Queries
Precise graph queries with joins, constraints, and regex:
```prolog
triples(A, "calls", B), triples(B, "calls", C)  # Find call chains
triples(?F, "defines", ?S), regex(?F, "handler")  # Find all handlers
```

#### Natural Language
Ask questions in plain English, auto-converted to Datalog:
```
"Who calls the panic function?"
"Find all functions that import http"
```

#### Semantic Search
Find code by meaning using vector embeddings:
```bash
GET /api/v1/semantic-search?project=gca&q=authentication%20logic&k=10
```
- **1536-dimensional embeddings** compressed to **int8** using hybrid block quantization
- **Sub-300ms** vector similarity search with SIMD optimization
- Matches documentation, not just symbol names

### Cross-Reference Analysis

Deep call graph analysis with:

- **Who Calls**: Find all callers of a symbol (backward slice)
- **What Calls**: Find all callees of a symbol (forward slice)
- **Recursive Traversal**: Get full caller/callee trees
- **Cycle Detection**: Find circular dependencies
- **Reachability**: Check if symbol A can reach symbol B
- **LCA**: Find least common ancestor in call graph

### AI-Powered Analysis

#### Multi-LLM Support
Powered by Firebase Genkit with support for multiple providers:

- **Google Gemini** — Default provider
- **OpenAI GPT-4** — via OpenAI API
- **Anthropic Claude** — via Anthropic API
- **MiniMax M2** — via MiniMax OpenAI-compatible API
- **Ollama** — Local LLM support

#### Smart Features

- **Unified NL Pipeline**: Natural language → Datalog → LLM answer
- **Graph Centrality**: Symbols ranked by architectural significance (entry points, hubs, interfaces)
- **Intent Classification**: 11 task types (insight, narrative, resolve_symbol, etc.)
- **Path Narratives**: Traces and explains interaction flows
- **Context-Aware Prompts**: Injects local symbols, relations, and documentation
- **Circuit Breaker**: AI service resilience with automatic failover
- **Idempotent Analytics**: Safe re-runs without duplicate facts or stale data

### Code Ingestion

- **Multi-Language Support**: Go, Python, TypeScript, JavaScript via tree-sitter
- **High-Fidelity Extraction**: Preserves structure, documentation, and relationships
- **Parallel Processing**: Worker pools for fast ingestion (1000+ files/min)
- **Incremental Updates**: Re-ingest only changed files via git diff
- **Symbol Resolution**: Resolves callee names to symbol IDs for accurate cross-references
- **Idempotent Virtual Facts**: Safe re-runs with duplicate prevention
- **Analytics Versioning**: Skip redundant computations on re-ingestion
- **Auto-Scan Smells**: All `policies/smells/*.mg` rules run automatically post-ingest via `analyzer.go`

## Why This Matters for Code Understanding

| Question | Without GCA | With GCA |
|----------|-------------|----------|
| "What calls this function?" | Manual grep, miss indirect callers | Full backward slice, recursive |
| "Find all auth-related code" | Keyword search, many false positives | Semantic search + graph traversal |
| "Will this change break anything?" | Code review guesswork | Blast radius calculation |
| "Explain this codebase to me" | Read files linearly | AI narrates with graph context |

## Planned Features

The following features are planned for future releases:

| Feature | Status | Description |
|---------|--------|-------------|
| Architecture Smell Detection | ✅ DONE | 17 policy files: circular deps, god files, hub anomalies, layer violations, security risks, surprise scoring, knowledge gaps |
| Code Health Dashboard | ✅ DONE | SVG health score, risk leaderboard, metrics radar, smells list |
| Graph Diff for PR Reviews | ✅ DONE | Snapshot comparison and set-difference for code state changes |
| MCP Server | ✅ DONE | Model Context Protocol server for structured code queries |
| Generate Integration Tests | 🟡 TODO | AI-powered integration test generation |
| Automated Code Review | 🟡 TODO | PR analysis for bugs and security issues |
| Dependency Migration Advisor | 🟡 TODO | Impact analysis for library upgrades |
| Incident Debugging Assistant | 🟡 TODO | Trace errors to source code locations |
| API Contract Analysis | 🟡 TODO | Detect breaking API changes |
| License Compliance Scanning | 🟡 TODO | Scan dependencies for license types |
| Codebase Summarization | 🟡 TODO | Auto-generate README and API docs |
| Test Impact Analysis | 🟡 TODO | Map changed files to affected tests |
| Onboarding Assistant | 🟡 TODO | Guided tours of code architecture |
| Framework Migration | 🟢 TODO | Convert code between languages/frameworks |
| Idempotent Analytics | ✅ DONE | Safe re-runs without duplicate facts |
| AI Circuit Breaker | ✅ DONE | Graceful degradation on AI failures |

## RESTful API

### Discovery

- `GET /api/v1/projects` — List all ingested projects
- `GET /api/v1/files` — List files in a project
- `GET /api/v1/symbols` — List symbols in a project

### Querying

- `POST /api/v1/query` — Execute Datalog queries (any `.mg` file in `policies/` auto-registers)
- `GET /api/v1/semantic-search` — Vector similarity search

### Graph Exploration

- `GET /api/v1/graph/file-calls` — File-to-file call graph
- `GET /api/v1/graph/file-backbone` — Cross-file dependency graph
- `GET /api/v1/graph/path` — Shortest path between symbols
- `GET /api/v1/graph/cluster` — Graph clusters (Leiden algorithm)

### Cross-Reference

- `GET /api/v1/graph/who-calls` — Find who calls a symbol (backward slice)
- `GET /api/v1/graph/what-calls` — Find what a symbol calls (forward slice)
- `GET /api/v1/graph/reachable` — Check reachability between symbols
- `GET /api/v1/graph/cycles` — Detect cycles in call graph
- `GET /api/v1/graph/lca` — Find least common ancestor
- `GET /api/v1/graph/centrality` — Get symbols ranked by centrality

### AI Integration

- `POST /api/v1/ask` — Unified NL → Datalog → LLM pipeline

### Source Code

- `GET /api/v1/source` — Retrieve embedded source code
- `GET /api/v1/hydrate` — Get hydrated symbol with code + metadata

## Architecture

```
gca/
├── cmd/                        # CLI entry points
│   ├── root.go                 # Root command, global flags, store creation
│   ├── ingest.go               # Ingest command
│   ├── analyze.go              # Post-ingest analysis command
│   ├── mcp.go                  # MCP server command
│   ├── repl.go                 # Interactive REPL command
│   └── server.go               # HTTP server command
├── pkg/
│   ├── agent/                  # Multi-step reasoning agent (orchestrator, planner, executor, reflector)
│   ├── common/                 # Shared utilities (format, hash, path, query, truncate, errors)
│   ├── config/                 # Configuration constants, tag rules, attention heuristics
│   │   ├── constants.go       # Predicate constants (defines, calls, imports, etc.)
│   │   ├── attention.go       # IsAttentionWorthyName heuristics
│   │   └── tag_rules.go       # TagRule, ProjectTagConfig, pattern matching
│   ├── datalog/               # Datalog parser & executor
│   │   ├── parser.go           # Parse, SmartSplit
│   │   ├── enhanced.go        # ParseEnhanced, ApplyModifiers, ApplyAggregation
│   │   └── optimizer.go       # QueryOptimizer, predicate pushdown
│   ├── ephemeral/             # RAM-only session store for PR reviews
│   ├── export/                # D3 graph export
│   ├── ingest/                # Code ingestion pipeline
│   │   ├── extractor.go      # tree-sitter AST extraction (1131 lines)
│   │   ├── ingest.go          # Parallel worker orchestration (527 lines)
│   │   ├── resolve.go         # Symbol resolution & call graph (464 lines)
│   │   ├── virtual.go         # Virtual predicate enrichment (394 lines)
│   │   ├── incremental.go     # Incremental updates (521 lines)
│   │   └── git.go             # Git diff for incremental ingest
│   ├── llmconfig/             # Multi-LLM provider configuration
│   ├── logger/                # Structured slog-based logging
│   ├── mcp/                   # Model Context Protocol server (476 lines)
│   ├── meb/                   # Unified graph + vector + document store
│   ├── ooda/                  # OODA cognitive loop
│   │   ├── ooda.go           # Core types (GCAFrame, GCALoop)
│   │   ├── observer.go       # Intent classification + centrality
│   │   ├── decider.go        # Prompt building with PromptBuilder
│   │   ├── verifier_actor.go # Policy enforcement, GeminiActor
│   │   └── helpers.go        # Helper utilities
│   ├── promptbuilder/         # Shared prompt assembly (15 task builders)
│   ├── prompts/               # Prompt template loader
│   ├── registry/              # GenePool query registry & template store
│   ├── repl/                  # Interactive CLI (523 lines)
│   ├── server/                # HTTP API handlers (1409 lines in handlers.go)
│   │   └── server.go         # Gin router, middleware, CORS, rate limiting
│   ├── service/               # Business logic layer
│   │   ├── ai/               # AI service (GeminiAdapter, intent, query_gen, synthesize)
│   │   ├── graph.go          # Graph operations
│   │   ├── graph_xref.go     # Cross-reference analysis (who-calls, what-calls)
│   │   ├── graph_pathfinder.go # Weighted path finding (686 lines)
│   │   ├── graph_queries.go  # Semantic search, cycle detection
│   │   ├── graph_backbone.go # Backbone extraction
│   │   ├── graph_clustering.go # Community detection, hybrid clustering
│   │   ├── graph_hydration.go # Lazy node hydration
│   │   ├── graph_diff.go     # Snapshot comparison for PR reviews
│   │   ├── centrality.go     # Degree & PageRank centrality
│   │   ├── clustering.go     # Cluster operations
│   │   └── pathfinder.go     # Path finding with weighting
│   └── telemetry/             # Observability wrappers
├── internal/
│   └── manager/               # Multi-project store manager (StoreManager, EphemeralStore)
├── policies/                   # Datalog policy files
│   ├── smells/               # 17 smell detection policies
│   └── memory/               # Memory promotion rules
└── prompts/                   # LLM prompt templates (*.prompt files)
```

> **GCA Explorer** is the frontend React application. See [gca-fe/](https://github.com/duynguyendang/gca-fe) for the interactive graph UI.

## Installation

### Prerequisites

- **Go 1.25+**
- **GCC** (for tree-sitter CGO bindings)
- **API Key** for AI features (Gemini, OpenAI, Anthropic, or MiniMax)

### Build

```bash
git clone https://github.com/duynguyendang/gca.git
cd gca
go mod tidy
go build -o gca .
```

## Usage

### Ingest Code

```bash
# Set API key
export GEMINI_API_KEY="your_api_key_here"

# Ingest a project
./gca ingest ./my-project ./data/my-project

# Skip embedding generation (faster, saves API quota)
./gca ingest ./my-project ./data/my-project --no-embed

# Use low-memory mode
LOW_MEM=true ./gca ingest ./my-project ./data/my-project
```

### Start Server

```bash
./gca server
# Server starts on port 8080 by default
```

### Interactive REPL

```bash
./gca repl ./data/my-project
# > triples(?A, "calls", "panic")    # Datalog
# > Who calls panic?                    # Natural language
# > show main.go:main                  # View source
# > .exit
```

## MCP Integration (Beta)

GCA exposes its knowledge graph through the [Model Context Protocol](https://modelcontextprotocol.io), enabling MCP-compatible tools to query codebase structure.

### Tools

| Tool | What You Can Query |
|------|-------------------|
| `search_nodes` | Find symbols or files matching a pattern |
| `get_outgoing_edges` | List what a symbol calls |
| `get_incoming_edges` | List what calls a symbol |
| `scan_facts` | Raw Datalog queries against the graph |
| `get_clusters` | Detect logical communities (Leiden algorithm) |
| `trace_impact_path` | Shortest path between two symbols |
| `get_node_metadata` | Metadata for a symbol (kind, package, tags) |

### Resources

| URI | Description |
|-----|-------------|
| `gca://graph/summary` | Project statistics (fact count, etc.) |
| `gca://files/{path}` | Source code content |
| `gca://schema/conventions` | Architectural conventions and node types |

### Usage

```bash
# Start the MCP server (stdio mode — connect via any MCP client)
./gca mcp ./data/my-project
```

## Configuration

### Environment Variables

```bash
# AI Provider
export GEMINI_API_KEY="your_gemini_api_key"
export LLM_PROVIDER="googleai"    # googleai, openai, anthropic, minimax, ollama
export LLM_API_KEY="your_api_key"
export LLM_MODEL=""                # Override default model

# Server
export PORT=8080
export DATA_DIR=./data
export LOW_MEM=true                # Low-memory mode
```

### Multi-LLM Provider Configuration

| Provider | API Key Env | Default Model |
|----------|-------------|---------------|
| googleai | GEMINI_API_KEY | gemini-2.5-flash |
| openai | LLM_API_KEY | gpt-4o |
| anthropic | LLM_API_KEY | claude-3-5-sonnet |
| minimax | LLM_API_KEY | M2-her |
| ollama | (none) | llama3.2 |

## Schema & Predicates

### Core Predicates

| Predicate | Description | Example |
|-----------|-------------|---------|
| `defines` | File defines symbol | `triples("main.go", "defines", "main")` |
| `calls` | Function calls another | `triples("main", "calls", "fmt.Println")` |
| `imports` | File imports package | `triples("main.go", "imports", "fmt")` |
| `has_kind` | Symbol type | `triples("main", "has_kind", "func")` |
| `has_language` | Programming language | `triples("main.go", "has_language", "go")` |
| `called_by` | Inverse of calls | `triples("fmt.Println", "called_by", "main")` |

### Virtual Predicates

| Predicate | Description |
|-----------|-------------|
| `calls_api` | Detected API calls |
| `handled_by` | Route is handled by function |
| `exposes_model` | API handler exposes data contract |

### Architecture Smell Detection Queries

GCA ships with 17 policy files across `policies/smells/` for architecture smell detection:

| Smell | File | Severity |
|-------|------|----------|
| Circular Dependencies (direct) | `circular.mg` | High |
| Transitive Cycles (3-step, 4-step) | `circular.mg` | High |
| God File / God Module | `god_file.mg` | Medium |
| Hub Anomaly | `hub.mg` | Medium |
| Layer Violation | `layer.mg` | High |
| Unsanitized DB Access (recursive) | `security.mg` | Critical |
| Surprise Coupling (cross-community) | `surprise.mg` | Medium |
| Knowledge Gaps (isolated nodes, untested hotspots) | `knowledge_gaps.mg` | Low–High |

Execute via `/api/v1/query`:
```bash
curl -X POST "http://localhost:8080/api/v1/query?project=myproject" \
  -H "Content-Type: application/json" \
  -d '{"query":"smell_circular_direct"}'
```

All smells contribute to composite health scoring via `policies/smells/scoring.mg` — edit the weights there, no rebuild needed.

## Performance

### Benchmarks

#### Small Project (gca-v2): 104 files, 14,044 facts, 50 symbols

| Metric | Value |
| ------ | ----- |
| Ingestion | ~26s with LOW_MEM=true (~240 files/min) |
| Files list | ~69ms |
| Symbols list | ~1.7ms |
| What-calls | ~117ms |
| Who-calls | ~113ms |
| Cycle detection | ~123ms |
| Graph store size | 182 KiB |
| Dictionary size | 584 KiB |

#### Large Project (langchain): 2,536 files, 215,840 facts

| Metric | Value |
| ------ | ----- |
| Ingestion | ~6min with LOW_MEM=true (skip embeddings) |
| Files list | ~113ms |
| Symbols list | ~750ms |
| What-calls (depth=1) | ~10ms |
| Who-calls (depth=1) | ~37ms |
| What-calls (depth>1) | >30s (timed out) |
| Who-calls (depth>1) | >30s (timed out) |
| Cycle detection | >60s (timed out) |

> Note: `depth=1` queries use direct store scan and avoid building the full call graph, making them fast even on large projects.

## Deployment

### Docker

```bash
docker build -t gca:latest .
docker run -p 8080:8080 \
  -e GEMINI_API_KEY=$GEMINI_API_KEY \
  gca:latest
```

### Cloud Run + Firebase

```bash
./deploy.sh
```

## Troubleshooting

### Semantic Search Returns 0 Results

Project was ingested without API key. Re-ingest with:

```bash
rm -rf ./data/my-project
./gca ingest ./my-project ./data/my-project
```

### Out of Memory During Ingestion

```bash
# Skip embeddings entirely
SKIP_EMBEDDINGS=true ./gca ingest ./my-project ./data/my-project

# Or use low-memory mode
LOW_MEM=true ./gca ingest ./my-project ./data/my-project
```

## Testing

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests for specific packages
go test ./pkg/server/... ./pkg/service/... ./pkg/registry/...

# Run tests excluding slow integration tests (uses real data)
go test ./pkg/server/... -run "^Test[^H]"
```

## Built With

| Project | Purpose |
| ------- | ------- |
| [MEB](https://github.com/duynguyendang/meb) | Memory-Efficient Bidirectional graph store — purpose-built for join-heavy code analysis workloads |
| [Mangle](https://github.com/google/mangle) | Datalog extension for deductive database programming — powers the symbolic reasoning engine |

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.
