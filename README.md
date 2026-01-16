# GCA (Gem Code Analysis)

GCA is a knowledge graph-powered code analysis tool that allows you to query your Go codebase using Datalog and Natural Language. It ingests Go source code, builds a Semantic Knowledge Graph (Subject-Predicate-Object), and provides a neuro-symbolic query interface.

## Features

-   **Code Ingestion**: Uses `tree-sitter` to parse Go code and extract facts (function calls, imports, struct definitions, etc.).
-   **Knowledge Graph**: Stores facts in a highly optimized, memory-efficient graph database (`MEB`) backed by BadgerDB.
-   **Datalog Query Engine**: Built-in Datalog parser and execution engine supporting:
    -   Triples: `triples(S, P, O)`
    -   Joins: Multi-hop queries (e.g., `triples(A, "calls", B), triples(B, "calls", C)`)
    -   Constraints: `regex(Var, "pattern")`, Inequalities (`A != B`)
-   **AI-Powered Querying**: Integrates with Google's Gemini models to translate Natural Language questions into Datalog queries automatically.
-   **RESTful API**: Exposes a full-featured API for discovery, querying, and source code retrieval.
-   **Zero-Dependency Serving**: Embeds source code directly into the knowledge graph, allowing the server to operate without access to the original source files ("portability").
-   **Interactive REPL**: Explore your codebase interactively with autocomplete and history.

## Architecture

-   **`pkg/ingest`**: Handles parsing of source code and extraction of symbols into facts.
-   **`pkg/meb`**: The core graph storage engine (MEBStore) handling fact persistence and indexing.
-   **`pkg/datalog`**: Custom Datalog parser for query processing.
-   **`pkg/server`**: REST API server implementation for stateless project querying.
-   **`pkg/repl`**: Interactive Read-Eval-Print Loop for querying the graph.

## Installation

Prerequisites:
-   Go 1.23+
-   `gcc` (for tree-sitter bindings)
-   Gemini API Key (for NL features)

```bash
# Clone the repo
git clone https://github.com/duynguyendang/gca.git
cd gca

# Build the binary
go build -o gca .
```

## Usage

### 1. Ingest Code

First, ingest a target Go project into the knowledge graph.

```bash
# Usage: ./gca --ingest <source_code_path> <data_storage_path>
./gca --ingest ./my-go-project ./data
```

### 2. Start the API Server
Start the HTTP server to serve the API endpoints.

```bash
# Usage: ./gca --server [source_code_path]
# source_code_path is optional; server uses embedded source code from the DB.
./gca --server
```

The server will start on port `8080`.

### 3. Interactive Query Mode

Start the REPL to query the ingested data locally.

```bash
# Make sure GEMINI_API_KEY is set for AI features
export GEMINI_API_KEY="your_api_key_here"

./gca ./data
```

### 4. Example Queries

Once in the REPL, you can ask questions in Datalog or Natural Language.

**Datalog:**
```prolog
> triples(A, "calls", "panic")
> triples(A, "imports", "fmt"), regex(A, "main.go")
> triples(A, "calls", B), triples(B, "calls", C), A != C
```

**Natural Language (via Gemini):**
```text
> Who calls panic?
> Find all functions in main.go that import "fmt"
> Find cycles of length 3
```

## Schema

The current schema includes the following predicates:
-   `calls(Caller, Callee)`: Function usage.
-   `calls_at(Caller, Line)`: Line number of a call.
-   `defines_symbol(File, Symbol)`: File defines a symbol.
-   `has_source_code(Symbol, Code)`: Raw source code.
-   `imports(File, Package)`: Package dependencies.
-   `kind(Symbol, Kind)`: Symbol kind (e.g., `func`, `struct`).
-   `type(Symbol, Type)`: Type information.
-   `file(Symbol, File)`: Reverse lookup.
-   `package(Symbol, Package)`: Package membership.
-   `start_line/end_line(Symbol, Line)`: Source positioning.

## HTTP API Reference

The GCA server exposes a RESTful API for project discovery, querying, and source code retrieval.

### 1. Discovery API
**Endpoint:** `GET /v1/projects`

Lists all available projects that have been ingested.

**Response:**
```json
["project_a", "project_b"]
```

### 2. Query API
**Endpoint:** `POST /v1/query`

Executes a Datalog query against the specified project's knowledge graph.

**Query Parameters:**
- `project`: The ID of the project to query (e.g., `my-project`).

**Request Body:**
```json
{
  "query": "triples(?s, \"calls\", ?o)"
}
```

**Response:**
Returns a JSON object representing the graph, suitable for D3.js visualization.
```json
{
  "nodes": [
    {
      "id": "main.go:main",
      "name": "main",
      "kind": "func",
      "group": "main",
      "code": "func main() { ... }"
    }
  ],
  "links": [
    {
      "source": "main.go:main",
      "target": "pkg/foo:Bar",
      "relation": "calls"
    }
  ]
}
```

### 3. Source Code API
**Endpoint:** `GET /v1/source`

Retrieves the raw source code for a specific symbol or file from the knowledge store. This endpoint works even if the original source files are not present on the server.

**Query Parameters:**
- `project`: The ID of the project.
- `id`: The unique ID of the symbol or file (e.g., `main.go:main`).
- `start`: (Optional) Start line number (1-based).
- `end`: (Optional) End line number.

**Response:**
Plain text content of the requested source code.

### 4. Summary API
**Endpoint:** `GET /v1/summary`

Generates a high-level statistical summary of the project.

**Query Parameters:**
- `project`: The ID of the project.

**Response:**
```json
{
  "total_facts": 1250,
  "unique_predicates": ["calls", "imports", "defines"],
  "top_symbols": ["main", "ServeHTTP"],
  "stats": { ... }
}
```

### 5. Predicate Discovery API
**Endpoint:** `GET /v1/predicates`

Returns the graph schema (list of active predicates) with descriptions and usage examples.

**Query Parameters:**
- `project`: The ID of the project.

**Response:**
```json
{
  "predicates": [
    {
      "name": "calls",
      "description": "X calls Y",
      "example": "triples(X, 'calls', Y)"
    },
    ...
  ]
}
```

### 6. Symbol Search API
**Endpoint:** `GET /v1/symbols`

Provides fast prefix-based symbol search and autocomplete.

**Query Parameters:**
- `project`: The ID of the project.
- `q`: (Optional) Search query string (prefix). If omitted, returns top symbols.
- `p`: (Optional) Predicate filter (default: `defines_symbol`).

**Response:**
```json
{
  "symbols": [
    "main.go:main",
    "pkg/server:NewServer",
    ...
  ]
}
```
