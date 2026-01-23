package meb

import (
	"context"
	"fmt"
)

// ResolveDependencies infers missing dependencies and adds them as virtual triples.
// It analyzes interface implementations and dependency injection wiring.
func (m *MEBStore) ResolveDependencies(ctx context.Context) error {
	// 1. Interface Analysis: Interface-to-Implementation
	// Find all "defines(File, Interface)" where Interface is a type definition of kind "interface".
	// Ideally we look for triples(S, "kind", "interface") and then find usage.
	// But let's follow the task description:
	// "Scan for defines(File, Interface) and implements(Struct, Interface)"

	// Since we might not have explicit "implements" facts (Go uses structural typing),
	// this step implies either we have them from ingestion or we infer them here?
	// Task says "Scan for ... implements", suggesting they exist.

	// Scan for anything that "implements" something
	// Pattern: ?implements(Struct, Interface)
	// We iterate all "implements" facts.
	for fact := range m.ScanContext(ctx, "", "implements", "", "") {
		// fact.Subject = Struct
		// fact.Object = Interface
		iface, ok := fact.Object.(string)
		if !ok {
			continue
		}
		structID := fact.Subject

		// Now look for who CALLS the interface methods?
		// Or who USES the interface?
		// "Insert v:potentially_calls(S, O) where S calls the interface."
		// So we need to find calls to the Interface.
		// triples(?Caller, "calls", Interface)

		// Wait, usage of interface usually appears as:
		// func Foo(i Interface) { i.Bar() }
		// The call is to Interface.Bar.
		// The relationship might be triples(Caller, "calls", Interface) or specific methods.
		// Let's assume standard "calls" to the Interface symbol exists.

		for callFact := range m.ScanContext(ctx, "", "calls", iface, "") {
			callerID := callFact.Subject

			// We have: Caller -> Interface <- matches Struct
			// Inference: Caller -> potentially_calls -> Struct

			// Create virtual fact
			vFact := Fact{
				Subject:   callerID,
				Predicate: "v:potentially_calls",
				Object:    string(structID),
				Graph:     "default", // Virtual facts in default graph?
				Weight:    0.8,
				Source:    "virtual",
			}

			if err := m.AddFact(vFact); err != nil {
				return fmt.Errorf("failed to add virtual fact: %w", err)
			}
		}
	}

	// 2. DI/Wire Analysis
	// Scan for files using has_hash predicate (heuristic to iterate likely files)
	for fileFact := range m.ScanContext(ctx, "", "has_hash", "", "") {
		fileID := fileFact.Subject
		doc, err := m.GetDocument(fileID)
		if err != nil {
			continue
		}

		wireID, ok := doc.Metadata["wire"].(string)
		if !ok || wireID == "" {
			continue
		}

		// This component wires to `wireID`.
		// Find who defines `wireID`.

		// Iterate all facts where predicate is "defines".
		for defFact := range m.ScanContext(ctx, "", "defines", "", "") {
			targetSymID := defFact.Object.(string)
			// Check if targetSymID contains wireID
			if isWireMatch(targetSymID, wireID) {
				// v:wires_to(ComponentA, TargetSymbol)

				vFact := Fact{
					Subject:   fileID,
					Predicate: "v:wires_to",
					Object:    targetSymID,
					Graph:     "default",
					Weight:    0.5,
					Source:    "virtual",
				}
				// Ignore errors on add
				_ = m.AddFact(vFact)
			}
		}
	}

	return nil
}

func isWireMatch(fullID, wireName string) bool {
	// Simple heuristic: Does it end with ":wireName" or is it exactly "wireName"?
	// Also case-insensitive?
	if fullID == wireName {
		return true
	}
	suffix := ":" + wireName
	if len(fullID) >= len(suffix) && fullID[len(fullID)-len(suffix):] == suffix {
		return true
	}
	return false
}
