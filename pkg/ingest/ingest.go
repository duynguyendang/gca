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
	"github.com/duynguyendang/gca/pkg/meb/vector"
)

var symbolTable = make(map[string]string)
var fileIndex = make(map[string]bool)

const MaxWorkers = 8

// Run executes the ingestion process with an optional projectName prefix.
func Run(s *meb.MEBStore, projectName string, sourceDir string) error {
	ctx := context.Background()
	ext := NewTreeSitterExtractor()

	// Initialize embedding service for semantic doc search
	var embeddingService *EmbeddingService
	var embeddingErr error
	embeddingService, embeddingErr = NewEmbeddingService(ctx)
	if embeddingErr != nil {
		log.Printf("Warning: Embedding service unavailable: %v (skipping doc embeddings)", embeddingErr)
	} else {
		defer embeddingService.Close()
		log.Println("Embedding service initialized for semantic doc search")
	}

	// Pass 1: Collect Symbols and File Index
	fmt.Printf("Pass 1: Collecting symbols and index for %s...\n", projectName)
	symbolTable = make(map[string]string)
	fileIndex = make(map[string]bool)

	err := filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".git" || d.Name() == "dist" || d.Name() == "build" || d.Name() == ".next" {
				return filepath.SkipDir
			}
			return nil
		}
		if isSupportedFile(path) {
			relPath, _ := filepath.Rel(sourceDir, path)
			if projectName != "" {
				relPath = filepath.Join(projectName, relPath)
			}
			fileIndex[relPath] = true

			content, _ := os.ReadFile(path)
			symbols, _ := ext.ExtractSymbols(path, content, relPath)
			for _, sym := range symbols {
				symbolTable[sym.Name] = sym.ID
				if sym.Package != "" {
					symbolTable[sym.Package+"."+sym.Name] = sym.ID
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("pass 1 failed: %w", err)
	}

	// Pass 2: Concurrent Processing
	fmt.Printf("Pass 2: Processing files for %s...\n", projectName)
	jobs := make(chan string, 100)
	var wg sync.WaitGroup
	var pass2Err atomic.Uint64

	workerCount := runtime.NumCPU()
	if workerCount > MaxWorkers {
		workerCount = MaxWorkers
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			localExt := NewTreeSitterExtractor()
			for path := range jobs {
				rel, _ := filepath.Rel(sourceDir, path)
				fmt.Printf("  Processing %s/%s...\n", projectName, rel)
				if err := processFileIncremental(ctx, s, localExt, embeddingService, path, projectName, sourceDir); err != nil {
					log.Printf("Error: %v", err)
					pass2Err.Add(1)
				}
			}
		}()
	}

	filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".git" || d.Name() == "dist" || d.Name() == "build" || d.Name() == ".next" {
				return filepath.SkipDir
			}
			return nil
		}
		if isSupportedFile(path) {
			jobs <- path
		}
		return nil
	})
	close(jobs)
	wg.Wait()

	// Final Passes
	s.ResolveDependencies(ctx)
	EnhanceVirtualTriples(s)
	TagRoles(s)

	return nil
}

func processFileIncremental(ctx context.Context, s *meb.MEBStore, ext Extractor, embedder *EmbeddingService, path string, projectName string, sourceRoot string) error {
	relPath, _ := filepath.Rel(sourceRoot, path)
	if projectName != "" {
		relPath = filepath.Join(projectName, relPath)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Basic Ingestion (Simplified for this task, ensuring prefix is used)
	bundle, err := ext.Extract(ctx, relPath, content)
	if err != nil {
		return err
	}

	s.AddDocument(meb.DocumentID(relPath), content, nil, map[string]any{"project": projectName})

	// Embed documentation for semantic search
	if embedder != nil {
		for _, fact := range bundle.Facts {
			if fact.Predicate == meb.PredHasDoc {
				docText, ok := fact.Object.(string)
				if ok && len(docText) > 10 { // Only embed meaningful docs
					go func(symbolID meb.DocumentID, text string) {
						embed, err := embedder.GetEmbedding(ctx, text)
						if err == nil && len(embed) >= vector.FullDim {
							// Use hash of symbol ID as vector ID, store original string for lookup
							vecID := hashSymbolID(string(symbolID))
							s.Vectors().AddWithStringID(vecID, string(symbolID), embed)
						}
					}(fact.Subject, docText)
				}
			}
		}
	}

	// Store symbol documents (with file, start_line, end_line metadata for snippet extraction)
	for _, doc := range bundle.Documents {
		// Don't store content for symbols (we'll extract on-demand from parent file)
		// But we DO need to store the metadata (file, start_line, end_line)
		s.AddDocument(doc.ID, nil, nil, doc.Metadata)
	}

	finalFacts := make([]meb.Fact, 0, len(bundle.Facts)+2)

	// Inject Role Tags based on path
	// fmt.Printf("DEBUG: relPath=%s\n", relPath)
	if strings.Contains(relPath, "gca-be") || strings.HasSuffix(relPath, ".go") {
		// fmt.Printf("DEBUG: Tagging %s as backend\n", relPath)
		finalFacts = append(finalFacts, meb.Fact{Subject: meb.DocumentID(relPath), Predicate: "has_tag", Object: "backend", Graph: "default"})
	} else if strings.Contains(relPath, "gca-fe") || strings.HasSuffix(relPath, ".ts") || strings.HasSuffix(relPath, ".tsx") {
		// fmt.Printf("DEBUG: Tagging %s as frontend\n", relPath)
		finalFacts = append(finalFacts, meb.Fact{Subject: meb.DocumentID(relPath), Predicate: "has_tag", Object: "frontend", Graph: "default"})
	}

	// Make sure file has type "file"
	finalFacts = append(finalFacts, meb.Fact{Subject: meb.DocumentID(relPath), Predicate: "type", Object: "file", Graph: "default"})

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

	return s.AddFactBatch(finalFacts)
}

func calculateHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashSymbolID converts a symbol ID string to uint64 for vector storage.
// Uses FNV-1a hash for fast, deterministic conversion.
func hashSymbolID(id string) uint64 {
	h := uint64(14695981039346656037) // FNV offset basis
	for i := 0; i < len(id); i++ {
		h ^= uint64(id[i])
		h *= 1099511628211 // FNV prime
	}
	return h
}

func isSupportedFile(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".go" || ext == ".ts" || ext == ".tsx" || ext == ".js"
}

func TagRoles(s *meb.MEBStore) error {
	ctx := context.Background()
	// Tag API handlers
	res, _ := s.Query(ctx, `triples(?url, "handled_by", ?h)`)
	for _, r := range res {
		h, _ := r["?h"].(string)
		s.AddFact(meb.Fact{Subject: meb.DocumentID(h), Predicate: "has_role", Object: "api_handler", Graph: "virtual"})
	}
	// Tag Contracts
	res, _ = s.Query(ctx, `triples(?s, "in_package", ?pkg)`)
	for _, r := range res {
		p, _ := r["?pkg"].(string)
		sID, _ := r["?s"].(string)
		if strings.Contains(p, "types") || strings.Contains(p, "models") || strings.Contains(p, "meb") || strings.Contains(p, "ast") {
			s.AddFact(meb.Fact{Subject: meb.DocumentID(sID), Predicate: "has_role", Object: "data_contract", Graph: "virtual"})
		}
	}
	return nil
}
