# GCA (Gem Code Analysis)

**Neuro-Symbolic Code Analysis Platform** powered by Knowledge Graphs and Multi-LLM AI

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
Find code by meaning using vector embeddings:
```bash
GET /api/v1/semantic-search?project=gca&q=authentication%20logic&k=10
```
- **768-dimensional embeddings** compressed to **64-d int8** using MRL
- **Sub-300ms** vector similarity search with SIMD optimization
- Matches documentation, not just symbol names

### 🧠 **AI-Powered Analysis**

#### **Multi-LLM Support**
Powered by Firebase Genkit with support for multiple providers:
- **Google Gemini** - Default provider
- **OpenAI GPT-4** - via OpenAI API
- **Anthropic Claude** - via Anthropic API
- **MiniMax M2** - via MiniMax OpenAI-compatible API
- **Ollama** - Local LLM support

#### **Smart Features**
- **Unified NL Pipeline**: Natural language → Datalog → LLM answer
- **Graph Centrality**: Symbols ranked by architectural significance (entry points, hubs, interfaces)
- **Intent Classification**: 14+ task types (insight, narrative, resolve_symbol, etc.)
- **Cross-Reference Analysis**: "Who calls X?" / "What calls Y?" with recursive traversal
- **Path Narratives**: Traces and explains interaction flows
- **Context-Aware Prompts**: Injects local symbols, relations, and documentation

### 🔗 **Cross-Reference Analysis**

Deep call graph analysis with:
- **Who Calls**: Find all callers of a symbol (backward slice)
- **What Calls**: Find all callees of a symbol (forward slice)
- **Recursive Traversal**: Get full caller/callee trees
- **Cycle Detection**: Find circular dependencies
- **Reachability**: Check if symbol A can reach symbol B
- **LCA**: Find least common ancestor in call graph

### 📦 **Code Ingestion**

- **Multi-Language Support**: Go, Python, TypeScript, JavaScript via tree-sitter
- **High-Fidelity Extraction**: Preserves structure, documentation, and relationships
- **Parallel Processing**: Worker pools for fast ingestion (1000+ files/min)
- **Incremental Updates**: Re-ingest only changed files
- **Symbol Resolution**: Resolves callee names to symbol IDs for accurate cross-references

### 🗄️ **Knowledge Graph Storage**

#### **MEB (Memory-Efficient Bidirectional) Store**
- **BadgerDB Backend**: LSM-tree storage for durability
- **Dual Indexing**: SPO and OPS indices for bidirectional queries
- **Dictionary Compression**: String interning reduces memory 10x
- **Query Cache**: TTL-based caching for frequent queries

#### **Vector Registry (MRL Compression)**
- **768d → 64d int8**: Matryoshka Representation Learning compression
- **SIMD Search**: Vectorized dot product on int8 arrays
- **Snapshot Persistence**: Auto-save compressed vectors to disk

### 🌐 **RESTful API**

**Discovery**
- `GET /api/v1/projects` - List all ingested projects
- `GET /api/v1/files` - List files in a project
- `GET /api/v1/symbols` - List symbols in a project

**Querying**
- `POST /api/v1/query` - Execute Datalog queries
- `GET /api/v1/semantic-search` - Vector similarity search

**Graph Exploration**
- `GET /api/v1/graph/file-calls` - File-to-file call graph
- `GET /api/v1/graph/file-backbone` - Cross-file dependency graph
- `GET /api/v1/graph/path` - Shortest path between symbols
- `GET /api/v1/graph/cluster` - Graph clusters (Leiden algorithm)

**Cross-Reference**
- `GET /api/v1/graph/who-calls` - Find who calls a symbol (backward slice)
- `GET /api/v1/graph/what-calls` - Find what a symbol calls (forward slice)
- `GET /api/v1/graph/reachable` - Check reachability between symbols
- `GET /api/v1/graph/cycles` - Detect cycles in call graph
- `GET /api/v1/graph/lca` - Find least common ancestor
- `GET /api/v1/graph/centrality` - Get symbols ranked by centrality

**AI Integration**
- `POST /api/v1/ai/ask` - Legacy AI-powered analysis
- `POST /api/v1/ask` - Unified NL → Datalog → LLM pipeline

**Source Code**
- `GET /api/v1/source` - Retrieve embedded source code
- `GET /api/v1/hydrate` - Get hydrated symbol with code + metadata

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
│   │   ├── virtual.go         # Virtual predicate enrichment
│   │   └── ...
│   ├── meb/                   # MEB store wrapper
│   │   └── store.go           # Query wrapper, Scan API, QueryCache
│   ├── ooda/                  # OODA cognitive loop
│   │   ├── ooda.go            # Core types (GCAFrame, GCALoop)
│   │   ├── observer.go        # Intent classification + centrality
│   │   ├── orienter.go        # Context retrieval
│   │   ├── decider.go         # Prompt building
│   │   └── verifier_actor.go  # Policy enforcement
│   ├── service/               # Business logic layer
│   │   ├── ai/                # AI service (Genkit-based)
│   │   │   ├── gemini.go      # Main AI service
│   │   │   ├── intent.go      # Intent classification
│   │   │   ├── query_gen.go   # NL → Datalog generation
│   │   │   └── synthesize.go  # Answer synthesis
│   │   ├── graph.go           # Graph operations
│   │   ├── graph_xref.go      # Cross-reference analysis
│   │   ├── centrality.go      # Graph centrality computation
│   │   ├── pathfinder.go      # Weighted path finding
│   │   ├── clustering.go      # Graph clustering (Leiden)
│   │   └── ...
│   ├── server/                # HTTP API handlers
│   │   ├── server.go          # Gin server setup
│   │   ├── handlers.go        # Route handlers
│   │   └── ...
│   ├── repl/                  # Interactive CLI
│   ├── mcp/                   # Model Context Protocol server
│   └── common/                # Shared utilities
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
# Set API key for embeddings and AI features
export GEMINI_API_KEY="your_api_key_here"

# Ingest a project
./gca ingest ./my-project ./data/my-project

# Example: Ingest GCA itself
./gca ingest ./gca ./data/gca
```

> [!TIP]
> To skip embedding generation and save memory/API quota, set `LOW_MEM=true` before running ingestion.

**What happens during ingestion:**
1. **Parse**: tree-sitter extracts AST from source files
2. **Extract**: Facts (calls, imports, defines) and documentation are extracted
3. **Resolve**: Symbol resolution builds call graph with resolved symbol IDs
4. **Embed**: Documentation is embedded using Gemini embeddings
5. **Compress**: 768-d vectors → 64-d int8 using MRL
6. **Store**: Facts saved to BadgerDB, vectors flushed to disk on shutdown

### 2. Start Server

```bash
# Start REST API server
./gca server

# Server starts on port 8080 by default
# Environment variables:
#   PORT=8080
#   LLM_PROVIDER=googleai  # or openai, anthropic, minimax, ollama
#   LLM_API_KEY=your_key
```

### 3. Interactive REPL

```bash
# Start interactive query mode
./gca repl ./data/my-project

# Commands:
# > triples(?A, "calls", "panic")    # Datalog query
# > Who calls panic?                  # Natural language
# > show main.go:main                 # View source code
# > .schema                           # Show predicates
# > .exit                             # Quit
```

### 4. MCP Server

```bash
# Start MCP server for AI coding assistants
./gca mcp ./data/my-project
```

## API Reference

### Unified NL → Datalog → LLM Pipeline

**Endpoint:** `POST /api/v1/ask`

Single endpoint for natural language code analysis:

**Request:**
```json
{
  "project_id": "gca",
  "query": "How does authentication work?",
  "symbol_id": "auth.go:Authenticate",
  "depth": 2
}
```

**Response:**
```json
{
  "answer": "Based on the analysis...",
  "query": "triples(?s, 'calls', 'auth.go:Authenticate')",
  "intent": "narrative",
  "confidence": 0.95,
  "results": [...]
}
```

### Cross-Reference Analysis

**Who Calls (Backward Slice):**
```bash
curl 'http://localhost:8080/api/v1/graph/who-calls?project=gca&symbol=main.go:main'
```

**What Calls (Forward Slice):**
```bash
curl 'http://localhost:8080/api/v1/graph/what-calls?project=gca&symbol=main.go:main'
```

**Recursive Callers:**
```bash
curl 'http://localhost:8080/api/v1/graph/who-calls?project=gca&symbol=main.go:main&recursive=true&depth=3'
```

**Cycle Detection:**
```bash
curl 'http://localhost:8080/api/v1/graph/cycles?project=gca'
```

**Reachability Check:**
```bash
curl 'http://localhost:8080/api/v1/graph/reachable?project=gca&from=main.go:main&to=handlers.go:handleQuery'
```

### Graph Centrality

**Get Top Symbols by Centrality:**
```bash
curl 'http://localhost:8080/api/v1/graph/centrality?project=gca&limit=20'
```

Returns symbols ranked by architectural importance:
- Entry points (main, init) boosted 2.5x
- Hub nodes (high in+out degree) boosted 1.5x
- Interface-like patterns boosted 1.3x

## Configuration

### Environment Variables

```bash
# AI Provider Configuration
export GEMINI_API_KEY="your_gemini_api_key"  # Default AI provider
export LLM_PROVIDER="googleai"                # googleai, openai, anthropic, minimax, ollama
export LLM_API_KEY="your_api_key"            # Override default provider's API key
export LLM_MODEL=""                          # Override default model

# Server Configuration
export PORT=8080
export DATA_DIR=./data
export LOW_MEM=true                           # Low-memory mode
export CORS_ALLOW_ORIGINS="https://app.example.com"

# AI Behavior
export USE_OODA_LOOP=true                    # Use OODA-based AI dispatch
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

### Benchmarks (gca-v2 project: 104 files, 14,044 facts, 50 symbols)

**Ingestion:**
- **gca-v2 Project** (104 files, 14,044 facts): ~26s with LOW_MEM=true
- **Rate**: ~240 files/minute

**Query Execution:**
| Endpoint | Time | Results |
|----------|------|---------|
| Files list | ~69ms | 104 files |
| Symbols list | ~1.7ms | 50 symbols |
| What-calls | ~117ms | returns callers |
| Who-calls | ~113ms | returns callees |
| Cycle detection | ~123ms | detects cycles |

**Store Size:**
- **Graph store**: 182 KiB
- **Dictionary**: 584 KiB

**Memory Usage:**
- **Query Cache**: 5-min TTL, configurable max entries

## Deployment

### Docker

```bash
docker build -t gca:latest .
docker run -p 8080:8080 \
  -e GEMINI_API_KEY=$GEMINI_API_KEY \
  -e LLM_PROVIDER=googleai \
  gca:latest
```

### Cloud Run + Firebase

```bash
# Deploy backend to Cloud Run
./deploy.sh

# Frontend auto-deploys to Firebase
```

## Troubleshooting

### Semantic Search Returns 0 Results

**Cause:** Project was ingested without API key

**Fix:**
```bash
rm -rf ./data/my-project
./gca ingest ./my-project ./data/my-project
```

### Out of Memory During Ingestion

```bash
LOW_MEM=true ./gca ingest ./large-project ./data/large-project
```

### Cross-Reference Queries Return 0 Results

**Fixed.** The bug in `pkg/meb/store.go:buildLFTJRelations()` was in the dictionary ID packing for bound positions in LFTJ joins. The fix changes:

```go
// Before:
packedID := keys.PackID(topicID, keys.UnpackLocalID(dictID))

// After (correct):
packedID := keys.PackID(topicID, dictID)
```

If you still see 0 results, verify the project was re-ingested after the fix was applied.

## Testing

```bash
# Run all tests
go test ./...

# Run specific package tests
go test ./pkg/datalog/...
go test ./pkg/server/...
go test ./pkg/service/...

# Run with coverage
go test -cover ./...
```

## Recent Changes

### Graph Centrality (April 2026)
- Symbols ranked by architectural significance in AI context
- Entry points, hubs, and interfaces prioritized

### Multi-LLM Support (April 2026)
- Added MiniMax, OpenAI, Anthropic, Ollama providers via Genkit

### Cross-Reference Analysis (April 2026)
- Who-calls, what-calls, reachability, cycles, LCA endpoints
- Symbol resolution for accurate call graphs

### Unified NL Pipeline (April 2026)
- Single `POST /api/v1/ask` endpoint
- Intent classification + Datalog generation + LLM answer

## License

MIT License - see [LICENSE](LICENSE) for details.
