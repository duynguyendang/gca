package ingest

import (
	"fmt"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

// SymbolType constants
const (
	TypeFunction  = "function"
	TypeMethod    = "method"
	TypeStruct    = "struct"
	TypeInterface = "interface"
)

// Reference represents a dependency or call.
type Reference struct {
	Subject   string
	Predicate string
	Object    string
	Line      int
}

// Symbol represents a code entity extracted from AST.
type Symbol struct {
	ID         string
	Name       string
	Type       string
	Receiver   string // For methods
	Signature  string // Code signature (e.g. func Foo(a int) error)
	DocComment string // Preceding doc comment
	Content    string // Full source code
	StartLine  int
	EndLine    int
	Package    string
}

// lineFromOffset calculates line number from byte offset.
func lineFromOffset(content []byte, offset uint) int {
	if int(offset) >= len(content) {
		return 0
	}
	// Basic implementation
	return strings.Count(string(content[:offset]), "\n") + 1
}

// Extractor handles AST parsing and symbol extraction.
type Extractor struct {
	parser *sitter.Parser
}

// NewExtractor creates a new extractor instance.
func NewExtractor() *Extractor {
	parser := sitter.NewParser()
	parser.SetLanguage(sitter.NewLanguage(golang.Language()))
	return &Extractor{parser: parser}
}

// ExtractSymbols parses the content and returns a list of symbols.
func (e *Extractor) ExtractSymbols(filename string, content []byte, relPath string) ([]Symbol, error) {
	tree := e.parser.Parse(content, nil)
	if tree == nil {
		return nil, fmt.Errorf("failed to parse tree")
	}
	root := tree.RootNode()
	if root == nil {
		return nil, fmt.Errorf("empty root node")
	}

	var symbols []Symbol

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

	// Walk the tree
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		switch n.Kind() {
		case "function_declaration":
			sym := e.extractFunction(n, content, relPath, pkgName)
			symbols = append(symbols, sym)

		case "method_declaration":
			sym := e.extractMethod(n, content, relPath, pkgName)
			if sym.Name != "" {
				symbols = append(symbols, sym)
			}

		case "type_declaration":
			// A type declaration can contain multiple type specs
			// e.g. type ( ... ) or type A struct ...
			for i := uint(0); i < uint(n.ChildCount()); i++ {
				child := n.Child(i)
				if child.Kind() == "type_spec" {
					sym := e.extractType(child, n, content, relPath, pkgName)
					if sym.Name != "" {
						symbols = append(symbols, sym)
					}
				}
			}
		}

		// Recurse
		for i := uint(0); i < uint(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}

	walk(root)
	return symbols, nil
}

// ExtractReferences parses the content and returns a list of references (calls, imports, etc).
func (e *Extractor) ExtractReferences(filename string, content []byte, relPath string) ([]Reference, error) {
	tree := e.parser.Parse(content, nil)
	if tree == nil {
		return nil, fmt.Errorf("failed to parse tree")
	}
	root := tree.RootNode()

	var refs []Reference

	// Helper to extract receiver type
	getReceiverType := func(n *sitter.Node) string {
		for i := uint(0); i < uint(n.ChildCount()); i++ {
			child := n.Child(i)
			if child.Kind() == "parameter_declaration" {
				typeNode := child.ChildByFieldName("type")
				if typeNode != nil {
					t := clean(typeNode.Utf8Text(content))
					return strings.TrimPrefix(t, "*")
				}
				for j := uint(0); j < uint(child.ChildCount()); j++ {
					gc := child.Child(j)
					if gc.Kind() == "type_identifier" || gc.Kind() == "pointer_type" {
						t := clean(gc.Utf8Text(content))
						return strings.TrimPrefix(t, "*")
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
			// Imports are file-level usually. Subject is file.
			for i := uint(0); i < uint(n.ChildCount()); i++ {
				child := n.Child(i)
				if child.Kind() == "import_spec" {
					pathNode := child.ChildByFieldName("path")
					if pathNode != nil {
						impPath := clean(pathNode.Utf8Text(content))
						refs = append(refs, Reference{
							Subject:   relPath,
							Predicate: "imports",
							Object:    impPath,
							Line:      lineFromOffset(content, child.StartByte()),
						})
					}
				} else if child.Kind() == "import_spec_list" {
					for j := uint(0); j < uint(child.ChildCount()); j++ {
						grandChild := child.Child(j)
						if grandChild.Kind() == "import_spec" {
							pathNode := grandChild.ChildByFieldName("path")
							if pathNode != nil {
								impPath := clean(pathNode.Utf8Text(content))
								refs = append(refs, Reference{
									Subject:   relPath,
									Predicate: "imports",
									Object:    impPath,
									Line:      lineFromOffset(content, grandChild.StartByte()),
								})
							}
						}
					}
				}
			}

		case "function_declaration":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				funcName := clean(nameNode.Utf8Text(content))
				nextScope = fmt.Sprintf("%s:%s", relPath, funcName)
			}

		case "method_declaration":
			nameNode := n.ChildByFieldName("name")
			receiverNode := n.ChildByFieldName("receiver")
			if nameNode != nil && receiverNode != nil {
				methodName := clean(nameNode.Utf8Text(content))
				receiverType := getReceiverType(receiverNode)
				if receiverType != "" {
					nextScope = fmt.Sprintf("%s:%s.%s", relPath, receiverType, methodName)
				} else {
					nextScope = fmt.Sprintf("%s:.%s", relPath, methodName)
				}
			}

		case "call_expression":
			if currentScope != "" {
				funcNode := n.ChildByFieldName("function")
				if funcNode != nil {
					callee := clean(funcNode.Utf8Text(content))
					refs = append(refs, Reference{
						Subject:   currentScope,
						Predicate: "calls",
						Object:    callee,
						Line:      lineFromOffset(content, n.StartByte()),
					})
				}
			}
		}

		for i := uint(0); i < uint(n.ChildCount()); i++ {
			walk(n.Child(i), nextScope)
		}
	}

	walk(root, "")
	return refs, nil
}

func (e *Extractor) extractFunction(n *sitter.Node, content []byte, relPath string, pkgName string) Symbol {
	nameNode := n.ChildByFieldName("name")
	name := ""
	if nameNode != nil {
		name = clean(nameNode.Utf8Text(content))
	}

	// ID: path/to/file.go:FuncName
	id := fmt.Sprintf("%s:%s", relPath, name)

	// Doc Comment
	doc := e.getDocComment(n, content)

	// Signature (approximate: everything up to the block)
	signature := e.getSignature(n, content)

	return Symbol{
		ID:         id,
		Name:       name,
		Type:       TypeFunction,
		Signature:  signature,
		DocComment: doc,
		Content:    n.Utf8Text(content),
		StartLine:  lineFromOffset(content, n.StartByte()),
		EndLine:    lineFromOffset(content, n.EndByte()),
		Package:    pkgName,
	}
}

func (e *Extractor) extractMethod(n *sitter.Node, content []byte, relPath string, pkgName string) Symbol {
	nameNode := n.ChildByFieldName("name")
	name := ""
	if nameNode != nil {
		name = clean(nameNode.Utf8Text(content))
	}

	receiverNode := n.ChildByFieldName("receiver")
	receiverType := ""
	if receiverNode != nil {
		receiverType = e.getReceiverType(receiverNode, content)
	}

	// ID: path/to/file.go:Receiver.Method
	id := fmt.Sprintf("%s:%s.%s", relPath, receiverType, name)

	doc := e.getDocComment(n, content)
	signature := e.getSignature(n, content)

	return Symbol{
		ID:         id,
		Name:       name,
		Type:       TypeMethod,
		Receiver:   receiverType,
		Signature:  signature,
		DocComment: doc,
		Content:    n.Utf8Text(content),
		StartLine:  lineFromOffset(content, n.StartByte()),
		EndLine:    lineFromOffset(content, n.EndByte()),
		Package:    pkgName,
	}
}

func (e *Extractor) extractType(spec *sitter.Node, decl *sitter.Node, content []byte, relPath string, pkgName string) Symbol {
	// decl is the parent (type_declaration), spec is the node (type_spec)
	nameNode := spec.ChildByFieldName("name")
	name := ""
	if nameNode != nil {
		name = clean(nameNode.Utf8Text(content))
	}

	typeNode := spec.ChildByFieldName("type")
	kind := TypeStruct
	if typeNode != nil && typeNode.Kind() == "interface_type" {
		kind = TypeInterface
	}

	id := fmt.Sprintf("%s:%s", relPath, name)

	// Doc comments are usually on the parent type_declaration,
	// UNLESS it's a grouped declaration type ( A B; C D ), in which case it might be tricky.
	// For simplicity, we check the parent `type_declaration` if this is the only spec,
	// or scan locally. Tree-sitter usually attaches comments as preceding siblings.
	// If `decl` contains multiple `type_spec`s, the comment might refer to the group.
	// We'll check `decl` for doc comments.
	doc := e.getDocComment(decl, content)

	return Symbol{
		ID:         id,
		Name:       name,
		Type:       kind,
		Signature:  fmt.Sprintf("type %s %s", name, kind),
		DocComment: doc,
		Content:    spec.Utf8Text(content), // Just the spec, not the whole 'type' block if grouped?
		// Actually for single line `type A struct {...}`, parent `type_declaration` covers it.
		// If grouped, `spec` is just one line. Let's use `spec` content but `decl` doc?
		// If we use `spec` content, we miss the `type` keyword for grouped specs.
		// Let's use `spec` content for now.
		StartLine: lineFromOffset(content, spec.StartByte()),
		EndLine:   lineFromOffset(content, spec.EndByte()),
		Package:   pkgName,
	}
}

// getDocComment looks for comment nodes immediately preceding the given node.
func (e *Extractor) getDocComment(n *sitter.Node, content []byte) string {
	// Look at previous siblings
	var comments []string

	prev := n.PrevSibling()
	for prev != nil {
		if prev.Kind() == "comment" {
			text := prev.Utf8Text(content)
			comments = append([]string{text}, comments...) // Prepend to keep order
			// Check if there's a gap? Usually comments are adjacent.
			// Ideally check line numbers to ensure contiguity.
		} else {
			// Stop if we hit something else (whitespace/newlines are skipped by Next/Prev sibling usually?
			// No, Tree-sitter includes everything or skips named?
			// Go grammar usually hides layout.
			// Actually bindings/go might not expose whitespace nodes if they are anonymous.
			// But 'comment' is named or hidden?
			// Checking grammar...
			// Assuming 'comment' nodes appear as siblings.
			break
		}
		prev = prev.PrevSibling()
	}
	return strings.Join(comments, "\n")
}

// getSignature extracts the signature (up to Body).
func (e *Extractor) getSignature(n *sitter.Node, content []byte) string {
	// Simplistic: First line or everything before body?
	// Body is usually "block".
	body := n.ChildByFieldName("body")
	if body != nil {
		// Return text from start of N to start of Body
		full := n.Utf8Text(content)
		bodyText := body.Utf8Text(content)
		if idx := strings.Index(full, bodyText); idx != -1 {
			return strings.TrimSpace(full[:idx])
		}
	}
	// Fallback to first line
	full := n.Utf8Text(content)
	if idx := strings.Index(full, "\n"); idx != -1 {
		return full[:idx]
	}
	return full
}

// getReceiverType helper
func (e *Extractor) getReceiverType(n *sitter.Node, content []byte) string {
	for i := uint(0); i < uint(n.ChildCount()); i++ {
		child := n.Child(i)
		if child.Kind() == "parameter_declaration" {
			typeNode := child.ChildByFieldName("type")
			if typeNode != nil {
				t := clean(typeNode.Utf8Text(content))
				return strings.TrimPrefix(t, "*")
			}
			// Fallback
			for j := uint(0); j < uint(child.ChildCount()); j++ {
				gc := child.Child(j)
				if gc.Kind() == "type_identifier" || gc.Kind() == "pointer_type" {
					t := clean(gc.Utf8Text(content))
					return strings.TrimPrefix(t, "*")
				}
			}
		}
	}
	return ""
}
