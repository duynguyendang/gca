package ephemeral

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/duynguyendang/meb"

	gcamdb "github.com/duynguyendang/gca/pkg/meb"
)

func TestParseDiff_Empty(t *testing.T) {
	es := NewEphemeralStore(defaultSessionTTL)
	defer es.Close()

	session, err := es.NewSession("test")
	if err != nil {
		t.Fatal(err)
	}

	count, err := ParseDiff("", session)
	if err != nil {
		t.Fatalf("ParseDiff() error = %v", err)
	}
	if count != 0 {
		t.Errorf("ParseDiff('') = %d, want 0", count)
	}
}

func TestParseDiff_Addition(t *testing.T) {
	diff := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 line1
 line2
+new_line
 line3
`
	es := NewEphemeralStore(defaultSessionTTL)
	defer es.Close()

	session, err := es.NewSession("test")
	if err != nil {
		t.Fatal(err)
	}


	count, err := ParseDiff(diff, session)
	if err != nil {
		t.Fatalf("ParseDiff() error = %v", err)
	}

	if count != 1 {
		t.Fatalf("expected 1 fact, got %d", count)
	}

	verifyFacts(t, session.Facts, map[string]int{DiffAdded: 1})
	verifyFactExists(t, session.Facts, "file.go", DiffAdded, "3:new_line")
}

func TestParseDiff_Removal(t *testing.T) {
	diff := `--- a/file.go
+++ b/file.go
@@ -1,3 +1,2 @@
 line1
-remove_me
 line2
`
	es := NewEphemeralStore(defaultSessionTTL)
	defer es.Close()

	session, err := es.NewSession("test")
	if err != nil {
		t.Fatal(err)
	}


	count, err := ParseDiff(diff, session)
	if err != nil {
		t.Fatalf("ParseDiff() error = %v", err)
	}

	if count != 1 {
		t.Fatalf("expected 1 fact, got %d", count)
	}

	verifyFacts(t, session.Facts, map[string]int{DiffRemoved: 1})
	verifyFactExists(t, session.Facts, "file.go", DiffRemoved, "2:remove_me")
}

func TestParseDiff_Modification(t *testing.T) {
	diff := `--- a/file.go
+++ b/file.go
@@ -10,3 +10,3 @@
 unchanged
-old_content
+new_content
 unchanged_after
`
	es := NewEphemeralStore(defaultSessionTTL)
	defer es.Close()

	session, err := es.NewSession("test")
	if err != nil {
		t.Fatal(err)
	}


	count, err := ParseDiff(diff, session)
	if err != nil {
		t.Fatalf("ParseDiff() error = %v", err)
	}
	// diff_removed + diff_modified + diff_added = 3 facts for one modification
	if count != 3 {
		t.Fatalf("expected 3 facts (removed+modified+added), got %d", count)
	}

	verifyFacts(t, session.Facts, map[string]int{DiffRemoved: 1, DiffModified: 1, DiffAdded: 1})
	verifyFactExists(t, session.Facts, "file.go", DiffRemoved, "11:old_content")
	verifyFactExists(t, session.Facts, "file.go", DiffAdded, "11:new_content")
	verifyFactExists(t, session.Facts, "file.go", DiffModified, "11:11:new_content")
}

func TestParseDiff_ModificationLineNumbersDiffer(t *testing.T) {
	diff := `--- a/file.go
+++ b/file.go
@@ -5,5 +8,5 @@
 context1
-old_line
+new_line
 context2
 context3
`
	es := NewEphemeralStore(defaultSessionTTL)
	defer es.Close()

	session, err := es.NewSession("test")
	if err != nil {
		t.Fatal(err)
	}


	_, err = ParseDiff(diff, session)
	if err != nil {
		t.Fatalf("ParseDiff() error = %v", err)
	}

	verifyFacts(t, session.Facts, map[string]int{DiffRemoved: 1, DiffModified: 1, DiffAdded: 1})
	// old line 6 (oldStart=5, context at 5, remove at 6)
	verifyFactExists(t, session.Facts, "file.go", DiffRemoved, "6:old_line")
	// new line 9 (newStart=8, context at 8, add at 9)
	verifyFactExists(t, session.Facts, "file.go", DiffAdded, "9:new_line")
	// modified triple encodes old:new:content
	verifyFactExists(t, session.Facts, "file.go", DiffModified, "6:9:new_line")
}

func TestParseDiff_MultipleFiles(t *testing.T) {
	diff := `--- a/foo.go
+++ b/foo.go
@@ -1,1 +1,1 @@
-old_foo
+new_foo
--- a/bar.go
+++ b/bar.go
@@ -1,1 +1,1 @@
-old_bar
+new_bar
`
	es := NewEphemeralStore(defaultSessionTTL)
	defer es.Close()

	session, err := es.NewSession("test")
	if err != nil {
		t.Fatal(err)
	}


	count, err := ParseDiff(diff, session)
	if err != nil {
		t.Fatalf("ParseDiff() error = %v", err)
	}
	// 2 files × 3 facts each = 6
	t.Logf("count = %d, expected 6", count)
	verifyFacts(t, session.Facts, map[string]int{DiffRemoved: 2, DiffModified: 2, DiffAdded: 2})
	verifyFactExists(t, session.Facts, "foo.go", DiffRemoved, "1:old_foo")
	verifyFactExists(t, session.Facts, "foo.go", DiffAdded, "1:new_foo")
	verifyFactExists(t, session.Facts, "bar.go", DiffRemoved, "1:old_bar")
	verifyFactExists(t, session.Facts, "bar.go", DiffAdded, "1:new_bar")
}

func TestParseDiff_NewFile(t *testing.T) {
	diff := `diff --git a/newfile.go b/newfile.go
--- /dev/null
+++ b/newfile.go
@@ -0,0 +1,3 @@
+line1
+line2
+line3
`
	es := NewEphemeralStore(defaultSessionTTL)
	defer es.Close()

	session, err := es.NewSession("test")
	if err != nil {
		t.Fatal(err)
	}


	count, err := ParseDiff(diff, session)
	if err != nil {
		t.Fatalf("ParseDiff() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 facts, got %d", count)
	}

	verifyFacts(t, session.Facts, map[string]int{DiffAdded: 3})
}

func TestParseDiff_LineNumbersCorrectForContext(t *testing.T) {
	diff := `--- a/file.go
+++ b/file.go
@@ -10,4 +10,4 @@
 context10
 context11
-removed12
+added12
 context13
`
	es := NewEphemeralStore(defaultSessionTTL)
	defer es.Close()

	session, err := es.NewSession("test")
	if err != nil {
		t.Fatal(err)
	}


	count, err := ParseDiff(diff, session)
	if err != nil {
		t.Fatalf("ParseDiff() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 facts, got %d", count)
	}

	// old-start=10, context at 10,11 → oldLine=12 for removed
	verifyFactExists(t, session.Facts, "file.go", DiffRemoved, "12:removed12")
	// new-start=10, context at 10,11 → newLine=12 for added
	verifyFactExists(t, session.Facts, "file.go", DiffAdded, "12:added12")
	verifyFactExists(t, session.Facts, "file.go", DiffModified, "12:12:added12")
}

func TestParseDiff_NoTrailingNewline(t *testing.T) {
	diff := `--- a/f.go
+++ b/f.go
@@ -1,2 +1,2 @@
 a
-b
+c`
	es := NewEphemeralStore(defaultSessionTTL)
	defer es.Close()

	session, err := es.NewSession("test")
	if err != nil {
		t.Fatal(err)
	}


	count, err := ParseDiff(diff, session)
	if err != nil {
		t.Fatalf("ParseDiff() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 facts, got %d", count)
	}

	verifyFactExists(t, session.Facts, "f.go", DiffRemoved, "2:b")
	verifyFactExists(t, session.Facts, "f.go", DiffAdded, "2:c")
}

func TestParseDiff_AdjacentMultiRemovedAdded(t *testing.T) {
	diff := `--- a/f.go
+++ b/f.go
@@ -1,4 +1,4 @@
-rm1
-rm2
+add1
+add2
 keep
`
	es := NewEphemeralStore(defaultSessionTTL)
	defer es.Close()

	session, err := es.NewSession("test")
	if err != nil {
		t.Fatal(err)
	}


	count, err := ParseDiff(diff, session)
	if err != nil {
		t.Fatalf("ParseDiff() error = %v", err)
	}
	// 2 removals + 2 additions + 2 modifications = 6
	if count != 6 {
		t.Fatalf("expected 6 facts, got %d", count)
	}

	verifyFacts(t, session.Facts, map[string]int{
		DiffRemoved:  2,
		DiffAdded:    2,
		DiffModified: 2,
	})
	verifyFactExists(t, session.Facts, "f.go", DiffRemoved, "1:rm1")
	verifyFactExists(t, session.Facts, "f.go", DiffRemoved, "2:rm2")
	verifyFactExists(t, session.Facts, "f.go", DiffAdded, "1:add1")
	verifyFactExists(t, session.Facts, "f.go", DiffAdded, "2:add2")
	verifyFactExists(t, session.Facts, "f.go", DiffModified, "1:1:add1")
	verifyFactExists(t, session.Facts, "f.go", DiffModified, "2:2:add2")
}

func TestParseDiff_ModBufferResetOnContext(t *testing.T) {
	diff := `--- a/f.go
+++ b/f.go
@@ -1,5 +1,5 @@
-rm1
 context
+add1
`
	es := NewEphemeralStore(defaultSessionTTL)
	defer es.Close()

	session, err := es.NewSession("test")
	if err != nil {
		t.Fatal(err)
	}


	got, err := ParseDiff(diff, session)
	if err != nil {
		t.Fatalf("ParseDiff() error = %v", err)
	}
	// Only diff_removed + diff_added, no diff_modified because context reset the buffer
	if got != 2 {
		t.Fatalf("expected 2 facts (no modification), got %d", got)
	}

	verifyFacts(t, session.Facts, map[string]int{
		DiffRemoved: 1,
		DiffAdded:   1,
	})
}

// helpers

func verifyFacts(t *testing.T, store *meb.MEBStore, expected map[string]int) {
	t.Helper()
	ctx := context.Background()

	for predicate, want := range expected {
		query := fmt.Sprintf(`triples(S, "%s", O)`, predicate)
		results, err := gcamdb.Query(ctx, store, query)
		if err != nil {
			t.Fatalf("query %q error: %v", query, err)
		}
		if got := len(results); got != want {
			t.Errorf("predicate %q fact count = %d, want %d; results: %v", predicate, got, want, results)
		}
	}
}

func verifyFactExists(t *testing.T, store *meb.MEBStore, subject, predicate, object string) {
	t.Helper()
	ctx := context.Background()

	query := fmt.Sprintf(`triples(S, "%s", O), eq(S, "%s"), eq(O, "%s")`, predicate, subject, object)
	results, err := gcamdb.Query(ctx, store, query)
	if err != nil {
		t.Fatalf("query %q error: %v", query, err)
	}
	if len(results) == 0 {
		// Fallback: list all matching predicate to help debug
		allQuery := fmt.Sprintf(`triples(S, "%s", O)`, predicate)
		allResults, allErr := gcamdb.Query(ctx, store, allQuery)
		if allErr == nil {
			var strs []string
			for _, r := range allResults {
				s, _ := r["S"].(string)
				o, _ := r["O"].(string)
				strs = append(strs, fmt.Sprintf("%s: %s", s, o))
			}
			t.Errorf("expected fact (%s, %s, %q) not found. existing facts: %v", subject, predicate, object, strings.Join(strs, ", "))
		} else {
			t.Errorf("expected fact (%s, %s, %q) not found", subject, predicate, object)
		}
		return
	}

	var found bool
	for _, r := range results {
		s, _ := r["S"].(string)
		o, _ := r["O"].(string)
		if s == subject && o == object {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("fact (%s, %s, %q) not found in results: %v", subject, predicate, object, results)
	}

	// Also verify line number is correct
	objStr, _ := results[0]["O"].(string)
	parts := strings.SplitN(objStr, ":", 2)
	if len(parts) == 2 {
		_, err := strconv.Atoi(parts[0])
		if err != nil {
			t.Errorf("object %q has non-numeric line number prefix", objStr)
		}
	}
}
