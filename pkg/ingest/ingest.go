package ingest

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/keys"
)

type IngestState struct {
	SymbolTable map[string]string
	FileIndex   map[string]bool
}

func NewIngestState() *IngestState {
	return &IngestState{
		SymbolTable: make(map[string]string),
		FileIndex:   make(map[string]bool),
	}
}

// Run executes the ingestion process with an optional projectName prefix.
func Run(s *meb.MEBStore, projectName string, sourceDir string) error {
	state := NewIngestState()
	return RunWithState(s, projectName, sourceDir, state)
}

// RunWithState executes the ingestion process with explicit state management.
func RunWithState(s *meb.MEBStore, projectName string, sourceDir string, state *IngestState) error {
	SetIngestState(state)
	ctx := context.Background()
	ext := NewTreeSitterExtractor()

	// Set topic ID for project-scoped ingestion
	// Uses a hash of the project name to generate a unique 24-bit topic ID
	topicID := hashToTopicID(projectName)
	s.SetTopicID(topicID)
	logger.Info("Using topic ID for project", "topic_id", topicID, "project", projectName)

	var embeddingService *EmbeddingService
	var embeddingErr error

	embeddingService, embeddingErr = NewEmbeddingService(ctx)
	if embeddingErr != nil {
		logger.Warn("Embedding service unavailable, skipping doc embeddings", "error", embeddingErr)
	} else {
		defer embeddingService.Close()
		logger.Info("Embedding service initialized for semantic doc search")
	}

	fmt.Printf("Pass 1: Collecting symbols and index for %s...\n", projectName)
	state.SymbolTable = make(map[string]string)
	state.FileIndex = make(map[string]bool)

	// Check for project metadata
	var projectMeta *ProjectMetadata
	metadataPath := filepath.Join(sourceDir, "project.yaml")
	if _, err := os.Stat(metadataPath); err == nil {
		fmt.Printf("Found project metadata at %s\n", metadataPath)
		var metaErr error
		projectMeta, metaErr = LoadProjectMetadata(metadataPath)
		if metaErr != nil {
			logger.Warn("Failed to load project metadata", "error", metaErr)
		} else {
			// Create Project Node
			s.AddFact(meb.Fact{
				Subject:   string(projectMeta.Name),
				Predicate: config.PredicateType,
				Object:    "project",
			})
			s.AddFact(meb.Fact{
				Subject:   string(projectMeta.Name),
				Predicate: "description",
				Object:    projectMeta.Description,
			})
			for _, tag := range projectMeta.Tags {
				s.AddFact(meb.Fact{
					Subject:   string(projectMeta.Name),
					Predicate: config.PredicateHasTag,
					Object:    tag,
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
			state.FileIndex[relPath] = true

			content, _ := os.ReadFile(path)
			symbols, _ := ext.ExtractSymbols(path, content, relPath)
			for _, sym := range symbols {
				state.SymbolTable[sym.Name] = sym.ID
				if sym.Package != "" {
					state.SymbolTable[sym.Package+"."+sym.Name] = sym.ID
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
	if workerCount > config.MaxWorkers {
		workerCount = config.MaxWorkers
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			localExt := NewTreeSitterExtractor()
			// Global semaphore for embeddings limit (max 10 concurrent)
			sem := make(chan struct{}, 10)
			for path := range jobs {
				rel, _ := filepath.Rel(sourceDir, path)
				fmt.Printf("  Processing %s/%s...\n", projectName, rel)
				if err := processFile(ctx, s, localExt, embeddingService, path, projectName, sourceDir, projectMeta, &embeddingWg, sem, state); err != nil {
					logger.Error("Failed to process file", "error", err)
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
	EnhanceVirtualTriples(s)
	TagRoles(s)

	if embeddingService != nil {
		fmt.Println("Waiting for embeddings to complete...")
		embeddingWg.Wait()
	}

	return nil
}

func processFile(ctx context.Context, s *meb.MEBStore, ext Extractor, embedder *EmbeddingService, path string, projectName string, sourceRoot string, meta *ProjectMetadata, embeddingWg *sync.WaitGroup, sem chan struct{}, state *IngestState) error {
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
		addErr = s.AddDocumentWithTopic(s.TopicID(), string(relPath), content, nil, map[string]any{"project": projectName})
		if addErr == nil {
			logger.Debug("Successfully stored raw content", "file", relPath)
			break
		}
		// fast retry for conflicts
		time.Sleep(time.Millisecond * time.Duration(10*(retries+1)))
	}
	if addErr != nil {
		logger.Error("Failed to store raw content", "file", relPath, "error", addErr)
		return fmt.Errorf("failed to add document %s: %w", relPath, addErr)
	}

	// Store symbol documents (with file, start_line, end_line metadata for snippet extraction)
	for _, doc := range bundle.Documents {
		if err := s.AddDocumentWithTopic(s.TopicID(), doc.ID, nil, nil, doc.Metadata); err != nil {
			logger.Warn("Failed to add symbol doc", "doc_id", doc.ID, "error", err)
		}
	}

	// Embed documentation for semantic search (AFTER symbols are added to ensure IDs exist)
	if embedder != nil {
		docFactsFound := 0

		for _, fact := range bundle.Facts {
			if fact.Predicate == config.PredicateHasDoc {
				docFactsFound++
				docText, ok := fact.Object.(string)
				logger.Debug("Found doc for symbol", "subject", fact.Subject, "length", len(docText))

				if ok && len(docText) > 10 { // Only embed meaningful docs
					if embeddingWg != nil {
						embeddingWg.Add(1)
					} else {
						logger.Warn("embeddingWg is nil", "subject", fact.Subject)
					}

					go func(symbolID string, text string) {
						defer func() {
							if r := recover(); r != nil {
								logger.Error("Panic in embedding goroutine", "symbol", symbolID, "panic", r)
							}
						}()

						// Acquire semaphore
						if sem != nil {
							sem <- struct{}{}
							defer func() { <-sem }()
						}

						if embeddingWg != nil {
							defer embeddingWg.Done()
						}

						// Add a timeout to prevent hanging
						ctxWithTimeout, cancel := context.WithTimeout(context.Background(), config.EmbeddingTimeout)
						defer cancel()

						logger.Debug("Generating embedding", "symbol", symbolID, "length", len(text))
						embed, err := embedder.GetEmbedding(ctxWithTimeout, text)
						if err != nil {
							logger.Error("Error generating embedding", "symbol", symbolID, "error", err)
							return
						}

						if len(embed) == 0 {
							logger.Error("Empty embedding", "symbol", symbolID)
							return
						}

						// Look up the correct dictionary ID for the symbol
						dictID, found := s.LookupID(string(symbolID))
						if !found {
							logger.Error("ID not found in dictionary, cannot store vector", "symbol", symbolID)
							return
						}

						if err := s.Vectors().Add(dictID, embed); err != nil {
							logger.Error("Error adding vector to store", "symbol", symbolID, "error", err)
						} else {
							logger.Info("Successfully stored embedding", "symbol", symbolID, "dict_id", dictID)
						}
					}(fact.Subject, docText)
				} else if ok {
					logger.Debug("Skipping embedding, text too short", "subject", fact.Subject, "length", len(docText))
				}
			}
		}
	}

	finalFacts := make([]meb.Fact, 0, len(bundle.Facts)+2)

	// Inject Role Tags based on path or metadata
	tagged := false
	if meta != nil && meta.Components != nil {
		for _, comp := range meta.Components {
			if strings.Contains(relPath, comp.Path) {
				finalFacts = append(finalFacts, meb.Fact{Subject: string(relPath), Predicate: config.PredicateHasTag, Object: comp.Type})
				tagged = true
				break // Assume one component per file for now
			}
		}
	}

	if !tagged {
		if strings.HasSuffix(relPath, ".go") {
			finalFacts = append(finalFacts, meb.Fact{Subject: string(relPath), Predicate: config.PredicateHasTag, Object: "backend"})
		} else if strings.HasSuffix(relPath, ".ts") || strings.HasSuffix(relPath, ".tsx") {
			finalFacts = append(finalFacts, meb.Fact{Subject: string(relPath), Predicate: config.PredicateHasTag, Object: "frontend"})
		}
	}

	// Make sure file has type "file"
	finalFacts = append(finalFacts, meb.Fact{Subject: string(relPath), Predicate: config.PredicateType, Object: config.SymbolKindFile})

	hasNameCount := 0
	for _, f := range bundle.Facts {
		if f.Predicate == config.PredicateCalls {
			if objStr, ok := f.Object.(string); ok {
				if resolved, ok := state.SymbolTable[objStr]; ok {
					f.Object = resolved
				}
			}
		}

		// Track has_name facts for debug logging
		if f.Predicate == config.PredicateHasName {
			hasNameCount++
		}

		finalFacts = append(finalFacts, f)
	}

	logger.Debug("Total facts being added", "total", len(finalFacts), "has_name_count", hasNameCount)

	return s.AddFactBatch(finalFacts)
}

func isSupportedFile(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".go" || ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".py" || ext == ".md"
}

// hashToTopicID generates a deterministic 24-bit topic ID from a project name.
func hashToTopicID(name string) uint32 {
	if name == "" {
		return 1
	}
	var h uint32 = 2166136261 // FNV-1a offset basis
	for i := 0; i < len(name); i++ {
		h ^= uint32(name[i])
		h *= 16777619 // FNV-1a prime
	}
	return (h & 0xFFFFFF) | 1 // ensure non-zero (0 is reserved)
}

func TagRoles(s *meb.MEBStore) error {
	for fact, err := range s.ScanWithPruning("", config.PredicateHandledBy, "", keys.EntityFunc, false) {
		if err != nil {
			continue
		}
		h, ok := fact.Object.(string)
		if !ok {
			continue
		}
		s.AddFact(meb.Fact{Subject: string(h), Predicate: config.PredicateHasRole, Object: config.RoleAPIHandler})
	}
	for fact, err := range s.Scan("", config.PredicateInPackage, "") {
		if err != nil {
			continue
		}
		p, ok := fact.Object.(string)
		if !ok {
			continue
		}
		if strings.Contains(p, "types") || strings.Contains(p, "models") || strings.Contains(p, "meb") || strings.Contains(p, "ast") {
			s.AddFact(meb.Fact{Subject: fact.Subject, Predicate: config.PredicateHasRole, Object: config.RoleDataContract})
		}
	}
	return nil
}
