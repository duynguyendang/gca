#!/bin/bash
PROJECT="design-agent"
HOST="http://localhost:8080"

echo "Using Project: $PROJECT"

echo "1. Verifying /v1/projects..."
curl -s "$HOST/v1/projects" | grep "$PROJECT" && echo "PASS" || echo "FAIL"

echo "2. Verifying /v1/predicates..."
curl -s "$HOST/v1/predicates?project=$PROJECT" | grep "defines" && echo "PASS" || echo "FAIL"

echo "3. Verifying /v1/symbols..."
curl -s "$HOST/v1/symbols?project=$PROJECT&q=design" | grep "Workflow" && echo "PASS" || echo "FAIL"

echo "4. Verifying /v1/query (Hydration)..."
# Query for WorkflowStatus definition and check if 'code' field is present
QUERY='{"query": "triples(?s, \"defines\", ?o), regex(?o, \"WorkflowStatus\")"}'
curl -s -X POST -H "Content-Type: application/json" -d "$QUERY" \
     "$HOST/v1/query?project=$PROJECT&hydrate=true" > query_response.json

if grep -q '"code":' query_response.json; then
    echo "PASS (Hydrated Code Found)"
else
    echo "FAIL (No Code Field)"
    cat query_response.json
fi

echo "5. Verifying /v1/source..."
# Fetch source for a known file
FILE_ID="design_agent/state.py"
curl -s "$HOST/v1/source?project=$PROJECT&id=$FILE_ID" > source_response.txt
if grep -q "class WorkflowStatus" source_response.txt; then
    echo "PASS (Source Retrieval)"
else
    echo "FAIL (Source Content Missing)"
    head -n 5 source_response.txt
fi

rm query_response.json source_response.txt
