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

**AI Integration**  
- `POST /v1/ai/ask` - Natural language queries with context injection

**Source Code**
- `GET /v1/source` - Retrieve embedded source code

## Architecture

```
gca/
├── cmd/                        # CLI entry points
├── pkg/
│   ├── ingest/                # Code ingestion pipeline
│   │   ├── extractor.go       # tree-sitter AST extraction
│   │   ├── ingest.go          # Parallel worker orchestration
│   │   └── bridge/            # Package-level relationship inference
│   ├── meb/                   # Knowledge graph storage
│   │   ├── store.go           # Main MEB store
│   │   ├── query_builder.go  # Datalog query engine
│   │   ├── dictionary.go      # String compression
│   │   └── vector/            # Vector registry & MRL
│   │       ├── registry.go    # In-memory vector store
│   │       ├── storage.go     # BadgerDB persistence
│   │       └── mrl.go         # MRL compression
│   ├── service/               # Business logic layer
│   │   ├── graph.go           # Graph service (queries, paths)
│   │   ├── virtual.go         # Virtual triple materialization
│   │   └── ai/
│   │       └── gemini.go      # Gemini AI integration
│   ├── server/                # HTTP API handlers
│   │   ├── server.go          # Gin server setup
│   │   └── handlers.go        # Route handlers
│   ├── datalog/               # Datalog parser & executor
│   │   ├── parser.go          # Query parsing
│   │   └── executor.go        # Query execution
│   └── repl/                  # Interactive CLI
│       └── repl.go            # Read-Eval-Print Loop
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
go mod download

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
```

**What happens during ingestion:**
1. **Parse**: tree-sitter extracts AST from source files
2. **Extract**: Facts (calls, imports, defines) and documentation are extracted
3. **Embed**: Documentation is embedded using Gemini `text-embedding-004`
4. **Compress**: 768-d vectors → 64-d int8 using MRL
5. **Store**: Facts saved to BadgerDB, vectors to snapshot
6. **Index**: SPO and OPS indices built for fast queries

**Output:**
```
2026/02/01 13:59:55 INFO vector snapshot saved vectorCount=543 vectorsSizeBytes=34752
2026/02/01 13:59:56 INFO store closed successfully
```

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

# Optional: Low-memory mode (Cloud Run, 1GB RAM)
export LOW_MEM=true
```

### Low-Memory Mode

For deployment on constrained environments (e.g., Cloud Run with 1GB RAM):

```bash
./gca --server --low-mem
```

**Changes in low-mem mode:**
- Reduced BadgerDB cache sizes
- Disabled source code embedding
- Lazy-load vector snapshots
- Stream large query results

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
```bash
./gca --ingest ./large-project ./data/large-project --low-mem
```

## Contributing

Contributions are welcome! Areas of focus:

- **Language Support**: Add Java, Rust, C++ tree-sitter grammars
- **Incremental Ingestion**: Detect file changes and re-index only diffs
- **Query Optimization**: Implement join reordering and predicate pushdown
- **Distributed Storage**: Shard large graphs across multiple BadgerDB instances

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

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
