package promptbuilder

import (
	"fmt"
	"strings"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/gca/pkg/config"
	"github.com/duynguyendang/meb"
)

func FormatNodesWithCode(data interface{}, limit int) string {
	return common.FormatNodesWithCode(data, limit)
}

func FormatNodesSimple(data interface{}, limit int) string {
	return common.FormatNodesSimple(data, limit)
}

func FormatPredicatesList(data interface{}) string {
	return common.FormatPredicatesList(data)
}

func FormatNodeList(data interface{}) string {
	return common.FormatNodeList(data)
}

func FormatGraphResults(data interface{}, key string) string {
	m, ok := data.(map[string]interface{})
	if !ok {
		return ""
	}
	list, ok := m[key].([]interface{})
	if !ok {
		return ""
	}
	var sb strings.Builder
	if key == "nodes" {
		for i, item := range list {
			if node, ok := item.(map[string]interface{}); ok {
				name, _ := node["name"].(string)
				kind, _ := node["kind"].(string)
				id, _ := node["id"].(string)
				sb.WriteString(fmt.Sprintf("%d. **%s** (Type: %s)\n   ID: `%s`\n", i+1, name, kind, id))
			}
		}
	} else if key == "links" {
		for i, item := range list {
			if link, ok := item.(map[string]interface{}); ok {
				source, _ := link["source"].(string)
				target, _ := link["target"].(string)
				relation, _ := link["relation"].(string)
				if relation == "" {
					relation = config.PredicateCalls
				}
				sb.WriteString(fmt.Sprintf("%d. `%s` **%s** `%s`\n", i+1, source, relation, target))
			}
		}
	}
	return sb.String()
}

func ExtractNodeNames(data interface{}) string {
	return common.ExtractNodeNames(data)
}

func ExtractStringList(data interface{}, limit int) string {
	return common.ExtractStringList(data, limit)
}

func ExtractPathString(data interface{}) string {
	return common.ExtractPathString(data)
}

func AppendSymbolContext(ctx interface{}, store *meb.MEBStore, symbolID string, sb *strings.Builder) error {
	contentBytes, err := store.GetContentByKey(symbolID)
	if err != nil {
		return err
	}
	content := string(contentBytes)

	sb.WriteString(fmt.Sprintf("\n### Symbol: %s\n", symbolID))
	sb.WriteString("```go\n")
	sb.WriteString(common.SymbolContext(content))
	sb.WriteString("\n```\n")

	return nil
}