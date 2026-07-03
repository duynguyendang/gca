package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)
// cacheResponse caches an AI response for a given query (LRU)
func (s *AIService) cacheResponse(cacheKey string, answer, summary string) {
	s.responseCacheMu.Lock()
	defer s.responseCacheMu.Unlock()

	// Evict oldest if at capacity
	if s.responseCacheList.Len() >= s.responseCacheMaxSize {
		if oldest := s.responseCacheList.Back(); oldest != nil {
			delete(s.responseCache, oldest.Value.(string))
			s.responseCacheList.Remove(oldest)
		}
	}

	// Add new entry to front of list
	e := s.responseCacheList.PushFront(cacheKey)
	s.responseCache[cacheKey] = &cachedResponse{
		Answer:  answer,
		Summary: summary,
		Time:    time.Now(),
		e:       e,
	}
}

// getCachedResponse retrieves a cached response if valid (LRU promotion)
func (s *AIService) getCachedResponse(cacheKey string) (string, string, bool) {
	s.responseCacheMu.RLock()
	cached, ok := s.responseCache[cacheKey]
	s.responseCacheMu.RUnlock()

	if !ok {
		return "", "", false
	}

	if time.Since(cached.Time) >= s.responseCacheTTL {
		return "", "", false
	}

	// Promote to front (LRU)
	s.responseCacheMu.Lock()
	if cached.e != nil {
		s.responseCacheList.MoveToFront(cached.e)
	}
	s.responseCacheMu.Unlock()

	return cached.Answer, cached.Summary, true
}

// generateCacheKey creates a deterministic cache key from query + results hash
func (s *AIService) generateCacheKey(query string, intent Intent, results interface{}) string {
	data := fmt.Sprintf("%s|%s|%v", query, intent, results)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// cleanupExpiredCache removes expired cache entries and enforces max size (LRU)
func (s *AIService) cleanupExpiredCache() {
	s.responseCacheMu.Lock()
	defer s.responseCacheMu.Unlock()
	now := time.Now()

	// Remove expired entries (traverse list from back = oldest)
	for e := s.responseCacheList.Back(); e != nil; e = e.Prev() {
		key := e.Value.(string)
		cached, ok := s.responseCache[key]
		if !ok {
			continue
		}
		if now.Sub(cached.Time) >= s.responseCacheTTL {
			delete(s.responseCache, key)
			s.responseCacheList.Remove(e)
		}
	}

	// Enforce max size (remove oldest from back if still over limit)
	for s.responseCacheList.Len() > s.responseCacheMaxSize {
		if oldest := s.responseCacheList.Back(); oldest != nil {
			delete(s.responseCache, oldest.Value.(string))
			s.responseCacheList.Remove(oldest)
		} else {
			break
		}
	}
}
