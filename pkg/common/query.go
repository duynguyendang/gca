package common

import (
	"fmt"
	"strings"
)

// NamedQueries maps query names (matching policies/queries.mg entries) to
// their Datalog template strings. This is the Go-side registry for named
// queries. To add a new query, add it to policies/queries.mg first, then
// add a corresponding entry here.
var NamedQueries = map[string]string{
	"smell_type":       `triples(Subject, "has_smell_type", Type)`,
	"smell_severity":   `triples(Subject, "has_smell_severity", Severity)`,
	"smell":            `triples(Subject, "has_smell", Object)`,
	"hub_score":        `triples(Subject, "has_hub_score", Score)`,
	"entry_point":      `triples(Subject, "is_entry_point", "true")`,
	"centrality":       `triples(Subject, "has_centrality", Score)`,
	"in_degree":        `triples(Subject, "has_in_degree", Degree)`,
	"out_degree":       `triples(Subject, "has_out_degree", Degree)`,
	"cluster":          `triples(Subject, "belongs_to_cluster", Cluster)`,
	"health_debt":      `triples(Subject, "has_health_debt", Debt)`,
	"health_score":     `triples(Subject, "has_health_score", Score)`,
	"surprise":         `triples(Subject, "has_surprise", Type), triples(Subject, "calls", Target)`,
	"surprise_score":   `triples(Subject, "has_surprise_score", ScoreStr)`,
	"in_degree_short":  `triples(S, "has_in_degree", D)`,
	"out_degree_short": `triples(S, "has_out_degree", D)`,
	"cluster_short":    `triples(S, "belongs_to_cluster", C)`,
	"test_symbol":      `triples(S, "is_test_symbol", "true")`,
	"in_file":          `triples(S, "in_file", F)`,
	"all_calls":        `triples(?s, "calls", ?o)`,
	"all_imports":      `triples(?s, "imports", ?o)`,
	"query_template_body": `triples(TemplateID, "query_template", Body)`,
	"defines":          `triples(File, "defines", Symbol)`,
	"imports":          `triples(File, "imports", Target)`,
	"calls_from":       `triples(Node, "calls", Target)`,
	"calls_to":         `triples(Caller, "calls", Node)`,
	"has_kind":         `triples(Subject, "has_kind", Kind)`,
	"has_tag":          `triples(Subject, "has_tag", Tag)`,
	"who_calls":        `triples(Caller, "calls", Symbol)`,
	"what_calls":       `triples(Symbol, "calls", Callee)`,
	"smell_weight":     `triples(Name, "smell_weight", Weight)`,
	"hub_candidates":   `triples(File, "calls", _), not contains(File, ":")`,
	"entry_candidates": `triples(File, "defines", Symbol), or(contains(Symbol, "main"), contains(Symbol, "init"))`,
	"symbol_calls":     `triples(File, "defines", Symbol), triples(Symbol, "calls", Target)`,

	// OKF (Open Knowledge Format) queries — registered at the top of the map literal
	// so they are available before any GetNamedQuery call (GetNamedQuery panics on
	// unknown names — see query.go panic-on-unknown contract).
	"okf_concept":         `triples(Concept, "has_role", "okf_concept")`,
	"okf_concept_title":   `triples(Concept, "okf_title", Title)`,
	"okf_concept_desc":    `triples(Concept, "okf_description", Description)`,
	"okf_concepts":        `triples(Concept, "has_role", "okf_concept")`,
	"okf_concept_links":   `triples(Concept, "okf_link", Target)`,
	"okf_concept_bridges": `triples(Concept, "bridges_to", Symbol)`,
	"bridges_to":          `triples(Concept, "bridges_to", Symbol)`,
}

// GetNamedQuery returns the Datalog string for a named query.
// It panics on unknown names to catch typos at startup.
func GetNamedQuery(name string) string {
	q, ok := NamedQueries[name]
	if !ok {
		panic(fmt.Sprintf("unknown named query: %s (add to policies/queries.mg and pkg/common/NamedQueries)", name))
	}
	return q
}

// EscapeDatalogValue escapes a user-supplied value for safe insertion into
// a Datalog query string. Handles backslashes, double quotes, and newlines.
func EscapeDatalogValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}


