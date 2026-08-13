package main

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/duynguyendang/gca/internal/manager"
	mebpkg "github.com/duynguyendang/gca/pkg/meb"
)

func main() {
	ctx := context.Background()
	dataDir := os.Args[1]
	projectID := os.Args[2]

	sm := manager.NewStoreManager(dataDir, "Low", false)
	sm.SetIndexType("btree")
	sm.SetMebProfile("Live")
	defer sm.CloseAll()

	ana, err := sm.GetAnalyticalStore(projectID)
	if err != nil {
		fmt.Println("analytical store error:", err)
		return
	}
	src, err := sm.GetSourceStore(projectID)
	if err != nil {
		fmt.Println("source store error:", err)
		return
	}

	counts := map[string]int{}
	for fact := range ana.ScanContext(ctx, "", "", "") {
		counts[fact.Predicate]++
	}
	srcCounts := map[string]int{}
	for fact := range src.ScanContext(ctx, "", "", "") {
		srcCounts[fact.Predicate]++
	}

	preds := []string{
		"has_smell_type", "has_smell_category", "has_smell_severity",
		"has_health_score", "has_health_debt", "has_hub_score", "is_entry_point",
		"has_surprise", "has_surprise_score", "has_knowledge_gap", "has_centrality",
		"has_in_degree", "has_out_degree", "belongs_to_cluster",
		"has_import_count", "has_define_count", "has_complexity",
		"okf_age_days", "okf_concept", "okf_link", "bridges_to",
	}
	fmt.Println("=== ANALYTICAL counts ===")
	for _, p := range preds {
		fmt.Printf("  %-22s %d\n", p, counts[p])
	}
	fmt.Println("=== SOURCE counts ===")
	for _, p := range preds {
		fmt.Printf("  %-22s %d\n", p, srcCounts[p])
	}

	// smell breakdown
	smells := map[string]int{}
	for fact := range ana.ScanContext(ctx, "", "has_smell_type", "") {
		if o, ok := fact.Object.(string); ok {
			smells[o]++
		}
	}
	fmt.Println("=== has_smell_type breakdown ===")
	keys := make([]string, 0, len(smells))
	for k := range smells {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %-24s %d\n", k, smells[k])
	}

	// god_file test
	dc := 0
	for range src.ScanContext(ctx, "", "has_define_count", "") {
		dc++
	}
	fmt.Println("has_define_count total:", dc)

	res, qerr := mebpkg.Query(ctx, src, `triples(File, "has_define_count", "excessive")`)
	fmt.Println("query has_define_count==excessive ->", len(res), "qerr:", qerr)
	if len(res) > 0 {
		fmt.Printf("  first result keys: %v\n", res[0])
	}

	dc2 := 0
	for range src.ScanContext(ctx, "", "has_import_count", "") {
		dc2++
	}
	fmt.Println("has_import_count total:", dc2)

	// surprise + gap breakdown
	for _, p := range []string{"has_surprise", "has_knowledge_gap"} {
		m := map[string]int{}
		for fact := range ana.ScanContext(ctx, "", p, "") {
			if o, ok := fact.Object.(string); ok {
				m[o]++
			}
		}
		fmt.Printf("=== %s breakdown ===\n", p)
		for k, v := range m {
			fmt.Printf("  %-28s %d\n", k, v)
		}
	}

	// core source facts
	for _, p := range []string{"calls", "defines", "imports", "has_kind", "in_file"} {
		n := 0
		for range src.ScanContext(ctx, "", p, "") {
			n++
		}
		fmt.Printf("CORE-SOURCE %-12s %d\n", p, n)
	}
}
