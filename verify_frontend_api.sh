#!/bin/bash
echo "Verifying /v1/predicates..."
curl -s http://localhost:8080/v1/predicates | grep "defines_symbol" && echo "PASS" || echo "FAIL"

echo "Verifying /v1/symbols..."
# Ingest mock project first? We can reuse existing 'mangle' data if available or create mock
# Assuming 'mangle' exists from user previous interactions.
curl -s "http://localhost:8080/v1/symbols?project=mangle&q=Main" | grep "symbols" && echo "PASS" || echo "FAIL"

echo "Verifying /v1/ai/proxy..."
# Mock AI request
# This requires GEMINI_API_KEY to be set
curl -s -X POST -H "Content-Type: application/json" \
     -d '{"contents":[{"parts":[{"text":"Hello"}]}]}' \
     "http://localhost:8080/v1/ai/proxy?model=gemini-1.5-flash" > ai_response.json

if [ -s ai_response.json ]; then
    echo "PASS (Response received)"
else
    echo "FAIL (Empty response)"
fi
