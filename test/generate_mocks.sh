#!/bin/bash
PROJECT="design-agent"
HOST="http://localhost:8080"
OUT_DIR="test/mock_api"

mkdir -p "$OUT_DIR"

echo "Generating mock data from $HOST for project $PROJECT..."

# 1. Projects
echo "Fetching /v1/projects..."
curl -s "$HOST/v1/projects" > "$OUT_DIR/projects.json"

# 2. Predicates
echo "Fetching /v1/predicates..."
curl -s "$HOST/v1/predicates?project=$PROJECT" > "$OUT_DIR/predicates.json"

# 3. Symbols
echo "Fetching /v1/symbols..."
curl -s "$HOST/v1/symbols?project=$PROJECT&q=design" > "$OUT_DIR/symbols.json"

# 4. Query (Hydrated)
echo "Fetching /v1/query..."
QUERY_BODY='{"query": "triples(?s, \"defines\", ?o), regex(?o, \"WorkflowStatus\")"}'
curl -s -X POST -H "Content-Type: application/json" -d "$QUERY_BODY" \
     "$HOST/v1/query?project=$PROJECT&hydrate=true" > "$OUT_DIR/query.json"

# 5. Source
echo "Fetching /v1/source..."
FILE_ID="design_agent/state.py"
curl -s "$HOST/v1/source?project=$PROJECT&id=$FILE_ID" > "$OUT_DIR/source.txt"

echo "Mock data saved to $OUT_DIR/"
ls -lh "$OUT_DIR"
