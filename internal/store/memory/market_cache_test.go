package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"telegram-bot-moex/internal/domain"
)

func TestMarketDataCache_GetSetExpiration(t *testing.T) {
	cache := NewMarketDataCache()
	fixedTime := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC) // 15:00 MSK (trading hours)
	cache.nowFn = func() time.Time { return fixedTime }

	md := &domain.MarketData{
		ISIN:         "RU000A107456",
		LastPricePct: 98.40,
	}

	cache.Set(md.ISIN, md)

	// Hit
	cached, found := cache.Get(md.ISIN)
	assert.True(t, found)
	assert.Equal(t, 98.40, cached.LastPricePct)

	// Advance time within TTL (60s later)
	cache.nowFn = func() time.Time { return fixedTime.Add(60 * time.Second) }
	cached, found = cache.Get(md.ISIN)
	assert.True(t, found)

	// Advance time past 120s TTL (121s later)
	cache.nowFn = func() time.Time { return fixedTime.Add(121 * time.Second) }
	_, found = cache.Get(md.ISIN)
	assert.False(t, found)
}

func TestMarketDataCache_AdaptiveTTL(t *testing.T) {
	cache := NewMarketDataCache()

	// Tuesday 12:00 UTC = 15:00 MSK (Day session)
	tTrading := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, 120*time.Second, cache.GetAdaptiveTTL(tTrading))

	// Tuesday 18:00 UTC = 21:00 MSK (Evening session up to 23:50 MSK)
	tEvening := time.Date(2026, 10, 6, 18, 0, 0, 0, time.UTC)
	assert.Equal(t, 120*time.Second, cache.GetAdaptiveTTL(tEvening))

	// Tuesday 22:00 UTC = 01:00 MSK next day (Night / off-hours)
	tNight := time.Date(2026, 10, 6, 22, 0, 0, 0, time.UTC)
	assert.Equal(t, 60*time.Minute, cache.GetAdaptiveTTL(tNight))

	// Sunday 12:00 UTC = 15:00 MSK (Weekend)
	tWeekend := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, 60*time.Minute, cache.GetAdaptiveTTL(tWeekend))
}
