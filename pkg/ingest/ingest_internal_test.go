package ingest

import (
	"context"
	"testing"

	"github.com/duynguyendang/gca/pkg/config"
)

func TestSymbolResolver_NewSymbolResolver(t *testing.T) {
	sr := NewSymbolResolver(nil)
	if sr == nil {
		t.Fatal("NewSymbolResolver returned nil")
	}
	if sr.store != nil {
		t.Error("store should be nil")
	}
	if sr.importMap == nil {
		t.Error("importMap should be initialized")
	}
}

func TestSymbolResolver_BuildSymbolIndex(t *testing.T) {
	sr := NewSymbolResolver(nil)
	err := sr.BuildSymbolIndex(nil)
	if err != nil {
		t.Errorf("BuildSymbolIndex should return nil, got: %v", err)
	}
}

func TestSymbolResolver_scoreCandidate(t *testing.T) {
	sr := &SymbolResolver{}

	tests := []struct {
		name      string
		sym       string
		pkg       string
		shortName string
		callerDir string
		wantMin   int
	}{
		{
			name:      "same directory",
			sym:       "pkg/utils/helper.go:Helper",
			pkg:       "",
			shortName: "Helper",
			callerDir: "pkg/utils",
			wantMin:   100, // same dir bonus
		},
		{
			name:      "parent contains pkg",
			sym:       "mypkg/utils/helper.go:Helper",
			pkg:       "mypkg",
			shortName: "Helper",
			callerDir: "other/pkg",
			wantMin:   50, // parent contains pkg bonus
		},
		{
			name:      "short name match suffix",
			sym:       "pkg/main.go:main",
			pkg:       "",
			shortName: "main",
			callerDir: "other",
			wantMin:   25, // suffix match bonus
		},
		{
			name:      "no matching criteria",
			sym:       "com unrelated/pkg/other.go:Other",
			pkg:       "mypkg",
			shortName: "Helper",
			callerDir: "somewhere/else",
			wantMin:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := sr.scoreCandidate(tt.sym, tt.pkg, tt.shortName, tt.callerDir)
			if score < tt.wantMin {
				t.Errorf("scoreCandidate(%q, %q, %q, %q) = %d, want >= %d",
					tt.sym, tt.pkg, tt.shortName, tt.callerDir, score, tt.wantMin)
			}
		})
	}
}

func TestSymbolResolver_findBestCandidate_EmptyCandidates(t *testing.T) {
	sr := &SymbolResolver{}
	result := sr.findBestCandidate(context.Background(), []string{}, "", "", "")
	if result != "" {
		t.Errorf("findBestCandidate with empty candidates should return empty string")
	}
}

func TestSymbolResolver_findBestCandidate_SingleCandidate(t *testing.T) {
	sr := &SymbolResolver{}
	candidates := []string{"pkg/main.go:main"}
	result := sr.findBestCandidate(context.Background(), candidates, "", "", "")
	if result != "pkg/main.go:main" {
		t.Errorf("findBestCandidate with single candidate should return that candidate")
	}
}

func TestFileHash_Structure(t *testing.T) {
	fh := FileHash{
		Path:  "test.go",
		Hash:  "abc123",
		Mtime: 1234567890,
	}

	if fh.Path != "test.go" {
		t.Errorf("Path = %q, want %q", fh.Path, "test.go")
	}
	if fh.Hash != "abc123" {
		t.Errorf("Hash = %q, want %q", fh.Hash, "abc123")
	}
	if fh.Mtime != 1234567890 {
		t.Errorf("Mtime = %d, want %d", fh.Mtime, 1234567890)
	}
}

func TestFileHashMap_Type(t *testing.T) {
	hashes := make(FileHashMap)
	hashes["test.go"] = FileHash{Path: "test.go", Hash: "hash123"}

	if len(hashes) != 1 {
		t.Errorf("FileHashMap length = %d, want %d", len(hashes), 1)
	}
	if hashes["test.go"].Hash != "hash123" {
		t.Errorf("FileHashMap[test.go].Hash = %q, want %q", hashes["test.go"].Hash, "hash123")
	}
}

func TestGetFileGraphName(t *testing.T) {
	tests := []struct {
		relPath string
		want    string
	}{
		{"main.go", "file:main.go"},
		{"pkg/server/handlers.go", "file:pkg/server/handlers.go"},
		{"", "file:"},
	}

	for _, tt := range tests {
		t.Run(tt.relPath, func(t *testing.T) {
			got := getFileGraphName(tt.relPath)
			if got != tt.want {
				t.Errorf("getFileGraphName(%q) = %q, want %q", tt.relPath, got, tt.want)
			}
		})
	}
}

func TestIngestState_InitialState(t *testing.T) {
	state := NewIngestState()
	if state == nil {
		t.Fatal("NewIngestState returned nil")
	}
	if state.SymbolTable == nil {
		t.Error("SymbolTable should be initialized")
	}
	if state.FileIndex == nil {
		t.Error("FileIndex should be initialized")
	}
}

func TestIngestState_WithFile(t *testing.T) {
	state := NewIngestState()
	state.FileIndex["test.go"] = true

	if !state.FileIndex["test.go"] {
		t.Error("FileIndex should contain test.go")
	}
}

func TestIngestState_WithSymbol(t *testing.T) {
	state := NewIngestState()
	state.SymbolTable["main"] = "main.go:main"

	if state.SymbolTable["main"] != "main.go:main" {
		t.Errorf("SymbolTable[main] = %q, want %q", state.SymbolTable["main"], "main.go:main")
	}
}

func TestIngestOptions_Defaults(t *testing.T) {
	opts := &IngestOptions{}

	if opts.SkipEmbeddings != false {
		t.Errorf("SkipEmbeddings default should be false")
	}
	if opts.ReEmbed != false {
		t.Errorf("ReEmbed default should be false")
	}
}

func TestIngestOptions_Custom(t *testing.T) {
	opts := &IngestOptions{
		SkipEmbeddings: true,
		ReEmbed:        true,
	}

	if !opts.SkipEmbeddings {
		t.Error("SkipEmbeddings should be true")
	}
	if !opts.ReEmbed {
		t.Error("ReEmbed should be true")
	}
}

func TestConfig_PredicateConstants(t *testing.T) {
	predicates := []string{
		config.PredicateDefines,
		config.PredicateCalls,
		config.PredicateImports,
		config.PredicateHasName,
		config.PredicateHasKind,
		config.PredicateHasLanguage,
		config.PredicateHasTag,
		config.PredicateReferences,
		config.PredicateHandledBy,
		config.PredicateCallsAPI,
	}

	for _, pred := range predicates {
		if pred == "" {
			t.Error("Predicate constant should not be empty")
		}
	}
}
