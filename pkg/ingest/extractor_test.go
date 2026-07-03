package ingest

import (
	"testing"

	"github.com/duynguyendang/gca/pkg/config"
)

func TestExtractReferences_Go(t *testing.T) {
	e := NewTreeSitterExtractor()
	content := []byte(`package main

import "fmt"

func hello() {
	custom.Func("Hello")
	_ = "/api/v1/users"
}
`)

	refs, err := e.ExtractReferences("main.go", content, "main.go")
	if err != nil {
		t.Fatal(err)
	}

	var gotImport, gotCall, gotStrRef bool
	for _, r := range refs {
		switch r.Predicate {
		case config.PredicateImports:
			if r.Object == "fmt" {
				gotImport = true
			}
		case config.PredicateCalls:
			if r.Object == "custom.Func" && r.Subject == "main.go:hello" {
				gotCall = true
			}
		case config.PredicateReferences:
			if r.Object == "/api/v1/users" && r.Subject == "main.go:hello" {
				gotStrRef = true
			}
		}
	}

	if !gotImport {
		t.Error("expected import ref to 'fmt'")
	}
	if !gotCall {
		t.Error("expected call ref to 'custom.Func' from 'main.go:hello'")
	}
	if !gotStrRef {
		t.Error("expected string ref to '/api/v1/users' from 'main.go:hello'")
	}
}

func TestExtractReferences_Go_NoStdLibCalls(t *testing.T) {
	e := NewTreeSitterExtractor()
	content := []byte(`package main

import "fmt"

func check() {
	fmt.Println("hi")
	fn()
}
`)

	refs, err := e.ExtractReferences("main.go", content, "main.go")
	if err != nil {
		t.Fatal(err)
	}

	var gotStdLib, gotCustom bool
	for _, r := range refs {
		if r.Predicate != config.PredicateCalls {
			continue
		}
		if r.Object == "fmt.Println" {
			gotStdLib = true
		}
		if r.Object == "fn" {
			gotCustom = true
		}
	}

	if gotStdLib {
		t.Error("expected stdlib call 'fmt.Println' to be filtered out")
	}
	if !gotCustom {
		t.Error("expected custom call 'fn' to be present")
	}
}

func TestExtractReferences_Python(t *testing.T) {
	e := NewTreeSitterExtractor()
	content := []byte(`import os

def greet(name):
    os.getcwd()
`)

	refs, err := e.ExtractReferences("main.py", content, "main.py")
	if err != nil {
		t.Fatal(err)
	}

	var gotImport, gotCall bool
	for _, r := range refs {
		switch r.Predicate {
		case config.PredicateImports:
			if r.Object == "os" {
				gotImport = true
			}
		case config.PredicateCalls:
			if r.Object == "os.getcwd" && r.Subject == "main.py:greet" {
				gotCall = true
			}
		}
	}

	if !gotImport {
		t.Error("expected import ref to 'os'")
	}
	if !gotCall {
		t.Error("expected call ref to 'os.getcwd' from 'main.py:greet'")
	}
}

func TestExtractReferences_JS(t *testing.T) {
	e := NewTreeSitterExtractor()
	content := []byte(`import { useState } from "react";

function App() {
  useState(0);
  let x = "/api/data";
}
`)

	refs, err := e.ExtractReferences("app.js", content, "app.js")
	if err != nil {
		t.Fatal(err)
	}

	var gotImport, gotCall, gotStrRef bool
	for _, r := range refs {
		switch r.Predicate {
		case config.PredicateImports:
			if r.Object == "react" {
				gotImport = true
			}
		case config.PredicateCalls:
			if r.Object == "useState" && r.Subject == "app.js:App" {
				gotCall = true
			}
		case config.PredicateReferences:
			if r.Object == "/api/data" && r.Subject == "app.js:App" {
				gotStrRef = true
			}
		}
	}

	if !gotImport {
		t.Error("expected import ref to 'react'")
	}
	if !gotCall {
		t.Error("expected call ref to 'useState' from 'app.js:App'")
	}
	if !gotStrRef {
		t.Error("expected string ref to '/api/data' from 'app.js:App'")
	}
}

func TestExtractReferences_EmptyScope(t *testing.T) {
	e := NewTreeSitterExtractor()
	content := []byte(`package main

import "fmt"
`)

	refs, err := e.ExtractReferences("main.go", content, "main.go")
	if err != nil {
		t.Fatal(err)
	}

	var gotCall bool
	for _, r := range refs {
		if r.Predicate == config.PredicateCalls {
			gotCall = true
			break
		}
	}

	if gotCall {
		t.Error("expected no call refs when there is no enclosing function")
	}
}

func TestExtractReferences_JS_LongCallFilter(t *testing.T) {
	e := NewTreeSitterExtractor()
	longIdent := ""
	for i := 0; i < 2000; i++ {
		longIdent += "x"
	}
	content := []byte(`function test() {
  ` + longIdent + `();
  fn();
}
`)

	refs, err := e.ExtractReferences("test.js", content, "test.js")
	if err != nil {
		t.Fatal(err)
	}

	var gotLong, gotShort bool
	for _, r := range refs {
		if r.Predicate != config.PredicateCalls {
			continue
		}
		if r.Object == longIdent {
			gotLong = true
		}
		if r.Object == "fn" {
			gotShort = true
		}
	}

	if gotLong {
		t.Error("expected call to long identifier to be filtered out (>=1024 chars)")
	}
	if !gotShort {
		t.Error("expected call to 'fn' to be present")
	}
}
