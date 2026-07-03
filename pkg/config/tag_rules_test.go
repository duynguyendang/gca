package config

import (
	"regexp"
	"testing"
)

func TestDefaultTagRules(t *testing.T) {
	rules := DefaultTagRules()

	if len(rules) == 0 {
		t.Fatal("DefaultTagRules should return at least one rule")
	}

	// Check that we have rules for each tag type
	tags := make(map[string]bool)
	for _, r := range rules {
		tags[r.Tag] = true
		if r.Pattern == nil {
			t.Errorf("Rule for tag %q has nil Pattern", r.Tag)
		}
		if r.Weight < 0 {
			t.Errorf("Rule for tag %q has negative weight %d", r.Tag, r.Weight)
		}
	}

	wantTags := []string{TagPublicAPI, TagSanitizer, TagDatabase}
	for _, tag := range wantTags {
		if !tags[tag] {
			t.Errorf("DefaultTagRules missing tag %q", tag)
		}
	}
}

func TestProjectTagConfig_MatchingTags(t *testing.T) {
	cfg := &ProjectTagConfig{
		Rules: []TagRule{
			{Tag: "api", Pattern: regexp.MustCompile(`.*_handler\.go$`), Weight: 10},
			{Tag: "db", Pattern: regexp.MustCompile(`.*_repo\.go$`), Weight: 5},
		},
	}

	tests := []struct {
		path     string
		wantTags int
		wantHas  []string
	}{
		{"foo_handler.go", 1, []string{"api"}},
		{"bar_repo.go", 1, []string{"db"}},
		{"baz_handler.go", 1, []string{"api"}},
		{"qux_repo.go", 1, []string{"db"}},
		{"other.go", 0, nil},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			tags := cfg.MatchingTags(tt.path)
			if len(tags) != tt.wantTags {
				t.Errorf("MatchingTags(%q) = %v, want %d tags", tt.path, tags, tt.wantTags)
			}
			for _, want := range tt.wantHas {
				found := false
				for _, tag := range tags {
					if tag == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("MatchingTags(%q) missing tag %q", tt.path, want)
				}
			}
		})
	}
}

func TestProjectTagConfig_HasTag(t *testing.T) {
	cfg := &ProjectTagConfig{
		Rules: []TagRule{
			{Tag: "api", Pattern: regexp.MustCompile(`.*_handler\.go$`), Weight: 10},
		},
	}

	tests := []struct {
		path string
		tag  string
		want bool
	}{
		{"foo_handler.go", "api", true},
		{"foo_handler.go", "db", false},
		{"other.go", "api", false},
		{"other.go", "db", false},
	}

	for _, tt := range tests {
		t.Run(tt.path+"_"+tt.tag, func(t *testing.T) {
			if got := cfg.HasTag(tt.path, tt.tag); got != tt.want {
				t.Errorf("HasTag(%q, %q) = %v, want %v", tt.path, tt.tag, got, tt.want)
			}
		})
	}
}

func TestProjectTagConfig_TagWeight(t *testing.T) {
	cfg := &ProjectTagConfig{
		Rules: []TagRule{
			{Tag: "api", Pattern: regexp.MustCompile(`.*`), Weight: 10},
			{Tag: "db", Pattern: regexp.MustCompile(`.*`), Weight: 5},
		},
	}

	tests := []struct {
		tag  string
		want int
	}{
		{"api", 10},
		{"db", 5},
		{"missing", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			if got := cfg.TagWeight(tt.tag); got != tt.want {
				t.Errorf("TagWeight(%q) = %d, want %d", tt.tag, got, tt.want)
			}
		})
	}
}

func TestProjectTagConfig_EmptyRules(t *testing.T) {
	cfg := &ProjectTagConfig{Rules: []TagRule{}}

	if len(cfg.MatchingTags("any/path.go")) != 0 {
		t.Error("Empty rules should return no tags")
	}
	if cfg.HasTag("any/path.go", "anytag") {
		t.Error("Empty rules should not have any tag")
	}
	if cfg.TagWeight("anytag") != 0 {
		t.Error("Empty rules should return 0 weight")
	}
}

func TestConfig_TagTestFileConstant(t *testing.T) {
	if TagTestFile != "test_file" {
		t.Errorf("Expected TagTestFile='test_file', got %q", TagTestFile)
	}
}

func TestConfig_TagTestSymbolConstant(t *testing.T) {
	if TagTestSymbol != "test_symbol" {
		t.Errorf("Expected TagTestSymbol='test_symbol', got %q", TagTestSymbol)
	}
}

func TestTagRules_TestFilePattern(t *testing.T) {
	rules := DefaultTagRules()
	if len(rules) == 0 {
		t.Fatal("DefaultTagRules should not be empty")
	}
	hasTestFileRule := false
	for _, r := range rules {
		if r.Tag == TagTestFile && r.Pattern != nil {
			hasTestFileRule = true
			if !r.Pattern.MatchString("foo_test.go") {
				t.Errorf("Test file pattern should match foo_test.go")
			}
			if !r.Pattern.MatchString("test_bar.py") {
				t.Errorf("Test file pattern should match test_bar.py")
			}
			if r.Pattern.MatchString("normal.go") {
				t.Errorf("Test file pattern should NOT match normal.go")
			}
			break
		}
	}
	if !hasTestFileRule {
		t.Error("DefaultTagRules should include a rule for TagTestFile")
	}
}
