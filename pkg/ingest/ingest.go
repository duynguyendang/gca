package ingest

import (
	"fmt"
	"hash/fnv"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"context"

	"github.com/duynguyendang/gca/pkg/meb"
)

var symbolTable = make(map[string]string)

// Run executes the ingestion process.
func Run(s *meb.MEBStore, sourceDir string) error {
	// Initialize Embedding Service
	ctx := context.Background()
	embedService, initErr := NewEmbeddingService(ctx)
	if initErr != nil {
		return fmt.Errorf("failed to initialize embedding service: %w", initErr)
	}
	defer embedService.Close()

	ext := NewExtractor()

	testDir := sourceDir

	// Pass 1: Collect Symbols
	fmt.Println("Pass 1: Collecting symbols...")
	err := filepath.WalkDir(testDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			relPath, _ := filepath.Rel(testDir, path)
			symbols, err := ext.ExtractSymbols(path, content, relPath)
			if err != nil {
				log.Printf("Error extracting symbols from %s: %v", path, err)
			}
			for _, sym := range symbols {
				symbolTable[sym.Name] = sym.ID
				// Also map package.SymName? The extractor doesn't return pkg name yet in Symbol directly, wait it does.
				if sym.Package != "" {
					symbolTable[sym.Package+"."+sym.Name] = sym.ID
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error in Pass 1: %w", err)
	}
	fmt.Printf("Collected %d symbols\n", len(symbolTable))

	// Pass 2: Process Files
	fmt.Println("Pass 2: Processing files...")
	err = filepath.WalkDir(testDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			fmt.Printf("Processing file: %s\n", path)
			if !d.IsDir() && strings.HasSuffix(path, ".go") {
				fmt.Printf("Processing file: %s\n", path)
				if err := processFile(ctx, s, embedService, ext, path, testDir); err != nil {
					log.Printf("Error processing file %s: %v", path, err)
				}
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("error walking directory: %w", err)
	}

	return nil
}

// collectSymbols removed (logic moved to Run)

func processFile(ctx context.Context, s *meb.MEBStore, embedder *EmbeddingService, ext *Extractor, path string, sourceRoot string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	relPath, _ := filepath.Rel(sourceRoot, path)

	// fileId := strHash(path)
	// s.SetContent(fileId, content) // Optional: Store full file content too? Or rely on documents?
	// Storing full file content is good for "file retrieval".
	// Let's keep it but skip AddDocument for file itself for now? Or maybe add it?
	// User req: "Each code entity (Function, Struct, Interface) must be stored as a Document"

	symbols, err := ext.ExtractSymbols(path, content, relPath)
	if err != nil {
		return err
	}

	for _, sym := range symbols {
		// Embed string: DocComment + Signature
		embedText := fmt.Sprintf("%s\n%s", sym.Signature, sym.DocComment)
		// Fallback if empty doc
		if sym.DocComment == "" {
			embedText = sym.Signature
		}

		vec, err := embedder.GetEmbedding(ctx, embedText)
		if err != nil {
			log.Printf("Warning: embedding failed for %s: %v", sym.ID, err)
			continue
		}

		meta := map[string]any{
			"type":       sym.Type,
			"file":       relPath,
			"start_line": sym.StartLine,
			"end_line":   sym.EndLine,
			"package":    sym.Package,
		}

		if err := s.AddDocument(sym.ID, []byte(sym.Content), vec, meta); err != nil {
			log.Printf("Error adding document %s: %v", sym.ID, err)
		}

		// Add structural facts
		s.AddFact(meb.Fact{Subject: sym.ID, Predicate: "type", Object: sym.Type, Graph: "default"})
		s.AddFact(meb.Fact{Subject: sym.ID, Predicate: "defines", Object: sym.Name, Graph: "default"})
		s.AddFact(meb.Fact{Subject: relPath, Predicate: "defines_symbol", Object: sym.ID, Graph: "default"})
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
			}
			s.AddFact(meb.Fact{Subject: ref.Subject, Predicate: ref.Predicate, Object: object, Graph: "default"})
		}
	}

	return nil
}

func clean(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\"", ""))
}

func strHash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}
