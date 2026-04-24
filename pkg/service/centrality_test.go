package service

import (
	"testing"
)

func TestSortByCentralityDesc(t *testing.T) {
	symbols := []string{"a", "b", "c", "d"}
	centrality := map[string]float64{
		"a": 0.1,
		"b": 0.5,
		"c": 0.3,
		"d": 0.2,
	}

	sortByCentralityDesc(symbols, centrality)

	expected := []string{"b", "c", "d", "a"}
	for i, sym := range symbols {
		if sym != expected[i] {
			t.Errorf("symbols[%d] = %q, want %q (centrality: %v)", i, sym, expected[i], centrality[sym])
		}
	}
}

func TestSortByCentralityDesc_Empty(t *testing.T) {
	symbols := []string{}
	centrality := map[string]float64{}

	sortByCentralityDesc(symbols, centrality)

	if len(symbols) != 0 {
		t.Errorf("Empty slice should remain empty")
	}
}

func TestSortByCentralityDesc_SingleElement(t *testing.T) {
	symbols := []string{"only"}
	centrality := map[string]float64{"only": 1.0}

	sortByCentralityDesc(symbols, centrality)

	if len(symbols) != 1 || symbols[0] != "only" {
		t.Errorf("Single element should remain unchanged")
	}
}

func TestSortByCentralityDesc_Ties(t *testing.T) {
	symbols := []string{"a", "b", "c"}
	centrality := map[string]float64{
		"a": 0.5,
		"b": 0.5,
		"c": 0.5,
	}

	sortByCentralityDesc(symbols, centrality)

	if len(symbols) != 3 {
		t.Errorf("All same centrality should maintain length")
	}
}

func TestSortByCentralityDesc_AllZeros(t *testing.T) {
	symbols := []string{"x", "y", "z"}
	centrality := map[string]float64{
		"x": 0,
		"y": 0,
		"z": 0,
	}

	sortByCentralityDesc(symbols, centrality)

	if len(symbols) != 3 {
		t.Errorf("All zeros should maintain length")
	}
}
