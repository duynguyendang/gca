package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/duynguyendang/gca/pkg/meb"
)

var symbolTable = make(map[string]string)

const MaxWorkers = 8 // Default max workers, can be tuned or runtime.NumCPU()

// Run executes the ingestion process.
func Run(s *meb.MEBStore, sourceDir string) error {
	ctx := context.Background()
	ext := NewExtractor()
	testDir := sourceDir

	// Pass 1: Collect Symbols (Sequential for now, to ensure map is populated safely)
	fmt.Println("Pass 1: Collecting symbols...")
	// Reset symbol table
	symbolTable = make(map[string]string)
	var pass1Errors []error

	err := filepath.WalkDir(testDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && isSupportedFile(path) {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			relPath, _ := filepath.Rel(testDir, path)
			symbols, err := ext.ExtractSymbols(path, content, relPath)
			if err != nil {
				// Don't logspam, just collect
				pass1Errors = append(pass1Errors, fmt.Errorf("%s: %w", path, err))
			} else {
				for _, sym := range symbols {
					symbolTable[sym.Name] = sym.ID
					if sym.Package != "" {
						symbolTable[sym.Package+"."+sym.Name] = sym.ID
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error walking directory in Pass 1: %w", err)
	}

	if len(pass1Errors) > 0 {
		return fmt.Errorf("pass 1 failed with %d errors (first error: %v)", len(pass1Errors), pass1Errors[0])
	}
	fmt.Printf("Collected %d symbols\n", len(symbolTable))

	// Pass 2: Process Files Concurrent
	fmt.Println("Pass 2: Processing files (Concurrent)...")

	jobs := make(chan string, 100)
	var wg sync.WaitGroup
	var pass2ErrorCount atomic.Uint64

	// Determine worker count
	workerCount := runtime.NumCPU()
	if workerCount > MaxWorkers {
		workerCount = MaxWorkers
	}
	fmt.Printf("Starting %d workers\n", workerCount)

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		workerID := i // Capture loop variable
		go func() {
			defer wg.Done()
			log.Printf("Worker %d started", workerID)
			localExt := NewExtractor() // Thread-local extractor
			for path := range jobs {
				log.Printf("Worker %d processing: %s", workerID, path)
				if err := processFileIncremental(ctx, s, nil, localExt, path, testDir); err != nil {
					log.Printf("Error processing file %s: %v", path, err)
					pass2ErrorCount.Add(1)
				}
				log.Printf("Worker %d finished: %s", workerID, path)
			}
			log.Printf("Worker %d exiting", workerID)
		}()
	}

	err = filepath.WalkDir(testDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && isSupportedFile(path) {
			jobs <- path
		}
		return nil
	})

	close(jobs)
	wg.Wait()

	if err != nil {
		return fmt.Errorf("error walking directory: %w", err)
	}

	if count := pass2ErrorCount.Load(); count > 0 {
		return fmt.Errorf("pass 2 completed with %d file processing errors", count)
	}

	return nil
}

// processFileIncremental handles hashing and skipping
func processFileIncremental(ctx context.Context, s *meb.MEBStore, embedder *EmbeddingService, ext *Extractor, path string, sourceRoot string) error {
	relPath, _ := filepath.Rel(sourceRoot, path)

	// 1. Calculate Hash
	newHash, err := calculateHash(path)
	if err != nil {
		return fmt.Errorf("failed to hash file: %w", err)
	}

	// 2. Check Existing Hash
	qPath := fmt.Sprintf("\"%s\"", strings.ReplaceAll(relPath, "\"", "\\\""))
	query := fmt.Sprintf(`triples(%s, "hash_sha256", ?h)`, qPath)

	res, err := s.Query(ctx, query)
	if err == nil && len(res) > 0 {
		if oldHash, ok := res[0]["?h"].(string); ok {
			if oldHash == newHash {
				// Unchanged
				return nil
			}
		}
	}

	// 3. File Changed or New -> Process
	// Clear old facts using Subject = relPath
	// Step 3a: Find symbols defined by this file (to delete them)
	qSym := fmt.Sprintf(`triples(%s, "defines_symbol", ?sym)`, qPath)
	symRes, _ := s.Query(ctx, qSym)

	// Step 3b: Delete File facts
	if err := s.DeleteFactsBySubject(relPath); err != nil {
		return fmt.Errorf("failed to clear file facts: %w", err)
	}

	// Step 3c: Delete Symbol facts
	for _, r := range symRes {
		if symID, ok := r["?sym"].(string); ok {
			s.DeleteFactsBySubject(symID)
		}
	}

	// Step 4: Full Processing (Extraction & Storage)
	if err := processFile(ctx, s, embedder, ext, path, sourceRoot); err != nil {
		return err
	}

	// Step 5: Store New Hash
	return s.AddFact(meb.Fact{
		Subject:   relPath,
		Predicate: "hash_sha256",
		Object:    newHash,
		Graph:     "default",
	})
}

func processFile(ctx context.Context, s *meb.MEBStore, embedder *EmbeddingService, ext *Extractor, path string, sourceRoot string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	relPath, _ := filepath.Rel(sourceRoot, path)

	symbols, err := ext.ExtractSymbols(path, content, relPath)
	if err != nil {
		return err
	}

	var batch []meb.Fact

	for _, sym := range symbols {
		// Embeddings and Content Store handled separately from batch
		var vec []float32
		// if embedder != nil { ... }

		// Metadata map for AddDocument (which adds its own metadata facts internally, but we manually add some for batch optimization if needed)
		// Actually AddDocument calls AddFact internally.
		// To use batching efficiently, we should collect facts here and call AddFactBatch,
		// OR modify AddDocument to accept a batch or return facts.
		// Given AddDocument interface, we'll use it as is for vector/content, but maybe duplicate metadata facts in batch?
		// No, AddDocument adds facts. If we want batching, we should probably manually add facts and use AddDocument only for vector/content.
		// Let's defer Vector/Content logic to AddDocument but handling metadata manually in batch for speed.

		// Metadata map for AddDocument (which adds its own metadata facts internally, but we manually add some for batch optimization if needed)
		// We use s.AddDocument for the "Document" aspect (content + vector)
		// But passing empty metadata to avoid individual AddFact calls inside it.
		// We will add metadata facts in our big batch.
		meta := map[string]any{
			"type":       sym.Type,
			"file":       relPath,
			"start_line": sym.StartLine,
			"end_line":   sym.EndLine,
			"package":    sym.Package,
		}

		// 1. Add Document content/vector (optimized: no metadata passed to avoid individual writes)
		if err := s.AddDocument(sym.ID, []byte(sym.Content), vec, nil); err != nil {
			log.Printf("Error adding document %s: %v", sym.ID, err)
		}

		// 2. Add structural/metadata facts to BATCH
		batch = append(batch, meb.Fact{Subject: sym.ID, Predicate: "type", Object: sym.Type, Graph: "default"})
		batch = append(batch, meb.Fact{Subject: sym.ID, Predicate: "defines", Object: sym.Name, Graph: "default"})
		batch = append(batch, meb.Fact{Subject: relPath, Predicate: "defines_symbol", Object: sym.ID, Graph: "default"})

		// Metadata facts
		for k, v := range meta {
			batch = append(batch, meb.Fact{Subject: sym.ID, Predicate: k, Object: v, Graph: "default"})
		}
	}

	// Extract References (Calls, Imports)
	refs, err := ext.ExtractReferences(path, content, relPath)
	if err != nil {
		log.Printf("Warning: failed to extract references from %s: %v", path, err)
	} else {
		for _, ref := range refs {
			// Resolve callee if possible
			object := ref.Object
			if ref.Predicate == "calls" {
				if resolved, ok := symbolTable[object]; ok {
					object = resolved
				}

				// Optional: Add calls_at fact
				// triples(Caller, "calls_at", LineNumber)
				// ref.Subject is the Caller Scope (e.g. "path:CallerFunc")
				// We need "calls_at" to point to the line number.
				// NOTE: We probably want `calls` fact to remain as is: `calls(Caller, Callee)`.
				// And add a parallel fact `calls_at(Caller, LineNumber)`?
				// But one caller can call multiple things.
				// `calls_at` needs to target the unique call event.
				// User Req: "triples(Caller, 'calls_at', LineNumber)"
				// This links the Caller to the line number.
				// If Caller calls multiple things on multiple lines, we get multiple `calls_at` facts.
				// S -> calls_at -> Line.
				// This seems fine.
				batch = append(batch, meb.Fact{Subject: ref.Subject, Predicate: "calls_at", Object: ref.Line, Graph: "default"})
			}

			batch = append(batch, meb.Fact{Subject: ref.Subject, Predicate: ref.Predicate, Object: object, Graph: "default"})
		}
	}

	// Flush Batch
	if len(batch) > 0 {
		return s.AddFactBatch(batch)
	}

	return nil
}

func clean(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\"", ""))
}

func calculateHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func isSupportedFile(path string) bool {
	ext := filepath.Ext(path)
	switch ext {
	case ".go", ".py", ".js", ".jsx", ".ts", ".tsx":
		return true
	}
	return false
}
