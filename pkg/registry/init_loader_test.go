package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveInitPath(t *testing.T) {
	// Save and restore GCA_INIT_DL
	orig := os.Getenv("GCA_INIT_DL")
	defer func() {
		if orig != "" {
			os.Setenv("GCA_INIT_DL", orig)
		} else {
			os.Unsetenv("GCA_INIT_DL")
		}
	}()

	os.Unsetenv("GCA_INIT_DL")
	if got := ResolveInitPath(); got != DefaultInitFile {
		t.Errorf("ResolveInitPath() without env = %q, want %q", got, DefaultInitFile)
	}

	os.Setenv("GCA_INIT_DL", "custom/path/init.mg")
	if got := ResolveInitPath(); got != "custom/path/init.mg" {
		t.Errorf("ResolveInitPath() with GCA_INIT_DL = %q, want %q", got, "custom/path/init.mg")
	}
}

func TestPolicyManifestResolvePath(t *testing.T) {
	m := &PolicyManifest{
		PolicyDir: "/workspace/policies",
	}
	if got := m.ResolvePath("smells/circular.mg"); got != "/workspace/policies/smells/circular.mg" {
		t.Errorf("ResolvePath() = %q, want %q", got, "/workspace/policies/smells/circular.mg")
	}
}

func TestLoadManifest(t *testing.T) {
	// Create a temp directory with a valid init.mg
	tmpDir := t.TempDir()
	initPath := filepath.Join(tmpDir, "init.mg")
	subDir := filepath.Join(tmpDir, "policies")

	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a valid init.mg
	validInit := `load_policy("policies/queries.mg")
load_policy("policies/security.mg")`
	if err := os.WriteFile(initPath, []byte(validInit), 0644); err != nil {
		t.Fatal(err)
	}

	// Write referenced files
	if err := os.WriteFile(filepath.Join(subDir, "queries.mg"), []byte("% queries\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "security.mg"), []byte("% security\n"), 0644); err != nil {
		t.Fatal(err)
	}

	manifest, err := LoadManifest(initPath)
	if err != nil {
		t.Fatalf("LoadManifest(%q) err = %v, want nil", initPath, err)
	}
	if len(manifest.FilePaths) != 2 {
		t.Errorf("FilePaths len = %d, want 2", len(manifest.FilePaths))
	}
	if manifest.PolicyDir != tmpDir {
		t.Errorf("PolicyDir = %q, want %q", manifest.PolicyDir, tmpDir)
	}
}

func TestLoadManifestFileNotFound(t *testing.T) {
	_, err := LoadManifest("/nonexistent/path/init.mg")
	if err == nil {
		t.Error("LoadManifest for nonexistent file expected error, got nil")
	}
}

func TestLoadManifestMissingPolicyFile(t *testing.T) {
	tmpDir := t.TempDir()
	initPath := filepath.Join(tmpDir, "init.mg")

	initWithMissing := `load_policy("policies/missing.mg")`
	if err := os.WriteFile(initPath, []byte(initWithMissing), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadManifest(initPath)
	if err == nil {
		t.Error("LoadManifest with missing policy file expected error, got nil")
	}
}

func TestLoadManifestNoDirectives(t *testing.T) {
	tmpDir := t.TempDir()
	initPath := filepath.Join(tmpDir, "init.mg")

	// File with no load_policy directives
	if err := os.WriteFile(initPath, []byte("% just a comment\n# not a directive"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadManifest(initPath)
	if err == nil {
		t.Error("LoadManifest with no directives expected error, got nil")
	}
}

func TestLoadManifestNonMgFile(t *testing.T) {
	tmpDir := t.TempDir()
	initPath := filepath.Join(tmpDir, "init.mg")

	badInit := `load_policy("policies/security.yaml")`
	if err := os.WriteFile(initPath, []byte(badInit), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadManifest(initPath)
	if err == nil {
		t.Error("LoadManifest with non-.mg file expected error, got nil")
	}
}