package meb

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
)

func newTestStore(t *testing.T, facts []meb.Fact) *meb.MEBStore {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "meb_constraint_test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	cfg := store.DefaultConfig(tmpDir)
	s, err := meb.NewMEBStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	s.SetTopicID(1)

	for _, f := range facts {
		if err := s.AddFact(f); err != nil {
			t.Fatalf("failed to add fact %+v: %v", f, err)
		}
	}
	return s
}

func querySubjects(t *testing.T, ctx context.Context, s *meb.MEBStore, q string) []string {
	t.Helper()
	rows, err := Query(ctx, s, q)
	if err != nil {
		t.Fatalf("Query(%q) error: %v", q, err)
	}
	var subjects []string
	for _, r := range rows {
		if v, ok := r["Subject"].(string); ok {
			subjects = append(subjects, v)
		}
	}
	return subjects
}

func containsAny(rows []string, want string) bool {
	for _, r := range rows {
		if r == want {
			return true
		}
	}
	return false
}

func TestQueryConstraintContains(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, []meb.Fact{
		{Subject: "api/user.go", Predicate: "has_role", Object: "api_handler"},
		{Subject: "api/admin.go", Predicate: "has_role", Object: "api_handler"},
		{Subject: "internal/util.go", Predicate: "has_role", Object: "utility"},
	})

	rows := querySubjects(t, ctx, s, `triples(Subject, "has_role", "api_handler"), contains(Subject, "admin")`)
	if len(rows) != 1 || rows[0] != "api/admin.go" {
		t.Errorf("contains() got %v, want [api/admin.go]", rows)
	}

	// Unbound variable must not match
	rows = querySubjects(t, ctx, s, `triples(Subject, "has_role", "api_handler"), contains(Missing, "admin")`)
	if len(rows) != 0 {
		t.Errorf("contains() over unbound var got %v, want []", rows)
	}
}

func TestQueryConstraintRegex(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, []meb.Fact{
		{Subject: "api/user.go", Predicate: "has_role", Object: "api_handler"},
		{Subject: "api/admin.go", Predicate: "has_role", Object: "api_handler"},
		{Subject: "internal/util.go", Predicate: "has_role", Object: "utility"},
	})

	rows := querySubjects(t, ctx, s, `triples(Subject, "has_role", "api_handler"), regex(Subject, "admin|user")`)
	if len(rows) != 2 {
		t.Errorf("regex() got %v, want 2 subjects", rows)
	}

	// Invalid regex must error loudly
	if _, err := Query(ctx, s, `triples(Subject, "has_role", "api_handler"), regex(Subject, "[")`); err == nil {
		t.Error("invalid regex should return an error")
	}
}

func TestQueryConstraintNotTriples(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, []meb.Fact{
		{Subject: "a.go", Predicate: "calls", Object: "b.go"},
		{Subject: "a.go", Predicate: "calls", Object: "c.go"},
		{Subject: "d.go", Predicate: "calls", Object: "e.go"},
		{Subject: "h.go", Predicate: "calls", Object: "g.go"},
		{Subject: "f.go", Predicate: "has_smell_type", Object: "god_file"},
		{Subject: "h.go", Predicate: "has_smell_type", Object: "god_file"},
	})

	// Sugar form: not triples(Subject, "calls", _) — f.go has no calls, h.go does.
	rows := querySubjects(t, ctx, s, `triples(Subject, "has_smell_type", "god_file"), not triples(Subject, "calls", _)`)
	if len(rows) != 1 || rows[0] != "f.go" {
		t.Errorf("not triples(...) got %v, want [f.go]", rows)
	}

	// Parenthesized form: not(triples(...)) with a bound var and unbound object.
	rows = querySubjects(t, ctx, s, `triples(Subject, "has_smell_type", "god_file"), not(triples(Subject, "calls", ErrCheck))`)
	if len(rows) != 1 || rows[0] != "f.go" {
		t.Errorf("not(triples(...)) got %v, want [f.go]", rows)
	}
}

func TestQueryConstraintNotContains(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, []meb.Fact{
		{Subject: "api/user.go", Predicate: "has_role", Object: "api_handler"},
		{Subject: "internal/util.go", Predicate: "has_role", Object: "api_handler"},
	})

	rows := querySubjects(t, ctx, s, `triples(Subject, "has_role", "api_handler"), not contains(Subject, "api")`)
	if len(rows) != 1 || rows[0] != "internal/util.go" {
		t.Errorf("not contains() got %v, want [internal/util.go]", rows)
	}
}

func TestQueryConstraintOrErrorsLoudly(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, []meb.Fact{
		{Subject: "main.go", Predicate: "defines", Object: "main"},
	})

	_, err := Query(ctx, s, `triples(Subject, "defines", Symbol), or(contains(Symbol, "main"), contains(Symbol, "init"))`)
	if err == nil {
		t.Fatal("or(...) should error loudly, got nil")
	}
	if !strings.Contains(err.Error(), "or(") {
		t.Errorf("or(...) error should mention or(), got: %v", err)
	}
}

func TestQueryConstraintUnknownAtomErrorsLoudly(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, []meb.Fact{
		{Subject: "a.go", Predicate: "calls", Object: "b.go"},
	})

	// Unknown constraint atom must error even when the triples side is empty.
	_, err := Query(ctx, s, `triples(Subject, "calls", "b.go"), foo(Subject, "bar")`)
	if err == nil {
		t.Fatal("unknown constraint atom should error loudly, got nil")
	}

	// Derived predicate inside not() must error loudly.
	_, err = Query(ctx, s, `triples(Subject, "calls", "b.go"), not derived_pred(Subject, _)`)
	if err == nil {
		t.Fatal("not(<derived>) should error loudly, got nil")
	}
	if !strings.Contains(err.Error(), "derived predicate") {
		t.Errorf("not(<derived>) error should mention derived predicate, got: %v", err)
	}
}

func TestQueryWildcard(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, []meb.Fact{
		{Subject: "a.go", Predicate: "calls", Object: "b.go"},
		{Subject: "c.go", Predicate: "calls", Object: "d.go"},
	})

	// '_' must act as a wildcard object.
	rows := querySubjects(t, ctx, s, `triples(Subject, "calls", _)`)
	if len(rows) != 2 || !containsAny(rows, "a.go") || !containsAny(rows, "c.go") {
		t.Errorf("wildcard object got %v, want both subjects", rows)
	}

	// '_' must act as a wildcard subject.
	wildRows, err := Query(ctx, s, `triples(_, "calls", Obj)`)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(wildRows) != 2 {
		t.Errorf("wildcard subject got %d rows, want 2", len(wildRows))
	}
}

func TestQueryConstraintComparisonStillWorks(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t, []meb.Fact{
		{Subject: "a.go", Predicate: "has_in_degree", Object: "3"},
		{Subject: "b.go", Predicate: "has_in_degree", Object: "8"},
	})

	rows := querySubjects(t, ctx, s, `triples(Subject, "has_in_degree", Degree), gt(Degree, 5)`)
	if len(rows) != 1 || rows[0] != "b.go" {
		t.Errorf("gt() got %v, want [b.go]", rows)
	}

	rows = querySubjects(t, ctx, s, `triples(Subject, "has_in_degree", Degree), Subject != "a.go"`)
	if len(rows) != 1 || rows[0] != "b.go" {
		t.Errorf("!= got %v, want [b.go]", rows)
	}
}
