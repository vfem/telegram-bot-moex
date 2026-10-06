package memory

import (
	"sync"
	"time"

	"telegram-bot-moex/internal/domain"
)

type cacheItem struct {
	data      *domain.MarketData
	expiresAt time.Time
}

// MarketDataCache provides an in-memory, thread-safe cache for real-time bond market data.
type MarketDataCache struct {
	mu    sync.RWMutex
	items map[string]cacheItem
	nowFn func() time.Time
}

// NewMarketDataCache creates a new in-memory market data cache.
func NewMarketDataCache() *MarketDataCache {
	return &MarketDataCache{
		items: make(map[string]cacheItem),
		nowFn: time.Now,
	}
}

// Get retrieves cached market data for an ISIN if not expired.
func (c *MarketDataCache) Get(isin string) (*domain.MarketData, bool) {
	c.mu.RLock()
	item, exists := c.items[isin]
	if !exists {
		c.mu.RUnlock()
		return nil, false
	}

	if c.nowFn().After(item.expiresAt) {
		c.mu.RUnlock()
		// Lazy eviction
		c.mu.Lock()
		delete(c.items, isin)
		c.mu.Unlock()
		return nil, false
	}
	c.mu.RUnlock()

	// Return a copy to prevent mutation of cached data (C-13)
	if item.data == nil {
		return nil, false
	}
	cp := *item.data
	return &cp, true
}

// Set stores market data in cache using the adaptive TTL.
func (c *MarketDataCache) Set(isin string, data *domain.MarketData) {
	ttl := c.GetAdaptiveTTL(c.nowFn())
	c.SetWithTTL(isin, data, ttl)
}

// SetWithTTL stores market data with an explicit duration.
func (c *MarketDataCache) SetWithTTL(isin string, data *domain.MarketData, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[isin] = cacheItem{
		data:      data,
		expiresAt: c.nowFn().Add(ttl),
	}
}

// GetAdaptiveTTL returns 120s during Moscow trading hours (09:50-18:50 MSK on weekdays), or 60m off-hours.
func (c *MarketDataCache) GetAdaptiveTTL(now time.Time) time.Duration {
	if isMoscowTradingHours(now) {
		return 120 * time.Second
	}
	return 60 * time.Minute
}

func isMoscowTradingHours(now time.Time) bool {
	mskLoc := time.FixedZone("MSK", 3*3600)
	msk := now.In(mskLoc)

	weekday := msk.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return false
	}

	mins := msk.Hour()*60 + msk.Minute()
	// Covers morning/day session (09:50-18:50) and evening session (19:00-23:50 MSK)
	return mins >= (9*60+50) && mins <= (23*60+50)
}
