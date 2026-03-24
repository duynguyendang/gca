package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

	"github.com/duynguyendang/gca/pkg/config"
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
func SaveFileHashes(s *meb.MEBStore, hashes FileHashMap) error {
	data, err := json.Marshal(hashes)
	if err != nil {
		return err
	}
	return s.AddDocument(HashMapKey, data, nil, nil)
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
// Uses the file's graph context for efficient batch deletion.
func deleteFileFacts(s *meb.MEBStore, relPath string) error {
	graphName := getFileGraphName(relPath)
	if err := s.DeleteGraph(graphName); err != nil {
		log.Printf("Warning: Failed to delete facts for file %s: %v", relPath, err)
		return err
	}
	return nil
}

func RunIncremental(s *meb.MEBStore, projectName string, sourceDir string) error {
	state := NewIngestState()
	return RunIncrementalWithState(s, projectName, sourceDir, state)
}

func RunIncrementalWithState(s *meb.MEBStore, projectName string, sourceDir string, state *IngestState) error {
	SetIngestState(state)
	ctx := context.Background()
	ext := NewTreeSitterExtractor()

	existingHashes, err := LoadFileHashes(s)
	if err != nil {
		log.Printf("Warning: Could not load existing hashes: %v (starting fresh)", err)
		existingHashes = make(FileHashMap)
	}

	embeddingService, embeddingErr := NewEmbeddingService(ctx)
	if embeddingErr != nil {
		log.Printf("Warning: Embedding service unavailable: %v (skipping doc embeddings)", embeddingErr)
	} else {
		defer embeddingService.Close()
		log.Println("Embedding service initialized for semantic doc search")
	}

	var projectMeta *ProjectMetadata
	metadataPath := filepath.Join(sourceDir, "project.yaml")
	if _, err := os.Stat(metadataPath); err == nil {
		fmt.Printf("Found project metadata at %s\n", metadataPath)
		projectMeta, _ = LoadProjectMetadata(metadataPath)
		if projectMeta != nil {
			s.AddFact(meb.Fact{
				Subject:   string(projectMeta.Name),
				Predicate: "type",
				Object:    "project",
				Graph:     "default",
			})
			s.AddFact(meb.Fact{
				Subject:   string(projectMeta.Name),
				Predicate: "description",
				Object:    projectMeta.Description,
				Graph:     "default",
			})
			for _, tag := range projectMeta.Tags {
				s.AddFact(meb.Fact{
					Subject:   string(projectMeta.Name),
					Predicate: "has_tag",
					Object:    tag,
					Graph:     "default",
				})
			}
		}
	}

	newHashes := make(FileHashMap)
	changedFiles := []string{}
	deletedFiles := []string{}

	existingFilePaths := make(map[string]bool)
	for path := range existingHashes {
		existingFilePaths[path] = true
	}

	err = filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
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

			hash, mtime, hashErr := computeFileHash(path)
			if hashErr != nil {
				log.Printf("Warning: Could not hash %s: %v", path, hashErr)
				changedFiles = append(changedFiles, path)
				return nil
			}

			newHashes[relPath] = FileHash{Path: relPath, Hash: hash, Mtime: mtime}
			delete(existingFilePaths, relPath)

			existingHash, exists := existingHashes[relPath]
			if !exists || existingHash.Hash != hash {
				changedFiles = append(changedFiles, path)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("hash computation failed: %w", err)
	}

	for path := range existingFilePaths {
		deletedFiles = append(deletedFiles, path)
	}

	fmt.Printf("Incremental Ingestion: %d changed, %d deleted, %d unchanged\n",
		len(changedFiles), len(deletedFiles), len(newHashes)-len(changedFiles))

	if len(changedFiles) == 0 && len(deletedFiles) == 0 {
		fmt.Println("No changes detected. Skipping processing.")
		EnhanceVirtualTriples(s)
		TagRoles(s)
		return nil
	}

	if len(changedFiles) > 0 {
		fmt.Printf("Processing %d changed files...\n", len(changedFiles))

		// Clean up old facts for changed files before re-ingestion
		fmt.Println("Cleaning up old facts for changed files...")
		for _, path := range changedFiles {
			rel, _ := filepath.Rel(sourceDir, path)
			if projectName != "" {
				rel = filepath.Join(projectName, rel)
			}
			if err := cleanupFileFacts(s, rel); err != nil {
				log.Printf("Warning: Failed to cleanup old facts for %s: %v", rel, err)
			}
		}

		state.SymbolTable = make(map[string]string)
		for path := range newHashes {
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
				for path := range jobs {
					rel, _ := filepath.Rel(sourceDir, path)
					fmt.Printf("  Processing %s/%s...\n", projectName, rel)
					if err := processFile(ctx, s, localExt, embeddingService, path, projectName, sourceDir, projectMeta, &embeddingWg, sem, state); err != nil {
						log.Printf("Error: %v", err)
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
			fmt.Println("Waiting for embeddings to complete...")
			embeddingWg.Wait()
		}
	}

	if len(deletedFiles) > 0 {
		fmt.Printf("Removing %d deleted files from graph...\n", len(deletedFiles))
		removeDeletedFiles(s, projectName, deletedFiles)
	}

	if err := SaveFileHashes(s, newHashes); err != nil {
		log.Printf("Warning: Could not save file hashes: %v", err)
	}

	EnhanceVirtualTriples(s)
	TagRoles(s)

	return nil
}

// removeDeletedFiles removes all facts associated with deleted files.
// Uses the file's graph context for efficient batch deletion.
func removeDeletedFiles(s *meb.MEBStore, projectName string, deletedFiles []string) {
	for _, filePath := range deletedFiles {
		if err := deleteFileFacts(s, filePath); err != nil {
			log.Printf("Error: Failed to delete facts for %s: %v", filePath, err)
		} else {
			log.Printf("Successfully removed facts for deleted file: %s", filePath)
		}
	}
}

// cleanupFileFacts removes all facts for a file before re-ingestion.
// This ensures old facts are cleared when a file is modified.
func cleanupFileFacts(s *meb.MEBStore, relPath string) error {
	return deleteFileFacts(s, relPath)
}
