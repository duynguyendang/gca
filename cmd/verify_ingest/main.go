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

	// 2. Create a dummy Go file for ingestion
	srcDir, err := os.MkdirTemp("", "gca-src-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(srcDir)

	dummyCode := `package main
	
// Hello says hello.
func Hello(name string) string {
	return "Hello " + name
}

type User struct {
	Name string
}
`
	srcFile := srcDir + "/hello.go"
	if err := os.WriteFile(srcFile, []byte(dummyCode), 0644); err != nil {
		log.Fatal(err)
	}

	// 3. Run Ingestion (requires GEMINI_API_KEY)
	if os.Getenv("GEMINI_API_KEY") == "" {
		fmt.Println("SKIPPING: GEMINI_API_KEY not set")
		return
	}

	fmt.Println("Running ingestion...")
	if err := ingest.Run(s, srcDir); err != nil {
		log.Fatalf("Ingestion failed: %v", err)
	}

	// 4. Verify Content & Stats
	// Count
	n := s.Count()
	fmt.Printf("Total Facts: %d\n", n)
	if n == 0 {
		log.Fatal("Expected > 0 facts")
	}

	// Check Vector Count (should be > 0 because we added docs)
	vecCount := s.Vectors().Count()
	fmt.Printf("Total Vectors: %d\n", vecCount)
	if vecCount == 0 {
		log.Fatal("Expected > 0 vectors")
	}

	// 5. Verify Content Retrieval
	// Check for "hello.go:Hello" (namespace might vary based on rel path)
	// We didn't strictly predict the rel path since it's a temp dir,
	// but let's try to query facts to find the ID.

	results, err := s.Query(context.Background(), `triples(?s, "type", "function")`)
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}
	if len(results) == 0 {
		log.Fatal("Query returned no functions")
	}

	funcIDStr := results[0]["?s"].(string)
	fmt.Printf("Found function ID: %s\n", funcIDStr)

	// Get ID
	// id, err := s.GetID(funcIDStr)
	// Store doesn't expose Dict directly easily, but SetContent used GetOrCreateID.
	// We can use s.Dict() if available or just assume we can get content if we have ID.
	// Wait, GetContent takes uint64.

	// We need to resolve string ID to uint64.
	// We can use internal dict if accessible, or maybe add a method?
	// MEBStore has GetID(string) (uint64, error) usually?
	// Checking knowledge_store.go or dictionary usage.
	// knowledge_store.go uses m.dict.GetID.
	// Let's assume we can't easily get the uint64 ID from outside without a lookup helper.
	// However, we can use Vector Search to find it!

	// 6. Vector Search
	// Generate embedding for "Hello function"
	es, _ := ingest.NewEmbeddingService(context.Background())
	qVec, _ := es.GetEmbedding(context.Background(), "Hello function")

	// Search
	matches, err := s.Vectors().Search(qVec, 5)
	if err != nil {
		log.Fatalf("Vector search failed: %v", err)
	}

	if len(matches) == 0 {
		log.Fatal("Vector search returned 0 matches")
	}

	fmt.Printf("Top Match ID: %d, Score: %f\n", matches[0].ID, matches[0].Score)

	// 7. Get Content
	content, err := s.GetContent(matches[0].ID)
	if err != nil {
		log.Fatalf("GetContent failed: %v", err)
	}

	fmt.Printf("Content:\n%s\n", string(content))
	if len(content) == 0 {
		log.Fatal("Content is empty")
	}

	fmt.Println("Verification SUCCESS!")
}
