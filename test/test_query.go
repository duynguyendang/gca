package main

import (
    "context"
    "fmt"
    "log"

    "github.com/duynguyendang/meb"
    "github.com/duynguyendang/meb/store"
)

func main() {
    cfg := store.DefaultConfig("data/genkit")
    cfg.ReadOnly = true
    s, err := meb.NewMEBStore(cfg)
    if err != nil {
        log.Fatal(err)
    }
    defer s.Close()

    results, err := s.Query(context.Background(), `triples(?s, "type", "file")`)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Total files found: %d\n", len(results))
    if len(results) > 0 {
         fmt.Printf("Example result: %+v\n", results[0])
    }
}
