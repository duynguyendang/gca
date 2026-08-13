package main

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/duynguyendang/gca/internal/manager"
	"github.com/duynguyendang/gca/pkg/registry"
)

func main() {
	ctx := context.Background()
	sm := manager.NewStoreManager("/tmp/opencode/e2e-data", "Low", false)
	sm.SetIndexType("btree")
	sm.SetMebProfile("Live")
	defer sm.CloseAll()

	ts := registry.NewTemplateStore(sm)
	if err := ts.LoadPolicyFiles(ctx, "policies/init.mg"); err != nil {
		fmt.Println("load error:", err)
		os.Exit(1)
	}

	tmpls, err := ts.ListTemplates(ctx, ".", "")
	if err != nil {
		fmt.Println("list error:", err)
		os.Exit(1)
	}

	fmt.Println("=== template count:", len(tmpls))
	ids := make([]string, 0, len(tmpls))
	for _, t := range tmpls {
		ids = append(ids, t.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		for _, t := range tmpls {
			if t.ID == id {
				fmt.Printf("  %-28s smell=%q pred=%q body=%q\n", id, t.SmellType, t.Predicate, t.Body)
			}
		}
	}
}
