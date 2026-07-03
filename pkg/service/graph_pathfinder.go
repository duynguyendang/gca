package service

import (
	"context"
	"fmt"
	"strings"

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

// extractFileFromID extracts file path from a node ID (format: "file:symbol" or just "file")
func extractFileFromID(id string) string {
	if strings.Contains(id, ":") {
		return strings.SplitN(id, ":", 2)[0]
	}
	return id
}

// filterToFilesOnly removes function-level nodes and aggregates links to file level
func (s *GraphService) filterToFilesOnly(graph *export.D3Graph) {
	fileNodes := make(map[string]export.D3Node)

	for _, n := range graph.Nodes {
		fileID := extractFileFromID(n.ID)
		if _, exists := fileNodes[fileID]; !exists {
			fileName := fileID
			if idx := strings.LastIndex(fileID, "/"); idx != -1 {
				fileName = fileID[idx+1:]
			}
			isInternal := true
			fileNodes[fileID] = export.D3Node{
				ID:         fileID,
				Name:       fileName,
				Kind:       config.SymbolKindFile,
				IsInternal: &isInternal,
			}
		}
	}

	linkSet := make(map[string]bool)
	var newLinks []export.D3Link

	for _, l := range graph.Links {
		sourceFile := extractFileFromID(l.Source)
		targetFile := extractFileFromID(l.Target)

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
