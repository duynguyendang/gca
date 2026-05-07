package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	LoadPolicyDirective = "load_policy"
	DefaultInitFile     = "policies/init.mg"
)

type PolicyManifest struct {
	InitPath   string
	PolicyDir  string
	FilePaths  []string
	LoadedBy   map[string]string
}

func (m *PolicyManifest) ResolvePath(relativePath string) string {
	return filepath.Join(m.PolicyDir, relativePath)
}

func ResolveInitPath() string {
	if path := os.Getenv("GCA_INIT_DL"); path != "" {
		return path
	}
	return DefaultInitFile
}

func LoadManifest(initPath string) (*PolicyManifest, error) {
	absInit, err := filepath.Abs(initPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve init path: %w", err)
	}

	data, err := os.ReadFile(absInit)
	if err != nil {
		return nil, fmt.Errorf("init.mg not found or unreadable (strict mode): %w", err)
	}

	policyDir := filepath.Dir(absInit)

	manifest := &PolicyManifest{
		InitPath:  absInit,
		PolicyDir: policyDir,
		LoadedBy:  make(map[string]string),
	}

	lines := strings.Split(string(data), "\n")
	directiveRe := regexp.MustCompile(`load_policy\s*\(\s*"([^"]+)"\s*\)`)

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "%") {
			continue
		}

		matches := directiveRe.FindStringSubmatch(line)
		if len(matches) == 0 {
			continue
		}

		relativePath := matches[1]
		if !strings.HasSuffix(relativePath, ".mg") {
			return nil, fmt.Errorf("line %d: only .mg files supported, got %s", lineNum+1, relativePath)
		}

		fullPath := filepath.Join(policyDir, relativePath)
		if _, err := os.Stat(fullPath); err != nil {
			return nil, fmt.Errorf("line %d: policy file not found: %s", lineNum+1, relativePath)
		}

		manifest.FilePaths = append(manifest.FilePaths, fullPath)
		manifest.LoadedBy[fullPath] = relativePath
	}

	if len(manifest.FilePaths) == 0 {
		return nil, fmt.Errorf("no load_policy directives found in init.mg")
	}

	return manifest, nil
}