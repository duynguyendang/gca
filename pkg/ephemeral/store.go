package ephemeral

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/duynguyendang/gca/pkg/common"
	"github.com/duynguyendang/meb"
	"github.com/duynguyendang/meb/store"
	"github.com/oklog/ulid/v2"
)

// ErrSessionExpired is returned when a session has expired.
var ErrSessionExpired = errors.New("session expired")

// ErrSessionNotFound is returned when the session does not exist.
var ErrSessionNotFound = errors.New("session not found")

const (
	defaultSessionTTL   = 30 * time.Minute
	maxSessions         = 100
	sweepInterval       = 5 * time.Minute
	topicIDMask         = 0xFFFFFF
	topicIDEphemeralBit = 0x400000 // ephemeral sessions use bit 22 (distinct from analytical 0x800000)
)

// EphemeralStore manages RAM-only sessions for transient facts (PR diffs, active incidents).
type EphemeralStore struct {
	mu            sync.RWMutex
	sessions      map[string]*Session
	defaultTTL    time.Duration
	stopCh        chan struct{}
	stopped       bool
	stopOnce      sync.Once
	telemetrySink meb.TelemetrySink
}

// Session represents a temporary RAM-based store for diff facts.
// Note: Callers must call SetTopicID() on Facts if topic-scoped queries are needed.
// The TopicID is not set automatically in NewSession since the caller knows
// which partition (Source/Analytical) the session should target.
type Session struct {
	ID        string
	ProjectID string
	Facts     *meb.MEBStore // RAM-only in-memory store
	ExpiresAt time.Time
	createdAt time.Time // Time when session was created
}

// NewEphemeralStore creates a new EphemeralStore with background sweeper.
func NewEphemeralStore(ttl time.Duration) *EphemeralStore {
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	es := &EphemeralStore{
		sessions:   make(map[string]*Session),
		defaultTTL: ttl,
		stopCh:     make(chan struct{}),
	}
	go es.runSweeper()
	return es
}

// SetTelemetrySink sets the telemetry sink for ephemeral sessions.
// This should be called before creating any sessions.
func (es *EphemeralStore) SetTelemetrySink(sink meb.TelemetrySink) {
	es.mu.Lock()
	defer es.mu.Unlock()
	es.telemetrySink = sink
}

// NewSession creates a RAM-only MEB store for transient facts.
// The session is automatically assigned a TopicID based on the projectID.
// Callers should call SetTopicID() on Session.Facts if a different TopicID is needed
// (e.g., for analytical ephemeral sessions).
func (es *EphemeralStore) NewSession(projectID string) (*Session, error) {
	if projectID == "" {
		return nil, fmt.Errorf("projectID cannot be empty")
	}

	es.mu.Lock()
	defer es.mu.Unlock()

	if len(es.sessions) >= maxSessions {
		return nil, fmt.Errorf("max sessions limit reached (%d)", maxSessions)
	}

	cfg := &store.Config{
		InMemory:       true,
		BlockCacheSize: 64 << 20,
		IndexCacheSize: 64 << 20,
		Profile:        "Safe-Serving",
		EnableAutoGC:   false,
	}

	factsStore, err := meb.NewMEBStore(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create in-memory store: %w", err)
	}

	// Set TopicID for project-scoped queries
	topicID := hashToTopicID(projectID)
	factsStore.SetTopicID(topicID)

	// Register telemetry sink if configured
	if es.telemetrySink != nil {
		factsStore.RegisterTelemetrySink(es.telemetrySink)
		log.Printf("Registered telemetry sink for ephemeral session (projectID=%s, topicID=%d)", projectID, topicID)
	}

	session := &Session{
		ID:        ulid.Make().String(),
		ProjectID: projectID,
		Facts:     factsStore,
		ExpiresAt: time.Now().Add(es.defaultTTL),
		createdAt: time.Now(),
	}

	es.sessions[session.ID] = session
	return session, nil
}

// GetSession returns a session by ID.
// Returns ErrSessionExpired if the session has expired.
func (es *EphemeralStore) GetSession(id string) (*Session, error) {
	es.mu.RLock()
	defer es.mu.RUnlock()

	session, ok := es.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	if time.Now().After(session.ExpiresAt) {
		return nil, ErrSessionExpired
	}
	return session, nil
}

// DeleteSession removes a session by ID from the store (without closing).
func (es *EphemeralStore) DeleteSession(id string) {
	es.mu.Lock()
	defer es.mu.Unlock()
	delete(es.sessions, id)
}

// Close releases the memory for a session.
func (s *Session) Close() error {
	return s.Facts.Close()
}

// ExtendTTL refreshes the session expiry time.
func (s *Session) ExtendTTL(d time.Duration) {
	s.ExpiresAt = time.Now().Add(d)
}

// Close shuts down the EphemeralStore and releases all sessions.
// It is safe to call multiple times.
func (es *EphemeralStore) Close() {
	es.stopOnce.Do(func() {
		es.mu.Lock()
		if !es.stopped {
			es.stopped = true
			close(es.stopCh)
		}
		es.mu.Unlock()
		for _, session := range es.sessions {
			_ = session.Facts.Close()
		}
		es.mu.Lock()
		es.sessions = make(map[string]*Session)
		es.mu.Unlock()
	})
}

// runSweeper periodically cleans up expired sessions.
func (es *EphemeralStore) runSweeper() {
	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()

	for {
		es.mu.RLock()
		stopped := es.stopped
		es.mu.RUnlock()
		if stopped {
			return
		}
		select {
		case <-es.stopCh:
			return
		case <-ticker.C:
			es.cleanupExpired()
		}
	}
}

// cleanupExpired removes expired sessions.
func (es *EphemeralStore) cleanupExpired() {
	es.mu.Lock()
	defer es.mu.Unlock()

	now := time.Now()
	for id, session := range es.sessions {
		if now.After(session.ExpiresAt) {
			session.Facts.Close()
			delete(es.sessions, id)
		}
	}
}

// hashToTopicID generates a deterministic 24-bit topic ID for ephemeral sessions.
// Uses bit 22 to distinguish from Source (high bit clear) and Analytical (high bit set) partitions.
func hashToTopicID(name string) uint32 {
	if name == "" {
		return 1
	}
	h := common.FNV1aHash(name)
	return (h & topicIDMask) | topicIDEphemeralBit
}
