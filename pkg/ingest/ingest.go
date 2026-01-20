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
	ext := NewTreeSitterExtractor()
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
			localExt := NewTreeSitterExtractor() // Thread-local extractor
			for path := range jobs {
				log.Printf("Worker %d processing: %s", workerID, path)
				if err := processFileIncremental(ctx, s, localExt, path, testDir); err != nil {
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
func processFileIncremental(ctx context.Context, s *meb.MEBStore, ext Extractor, path string, sourceRoot string) error {
	relPath, _ := filepath.Rel(sourceRoot, path)

	// 1. Calculate Hash
	newHash, err := calculateHash(path)
	if err != nil {
		return fmt.Errorf("failed to hash file: %w", err)
	}

	// 2. Check Existing Hash
	qPath := fmt.Sprintf("\"%s\"", strings.ReplaceAll(relPath, "\"", "\\\""))
	query := fmt.Sprintf(`triples(%s, "%s", ?h)`, qPath, meb.PredHash)

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
	qSym := fmt.Sprintf(`triples(%s, "%s", ?sym)`, qPath, meb.PredDefines)
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
	if err := processFile(s, ext, path, sourceRoot); err != nil {
		return err
	}

	// Step 5: Store New Hash
	return s.AddFact(meb.Fact{
		Subject:   meb.DocumentID(relPath),
		Predicate: meb.PredHash,
		Object:    newHash,
		Graph:     "default",
	})
}

func processFile(s *meb.MEBStore, ext Extractor, path string, sourceRoot string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	relPath, _ := filepath.Rel(sourceRoot, path)

	// Use the new Extract interface
	bundle, err := ext.Extract(context.Background(), relPath, content)
	if err != nil {
		return err
	}

	// 1. Add Documents
	// Add file-level document
	if err := s.AddDocument(meb.DocumentID(relPath), content, nil, map[string]any{"type": "file"}); err != nil {
		log.Printf("Error adding file document %s: %v", relPath, err)
	}

	for _, doc := range bundle.Documents {
		if err := s.AddDocument(doc.ID, doc.Content, doc.Embedding, doc.Metadata); err != nil {
			log.Printf("Error adding document %s: %v", doc.ID, err)
		}
	}

	// 2. Add Facts
	// Resolve calls in facts if possible (local symbol table logic)
	// Ideally this should be done inside Extractor or a post-processing step?
	// Existing logic did it in processFile.
	// Since AnalysisBundle has Facts, we can iterate and modify them or just add them.
	// The symbol table logic requires modify OBJECT of preds=calls.
	// Let's iterate and fixup.

	finalFacts := make([]meb.Fact, 0, len(bundle.Facts))
	for _, f := range bundle.Facts {
		if f.Predicate == meb.PredCalls {
			if objStr, ok := f.Object.(string); ok {
				if resolved, ok := symbolTable[objStr]; ok {
					f.Object = resolved
				}
			}
		}
		finalFacts = append(finalFacts, f)
	}

	if len(finalFacts) > 0 {
		return s.AddFactBatch(finalFacts)
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
