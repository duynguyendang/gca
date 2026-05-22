package entity

import (
	"testing"

	"github.com/duynguyendang/gca/pkg/nlp/types"
)

type mockEntityStore struct {
	lookup  map[string]uint64
	scanRes [][3]string
}

func (m *mockEntityStore) LookupID(key string) (uint64, bool) {
	if id, ok := m.lookup[key]; ok {
		return id, true
	}
	return 0, false
}

func (m *mockEntityStore) ScanFacts(subject, predicate, object string) [][3]string {
	return m.scanRes
}

func (m *mockEntityStore) GetContentByKey(key string) ([]byte, error) {
	return nil, nil
}

func TestResolver_ResolveSymbol(t *testing.T) {
	store := &mockEntityStore{
		lookup: map[string]uint64{
			"auth.go:AuthService": 1,
		},
		scanRes: [][3]string{
			{"file:auth.go", "defines", "auth.go:AuthService"},
			{"file:auth.go", "defines", "auth.go:LoginHandler"},
		},
	}

	resolver := NewResolver(store)

	t.Run("exact match via LookupID", func(t *testing.T) {
		entity, found := resolver.ResolveSymbol("auth.go:AuthService")
		if !found {
			t.Fatal("expected to find symbol")
		}
		if entity.EntityType != "symbol" {
			t.Errorf("EntityType = %q, want %q", entity.EntityType, "symbol")
		}
	})

	t.Run("search via scan", func(t *testing.T) {
		entity, found := resolver.ResolveSymbol("AuthService")
		if !found {
			t.Fatal("expected to find symbol via search")
		}
		if entity.Name != "AuthService" {
			t.Errorf("Name = %q, want %q", entity.Name, "AuthService")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, found := resolver.ResolveSymbol("NonExistent")
		if found {
			t.Error("expected not to find non-existent symbol")
		}
	})
}

func TestResolver_ResolveFile(t *testing.T) {
	resolver := NewResolver(nil)

	tests := []struct {
		path     string
		expected bool
	}{
		{"auth/service.go", true},
		{"main.go", true},
		{"utils.go", true},
		{"internal/pkg/auth.go", true},
		{"AuthService", false},
		{"auth", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			_, found := resolver.ResolveFile(tt.path)
			if found != tt.expected {
				t.Errorf("ResolveFile(%q) = %v, want %v", tt.path, found, tt.expected)
			}
		})
	}
}

func TestResolver_ResolvePackage(t *testing.T) {
	resolver := NewResolver(nil)

	tests := []struct {
		name     string
		expected bool
	}{
		{"auth/service", true},
		{"internal/pkg", true},
		{"main.go", false},
		{"AuthService", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, found := resolver.ResolvePackage(tt.name)
			if found != tt.expected {
				t.Errorf("ResolvePackage(%q) = %v, want %v", tt.name, found, tt.expected)
			}
		})
	}
}

func TestResolver_Resolve(t *testing.T) {
	store := &mockEntityStore{
		lookup: map[string]uint64{},
		scanRes: [][3]string{
			{"file:auth.go", "defines", "auth.go:AuthService"},
		},
	}

	resolver := NewResolver(store)
	ctx := []*types.Entity{{Name: "AuthService", EntityType: "symbol"}}

	t.Run("resolves with history context", func(t *testing.T) {
		entities, err := resolver.Resolve("AuthService", ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entities) == 0 {
			t.Error("expected at least one entity")
		}
	})

	t.Run("caches results", func(t *testing.T) {
		_, err := resolver.Resolve("AuthService", ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		resolver.cacheMu.RLock()
		cached, ok := resolver.cache["AuthService"]
		resolver.cacheMu.RUnlock()

		if !ok {
			t.Error("expected results to be cached")
		}
		if len(cached) == 0 {
			t.Error("cached results should not be empty")
		}
	})
}