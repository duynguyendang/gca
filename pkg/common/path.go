package common

import (
	"fmt"
	"path/filepath"
	"strings"
)

func ExtractBaseName(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func ExtractDir(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx == -1 {
		return ""
	}
	return path[:idx]
}

func QuotePath(path string) string {
	return fmt.Sprintf("\"%s\"", path)
}

func JoinProjectPath(projectName, relPath string) string {
	if projectName == "" {
		return relPath
	}
	return filepath.Join(projectName, relPath)
}

func MakeLinkKey(source, target string) string {
	return fmt.Sprintf("%s->%s", source, target)
}

func MakeTripleLinkKey(source, relation, target string) string {
	return fmt.Sprintf("%s-%s-%s", source, relation, target)
}

func ExtractSymbolFile(symbolID string) string {
	parts := strings.SplitN(symbolID, ":", 2)
	if len(parts) < 2 {
		return ""
	}
	return parts[0]
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
