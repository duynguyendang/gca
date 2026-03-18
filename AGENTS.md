# AGENTS.md - AI Agent Instructions for GCA

This file provides instructions for AI agents working on the GCA (Gem Code Analysis) codebase.

---

## Project Overview

**GCA (Gem Code Analysis)** is a Neuro-Symbolic Code Analysis Platform that ingests source code into a semantic knowledge graph, enabling powerful queries through Datalog, natural language, and semantic search. It combines symbolic reasoning with neural language models (Gemini AI) for deep code understanding.

### Key Features
- Multi-modal search (Datalog, natural language, semantic)
- Knowledge graph with BadgerDB backend
- Vector embeddings with MRL compression
- AI-powered code analysis and explanations
- MCP (Model Context Protocol) server

---

## Quick Start

### Prerequisites
- Go 1.25+
- Gemini API key (for AI features)
- Git

### 1. Clone and Setup
```bash
git clone https://github.com/your-repo/gca.git
cd gca
cp .env.example .env  # Create environment file
```

### 2. Configure Environment
```bash
# Required for AI features
export GEMINI_API_KEY="your-api-key"

# Optional
export PORT=8080
export LOW_MEM=false
export CORS_ALLOW_ORIGINS="http://localhost:3000"
export USE_OODA_LOOP=true
```

### 3. Build and Run
```bash
# Build
go build -o gca .

# Start server
./gca server

# Or use the convenience script
./local-run.sh
```

### 4. Ingest a Project
```bash
# Ingest a repository
./gca ingest ./path/to/repo ./data/my-project

# Check ingestion status
curl http://localhost:8080/api/v1/projects
```

---

## Development Workflow

### Code Changes

1. **Make changes** in the appropriate package
2. **Build to verify**: `go build ./...`
3. **Run tests**: `go test ./...`
4. **Restart server**: Kill port and restart

```bash
# Quick restart cycle
fuser -k 8080/tcp 2>/dev/null
./gca server &
```

### Debugging API Endpoints

```bash
# Health check
curl http://localhost:8080/api/health

# List projects
curl http://localhost:8080/api/v1/projects

# List files in a project
curl "http://localhost:8080/api/v1/files?project=my-project"

# Query knowledge graph
curl -X POST "http://localhost:8080/api/v1/query?project=my-project" \
  -H "Content-Type: application/json" \
  -d '{"query": "triples(main, _, _)"}'

# Get file source
curl "http://localhost:8080/api/v1/source?id=path/to/file.go&project=my-project"

# Get hydrated symbol (with code)
curl "http://localhost:8080/api/v1/hydrate?id=path/to/file.go&project=my-project"

# Get file call graph
curl "http://localhost:8080/api/v1/graph/file-calls?id=path/to/file.go&project=my-project&depth=2"
```

### Common Issues

| Issue | Solution |
|-------|----------|
| `GEMINI_API_KEY` not set | Export your key before running |
| Port 8080 in use | Run `fuser -k 8080/tcp` to kill existing process |
| Empty query results | Check project is ingested: `/api/v1/projects` |
| Slow queries | Use depth=1 or depth=2 (depth=3 is slow) |
| Hydrate returns empty code | Check ID format matches ingestion (with/without project prefix) |

---

## Deployment

### Local Production Build
```bash
# Build optimized binary
GOOS=linux GOARCH=amd64 go build -o gca-linux-amd64 .

# Or use the Makefile if available
make build
```

### Docker Deployment

```dockerfile
# Dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o gca .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/gca .
COPY --from=builder /app/prompts ./prompts
ENV PORT=8080
EXPOSE 8080
CMD ["./gca", "server"]
```

```bash
# Build and run
docker build -t gca:latest .
docker run -p 8080:8080 -e GEMINI_API_KEY=your-key gca:latest
```

### Environment Variables for Production

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GEMINI_API_KEY` | Yes | - | Google Gemini API key |
| `PORT` | No | 8080 | Server port |
| `DATA_DIR` | No | ./data | Data storage directory |
| `LOW_MEM` | No | false | Enable low-memory mode |
| `CORS_ALLOW_ORIGINS` | No | * | CORS origins (comma-separated) |
| `GEMINI_MODEL` | No | gemini-1.5-flash | Model name |
| `USE_OODA_LOOP` | No | false | Use OODA-based AI dispatch |

### Kubernetes Deployment

```yaml
# k8s/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gca
spec:
  replicas: 1
  selector:
    matchLabels:
      app: gca
  template:
    metadata:
      labels:
        app: gca
    spec:
      containers:
      - name: gca
        image: gca:latest
        ports:
        - containerPort: 8080
        env:
        - name: GEMINI_API_KEY
          valueFrom:
            secretKeyRef:
              name: gca-secrets
              key: gemini-api-key
        resources:
          requests:
            memory: "512Mi"
            cpu: "250m"
          limits:
            memory: "2Gi"
            cpu: "1000m"
---
apiVersion: v1
kind: Service
metadata:
  name: gca
spec:
  selector:
    app: gca
  ports:
  - port: 80
    targetPort: 8080
```

```bash
# Deploy
kubectl apply -f k8s/
```

### Performance Tuning

- **Low-memory mode**: Set `LOW_MEM=true` for constrained environments
- **Query timeout**: Configured in `pkg/config/config.go` (default 30s)
- **Cache**: In-memory cache enabled by default (128MB)
- **Max workers**: Default 2 concurrent workers for ingestion

---

## Codebase Structure

```
gca/
├── cmd/                    # CLI entry points
│   └── gca/main.go        # Main application entry
├── pkg/
│   ├── config/            # Configuration constants
│   ├── datalog/          # Datalog parser & executor
│   ├── ingest/            # Code ingestion pipeline
│   │   ├── extractor.go   # tree-sitter AST extraction
│   │   ├── ingest.go      # Parallel worker orchestration
│   │   └── incremental.go # Incremental updates
│   ├── service/           # Business logic
│   │   ├── graph.go       # Graph queries & operations
│   │   ├── pathfinder.go  # Weighted path finding
│   │   ├── clustering.go  # Graph clustering
│   │   └── ai/
│   │       └── gemini.go # Gemini AI integration
│   ├── server/            # HTTP API handlers
│   │   ├── server.go     # Gin server setup
│   │   └── handlers.go   # Route handlers
│   ├── repl/              # Interactive CLI
│   ├── mcp/               # Model Context Protocol
│   ├── prompts/           # AI prompt templates
│   ├── ooda/              # OODA cognitive loop
│   └── common/            # Shared utilities
├── internal/
│   └── manager/          # Multi-project store manager
├── data/                 # Data storage (gitignored)
├── prompts/              # Prompt template files
└── test/                 # Test cases and scenarios
```

---

## Key Technologies

| Component | Technology |
|-----------|------------|
| Language | Go 1.25+ |
| Web Framework | Gin |
| Database | BadgerDB |
| AI | Google Gemini |
| Parsing | tree-sitter |
| Embeddings | Gemini Embedding (768d→64d MRL) |

---

## Development Commands

### Building
```bash
# Build binary
go build -o gca .

# Build for Docker
docker build -t gca:latest .
```

### Running
```bash
# Ingest a project
./gca ingest ./my-project ./data/my-project

# Incremental ingestion
./gca ingest --incremental ./my-project ./data/my-project

# Start server (correct command)
./gca server

# Start REPL
./gca repl ./data/my-project

# Start MCP server
./gca mcp ./data/my-project
```

### Testing
```bash
# Run all tests
go test ./...

# Run specific package
go test ./pkg/datalog/...
go test ./pkg/service/...

# Run with coverage
go test -cover ./...
```

### Linting
```bash
# Format code
go fmt ./...

# Vet checks
go vet ./...
```

---

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GEMINI_API_KEY` | Yes | Google Gemini API key for embeddings & AI |
| `PORT` | No | Server port (default: 8080) |
| `LOW_MEM` | No | Enable low-memory mode (true/false) |
| `CORS_ALLOW_ORIGINS` | No | Comma-separated CORS origins |
| `GEMINI_MODEL` | No | Gemini model name (default: gemini-3-flash-preview) |
| `USE_OODA_LOOP` | No | Use OODA-based AI dispatch (true/false) |

---

## Architecture Patterns

### 1. Knowledge Graph (SPO)

Facts are stored as Subject-Predicate-Object triples:
```prolog
triples("main.go", "defines", "main")
triples("main", "calls", "fmt.Println")
triples("main.go", "imports", "fmt")
```

### 2. OODA Loop (AI Workflows)

The AI system uses the OODA (Observe-Orient-Decide-Verify-Act) cognitive loop:

```
pkg/ooda/
├── ooda.go             # Core types (GCAFrame, GCALoop)
├── observer.go         # Intent classification + symbol extraction
├── orienter.go         # Graph context retrieval
├── decider.go          # Prompt building
├── verifier_actor.go   # Policy enforcement + execution
└── helpers.go          # Configuration utilities
```

**Phases:**
- **Observe**: Parse query, extract intent and symbols
- **Orient**: Retrieve graph context from MEB store
- **Decide**: Build prompt based on task type
- **Verify**: Enforce policies (max length, allowed tasks)
- **Act**: Execute Gemini call

### Intent Classification

The Observer classifies user queries into these task types:

| Task Type | Trigger Keywords |
|-----------|------------------|
| `insight` | analyze, architectural, design pattern, structure |
| `narrative` | explain flow, trace path, call chain, how work |
| `resolve_symbol` | find handler, resolve symbol, where defined |
| `path_endpoints` | api endpoints, routes, http handlers |
| `datalog` | query datalog, prolog |
| `prune` | top nodes, key components, simplify |
| `summary` | summarize, explain code, file summary |
| `smart_search` | smart search, find similar |
| `multi_file` | multiple files, bulk analyze |
| `refactor` | refactor, improve code, technical debt |
| `test_generation` | test, write test, unit test, coverage |
| `security_audit` | security, vulnerability, audit, permission |
| `performance` | performance, bottleneck, optimize, complexity |
| `chat` | what, how, why (fallback) |

The classifier uses weighted pattern matching with the highest weight wins.

### 3. MEB Store

Memory-Efficient Bidirectional store with:
- Dual indexing (SPO + OPS)
- Dictionary compression
- Vector snapshot persistence

---

## Common Tasks

### Adding a New AI Task

1. Define task in `pkg/ooda/ooda.go`:
```go
const TaskMyTask GCATask = "my_task"
```

2. Add prompt building in `pkg/ooda/decider.go`:
```go
case TaskMyTask:
    return d.buildMyTaskPrompt(frame)
```

3. Add handler in `pkg/service/ai/gemini.go`:
```go
case "my_task":
    // Handle task
```

### Adding a New Predicate

1. Add to virtual predicates in `pkg/ingest/virtual.go`
2. Add query support in `pkg/datalog/`
3. Add test cases in `test/`

### Modifying AI Prompts

Prompt templates are stored in `prompts/`:
- `datalog.prompt` - Datalog query generation
- `chat.prompt` - General conversation
- `smart_search.prompt` - Search result analysis
- `path_narrative.prompt` - Path explanation
- etc.

---

## Testing Guidelines

### Test Structure

| Package | Focus |
|---------|-------|
| `pkg/datalog/` | Parser, query execution |
| `pkg/server/` | HTTP handlers, routing |
| `pkg/service/` | Graph operations, AI integration |
| `pkg/ingest/` | Code extraction, metadata |
| `pkg/repl/` | Command parsing |

### Test File Naming
- Unit tests: `*_test.go`
- Integration tests: `test/integration/`
- Test scenarios: `test/*.md`

### Running Specific Tests
```bash
# Run datalog tests
go test -v ./pkg/datalog/...

# Run with race detector
go test -race ./...

# Run specific test
go test -run TestQueryExecution ./pkg/datalog/...
```

---

## API Reference

### Core Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/health` | Health check |
| GET | `/api/v1/projects` | List all projects |
| GET | `/api/v1/files` | List files in project |
| GET | `/api/v1/symbols` | List symbols in project |
| POST | `/api/v1/query` | Execute Datalog query |
| GET | `/api/v1/source` | Get file source code |
| GET | `/api/v1/hydrate` | Get hydrated symbol (with code + metadata) |
| GET | `/api/v1/graph` | Get full graph |
| GET | `/api/v1/graph/file-calls` | Get file-to-file call graph |
| GET | `/api/v1/graph/backbone` | Get cross-file dependency backbone |
| GET | `/api/v1/graph/path` | Find path between symbols |
| GET | `/api/v1/graph/cluster` | Get graph clusters |
| GET | `/api/v1/semantic-search` | Vector similarity search |
| POST | `/api/v1/ai/ask` | AI-powered analysis |

### Query Parameters

| Endpoint | Parameters |
|----------|------------|
| `/v1/files` | `project` (required) |
| `/v1/source` | `project` (required), `id` (required) |
| `/v1/hydrate` | `project` (required), `id` (required) |
| `/v1/graph/file-calls` | `project` (required), `id` (required), `depth` (1-2) |
| `/v1/query` | `project` (required), `hydrate` (true/false), `raw` (true/false) |

### MCP Tools

| Tool | Description |
|------|-------------|
| `search_nodes` | Search for symbols/files |
| `get_outgoing_edges` | Get dependencies |
| `get_incoming_edges` | Get consumers |
| `get_clusters` | Detect communities |
| `trace_impact_path` | Trace weighted paths |

---

## Important Files

| File | Purpose |
|------|---------|
| `pkg/service/ai/gemini.go` | Main AI service (HandleRequest, HandleRequestOODA) |
| `pkg/ingest/extractor.go` | tree-sitter code extraction |
| `pkg/datalog/parser.go` | Datalog query parser |
| `pkg/service/graph.go` | Graph operations |
| `pkg/server/server.go` | HTTP server setup |
| `internal/manager/store_manager.go` | Project store management |

---

## Dependencies

Key internal dependencies:
- `github.com/duynguyendang/meb` - MEB store
- `github.com/duynguyendang/manglekit` - OODA loop, governance

---

## Notes for AI Agents

1. **Always run `go build ./...`** after making changes to verify compilation
2. **Use existing patterns** - follow the code style in each package
3. **Add tests** for new functionality
4. **Check prompts/** before modifying AI behavior
5. **Environment variables** - `GEMINI_API_KEY` required for AI features
6. **Low-memory mode** - Set `LOW_MEM=true` for constrained environments

---

## Related Documentation

- [README.md](README.md) - Full project documentation
- [docs/ROADMAP.md](ROADMAP.md) - Feature roadmap
- [test/gca-test.md](test/gca-test.md) - Test scenarios
