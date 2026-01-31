package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const baseURL = "http://localhost:8080"
const projectID = "gca"

func main() {
	// Query to check in_package
	query := `triples("gca/gca-be/pkg/ingest/ingest.go", "in_package", ?o)`

	body := map[string]string{
		"project_id": projectID,
		"query":      query,
	}
	jsonBody, _ := json.Marshal(body)

	url := fmt.Sprintf("%s/v1/query?project=%s&raw=true", baseURL, projectID)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	rows, _ := result["results"].([]interface{})
	fmt.Printf("found %d triples for mangle-new/analysis/rectify.go\n", len(rows))
	for i, r := range rows {
		if i > 20 {
			break
		}
		row := r.(map[string]interface{})
		fmt.Printf("%v %v %v\n", row["?s"], row["?p"], row["?o"])
	}
}
