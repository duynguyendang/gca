package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/duynguyendang/gca/pkg/ingest"
)

func main() {
	cwd, _ := os.Getwd()
	filePath := filepath.Join(cwd, "pkg/meb/vector/math.go")
	relPath := "pkg/meb/vector/math.go"

	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Failed to read file: %v", err)
	}

	extractor := ingest.NewTreeSitterExtractor()
	symbols, err := extractor.ExtractSymbols(filePath, content, relPath)
	if err != nil {
		log.Fatalf("Failed to extract symbols: %v", err)
	}

	fmt.Printf("Extracted %d symbols from %s\n", len(symbols), relPath)
	for _, sym := range symbols {
		if sym.Name == "ProcessMRL" {
			fmt.Printf("Symbol: %s\n", sym.Name)
			fmt.Printf("ID: %s\n", sym.ID)
			fmt.Printf("Type: %s\n", sym.Type)
			fmt.Printf("StartLine: %d, EndLine: %d\n", sym.StartLine, sym.EndLine)
			fmt.Printf("Content Length: %d\n", len(sym.Content))
			fmt.Printf("Content Preview: %.50s...\n", sym.Content)
		}
	}
}
