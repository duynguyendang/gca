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

	s, _ := sm.GetStore(".")

	g := manager.GlobalTopicID(".")
	w := manager.WindowTopicID(".")

	gc, wc := 0, 0
	for range s.ScanInTopicContext(ctx, g, "", "calls", "") {
		gc++
	}
	for range s.ScanInTopicContext(ctx, w, "", "calls", "") {
		wc++
	}
	fmt.Printf("calls in Global=%d Window=%d\n", gc, wc)

	// same for defines
	gd, wd := 0, 0
	for range s.ScanInTopicContext(ctx, g, "", "defines", "") {
		gd++
	}
	for range s.ScanInTopicContext(ctx, w, "", "defines", "") {
		wd++
	}
	fmt.Printf("defines in Global=%d Window=%d\n", gd, wd)
}