package meb

import (
	"fmt"

	"github.com/google/mangle/ast"
)

// === Validation Functions ===

// validateFact checks if a fact has valid fields.
func validateFact(fact Fact) error {
	if fact.Subject == "" {
		return fmt.Errorf("%w: subject cannot be empty", ErrInvalidFact)
	}
	if fact.Predicate == "" {
		return fmt.Errorf("%w: predicate cannot be empty", ErrInvalidFact)
	}
	if fact.Object == nil {
		return fmt.Errorf("%w: object cannot be nil", ErrInvalidFact)
	}
	return nil
}

// validateFacts validates a batch of facts.
func validateFacts(facts []Fact) error {
	if len(facts) == 0 {
		return ErrEmptyBatch
	}
	for i, fact := range facts {
		if err := validateFact(fact); err != nil {
			return fmt.Errorf("fact at index %d: %w", i, err)
		}
	}
	return nil
}

// === Normalization Functions ===

// normalizeGraph returns "default" if graph is empty, otherwise returns the input.
func normalizeGraph(graph string) string {
	if graph == "" {
		return "default"
	}
	return graph
}

// === ID Resolution Functions ===

// resolveStringToID converts a string to its dictionary ID.
// Returns error if string not found.
func (m *MEBStore) resolveStringToID(s string) (uint64, error) {
	if s == "" {
		return 0, fmt.Errorf("cannot resolve empty string to ID")
	}
	id, err := m.dict.GetID(s)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve '%s' to ID: %w", s, err)
	}
	return id, nil
}

// === Encoding Functions ===

// encodeObject converts an object value to its dictionary ID and string representation.
// Handles various types including int, int64, float64, bool, and string.
func (m *MEBStore) encodeObject(obj any) (string, uint64, error) {
	if obj == nil {
		return "", 0, fmt.Errorf("%w: object cannot be nil", ErrInvalidFact)
	}

	switch v := obj.(type) {
	case string:
		return v, 0, nil // ID will be obtained from batch
	case int:
		objStr := fmt.Sprintf("%d", v)
		oID, err := m.dict.GetOrCreateID(objStr)
		if err != nil {
			return "", 0, fmt.Errorf("failed to encode int object: %w", err)
		}
		return objStr, oID, nil
	case int64:
		objStr := fmt.Sprintf("%d", v)
		oID, err := m.dict.GetOrCreateID(objStr)
		if err != nil {
			return "", 0, fmt.Errorf("failed to encode int64 object: %w", err)
		}
		return objStr, oID, nil
	case float64:
		objStr := fmt.Sprintf("%f", v)
		oID, err := m.dict.GetOrCreateID(objStr)
		if err != nil {
			return "", 0, fmt.Errorf("failed to encode float64 object: %w", err)
		}
		return objStr, oID, nil
	case bool:
		objStr := fmt.Sprintf("%t", v)
		oID, err := m.dict.GetOrCreateID(objStr)
		if err != nil {
			return "", 0, fmt.Errorf("failed to encode bool object: %w", err)
		}
		return objStr, oID, nil
	default:
		objStr := fmt.Sprintf("%v", v)
		oID, err := m.dict.GetOrCreateID(objStr)
		if err != nil {
			return "", 0, fmt.Errorf("failed to encode object of type %T: %w", v, err)
		}
		return objStr, oID, nil
	}
}

// === Mangle Integration Functions ===

// factToAtom converts a Fact to a Mangle Atom.
func (m *MEBStore) factToAtom(fact Fact) (ast.Atom, error) {
	// Create args from Subject, Object, and Graph (not Predicate)
	args := []ast.BaseTerm{
		ast.String(fact.Subject),
		m.goValueToTerm(fact.Object),
		ast.String(fact.Graph),
	}
	return ast.NewAtom(fact.Predicate, args...), nil
}

// goValueToTerm converts a Go value to a Mangle term.
func (m *MEBStore) goValueToTerm(v any) ast.BaseTerm {
	switch val := v.(type) {
	case string:
		return ast.String(val)
	case int:
		return ast.Number(int64(val))
	case int64:
		return ast.Number(val)
	case float64:
		return ast.Float64(val)
	case bool:
		if val {
			return ast.String("true")
		}
		return ast.String("false")
	default:
		return ast.String(fmt.Sprintf("%v", val))
	}
}

// termToGoValue converts a Mangle term to a Go value.
func (m *MEBStore) termToGoValue(term ast.BaseTerm) any {
	switch t := term.(type) {
	case ast.Constant:
		switch t.Type {
		case ast.StringType:
			return t.Symbol
		case ast.NumberType:
			return t.NumValue
		case ast.Float64Type:
			return t.NumValue
		default:
			return t.Symbol
		}
	default:
		return fmt.Sprintf("%v", t)
	}
}

// (Parsing functions moved to pkg/datalog)
