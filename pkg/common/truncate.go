package common

import "fmt"

const (
	MaxSymbolContextLength = 2000
	MaxContentPreviewLength = 500
	MaxDocPreviewLength     = 200
	MaxExportLabelLength    = 200
	MaxCodePreviewLength   = 2000
	MaxExecutorCacheCleanup = 500
	ExecutorQueryTimeoutMs = 500
	MaxJoinResults         = 5000
)

func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func TruncateWithEllipsis(s string, maxLen int, suffix string) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + suffix
}

func SymbolContext(s string) string {
	return s
}

func ContentPreview(s string) string {
	return TruncateWithEllipsis(s, MaxContentPreviewLength, "...")
}

func DocPreview(s string) string {
	return TruncateWithEllipsis(s, MaxDocPreviewLength, "...")
}

func ExportLabel(s string) string {
	return TruncateWithEllipsis(s, MaxExportLabelLength, "...")
}

func CodePreview(s string) string {
	return TruncateWithEllipsis(s, MaxCodePreviewLength, "\n... (truncated)")
}

func FormatTruncation(len int) string {
	return fmt.Sprintf("(truncated, original length: %d)", len)
}
