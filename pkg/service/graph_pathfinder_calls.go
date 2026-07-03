package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/export"
	"github.com/duynguyendang/gca/pkg/logger"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
)

// GetFileCalls returns a recursive file-to-file call graph starting from a specific file.
func (s *GraphService) GetFileCalls(ctx context.Context, projectID, fileID string, depth int) (*export.D3Graph, error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Warn("GetFileCalls recovered from panic", "error", r)
		}
	}()

	logger.Debug("GetFileCalls start", "projectID", projectID, "fileID", fileID, "depth", depth)
	if depth <= 0 {
		depth = config.DefaultFileDepthLimit
	}
	if depth > config.MaxFileDepthLimit {
		depth = config.MaxFileDepthLimit
	}

	cacheKey := fmt.Sprintf("file_calls:%s:%d", fileID, depth)
	s.cacheMu.RLock()
	if cached, ok := s.projectMapCache[cacheKey]; ok {
		s.cacheMu.RUnlock()
		return cached, nil
	}
	s.cacheMu.RUnlock()

	store, err := s.getStore(projectID)
	if err != nil {
		logger.Error("GetFileCalls getStore error", "error", err)
		return nil, err
	}
	if store == nil {
		logger.Error("GetFileCalls store is nil", "projectID", projectID)
		return nil, fmt.Errorf("store is nil for project: %s", projectID)
	}

	cleanFileID := strings.Trim(fileID, "\"")

	// Try to find the actual stored file ID (may or may not have project prefix)
	storedFileID := cleanFileID
	if projectID != "" && strings.HasPrefix(cleanFileID, projectID+"/") {
		// File ID has project prefix, try to find if it's stored without prefix
		withoutPrefix := strings.TrimPrefix(cleanFileID, projectID+"/")
		if _, err := store.GetContentByKey(withoutPrefix); err == nil {
			storedFileID = withoutPrefix
		}
	} else if projectID != "" {
		// File ID doesn't have project prefix, check if it's stored with prefix
		prefixedFileID := projectID + "/" + cleanFileID
		if _, err := store.GetContentByKey(prefixedFileID); err == nil {
			storedFileID = prefixedFileID
		}
	}

	logger.Debug("GetFileCalls fileID vs storedFileID", "cleanFileID", cleanFileID, "storedFileID", storedFileID)

	logger.Debug("GetFileCalls IDs", "cleanFileID", cleanFileID, "storedFileID", storedFileID, "projectID", projectID)

	nodesMap := make(map[string]export.D3Node)
	linksMap := make(map[string]export.D3Link)

	type queueItem struct {
		file  string
		depth int
	}
	queue := []queueItem{{file: storedFileID, depth: 0}}
	visited := make(map[string]bool)
	visited[storedFileID] = true

	startFile := storedFileID
	nodesMap[startFile] = export.D3Node{
		ID:   startFile,
		Name: common.ExtractBaseName(startFile),
		Kind: config.SymbolKindFile,
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current.depth >= depth {
			continue
		}

		cleanCurrentFile := current.file

		filesToVisit := make(map[string]bool)
		targetCalls := make(map[string]int)

		// First try to find calls via defines (function calls to other files)
		q := fmt.Sprintf("triples(\"%s\", \"%s\", ?sym), triples(?sym, \"%s\", ?o)", cleanCurrentFile, config.PredicateDefines, config.PredicateCalls)
		results, err := gcamdb.Query(ctx, store, q)
		if err != nil {
			logger.Warn("GetFileCalls calls query error", "error", err)
		}

		if len(results) == 0 {
			// Fall back to imports if no calls found
			q = fmt.Sprintf("triples(\"%s\", \"%s\", ?o)", cleanCurrentFile, config.PredicateImports)
			results, err = gcamdb.Query(ctx, store, q)
			if err != nil {
				logger.Warn("GetFileCalls imports query error", "error", err)
			}
		}

		for _, row := range results {
			targetSymbol, ok := row["?o"].(string)
			if !ok {
				continue
			}

			var targetFile string
			parts := strings.SplitN(targetSymbol, ":", 2)
			if len(parts) >= 2 && isValidFilePath(parts[0]) {
				targetFile = parts[0]
			} else {
				// Use MEB-based O(1) lookup instead of O(N) map scan
				targetFile = findFileForSymbolByStore(ctx, store, targetSymbol)
				if targetFile == "" {
					// For imports, convert package path to project file
					targetFile = findProjectFileForImport(targetSymbol, projectID)
					if targetFile == "" {
						continue
					}
				}
			}

			if targetFile == cleanCurrentFile {
				continue
			}

			targetCalls[targetFile]++
		}

		for targetFile, weight := range targetCalls {
			linkKey := fmt.Sprintf("%s->%s", cleanCurrentFile, targetFile)
			linksMap[linkKey] = export.D3Link{
				Source:   cleanCurrentFile,
				Target:   targetFile,
				Relation: config.RelationCallsFile,
				Weight:   float64(weight),
			}

			if _, exists := nodesMap[targetFile]; !exists {
				nodesMap[targetFile] = export.D3Node{
					ID:   targetFile,
					Name: common.ExtractBaseName(targetFile),
					Kind: config.SymbolKindFile,
				}
			}

			if !visited[targetFile] {
				visited[targetFile] = true
				filesToVisit[targetFile] = true
			}
		}

		for f := range filesToVisit {
			queue = append(queue, queueItem{file: f, depth: current.depth + 1})
		}
	}

	nodes := make([]export.D3Node, 0, len(nodesMap))
	for _, n := range nodesMap {
		nodes = append(nodes, n)
	}
	links := make([]export.D3Link, 0, len(linksMap))
	for _, l := range linksMap {
		links = append(links, l)
	}

	result := &export.D3Graph{Nodes: nodes, Links: links}

	s.cacheMu.Lock()
	s.projectMapCache[cacheKey] = result
	s.cacheMu.Unlock()

	return result, nil
}

// GetFlowPath returns the shortest call graph path between two nodes (files or symbols).
func (s *GraphService) GetFlowPath(ctx context.Context, projectID, fromID, toID string) (*export.D3Graph, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	fromID = strings.Trim(fromID, "\"")
	toID = strings.Trim(toID, "\"")

	maxDepth := config.MaxPathDepth
	type pathNode struct {
		id   string
		path []string
	}

	queue := []pathNode{{id: fromID, path: []string{fromID}}}
	visited := make(map[string]bool)
	visited[fromID] = true

	var foundPath []string

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current.id == toID {
			foundPath = current.path
			break
		}

		if len(current.path) >= maxDepth {
			continue
		}

		cleanCurrentID := strings.Trim(current.id, "\"")
		q := fmt.Sprintf("triples(\"%s\", \"%s\", ?next)", cleanCurrentID, config.PredicateCalls)
		results, err := gcamdb.Query(ctx, store, q)
		if err != nil {
			return nil, err
		}

		for _, r := range results {
			next, ok := r["?next"].(string)
			if !ok {
				continue
			}
			next = strings.Trim(next, "\"")

			if !visited[next] {
				visited[next] = true
				newPath := make([]string, len(current.path))
				copy(newPath, current.path)
				newPath = append(newPath, next)
				queue = append(queue, pathNode{id: next, path: newPath})
			}
		}
	}

	if foundPath == nil {
		return &export.D3Graph{Nodes: []export.D3Node{}, Links: []export.D3Link{}}, nil
	}

	nodes := []export.D3Node{}
	links := []export.D3Link{}
	nodeSet := make(map[string]bool)

	for i := 0; i < len(foundPath); i++ {
		id := foundPath[i]
		if !nodeSet[id] {
			nodes = append(nodes, export.D3Node{
				ID:   id,
				Name: common.ExtractBaseName(id),
				Kind: config.SymbolKindSymbol,
			})
			nodeSet[id] = true
		}

		if i < len(foundPath)-1 {
			links = append(links, export.D3Link{
				Source:   foundPath[i],
				Target:   foundPath[i+1],
				Relation: config.RelationCalls,
				Weight:   1,
			})
		}
	}

	if len(nodes) > 0 {
		if err := s.enrichNodes(ctx, store, &export.D3Graph{Nodes: nodes}, true); err != nil {
			logger.Warn("Failed to enrich nodes", "error", err)
		}
	}

	return &export.D3Graph{Nodes: nodes, Links: links}, nil
}
