package ooda

import (
	"context"
	"fmt"
	"strings"

	"github.com/duynguyendang/meb"
)

type DatalogSchema struct {
	Predicates    string
	FactExamples  string
	ConstraintDoc string
}

func predicateDescription(pred string) string {
	descriptions := map[string]string{
		"defines":        "Subject defines (contains) the Object symbol, type, or interface",
		"calls":           "Subject calls the Object function or method",
		"handled_by":     "URL path (Subject) is handled by the Object handler function",
		"has_role":       "Subject has the role Object (e.g., 'api_handler', 'middleware')",
		"has_kind":       "Subject has kind Object (e.g., 'func', 'struct', 'interface')",
		"has_doc":        "Subject has documentation string Object",
		"has_name":       "Subject has display name Object",
		"has_tag":        "Subject has tag Object (e.g., 'test', 'mock', 'generated')",
		"in_package":     "Subject belongs to package Object",
		"imports":        "Subject file imports the Object package or file",
		"type":            "Subject has type Object",
		"parameter":      "Subject function has parameter Object",
		"returns":       "Subject function returns type Object",
		"belongs_to_cluster": "Subject belongs to cluster Object",
		"is_entry_point":  "Subject is an entry point (true)",
		"has_hub_score":  "Subject has hub score Object",
		"has_smell":      "Subject has architectural smell Object",
	}
	if desc, ok := descriptions[pred]; ok {
		return desc
	}
	return "Relationship: " + pred
}

func BuildDatalogSchemaContext(ctx context.Context, store *meb.MEBStore) (*DatalogSchema, error) {
	if store == nil {
		return &DatalogSchema{
			Predicates:    "No store available",
			FactExamples:  "",
			ConstraintDoc: defaultConstraintDoc(),
		}, nil
	}

	var predLines, exampleLines []string
	predicates := store.ListPredicates()
	for _, p := range predicates {
		pred := string(p.Symbol)
		desc := predicateDescription(pred)
		predLines = append(predLines, fmt.Sprintf("- %s: %s", pred, desc))
	}

	if len(predicates) == 0 {
		predLines = append(predLines, "(no predicates found in store)")
	}

	count := 0
	for fact := range store.Scan("", "", "") {
		if count >= 20 {
			break
		}
		subj := truncate(fact.Subject, 40)
		pred := fact.Predicate
		obj := truncate(fmt.Sprintf("%v", fact.Object), 40)
		exampleLines = append(exampleLines, fmt.Sprintf(`triples("%s", "%s", "%s")`, subj, pred, obj))
		count++
	}

	if len(exampleLines) == 0 {
		exampleLines = append(exampleLines, "(no facts available)")
	}

	return &DatalogSchema{
		Predicates:    strings.Join(predLines, "\n"),
		FactExamples:  strings.Join(exampleLines, "\n"),
		ConstraintDoc: defaultConstraintDoc(),
	}, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func defaultConstraintDoc() string {
	return `## Constraint Reference
- regex(Variable, "pattern") — match variable against regex pattern
- Comparison: =, !=, <, >, <=, >=  (e.g., ?x < 100)
- Logical: comma-separated atoms imply AND
- Rule: p(A,B) :- q(A,C), r(C,B)  (head depends on body)

## Output Format
- Raw Datalog only. No markdown. No explanation.
- Datalog string: triples(?S, "predicate", "object")
- JSON tool call: {"tool": "find_connection", "source_id": "X", "target_id": "Y"}`
}