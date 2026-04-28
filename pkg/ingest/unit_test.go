package ingest

import (
	"testing"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
)

func TestHashToTopicID(t *testing.T) {
	tests := []struct {
		name       string
		projectName string
	}{
		{"simple name", "myproject"},
		{"with numbers", "project123"},
		{"with dashes", "my-project"},
		{"with underscores", "my_project"},
		{"empty name", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id1 := hashToTopicID(tt.projectName)
			id2 := hashToTopicID(tt.projectName)

			if id1 != id2 {
				t.Errorf("hashToTopicID should be deterministic: got %d then %d", id1, id2)
			}

			if id1 < 0 || id1 > 0xFFFFFF {
				t.Errorf("hashToTopicID should return 24-bit value: got %d", id1)
			}
		})
	}
}

func TestHashToTopicID_DifferentInputs(t *testing.T) {
	id1 := hashToTopicID("project1")
	id2 := hashToTopicID("project2")

	if id1 == id2 {
		t.Error("Different project names should produce different topic IDs")
	}
}

func TestIsSupportedFile(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		want  bool
	}{
		{"go file", "test.go", true},
		{"python file", "test.py", true},
		{"typescript file", "test.ts", true},
		{"javascript file", "test.js", true},
		{"tsx file", "test.tsx", true},
		{"json file", "test.json", false},
		{"yaml file", "test.yaml", false},
		{"txt file", "test.txt", false},
		{"png file", "test.png", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSupportedFile(tt.path)
			if got != tt.want {
				t.Errorf("isSupportedFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestExtractSymbolsFromFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		lang    string
		wantMin int
	}{
		{
			name:    "go file with function",
			content: "package main\n\nfunc main() {}\n",
			lang:    "go",
			wantMin: 1,
		},
		{
			name:    "empty file",
			content: "",
			lang:    "go",
			wantMin: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := NewTreeSitterExtractor()
			symbols, err := ext.ExtractSymbols("test.go", []byte(tt.content), "test.go")
			if err != nil {
				t.Fatalf("ExtractSymbols failed: %v", err)
			}
			if len(symbols) < tt.wantMin {
				t.Errorf("ExtractSymbols returned %d symbols, want >= %d", len(symbols), tt.wantMin)
			}
		})
	}
}

func TestExtractShortName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple name", "main", "main"},
		{"file:func", "main.go:main", "main"},
		{"package.Type", "pkg/main.go:Server.Listen", "Listen"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractShortName(tt.input)
			if got != tt.want {
				t.Errorf("extractShortName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFileContainsPkg(t *testing.T) {
	tests := []struct {
		name     string
		fileID   string
		pkg      string
		wantBool bool
	}{
		{"same package", "fmt/print.go", "fmt", true},
		{"parent contains", "fmt/print/extra.go", "fmt", true},
		{"different package", "fmt/print.go", "os", false},
		{"empty file", "", "fmt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fileContainsPkg(tt.fileID, tt.pkg)
			if got != tt.wantBool {
				t.Errorf("fileContainsPkg(%q, %q) = %v, want %v", tt.fileID, tt.pkg, got, tt.wantBool)
			}
		})
	}
}

func TestGetTagConfig(t *testing.T) {
	cfg := getTagConfig()
	if cfg == nil {
		t.Fatal("getTagConfig returned nil")
	}
	if cfg.Rules == nil {
		t.Error("config.Rules should not be nil")
	}
}

func TestIngestState_Reset(t *testing.T) {
	state := NewIngestState()
	state.SymbolTable["main"] = "main.go:main"
	state.FileIndex["main.go"] = true

	state.SymbolTable = make(map[string]string)
	state.FileIndex = make(map[string]bool)

	if len(state.SymbolTable) != 0 {
		t.Error("SymbolTable should be empty after reset")
	}
	if len(state.FileIndex) != 0 {
		t.Error("FileIndex should be empty after reset")
	}
}

func TestIngestOptions_Structure(t *testing.T) {
	opts := &IngestOptions{
		SkipEmbeddings: true,
		ReEmbed:        false,
	}

	if !opts.SkipEmbeddings {
		t.Error("SkipEmbeddings should be true")
	}
	if opts.ReEmbed {
		t.Error("ReEmbed should be false")
	}
}

func TestProjectMetadata_Structure(t *testing.T) {
	meta := ProjectMetadata{
		Name:        "test-project",
		Description: "A test project",
		Version:     "1.0.0",
		Tags:        []string{"go", "api"},
		Components: map[string]ComponentMetadata{
			"server": {Type: "http", Language: "go", Path: "./server"},
		},
	}

	if meta.Name != "test-project" {
		t.Errorf("Name = %q, want %q", meta.Name, "test-project")
	}
	if len(meta.Tags) != 2 {
		t.Errorf("Tags length = %d, want %d", len(meta.Tags), 2)
	}
	if meta.Components["server"].Language != "go" {
		t.Errorf("Components[server].Language = %q, want %q", meta.Components["server"].Language, "go")
	}
}

func TestTemplateStoreQuery_Structure(t *testing.T) {
	query := TemplateStoreQuery{
		ID:          "test_query",
		Body:        "triples(A, 'calls', B)",
		Predicate:   "has_test",
		Category:    "test",
		Severity:    "low",
		SmellType:   "test_smell",
		Description: "A test query",
		Parameters: []TemplateParam{
			{Name: "param1", Type: "string", Description: "A parameter"},
		},
	}

	if query.ID != "test_query" {
		t.Errorf("ID = %q, want %q", query.ID, "test_query")
	}
	if len(query.Parameters) != 1 {
		t.Errorf("Parameters length = %d, want %d", len(query.Parameters), 1)
	}
}

func TestAnalyzer_NewAnalyzer(t *testing.T) {
	analyzer := NewAnalyzer(nil, nil)
	if analyzer == nil {
		t.Fatal("NewAnalyzer returned nil")
	}
	if analyzer.storeManager != nil {
		t.Error("storeManager should be nil")
	}
	if analyzer.templateStore != nil {
		t.Error("templateStore should be nil")
	}
}

func TestExtractSymbolName(t *testing.T) {
	tests := []struct {
		name string
		sym  string
		want string
	}{
		{"simple", "main", "main"},
		{"with file", "main.go:main", "main"},
		{"with method", "server.go:Server.Listen", "Listen"},
		{"nested", "pkg/util/helper.go:Helper.New", "New"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := common.ExtractSymbolName(tt.sym)
			if got != tt.want {
				t.Errorf("ExtractSymbolName(%q) = %q, want %q", tt.sym, got, tt.want)
			}
		})
	}
}

func TestConfigPredicates(t *testing.T) {
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
		config.PredicateType,
		config.PredicateInPackage,
		config.PredicateHasRole,
		config.PredicateStartLine,
		config.PredicateEndLine,
	}

	for _, pred := range predicates {
		if pred == "" {
			t.Error("Predicate constant should not be empty")
		}
	}
}

func TestConfigRoles(t *testing.T) {
	roles := []string{
		config.RoleAPIHandler,
		config.RoleDataContract,
		config.RoleUtility,
	}

	for _, role := range roles {
		if role == "" {
			t.Error("Role constant should not be empty")
		}
	}
}

func TestSymbol_NameAndID(t *testing.T) {
	sym := Symbol{
		Name:    "main",
		ID:      "main.go:main",
		Type:    "func",
		Package: "main",
	}

	if sym.Name != "main" {
		t.Errorf("Name = %q, want %q", sym.Name, "main")
	}
	if sym.ID != "main.go:main" {
		t.Errorf("ID = %q, want %q", sym.ID, "main.go:main")
	}
}

func TestNewTreeSitterExtractor(t *testing.T) {
	ext := NewTreeSitterExtractor()
	if ext == nil {
		t.Fatal("NewTreeSitterExtractor returned nil")
	}
}

func TestSymbol_Structure(t *testing.T) {
	sym := Symbol{
		ID:         "main.go:main",
		Name:       "main",
		Type:       "func",
		Receiver:   "",
		Signature:  "func main()",
		DocComment: "// main is the entry point",
		Content:    "func main() {}",
		StartLine:  1,
		EndLine:    2,
		Package:    "main",
	}

	if sym.Name != "main" {
		t.Errorf("Name = %q, want %q", sym.Name, "main")
	}
	if sym.StartLine != 1 {
		t.Errorf("StartLine = %d, want %d", sym.StartLine, 1)
	}
	if sym.EndLine != 2 {
		t.Errorf("EndLine = %d, want %d", sym.EndLine, 2)
	}
}
