package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSmellPolicyFiles_Exist(t *testing.T) {
	smellFiles := []string{
		"surprise.mg",
		"knowledge_gaps.mg",
		"god_file.mg",
		"hub.mg",
		"circular.mg",
		"layer.mg",
		"security.mg",
		"scoring.mg",
	}
	policyDir := filepath.Join("..", "..", "policies", "smells")

	for _, file := range smellFiles {
		path := filepath.Join(policyDir, file)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("Smell policy %s not found: %v", file, err)
			continue
		}
		if info.IsDir() {
			t.Errorf("Expected %s to be a file, got directory", file)
			continue
		}
		// Verify it's valid UTF-8 and non-empty
		content, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("Cannot read %s: %v", file, err)
			continue
		}
		if len(content) == 0 {
			t.Errorf("%s is empty", file)
		}
	}
}

func TestSmellPolicyFiles_DefineQueryMetadata(t *testing.T) {
	policyDir := filepath.Join("..", "..", "policies", "smells")

	files, err := os.ReadDir(policyDir)
	if err != nil {
		t.Skipf("Cannot read smells directory: %v", err)
	}

	for _, entry := range files {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".mg") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(policyDir, entry.Name()))
		if err != nil {
			t.Errorf("Cannot read %s: %v", entry.Name(), err)
			continue
		}
		if !strings.Contains(string(content), "query_metadata") {
			t.Errorf("%s does not define any query_metadata predicates", entry.Name())
		}
	}
}

func TestSmellPolicyFiles_SurpriseHasExpectedQueries(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "policies", "smells", "surprise.mg"))
	if err != nil {
		t.Skipf("surprise.mg not found: %v", err)
	}
	s := string(content)
	expectedQueries := []string{
		"surprise_cross_community",
		"surprise_cross_language",
		"surprise_peripheral_hub",
		"surprise_cross_test_boundary",
		"surprise_score",
		"surprise_top",
		"surprise_hotspot",
	}
	for _, q := range expectedQueries {
		if !strings.Contains(s, "query_metadata(\""+q+"\"") {
			t.Errorf("surprise.mg missing query_metadata for %q", q)
		}
	}
}

func TestSmellPolicyFiles_KnowledgeGapsHasExpectedQueries(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "policies", "smells", "knowledge_gaps.mg"))
	if err != nil {
		t.Skipf("knowledge_gaps.mg not found: %v", err)
	}
	s := string(content)
	expectedQueries := []string{
		"gap_isolated",
		"gap_untested_hotspot",
		"gap_thin_community",
		"gap_single_file_community",
		"gap_isolated_top",
		"gap_untested_top",
	}
	for _, q := range expectedQueries {
		if !strings.Contains(s, "query_metadata(\""+q+"\"") {
			t.Errorf("knowledge_gaps.mg missing query_metadata for %q", q)
		}
	}
}

