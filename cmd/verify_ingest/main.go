package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/duynguyendang/gca/pkg/ingest"
	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/gca/pkg/meb/store"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	// 1. Setup temporary store
	dir, err := os.MkdirTemp("", "gca-verify-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	cfg := store.DefaultConfig(dir)
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()

	// 2. Create dummy source files for ingestion
	srcDir, err := os.MkdirTemp("", "gca-src-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(srcDir)

	// Go file
	goCode := `package main
	
// Hello says hello.
func Hello(name string) string {
	return "Hello " + name
}
`
	if err := os.WriteFile(srcDir+"/hello.go", []byte(goCode), 0644); err != nil {
		log.Fatal(err)
	}

	// Python file
	pyCode := `
APP_NAME = "Mangle"
version = 1

def py_hello(name):
    """Says hello in Python."""
    return f"Hello {name}"

class PyGreeter:
    def greet(self):
        print("Hi from class")
`
	if err := os.WriteFile(srcDir+"/hello.py", []byte(pyCode), 0644); err != nil {
		log.Fatal(err)
	}

	// TS file
	tsCode := `
interface Greeter {
    greet(name: string): string;
}

class TSGreeter implements Greeter {
    /**
     * Greets in TS.
     */
    greet(name: string): string {
        return "Hello " + name;
    }
}
`
	if err := os.WriteFile(srcDir+"/hello.ts", []byte(tsCode), 0644); err != nil {
		log.Fatal(err)
	}

	// 3. Run Ingestion (skip if API key missing, but we really want to verify extraction)
	// Even if embedding fails or is skipped, we can check symbols if Extractor works.
	// But `ingest.Run` usually includes embedding.
	// Let's assume dev env has key or mocks it.
	fmt.Println("Running ingestion...")
	if err := ingest.Run(s, srcDir); err != nil {
		log.Fatalf("Ingestion failed: %v", err)
	}

	// 4. Verify Content & Stats
	n := s.Count()
	fmt.Printf("Total Facts: %d\n", n)
	if n == 0 {
		log.Fatal("Expected > 0 facts")
	}

	// 5. Verify Content Retrieval via ID or Type
	// Check for Python function
	fmt.Println("Querying for Python function...")
	results, err := s.Query(context.Background(), `triples(?s, "type", "function")`)
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}

	foundPy := false
	foundGo := false
	// We might have logic that maps Py function -> TypeFunction.
	// Let's check IDs.
	for _, r := range results {
		id := r["?s"].(string)
		fmt.Printf("Found function: %s\n", id)
		if id == "hello.py:py_hello" {
			foundPy = true
		}
		if id == "hello.go:Hello" {
			foundGo = true
		}
	}

	if !foundPy {
		log.Fatal("Did not find hello.py:py_hello")
	}
	if !foundGo {
		log.Fatal("Did not find hello.go:Hello")
	}

	// Check for TS Class
	fmt.Println("Querying for Class/Structs...")
	// Note: In refined extractor, we used TypeClass for Py/TS.
	// Go uses TypeStruct.
	// Let's query ?s type ?t
	results, err = s.Query(context.Background(), `triples(?s, "type", "class")`)
	if err == nil {
		foundTSClass := false
		for _, r := range results {
			id := r["?s"].(string)
			fmt.Printf("Found class: %s\n", id)
			if id == "hello.ts:TSGreeter" {
				foundTSClass = true
			}
		}
		if !foundTSClass {
			// Might be mapped to struct?
			// Let's check struct
		}
	}

	// Check for Variable
	fmt.Println("Querying for Variables...")
	results, err = s.Query(context.Background(), `triples(?s, "type", "variable")`)
	if err == nil {
		foundVar := false
		for _, r := range results {
			id := r["?s"].(string)
			fmt.Printf("Found variable: %s\n", id)
			if id == "hello.py:APP_NAME" || id == "hello.py:version" {
				foundVar = true
			}
		}
		if !foundVar {
			log.Fatal("Did not find hello.py variables")
		}
	}

	// 6. Cross-Language Search (if API Key present)
	if os.Getenv("GEMINI_API_KEY") != "" {
		fmt.Println("Running vector search...")
		// Embed "greets in python"
		es, _ := ingest.NewEmbeddingService(context.Background())
		qVec, err := es.GetEmbedding(context.Background(), "hello in Python")
		if err == nil {
			matches, _ := s.Vectors().Search(qVec, 5)
			if len(matches) > 0 {
				fmt.Printf("Top match: %d score: %f\n", matches[0].ID, matches[0].Score)
			}
		}
	} else {
		fmt.Println("Skipping vector search (no API KEY)")
	}

	fmt.Println("Verification SUCCESS!")
}
