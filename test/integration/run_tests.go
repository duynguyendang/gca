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
const projectID = "gca-be"

func main() {
	fmt.Println("🚀 Starting GCA Integration Tests...")

	if err := waitForServer(); err != nil {
		fmt.Printf("❌ Server not ready: %v\n", err)
		os.Exit(1)
	}

	failures := 0

	// === INT-01 ===
	// Trace flow from App.tsx to executor.go
	// Need exact IDs. Let's assume standard IDs first.
	// files might be: "gca-be/test-code/gca/gca-fe/App.tsx", "gca-be/pkg/repl/executor.go"
	// Wait, ingestion uses relative paths from where it ran.
	// If I ran from `gca` folder, and passed `.`, and project name "gca-be".
	// ID for `main.go` would be `gca-be/main.go`.
	// ID for `test-code/gca/gca-fe/App.tsx` -> `gca-be/test-code/gca/gca-fe/App.tsx`.
	// Updated IDs based on actual ingestion (including test-code prefix)
	fromID := "gca-be/test-code/gca/gca-fe/App.tsx:syncApi"
	toID := "gca-be/test-code/gca/gca-be/pkg/server/handlers.go:Server.handleProjects"

	if err := runINT01(fromID, toID); err != nil {
		fmt.Printf("❌ INT-01 Failed: %v\n", err)
		failures++
	} else {
		fmt.Println("✅ INT-01 Passed")
	}

	// === INT-02 ===
	if err := runINT02(); err != nil {
		fmt.Printf("❌ INT-02 Failed: %v\n", err)
		failures++
	} else {
		fmt.Println("✅ INT-02 Passed")
	}

	// === AI-01 ===
	// Purpose of graphService.ts
	// ID: gca-be/test-code/gca/gca-fe/services/graphService.ts
	graphSvcID := "gca-be/test-code/gca/gca-fe/services/graphService.ts"
	if err := runAI01(graphSvcID); err != nil {
		fmt.Printf("❌ AI-01 Failed: %v\n", err)
		failures++
	} else {
		fmt.Println("✅ AI-01 Passed")
	}

	if failures > 0 {
		fmt.Printf("\n💀 %d Tests Failed\n", failures)
		os.Exit(1)
	}
	fmt.Println("\n🎉 All Tests Passed!")
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

func runINT01(from, to string) error {
	// Use /v1/graph/path?project=...&source=...&target=...
	url := fmt.Sprintf("%s/v1/graph/path?project=%s&source=%s&target=%s", baseURL, projectID, from, to)
	start := time.Now()
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	duration := time.Since(start)

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	// Validation
	// Expect result.nodes (list) and result.links (list)
	nodes, _ := result["nodes"].([]interface{})
	if len(nodes) == 0 {
		return fmt.Errorf("no path found (0 nodes)")
	}
	if len(nodes) >= 15 {
		return fmt.Errorf("too many nodes (%d >= 15) - noise detected", len(nodes))
	}

	fmt.Printf("   INT-01 Latency: %v | Path Length: %d\n", duration, len(nodes))
	return nil
}

func runINT02() error {
	// Query datalog
	// triples(?s, "calls_api", ?o), triples(?o, "handled_by", ?h)
	query := `triples(?s, "calls_api", ?o), triples(?o, "handled_by", ?h)`

	body := map[string]string{
		"project_id": projectID,
		"query":      query,
	}
	jsonBody, _ := json.Marshal(body)

	url := fmt.Sprintf("%s/v1/query?project=%s", baseURL, projectID)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	// /v1/query returns { "results": [ ... row maps ... ] }
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	rows, _ := result["results"].([]interface{})
	if len(rows) == 0 {
		// Try fallback query or check manually if we expect results.
		// If "calls_api" is virtual, maybe it's not materialized yet or needs hydration?
		// Datalog queries usually run on materialized facts.
		return fmt.Errorf("no handlers found (0 results)")
	}

	fmt.Printf("   INT-02 Found %d links\n", len(rows))
	return nil
}

func runAI01(symbolID string) error {
	body := map[string]string{
		"project_id": projectID,
		"task":       "insight",
		"symbol_id":  symbolID,
	}
	jsonBody, _ := json.Marshal(body)

	resp, err := http.Post(baseURL+"/v1/ai/ask", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	answer, _ := result["answer"].(string)
	if answer == "" {
		return fmt.Errorf("empty answer")
	}

	fmt.Printf("   AI-01 Answer: %s...\n", answer[:50])
	return nil
}
