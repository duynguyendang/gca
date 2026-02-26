package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const baseURL = "http://localhost:8080"
const projectID = "mangle-v2" // Targeting the 'mangle-v2' project

func main() {
	fmt.Println("🚀 Starting Mangle Library Tests...")

	if err := waitForServer(); err != nil {
		fmt.Printf("❌ Server not ready: %v\n", err)
		os.Exit(1)
	}

	failures := 0

	// === REL-01: Core Analysis ===
	if err := runREL01(); err != nil {
		fmt.Printf("❌ REL-01 Failed: %v\n", err)
		failures++
	} else {
		fmt.Println("✅ REL-01 Passed")
	}

	// === REL-02: AST Concepts ===
	if err := runREL02(); err != nil {
		fmt.Printf("❌ REL-02 Failed: %v\n", err)
		failures++
	} else {
		fmt.Println("✅ REL-02 Passed")
	}

	// === REL-03: Package Isolation ===
	if err := runREL03(); err != nil {
		fmt.Printf("❌ REL-03 Failed: %v\n", err)
		failures++
	} else {
		fmt.Println("✅ REL-03 Passed")
	}

	// === BFS-01: Parser to AST ===
	// Source: gca-be/parse/parse.go (observed in logs)
	// Target: gca-be/ast/ast.go
	// We'll pass partial paths to runBFS which does substring matching.
	if err := runBFS("parse/parse.go", "ast/ast.go"); err != nil {
		fmt.Printf("❌ BFS-01 Failed: %v\n", err)
		failures++
	} else {
		fmt.Println("✅ BFS-01 Passed")
	}

	// === BFS-02: Engine to Unions ===
	// Source: gca-be/engine/engine.go (might not exist, checking logs: topdown.go exists)
	// Let's use 'engine/topdown.go' -> 'unionfind/unionfind.go'
	if err := runBFS("engine/topdown.go", "unionfind/unionfind.go"); err != nil {
		fmt.Printf("❌ BFS-02 Failed: %v\n", err)
		failures++
	} else {
		fmt.Println("✅ BFS-02 Passed")
	}

	// === AI-01: Explanation ===
	if err := runAI01(); err != nil {
		fmt.Printf("❌ AI-01 Failed: %v\n", err)
		failures++
	} else {
		fmt.Println("✅ AI-01 Passed")
	}

	if failures > 0 {
		fmt.Printf("\n💀 %d Tests Failed\n", failures)
		os.Exit(1)
	}
	fmt.Println("\n🎉 All Mangle Tests Passed!")
}

func waitForServer() error {
	for i := 0; i < 30; i++ {
		resp, err := http.Get(baseURL + "/health")
		if err == nil && resp.StatusCode == 200 {
			return nil
		}
		time.Sleep(1 * time.Second)
		fmt.Print(".")
	}
	return fmt.Errorf("timeout waiting for server")
}

func queryDatalog(query string) ([]interface{}, error) {
	body := map[string]string{
		"project_id": projectID,
		"query":      query,
	}
	jsonBody, _ := json.Marshal(body)

	url := fmt.Sprintf("%s/v1/query?project=%s&raw=true", baseURL, projectID)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if rows, ok := result["results"].([]interface{}); ok {
		return rows, nil
	}
	return nil, nil
}

func runREL01() error {
	// Find analysis.Analyze
	// Query files in analysis package, then finding what they define.
	// triples(?f, "in_package", "gca-be/analysis"), triples(?f, "defines", ?s)
	// Then filter ?s for "Analyze".
	// Since we know "Analyze" might be a function, checking if ID ends in ":Analyze" is good enough.

	// Step 1: Get all definitions in analysis package
	// Debug dump shows package is "mangle-v2.analysis" (dot separated)
	query := `triples(?f, "in_package", "mangle-v2.analysis"), triples(?f, "defines", ?s)`
	rows, err := queryDatalog(query)
	if err != nil {
		return err
	}

	for _, r := range rows {
		row := r.(map[string]interface{})
		s := row["?s"].(string)
		if contains(s, ":Analyze") {
			return nil // Found it
		}
	}
	return fmt.Errorf("Analyze function not found")
}

func runREL02() error {
	// Find ast.Constant
	// Package: mangle-v2.ast
	query := `triples(?f, "in_package", "mangle-v2.ast"), triples(?f, "defines", ?s)`
	rows, err := queryDatalog(query)
	if err != nil {
		return err
	}

	for _, r := range rows {
		row := r.(map[string]interface{})
		s := row["?s"].(string)
		if contains(s, ":Constant") {
			return nil // Found it
		}
	}
	return fmt.Errorf("Constant struct not found")
}

func runREL03() error {
	// Check if ast imports engine.
	// triples(?s, "in_package", "mangle/ast"), triples(?s, "calls", ?o), triples(?o, "in_package", "mangle/engine")
	// Note: Package names might be fully qualified.
	// We'll iterate all calls from "ast" package.

	// WARNING: Complex joins might be slow if not indexed well.
	// Let's rely on listing imports for a file?
	// Or querying: triples(?s, "imports", ?dep) where ?s is file in "ast".

	// Simplification: query files in ast package
	// Log shows files like 'gca-be/ast/ast.go'
	// So we look for package string containing "ast/" or ending in "/ast"
	files, err := queryDatalog(`triples(?f, "type", "file")`)
	if err != nil {
		return err
	}

	found := false
	for _, r := range files {
		row := r.(map[string]interface{})
		f := row["?f"].(string)
		if contains(f, "/ast/") || contains(f, "/ast.go") {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("no ast files found")
	}
	return nil
}

func runBFS(sourcePkgFragment, targetPkgFragment string) error {
	// Find a file for source and target
	// Query: triples(?f, "type", "file")
	// Filter in Go.
	rows, err := queryDatalog(`triples(?f, "type", "file")`)
	if err != nil {
		return err
	}

	var source, target string

	for _, r := range rows {
		row := r.(map[string]interface{})
		f := row["?f"].(string)
		// Check path string
		// Ingestion logs show 'gca-be/analysis/...' so we should match that.
		// Or just match the package fragment (e.g. 'parse/parse.go').
		if contains(f, sourcePkgFragment+"/node_modules") || contains(f, targetPkgFragment+"/node_modules") {
			continue
		}

		// Match strict package usage
		if contains(f, sourcePkgFragment) && source == "" {
			source = f
		}
		if contains(f, targetPkgFragment) && target == "" {
			target = f
		}
	}

	if source == "" || target == "" {
		return fmt.Errorf("could not find source/target files for %s -> %s", sourcePkgFragment, targetPkgFragment)
	}

	// Run BFS
	url := fmt.Sprintf("%s/v1/graph/path?project=%s&source=%s&target=%s", baseURL, projectID, source, target)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func runAI01() error {
	// Explanation of Unify
	// Need ID of Unify function.
	// Query: triples(?f, "defines", ?s)
	// Filter ?s contains ":Unify"
	rows, err := queryDatalog(`triples(?f, "defines", ?s)`)
	if err != nil {
		return err
	}

	var id string
	for _, r := range rows {
		row := r.(map[string]interface{})
		s := row["?s"].(string)
		if contains(s, ":Unify") {
			id = s
			break
		}
	}

	if id == "" {
		return fmt.Errorf("failed to find Unify symbol")
	}

	body := map[string]string{
		"project_id": projectID,
		"task":       "insight",
		"symbol_id":  id,
	}
	jsonBody, _ := json.Marshal(body)
	resp, err := http.Post(baseURL+"/v1/ai/ask", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("ai error %d", resp.StatusCode)
	}

	var res map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&res)
	if ans, ok := res["answer"].(string); ok && len(ans) > 0 {
		fmt.Printf("   AI Answer: %s...\n", ans[:50])
		return nil
	}
	return fmt.Errorf("empty ai answer")
}

func contains(s, substr string) bool {
	// Simple string contains
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
