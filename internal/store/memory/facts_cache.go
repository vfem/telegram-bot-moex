package memory

import (
	"sync"
	"time"

	"telegram-bot-moex/internal/domain"
)

const (
	defaultFactsTTL    = 45 * time.Minute
	maxFactsCacheItems = 5000
)

type factsCacheEntry struct {
	facts      []domain.MaterialFact
	expiresAt  time.Time
	isNotFound bool
}

// FactsCache provides an in-memory, thread-safe cache for issuer material facts (TTL 45 minutes).
type FactsCache struct {
	mu         sync.RWMutex
	items      map[string]factsCacheEntry
	defaultTTL time.Duration
	maxEntries int
	nowFn      func() time.Time
}

// NewFactsCache creates a new in-memory facts cache with a 45-minute default TTL.
func NewFactsCache() *FactsCache {
	return &FactsCache{
		items:      make(map[string]factsCacheEntry),
		defaultTTL: defaultFactsTTL,
		maxEntries: maxFactsCacheItems,
		nowFn:      time.Now,
	}
}

// Get retrieves cached material facts for an INN if not expired.
func (c *FactsCache) Get(inn string) ([]domain.MaterialFact, bool) {
	c.mu.RLock()
	entry, exists := c.items[inn]
	if !exists {
		c.mu.RUnlock()
		return nil, false
	}

	if c.nowFn().After(entry.expiresAt) {
		c.mu.RUnlock()
		c.mu.Lock()
		if cur, ok := c.items[inn]; ok && c.nowFn().After(cur.expiresAt) {
			delete(c.items, inn)
		}
		c.mu.Unlock()
		return nil, false
	}
	c.mu.RUnlock()

	if entry.isNotFound {
		return nil, false
	}

	// Copy-on-read to prevent external slice mutation
	cp := make([]domain.MaterialFact, len(entry.facts))
	copy(cp, entry.facts)
	return cp, true
}

// Set stores material facts in cache with the default TTL (45 minutes).
func (c *FactsCache) Set(inn string, facts []domain.MaterialFact) {
	c.SetWithTTL(inn, facts, c.defaultTTL)
}

// SetWithTTL stores material facts with an explicit TTL.
func (c *FactsCache) SetWithTTL(inn string, facts []domain.MaterialFact, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ensureCapacityLocked()

	cp := make([]domain.MaterialFact, len(facts))
	copy(cp, facts)

	c.items[inn] = factsCacheEntry{
		facts:      cp,
		expiresAt:  c.nowFn().Add(ttl),
		isNotFound: false,
	}
}

// SetNotFound caches a negative lookup result to protect upstream feeds from repeated queries.
func (c *FactsCache) SetNotFound(inn string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ensureCapacityLocked()

	c.items[inn] = factsCacheEntry{
		facts:      nil,
		expiresAt:  c.nowFn().Add(ttl),
		isNotFound: true,
	}
}

// IsNotFound checks whether an INN is cached as having no material facts.
func (c *FactsCache) IsNotFound(inn string) bool {
	c.mu.RLock()
	entry, exists := c.items[inn]
	if !exists {
		c.mu.RUnlock()
		return false
	}

	if c.nowFn().After(entry.expiresAt) {
		c.mu.RUnlock()
		c.mu.Lock()
		if cur, ok := c.items[inn]; ok && c.nowFn().After(cur.expiresAt) {
			delete(c.items, inn)
		}
		c.mu.Unlock()
		return false
	}
	c.mu.RUnlock()

	return entry.isNotFound
}

func (c *FactsCache) ensureCapacityLocked() {
	if len(c.items) < c.maxEntries {
		return
	}
	now := c.nowFn()
	for k, v := range c.items {
		if now.After(v.expiresAt) {
			delete(c.items, k)
		}
	}
	if len(c.items) >= c.maxEntries {
		for k := range c.items {
			delete(c.items, k)
			break
		}
	}
}
