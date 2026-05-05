package config

import (
	"regexp"
)

// TagRule defines a regex pattern for tagging files with an architectural role.
type TagRule struct {
	Tag     string
	Pattern *regexp.Regexp
	Weight  int // For scoring — higher weight = more critical smell
}

// ProjectTagConfig holds the active tagging rules for a project.
type ProjectTagConfig struct {
	Rules []TagRule
}

// DefaultTagRules returns the fallback regex-based rules used when no gca.yaml is present.
func DefaultTagRules() []TagRule {
	return []TagRule{
		{
			Tag: TagPublicAPI,
			Pattern: regexp.MustCompile(`^(api|controllers|routers|handlers)/|` +
				`^(cmd|cmd/api|internal/api|internal/handlers)/|` +
				`.*_handler\.go$|` +
				`.*_router\.go$|` +
				`.*_controller\.go$`),
			Weight: 10,
		},
		{
			Tag: TagSanitizer,
			Pattern: regexp.MustCompile(`^(middleware|security|auth)/|` +
				`^(internal/middleware|internal/security)/|` +
				`.*_(middleware|auth|sanitizer|validator|guard)\.go$`),
			Weight: 0, // Sanitizers reduce risk, not add it
		},
		{
			Tag: TagDatabase,
			Pattern: regexp.MustCompile(`^(db|repository|models|dal|data)/|` +
				`^(internal/dal|internal/repo|internal/persistence)/|` +
				`.*_(repository|dao|db|store|query)\.go$|` +
				`.*/migrations?/.*`),
			Weight: 5,
		},
		{
			Tag: TagTestFile,
			Pattern: regexp.MustCompile(`_test\.go$|_test\.py$|_test\.ts$|_test\.tsx$|` +
				`_tests\.go$|test_.*\.py$|test_.*\.ts$|test_.*\.tsx$|` +
				`.*\.spec\.(ts|js|py)$|.*\.test\.(ts|js|py)$|` +
				`tests?/.*`),
			Weight: 0, // Test files don't add risk
		},
	}
}

// LoadTagRulesFromYAML parses a gca.yaml map into []TagRule.
// The yaml map comes from parsing `tagging_rules:` in gca.yaml.
func LoadTagRulesFromYAML(yamlRules []map[string]any) ([]TagRule, error) {
	rules := make([]TagRule, 0, len(yamlRules))
	for _, r := range yamlRules {
		tag, ok := r["tag"].(string)
		if !ok || tag == "" {
			continue
		}
		patternStr, ok := r["pattern"].(string)
		if !ok || patternStr == "" {
			continue
		}
		re, err := regexp.Compile(patternStr)
		if err != nil {
			return nil, err
		}
		weight := 0
		if w, ok := r["weight"].(int); ok {
			weight = w
		}
		rules = append(rules, TagRule{Tag: tag, Pattern: re, Weight: weight})
	}
	return rules, nil
}

// MatchingTags returns all tags that match the given file path.
func (ptc *ProjectTagConfig) MatchingTags(filePath string) []string {
	var tags []string
	for _, rule := range ptc.Rules {
		if rule.Pattern.MatchString(filePath) {
			tags = append(tags, rule.Tag)
		}
	}
	return tags
}

// HasTag checks if any rule would tag this file with the given tag.
func (ptc *ProjectTagConfig) HasTag(filePath, tag string) bool {
	for _, rule := range ptc.Rules {
		if rule.Tag == tag && rule.Pattern.MatchString(filePath) {
			return true
		}
	}
	return false
}

// TagWeight returns the weight for a given tag, or 0 if not found.
func (ptc *ProjectTagConfig) TagWeight(tag string) int {
	for _, rule := range ptc.Rules {
		if rule.Tag == tag {
			return rule.Weight
		}
	}
	return 0
}
