package ingest

import (
	"fmt"
	"strings"

	"github.com/duynguyendang/gca/pkg/common"
)

// normalizeBody strips comments, collapses whitespace, and trims to produce
// a canonical form for duplication detection. Two functions with the same
// structure but different variable names/comments produce different hashes.
func normalizeBody(s string) string {
	var b strings.Builder
	inComment := false
	inLineComment := false
	prevSpace := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		// Handle block comments.
		if inComment {
			if c == '*' && i+1 < len(s) && s[i+1] == '/' {
				inComment = false
				i++ // skip '/'
			}
			continue
		}
		// Handle line comments.
		if inLineComment {
			if c == '\n' {
				inLineComment = false
			}
			continue
		}

		// Detect comment start.
		if c == '/' && i+1 < len(s) {
			if s[i+1] == '/' {
				inLineComment = true
				i++ // skip second /
				continue
			}
			if s[i+1] == '*' {
				inComment = true
				i++ // skip *
				continue
			}
		}

		// Collapse whitespace.
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = false
		b.WriteByte(c)
	}

	return strings.TrimSpace(b.String())
}

// hashNormalizedBody returns a compact hash string of normalized function body.
func hashNormalizedBody(normalized string) string {
	h := common.FNV1aHash(normalized)
	return fmt.Sprintf("h%08x", h)
}
