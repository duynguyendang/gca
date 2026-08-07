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

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

// IngestOptions controls embedding behavior during ingestion.
type IngestOptions struct {
	SkipEmbeddings bool   // Skip all embedding generation
	ReEmbed        bool   // Re-embed ALL symbols (not just has_doc facts)
	FromCommit     string // Start commit SHA for git-based incremental ingestion
	ToCommit       string // End commit SHA for git-based incremental (empty = working tree)
}

// IngestState is defined in types.go

// Run executes the ingestion process with an optional projectName prefix.
func Run(s *meb.MEBStore, projectName string, sourceDir string) error {
	state := NewIngestState()
	return RunWithOptions(s, projectName, sourceDir, state, nil)
}

// RunWithState executes the ingestion process with explicit state management.
func RunWithState(s *meb.MEBStore, projectName string, sourceDir string, state *IngestState) error {
	return RunWithOptions(s, projectName, sourceDir, state, nil)
}

// RunWithOptions executes the ingestion process with explicit state and embedding options.
func RunWithOptions(s *meb.MEBStore, projectName string, sourceDir string, state *IngestState, opts *IngestOptions) error {
	SetIngestState(state)
	state.ProjectName = projectName
	ctx := context.Background()
	ext := NewTreeSitterExtractor()

	// Set topic ID for project-scoped ingestion
	// Uses a hash of the project name to generate a unique 24-bit topic ID
	topicID := hashToTopicID(projectName)
	s.SetTopicID(topicID)
	logger.Info("Using topic ID for project", "topic_id", topicID, "project", projectName)

	var embeddingService *EmbeddingService
	var embeddingErr error

	// Skip embedding initialization if requested
	if opts != nil && opts.SkipEmbeddings {
		logger.Info("Skipping embeddings due to --no-embed flag or SKIP_EMBEDDINGS env var")
	} else {
		embeddingService, embeddingErr = NewEmbeddingService(ctx)
		if embeddingErr != nil {
			logger.Warn("Embedding service unavailable, skipping doc embeddings", "error", embeddingErr)
		} else {
			defer embeddingService.Close()
			logger.Info("Embedding service initialized for semantic doc search")
		}
	}

	logger.Info("Pass 1: Collecting symbols and index", "project", projectName)
	state.SymbolTable = make(map[string]string)
	state.FileIndex = make(map[string]bool)
	state.FileContentCache = make(map[string][]byte)

	// Check for project metadata
	var projectMeta *ProjectMetadata
	metadataPath := filepath.Join(sourceDir, "project.yaml")
	if _, err := os.Stat(metadataPath); err == nil {
		logger.Info("Found project metadata", "path", metadataPath)
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
			if config.IsSkippedDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if isSupportedFile(path) {
			relPath, relErr := filepath.Rel(sourceDir, path)
			if relErr != nil {
				logger.Error("Failed to get relative path", "path", path, "error", relErr)
				return relErr
			}
			if projectName != "" {
				relPath = filepath.Join(projectName, relPath)
			}
			state.FileIndex[relPath] = true

			content, readErr := os.ReadFile(path)
			if readErr != nil {
				logger.Error("Failed to read file", "path", path, "error", readErr)
				return readErr
			}
			state.FileContentCache[relPath] = content
			symbols, extractErr := ext.ExtractSymbols(path, content, relPath)
			if extractErr != nil {
				logger.Error("Failed to extract symbols", "path", path, "error", extractErr)
				return extractErr
			}
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
	logger.Info("Pass 2: Processing files", "project", projectName)
	jobs := make(chan string, 100)
	var wg sync.WaitGroup
	var embeddingWg sync.WaitGroup // Wait for embeddings to finish
	var pass2Err atomic.Uint64

	workerCount := runtime.NumCPU()
	if workerCount > config.MaxWorkers {
		workerCount = config.MaxWorkers
	}

	// Shared semaphore for embeddings limit (max 10 concurrent)
	sem := make(chan struct{}, 10)

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			localExt := NewTreeSitterExtractor()
			for {
				var path string
				var ok bool
				select {
				case <-ctx.Done():
					return
				case path, ok = <-jobs:
					if !ok {
						return
					}
				}
				rel, relErr := filepath.Rel(sourceDir, path)
				if relErr != nil {
					logger.Error("Failed to get relative path", "path", path, "error", relErr)
					pass2Err.Add(1)
					continue
				}
				logger.Debug("Processing file", "project", projectName, "file", rel)
				if err := processFile(ctx, path, &ProcessFileConfig{
					Store: s, Extractor: localExt, Embedder: embeddingService,
					ProjectName: projectName, SourceRoot: sourceDir, Meta: projectMeta,
					EmbeddingWg: &embeddingWg, Sem: sem, State: state, Options: opts,
				}); err != nil {
					logger.Error("Failed to process file", "error", err)
					pass2Err.Add(1)
				}
			}
		}()
	}

	var walkErr error
	if err := filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			walkErr = err
			return nil // Continue walking despite error
		}
		if d.IsDir() {
			if config.IsSkippedDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if isSupportedFile(path) {
			jobs <- path
		}
		return nil
	}); err != nil {
		walkErr = err
	}
	close(jobs)
	wg.Wait()

	// Final Passes
	EnhanceVirtualTriples(s)
	TagRoles(ctx, s)

	// OKF link resolution: resolve raw links into okf_link and bridges_to facts
	if err := resolveOKFLinks(ctx, s, projectName, sourceDir); err != nil {
		logger.Warn("OKF link resolution failed", "error", err)
	}

	// Save HEAD commit SHA for git-based incremental detection
	if IsGitRepo(sourceDir) {
		if head, headErr := GetHEADCommitSHA(sourceDir); headErr == nil {
			SaveLastCommitSHA(s, head)
		}
	}

	// Record the schema version used to produce this store.
	SaveSchemaVersion(s, config.SchemaVersion)

	if embeddingService != nil {
		logger.Info("Waiting for embeddings to complete")
		embeddingWg.Wait()
	}

	// Return error if any files failed to process
	if pass2Err.Load() > 0 {
		return fmt.Errorf("ingestion completed with %d errors", pass2Err.Load())
	}
	if walkErr != nil {
		return fmt.Errorf("pass 2 walk failed: %w", walkErr)
	}
	return nil
}

// ProcessFileConfig holds all dependencies for processFile.
type ProcessFileConfig struct {
	Store       *meb.MEBStore
	Extractor   Extractor
	Embedder    *EmbeddingService
	ProjectName string
	SourceRoot  string
	Meta        *ProjectMetadata
	EmbeddingWg *sync.WaitGroup
	Sem         chan struct{}
	State       *IngestState
	Options     *IngestOptions
}

func processFile(ctx context.Context, path string, cfg *ProcessFileConfig) error {
	s := cfg.Store
	ext := cfg.Extractor
	embedder := cfg.Embedder
	projectName := cfg.ProjectName
	sourceRoot := cfg.SourceRoot
	meta := cfg.Meta
	embeddingWg := cfg.EmbeddingWg
	sem := cfg.Sem
	state := cfg.State
	opts := cfg.Options
	relPath, relErr := filepath.Rel(sourceRoot, path)
	if relErr != nil {
		return fmt.Errorf("failed to get relative path for %s: %w", path, relErr)
	}

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

	content, ok := state.FileContentCache[relPath]
	if !ok {
		var readErr error
		content, readErr = os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
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
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond * time.Duration(10*(retries+1))):
			// continue retry
		}
	}
	if addErr != nil {
		logger.Error("Failed to store raw content", "file", relPath, "error", addErr)
		return fmt.Errorf("failed to add document %s: %w", relPath, addErr)
	}

	// Pre-create dict entries for all symbols so async embedding goroutines
	// can look them up via LookupID(). Actual metadata facts are stored in
	// the single transaction below alongside bundle facts.
	for _, doc := range bundle.Documents {
		if _, err := s.Dict().GetOrCreateID(doc.ID); err != nil {
			logger.Warn("Failed to pre-create dict entry for symbol", "doc_id", doc.ID, "error", err)
		}
	}

	// Embed documentation for semantic search (AFTER dict entries exist)
	if embedder != nil {
		docFactsFound := 0

		// Determine which symbols to embed
		var symbolsToEmbed []symbolEmbedTarget

		if opts != nil && opts.ReEmbed {
			// ReEmbed mode: embed ALL symbols from their source code
			for _, doc := range bundle.Documents {
				// Build embed text from name + doc + content
				text := buildEmbedText(doc.ID, bundle.Facts, doc.Content)
				if len(text) > 10 {
					symbolsToEmbed = append(symbolsToEmbed, symbolEmbedTarget{
						symbolID: doc.ID,
						text:     text,
					})
				}
			}
			logger.Debug("Re-embed mode: embedding all symbols", "count", len(symbolsToEmbed))
		} else {
			// Normal mode: only embed has_doc facts > 10 chars
			for _, fact := range bundle.Facts {
				if fact.Predicate == config.PredicateHasDoc {
					docFactsFound++
					docText, ok := fact.Object.(string)
					if ok && len(docText) > 10 {
						symbolsToEmbed = append(symbolsToEmbed, symbolEmbedTarget{
							symbolID: fact.Subject,
							text:     docText,
						})
					}
				}
			}
		}

		for _, target := range symbolsToEmbed {
			if embeddingWg != nil {
				embeddingWg.Add(1)
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
			}(target.symbolID, target.text)
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

	// Language detection
	var lang string
	switch {
	case strings.HasSuffix(relPath, ".go"):
		lang = "go"
	case strings.HasSuffix(relPath, ".py"):
		lang = "python"
	case strings.HasSuffix(relPath, ".ts") || strings.HasSuffix(relPath, ".tsx"):
		lang = "typescript"
	case strings.HasSuffix(relPath, ".js") || strings.HasSuffix(relPath, ".jsx"):
		lang = "javascript"
	case strings.HasSuffix(relPath, ".java"):
		lang = "java"
	case strings.HasSuffix(relPath, ".rs"):
		lang = "rust"
	}
	if lang != "" {
		finalFacts = append(finalFacts, meb.Fact{Subject: string(relPath), Predicate: config.PredicateHasLanguage, Object: lang})
	}

	// Make sure file has type "file" and kind "file"
	finalFacts = append(finalFacts, meb.Fact{Subject: string(relPath), Predicate: config.PredicateType, Object: config.SymbolKindFile})
	finalFacts = append(finalFacts, meb.Fact{Subject: string(relPath), Predicate: config.PredicateHasKind, Object: config.SymbolKindFile})

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

	// Cross-subsystem transaction: symbol metadata + bundle facts in one atomic batch.
	// Reduces N separate AddDocumentWithTopic calls + AddFactBatch to a single Update().
	return s.Update(func(txn *meb.StoreTxn) error {
		var allSymFacts []meb.Fact
		for _, doc := range bundle.Documents {
			for k, v := range doc.Metadata {
				allSymFacts = append(allSymFacts, meb.Fact{Subject: doc.ID, Predicate: k, Object: v})
			}
		}
		if len(allSymFacts) > 0 {
			if err := txn.AddFactBatchWithTopic(allSymFacts, s.TopicID()); err != nil {
				logger.Warn("Failed to store symbol metadata batch", "count", len(allSymFacts), "error", err)
			}
		}
		// All bundle facts
		return txn.AddFactBatchWithTopic(finalFacts, s.TopicID())
	})
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
	h := common.FNV1aHash(name)
	return (h & 0xFFFFFF) | 1 // ensure non-zero (0 is reserved)
}
