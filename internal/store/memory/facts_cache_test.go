package memory

import (
	"testing"
	"time"

	"telegram-bot-moex/internal/domain"
)

func TestFactsCache_GetSetAndExpiry(t *testing.T) {
	cache := NewFactsCache()
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	cache.nowFn = func() time.Time { return now }

	sample := []domain.MaterialFact{
		{
			ID:       "f1",
			INN:      "7708304947",
			Title:    "Выплата купонного дохода",
			Category: domain.FactCoupon,
			Severity: domain.SeverityInfo,
		},
	}

	cache.Set("7708304947", sample)

	got, ok := cache.Get("7708304947")
	if !ok || len(got) != 1 || got[0].ID != "f1" {
		t.Fatalf("expected cached fact f1, got %v (ok=%v)", got, ok)
	}

	// Mutating returned slice should not mutate cached entry
	got[0].Title = "mutated"
	got2, _ := cache.Get("7708304947")
	if got2[0].Title == "mutated" {
		t.Error("cache should copy on read to prevent external mutation")
	}

	// Advance time past 45m TTL
	now = now.Add(46 * time.Minute)
	if _, ok := cache.Get("7708304947"); ok {
		t.Error("expected cache entry to expire after 45 minutes")
	}
}

func TestFactsCache_NegativeCaching(t *testing.T) {
	cache := NewFactsCache()
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	cache.nowFn = func() time.Time { return now }

	cache.SetNotFound("7701234560", 15*time.Minute)

	if !cache.IsNotFound("7701234560") {
		t.Fatal("expected IsNotFound to be true")
	}
	if _, ok := cache.Get("7701234560"); ok {
		t.Fatal("expected Get to return false for negatively cached INN")
	}

	now = now.Add(16 * time.Minute)
	if cache.IsNotFound("7701234560") {
		t.Fatal("expected negative cache entry to expire after TTL")
	}
}
