package ephemeral

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/duynguyendang/meb"
)

var (
	hunkHeaderRE = regexp.MustCompile(`^@@\s+-(\d+)(?:,(\d+))?\s+\+(\d+)(?:,(\d+))?\s+@@`)
	diffFileRE   = regexp.MustCompile(`^---\s+[ab]/(.+)$`)
	diffGitRE    = regexp.MustCompile(`^diff --git a/(.+) b/.+$`)
)

const (
	DiffAdded    = "diff_added"
	DiffRemoved  = "diff_removed"
	DiffModified = "diff_modified"
)

type removedEntry struct {
	oldLine int
	content string
}

// ParseDiff parses a unified git diff and stores facts into the session's Facts store.
// Returns the number of facts stored.
// Produces diff_added and diff_removed triples with file-relative line numbers.
// Also produces diff_modified triples for adjacent removal-addition pairs within a hunk.
func ParseDiff(diff string, session *Session) (int, error) {
	if diff == "" {
		return 0, nil
	}

	var currentFile string
	var facts []meb.Fact
	lines := strings.Split(diff, "\n")
	i := 0

	for i < len(lines) {
		line := lines[i]

		// Skip /dev/null headers for new/deleted files
		if strings.HasPrefix(line, "--- /dev/null") || strings.HasPrefix(line, "+++ /dev/null") {
			i++
			continue
		}

		// diff --git a/path b/path: git-diff header, sets current file
		if m := diffGitRE.FindStringSubmatch(line); len(m) > 0 {
			currentFile = m[1]
			i++
			continue
		}

		// File header: --- a/path (also handles +++ for existing files)
		if strings.HasPrefix(line, "--- ") {
			if m := diffFileRE.FindStringSubmatch(line); len(m) > 1 {
				currentFile = m[1]
			}
			i++
			continue
		}

		// +++ line: skip (file was already set by preceding ---)
		if strings.HasPrefix(line, "+++ ") {
			i++
			continue
		}

		// Hunk header
		if m := hunkHeaderRE.FindStringSubmatch(line); len(m) > 0 {
			oldStart, _ := strconv.Atoi(m[1])
			newStart, _ := strconv.Atoi(m[3])
			oldLine := oldStart
			newLine := newStart
			i++

			// Buffer for per-hunk modification detection:
			// adjacent removal(s) followed by addition(s) are tagged as modifications.
			var removedBuf []removedEntry

			for i < len(lines) {
				hunkLine := lines[i]

				// Empty line: context line with no prefix
				if hunkLine == "" {
					removedBuf = nil
					oldLine++
					newLine++
					i++
					continue
				}

				// Check for file header markers before routing by prefix.
				// These indicate the end of the current hunk.
				if strings.HasPrefix(hunkLine, "@@") || strings.HasPrefix(hunkLine, "diff ") ||
					strings.HasPrefix(hunkLine, "--- ") || strings.HasPrefix(hunkLine, "+++ ") {
					break
				}

				// Check first char to route to appropriate handler
				prefix := hunkLine[0]

				// Context line ' '
				if prefix == ' ' {
					removedBuf = nil
					oldLine++
					newLine++
					i++
					continue
				}

				// Removal line '-'
				if prefix == '-' {
					facts = append(facts, meb.Fact{
						Subject:   currentFile,
						Predicate: DiffRemoved,
						Object:    fmt.Sprintf("%d:%s", oldLine, hunkLine[1:]),
					})
					removedBuf = append(removedBuf, removedEntry{oldLine: oldLine, content: hunkLine[1:]})
					oldLine++
					i++
					continue
				}

				// Addition line '+'
				if prefix == '+' {
					content := hunkLine[1:]
					if len(removedBuf) > 0 {
						r := removedBuf[0]
						removedBuf = removedBuf[1:]
						facts = append(facts, meb.Fact{
							Subject:   currentFile,
							Predicate: DiffModified,
							Object:    fmt.Sprintf("%d:%d:%s", r.oldLine, newLine, content),
						})
					}
					facts = append(facts, meb.Fact{
						Subject:   currentFile,
						Predicate: DiffAdded,
						Object:    fmt.Sprintf("%d:%s", newLine, content),
					})
					newLine++
					i++
					continue
				}

				// Unknown content (e.g. backslash continuation): skip
				i++
				continue
			}
			continue
		}

		i++
	}

	if len(facts) == 0 {
		return 0, nil
	}

	if err := session.Facts.AddFactBatch(facts); err != nil {
		return 0, fmt.Errorf("store diff facts: %w", err)
	}

	return len(facts), nil
}

// ParseDiffAndCreateSession creates a new session and parses the diff into it.
func (es *EphemeralStore) ParseDiffAndCreateSession(projectID, diff string) (*Session, int, error) {
	session, err := es.NewSession(projectID)
	if err != nil {
		return nil, 0, fmt.Errorf("create session: %w", err)
	}

	count, err := ParseDiff(diff, session)
	if err != nil {
		_ = session.Close()
		es.DeleteSession(session.ID)
		return nil, 0, fmt.Errorf("parse diff: %w", err)
	}

	return session, count, nil
}