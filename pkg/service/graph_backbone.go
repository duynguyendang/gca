package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/duynguyendang/gca/pkg/export"
)

// GetFileBackbone returns the bidirectional file-level dependency graph (depth 1) for a specific file.
// It finds files that call this file (upstream) and files that this file calls (downstream).
func (s *GraphService) GetFileBackbone(ctx context.Context, projectID, fileID string) (*export.D3Graph, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	cleanFileID := strings.Trim(fileID, "\"")

	// Try to resolve the file ID - it might need the project prefix
	// If fileID doesn't start with the projectID, try with the prefix
	if projectID != "" && !strings.HasPrefix(cleanFileID, projectID+"/") {
		prefixedFileID := projectID + "/" + cleanFileID
		// Check if the prefixed version exists in the store
		if _, err := store.GetContentByKey(string(prefixedFileID)); err == nil {
			cleanFileID = prefixedFileID
		}
	}

	quotedFileID := fmt.Sprintf("\"%s\"", cleanFileID)

	nodesMap := make(map[string]export.D3Node)
	linksMap := make(map[string]export.D3Link)

	// Add Center Node
	nodesMap[cleanFileID] = export.D3Node{ID: cleanFileID, Name: getNodeName(cleanFileID), Kind: "file", Group: "center"}

	// 1. Downstream: File -> Calls -> ?
	// Query: defined symbols in File -> calls -> ?target
	qDown := fmt.Sprintf("triples(%s, \"defines\", ?s), triples(?s, \"calls\", ?o)", quotedFileID)
	resDown, err := store.Query(ctx, qDown)
	if err != nil {
		return nil, fmt.Errorf("query downstream failed: %w", err)
	}

	for _, row := range resDown {
		targetAuth, ok := row["?o"].(string)
		if !ok {
			continue
		}
		// Extract file from target ID (convention: path/to/file.go:Symbol)
		parts := strings.SplitN(targetAuth, ":", 2)
		if len(parts) < 2 {
			continue
		}
		targetFile := parts[0]
		if targetFile == cleanFileID {
			continue // ignore self-calls
		}

		// Add Node
		if _, exists := nodesMap[targetFile]; !exists {
			nodesMap[targetFile] = export.D3Node{ID: targetFile, Name: getNodeName(targetFile), Kind: "file", Group: "downstream"}
		}
		// Add Link
		linkKey := fmt.Sprintf("%s->%s", cleanFileID, targetFile)
		if _, exists := linksMap[linkKey]; !exists {
			linksMap[linkKey] = export.D3Link{Source: cleanFileID, Target: targetFile, Relation: "calls", Weight: 1}
		} else {
			l := linksMap[linkKey]
			l.Weight++
			linksMap[linkKey] = l
		}
	}

	// 2. Upstream: ? -> Calls -> File
	// Query: defined symbols in File (targets) <- called by ?caller
	qUp := fmt.Sprintf("triples(%s, \"defines\", ?target), triples(?caller, \"calls\", ?target)", quotedFileID)
	resUp, err := store.Query(ctx, qUp)
	if err != nil {
		return nil, fmt.Errorf("query upstream failed: %w", err)
	}

	for _, row := range resUp {
		callerAuth, ok := row["?caller"].(string)
		if !ok {
			continue
		}
		// Extract file from caller ID
		parts := strings.SplitN(callerAuth, ":", 2)
		if len(parts) < 2 {
			continue
		}
		callerFile := parts[0]
		if callerFile == cleanFileID {
			continue // ignore self-calls
		}

		// Add Node
		if _, exists := nodesMap[callerFile]; !exists {
			nodesMap[callerFile] = export.D3Node{ID: callerFile, Name: getNodeName(callerFile), Kind: "file", Group: "upstream"}
		}
		// Add Link
		linkKey := fmt.Sprintf("%s->%s", callerFile, cleanFileID)
		if _, exists := linksMap[linkKey]; !exists {
			linksMap[linkKey] = export.D3Link{Source: callerFile, Target: cleanFileID, Relation: "calls", Weight: 1}
		} else {
			l := linksMap[linkKey]
			l.Weight++
			linksMap[linkKey] = l
		}
	}

	// Convert to Slices
	nodes := make([]export.D3Node, 0, len(nodesMap))
	for _, n := range nodesMap {
		nodes = append(nodes, n)
	}
	links := make([]export.D3Link, 0, len(linksMap))
	for _, l := range linksMap {
		links = append(links, l)
	}

	return &export.D3Graph{Nodes: nodes, Links: links}, nil
}
