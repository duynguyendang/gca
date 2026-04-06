package service

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/export"
	gcamdb "github.com/duynguyendang/gca/pkg/meb"
	"github.com/duynguyendang/meb"
)

// GetFileGraph returns a composite graph for a specific file (Defines + Imports + Calls).
func (s *GraphService) GetFileGraph(ctx context.Context, projectID, fileID string, lazy bool) (*export.D3Graph, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	cleanFileID := strings.Trim(fileID, "\"")

	if projectID != "" && !strings.HasPrefix(cleanFileID, projectID+"/") {
		prefixedFileID := projectID + "/" + cleanFileID
		if _, err := store.GetContentByKey(string(prefixedFileID)); err == nil {
			cleanFileID = prefixedFileID
		}
	}

	quotedFileID := fmt.Sprintf("\"%s\"", cleanFileID)

	var mergedGraph *export.D3Graph = &export.D3Graph{
		Nodes: []export.D3Node{},
		Links: []export.D3Link{},
	}

	linkMap := make(map[string]bool)

	merge := func(query string) error {
		results, err := gcamdb.Query(ctx, store, query)
		if err != nil {
			return err
		}
		if len(results) == 0 {
			return nil
		}

		subGraph, err := export.ExportD3(ctx, store, query, results)
		if err != nil {
			return err
		}

		nodeMap := make(map[string]export.D3Node)
		for _, n := range mergedGraph.Nodes {
			nodeMap[n.ID] = n
		}
		for _, n := range subGraph.Nodes {
			if _, exists := nodeMap[n.ID]; !exists {
				nodeMap[n.ID] = n
				mergedGraph.Nodes = append(mergedGraph.Nodes, n)
			}
		}

		for _, l := range subGraph.Links {
			key := fmt.Sprintf("%s-%s-%s", l.Source, l.Relation, l.Target)
			if _, exists := linkMap[key]; !exists {
				linkMap[key] = true
				mergedGraph.Links = append(mergedGraph.Links, l)
			}
		}
		return nil
	}

	q1 := fmt.Sprintf("triples(%s, \"%s\", ?s)", quotedFileID, config.PredicateDefines)
	if err := merge(q1); err != nil {
		return nil, fmt.Errorf("failed to get definitions: %w", err)
	}

	q2 := fmt.Sprintf("triples(%s, \"%s\", ?t)", quotedFileID, config.PredicateImports)
	if err := merge(q2); err != nil {
		return nil, fmt.Errorf("failed to get imports: %w", err)
	}

	if !lazy {
		q3 := fmt.Sprintf("triples(?s, \"%s\", ?t), triples(%s, \"%s\", ?s)", config.PredicateCalls, quotedFileID, config.PredicateDefines)
		if err := merge(q3); err != nil {
			return nil, fmt.Errorf("failed to get calls: %w", err)
		}
	}

	if len(mergedGraph.Nodes) > 0 {
		if err := s.enrichNodes(ctx, store, mergedGraph, lazy); err != nil {
			return nil, fmt.Errorf("hydration failed: %w", err)
		}
	}

	s.resolvePackageImportsToFiles(ctx, store, mergedGraph, cleanFileID)

	s.filterToFilesOnly(mergedGraph)

	return mergedGraph, nil
}

// filterToFilesOnly removes function-level nodes and aggregates links to file level
func (s *GraphService) filterToFilesOnly(graph *export.D3Graph) {
	fileNodes := make(map[string]export.D3Node)
	symbolToFile := make(map[string]string)

	for _, n := range graph.Nodes {
		if !strings.Contains(n.ID, ":") {
			fileNodes[n.ID] = n
		} else {
			parts := strings.SplitN(n.ID, ":", 2)
			filePath := parts[0]
			symbolToFile[n.ID] = filePath

			if _, exists := fileNodes[filePath]; !exists {
				fileName := filePath
				if idx := strings.LastIndex(filePath, "/"); idx != -1 {
					fileName = filePath[idx+1:]
				}
				isInternal := true
				fileNodes[filePath] = export.D3Node{
					ID:         filePath,
					Name:       fileName,
					Kind:       config.SymbolKindFile,
					IsInternal: &isInternal,
				}
			}
		}
	}

	linkSet := make(map[string]bool)
	var newLinks []export.D3Link

	for _, l := range graph.Links {
		sourceFile := l.Source
		targetFile := l.Target

		if sf, ok := symbolToFile[l.Source]; ok {
			sourceFile = sf
		}
		if tf, ok := symbolToFile[l.Target]; ok {
			targetFile = tf
		}

		if sourceFile == targetFile {
			continue
		}

		linkKey := sourceFile + "->" + targetFile
		if !linkSet[linkKey] {
			linkSet[linkKey] = true
			newLinks = append(newLinks, export.D3Link{
				Source:   sourceFile,
				Target:   targetFile,
				Relation: l.Relation,
				Type:     l.Type,
			})
		}
	}

	var newNodes []export.D3Node
	for _, n := range fileNodes {
		newNodes = append(newNodes, n)
	}

	graph.Nodes = newNodes
	graph.Links = newLinks
}

// resolvePackageImportsToFiles expands package import nodes to show actual files
func (s *GraphService) resolvePackageImportsToFiles(ctx context.Context, store *meb.MEBStore, graph *export.D3Graph, sourceFileID string) {
	packagesToResolve := make(map[string]bool)

	for _, n := range graph.Nodes {
		if !strings.Contains(n.ID, ":") && strings.Contains(n.ID, "/") && !strings.Contains(n.ID, ".go") {
			packagesToResolve[n.ID] = true
		}
	}

	if len(packagesToResolve) == 0 {
		return
	}

	for pkgPath := range packagesToResolve {
		files := s.findFilesWithPrefix(store, pkgPath)

		if len(files) == 0 {
			continue
		}

		var newLinks []export.D3Link
		for _, l := range graph.Links {
			if l.Target == pkgPath {
				for _, f := range files {
					newLinks = append(newLinks, export.D3Link{
						Source:   l.Source,
						Target:   f,
						Relation: l.Relation,
						Type:     l.Type,
					})
				}
			} else {
				newLinks = append(newLinks, l)
			}
		}
		graph.Links = newLinks

		var newNodes []export.D3Node
		for _, n := range graph.Nodes {
			if n.ID == pkgPath {
				for _, f := range files {
					fileName := f
					if idx := strings.LastIndex(f, "/"); idx != -1 {
						fileName = f[idx+1:]
					}
					isInternal := true
					newNodes = append(newNodes, export.D3Node{
						ID:         f,
						Name:       fileName,
						Kind:       config.SymbolKindFile,
						IsInternal: &isInternal,
					})
				}
			} else {
				newNodes = append(newNodes, n)
			}
		}
		graph.Nodes = newNodes
	}
}

// findFilesWithPrefix finds all ingested files that match a package path.
func (s *GraphService) findFilesWithPrefix(store *meb.MEBStore, prefix string) []string {
	var files []string
	seen := make(map[string]bool)

	toSlashed := func(p string) string {
		return strings.ReplaceAll(p, ".", "/")
	}

	for fact, _ := range store.Scan("", config.PredicateInPackage, "") {
		filePath := string(fact.Subject)
		pkgName, ok := fact.Object.(string)
		if !ok {
			continue
		}

		matched := false

		if strings.HasPrefix(filePath, prefix) {
			matched = true
		}

		internalPkg := toSlashed(pkgName)

		parts := strings.Split(prefix, "/")
		if len(parts) > 2 {
			suffix := strings.Join(parts[len(parts)-2:], "/")
			if strings.Contains(internalPkg, suffix) {
				matched = true
			}
		} else if len(parts) > 0 {
			suffix := parts[len(parts)-1]
			if strings.HasSuffix(internalPkg, "/"+suffix) || internalPkg == suffix {
				matched = true
			}
		}

		if matched && !seen[filePath] {
			seen[filePath] = true
			files = append(files, filePath)
		}
	}

	if len(files) > config.MaxPackageFilesToResolve {
		files = files[:config.MaxPackageFilesToResolve]
	}

	return files
}

// GetFileCalls returns a recursive file-to-file call graph starting from a specific file.
func (s *GraphService) GetFileCalls(ctx context.Context, projectID, fileID string, depth int) (*export.D3Graph, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("GetFileCalls recovered from panic: %v", r)
		}
	}()

	fmt.Printf("GetFileCalls START: projectID=%s, fileID=%s, depth=%d\n", projectID, fileID, depth)
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
		log.Printf("GetFileCalls: getStore error: %v", err)
		return nil, err
	}
	if store == nil {
		log.Printf("GetFileCalls: store is nil for project %s", projectID)
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

	log.Printf("GetFileCalls: fileID=%s, storedFileID=%s", cleanFileID, storedFileID)

	log.Printf("GetFileCalls: cleanFileID=%s, storedFileID=%s, projectID=%s", cleanFileID, storedFileID, projectID)

	symbolToFile := make(map[string]string)
	// Build symbol-to-file mapping using the fixed query path
	q := fmt.Sprintf("triples(?f, \"%s\", ?sym)", config.PredicateDefines)
	defineResults, err := gcamdb.Query(ctx, store, q)
	if err != nil {
		log.Printf("GetFileCalls: defines query error: %v", err)
	}
	for _, row := range defineResults {
		if f, ok := row["?f"].(string); ok {
			if sym, ok := row["?sym"].(string); ok {
				symbolToFile[sym] = f
			}
		}
	}

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
			log.Printf("GetFileCalls: calls query error: %v", err)
		}

		if len(results) == 0 {
			// Fall back to imports if no calls found
			q = fmt.Sprintf("triples(\"%s\", \"%s\", ?o)", cleanCurrentFile, config.PredicateImports)
			results, err = gcamdb.Query(ctx, store, q)
			if err != nil {
				log.Printf("GetFileCalls: imports query error: %v", err)
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
			} else if defFile, exists := symbolToFile[targetSymbol]; exists {
				targetFile = defFile
			} else {
				// Try to resolve external calls like "api.KeyFromName" or "core.NewError"
				// by searching the symbolToFile map for matching symbols
				targetFile = findFileForSymbol(targetSymbol, symbolToFile)
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
		_ = s.enrichNodes(ctx, store, &export.D3Graph{Nodes: nodes}, true)
	}

	return &export.D3Graph{Nodes: nodes, Links: links}, nil
}

// findFileForSymbol looks up the file that defines a symbol.
// It handles both qualified symbols (e.g., "main.go:main") and unqualified
// symbols (e.g., "fmt.Println" or just "Println") by checking if any
// defined symbol in symbolToFile ends with the target symbol name.
func findFileForSymbol(target string, symbolToFile map[string]string) string {
	// Direct lookup first
	if file, exists := symbolToFile[target]; exists {
		return file
	}

	// Try to find by suffix - e.g., target="fmt.Println" -> look for "*/fmt.Println"
	// or target="Println" -> look for "*/Println"
	for sym, file := range symbolToFile {
		if strings.HasSuffix(sym, ":"+target) || sym == target {
			return file
		}
	}

	// Try stripping package prefix - e.g., "fmt.Println" -> "Println"
	parts := strings.Split(target, ".")
	if len(parts) > 1 {
		lastPart := parts[len(parts)-1]
		for sym, file := range symbolToFile {
			if strings.HasSuffix(sym, ":"+lastPart) {
				return file
			}
		}
	}

	return ""
}

// findProjectFileForImport converts an import path to a project file path.
// For example, "github.com/firebase/genkit/go/core" might map to "genkit-go/core"
// if the project is named "genkit-go".
func findProjectFileForImport(importPath, projectID string) string {
	// If the import starts with the project ID, convert it
	if strings.HasPrefix(importPath, projectID) {
		return strings.TrimPrefix(importPath, projectID+"/")
	}

	// Try common patterns
	// e.g., "github.com/firebase/genkit/go/core" -> "genkit-go/core" for projectID "genkit-go"
	parts := strings.Split(importPath, "/")
	if len(parts) >= 3 {
		// Try to find the project in the import path
		// "github.com/firebase/genkit/go/core" -> look for "genkit" or projectID
		for i, part := range parts {
			if part == projectID || strings.Contains(importPath, projectID) {
				// Reconstruct with just the path after the project
				remaining := strings.Join(parts[i+1:], "/")
				if remaining != "" {
					return remaining
				}
			}
		}
	}

	// Return the import path as-is (might be external)
	return ""
}

// isValidFilePath checks if a string looks like a valid source file path
func isValidFilePath(path string) bool {
	if path == "" {
		return false
	}
	for _, ext := range config.SourceFileExtensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}
