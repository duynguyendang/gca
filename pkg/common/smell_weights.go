package common

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// resolvePolicyPath locates a policy file given its path relative to the gca
// backend root (e.g. "policies/smells/scoring.mg"). Production runs with CWD =
// gca/ (see config.GenePoolPath); unit tests run from a package subdirectory, so
// we walk up from CWD until the file is found.
func resolvePolicyPath(rel string) string {
	if _, err := os.Stat(rel); err == nil {
		return rel
	}
	dir, err := os.Getwd()
	if err != nil {
		return rel
	}
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, rel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return rel
}

// SmellWeightPolicyFiles returns the scoring policy files whose smell_weight/2
// facts are the single source of truth for health scoring.
func SmellWeightPolicyFiles() []string {
	return []string{
		filepath.Join("policies", "smells", "scoring.mg"),
		filepath.Join("policies", "okf", "smells", "scoring.mg"),
	}
}

// smellWeightFactPattern matches `smell_weight("type", N).` declarations.
var smellWeightFactPattern = regexp.MustCompile(`smell_weight\s*\(\s*"([^"]+)"\s*,\s*(\d+)\s*\)\s*\.`)

// securityTypePattern matches `security_type("name").` declarations in
// policies/smells/security.mg — the exact has_smell_type values that should be
// classified as security smells.
var securityTypePattern = regexp.MustCompile(`security_type\s*\(\s*"([^"]+)"\s*\)\s*\.`)

// LoadSmellWeights parses smell_weight("type", N) facts from the scoring policy
// files and returns smell type -> weight. Unreadable/missing files are skipped
// (the caller decides how to handle a partial result).
func LoadSmellWeights() map[string]int {
	weights := make(map[string]int)
	for _, path := range SmellWeightPolicyFiles() {
		content, err := os.ReadFile(resolvePolicyPath(path))
		if err != nil {
			continue
		}
		for _, m := range smellWeightFactPattern.FindAllStringSubmatch(string(content), -1) {
			if w, err := strconv.Atoi(m[2]); err == nil {
				weights[m[1]] = w
			}
		}
	}
	return weights
}

// LoadSecuritySmellTypes parses the security_type("name") declarations in
// policies/smells/security.mg, returning the exact has_smell_type values that
// should be classified as security smells.
func LoadSecuritySmellTypes() map[string]bool {
	types := make(map[string]bool)
	content, err := os.ReadFile(resolvePolicyPath(filepath.Join("policies", "smells", "security.mg")))
	if err != nil {
		return types
	}
	for _, m := range securityTypePattern.FindAllStringSubmatch(string(content), -1) {
		types[m[1]] = true
	}
	return types
}
