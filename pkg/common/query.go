package common

import (
	"fmt"
	"strings"
)

func BuildDefinesQuery(fileID string) string {
	quoted := QuotePath(fileID)
	return fmt.Sprintf("triples(%s, \"defines\", ?s)", quoted)
}

func BuildImportsQuery(fileID string) string {
	quoted := QuotePath(fileID)
	return fmt.Sprintf("triples(%s, \"imports\", ?t)", quoted)
}

func BuildCallsQuery(fileID string) string {
	quoted := QuotePath(fileID)
	return fmt.Sprintf("triples(?s, \"calls\", ?t), triples(%s, \"defines\", ?s)", quoted)
}

func BuildAllCallsQuery() string {
	return `triples(?s, "calls", ?o)`
}

func BuildImportsOnlyQuery() string {
	return `triples(?s, "imports", ?o)`
}

func BuildTypeQuery(symbol, typeVal string) string {
	quotedSymbol := QuotePath(symbol)
	return fmt.Sprintf("triples(%s, \"type\", \"%s\")", quotedSymbol, typeVal)
}

func BuildCallsBetweenQuery(sourceFile, targetFile string) string {
	quotedSource := QuotePath(sourceFile)
	quotedTarget := QuotePath(targetFile)
	return fmt.Sprintf("triples(%s, \"defines\", ?s), triples(%s, \"defines\", ?o), triples(?s, \"calls\", ?o)",
		quotedSource, quotedTarget)
}

func BuildFileDefinesQuery(fileID string) string {
	quoted := QuotePath(fileID)
	return fmt.Sprintf(`triples(%s, "defines", ?s)`, quoted)
}

func BuildFileInternalCallsQuery(fileID string) string {
	quoted := QuotePath(fileID)
	return fmt.Sprintf(`triples(?s, "calls", ?o), triples(%s, "defines", ?s), triples(%s, "defines", ?o)`,
		quoted, quoted)
}

func BuildInPackageQuery(pkg string) string {
	quotedPkg := QuotePath(pkg)
	return fmt.Sprintf("triples(?s, \"in_package\", %s)", quotedPkg)
}

func BuildHasKindQuery(kind string) string {
	return fmt.Sprintf(`triples(?s, "has_kind", "%s")`, kind)
}

func BuildHasTagQuery(tag string) string {
	return fmt.Sprintf(`triples(?s, "has_tag", "%s")`, tag)
}

func BuildOutgoingEdgesQuery(nodeID string) string {
	quoted := QuotePath(nodeID)
	return fmt.Sprintf("triples(%s, \"calls\", ?next)", quoted)
}

func BuildIncomingEdgesQuery(nodeID string) string {
	return fmt.Sprintf("triples(?s, \"calls\", %s)", QuotePath(nodeID))
}

func BuildHandledByQuery() string {
	return `triples(?url, "handled_by", ?h)`
}

func ExtractPredicatesFromQuery(query string) []string {
	var predicates []string
	parts := strings.Split(query, "triples(")
	for _, part := range parts[1:] {
		idx := strings.Index(part, ")")
		if idx > 0 {
			args := part[:idx]
			argList := strings.Split(args, ",")
			if len(argList) >= 2 {
				pred := strings.TrimSpace(argList[1])
				pred = strings.Trim(pred, "\"")
				predicates = append(predicates, pred)
			}
		}
	}
	return predicates
}
