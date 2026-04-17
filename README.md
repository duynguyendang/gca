# GCA (Gem Code Analysis)

**Master Your Codebase Complexity** — GCA uncovers hidden relationships and architectural patterns, enabling safer refactoring and instant onboarding for complex systems.

## Why GCA?

Reading code is easy; understanding its impact is hard. Traditional tools give you symbols and text search, but they fail to explain the "why" and the "what if" of a codebase.

GCA solves this by transforming your source code into a **Semantic Knowledge Graph**. It doesn't just find keywords; it understands how components interact, allowing you to:

- **Navigate with Certainty**: Map full call trees and detect circular dependencies instantly.
- **Refactor without Fear**: Precisely calculate the "blast radius" of any change.
- **Onboard in Minutes**: Let AI narrate the architectural flow of a new repository using grounded graph data.

## The Core Advantage: Neuro-Symbolic AI

GCA combines the **rigorous logic** of Datalog (via the Mangle engine) with the **intuitive reasoning** of modern LLMs. This "Neuro-Symbolic" approach ensures that AI insights are not just "hallucinations" but are grounded in the actual facts of your code's structure.

## Built for Production

GCA runs efficiently on modest hardware — no external databases or services required:

| Capability | Details |
| --------- | ------- |
| **Low Memory Mode** | `LOW_MEM=true` ingests large projects on limited RAM |
| **Single Binary** | Graph store, vector embeddings, and source content — all in one BadgerDB instance |
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
- **Intent Classification**: 14+ task types (insight, narrative, resolve_symbol, etc.)
- **Path Narratives**: Traces and explains interaction flows
- **Context-Aware Prompts**: Injects local symbols, relations, and documentation

### Code Ingestion

- **Multi-Language Support**: Go, Python, TypeScript, JavaScript via tree-sitter
- **High-Fidelity Extraction**: Preserves structure, documentation, and relationships
- **Parallel Processing**: Worker pools for fast ingestion (1000+ files/min)
- **Incremental Updates**: Re-ingest only changed files
- **Symbol Resolution**: Resolves callee names to symbol IDs for accurate cross-references

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
| Generate Integration Tests | 🔴 TODO | AI-powered integration test generation |
| Architecture Smell Detection | 🔴 TODO | Detect god files, circular deps, hub anomalies |
| Automated Code Review | 🟡 TODO | PR analysis for bugs and security issues |
| Dependency Migration Advisor | 🟡 TODO | Impact analysis for library upgrades |
| Incident Debugging Assistant | 🟡 TODO | Trace errors to source code locations |
| API Contract Analysis | 🟡 TODO | Detect breaking API changes |
| License Compliance Scanning | 🟡 TODO | Scan dependencies for license types |
| Codebase Summarization | 🟡 TODO | Auto-generate README and API docs |
| Test Impact Analysis | 🟡 TODO | Map changed files to affected tests |
| Onboarding Assistant | 🟡 TODO | Guided tours of code architecture |
| Framework Migration | 🟢 TODO | Convert code between languages/frameworks |

## RESTful API

### Discovery

- `GET /api/v1/projects` — List all ingested projects
- `GET /api/v1/files` — List files in a project
- `GET /api/v1/symbols` — List symbols in a project

### Querying

- `POST /api/v1/query` — Execute Datalog queries
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
│   ├── ingest.go              # Ingest command
│   ├── mcp.go                 # MCP server command
│   ├── repl.go                # REPL command
│   ├── root.go                # Root command
│   └── server.go              # Server command
├── pkg/
│   ├── config/                 # Configuration constants
│   ├── constants.go           # Predicate constants (defines, calls, etc.)
│   ├── datalog/               # Datalog parser & executor
│   ├── ingest/                # Code ingestion pipeline
│   │   ├── extractor.go       # tree-sitter AST extraction
│   │   ├── ingest.go          # Parallel worker orchestration
│   │   ├── incremental.go     # Incremental updates
│   │   ├── resolve.go         # Symbol resolution & call graph building
│   │   └── virtual.go         # Virtual predicate enrichment
│   ├── meb/                   # MEB store wrapper
│   ├── ooda/                  # OODA cognitive loop
│   │   ├── ooda.go            # Core types (GCAFrame, GCALoop)
│   │   ├── observer.go        # Intent classification + centrality
│   │   ├── orienter.go        # Context retrieval
│   │   ├── decider.go         # Prompt building
│   │   └── verifier_actor.go  # Policy enforcement
│   ├── service/               # Business logic layer
│   │   ├── ai/                # AI service (Genkit-based)
│   │   ├── graph.go           # Graph operations
│   │   ├── graph_xref.go      # Cross-reference analysis
│   │   ├── centrality.go      # Graph centrality computation
│   │   ├── pathfinder.go      # Weighted path finding
│   │   └── clustering.go      # Graph clustering (Leiden)
│   ├── server/                # HTTP API handlers
│   ├── repl/                  # Interactive CLI
│   └── mcp/                   # Model Context Protocol server
└── internal/
    └── manager/               # Multi-project store manager
```

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

### MCP Server

```bash
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
go test ./...
go test -cover ./...
```

## Built With

| Project | Purpose |
| ------- | ------- |
| [MEB](https://github.com/duynguyendang/meb) | Memory-Efficient Bidirectional graph store — purpose-built for join-heavy code analysis workloads |
| [Mangle](https://github.com/google/mangle) | Datalog extension for deductive database programming — powers the symbolic reasoning engine |

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.
