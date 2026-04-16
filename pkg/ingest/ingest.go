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

// IngestOptions controls embedding behavior during ingestion.
type IngestOptions struct {
	SkipEmbeddings bool // Skip all embedding generation
	ReEmbed        bool // Re-embed ALL symbols (not just has_doc facts)
}

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
	return RunWithOptions(s, projectName, sourceDir, state, nil)
}

// RunWithState executes the ingestion process with explicit state management.
func RunWithState(s *meb.MEBStore, projectName string, sourceDir string, state *IngestState) error {
	return RunWithOptions(s, projectName, sourceDir, state, nil)
}

// RunWithOptions executes the ingestion process with explicit state and embedding options.
func RunWithOptions(s *meb.MEBStore, projectName string, sourceDir string, state *IngestState, opts *IngestOptions) error {
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
	logger.Info("Pass 2: Processing files", "project", projectName)
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
				logger.Debug("Processing file", "project", projectName, "file", rel)
				if err := processFile(ctx, s, localExt, embeddingService, path, projectName, sourceDir, projectMeta, &embeddingWg, sem, state, opts); err != nil {
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
		logger.Info("Waiting for embeddings to complete")
		embeddingWg.Wait()
	}

	return nil
}

// symbolEmbedTarget holds a symbol ID and text to embed
type symbolEmbedTarget struct {
	symbolID string
	text     string
}

// buildEmbedText constructs embedding text for re-embedding.
// Uses has_name (symbol name), has_doc (doc comment), and content from the bundle.
// The symbolID is used to look up related facts in the bundle.
func buildEmbedText(symbolID string, bundleFacts []meb.Fact, content []byte) string {
	var parts []string

	// Look up name and doc from facts
	var name, doc string
	for _, fact := range bundleFacts {
		if string(fact.Subject) == symbolID {
			if fact.Predicate == config.PredicateHasName {
				if n, ok := fact.Object.(string); ok {
					name = n
				}
			} else if fact.Predicate == config.PredicateHasDoc {
				if d, ok := fact.Object.(string); ok {
					doc = d
				}
			}
		}
	}

	if name != "" {
		parts = append(parts, name)
	}
	if doc != "" {
		parts = append(parts, doc)
	}
	// Add content preview (truncated to avoid bloat)
	if len(content) > 0 {
		contentStr := string(content)
		if len(contentStr) > 500 {
			contentStr = contentStr[:500] + "..."
		}
		parts = append(parts, contentStr)
	}

	return strings.Join(parts, "\n---\n")
}

func processFile(ctx context.Context, s *meb.MEBStore, ext Extractor, embedder *EmbeddingService, path string, projectName string, sourceRoot string, meta *ProjectMetadata, embeddingWg *sync.WaitGroup, sem chan struct{}, state *IngestState, opts *IngestOptions) error {
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
