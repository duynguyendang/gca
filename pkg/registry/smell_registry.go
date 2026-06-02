package registry

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/duynguyendang/gca/pkg/logger"
	mebpkg "github.com/duynguyendang/gca/pkg/meb"
	externmeb "github.com/duynguyendang/meb"
)

// SmellRegistry reads smell metadata from the Analytical Store.
// It replaces the hardcoded smell weight map in handlers.go.
type SmellRegistry struct {
	storeManager interface {
		GetAnalyticalStore(projectID string) (*externmeb.MEBStore, error)
	}
	mu              sync.RWMutex
	weights         map[string]int    // smell_name -> weight
	securityPrefixes map[string]bool  // prefixes that indicate security smells
	loadedAt        time.Time
	ttl             time.Duration
}

// NewSmellRegistry creates a new SmellRegistry.
func NewSmellRegistry(storeManager interface {
	GetAnalyticalStore(projectID string) (*externmeb.MEBStore, error)
}) *SmellRegistry {
	return &SmellRegistry{
		storeManager:    storeManager,
		weights:         make(map[string]int),
		securityPrefixes: make(map[string]bool),
		ttl:             5 * time.Minute,
	}
}

// LoadFromPolicies reads smell_weight/2 facts from the Analytical Store.
// It should be called at startup and can be reloaded periodically.
func (sr *SmellRegistry) LoadFromPolicies(ctx context.Context, projectID string) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	store, err := sr.storeManager.GetAnalyticalStore(projectID)
	if err != nil {
		logger.Warn("SmellRegistry: failed to get analytical store", "error", err)
		return err
	}

	weights := make(map[string]int)
	securityPrefixes := make(map[string]bool)

	// Query smell_weight/2 facts: triples(Name, "smell_weight", Weight)
	query := `triples(Name, "smell_weight", Weight)`
	results, err := mebpkg.Query(ctx, store, query)
	if err != nil {
		logger.Warn("SmellRegistry: failed to query smell_weight", "error", err)
		return err
	}

	for _, r := range results {
		name, _ := r["Name"].(string)
		weightStr, _ := r["Weight"].(string)
		if name == "" || weightStr == "" {
			continue
		}
		var weight int
		if _, err := fmt.Sscanf(weightStr, "%d", &weight); err != nil {
			continue
		}
		weights[name] = weight
	}

	// Query smell category facts to identify security smells
	catQuery := `triples(Name, "category", "security")`
	catResults, err := mebpkg.Query(ctx, store, catQuery)
	if err == nil {
		for _, r := range catResults {
			name, _ := r["Name"].(string)
			if name != "" {
				securityPrefixes[name] = true
			}
		}
	}

	sr.weights = weights
	sr.securityPrefixes = securityPrefixes
	sr.loadedAt = time.Now()

	logger.Info("SmellRegistry loaded", "weights", len(weights), "security_prefixes", len(securityPrefixes))
	return nil
}

// Weight returns the weight for a smell name, with prefix matching.
func (sr *SmellRegistry) Weight(smellName string) (int, bool) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	if len(sr.weights) == 0 {
		return 0, false
	}

	// Exact match first
	if w, ok := sr.weights[smellName]; ok {
		return w, true
	}

	// Prefix match
	for prefix, w := range sr.weights {
		if strings.HasPrefix(smellName, prefix) {
			return w, true
		}
	}

	return 0, false
}

// IsSecurity checks if a smell name matches a security category.
func (sr *SmellRegistry) IsSecurity(smellName string) bool {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	if len(sr.securityPrefixes) == 0 {
		// Fallback: use known prefixes if no category facts
		for prefix := range sr.securityPrefixes {
			if strings.HasPrefix(smellName, prefix) {
				return true
			}
		}
		return false
	}

	for prefix := range sr.securityPrefixes {
		if strings.HasPrefix(smellName, prefix) {
			return true
		}
	}
	return false
}

// DefaultWeight returns the default weight for unknown smells.
func (sr *SmellRegistry) DefaultWeight() int {
	return 2
}
