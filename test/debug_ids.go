package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const baseURL = "http://localhost:8080"
const projectID = "gca-be"

func main() {
	fmt.Println("🔍 Debugging IDs and Facts...")

	// 1. Check Files
	fmt.Println("\n--- Files starting with gca-be ---")
	files := getFiles("gca-be")
	if len(files) == 0 {
		fmt.Println("No files found!")
	} else {
		for i, f := range files {
			if i < 10 {
				fmt.Printf("%s\n", f)
			}
		}
		if len(files) > 10 {
			fmt.Printf("... and %d more\n", len(files)-10)
		}
	}

	// 2. Search for 'App' symbol
	fmt.Println("\n--- Symbols matching 'App' in gca-fe/App.tsx ---")
	queryDatalog(`triples(?f, "defines", ?s), triples(?s, "type", ?t)`)
	// Too broad. Let's use search API first if available, or just query specific file defines
	// Find file ID for App.tsx
	var appFile string
	for _, f := range files {
		if hasSuffix(f, "App.tsx") {
			appFile = f
			break
		}
	}
	if appFile != "" {
		fmt.Printf("Found App File: %s\n", appFile)
		// Get symbols defined in App.tsx
		q := fmt.Sprintf(`triples("%s", "defines", ?s)`, appFile)
		results := queryDatalog(q)
		for _, r := range results {
			fmt.Printf(" - Defined: %v\n", r["?s"])
		}
	} else {
		fmt.Println("⚠️ App.tsx not found in file list")
	}

	// 3. Check has_tag
	fmt.Println("\n--- Checking has_tag facts ---")
	tags := queryDatalog(`triples(?s, "has_tag", ?t)`)
	fmt.Printf("Total has_tag facts: %d\n", len(tags))
	if len(tags) > 0 {
		fmt.Printf("Sample: %v has tag %v\n", tags[0]["?s"], tags[0]["?t"])
	}

	// 4. Check handled_by
	fmt.Println("\n--- Checking handled_by facts ---")
	handlers := queryDatalog(`triples(?s, "handled_by", ?h)`)
	fmt.Printf("Total handled_by facts: %d\n", len(handlers))
	for _, h := range handlers {
		fmt.Printf("Route %v -> Handler %v\n", h["?s"], h["?h"])
	}

	// 5. DUMP ALL FACTS (LIMIT 50)
	fmt.Println("\n--- DUMP 50 FACTS ---")
	dump := queryDatalog(`triples(?s, ?p, ?o)`)
	fmt.Printf("Total facts returned: %d\n", len(dump))
	for i, d := range dump {
		if i >= 50 {
			break
		}
		fmt.Printf("%v %v %v\n", d["?s"], d["?p"], d["?o"])
	}
}

func getFiles(prefix string) []string {
	resp, err := http.Get(baseURL + "/v1/files?project=" + projectID + "&prefix=" + prefix)
	if err != nil {
		fmt.Println("Error getting files:", err)
		return nil
	}
	defer resp.Body.Close()
	var files []string
	json.NewDecoder(resp.Body).Decode(&files)
	return files
}

func queryDatalog(q string) []map[string]interface{} {
	body := map[string]string{
		"query": q,
	}
	jsonBody, _ := json.Marshal(body)
	// Add project param to URL and raw=true
	url := fmt.Sprintf("%s/v1/query?project=%s&raw=true", baseURL, projectID)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		fmt.Println("Query error:", err)
		return nil
	}
	defer resp.Body.Close()
	var res map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&res)
	if results, ok := res["results"].([]interface{}); ok {
		out := make([]map[string]interface{}, len(results))
		for i, r := range results {
			out[i] = r.(map[string]interface{})
		}
		return out
	}
	return nil
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
