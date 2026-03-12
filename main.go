package main

import (
	"fmt"
	"os"

	"github.com/duynguyendang/gca/cmd"
)

func main() {
	// Execute the Cobra command tree
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
