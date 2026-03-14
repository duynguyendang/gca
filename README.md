# GCA (Gem Code Analysis)

**Neuro-Symbolic Code Analysis Platform** powered by Knowledge Graphs and Gemini AI

GCA is a next-generation code analysis tool that ingests source code into a semantic knowledge graph, enabling powerful queries through Datalog, natural language, and semantic search. It combines symbolic reasoning with neural language models for deep code understanding.

## Features

### 🔍 **Multi-Modal Search**

#### **1. Datalog Queries**
Precise graph queries with joins, constraints, and regex:
```prolog
triples(A, "calls", B), triples(B, "calls", C)  # Find call chains
triples(?F, "defines", ?S), regex(?F, "handler")  # Find all handlers
```

#### **2. Natural Language**
Ask questions in plain English, auto-converted to Datalog:
```text
"Who calls the panic function?"
"Find all functions that import http"
```

#### **3. Semantic Search**
Find code by meaning using Gemini embeddings:
```bash
GET /v1/semantic-search?project=gca&q=authentication logic&k=10
```
- **768-dimensional embeddings** compressed to **64-d int8** using MRL
- **Sub-300ms** vector similarity search with SIMD optimization
- Matches documentation, not just symbol names

### 🧠 **AI-Powered Analysis**

- **Smart Search Synthesis**: Analyzes query results with full graph context
- **Architectural Insights**: Explains component roles and design patterns
- **Path Narratives**: Traces and explains interaction flows
- **Semantic Passport**: Injects "Identity" metadata (Roles, Layers, Tags) for every symbol
- **Three-Layer Context**: Combines Global (Project), Local (File/Package), and Relational (Graph) context
- **Context-Aware Prompts**: Injects local symbols, relations, and documentation

### 📦 **Code Ingestion**

- **Multi-Language Support**: Go, Python, TypeScript, JavaScript via tree-sitter
- **High-Fidelity Extraction**: Preserves structure, documentation, and relationships
- **Parallel Processing**: Worker pools for fast ingestion (1000+ files/min)
- **Incremental Updates**: Re-ingest without full rebuild (coming soon)

### 🗄️ **Knowledge Graph Storage**

#### **MEB (Memory-Efficient Bidirectional) Store**
- **BadgerDB Backend**: LSM-tree storage for durability
- **Dual Indexing**: SPO and OPS indices for bidirectional queries
- **Dictionary Compression**: String interning reduces memory 10x
- **Embeddable Source**: Code stored directly in graph for portability

#### **Vector Registry (MRL Compression)**
- **768d → 64d int8**: Matryoshka Representation Learning compression
- **SIMD Search**: Vectorized dot product on int8 arrays
- **Snapshot Persistence**: Auto-save compressed vectors to disk
- **String ID Mapping**: Fast lookup from vector IDs to symbol IDs

### 🌐 **RESTful API**

**Discovery**
- `GET /v1/projects` - List all ingested projects
- `GET /v1/files` - List files in a project  
- `GET /v1/predicates` - Get graph schema

**Querying**
- `POST /v1/query` - Execute Datalog queries
- `GET /v1/symbols` - Fuzzy symbol search
- `GET /v1/semantic-search` - Vector similarity search

**Graph Exploration**
- `GET /v1/graph/map` - File-level overview
- `GET /v1/graph/file-details` - Symbol-level details
- `GET /v1/graph/backbone` - Cross-file architecture
- `GET /v1/graph/path` - Shortest path between symbols
- `GET /v1/graph/subgraph` - Retrieve specific subgraph for cluster expansion

*Note: The frontend leverages these endpoints to power its diagnostic **Entropy View**, pinpointing technical debt, test coverage, and code churn.*

**AI Integration**  
- `POST /v1/ai/ask` - Natural language queries with context injection

**Source Code**
- `GET /v1/source` - Retrieve embedded source code

## Architecture

```
gca/
├── cmd/                        # CLI entry points
├── pkg/
│   ├── config/                 # Centralized configuration constants
│   ├── ingest/                # Code ingestion pipeline
│   │   ├── extractor.go       # tree-sitter AST extraction
│   │   ├── ingest.go          # Parallel worker orchestration
│   │   └── incremental.go     # Incremental updates
│   ├── service/               # Business logic layer
│   │   ├── graph.go           # Graph service (queries, paths)
│   │   ├── pathfinder.go      # Weighted path finding
│   │   ├── clustering.go      # Graph clustering
│   │   └── ai/
│   │       └── gemini.go      # Gemini AI integration
│   ├── server/                # HTTP API handlers
│   │   ├── server.go          # Gin server setup
│   │   ├── handlers.go        # Route handlers
│   │   └── handlers_backbone.go
│   ├── datalog/               # Datalog parser & executor
│   ├── repl/                  # Interactive CLI
│   │   ├── repl.go            # Read-Eval-Print Loop
│   │   ├── executor.go        # Plan execution
│   │   └── search.go          # Search functionality
│   ├── mcp/                   # Model Context Protocol server
│   └── common/                # Shared utilities
└── internal/
    └── manager/               # Multi-project store manager
        └── store_manager.go   # Project isolation
```

## Installation

### Prerequisites
- **Go 1.23+**
- **GCC** (for tree-sitter CGO bindings)
- **Gemini API Key** (for AI features and embeddings)

```bash
# Clone repository
git clone https://github.com/duynguyendang/gca.git
cd gca

# Install dependencies
go mod tidy

# Build binary
go build -o gca .
```

## Usage

### 1. Ingest Code

```bash
# Set Gemini API key for documentation embeddings
export GEMINI_API_KEY="your_api_key_here"

# Ingest a project
# Usage: ./gca --ingest <source_folder> <data_folder>
./gca --ingest ./my-project ./data/my-project

# Example: Ingest GCA itself
./gca --ingest ./gca ./data/gca

# Example: Ingest Full Stack (Frontend + Backend)
./gca --ingest ./source-code/gca ./data/gca
```

> [!TIP]
> To skip embedding generation and save memory/API quota, set `LOW_MEM=true` before running ingestion.


**What happens during ingestion:**
1. **Parse**: tree-sitter extracts AST from source files
2. **Extract**: Facts (calls, imports, defines) and documentation are extracted
3. **Embed**: Documentation is embedded using `gemini-embedding-001` (MRL-enabled)
4. **Compress**: 768-d vectors → 64-d int8 using MRL (truncated from larger model outputs)
5. **Store**: Facts saved to BadgerDB, vectors buffered and flushed to disk on graceful shutdown
6. **Index**: SPO and OPS indices built for fast bidirectional queries

**Output:**
```
2026/02/01 13:59:55 INFO vector snapshot saved vectorCount=543 vectorsSizeBytes=34752
2026/02/01 13:59:56 INFO store closed successfully
```


### Project Metadata

To organize your project and enable automatic role tagging, create a `project.yaml` file in the root of your source directory:

```yaml
name: my-project
description: "My awesome project"
version: "1.0.0"
tags: ["go", "backend", "api"]
components:
  backend:
    type: backend
    language: go
    path: cmd/server  # Files in this path will be tagged as 'backend'
  frontend:
    type: frontend
    language: typescript
    path: web/ui      # Files in this path will be tagged as 'frontend'
```

Running ingestion with this file present will:
1. Create a `Project` node with the specified metadata.
2. Automatically tag files with `has_tag` based on the defined components (e.g., `has_tag: backend`).
3. Fallback to generic file extension tagging (e.g., `.go` -> backend) if no component matches.

### 2. Start Server

```bash
# Start REST API server
./gca --server

# Optional: Specify source code path for source view
./gca --server --source ./my-project
```

The server starts on port `8080` by default.

**Logs:**
```
2026/02/01 14:04:02 Gemini Service initialized successfully
[GIN-debug] GET /v1/semantic-search -> handleSemanticSearch
[GIN-debug] Listening and serving HTTP on :8080
```

### 3. Interactive REPL

```bash
# Start interactive query mode
./gca ./data/my-project
```

**Commands:**
```prolog
> triples(?A, "calls", "panic")       # Datalog query
> Who calls panic?                     # Natural language
> show main.go:main                    # View source code
> .schema                              # Show predicates
> .exit                                # Quit
```

### 4. MCP Server (Model Context Protocol)

Run GCA as an MCP server to expose the knowledge graph to AI coding assistants (like Claude Desktop or other MCP clients).

```bash
# Start MCP server (communicates via Stdio)
./gca --mcp ./data/my-project
```

**Features Exposed:**
- **Resources**:
  - `gca://graph/summary`: Graph statistics
  - `gca://files/{path}`: Source code content
  - `gca://schema/conventions`: Architectural schema docs
- **Tools**:
  - `search_nodes`: Search for symbols/files
  - `get_outgoing_edges`: Get dependencies
  - `get_incoming_edges`: Get consumers
  - `get_clusters`: Detect logical communities (Leiden)
  - `trace_impact_path`: Trace weighted paths between nodes


## API Reference

### Semantic Search

**Endpoint:** `GET /v1/semantic-search`

Find symbols by semantic similarity using vector embeddings.

**Parameters:**
- `project`: Project ID (e.g., `gca`)
- `q`: Natural language query
- `k`: Number of results (default: 10)

**Example:**
```bash
curl 'http://localhost:8080/v1/semantic-search?project=gca&q=graph%20pathfinding&k=5'
```

**Response:**
```json
{
  "query": "graph pathfinding",
  "count": 5,
  "results": [
    {
      "symbol_id": "gca/gca-fe/utils/pathfinding.ts:GraphNode",
      "score": 0.6406473,
      "name": "GraphNode"
    },
    {
      "symbol_id": "gca/gca-fe/utils/pathfinding.ts:buildAdjacencyList",
      "score": 0.62595326,
      "name": "buildAdjacencyList"
    }
  ]
}
```

### Smart Search Analysis

**Endpoint:** `POST /v1/ai/ask`

AI-powered query analysis with graph context injection.

**Request:**
```json
{
  "project_id": "gca",
  "task": "smart_search_analysis",
  "query": "who calls handlers?",
  "data": {
    "nodes": [
      {"id": "handlers.go:handleAIAsk", "name": "handleAIAsk", "kind": "func"}
    ],
    "links": [
      {"source": "App.tsx", "target": "handlers.go:handleAIAsk", "relation": "calls"}
    ]
  }
}
```

**Response:**
```json
{
  "answer": "Based on the graph analysis, `handlers.go:handleAIAsk` is called by:\n\n1. **App.tsx** - The frontend component makes HTTP POST requests to `/v1/ai/ask`\n2. **useSmartSearch.ts** - Invokes the AI handler during smart search workflows\n\nThese handlers serve as the API gateway for AI-powered features in the application."
}
```

### Graph Path Finding

**Endpoint:** `GET /v1/graph/path`

Find shortest path between two symbols using weighted BFS.

**Parameters:**
- `project`: Project ID
- `source`: Source symbol ID
- `target`: Target symbol ID

**Example:**
```bash
curl 'http://localhost:8080/v1/graph/path?project=gca&source=main.go:main&target=handlers.go:handleQuery'
```

**Response:**
```json
{
  "nodes": [
    {"id": "main.go:main", "name": "main", "kind": "func"},
    {"id": "server.go:NewServer", "name": "NewServer", "kind": "func"},
    {"id": "handlers.go:handleQuery", "name": "handleQuery", "kind": "func"}
  ],
  "links": [
    {"source": "main.go:main", "target": "server.go:NewServer", "relation": "calls"},
    {"source": "server.go:NewServer", "target": "handlers.go:handleQuery", "relation": "references"}
  ]
}
```

### Cluster Expansion

**Endpoint:** `GET /v1/graph/subgraph`

Retrieve a subgraph for specific nodes (used for expanding clusters).

**Parameters:**
- `project`: Project ID
- `nodes`: Comma-separated list of Node IDs

**Example:**
```bash
curl 'http://localhost:8080/v1/graph/subgraph?project=gca&nodes=node1,node2,node3'
```

**Response:**
```json
{
  "nodes": [...],
  "links": [...]
}
```

## Schema & Predicates

### Core Predicates

| Predicate | Description | Example |
|-----------|-------------|---------|
| `defines` | File defines symbol | `triples("main.go", "defines", "main")` |
| `calls` | Function calls another | `triples("main", "calls", "fmt.Println")` |
| `imports` | File imports package | `triples("main.go", "imports", "fmt")` |
| `has_doc` | Symbol has documentation | `triples("main", "has_doc", ?Doc)` |
| `in_package` | Symbol belongs to package | `triples("Server", "in_package", "server")` |
| `has_role` | Symbol has semantic role | `triples(?X, "has_role", "api_handler")` |
| `has_tag` | Symbol has semantic tag | `triples(?X, "has_tag", "deprecated")` |

### Virtual Predicates

Materialized on-the-fly during queries:

| Predicate | Description |
|-----------|-------------|
| `exposes_model` | API handler exposes data contract |
| `handled_by` | Route is handled by function |
| `propagates_to` | Data flows from source to sink |

## Performance

### Benchmarks (on MacBook Pro M1)

**Ingestion:**
- **GCA Project** (100 files, 15k LOC): 4.2s
- **Mangle Project** (50 files, 8k LOC): 3.8s
- **Rate**: ~250 files/minute

**Query Execution:**
- **Simple triple**: <1ms
- **2-hop join**: 2-5ms
- **3-hop join with constraints**: 10-50ms
- **Semantic search (k=10)**: 200-300ms

**Vector Operations:**
- **Embedding generation**: 50-100ms per batch (Gemini API)
- **MRL compression**: <1µs per vector (in-memory)
- **SIMD search (543 vectors)**: 150µs

**Memory Usage:**
- **GCA Store**: 45MB (27k facts, 543 vectors)
- **Mangle Store**: 110MB (186k facts, 532 vectors)
- **Vector Registry**: ~50KB per 1000 vectors (MRL-compressed)

## Configuration

### Environment Variables

```bash
# Required for embedddings and AI features
export GEMINI_API_KEY="your_gemini_api_key"

# Optional: Server port (default: 8080)
export PORT=8080

# Optional: Low-memory mode (true/false)
# Set to 'true' to skip embedding generation during ingestion 
# and use mmap for vector snapshots during server/repl mode.
export LOW_MEM=true

# Optional: CORS allowed origins (comma-separated)
# Only enforced in production (GIN_MODE=release)
# Example: export CORS_ALLOW_ORIGINS="https://app.example.com,https://dashboard.example.com"
export CORS_ALLOW_ORIGINS=""
```

### Low-Memory Mode

For deployment on constrained environments (e.g., Cloud Run with 1GB RAM), set `LOW_MEM=true`:

```bash
export LOW_MEM=true
./gca --server
```

**GCA Low-mem Strategy:**
- **Skip Embeddings**: During ingestion, if `LOW_MEM=true` is set, the embedding service is skipped, and only metadata indexing is performed.
- **On-demand Loading**: Vector snapshots are memory-mapped (mmap) to keep RAM footprint minimal (sub-100MB for medium projects).
- **Lazy-load Graph**: Only expand sub-graphs based on active search queries.
- **Quantization**: Uses Int8 quantization for compressed vectors.

## Advanced Features

### Virtual Triple Resolver

Automatically infers hidden relationships:

```go
// Detect API handlers by role tag
triples(?Handler, "has_role", "api_handler")

// Find data contracts exposed by handlers
triples(?Handler, "exposes_model", ?Model)

// Trace data propagation
triples(?Source, "propagates_to", ?Sink)
```

**Bridge Logic:**
- Detects API routes (strings starting with `/`)
- Creates `handled_by` edges from routes to handlers
- Infers framework conventions (Gin, Echo, Chi)

### MRL Vector Compression

Matryoshka Representation Learning reduces embedding dimensions while preserving semantic structure:

**Algorithm:**
1. Normalize 768-d Gemini embedding
2. Take first 64 dimensions (MRL property: earlier dims encode more info)
3. Quantize float32 → int8 using min-max scaling
4. Store compressed vector (8x smaller)

**Search:**
```go
// Dot product on int8 arrays using SIMD
score := int8DotProduct(queryVec, candidateVec)
```

**Benefits:**
- **8x smaller**: 768 floats (3KB) → 64 bytes
- **10x faster**: SIMD int8 dot product vs float32
- **95% recall**: Minimal accuracy loss for top-k search

## Deployment

### Docker

```dockerfile
FROM golang:1.23-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY . .
RUN go build -o gca .

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/gca /usr/local/bin/
COPY --from=builder /app/data /data
CMD ["gca", "--server"]
```

```bash
docker build -t gca:latest .
docker run -p 8080:8080 -e GEMINI_API_KEY=$GEMINI_API_KEY gca:latest
```

### Cloud Run

```bash
# Deploy with 1GB RAM
gcloud run deploy gca \
  --source . \
  --platform managed \
  --region us-central1 \
  --memory 1Gi \
  --set-env-vars GEMINI_API_KEY=$GEMINI_API_KEY,LOW_MEM=true
```

## Troubleshooting

### Semantic Search Returns 0 Results

**Cause:** Project was ingested without `GEMINI_API_KEY`

**Fix:**
```bash
export GEMINI_API_KEY="your_key"
rm -rf ./data/my-project  # Clear old data
./gca --ingest ./my-project ./data/my-project  # Re-ingest with embeddings
```

### "Index Out of Range" Panic

**Cause:** Old vector snapshot without stringIDs

**Fix:**
```bash
# Clear vector snapshot
rm ./data/my-project/vectors/sys:mrl:*
./gca --server  # Will rebuild snapshot from full vectors
```

### Out of Memory During Ingestion

**Fix:** Enable low-memory mode or process in batches:
**Fix:** Enable low-memory mode or process in batches:
```bash
LOW_MEM=true ./gca --ingest ./large-project ./data/large-project
```

## Contributing

Contributions are welcome! Areas of focus:

- **Language Support**: Add Java, Rust, C++ tree-sitter grammars
- **Incremental Ingestion**: Detect file changes and re-index only diffs
- **Query Optimization**: Implement join reordering and predicate pushdown
- **Distributed Storage**: Shard large graphs across multiple BadgerDB instances

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

---

## 🚀 Roadmap

This roadmap outlines planned features and improvements for GCA. Items are organized by priority and estimated timeline.

### Phase 1: Core Enhancements (Near-term)

#### 1.1 Incremental Ingestion ✅
- **Description**: Re-ingest only changed files instead of full rebuild
- **Implementation**: Track file hashes, detect changes, update affected triples
- **Benefits**: 10-100x faster updates for large projects
- **Status**: Completed

**Usage:**
```bash
# Full ingestion (first time)
./gca --ingest ./my-project ./data/my-project

# Incremental ingestion (subsequent runs - only processes changed files)
./gca --ingest --incremental ./my-project ./data/my-project
# or
./gca ingest -i ./my-project ./data/my-project
```

#### 1.2 Enhanced Datalog Engine ✅
- **Description**: Add aggregation, sorting, and limit clauses
- **Implementation**: Extend parser to support `count()`, `sum()`, `group by`
- **Benefits**: More powerful queries without AI assistance
- **Status**: Completed

**New Query Syntax:**
```prolog
-- Limit results
triples(?s, "calls", ?o) LIMIT 10

-- Sort results
triples(?s, "calls", ?o) ORDER BY ?s ASC
triples(?s, "calls", ?o) ORDER BY ?s DESC

-- Offset for pagination
triples(?s, "calls", ?o) OFFSET 20 LIMIT 10

-- Group by with aggregation
triples(?s, "calls", ?o) GROUP BY ?s COUNT(?o) AS callCount

-- Sum aggregation
triples(?file, "imports", ?pkg), triples(?pkg, "has_loc", ?loc) GROUP BY ?pkg SUM(?loc) AS totalLines
```

#### 1.3 Query Performance Optimization  
- **Description**: Implement join reordering, predicate pushdown, and query caching
- **Implementation**: Analyze query patterns, optimize execution plans
- **Benefits**: 2-10x faster complex joins
- **Status**: Planned

### Phase 2: Language & Framework Support (Medium-term)

#### 2.1 Extended Language Support
- **Description**: Add tree-sitter parsers for additional languages
- **Priority Languages**:
  - Java (enterprise backends)
  - Rust (systems programming)
  - C/C++ (embedded/performance)
  - C# (.NET ecosystem)
- **Status**: Planned

#### 2.2 Framework-Specific Analysis
- **Description**: Detect and analyze framework-specific patterns
- **Frameworks**:
  - React/Next.js component analysis
  - Spring Boot dependency injection
  - Django/Flask routing
  - Express.js middleware chains
- **Status**: Planned

#### 2.3 Package Manager Integration
- **Description**: Parse `package.json`, `go.mod`, `requirements.txt` for dependency graphs
- **Implementation**: Extract external dependencies, build dependency networks
- **Benefits**: Vulnerability scanning, upgrade planning
- **Status**: Planned

### Phase 3: AI & Intelligence (Medium-term)

#### 3.1 Code Generation Capabilities
- **Description**: Generate unit tests, documentation, refactoring suggestions
- **Implementation**: Prompt engineering for generation tasks
- **Benefits**: Developer productivity boost
- **Status**: Planned

#### 3.2 Semantic Code Navigation
- **Description**: Natural language "find me the auth logic" navigation
- **Implementation**: Enhanced embeddings with code-aware chunking
- **Benefits**: Faster code discovery
- **Status**: Planned

### Phase 4: Scale & Distribution (Long-term)

#### 4.1 Distributed Graph Storage
- **Description**: Shard graphs across multiple BadgerDB instances
- **Implementation**: Consistent hashing for node distribution
- **Benefits**: Support projects with 1M+ files
- **Status**: Planned

#### 4.2 Real-time Collaboration
- **Description**: WebSocket support for live graph updates
- **Implementation**: Event-driven architecture with change streams
- **Benefits**: Team-based code analysis sessions
- **Status**: Planned

#### 4.3 Plugin System
- **Description**: Extensible architecture for custom analyzers
- **Implementation**: Go plugin interface or WASM modules
- **Benefits**: Community-driven analysis capabilities
- **Status**: Planned

---

## 🧪 Testing

GCA uses Go's standard testing framework. Run tests with:

```bash
# Run all tests
go test ./...

# Run specific package tests
go test ./pkg/datalog/...
go test ./pkg/server/...

# Run with coverage
go test -cover ./...

# Run integration tests
go test ./test/integration/...
```

### Test Coverage Areas

| Package | Coverage Focus |
|---------|---------------|
| `pkg/datalog` | Parser, query execution, edge cases |
| `pkg/server` | HTTP handlers, routing, CORS |
| `pkg/service` | Graph operations, pathfinding, AI integration |
| `pkg/ingest` | Code extraction, metadata parsing, incremental updates |
| `pkg/repl` | Command parsing, session management |

### Writing Tests

When adding new features:

1. **Unit Tests**: Test individual functions in `*_test.go` files
2. **Integration Tests**: Add to `test/integration/` for end-to-end scenarios
3. **Benchmark Tests**: Add `Benchmark*` functions for performance-critical code
4. **Property-Based Tests**: Consider for complex transformations

Example test structure:
```go
func TestFeature_Name(t *testing.T) {
    // Arrange
    input := setupTestData()
    
    // Act
    result, err := YourFunction(input)
    
    // Assert
    if err != nil {
        t.Errorf("unexpected error: %v", err)
    }
    if result != expected {
        t.Errorf("got %v, want %v", result, expected)
    }
}
```

---

## License

MIT License - see [LICENSE](LICENSE) for details.

## Citation

```bibtex
@software{gca2024,
  title={GCA: Neuro-Symbolic Code Analysis Platform},
  author={Nguyen, Duy},
  year={2024},
  url={https://github.com/duynguyendang/gca}
}
```
