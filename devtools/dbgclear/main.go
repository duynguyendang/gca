package main

import (
	"context"
	"fmt"
	"os"

	"github.com/duynguyendang/gca/internal/manager"
)

func main() {
	ctx := context.Background()
	dir := os.Args[1]
	sm := manager.NewStoreManager(dir, "Low", false)
	sm.SetIndexType("btree")
	sm.SetMebProfile("Live")
	defer sm.CloseAll()

	src, _ := sm.GetSourceStore(".")
	ana, _ := sm.GetAnalyticalStore(".")

	// baseline source facts
	nSrc := 0
	for range src.ScanContext(ctx, "", "calls", "") {
		nSrc++
	}
	nAna := 0
	for range ana.ScanContext(ctx, "", "calls", "") {
		nAna++
	}
	fmt.Printf("baseline calls: source=%d analytical=%d\n", nSrc, nAna)

	// delete ALL subjects seen in analytical store (simulate clearAnalyticalData
	// which scans analyticalStore all-empty, which is topic-agnostic)
	subjects := map[string]bool{}
	for fact := range ana.ScanContext(ctx, "", "", "") {
		subjects[fact.Subject] = true
	}
	fmt.Printf("subjects seen via analytical store all-empty scan: %d\n", len(subjects))
	deleted := 0
	for s := range subjects {
		if err := ana.DeleteFactsBySubject(s); err != nil {
			fmt.Println("delete error:", s, err)
		} else {
			deleted++
		}
	}
	fmt.Printf("deleted %d subjects\n", deleted)

	nSrc2 := 0
	for range src.ScanContext(ctx, "", "calls", "") {
		nSrc2++
	}
	nAna2 := 0
	for range ana.ScanContext(ctx, "", "calls", "") {
		nAna2++
	}
	fmt.Printf("after delete: calls source=%d analytical=%d\n", nSrc2, nAna2)
}