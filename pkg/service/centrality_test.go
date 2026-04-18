package service

import (
	"testing"
)

func TestCentralityServiceNew(t *testing.T) {
	cs := NewCentralityService()
	if cs == nil {
		t.Fatal("NewCentralityService returned nil")
	}
	if cs.cache == nil {
		t.Error("cache is nil")
	}
	if cs.ttl == 0 {
		t.Error("ttl should not be zero")
	}
}

func TestCentralityResult(t *testing.T) {
	result := CentralityResult{
		SymbolID:   "pkg/main.go:main",
		Centrality: 0.75,
		InDegree:   5,
		OutDegree:  10,
		Kind:       "function",
		IsEntry:    true,
	}

	if result.SymbolID != "pkg/main.go:main" {
		t.Errorf("SymbolID = %q, want %q", result.SymbolID, "pkg/main.go:main")
	}
	if result.Centrality != 0.75 {
		t.Errorf("Centrality = %v, want %v", result.Centrality, 0.75)
	}
	if result.InDegree != 5 {
		t.Errorf("InDegree = %d, want %d", result.InDegree, 5)
	}
	if result.OutDegree != 10 {
		t.Errorf("OutDegree = %d, want %d", result.OutDegree, 10)
	}
	if result.Kind != "function" {
		t.Errorf("Kind = %q, want %q", result.Kind, "function")
	}
	if !result.IsEntry {
		t.Error("IsEntry should be true")
	}
}

func TestIsInterfacePattern(t *testing.T) {
	tests := []struct {
		name   string
		symbol string
		want   bool
	}{
		{"interface keyword", "HandlerInterface", true},  // contains "interface"
		{"handler keyword", "MyHandler", true},            // contains "handler"
		{"service keyword", "UserService", true},         // contains "service"
		{"repository keyword", "DataRepository", true},  // contains "repository"
		{"controller keyword", "HomeController", true},    // contains "controller"
		{"no match", "MyHandler", true},                  // contains handler
		{"IConnection matches client", "IClient", true}, // contains "client"
		{"lowercase not prefix match", "lowercase", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsInterfacePattern(tt.symbol)
			if got != tt.want {
				t.Errorf("IsInterfacePattern(%q) = %v, want %v", tt.symbol, got, tt.want)
			}
		})
	}
}

func TestNormalizeCentrality(t *testing.T) {
	tests := []struct {
		name   string
		scores map[string]float64
		wantLen int
	}{
		{
			name:   "normal scores",
			scores: map[string]float64{"a": 1.0, "b": 2.0, "c": 3.0},
			wantLen: 3,
		},
		{
			name:   "empty scores",
			scores: map[string]float64{},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeCentrality(tt.scores)
			if len(got) != tt.wantLen {
				t.Errorf("NormalizeCentrality len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}
