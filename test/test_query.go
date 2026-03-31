package main

import (
	"context"
	"fmt"
	"log"

	"github.com/duynguyendang/gca/pkg/meb"
	mebstore "github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func main() {
	cfg := store.DefaultConfig("data/genkit")
	cfg.ReadOnly = true
	s, err := mebstore.NewMEBStore(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()

	results, err := meb.Query(context.Background(), s, `triples(?s, "type", "file")`)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Total files found: %d\n", len(results))
	if len(results) > 0 {
		fmt.Printf("Example result: %+v\n", results[0])
	}
}
