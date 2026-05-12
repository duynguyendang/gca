package export

import (
	"encoding/json"
	"testing"
)

func TestPaginateGraph(t *testing.T) {
	nodes := make([]D3Node, 10)
	links := []D3Link{
		{Source: "0", Target: "1"},
		{Source: "1", Target: "2"},
		{Source: "5", Target: "6"},
	}
	for i := range nodes {
		nodes[i] = D3Node{ID: string(rune('0' + i)), Name: string(rune('A' + i))}
	}

	g := &D3Graph{Nodes: nodes, Links: links}

	tests := []struct {
		name      string
		opts      GraphPageOptions
		wantNodes int
		wantStart int
		wantEnd   int
		wantMore  bool
	}{
		{
			name:      "first page limit 3",
			opts:      GraphPageOptions{Limit: 3, Offset: 0},
			wantNodes: 3,
			wantStart: 0,
			wantEnd:   3,
			wantMore:  true,
		},
		{
			name:      "second page limit 3",
			opts:      GraphPageOptions{Limit: 3, Offset: 3},
			wantNodes: 3,
			wantStart: 3,
			wantEnd:   6,
			wantMore:  true,
		},
		{
			name:      "partial last page",
			opts:      GraphPageOptions{Limit: 3, Offset: 9},
			wantNodes: 1,
			wantStart: 9,
			wantEnd:   10,
			wantMore:  false,
		},
		{
			name:      "empty page beyond range",
			opts:      GraphPageOptions{Limit: 3, Offset: 15},
			wantNodes: 0,
			wantStart: 15,
			wantEnd:   15,
			wantMore:  false,
		},
		{
			name:      "default limit applies",
			opts:      GraphPageOptions{Limit: 0, Offset: 0},
			wantNodes: 10,
			wantStart: 0,
			wantEnd:   10,
			wantMore:  false,
		},
		{
			name:      "limit capped at 1000",
			opts:      GraphPageOptions{Limit: 2000, Offset: 0},
			wantNodes: 10,
			wantStart: 0,
			wantEnd:   10,
			wantMore:  false,
		},
		{
			name:      "negative offset clamped to 0",
			opts:      GraphPageOptions{Limit: 3, Offset: -5},
			wantNodes: 3,
			wantStart: 0,
			wantEnd:   3,
			wantMore:  true,
		},
		{
			name:      "exact fit pages",
			opts:      GraphPageOptions{Limit: 5, Offset: 0},
			wantNodes: 5,
			wantStart: 0,
			wantEnd:   5,
			wantMore:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, nextCursor := g.PaginateGraph(tt.opts)
			if len(result.Nodes) != tt.wantNodes {
				t.Errorf("Nodes len = %d, want %d", len(result.Nodes), tt.wantNodes)
			}
			if tt.wantNodes > 0 {
				startID := result.Nodes[0].ID
				endID := result.Nodes[len(result.Nodes)-1].ID
				t.Logf("Page IDs: %s to %s", startID, endID)
			}
			if result.HasMore != tt.wantMore {
				t.Errorf("HasMore = %v, want %v", result.HasMore, tt.wantMore)
			}
			if tt.wantMore && nextCursor == "" {
				t.Error("expected nextCursor but got empty")
			}
			if !tt.wantMore && nextCursor != "" {
				t.Errorf("expected no nextCursor but got %q", nextCursor)
			}
			if tt.wantMore {
				var cursor GraphCursor
				if err := json.Unmarshal([]byte(nextCursor), &cursor); err != nil {
					t.Fatalf("nextCursor is not valid JSON: %v", err)
				}
				if cursor.Limit != tt.opts.Limit {
					t.Errorf("cursor.Limit = %d, want %d", cursor.Limit, tt.opts.Limit)
				}
			}
		})
	}
}

func TestPaginateGraph_LinkFiltering(t *testing.T) {
	nodes := []D3Node{
		{ID: "0", Name: "A"},
		{ID: "1", Name: "B"},
		{ID: "2", Name: "C"},
	}
	links := []D3Link{
		{Source: "0", Target: "1"}, // within first page
		{Source: "1", Target: "2"}, // spans first to second page
		{Source: "2", Target: "0"}, // in second page
	}

	g := &D3Graph{Nodes: nodes, Links: links}

	// Page 0: nodes 0,1 — only link 0->1 is fully contained
	result, _ := g.PaginateGraph(GraphPageOptions{Limit: 2, Offset: 0})
	if len(result.Links) != 1 {
		t.Errorf("Page 0 links = %d, want 1 (only 0->1)", len(result.Links))
	}
	if len(result.Links) > 0 && result.Links[0].Source != "0" {
		t.Errorf("Page 0 link source = %q, want 0", result.Links[0].Source)
	}

	// Page 1: nodes 2 — no links (node 2's links reference nodes outside page)
	result2, _ := g.PaginateGraph(GraphPageOptions{Limit: 2, Offset: 2})
	if len(result2.Links) != 0 {
		t.Errorf("Page 1 links = %d, want 0", len(result2.Links))
	}
}

func TestParseCursor(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantOff int
		wantLim int
		wantErr bool
	}{
		{
			name:    "empty returns default",
			input:   "",
			wantOff: 0,
			wantLim: 100,
			wantErr: false,
		},
		{
			name:    "valid cursor",
			input:   `{"offset":50,"limit":25}`,
			wantOff: 50,
			wantLim: 25,
			wantErr: false,
		},
		{
			name:    "zero values",
			input:   `{"offset":0,"limit":0}`,
			wantOff: 0,
			wantLim: 0,
			wantErr: false,
		},
		{
			name:    "invalid json",
			input:   `{invalid}`,
			wantErr: true,
		},
		{
			name:    "not json object",
			input:   `"just a string"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cursor, err := ParseCursor(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCursor(%q) err = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if cursor.Offset != tt.wantOff {
					t.Errorf("cursor.Offset = %d, want %d", cursor.Offset, tt.wantOff)
				}
				if cursor.Limit != tt.wantLim {
					t.Errorf("cursor.Limit = %d, want %d", cursor.Limit, tt.wantLim)
				}
			}
		})
	}
}

func TestPaginateGraph_EmptyGraph(t *testing.T) {
	g := &D3Graph{Nodes: []D3Node{}, Links: []D3Link{}}
	result, nextCursor := g.PaginateGraph(GraphPageOptions{Limit: 10, Offset: 0})
	if len(result.Nodes) != 0 {
		t.Errorf("Nodes len = %d, want 0", len(result.Nodes))
	}
	if result.HasMore {
		t.Errorf("HasMore = true, want false for empty graph")
	}
	if nextCursor != "" {
		t.Errorf("nextCursor = %q, want empty", nextCursor)
	}
}