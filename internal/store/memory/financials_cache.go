package memory

import (
	"sync"
	"time"

	"telegram-bot-moex/internal/domain"
)

const (
	defaultFinancialsTTL = 30 * 24 * time.Hour
	maxFinancialsEntries = 5000
)

type financialsItem struct {
	balance    *domain.IssuerBalance
	expiresAt  time.Time
	isNotFound bool
}

// FinancialsCache provides an in-memory, thread-safe cache for annual financial statements.
type FinancialsCache struct {
	mu         sync.RWMutex
	items      map[string]financialsItem
	defaultTTL time.Duration
	maxEntries int
	nowFn      func() time.Time
}

// NewFinancialsCache creates a new in-memory financials cache with 30-day TTL.
func NewFinancialsCache() *FinancialsCache {
	return &FinancialsCache{
		items:      make(map[string]financialsItem),
		defaultTTL: defaultFinancialsTTL,
		maxEntries: maxFinancialsEntries,
		nowFn:      time.Now,
	}
}

// Get retrieves cached financial statement for an INN if not expired.
func (c *FinancialsCache) Get(inn string) (*domain.IssuerBalance, bool) {
	c.mu.RLock()
	item, exists := c.items[inn]
	if !exists {
		c.mu.RUnlock()
		return nil, false
	}

	if c.nowFn().After(item.expiresAt) {
		c.mu.RUnlock()
		// Safe lazy eviction: re-check condition under write lock
		c.mu.Lock()
		if cur, ok := c.items[inn]; ok && c.nowFn().After(cur.expiresAt) {
			delete(c.items, inn)
		}
		c.mu.Unlock()
		return nil, false
	}
	c.mu.RUnlock()

	if item.isNotFound || item.balance == nil {
		return nil, false
	}

	// Copy-on-read to prevent mutation of cached data
	cp := *item.balance
	return &cp, true
}

// Set stores financial statements in cache with the default 30-day TTL.
func (c *FinancialsCache) Set(inn string, balance *domain.IssuerBalance) {
	c.SetWithTTL(inn, balance, c.defaultTTL)
}

// SetWithTTL stores financial statements with an explicit TTL and bounds memory usage.
func (c *FinancialsCache) SetWithTTL(inn string, balance *domain.IssuerBalance, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ensureCapacityLocked()

	// Copy-on-write: store a copy so external callers mutating their struct do not mutate cache
	var cp *domain.IssuerBalance
	if balance != nil {
		bCopy := *balance
		cp = &bCopy
	}

	c.items[inn] = financialsItem{
		balance:    cp,
		expiresAt:  c.nowFn().Add(ttl),
		isNotFound: false,
	}
}

// SetNotFound caches a negative lookup result to protect upstream services from spam.
func (c *FinancialsCache) SetNotFound(inn string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ensureCapacityLocked()

	c.items[inn] = financialsItem{
		balance:    nil,
		expiresAt:  c.nowFn().Add(ttl),
		isNotFound: true,
	}
}

// IsNotFound checks whether an INN is cached as not existing / not found.
func (c *FinancialsCache) IsNotFound(inn string) bool {
	c.mu.RLock()
	item, exists := c.items[inn]
	if !exists {
		c.mu.RUnlock()
		return false
	}

	if c.nowFn().After(item.expiresAt) {
		c.mu.RUnlock()
		c.mu.Lock()
		if cur, ok := c.items[inn]; ok && c.nowFn().After(cur.expiresAt) {
			delete(c.items, inn)
		}
		c.mu.Unlock()
		return false
	}
	c.mu.RUnlock()

	return item.isNotFound
}

func (c *FinancialsCache) ensureCapacityLocked() {
	if len(c.items) < c.maxEntries {
		return
	}
	now := c.nowFn()
	for k, v := range c.items {
		if now.After(v.expiresAt) {
			delete(c.items, k)
		}
	}
	// If still at capacity, evict an arbitrary entry
	if len(c.items) >= c.maxEntries {
		for k := range c.items {
			delete(c.items, k)
			break
		}
	}
}
