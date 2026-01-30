package ingest

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/duynguyendang/gca/pkg/meb"
	sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
	javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// SymbolType constants
const (
	TypeFunction  = "function"
	TypeMethod    = "method"
	TypeStruct    = "struct"
	TypeInterface = "interface"
	TypeClass     = "class"
	TypeVariable  = "variable"
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

// TreeSitterExtractor handles AST parsing and symbol extraction.
type TreeSitterExtractor struct {
	parser *sitter.Parser
}

// NewTreeSitterExtractor creates a new extractor instance.
func NewTreeSitterExtractor() *TreeSitterExtractor {
	parser := sitter.NewParser()
	return &TreeSitterExtractor{parser: parser}
}

// GetParser returns the appropriate language parser for the given extension.
func (e *TreeSitterExtractor) GetParser(ext string) *sitter.Language {
	switch ext {
	case ".py":
		return sitter.NewLanguage(python.Language())
	case ".js", ".jsx":
		return sitter.NewLanguage(javascript.Language())
	case ".ts":
		return sitter.NewLanguage(typescript.LanguageTypescript())
	case ".tsx":
		return sitter.NewLanguage(typescript.LanguageTSX())
	default:
		return sitter.NewLanguage(golang.Language())
	}
}

// ExtractSymbols parses the content and returns a list of symbols.
func (e *TreeSitterExtractor) ExtractSymbols(filename string, content []byte, relPath string) ([]Symbol, error) {
	ext := filepath.Ext(filename)
	lang := e.GetParser(ext)
	e.parser.SetLanguage(lang)

	tree := e.parser.Parse(content, nil)
	if tree == nil {
		return nil, fmt.Errorf("failed to parse tree")
	}
	root := tree.RootNode()
	if root == nil {
		return nil, fmt.Errorf("empty root node")
	}

	var symbols []Symbol

	// Generic package name detection (Go mostly)
	pkgName := ""
	if ext == ".go" {
		for i := uint(0); i < uint(root.ChildCount()); i++ {
			child := root.Child(i)
			if child.Kind() == "package_clause" {
				nameNode := child.ChildByFieldName("name")
				if nameNode != nil {
					pkgName = clean(nameNode.Utf8Text(content))
					// If package name is empty (e.g. comment), defaults to "" which is fine
				}
				break
			}
		}
	}

	// Walk the tree
	var walk func(n *sitter.Node, currentScope string)
	walk = func(n *sitter.Node, currentScope string) {
		nextScope := currentScope
		switch ext {
		case ".go":
			e.extractGoNode(n, content, relPath, pkgName, &symbols)
		case ".py":
			if s := e.extractPythonNode(n, content, relPath, currentScope, &symbols); s != "" {
				nextScope = s
			}
		case ".js", ".jsx", ".ts", ".tsx":
			if s := e.extractJSNode(n, content, relPath, currentScope, &symbols); s != "" {
				nextScope = s
			}
		}

		// Recurse
		for i := uint(0); i < uint(n.ChildCount()); i++ {
			walk(n.Child(i), nextScope)
		}
	}

	walk(root, "")
	return symbols, nil
}

// ExtractReferences parses the content and returns a list of references (calls, imports, etc).
func (e *TreeSitterExtractor) ExtractReferences(filename string, content []byte, relPath string) ([]Reference, error) {
	ext := filepath.Ext(filename)
	lang := e.GetParser(ext)
	e.parser.SetLanguage(lang)

	tree := e.parser.Parse(content, nil)
	if tree == nil {
		return nil, fmt.Errorf("failed to parse tree")
	}
	root := tree.RootNode()

	var refs []Reference

	var walk func(n *sitter.Node, currentScope string)
	walk = func(n *sitter.Node, currentScope string) {
		nextScope := currentScope

		switch ext {
		case ".go":
			nextScope = e.extractGoRefs(n, content, relPath, currentScope, &refs)
		case ".py":
			nextScope = e.extractPythonRefs(n, content, relPath, currentScope, &refs)
		case ".js", ".jsx", ".ts", ".tsx":
			nextScope = e.extractJSRefs(n, content, relPath, currentScope, &refs)
		}

		for i := uint(0); i < uint(n.ChildCount()); i++ {
			walk(n.Child(i), nextScope)
		}
	}

	walk(root, "")
	return refs, nil
}

// Extract satisfies the Extractor interface.
func (e *TreeSitterExtractor) Extract(ctx context.Context, relPath string, content []byte) (*AnalysisBundle, error) {
	// Parse Symbols
	symbols, err := e.ExtractSymbols(relPath, content, relPath)
	if err != nil {
		return nil, err
	}

	bundle := &AnalysisBundle{
		Documents: make([]meb.Document, 0, len(symbols)),
		Facts:     make([]meb.Fact, 0, len(symbols)*5),
	}

	// Process Symbols
	for _, sym := range symbols {
		// Create Document
		doc := meb.Document{
			ID:      meb.DocumentID(sym.ID),
			Content: []byte(sym.Content),
			Metadata: map[string]any{
				"file":       relPath,
				"start_line": sym.StartLine,
				"end_line":   sym.EndLine,
				"package":    sym.Package,
			},
		}
		bundle.Documents = append(bundle.Documents, doc)

		// Create Facts
		bundle.Facts = append(bundle.Facts,
			meb.Fact{Subject: meb.DocumentID(sym.ID), Predicate: meb.PredType, Object: sym.Type, Graph: "default"},
			meb.Fact{Subject: meb.DocumentID(relPath), Predicate: meb.PredDefines, Object: sym.ID, Graph: "default"},
		)

		if sym.DocComment != "" {
			bundle.Facts = append(bundle.Facts, meb.Fact{
				Subject:   meb.DocumentID(sym.ID),
				Predicate: meb.PredHasDoc,
				Object:    sym.DocComment,
				Graph:     "default",
			})
		}
	}

	// Process References
	refs, err := e.ExtractReferences(relPath, content, relPath)
	if err != nil {
		// Log error but continue? Or fail? Sticking to non-fatal for refs extraction issues.
		// For now return partial bundle with error? Or just log?
		return bundle, fmt.Errorf("failed to extract references: %w", err)
	}

	for _, ref := range refs {
		bundle.Facts = append(bundle.Facts, meb.Fact{
			Subject:   meb.DocumentID(ref.Subject),
			Predicate: ref.Predicate,
			Object:    ref.Object,
			Graph:     "default",
		})
	}

	return bundle, nil
}

// --- Go Extraction ---

func (e *TreeSitterExtractor) extractGoNode(n *sitter.Node, content []byte, relPath, pkgName string, symbols *[]Symbol) {
	switch n.Kind() {
	case "function_declaration":
		*symbols = append(*symbols, e.extractFunction(n, content, relPath, pkgName))
	case "method_declaration":
		sym := e.extractMethod(n, content, relPath, pkgName)
		if sym.Name != "" {
			*symbols = append(*symbols, sym)
		}
	case "type_declaration":
		for i := uint(0); i < uint(n.ChildCount()); i++ {
			child := n.Child(i)
			if child.Kind() == "type_spec" {
				sym := e.extractType(child, n, content, relPath, pkgName)
				if sym.Name != "" {
					*symbols = append(*symbols, sym)
				}
			}
		}
	}
}

func (e *TreeSitterExtractor) extractGoRefs(n *sitter.Node, content []byte, relPath, currentScope string, refs *[]Reference) string {
	nextScope := currentScope
	switch n.Kind() {
	case "import_declaration":
		for i := uint(0); i < uint(n.ChildCount()); i++ {
			child := n.Child(i)
			if child.Kind() == "import_spec" {
				e.addImportRef(content, child, relPath, refs)
			} else if child.Kind() == "import_spec_list" {
				for j := uint(0); j < uint(child.ChildCount()); j++ {
					grandChild := child.Child(j)
					if grandChild.Kind() == "import_spec" {
						e.addImportRef(content, grandChild, relPath, refs)
					}
				}
			}
		}
	case "function_declaration":
		nameNode := n.ChildByFieldName("name")
		if nameNode != nil {
			funcName := clean(nameNode.Utf8Text(content))
			if funcName != "" {
				nextScope = fmt.Sprintf("%s:%s", relPath, funcName)
			}
		}
	case "method_declaration":
		nameNode := n.ChildByFieldName("name")
		receiverNode := n.ChildByFieldName("receiver")
		if nameNode != nil && receiverNode != nil {
			methodName := clean(nameNode.Utf8Text(content))
			if methodName != "" {
				receiverType := e.getReceiverType(receiverNode, content)
				if receiverType != "" {
					nextScope = fmt.Sprintf("%s:%s.%s", relPath, receiverType, methodName)
				} else {
					nextScope = fmt.Sprintf("%s:.%s", relPath, methodName)
				}
			}
		}
	case "call_expression":
		if currentScope != "" {
			funcNode := n.ChildByFieldName("function")
			if funcNode != nil {
				callee := clean(funcNode.Utf8Text(content))
				if callee != "" && !isStdLibCall(callee, "go") {
					*refs = append(*refs, Reference{
						Subject:   currentScope,
						Predicate: meb.PredCalls,
						Object:    callee,
						Line:      lineFromOffset(content, n.StartByte()),
					})
				}
			}
		}
	}
	return nextScope
}

// --- Python Extraction ---

func (e *TreeSitterExtractor) extractPythonNode(n *sitter.Node, content []byte, relPath, parentScope string, symbols *[]Symbol) string {
	newScope := ""
	switch n.Kind() {
	case "function_definition":
		nameNode := n.ChildByFieldName("name")
		if nameNode != nil {
			name := clean(nameNode.Utf8Text(content))
			id := ""
			if parentScope == "" {
				id = fmt.Sprintf("%s:%s", relPath, name)
			} else {
				id = fmt.Sprintf("%s.%s", parentScope, name)
			}
			newScope = id
			doc := e.getPythonDocString(n, content)
			sig := e.getSignature(n, content)
			*symbols = append(*symbols, Symbol{
				ID:         id,
				Name:       name,
				Type:       TypeFunction,
				Signature:  sig,
				DocComment: doc,
				Content:    n.Utf8Text(content),
				StartLine:  lineFromOffset(content, n.StartByte()),
				EndLine:    lineFromOffset(content, n.EndByte()),
			})
		}
	case "class_definition":
		nameNode := n.ChildByFieldName("name")
		if nameNode != nil {
			name := clean(nameNode.Utf8Text(content))
			id := ""
			if parentScope == "" {
				id = fmt.Sprintf("%s:%s", relPath, name)
			} else {
				id = fmt.Sprintf("%s.%s", parentScope, name)
			}
			newScope = id
			doc := e.getPythonDocString(n, content)
			sig := e.getSignature(n, content)
			*symbols = append(*symbols, Symbol{
				ID:         id,
				Name:       name,
				Type:       TypeClass,
				Signature:  sig,
				DocComment: doc,
				Content:    n.Utf8Text(content),
				StartLine:  lineFromOffset(content, n.StartByte()),
				EndLine:    lineFromOffset(content, n.EndByte()),
			})
		}
	}
	return newScope
}

func (e *TreeSitterExtractor) extractPythonRefs(n *sitter.Node, content []byte, relPath, currentScope string, refs *[]Reference) string {
	nextScope := currentScope
	switch n.Kind() {
	case "function_definition", "class_definition":
		nameNode := n.ChildByFieldName("name")
		if nameNode != nil {
			name := clean(nameNode.Utf8Text(content))
			if currentScope == "" {
				nextScope = fmt.Sprintf("%s:%s", relPath, name)
			} else {
				nextScope = fmt.Sprintf("%s.%s", currentScope, name)
			}
		}
	case "import_statement":
		// import X
		for i := uint(0); i < uint(n.ChildCount()); i++ {
			child := n.Child(i)
			if child.Kind() == "dotted_name" {
				imp := clean(child.Utf8Text(content))
				resolvedImp := resolveImportPath(relPath, imp)
				*refs = append(*refs, Reference{
					Subject:   relPath,
					Predicate: meb.PredImports,
					Object:    resolvedImp,
					Line:      lineFromOffset(content, n.StartByte()),
				})
			} else if child.Kind() == "aliased_import" {
				name := child.ChildByFieldName("name")
				if name != nil {
					imp := clean(name.Utf8Text(content))
					resolvedImp := resolveImportPath(relPath, imp)
					*refs = append(*refs, Reference{
						Subject:   relPath,
						Predicate: meb.PredImports,
						Object:    resolvedImp,
						Line:      lineFromOffset(content, n.StartByte()),
					})
				}
			}
		}
	case "import_from_statement":
		// from X import Y
		modNameNode := n.ChildByFieldName("module_name")
		if modNameNode != nil {
			modName := clean(modNameNode.Utf8Text(content))
			resolvedMod := resolveImportPath(relPath, modName)
			*refs = append(*refs, Reference{
				Subject:   relPath,
				Predicate: meb.PredImports,
				Object:    resolvedMod,
				Line:      lineFromOffset(content, n.StartByte()),
			})
		}
	case "call":
		if currentScope != "" {
			funcNode := n.ChildByFieldName("function")
			if funcNode != nil {
				callee := clean(funcNode.Utf8Text(content))
				if !isStdLibCall(callee, "python") {
					*refs = append(*refs, Reference{
						Subject:   currentScope,
						Predicate: meb.PredCalls,
						Object:    callee,
						Line:      lineFromOffset(content, n.StartByte()),
					})
				}
			}
		}
	}
	return nextScope
}

func (e *TreeSitterExtractor) getPythonDocString(n *sitter.Node, content []byte) string {
	body := n.ChildByFieldName("body")
	if body != nil && body.ChildCount() > 0 {
		firstStmt := body.Child(0)
		if firstStmt.Kind() == "expression_statement" {
			expr := firstStmt.Child(0)
			if expr.Kind() == "string" {
				return clean(expr.Utf8Text(content))
			}
		}
	}
	return ""
}

// --- JS/TS Extraction ---

func (e *TreeSitterExtractor) extractJSNode(n *sitter.Node, content []byte, relPath, parentScope string, symbols *[]Symbol) string {
	kind := n.Kind()
	var name, symType string
	var receiver string
	newScope := ""

	switch kind {
	case "function_declaration":
		nameNode := n.ChildByFieldName("name")
		if nameNode != nil {
			name = clean(nameNode.Utf8Text(content))
			symType = TypeFunction
		}
	case "method_definition":
		nameNode := n.ChildByFieldName("name")
		if nameNode != nil {
			name = clean(nameNode.Utf8Text(content))
			symType = TypeMethod
		}
	case "class_declaration", "class_definition":
		nameNode := n.ChildByFieldName("name")
		if nameNode != nil {
			name = clean(nameNode.Utf8Text(content))
			symType = TypeClass
		}
	case "interface_declaration":
		nameNode := n.ChildByFieldName("name")
		if nameNode != nil {
			name = clean(nameNode.Utf8Text(content))
			symType = TypeInterface
		}
	case "lexical_declaration", "variable_declaration":
		// const x = ..., let y = ...
		for i := uint(0); i < uint(n.ChildCount()); i++ {
			child := n.Child(i)
			if child.Kind() == "variable_declarator" {
				nameNode := child.ChildByFieldName("name")
				valNode := child.ChildByFieldName("value")
				if nameNode != nil {
					name = clean(nameNode.Utf8Text(content))
					// Check if it's an arrow function or function expression
					if valNode != nil && (valNode.Kind() == "arrow_function" || valNode.Kind() == "function_expression") {
						symType = TypeFunction
						e.addGenericSymbol(name, symType, receiver, n, content, relPath, parentScope, symbols)
						name = "" // reset
					} else {
						if n.Parent().Kind() == "program" || n.Parent().Kind() == "export_statement" {
							symType = TypeVariable
							e.addGenericSymbol(name, symType, "variable", n, content, relPath, parentScope, symbols)
							name = ""
						}
					}
				}
			}
		}
	}

	if name != "" {
		newScope = e.addGenericSymbol(name, symType, receiver, n, content, relPath, parentScope, symbols)
	}
	return newScope
}

func (e *TreeSitterExtractor) addGenericSymbol(name, symType, receiver string, n *sitter.Node, content []byte, relPath, parentScope string, symbols *[]Symbol) string {
	if name == "" {
		return ""
	}
	id := ""
	if parentScope == "" {
		id = fmt.Sprintf("%s:%s", relPath, name)
	} else {
		id = fmt.Sprintf("%s.%s", parentScope, name)
	}

	doc := e.getDocComment(n, content)
	// If no doc found, and parent is an export statement, check the parent's comments
	if doc == "" && n.Parent() != nil && n.Parent().Kind() == "export_statement" {
		doc = e.getDocComment(n.Parent(), content)
	}

	sig := e.getSignature(n, content)
	*symbols = append(*symbols, Symbol{
		ID:         id,
		Name:       name,
		Type:       symType,
		Receiver:   receiver,
		Signature:  sig,
		DocComment: doc,
		Content:    n.Utf8Text(content),
		StartLine:  lineFromOffset(content, n.StartByte()),
		EndLine:    lineFromOffset(content, n.EndByte()),
	})
	return id
}

func (e *TreeSitterExtractor) extractJSRefs(n *sitter.Node, content []byte, relPath, currentScope string, refs *[]Reference) string {
	nextScope := currentScope
	kind := n.Kind()

	switch kind {
	case "function_declaration", "method_definition", "class_declaration", "class_definition":
		// Update scope
		nameNode := n.ChildByFieldName("name")
		if nameNode != nil {
			name := clean(nameNode.Utf8Text(content))
			if currentScope == "" {
				nextScope = fmt.Sprintf("%s:%s", relPath, name)
			} else {
				nextScope = fmt.Sprintf("%s.%s", currentScope, name)
			}
		}
	case "import_statement":
		// import { X } from 'Y'; or import X from 'Y';
		sourceNode := n.ChildByFieldName("source")
		if sourceNode != nil {
			src := clean(sourceNode.Utf8Text(content))
			resolvedSrc := resolveImportPath(relPath, src)
			*refs = append(*refs, Reference{
				Subject:   relPath,
				Predicate: meb.PredImports,
				Object:    resolvedSrc,
				Line:      lineFromOffset(content, n.StartByte()),
			})
		}
	case "call_expression":
		if currentScope != "" {
			funcNode := n.ChildByFieldName("function")
			if funcNode != nil {
				callee := clean(funcNode.Utf8Text(content))
				if !isStdLibCall(callee, "js") {
					*refs = append(*refs, Reference{
						Subject:   currentScope,
						Predicate: meb.PredCalls,
						Object:    callee,
						Line:      lineFromOffset(content, n.StartByte()),
					})
				}
			}
		}
	}

	return nextScope
}

// --- Helpers ---

func (e *TreeSitterExtractor) addImportRef(content []byte, node *sitter.Node, relPath string, refs *[]Reference) {
	pathNode := node.ChildByFieldName("path")
	if pathNode != nil {
		impPath := clean(pathNode.Utf8Text(content))
		*refs = append(*refs, Reference{
			Subject:   relPath,
			Predicate: meb.PredImports,
			Object:    impPath,
			Line:      lineFromOffset(content, node.StartByte()),
		})
	}
}

// extractFunction, extractMethod, extractType are now integrated or helpers
// I'll keep the Go ones as helpers called by extractGoNode to reduce code movement if I want,
// but I've already inlined them in extractGoNode for cleaner switch.
// Wait, I used e.extractFunction in extractGoNode above. I need to keep them or move logic.
// I'll keep them to minimize diff noise if possible, but I rewrote the file logic.
// I will re-implement them or include them.

func (e *TreeSitterExtractor) extractFunction(n *sitter.Node, content []byte, relPath string, pkgName string) Symbol {
	nameNode := n.ChildByFieldName("name")
	name := ""
	if nameNode != nil {
		name = clean(nameNode.Utf8Text(content))
	}
	if name == "" {
		return Symbol{}
	}

	id := fmt.Sprintf("%s:%s", relPath, name)
	doc := e.getDocComment(n, content)
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

func (e *TreeSitterExtractor) extractMethod(n *sitter.Node, content []byte, relPath string, pkgName string) Symbol {
	nameNode := n.ChildByFieldName("name")
	name := ""
	if nameNode != nil {
		name = clean(nameNode.Utf8Text(content))
	}
	if name == "" {
		return Symbol{}
	}

	receiverNode := n.ChildByFieldName("receiver")
	receiverType := ""
	if receiverNode != nil {
		receiverType = e.getReceiverType(receiverNode, content)
	}

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

func (e *TreeSitterExtractor) extractType(spec *sitter.Node, decl *sitter.Node, content []byte, relPath string, pkgName string) Symbol {
	nameNode := spec.ChildByFieldName("name")
	name := ""
	if nameNode != nil {
		name = clean(nameNode.Utf8Text(content))
	}
	if name == "" {
		return Symbol{}
	}

	typeNode := spec.ChildByFieldName("type")
	kind := TypeStruct
	if typeNode != nil && typeNode.Kind() == "interface_type" {
		kind = TypeInterface
	}

	id := fmt.Sprintf("%s:%s", relPath, name)
	doc := e.getDocComment(decl, content) // Use decl doc

	return Symbol{
		ID:         id,
		Name:       name,
		Type:       kind,
		Signature:  fmt.Sprintf("type %s %s", name, kind),
		DocComment: doc,
		Content:    spec.Utf8Text(content),
		StartLine:  lineFromOffset(content, spec.StartByte()),
		EndLine:    lineFromOffset(content, spec.EndByte()),
		Package:    pkgName,
	}
}

func (e *TreeSitterExtractor) getDocComment(n *sitter.Node, content []byte) string {
	var comments []string
	prev := n.PrevSibling()
	for prev != nil {
		if prev.Kind() == "comment" {
			text := prev.Utf8Text(content)
			comments = append([]string{text}, comments...)
		} else {
			break
		}
		prev = prev.PrevSibling()
	}
	return strings.Join(comments, "\n")
}

func (e *TreeSitterExtractor) getSignature(n *sitter.Node, content []byte) string {
	body := n.ChildByFieldName("body")
	if body != nil {
		full := n.Utf8Text(content)
		bodyText := body.Utf8Text(content)
		if idx := strings.Index(full, bodyText); idx != -1 {
			return strings.TrimSpace(full[:idx])
		}
	}
	full := n.Utf8Text(content)
	if idx := strings.Index(full, "\n"); idx != -1 {
		return full[:idx]
	}
	return full
}

func (e *TreeSitterExtractor) getReceiverType(n *sitter.Node, content []byte) string {
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

func resolveImportPath(relPath, importPath string) string {
	if !strings.HasPrefix(importPath, ".") {
		return importPath
	}

	dir := filepath.Dir(relPath)
	basePath := filepath.Clean(filepath.Join(dir, importPath))

	// 1. Exact match
	if fileIndex[basePath] {
		return basePath
	}

	// 2. Try extensions
	extensions := []string{".ts", ".tsx", ".js", ".jsx", ".py", ".go"}
	for _, ext := range extensions {
		candidate := basePath + ext
		if fileIndex[candidate] {
			return candidate
		}
	}

	// 3. Try index files
	for _, ext := range extensions {
		candidate := filepath.Join(basePath, "index"+ext)
		if fileIndex[candidate] {
			return candidate
		}
	}

	return basePath
}
