package service

import (
	"context"
	"strconv"
	"strings"

	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/gca/pkg/export"
	"github.com/duynguyendang/meb"
)

func (s *GraphService) HydrateShallow(ctx context.Context, store *meb.MEBStore, ids []string) ([]HydratedSymbol, error) {
	hydrated := make([]HydratedSymbol, 0, len(ids))

	for _, id := range ids {
		hs := HydratedSymbol{ID: id, Metadata: make(map[string]interface{})}

		for fact, _ := range store.ScanContext(ctx, id, config.PredicateHasKind, "") {
			if str, ok := fact.Object.(string); ok {
				hs.Kind = str
				break
			}
		}
		for fact, _ := range store.ScanContext(ctx, id, config.PredicateHasLanguage, "") {
			if str, ok := fact.Object.(string); ok {
				hs.Metadata["language"] = str
				break
			}
		}
		for fact, _ := range store.ScanContext(ctx, id, config.PredicateStartLine, "") {
			if num, ok := fact.Object.(int); ok {
				hs.Metadata["start_line"] = num
			} else if floatNum, ok := fact.Object.(float64); ok {
				hs.Metadata["start_line"] = int(floatNum)
			} else if strNum, ok := fact.Object.(string); ok {
				if parsed, err := strconv.Atoi(strNum); err == nil {
					hs.Metadata["start_line"] = parsed
				}
			}
		}
		for fact, _ := range store.ScanContext(ctx, id, config.PredicateEndLine, "") {
			if num, ok := fact.Object.(int); ok {
				hs.Metadata["end_line"] = num
			} else if floatNum, ok := fact.Object.(float64); ok {
				hs.Metadata["end_line"] = int(floatNum)
			} else if strNum, ok := fact.Object.(string); ok {
				if parsed, err := strconv.Atoi(strNum); err == nil {
					hs.Metadata["end_line"] = parsed
				}
			}
		}

		hydrated = append(hydrated, hs)
	}
	return hydrated, nil
}

func (s *GraphService) HydrateShallowBatch(ctx context.Context, store *meb.MEBStore, ids []string) ([]HydratedSymbol, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	hydrated := make([]HydratedSymbol, len(ids))
	idToIdx := make(map[string]int, len(ids))
	for i, id := range ids {
		hydrated[i] = HydratedSymbol{ID: id, Metadata: make(map[string]interface{})}
		idToIdx[id] = i
	}

	metadataPredicates := []string{
		config.PredicateHasKind,
		config.PredicateHasLanguage,
		config.PredicateStartLine,
		config.PredicateEndLine,
	}

	for _, pred := range metadataPredicates {
		for _, id := range ids {
			for fact, err := range store.ScanContext(ctx, id, pred, "") {
				if err != nil {
					continue
				}
				idx, ok := idToIdx[id]
				if !ok {
					continue
				}
				hs := &hydrated[idx]

				switch pred {
				case config.PredicateHasKind:
					if str, ok := fact.Object.(string); ok {
						hs.Kind = str
					}
				case config.PredicateHasLanguage:
					if str, ok := fact.Object.(string); ok {
						hs.Metadata["language"] = str
					}
				case config.PredicateStartLine:
					if num, ok := fact.Object.(int); ok {
						hs.Metadata["start_line"] = num
					} else if floatNum, ok := fact.Object.(float64); ok {
						hs.Metadata["start_line"] = int(floatNum)
					} else if strNum, ok := fact.Object.(string); ok {
						if parsed, err := strconv.Atoi(strNum); err == nil {
							hs.Metadata["start_line"] = parsed
						}
					}
				case config.PredicateEndLine:
					if num, ok := fact.Object.(int); ok {
						hs.Metadata["end_line"] = num
					} else if floatNum, ok := fact.Object.(float64); ok {
						hs.Metadata["end_line"] = int(floatNum)
					} else if strNum, ok := fact.Object.(string); ok {
						if parsed, err := strconv.Atoi(strNum); err == nil {
							hs.Metadata["end_line"] = parsed
						}
					}
				}
				break
			}
		}
	}

	return hydrated, nil
}

func (s *GraphService) Hydrate(ctx context.Context, store *meb.MEBStore, projectID string, ids []string) ([]HydratedSymbol, error) {
	hydrated, err := s.HydrateShallow(ctx, store, ids)
	if err != nil {
		return nil, err
	}
	for i := range hydrated {
		hs := &hydrated[i]

		content, _ := store.GetContentByKey(hs.ID)
		if len(content) == 0 {
			content, _ = store.GetContentByKey("/" + hs.ID)
		}
		if len(content) == 0 && projectID != "" && !strings.HasPrefix(hs.ID, projectID+"/") {
			prefixedID := projectID + "/" + hs.ID
			content, _ = store.GetContentByKey(prefixedID)
		}
		if len(content) > 0 {
			hs.Content = string(content)
			continue
		}

		if strings.Contains(hs.ID, ":") {
			parts := strings.Split(hs.ID, ":")
			filePath := parts[0]
			fileContentBytes, _ := store.GetContentByKey(filePath)
			if len(fileContentBytes) == 0 && projectID != "" && !strings.HasPrefix(filePath, projectID+"/") {
				prefixedPath := projectID + "/" + filePath
				fileContentBytes, _ = store.GetContentByKey(prefixedPath)
			}
			if len(fileContentBytes) > 0 {
				startLine, hasStart := hs.Metadata["start_line"].(int)
				endLine, hasEnd := hs.Metadata["end_line"].(int)

				if hasStart && hasEnd {
					lines := strings.Split(string(fileContentBytes), "\n")
					start := startLine - 1
					end := endLine

					if start < 0 {
						start = 0
					}
					if end > len(lines) {
						end = len(lines)
					}
					if start < end {
						hs.Content = strings.Join(lines[start:end], "\n")
					}
				} else {
					hs.Content = string(fileContentBytes)
				}
			}
		}
	}
	return hydrated, nil
}

func (s *GraphService) enrichNodes(ctx context.Context, store *meb.MEBStore, graph *export.D3Graph, lazy bool) error {
	ids := make([]string, len(graph.Nodes))
	for i, n := range graph.Nodes {
		ids[i] = string(n.ID)
	}

	var hydrated []HydratedSymbol
	var err error

	if lazy {
		hydrated, err = s.HydrateShallowBatch(ctx, store, ids)
	} else {
		hydrated, err = s.Hydrate(ctx, store, "", ids)
	}

	if err != nil {
		return err
	}

	hMap := make(map[string]HydratedSymbol)
	for _, h := range hydrated {
		hMap[h.ID] = h
	}

	for i := range graph.Nodes {
		n := &graph.Nodes[i]
		if h, ok := hMap[string(n.ID)]; ok {
			n.Code = h.Content
			if h.Kind != "" {
				n.Kind = h.Kind
			}
			if len(h.Children) > 0 {
				n.Children = s.mapChildren(h.Children)
			}

			if n.Metadata == nil {
				n.Metadata = make(map[string]string)
			}
			if pkg, ok := h.Metadata["package"].(string); ok {
				n.Metadata["package"] = pkg
			}
			if tags, ok := h.Metadata["tags"].([]string); ok {
				n.Metadata["tags"] = strings.Join(tags, ",")
			} else if tags, ok := h.Metadata["tags"].([]interface{}); ok {
				var strTags []string
				for _, t := range tags {
					if s, ok := t.(string); ok {
						strTags = append(strTags, s)
					}
				}
				n.Metadata["tags"] = strings.Join(strTags, ",")
			} else if tags, ok := h.Metadata["tags"].(string); ok {
				n.Metadata["tags"] = tags
			}
		}
	}
	return nil
}

func (s *GraphService) mapChildren(hydrated []HydratedSymbol) []export.D3Node {
	if len(hydrated) == 0 {
		return nil
	}
	nodes := make([]export.D3Node, len(hydrated))
	for i, h := range hydrated {
		parts := strings.Split(string(h.ID), "/")
		name := parts[len(parts)-1]

		nodes[i] = export.D3Node{
			ID:       string(h.ID),
			Name:     name,
			Kind:     h.Kind,
			Code:     h.Content,
			Children: s.mapChildren(h.Children),
		}

		if lang, ok := h.Metadata["language"].(string); ok {
			nodes[i].Language = lang
			nodes[i].Group = lang
		}

		if nodes[i].Metadata == nil {
			nodes[i].Metadata = make(map[string]string)
		}
		if pkg, ok := h.Metadata["package"].(string); ok {
			nodes[i].Metadata["package"] = pkg
		}
		if tags, ok := h.Metadata["tags"].([]string); ok {
			nodes[i].Metadata["tags"] = strings.Join(tags, ",")
		} else if tags, ok := h.Metadata["tags"].([]interface{}); ok {
			var strTags []string
			for _, t := range tags {
				if s, ok := t.(string); ok {
					strTags = append(strTags, s)
				}
			}
			nodes[i].Metadata["tags"] = strings.Join(strTags, ",")
		} else if tags, ok := h.Metadata["tags"].(string); ok {
			nodes[i].Metadata["tags"] = tags
		}
	}
	return nodes
}
