package common

import (
	"fmt"
	"strings"
)

func ExtractBaseName(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func QuotePath(path string) string {
	return fmt.Sprintf("\"%s\"", path)
}

func MakeLinkKey(source, target string) string {
	return fmt.Sprintf("%s->%s", source, target)
}

func ExtractSymbolName(symbolID string) string {
	parts := strings.SplitN(symbolID, ":", 2)
	if len(parts) < 2 {
		return symbolID
	}
	name := parts[1]
	if idx := strings.LastIndex(name, "."); idx != -1 && idx < len(name)-1 {
		name = name[idx+1:]
	}
	return name
}
