package main

import (
	"context"
	"fmt"
	"os"

	"github.com/duynguyendang/gca/internal/manager"
	mebpkg "github.com/duynguyendang/gca/pkg/meb"
)

func main() {
	ctx := context.Background()
	dir := os.Args[1]
	sm := manager.NewStoreManager(dir, "Low", false)
	sm.SetIndexType("btree")
	sm.SetMebProfile("Live")
	defer sm.CloseAll()

	src, err := sm.GetSourceStore(".")
	if err != nil {
		panic(err)
	}

	// count has_define_count and has_import_count in the SOURCE (Global) store, topic-scoped
	dc, ic := 0, 0
	for range src.ScanInTopicContext(ctx, manager.GlobalTopicID("."), "", "has_define_count", "") {
		dc++
	}
	for range src.ScanInTopicContext(ctx, manager.GlobalTopicID("."), "", "has_import_count", "") {
		ic++
	}
	fmt.Printf("SOURCE(Global) has_define_count=%d has_import_count=%d\n", dc, ic)

	// run the exact god_file template query
	res, err := mebpkg.Query(ctx, src, `triples(File, "has_define_count", "excessive")`)
	fmt.Printf("query has_define_count==excessive -> %d results, err=%v\n", len(res), err)
	for _, r := range res {
		fmt.Printf("  %+v\n", r)
	}
}