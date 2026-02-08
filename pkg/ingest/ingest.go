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
	"time"

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

	// Check for Low-Mem profile
	isLowMem := s.Config().Profile == "Cloud-Run-LowMem"
	if isLowMem {
		log.Println("Low-memory mode detected: Skipping embedding generation (Metadata-only indexing)")
	} else {
		embeddingService, embeddingErr = NewEmbeddingService(ctx)
		if embeddingErr != nil {
			log.Printf("Warning: Embedding service unavailable: %v (skipping doc embeddings)", embeddingErr)
		} else {
			defer embeddingService.Close()
			log.Println("Embedding service initialized for semantic doc search")
		}
	}

	// Pass 1: Collect Symbols and File Index
	fmt.Printf("Pass 1: Collecting symbols and index for %s...\n", projectName)
	symbolTable = make(map[string]string)
	fileIndex = make(map[string]bool)

	// Check for project metadata
	var projectMeta *ProjectMetadata
	metadataPath := filepath.Join(sourceDir, "project.yaml")
	if _, err := os.Stat(metadataPath); err == nil {
		fmt.Printf("Found project metadata at %s\n", metadataPath)
		var metaErr error
		projectMeta, metaErr = LoadProjectMetadata(metadataPath)
		if metaErr != nil {
			log.Printf("Warning: Failed to load project metadata: %v", metaErr)
		} else {
			// Create Project Node
			s.AddFact(meb.Fact{
				Subject:   meb.DocumentID(projectMeta.Name),
				Predicate: "type",
				Object:    "project",
				Graph:     "default",
			})
			s.AddFact(meb.Fact{
				Subject:   meb.DocumentID(projectMeta.Name),
				Predicate: "description",
				Object:    projectMeta.Description,
				Graph:     "default",
			})
			for _, tag := range projectMeta.Tags {
				s.AddFact(meb.Fact{
					Subject:   meb.DocumentID(projectMeta.Name),
					Predicate: "has_tag",
					Object:    tag,
					Graph:     "default",
				})
			}
		}
	}

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
	var embeddingWg sync.WaitGroup // Wait for embeddings to finish
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
				if err := processFileIncremental(ctx, s, localExt, embeddingService, path, projectName, sourceDir, projectMeta, &embeddingWg); err != nil {
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

	if embeddingService != nil {
		fmt.Println("Waiting for embeddings to complete...")
		embeddingWg.Wait()
	}

	return nil
}

func processFileIncremental(ctx context.Context, s *meb.MEBStore, ext Extractor, embedder *EmbeddingService, path string, projectName string, sourceRoot string, meta *ProjectMetadata, embeddingWg *sync.WaitGroup) error {
	relPath, _ := filepath.Rel(sourceRoot, path)

	// Apply Logical Path Mapping from Metadata
	if meta != nil && meta.Components != nil {
		for compName, compMeta := range meta.Components {
			// Check if path starts with component path (handle directory boundaries)
			basePrefix := compMeta.Path
			if relPath == basePrefix || strings.HasPrefix(relPath, basePrefix+string(os.PathSeparator)) {
				// Rewrite path: replace physical prefix with logical component name
				suffix := strings.TrimPrefix(relPath, basePrefix)
				suffix = strings.TrimPrefix(suffix, string(os.PathSeparator))
				relPath = filepath.Join(compName, suffix)
				break // Match first component found
			}
		}
	}

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

	// Retry AddDocument to handle potential DB conflicts
	var addErr error
	for retries := 0; retries < 3; retries++ {
		addErr = s.AddDocument(meb.DocumentID(relPath), content, nil, map[string]any{"project": projectName})
		if addErr == nil {
			break
		}
		// fast retry for conflicts
		time.Sleep(time.Millisecond * time.Duration(10*(retries+1)))
	}
	if addErr != nil {
		return fmt.Errorf("failed to add document %s: %w", relPath, addErr)
	}

	// Store symbol documents (with file, start_line, end_line metadata for snippet extraction)
	for _, doc := range bundle.Documents {
		// Don't store content for symbols (we'll extract on-demand from parent file)
		// But we DO need to store the metadata (file, start_line, end_line)
		if err := s.AddDocument(doc.ID, nil, nil, doc.Metadata); err != nil {
			// Log error but continue for symbols? Or fail file?
			// Identifying symbols is important but not critical for file content.
			// Let's log warning.
			log.Printf("Warning: failed to add symbol doc %s: %v", doc.ID, err)
		}
	}

	// Embed documentation for semantic search (AFTER symbols are added to ensure IDs exist)
	if embedder != nil {
		docFactsFound := 0
		for _, fact := range bundle.Facts {
			if fact.Predicate == meb.PredHasDoc {
				docFactsFound++
				docText, ok := fact.Object.(string)
				log.Printf("DEBUG: Found doc for %s (len=%d)", fact.Subject, len(docText))

				if ok && len(docText) > 10 { // Only embed meaningful docs
					if embeddingWg != nil {
						embeddingWg.Add(1)
					} else {
						log.Printf("Warning: embeddingWg is nil for %s", fact.Subject)
					}

					go func(symbolID meb.DocumentID, text string) {
						if embeddingWg != nil {
							defer embeddingWg.Done()
						}

						// Add a timeout to prevent hanging
						ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						defer cancel()

						log.Printf("Generating embedding for %s (len=%d)...", symbolID, len(text))
						embed, err := embedder.GetEmbedding(ctxWithTimeout, text)
						if err != nil {
							log.Printf("Error generating embedding for %s: %v", symbolID, err)
							return
						}

						if len(embed) < vector.FullDim {
							log.Printf("Error: embedding dimension mismatch for %s: got %d, want >= %d", symbolID, len(embed), vector.FullDim)
							return
						}

						// Look up the correct dictionary ID for the symbol
						dictID, found := s.LookupID(string(symbolID))
						if !found {
							log.Printf("Error: ID not found in dictionary for %s (cannot store vector)", symbolID)
							return
						}

						if err := s.Vectors().AddWithStringID(dictID, string(symbolID), embed); err != nil {
							log.Printf("Error adding vector to store for %s: %v", symbolID, err)
						} else {
							log.Printf("Successfully stored embedding for %s (ID=%d)", symbolID, dictID)
						}
					}(fact.Subject, docText)
				} else if ok {
					log.Printf("Skipping embedding for %s: text too short (%d chars)", fact.Subject, len(docText))
				}
			}
		}
	}

	finalFacts := make([]meb.Fact, 0, len(bundle.Facts)+2)

	// Inject Role Tags based on path or metadata
	// fmt.Printf("DEBUG: relPath=%s\n", relPath)
	tagged := false
	if meta != nil && meta.Components != nil {
		for _, comp := range meta.Components {
			if strings.Contains(relPath, comp.Path) {
				finalFacts = append(finalFacts, meb.Fact{Subject: meb.DocumentID(relPath), Predicate: "has_tag", Object: comp.Type, Graph: "default"})
				tagged = true
				break // Assume one component per file for now
			}
		}
	}

	if !tagged {
		if strings.HasSuffix(relPath, ".go") {
			// fmt.Printf("DEBUG: Tagging %s as backend\n", relPath)
			finalFacts = append(finalFacts, meb.Fact{Subject: meb.DocumentID(relPath), Predicate: "has_tag", Object: "backend", Graph: "default"})
		} else if strings.HasSuffix(relPath, ".ts") || strings.HasSuffix(relPath, ".tsx") {
			// fmt.Printf("DEBUG: Tagging %s as frontend\n", relPath)
			finalFacts = append(finalFacts, meb.Fact{Subject: meb.DocumentID(relPath), Predicate: "has_tag", Object: "frontend", Graph: "default"})
		}
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
	return ext == ".go" || ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".py" || ext == ".md"
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
