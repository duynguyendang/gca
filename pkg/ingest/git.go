package ingest

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/duynguyendang/gca/pkg/logger"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"
)

// GitDiffResult holds the result of a git diff operation.
type GitDiffResult struct {
	ChangedFiles []string // Relative paths of added + modified files
	DeletedFiles []string // Relative paths of deleted files
	FromCommit   string   // Resolved from SHA
	ToCommit     string   // Resolved to SHA (empty = working tree)
}

// IsGitRepo returns true if the directory is a git repository.
func IsGitRepo(dir string) bool {
	_, err := git.PlainOpen(dir)
	return err == nil
}

// GetHEADCommitSHA returns the SHA of the HEAD commit.
func GetHEADCommitSHA(dir string) (string, error) {
	repo, err := git.PlainOpen(dir)
	if err != nil {
		return "", fmt.Errorf("failed to open repo: %w", err)
	}
	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}
	return head.Hash().String(), nil
}

// ResolveCommit resolves a commit reference (full SHA, short SHA, branch name, "HEAD")
// to a plumbing.Hash.
func ResolveCommit(repo *git.Repository, ref string) (plumbing.Hash, error) {
	// Try as direct hash (full or short)
	h := plumbing.NewHash(ref)
	if h != plumbing.ZeroHash {
		if _, cErr := repo.CommitObject(h); cErr == nil {
			return h, nil
		}
	}
	// Try as reference (branch, tag, HEAD)
	resolved, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("cannot resolve commit %q: %w", ref, err)
	}
	return *resolved, nil
}

// GitDiffBetweenCommits returns the files changed between two commits.
// Only files matching isSupportedFile are included.
func GitDiffBetweenCommits(from, to, dir string) (*GitDiffResult, error) {
	repo, err := git.PlainOpen(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to open repo: %w", err)
	}

	fromHash, err := ResolveCommit(repo, from)
	if err != nil {
		return nil, fmt.Errorf("resolve from-commit: %w", err)
	}
	toHash, err := ResolveCommit(repo, to)
	if err != nil {
		return nil, fmt.Errorf("resolve to-commit: %w", err)
	}

	fromCommit, err := repo.CommitObject(fromHash)
	if err != nil {
		return nil, fmt.Errorf("get from-commit object: %w", err)
	}
	toCommit, err := repo.CommitObject(toHash)
	if err != nil {
		return nil, fmt.Errorf("get to-commit object: %w", err)
	}

	fromTree, err := fromCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("get from-tree: %w", err)
	}
	toTree, err := toCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("get to-tree: %w", err)
	}

	changes, err := object.DiffTree(fromTree, toTree)
	if err != nil {
		return nil, fmt.Errorf("diff trees: %w", err)
	}

	result := &GitDiffResult{
		FromCommit: fromHash.String(),
		ToCommit:   toHash.String(),
	}

	for _, change := range changes {
		action, aErr := change.Action()
		if aErr != nil {
			logger.Warn("Could not determine change action", "error", aErr)
			continue
		}

		var filePath string
		switch action {
		case merkletrie.Insert, merkletrie.Modify:
			filePath = change.To.Name
		case merkletrie.Delete:
			filePath = change.From.Name
		}

		if !isSupportedFile(filePath) {
			continue
		}

		switch action {
		case merkletrie.Insert, merkletrie.Modify:
			result.ChangedFiles = append(result.ChangedFiles, filePath)
		case merkletrie.Delete:
			result.DeletedFiles = append(result.DeletedFiles, filePath)
		}
	}

	return result, nil
}

// GitDiffToWorkingTree returns the files changed between a commit and the current
// working tree (including staged and unstaged changes).
func GitDiffToWorkingTree(from, dir string) (*GitDiffResult, error) {
	repo, err := git.PlainOpen(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to open repo: %w", err)
	}

	fromHash, err := ResolveCommit(repo, from)
	if err != nil {
		return nil, fmt.Errorf("resolve from-commit: %w", err)
	}

	result := &GitDiffResult{
		FromCommit: fromHash.String(),
		ToCommit:   "", // working tree
	}

	// Track files we've already seen to deduplicate
	seen := make(map[string]bool)

	// 1. Diff from commit to HEAD (committed changes since from)
	headRef, headErr := repo.Head()
	if headErr == nil && headRef.Hash() != fromHash {
		fromCommit, fcErr := repo.CommitObject(fromHash)
		if fcErr == nil {
			headCommit, hcErr := repo.CommitObject(headRef.Hash())
			if hcErr == nil {
				fromTree, ftErr := fromCommit.Tree()
				headTree, htErr := headCommit.Tree()
				if ftErr == nil && htErr == nil {
					changes, diffErr := object.DiffTree(fromTree, headTree)
					if diffErr == nil {
						for _, change := range changes {
							collectChange(change, seen, result)
						}
					}
				}
			}
		}
	}

	// 2. Diff from HEAD to working tree (dirty files)
	if headErr == nil {
		wt, wtErr := repo.Worktree()
		if wtErr == nil {
			status, stErr := wt.Status()
			if stErr == nil {
				for path, fileStatus := range status {
					if fileStatus.Worktree == git.Unmodified && fileStatus.Staging == git.Unmodified {
						continue
					}
					if !isSupportedFile(path) {
						continue
					}
					if seen[path] {
						continue
					}
					seen[path] = true

					fullPath := filepath.Join(dir, path)
					if _, statErr := os.Stat(fullPath); os.IsNotExist(statErr) {
						result.DeletedFiles = append(result.DeletedFiles, path)
					} else {
						result.ChangedFiles = append(result.ChangedFiles, path)
					}
				}
			}
		}
	}

	return result, nil
}

// collectChange extracts a single change into the result, filtering by isSupportedFile.
func collectChange(change *object.Change, seen map[string]bool, result *GitDiffResult) {
	action, aErr := change.Action()
	if aErr != nil {
		return
	}

	var filePath string
	switch action {
	case merkletrie.Insert, merkletrie.Modify:
		filePath = change.To.Name
	case merkletrie.Delete:
		filePath = change.From.Name
	}

	if !isSupportedFile(filePath) {
		return
	}
	if seen[filePath] {
		return
	}
	seen[filePath] = true

	switch action {
	case merkletrie.Insert, merkletrie.Modify:
		result.ChangedFiles = append(result.ChangedFiles, filePath)
	case merkletrie.Delete:
		result.DeletedFiles = append(result.DeletedFiles, filePath)
	}
}

// deduplicatePaths removes duplicate paths from a slice.
func deduplicatePaths(paths []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, p := range paths {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	return result
}
