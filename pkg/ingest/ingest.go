package ingest

import (
	"fmt"
	"hash/fnv"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/duynguyendang/gca/pkg/meb"
	sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

var symbolTable = make(map[string]string)

// Run executes the ingestion process.
func Run(s *meb.MEBStore, sourceDir string) error {
	parser := sitter.NewParser()
	parser.SetLanguage(sitter.NewLanguage(golang.Language()))

	testDir := sourceDir

	// Pass 1: Collect Symbols
	fmt.Println("Pass 1: Collecting symbols...")
	err := filepath.WalkDir(testDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			if err := collectSymbols(parser, path, testDir); err != nil {
				log.Printf("Error collecting symbols from %s: %v", path, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error in Pass 1: %w", err)
	}
	fmt.Printf("Collected %d symbols\n", len(symbolTable))

	// Pass 2: Process Files
	fmt.Println("Pass 2: Processing files...")
	err = filepath.WalkDir(testDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			fmt.Printf("Processing file: %s\n", path)
			if err := processFile(s, parser, path, testDir); err != nil {
				log.Printf("Error processing file %s: %v", path, err)
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("error walking directory: %w", err)
	}

	return nil
}

func collectSymbols(parser *sitter.Parser, path string, sourceRoot string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	relPath, err := filepath.Rel(sourceRoot, path)
	if err != nil {
		relPath = path
	}

	tree := parser.Parse(content, nil)
	if tree == nil {
		return fmt.Errorf("failed to parse tree")
	}
	root := tree.RootNode()

	// Get package name
	pkgName := ""
	for i := uint(0); i < uint(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Kind() == "package_clause" {
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				pkgName = clean(nameNode.Utf8Text(content))
			}
			break
		}
	}

	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Kind() {
		case "function_declaration":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				funcName := clean(nameNode.Utf8Text(content))
				namespaced := fmt.Sprintf("%s:%s", relPath, funcName)
				symbolTable[funcName] = namespaced
				if pkgName != "" {
					symbolTable[pkgName+"."+funcName] = namespaced
				}
			}
		case "method_declaration":
			nameNode := n.ChildByFieldName("name")
			receiverNode := n.ChildByFieldName("receiver")
			if nameNode != nil && receiverNode != nil {
				methodName := clean(nameNode.Utf8Text(content))
				// Extracting receiver type (simplified)
				var receiverType string
				for i := uint(0); i < uint(receiverNode.ChildCount()); i++ {
					c := receiverNode.Child(i)
					if c.Kind() == "parameter_declaration" {
						tn := c.ChildByFieldName("type")
						if tn != nil {
							receiverType = clean(tn.Utf8Text(content))
							// strip * if pointer
							receiverType = strings.TrimPrefix(receiverType, "*")
						}
					}
				}

				if receiverType != "" {
					namespaced := fmt.Sprintf("%s:%s.%s", relPath, receiverType, methodName)
					symbolTable[receiverType+"."+methodName] = namespaced
					if pkgName != "" {
						symbolTable[pkgName+"."+receiverType+"."+methodName] = namespaced
					}
				}
			}
		case "type_declaration":
			for i := uint(0); i < uint(n.ChildCount()); i++ {
				child := n.Child(i)
				if child.Kind() == "type_spec" {
					nameNode := child.ChildByFieldName("name")
					if nameNode != nil {
						structName := clean(nameNode.Utf8Text(content))
						namespaced := fmt.Sprintf("%s:%s", relPath, structName)
						symbolTable[structName] = namespaced
						if pkgName != "" {
							symbolTable[pkgName+"."+structName] = namespaced
						}
					}
				}
			}
		}

		for i := uint(0); i < uint(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}

	walk(root)
	return nil
}

func processFile(s *meb.MEBStore, parser *sitter.Parser, path string, sourceRoot string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Use relative path for namespacing (e.g. mangle/file.go)
	relPath, err := filepath.Rel(sourceRoot, path)
	if err != nil {
		relPath = path // Fallback
	}

	tree := parser.Parse(content, nil)
	if tree == nil {
		return fmt.Errorf("failed to parse tree")
	}
	root := tree.RootNode()

	var facts []meb.Fact

	fileId := strHash(path)
	s.SetContent(fileId, content)

	// Helper to extract receiver type name (simplistic)
	getReceiverType := func(n *sitter.Node) string {
		// Receiver node is a parameter_list (e.g. "(d Decl)" or "(s *Server)")
		// It contains punctuation "(" and ")" and a "parameter_declaration"
		for i := uint(0); i < uint(n.ChildCount()); i++ {
			child := n.Child(i)
			if child.Kind() == "parameter_declaration" {
				// Found the declaration
				// Try to get 'type' field
				typeNode := child.ChildByFieldName("type")
				if typeNode != nil {
					return clean(typeNode.Utf8Text(content))
				}

				// Fallback: search within parameter_declaration
				for j := uint(0); j < uint(child.ChildCount()); j++ {
					grandChild := child.Child(j)
					if grandChild.Kind() == "type_identifier" || grandChild.Kind() == "pointer_type" {
						return clean(grandChild.Utf8Text(content))
					}
				}
			}
		}
		return ""
	}

	var walk func(n *sitter.Node, currentScope string)
	walk = func(n *sitter.Node, currentScope string) {
		nextScope := currentScope

		switch n.Kind() {
		case "import_declaration":
			for i := uint(0); i < uint(n.ChildCount()); i++ {
				child := n.Child(i)
				if child.Kind() == "import_spec" {
					pathNode := child.ChildByFieldName("path")
					if pathNode != nil {
						// Clean import path
						impPath := clean(pathNode.Utf8Text(content))
						facts = append(facts, meb.Fact{
							Subject:   relPath,
							Predicate: "imports",
							Object:    impPath,
							Graph:     "default",
						})
					}
				} else if child.Kind() == "import_spec_list" {
					for j := uint(0); j < uint(child.ChildCount()); j++ {
						grandChild := child.Child(j)
						if grandChild.Kind() == "import_spec" {
							pathNode := grandChild.ChildByFieldName("path")
							if pathNode != nil {
								impPath := clean(pathNode.Utf8Text(content))
								facts = append(facts, meb.Fact{
									Subject:   relPath,
									Predicate: "imports",
									Object:    impPath,
									Graph:     "default",
								})
							}
						}
					}
				}
			}
		case "type_declaration":
			for i := uint(0); i < uint(n.ChildCount()); i++ {
				child := n.Child(i)
				if child.Kind() == "type_spec" {
					nameNode := child.ChildByFieldName("name")
					typeNode := child.ChildByFieldName("type")

					if nameNode != nil && typeNode != nil {
						typeName := clean(nameNode.Utf8Text(content))
						namespacedType := fmt.Sprintf("%s:%s", relPath, typeName)

						// Check if it's a struct or interface
						if typeNode.Kind() == "struct_type" {
							facts = append(facts, meb.Fact{
								Subject:   namespacedType,
								Predicate: "defines_struct",
								Object:    typeName,
								Graph:     "default",
							})

							// Extract fields
							fieldList := typeNode.ChildByFieldName("fields")
							if fieldList != nil {
								for k := uint(0); k < uint(fieldList.ChildCount()); k++ {
									fieldDecl := fieldList.Child(k)
									if fieldDecl.Kind() == "field_declaration" {
										fieldNameNode := fieldDecl.ChildByFieldName("name")
										fieldTypeNode := fieldDecl.ChildByFieldName("type")
										if fieldNameNode != nil {
											fieldName := clean(fieldNameNode.Utf8Text(content))
											fieldType := "unknown"
											if fieldTypeNode != nil {
												fieldType = clean(fieldTypeNode.Utf8Text(content))
											}
											facts = append(facts, meb.Fact{
												Subject:   namespacedType,
												Predicate: "has_field",
												Object:    fieldName + " " + fieldType, // Store "Name Type" as object or just Name? Let's do Name for now to keep it simple, or "Name" and maybe another fact for type.
												// The plan said has_field(StructName, FieldName, FieldType). Datalog is triples.
												// Let's use Object=FieldName for 'has_field' and MAYBE 'field_type' later.
												// Or construct a composite object string?
												// Let's stick to triple: namespacedType "has_field" fieldName.
												// The type info is lost here unless we add another fact.
												Graph: "default",
											})
										}
									}
								}
							}

						} else if typeNode.Kind() == "interface_type" {
							facts = append(facts, meb.Fact{
								Subject:   namespacedType,
								Predicate: "defines_interface",
								Object:    typeName,
								Graph:     "default",
							})

							// Extract methods
							// Interface methods are direct children of interface_type (method_spec nodes)
							// checking grammar: interface_type -> "interface" "{" (method_spec | type_name)* "}"
							// The children are directly inside interface_type usually, strictly speaking inside the block.
							// Let's iterate children of typeNode.
							for k := uint(0); k < uint(typeNode.ChildCount()); k++ {
								spec := typeNode.Child(k)
								if spec.Kind() == "method_spec" {
									mNameNode := spec.ChildByFieldName("name")
									if mNameNode != nil {
										mName := clean(mNameNode.Utf8Text(content))
										facts = append(facts, meb.Fact{
											Subject:   namespacedType,
											Predicate: "has_method",
											Object:    mName,
											Graph:     "default",
										})
									}
								}
							}
						}

						s.SetContent(strHash(namespacedType), []byte(child.Utf8Text(content)))
					}
				}
			}
		case "function_declaration":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				funcName := clean(nameNode.Utf8Text(content))
				// Namespace the function: path/to/file.go:FuncName
				nextScope = fmt.Sprintf("%s:%s", relPath, funcName)
				s.SetContent(strHash(nextScope), []byte(n.Utf8Text(content)))
			}
		case "method_declaration":
			nameNode := n.ChildByFieldName("name")
			receiverNode := n.ChildByFieldName("receiver")
			if nameNode != nil && receiverNode != nil {
				methodName := clean(nameNode.Utf8Text(content))
				receiverType := getReceiverType(receiverNode)

				// Namespace: path/to/file.go:Receiver.Method
				if receiverType != "" {
					nextScope = fmt.Sprintf("%s:%s.%s", relPath, receiverType, methodName)
				} else {
					nextScope = fmt.Sprintf("%s:.%s", relPath, methodName) // Fallback
				}
				s.SetContent(strHash(nextScope), []byte(n.Utf8Text(content)))
			}

		case "call_expression":
			if currentScope != "" {
				funcNode := n.ChildByFieldName("function")
				if funcNode != nil {
					callee := clean(funcNode.Utf8Text(content))

					// Resolve callee if possible
					if resolved, ok := symbolTable[callee]; ok {
						callee = resolved
					}

					facts = append(facts, meb.Fact{
						Subject:   currentScope,
						Predicate: "calls",
						Object:    callee,
						Graph:     "default",
					})
				}
			}
		}

		for i := uint(0); i < uint(n.ChildCount()); i++ {
			walk(n.Child(i), nextScope)
		}
	}

	walk(root, "")

	if len(facts) > 0 {
		for _, f := range facts {
			if err := s.AddFact(f); err != nil {
				return err
			}
		}
	}
	return nil
}

func clean(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\"", ""))
}

func strHash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}
