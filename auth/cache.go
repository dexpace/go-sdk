// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package auth

import "sync"

// TokenCache stores access tokens keyed by an opaque key (the SDK uses the
// space-joined scope set). Implementations must be safe for concurrent use.
type TokenCache interface {
	// Get returns the cached token for key and whether one was present.
	Get(key string) (AccessToken, bool)
	// Set stores token under key.
	Set(key string, token AccessToken)
}

// InMemoryTokenCache is a concurrency-safe in-memory [TokenCache].
type InMemoryTokenCache struct {
	mu     sync.Mutex
	tokens map[string]AccessToken
}

// NewInMemoryTokenCache returns an empty in-memory cache.
func NewInMemoryTokenCache() *InMemoryTokenCache {
	return &InMemoryTokenCache{tokens: make(map[string]AccessToken)}
}

// Get implements [TokenCache].
func (c *InMemoryTokenCache) Get(key string) (AccessToken, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.tokens[key]
	return t, ok
}

// Set implements [TokenCache].
func (c *InMemoryTokenCache) Set(key string, token AccessToken) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokens[key] = token
}
