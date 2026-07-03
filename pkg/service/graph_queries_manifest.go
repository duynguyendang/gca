package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/duynguyendang/gca/pkg/common/errors"
	"github.com/duynguyendang/gca/pkg/config"
)

// GetManifest returns a compressed project manifest for the AI.
func (s *GraphService) GetManifest(ctx context.Context, projectID string) (map[string]interface{}, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	fileMap := make(map[string]string)
	symbolMap := make(map[string]string)

	for fact, err := range store.Scan("", config.PredicateDefines, "") {
		if err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}

		filePath := string(fact.Subject)
		fullID, ok := fact.Object.(string)
		if !ok {
			continue
		}

		fileMap[filePath] = filePath

		shortName := fullID
		parts := strings.Split(fullID, ":")
		if len(parts) > 1 {
			shortName = parts[len(parts)-1]
		}
		if idx := strings.LastIndex(shortName, "."); idx != -1 && idx < len(shortName)-1 {
			shortName = shortName[idx+1:]
		}

		symbolMap[shortName] = fullID
	}

	return map[string]interface{}{
		"F": fileMap,
		"S": symbolMap,
	}, nil
}

// GetSource returns the content of a specific file/symbol.
func (s *GraphService) GetSource(projectID, docID string) (string, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return "", err
	}

	doc, err := store.GetContentByKey(string(docID))
	if err != nil {
		if projectID != "" && !strings.HasPrefix(docID, projectID+"/") {
			prefixedDocID := projectID + "/" + docID
			doc, err = store.GetContentByKey(string(prefixedDocID))
		}

		if err != nil {
			return "", fmt.Errorf("%w: document not found", errors.ErrNotFound)
		}
	}

	return string(doc), nil
}

// GetSymbol retrieves the full hydrated symbol (content + metadata) for a given ID.
func (s *GraphService) GetSymbol(ctx context.Context, projectID, docID string) (*HydratedSymbol, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	ids := []string{string(docID)}
	hydrated, err := s.Hydrate(ctx, store, projectID, ids)
	if err != nil || len(hydrated) == 0 || hydrated[0].Content == "" {
		if projectID != "" && !strings.HasPrefix(docID, projectID+"/") {
			prefixedDocID := projectID + "/" + docID
			ids = []string{string(prefixedDocID)}
			hydrated, err = s.Hydrate(ctx, store, projectID, ids)
		}

		if err != nil || len(hydrated) == 0 || hydrated[0].Content == "" {
			return nil, fmt.Errorf("%w: symbol not found", errors.ErrNotFound)
		}
	}

	return &hydrated[0], nil
}

// GetPredicates returns known predicates.
func (s *GraphService) GetPredicates(projectID string) ([]map[string]string, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	var results []map[string]string
	for _, p := range store.ListPredicates() {
		results = append(results, map[string]string{
			"name": string(p.Symbol),
		})
	}
	return results, nil
}

// SearchSymbols performs symbol search.
func (s *GraphService) SearchSymbols(projectID, query, predicate string, limit int) ([]string, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = config.DefaultSearchLimit
	}

	var matches []string
	count := 0
	for fact, err := range store.Scan("", config.PredicateDefines, "") {
		if err != nil {
			continue
		}
		if obj, ok := fact.Object.(string); ok {
			if strings.Contains(strings.ToLower(obj), strings.ToLower(query)) {
				matches = append(matches, obj)
				count++
				if count >= limit {
					break
				}
			}
		}
	}
	return matches, nil
}

// ListFiles returns all ingested file paths for a project.
func (s *GraphService) ListFiles(projectID string) ([]string, error) {
	store, err := s.getStore(projectID)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var files []string

	for fact, err := range store.Scan("", config.PredicateType, "") {
		if err != nil {
			continue
		}
		if obj, ok := fact.Object.(string); ok && obj == config.FileTypeFile {
			f := string(fact.Subject)
			if !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
	}
	return files, nil
}
