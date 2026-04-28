package ephemeral

import (
	"testing"
	"time"
)

func TestNewEphemeralStore(t *testing.T) {
	es := NewEphemeralStore(0)
	if es == nil {
		t.Fatal("NewEphemeralStore should not return nil")
	}
	if es.defaultTTL != defaultSessionTTL {
		t.Errorf("default TTL = %v, want %v", es.defaultTTL, defaultSessionTTL)
	}
	if es.sessions == nil {
		t.Error("sessions map should be initialized")
	}
	es.Close()
}

func TestNewEphemeralStore_CustomTTL(t *testing.T) {
	customTTL := 1 * time.Hour
	es := NewEphemeralStore(customTTL)
	defer es.Close()

	if es.defaultTTL != customTTL {
		t.Errorf("defaultTTL = %v, want %v", es.defaultTTL, customTTL)
	}
}

func TestNewEphemeralStore_NegativeTTL(t *testing.T) {
	es := NewEphemeralStore(-1 * time.Hour)
	defer es.Close()

	if es.defaultTTL != defaultSessionTTL {
		t.Errorf("negative TTL should fall back to default, got %v, want %v", es.defaultTTL, defaultSessionTTL)
	}
}

func TestNewSession(t *testing.T) {
	es := NewEphemeralStore(defaultSessionTTL)
	defer es.Close()

	session, err := es.NewSession("test-project")
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	if session == nil {
		t.Fatal("NewSession should not return nil")
	}
	if session.ID == "" {
		t.Error("session.ID should not be empty")
	}
	if session.ProjectID != "test-project" {
		t.Errorf("session.ProjectID = %q, want %q", session.ProjectID, "test-project")
	}
	if session.Facts == nil {
		t.Error("session.Facts should not be nil")
	}
	if session.ExpiresAt.IsZero() {
		t.Error("session.ExpiresAt should not be zero")
	}
}

func TestNewSession_EmptyProjectID(t *testing.T) {
	es := NewEphemeralStore(defaultSessionTTL)
	defer es.Close()

	_, err := es.NewSession("")
	if err == nil {
		t.Error("NewSession with empty projectID should return error")
	}
}

func TestNewSession_MaxSessionsLimit(t *testing.T) {
	// Use a small max for testing
	// We can't easily change maxSessions, so just verify the limit check exists
	es := NewEphemeralStore(defaultSessionTTL)
	defer es.Close()

	// Create maxSessions to hit the limit
	for i := 0; i < maxSessions; i++ {
		_, err := es.NewSession("test-project")
		if err != nil {
			t.Fatalf("NewSession(%d) error = %v", i, err)
		}
	}

	// Next one should fail
	_, err := es.NewSession("another-project")
	if err == nil {
		t.Error("NewSession should return error when max sessions reached")
	}
}

func TestGetSession(t *testing.T) {
	es := NewEphemeralStore(defaultSessionTTL)
	defer es.Close()

	session, _ := es.NewSession("test-project")

	got, err := es.GetSession(session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if got.ID != session.ID {
		t.Errorf("GetSession() = %v, want %v", got.ID, session.ID)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	es := NewEphemeralStore(defaultSessionTTL)
	defer es.Close()

	_, err := es.GetSession("non-existent-id")
	if err == nil {
		t.Error("GetSession for non-existent ID should return error")
	}
}

func TestGetSession_Expired(t *testing.T) {
	es := NewEphemeralStore(1 * time.Millisecond) // Very short TTL
	defer es.Close()

	session, _ := es.NewSession("test-project")

	// Wait for expiration
	time.Sleep(10 * time.Millisecond)

	_, err := es.GetSession(session.ID)
	if err != ErrSessionExpired {
		t.Errorf("GetSession() error = %v, want %v", err, ErrSessionExpired)
	}
}

func TestSession_Close(t *testing.T) {
	es := NewEphemeralStore(defaultSessionTTL)
	session, _ := es.NewSession("test-project")

	err := session.Close()
	if err != nil {
		t.Fatalf("Session.Close() error = %v", err)
	}
}

func TestEphemeralStore_Close(t *testing.T) {
	es := NewEphemeralStore(defaultSessionTTL)
	es.NewSession("project-1")
	es.NewSession("project-2")

	es.Close()

	if len(es.sessions) != 0 {
		t.Errorf("After Close(), sessions count = %d, want 0", len(es.sessions))
	}
}

func TestHashToTopicID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantTop bool // should have ephemeral bit set
	}{
		{"non-empty string", "test-project", true},
		{"another", "another", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hashToTopicID(tt.input)
			if tt.wantTop && (got&topicIDEphemeralBit) == 0 {
				t.Errorf("hashToTopicID(%q) = %d, want ephemeral bit set", tt.input, got)
			}
			if !tt.wantTop && (got&topicIDEphemeralBit) != 0 {
				t.Errorf("hashToTopicID(%q) = %d, want ephemeral bit clear", tt.input, got)
			}
		})
	}
}

func TestHashToTopicID_EmptyString(t *testing.T) {
	// Empty string is a special case that returns 1 without the ephemeral bit
	got := hashToTopicID("")
	if got != 1 {
		t.Errorf("hashToTopicID(\"\") = %d, want 1", got)
	}
	if (got & topicIDEphemeralBit) != 0 {
		t.Errorf("hashToTopicID(\"\") should NOT have ephemeral bit set")
	}
}

func TestHashToTopicID_Deterministic(t *testing.T) {
	input := "test-project"
	first := hashToTopicID(input)
	second := hashToTopicID(input)

	if first != second {
		t.Errorf("hashToTopicID should be deterministic: first=%d, second=%d", first, second)
	}
}

func TestHashToTopicID_DifferentInputs(t *testing.T) {
	id1 := hashToTopicID("project-a")
	id2 := hashToTopicID("project-b")

	if id1 == id2 {
		t.Errorf("Different inputs should produce different IDs: %d vs %d", id1, id2)
	}
}
