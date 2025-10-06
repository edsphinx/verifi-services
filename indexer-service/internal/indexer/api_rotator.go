package indexer

import (
	"sync"
	"time"
)

// APIKeyRotator manages rotation between multiple API keys to avoid rate limits
type APIKeyRotator struct {
	aptosKeys  []string
	noditKeys  []string
	currentIdx int
	mu         sync.Mutex
	lastUsed   map[string]time.Time
	minDelay   time.Duration
}

// NewAPIKeyRotator creates a new API key rotator
func NewAPIKeyRotator(aptosKeys, noditKeys []string) *APIKeyRotator {
	return &APIKeyRotator{
		aptosKeys:  aptosKeys,
		noditKeys:  noditKeys,
		currentIdx: 0,
		lastUsed:   make(map[string]time.Time),
		minDelay:   100 * time.Millisecond, // Minimum delay between uses of same key
	}
}

// GetNextAptosKey returns the next Aptos API key in rotation
func (r *APIKeyRotator) GetNextAptosKey() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.aptosKeys) == 0 {
		return ""
	}

	// Round-robin through keys
	key := r.aptosKeys[r.currentIdx%len(r.aptosKeys)]

	// Wait if this key was used too recently
	if lastTime, exists := r.lastUsed[key]; exists {
		elapsed := time.Since(lastTime)
		if elapsed < r.minDelay {
			time.Sleep(r.minDelay - elapsed)
		}
	}

	r.lastUsed[key] = time.Now()
	r.currentIdx++

	return key
}

// GetNextNoditKey returns the next Nodit API key in rotation
func (r *APIKeyRotator) GetNextNoditKey() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.noditKeys) == 0 {
		return ""
	}

	// Round-robin through keys
	key := r.noditKeys[r.currentIdx%len(r.noditKeys)]

	// Wait if this key was used too recently
	if lastTime, exists := r.lastUsed[key]; exists {
		elapsed := time.Since(lastTime)
		if elapsed < r.minDelay {
			time.Sleep(r.minDelay - elapsed)
		}
	}

	r.lastUsed[key] = time.Now()

	return key
}

// GetStats returns usage statistics for monitoring
func (r *APIKeyRotator) GetStats() map[string]interface{} {
	r.mu.Lock()
	defer r.mu.Unlock()

	return map[string]interface{}{
		"aptos_keys_count": len(r.aptosKeys),
		"nodit_keys_count": len(r.noditKeys),
		"total_rotations":  r.currentIdx,
		"last_used_count":  len(r.lastUsed),
	}
}
