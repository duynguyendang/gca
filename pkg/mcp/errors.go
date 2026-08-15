package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// Tool error codes returned in a consistent envelope: {"error","code","details"}.
// Clients (coding agents) can branch on code rather than parsing free text.
const (
	// ErrCodeInvalidArgument is a missing/invalid required argument.
	ErrCodeInvalidArgument = "invalid_argument"
	// ErrCodeProjectNotFound is an unknown/missing project.
	ErrCodeProjectNotFound = "project_not_found"
	// ErrCodeReadOnly is an attempt to run a provisioning tool in read-only mode.
	ErrCodeReadOnly = "read_only"
	// ErrCodeQueryFailed is a datalog/scan/pathfinding/computation failure.
	ErrCodeQueryFailed = "query_failed"
	// ErrCodeUnavailable is a tool disabled because a dependency (e.g. AI) is absent.
	ErrCodeUnavailable = "unavailable"
	// ErrCodeInternal is an unexpected server error.
	ErrCodeInternal = "internal"
)

// errEnvelope is the canonical error body returned to MCP clients.
type errEnvelope struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Details string `json:"details,omitempty"`
}

// toolError builds a JSON error envelope as an mcp CallToolResult.
func toolError(code, msg string, details ...any) *mcp.CallToolResult {
	d := ""
	if len(details) > 0 {
		d = fmt.Sprint(details...)
	}
	b, err := json.Marshal(errEnvelope{Error: msg, Code: code, Details: d})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf(`{"error":%q,"code":%q}`, msg, code))
	}
	return mcp.NewToolResultError(string(b))
}

// classifyError maps a human-readable error message to a stable error code so
// that arbitrary messages produced by errorResult still carry a meaningful code.
func classifyError(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "project not found"),
		strings.Contains(lower, "missing project"),
		strings.Contains(lower, "no such project"):
		return ErrCodeProjectNotFound
	case strings.Contains(lower, "read-only"), strings.Contains(lower, "read only"):
		return ErrCodeReadOnly
	case strings.Contains(lower, "unavailable"), strings.Contains(lower, "not initialized"):
		return ErrCodeUnavailable
	case strings.Contains(lower, "required"), strings.Contains(lower, "must be"),
		strings.Contains(lower, "argument"), strings.Contains(lower, "does not exist"),
		strings.Contains(lower, "not a directory"), strings.Contains(lower, "invalid"):
		return ErrCodeInvalidArgument
	case strings.Contains(lower, "failed"), strings.Contains(lower, "failed to"),
		strings.Contains(lower, "query failed"), strings.Contains(lower, "hydration failed"),
		strings.Contains(lower, "pathfinding failed"), strings.Contains(lower, "no embeddings"):
		return ErrCodeQueryFailed
	default:
		return ErrCodeInternal
	}
}
