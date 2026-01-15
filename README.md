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
-   **Interactive REPL**: Explore your codebase interactively with autocomplete and history.

## Architecture

-   **`pkg/ingest`**: Handles parsing of source code and extraction of symbols into facts.
-   **`pkg/meb`**: The core graph storage engine (MEBStore) handling fact persistence and indexing.
-   **`pkg/datalog`**: Custom Datalog parser for query processing.
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

### 2. Interactive Query Mode

Start the REPL to query the ingested data.

```bash
# Make sure GEMINI_API_KEY is set for AI features
export GEMINI_API_KEY="your_api_key_here"

./gca ./data
```

### 3. Example Queries

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
-   `imports(File, Package)`: Package dependencies.
-   `defines_struct(File, StructName)`: Struct definitions.
-   `follows(Person, Person)`: (Example/Demo predicate)
-   `interest(Person, Topic)`: (Example/Demo predicate)
