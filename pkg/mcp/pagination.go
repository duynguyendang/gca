package mcp

import (
	"encoding/base64"
	"strconv"
)

// Cursor helpers provide opaque, offset-based pagination tokens. A cursor is a
// base64-encoded decimal offset so clients never depend on implementation
// details; empty cursor means "from the start".
//
// Responses that paginate include "next_cursor": a token to fetch the following
// page, or "" when there are no more results. Clients pass the token back via
// the "cursor" argument.

// encodeCursor encodes a 0-based offset as an opaque cursor string.
func encodeCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

// decodeCursor decodes a cursor token into a 0-based offset. Empty or invalid
// cursors are treated as 0, matching "start from the beginning".
func decodeCursor(cursor string) int {
	if cursor == "" {
		return 0
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0
	}
	offset, err := strconv.Atoi(string(raw))
	if err != nil || offset < 0 {
		return 0
	}
	return offset
}

// slicePage slices results[start:end] and returns the page plus a next page
// cursor token ("" if exhausted). start is the 0-based offset; limit <= 0 means
// no artificial cap.
func slicePage(results []string, start, limit int) (page []string, next string) {
	if start < 0 {
		start = 0
	}
	if start >= len(results) {
		return []string{}, ""
	}
	end := len(results)
	if limit > 0 && start+limit < end {
		end = start + limit
		next = encodeCursor(end)
	}
	return results[start:end], next
}
