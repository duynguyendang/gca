package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/duynguyendang/meb"
)

const (
	FileHashPredicate = "file:hash"
	HashMapKey        = "gca:file_hashes"
	// FileGraphPrefix is used to create unique graph contexts per file for efficient cleanup
	FileGraphPrefix = "file:"
)

type FileHash struct {
	Path  string `json:"path"`
	Hash  string `json:"hash"`
	Mtime int64  `json:"mtime"`
}

type FileHashMap map[string]FileHash

// IncrementalState extends FileHashMap with git commit tracking.
type IncrementalState struct {
	FileHashes FileHashMap `json:"file_hashes"`
}

const MetadataSubject = "gca:metadata"

// GetLastCommitSHA retrieves the last ingested commit SHA from the store.
func GetLastCommitSHA(s *meb.MEBStore) string {
	for fact, err := range s.Scan(MetadataSubject, config.PredicateLastCommitSHA, "") {
		if err != nil {
			continue
		}
		if sha, ok := fact.Object.(string); ok {
			return sha
		}
	}
	return ""
}

// SaveLastCommitSHA stores the last ingested commit SHA to the store.
func SaveLastCommitSHA(s *meb.MEBStore, sha string) {
	s.DeleteFactsBySubject(MetadataSubject)
	s.AddFact(meb.Fact{
		Subject:   MetadataSubject,
		Predicate: config.PredicateLastCommitSHA,
		Object:    sha,
	})
}

// IncrementalStateKey is the storage key for the incremental state.
const IncrementalStateKey = "gca:incremental_state"

// LoadFileHashes loads the file hash map from the store.
func LoadFileHashes(s *meb.MEBStore) (FileHashMap, error) {
	content, err := s.GetContentByKey(HashMapKey)
	if err != nil {
		return make(FileHashMap), nil
	}
	var hashes FileHashMap
	if err := json.Unmarshal(content, &hashes); err != nil {
		return make(FileHashMap), err
	}
	return hashes, nil
}

// SaveFileHashes persists the file hash map to the store.
// Deprecated: Use SaveIncrementalState instead.
func SaveFileHashes(s *meb.MEBStore, hashes FileHashMap) error {
	data, err := json.Marshal(hashes)
	if err != nil {
		return err
	}
	return s.AddDocument(HashMapKey, data, nil, nil)
}

// LoadIncrementalState loads the incremental state from the store.
// Backward compatible: if old FileHashMap format is found, wraps it.
func LoadIncrementalState(s *meb.MEBStore) (*IncrementalState, error) {
	content, err := s.GetContentByKey(IncrementalStateKey)
	if err != nil {
		// Try legacy key
		legacy, legacyErr := s.GetContentByKey(HashMapKey)
		if legacyErr != nil {
			return &IncrementalState{FileHashes: make(FileHashMap)}, nil
		}
		var hashes FileHashMap
		if jsonErr := json.Unmarshal(legacy, &hashes); jsonErr != nil {
			return &IncrementalState{FileHashes: make(FileHashMap)}, jsonErr
		}
		return &IncrementalState{FileHashes: hashes}, nil
	}
	var state IncrementalState
	if err := json.Unmarshal(content, &state); err != nil {
		// Try parsing as plain FileHashMap (backward compat)
		var hashes FileHashMap
		if jsonErr := json.Unmarshal(content, &hashes); jsonErr != nil {
			return &IncrementalState{FileHashes: make(FileHashMap)}, err
		}
		return &IncrementalState{FileHashes: hashes}, nil
	}
	if state.FileHashes == nil {
		state.FileHashes = make(FileHashMap)
	}
	return &state, nil
}

// SaveIncrementalState persists the incremental state to the store.
func SaveIncrementalState(s *meb.MEBStore, state *IncrementalState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.AddDocument(IncrementalStateKey, data, nil, nil)
}

// computeFileHash calculates SHA256 hash and modification time for a file.
func computeFileHash(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	mtime := stat.ModTime().UnixNano()

	h := sha256.New()
	io.Copy(h, f)
	hash := hex.EncodeToString(h.Sum(nil))

	return hash, mtime, nil
}

// getFileGraphName returns the graph context name for a file.
// This enables efficient deletion of all facts belonging to a specific file.
func getFileGraphName(relPath string) string {
	return FileGraphPrefix + relPath
}

// deleteFileFacts removes all facts associated with a specific file.
func deleteFileFacts(s *meb.MEBStore, relPath string) error {
	if err := s.DeleteFactsBySubject(relPath); err != nil {
		logger.Warn("Failed to delete facts for file", "file", relPath, "error", err)
		return err
	}
	return nil
}

func RunIncremental(s *meb.MEBStore, projectName string, sourceDir string) error {
	state := NewIngestState()
	return RunIncrementalWithOptions(s, projectName, sourceDir, state, nil)
}

func RunIncrementalWithState(s *meb.MEBStore, projectName string, sourceDir string, state *IngestState) error {
	return RunIncrementalWithOptions(s, projectName, sourceDir, state, nil)
}

func RunIncrementalWithOptions(s *meb.MEBStore, projectName string, sourceDir string, state *IngestState, opts *IngestOptions) error {
	SetIngestState(state)
	ctx := context.Background()
	ext := NewTreeSitterExtractor()

	// Set topic ID for project-scoped ingestion
	topicID := hashToTopicID(projectName)
	s.SetTopicID(topicID)
	logger.Info("Using topic ID for incremental project", "topicID", topicID, "project", projectName)

	// Load incremental state (replaces LoadFileHashes)
	incrState, err := LoadIncrementalState(s)
	if err != nil {
		logger.Warn("Could not load incremental state, starting fresh", "error", err)
		incrState = &IncrementalState{FileHashes: make(FileHashMap)}
	}

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

	var projectMeta *ProjectMetadata
	metadataPath := filepath.Join(sourceDir, "project.yaml")
	if _, err := os.Stat(metadataPath); err == nil {
		logger.Info("Found project metadata", "path", metadataPath)
		var metaErr error
		projectMeta, metaErr = LoadProjectMetadata(metadataPath)
		if metaErr != nil {
			logger.Warn("Failed to load project metadata, continuing without it", "error", metaErr)
		} else if projectMeta != nil {
			s.AddFact(meb.Fact{
				Subject:   string(projectMeta.Name),
				Predicate: "type",
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
					Predicate: "has_tag",
					Object:    tag,
				})
			}
		}
	}

	changedFiles, deletedFiles, newHashes, usedGit := computeDiff(s, sourceDir, projectName, incrState, opts)

	mode := "hash"
	if usedGit {
		mode = "git"
	}
	logger.Info("Incremental Ingestion stats",
		"changed", len(changedFiles),
		"deleted", len(deletedFiles),
		"mode", mode)

	if len(changedFiles) == 0 && len(deletedFiles) == 0 {
		logger.Info("No changes detected. Skipping processing.")
		EnhanceVirtualTriples(s)
		TagRoles(ctx, s)
		return nil
	}

	if len(changedFiles) > 0 {
		logger.Info("Processing changed files", "count", len(changedFiles))

		// Clean up old facts for changed files before re-ingestion
		logger.Info("Cleaning up old facts for changed files")
		for _, path := range changedFiles {
			rel, _ := filepath.Rel(sourceDir, path)
			if projectName != "" {
				rel = filepath.Join(projectName, rel)
			}
			if err := cleanupFileFacts(s, rel); err != nil {
				logger.Warn("Failed to cleanup old facts", "file", rel, "error", err)
			}
		}

		state.SymbolTable = make(map[string]string)
		// Build symbol table from all known files
		symbolSource := newHashes
		if usedGit {
			// In git mode, use stored hashes as the full file list
			symbolSource = incrState.FileHashes
		}
		for path := range symbolSource {
			if isSupportedFile(path) {
				fullPath := path
				if projectName != "" {
					fullPath = filepath.Join(sourceDir, strings.TrimPrefix(path, projectName+"/"))
				}
				if content, err := os.ReadFile(fullPath); err == nil {
					symbols, _ := ext.ExtractSymbols(path, content, path)
					for _, sym := range symbols {
						state.SymbolTable[sym.Name] = sym.ID
						if sym.Package != "" {
							state.SymbolTable[sym.Package+"."+sym.Name] = sym.ID
						}
					}
				}
			}
		}

		jobs := make(chan string, 100)
		var wg sync.WaitGroup
		var embeddingWg sync.WaitGroup
		var passErr atomic.Uint64

		workerCount := runtime.NumCPU()
		if workerCount > config.MaxWorkers {
			workerCount = config.MaxWorkers
		}

		for i := 0; i < workerCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				localExt := NewTreeSitterExtractor()
				sem := make(chan struct{}, 10)
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
					rel, _ := filepath.Rel(sourceDir, path)
					logger.Debug("Processing file", "project", projectName, "file", rel)
					if err := processFile(ctx, path, &ProcessFileConfig{
						Store: s, Extractor: localExt, Embedder: embeddingService,
						ProjectName: projectName, SourceRoot: sourceDir, Meta: projectMeta,
						EmbeddingWg: &embeddingWg, Sem: sem, State: state, Options: opts,
					}); err != nil {
						logger.Error("Error processing file", "error", err)
						passErr.Add(1)
					}
				}
			}()
		}

		for _, path := range changedFiles {
			jobs <- path
		}
		close(jobs)
		wg.Wait()

		if embeddingService != nil {
			logger.Info("Waiting for embeddings to complete")
			embeddingWg.Wait()
		}
	}

	if len(deletedFiles) > 0 {
		logger.Info("Removing deleted files from graph", "count", len(deletedFiles))
		removeDeletedFiles(s, projectName, deletedFiles)
	}

	// Save state
	if usedGit {
		// In git mode, compute hashes for changed files only
		if newHashes == nil {
			newHashes = incrState.FileHashes
		}
		for _, absPath := range changedFiles {
			rel, _ := filepath.Rel(sourceDir, absPath)
			if projectName != "" {
				rel = filepath.Join(projectName, rel)
			}
			if h, m, hashErr := computeFileHash(absPath); hashErr == nil {
				newHashes[rel] = FileHash{Path: rel, Hash: h, Mtime: m}
			}
		}
		// Remove deleted files from hashes
		for _, del := range deletedFiles {
			delete(newHashes, del)
		}
	}

	// Save current HEAD as last commit SHA for git repos
	if usedGit {
		if head, headErr := GetHEADCommitSHA(sourceDir); headErr == nil {
			SaveLastCommitSHA(s, head)
		}
	}

	newState := &IncrementalState{
		FileHashes: newHashes,
	}
	if err := SaveIncrementalState(s, newState); err != nil {
		logger.Warn("Could not save incremental state", "error", err)
	}

	EnhanceVirtualTriples(s)
	TagRoles(ctx, s)

	return nil
}

func computeDiff(s *meb.MEBStore, sourceDir, projectName string, incrState *IncrementalState, opts *IngestOptions) (changedFiles []string, deletedFiles []string, newHashes FileHashMap, usedGit bool) {
	if IsGitRepo(sourceDir) {
		fromRef := ""
		if opts != nil && opts.FromCommit != "" {
			fromRef = opts.FromCommit
		} else if lastSHA := GetLastCommitSHA(s); lastSHA != "" {
			fromRef = lastSHA
		}

		if fromRef != "" {
			var diff *GitDiffResult
			var diffErr error

			if opts != nil && opts.ToCommit != "" {
				diff, diffErr = GitDiffBetweenCommits(fromRef, opts.ToCommit, sourceDir)
			} else {
				diff, diffErr = GitDiffToWorkingTree(fromRef, sourceDir)
			}

			if diffErr != nil {
				logger.Warn("Git diff failed, falling back to hash-based", "error", diffErr)
			} else {
				logger.Info("Git diff result",
					"from", diff.FromCommit,
					"to", diff.ToCommit,
					"changed", len(diff.ChangedFiles),
					"deleted", len(diff.DeletedFiles))

				for _, rel := range diff.ChangedFiles {
					changedFiles = append(changedFiles, filepath.Join(sourceDir, rel))
				}
				deletedFiles = diff.DeletedFiles
				usedGit = true
			}
		}
	}

	if !usedGit {
		newHashes = make(FileHashMap)
		existingFilePaths := make(map[string]bool)
		for path := range incrState.FileHashes {
			existingFilePaths[path] = true
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
				relPath, _ := filepath.Rel(sourceDir, path)
				if projectName != "" {
					relPath = filepath.Join(projectName, relPath)
				}

				hash, mtime, hashErr := computeFileHash(path)
				if hashErr != nil {
					logger.Warn("Could not hash file", "path", path, "error", hashErr)
					changedFiles = append(changedFiles, path)
					return nil
				}

				newHashes[relPath] = FileHash{Path: relPath, Hash: hash, Mtime: mtime}
				delete(existingFilePaths, relPath)

				existingHash, exists := incrState.FileHashes[relPath]
				if !exists || existingHash.Hash != hash {
					changedFiles = append(changedFiles, path)
				}
			}
			return nil
		})
		if err != nil {
			logger.Warn("Hash computation failed, treating all files as changed", "error", err)
			return nil, nil, nil, false
		}

		for path := range existingFilePaths {
			deletedFiles = append(deletedFiles, path)
		}
	}

	return
}

// removeDeletedFiles removes all facts associated with deleted files.
// Uses the file's graph context for efficient batch deletion.
func removeDeletedFiles(s *meb.MEBStore, projectName string, deletedFiles []string) {
	for _, filePath := range deletedFiles {
		if err := deleteFileFacts(s, filePath); err != nil {
			logger.Error("Failed to delete facts for deleted file", "file", filePath, "error", err)
		} else {
			logger.Info("Successfully removed facts for deleted file", "file", filePath)
		}
	}
}

// cleanupFileFacts removes all facts and vectors for a file before re-ingestion.
// This ensures old facts and vectors are cleared when a file is modified.
func cleanupFileFacts(s *meb.MEBStore, relPath string) error {
	// First, collect symbol IDs defined in this file so we can delete their vectors
	symbolIDs := []string{}
	for fact, err := range s.ScanContext(context.Background(), relPath, config.PredicateDefines, "") {
		if err != nil {
			continue
		}
		if objStr, ok := fact.Object.(string); ok {
			symbolIDs = append(symbolIDs, objStr)
		}
	}

	// Delete facts first
	if err := deleteFileFacts(s, relPath); err != nil {
		logger.Warn("Failed to delete facts for file", "file", relPath, "error", err)
		return err
	}

	// Delete vectors for each symbol defined in this file
	for _, symbolID := range symbolIDs {
		dictID, found := s.LookupID(symbolID)
		if !found {
			continue
		}
		if ok := s.Vectors().Delete(dictID); !ok {
			logger.Debug("No vector found for symbol", "symbolID", symbolID)
		} else {
			logger.Debug("Deleted vector for symbol", "symbolID", symbolID)
		}
	}

	return nil
}
