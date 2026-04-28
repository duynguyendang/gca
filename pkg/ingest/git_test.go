package ingest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
)

// setupTestRepo creates a temp dir with git init and an initial commit
// containing a single main.go file. Returns the repo directory.
func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test User")
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial commit")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
}

func TestIsGitRepo_ValidRepo(t *testing.T) {
	dir := setupTestRepo(t)
	if !IsGitRepo(dir) {
		t.Error("IsGitRepo should return true for a valid git repo")
	}
}

func TestIsGitRepo_NotGitDir(t *testing.T) {
	dir := t.TempDir()
	if IsGitRepo(dir) {
		t.Error("IsGitRepo should return false for a non-git directory")
	}
}

func TestGetHEADCommitSHA(t *testing.T) {
	dir := setupTestRepo(t)
	sha, err := GetHEADCommitSHA(dir)
	if err != nil {
		t.Fatalf("GetHEADCommitSHA failed: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("SHA should be 40 chars, got %d: %q", len(sha), sha)
	}
}

func TestGitDiffBetweenCommits_AddFile(t *testing.T) {
	dir := setupTestRepo(t)
	sha1 := getHEAD(t, dir)

	writeFile(t, dir, "utils.go", "package main\n\nfunc helper() {}\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "add utils")

	diff, err := GitDiffBetweenCommits(sha1, "HEAD", dir)
	if err != nil {
		t.Fatalf("GitDiffBetweenCommits failed: %v", err)
	}

	if len(diff.ChangedFiles) != 1 {
		t.Fatalf("Expected 1 changed file, got %d: %v", len(diff.ChangedFiles), diff.ChangedFiles)
	}
	if diff.ChangedFiles[0] != "utils.go" {
		t.Errorf("Expected utils.go, got %s", diff.ChangedFiles[0])
	}
	if len(diff.DeletedFiles) != 0 {
		t.Errorf("Expected 0 deleted files, got %d", len(diff.DeletedFiles))
	}
}

func TestGitDiffBetweenCommits_ModifyFile(t *testing.T) {
	dir := setupTestRepo(t)
	sha1 := getHEAD(t, dir)

	writeFile(t, dir, "main.go", "package main\n\nfunc main() { println(\"hello\") }\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "modify main")

	diff, err := GitDiffBetweenCommits(sha1, "HEAD", dir)
	if err != nil {
		t.Fatalf("GitDiffBetweenCommits failed: %v", err)
	}

	if len(diff.ChangedFiles) != 1 {
		t.Fatalf("Expected 1 changed file, got %d: %v", len(diff.ChangedFiles), diff.ChangedFiles)
	}
	if diff.ChangedFiles[0] != "main.go" {
		t.Errorf("Expected main.go, got %s", diff.ChangedFiles[0])
	}
}

func TestGitDiffBetweenCommits_DeleteFile(t *testing.T) {
	dir := setupTestRepo(t)

	writeFile(t, dir, "temp.go", "package main\n\nfunc temp() {}\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "add temp")
	sha2 := getHEAD(t, dir)

	os.Remove(filepath.Join(dir, "temp.go"))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "delete temp")

	diff, err := GitDiffBetweenCommits(sha2, "HEAD", dir)
	if err != nil {
		t.Fatalf("GitDiffBetweenCommits failed: %v", err)
	}

	if len(diff.DeletedFiles) != 1 {
		t.Fatalf("Expected 1 deleted file, got %d: %v", len(diff.DeletedFiles), diff.DeletedFiles)
	}
	if diff.DeletedFiles[0] != "temp.go" {
		t.Errorf("Expected temp.go, got %s", diff.DeletedFiles[0])
	}
}

func TestGitDiffBetweenCommits_FiltersUnsupported(t *testing.T) {
	dir := setupTestRepo(t)
	sha1 := getHEAD(t, dir)

	writeFile(t, dir, "readme.txt", "Hello world")
	writeFile(t, dir, "app.go", "package main\n\nfunc app() {}\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "add mixed files")

	diff, err := GitDiffBetweenCommits(sha1, "HEAD", dir)
	if err != nil {
		t.Fatalf("GitDiffBetweenCommits failed: %v", err)
	}

	// .txt should be filtered out, .go should be kept
	if len(diff.ChangedFiles) != 1 {
		t.Fatalf("Expected 1 changed file (only .go), got %d: %v", len(diff.ChangedFiles), diff.ChangedFiles)
	}
	if diff.ChangedFiles[0] != "app.go" {
		t.Errorf("Expected app.go, got %s", diff.ChangedFiles[0])
	}
}

func TestGitDiffToWorkingTree_DirtyFile(t *testing.T) {
	dir := setupTestRepo(t)
	sha1 := getHEAD(t, dir)

	// Modify file without committing
	writeFile(t, dir, "main.go", "package main\n\nfunc main() { println(\"modified\") }\n")

	diff, err := GitDiffToWorkingTree(sha1, dir)
	if err != nil {
		t.Fatalf("GitDiffToWorkingTree failed: %v", err)
	}

	found := false
	for _, f := range diff.ChangedFiles {
		if f == "main.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected main.go in changed files, got %v", diff.ChangedFiles)
	}
}

func TestGitDiffToWorkingTree_UntrackedFile(t *testing.T) {
	dir := setupTestRepo(t)
	sha1 := getHEAD(t, dir)

	// Add untracked file (not git-added)
	writeFile(t, dir, "new.go", "package main\n\nfunc newFunc() {}\n")

	diff, err := GitDiffToWorkingTree(sha1, dir)
	if err != nil {
		t.Fatalf("GitDiffToWorkingTree failed: %v", err)
	}

	found := false
	for _, f := range diff.ChangedFiles {
		if f == "new.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected new.go in changed files, got %v", diff.ChangedFiles)
	}
}

func TestGitDiffToWorkingTree_DeletedWorkingTree(t *testing.T) {
	dir := setupTestRepo(t)
	sha1 := getHEAD(t, dir)

	// Delete file from working tree without committing
	os.Remove(filepath.Join(dir, "main.go"))

	diff, err := GitDiffToWorkingTree(sha1, dir)
	if err != nil {
		t.Fatalf("GitDiffToWorkingTree failed: %v", err)
	}

	found := false
	for _, f := range diff.DeletedFiles {
		if f == "main.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected main.go in deleted files, got deleted=%v changed=%v", diff.DeletedFiles, diff.ChangedFiles)
	}
}

func TestResolveCommit_FullSHA(t *testing.T) {
	dir := setupTestRepo(t)
	repo, err := openRepo(dir)
	if err != nil {
		t.Fatalf("openRepo failed: %v", err)
	}

	sha := getHEAD(t, dir)
	hash, err := ResolveCommit(repo, sha)
	if err != nil {
		t.Fatalf("ResolveCommit failed: %v", err)
	}
	if hash.String() != sha {
		t.Errorf("Expected %s, got %s", sha, hash.String())
	}
}

func TestResolveCommit_HEAD(t *testing.T) {
	dir := setupTestRepo(t)
	repo, err := openRepo(dir)
	if err != nil {
		t.Fatalf("openRepo failed: %v", err)
	}

	sha := getHEAD(t, dir)
	hash, err := ResolveCommit(repo, "HEAD")
	if err != nil {
		t.Fatalf("ResolveCommit(HEAD) failed: %v", err)
	}
	if hash.String() != sha {
		t.Errorf("Expected %s, got %s", sha, hash.String())
	}
}

func TestResolveCommit_BranchName(t *testing.T) {
	dir := setupTestRepo(t)
	repo, err := openRepo(dir)
	if err != nil {
		t.Fatalf("openRepo failed: %v", err)
	}

	sha := getHEAD(t, dir)

	// Get the actual branch name (could be main or master)
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("repo.Head failed: %v", err)
	}
	branchName := head.Name().Short()

	hash, err := ResolveCommit(repo, branchName)
	if err != nil {
		t.Fatalf("ResolveCommit(%s) failed: %v", branchName, err)
	}
	if hash.String() != sha {
		t.Errorf("Expected %s, got %s", sha, hash.String())
	}
}

func TestResolveCommit_InvalidRef(t *testing.T) {
	dir := setupTestRepo(t)
	repo, err := openRepo(dir)
	if err != nil {
		t.Fatalf("openRepo failed: %v", err)
	}

	_, err = ResolveCommit(repo, "nonexistent-ref-12345")
	if err == nil {
		t.Error("ResolveCommit should fail for nonexistent ref")
	}
}

func TestDeduplicatePaths(t *testing.T) {
	input := []string{"a.go", "b.go", "a.go", "c.go", "b.go"}
	result := deduplicatePaths(input)
	if len(result) != 3 {
		t.Errorf("Expected 3 unique paths, got %d: %v", len(result), result)
	}
}

func TestDeduplicatePaths_Empty(t *testing.T) {
	result := deduplicatePaths(nil)
	if len(result) != 0 {
		t.Errorf("Expected empty result, got %d", len(result))
	}
}

func getHEAD(t *testing.T, dir string) string {
	t.Helper()
	sha, err := GetHEADCommitSHA(dir)
	if err != nil {
		t.Fatalf("GetHEADCommitSHA failed: %v", err)
	}
	return sha
}

func openRepo(dir string) (*git.Repository, error) {
	return git.PlainOpen(dir)
}
