package main

import (
	"fmt"

	sitter "github.com/tree-sitter/go-tree-sitter"
	python "github.com/tree-sitter/tree-sitter-python/bindings/go"
)

func main() {
	code := []byte(`
APP_NAME = "Mangle"
version = 1
`)
	lang := sitter.NewLanguage(python.Language())
	parser := sitter.NewParser()
	parser.SetLanguage(lang)

	tree := parser.Parse(code, nil)
	root := tree.RootNode()

	var walk func(n *sitter.Node, depth int)
	walk = func(n *sitter.Node, depth int) {
		indent := ""
		for i := 0; i < depth; i++ {
			indent += "  "
		}
		fmt.Printf("%s%s (Field: %s)\n", indent, n.Kind(), n.FieldNameForChild(0)) // simplistic field name check

		for i := uint(0); i < uint(n.ChildCount()); i++ {
			walk(n.Child(i), depth+1)
		}
	}
	walk(root, 0)
}
