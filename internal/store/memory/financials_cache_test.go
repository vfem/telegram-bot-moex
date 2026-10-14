package memory

import (
	"testing"
	"time"

	"telegram-bot-moex/internal/domain"
)

func TestFinancialsCache_GetSetExpiration(t *testing.T) {
	cache := NewFinancialsCache()
	fixedNow := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	cache.nowFn = func() time.Time { return fixedNow }

	balance := &domain.IssuerBalance{
		INN:         "7736050003",
		IssuerName:  "ПАО ГАЗПРОМ",
		ReportYear:  2024,
		TotalAssets: 100e9,
	}

	cache.Set("7736050003", balance)

	// Immediate lookup
	got, found := cache.Get("7736050003")
	if !found {
		t.Fatalf("expected to find cached balance, got not found")
	}
	if got.TotalAssets != 100e9 {
		t.Errorf("TotalAssets = %v, want %v", got.TotalAssets, 100e9)
	}

	// Mutate returned copy: verify original is unchanged (copy-on-read)
	got.TotalAssets = 999
	got2, _ := cache.Get("7736050003")
	if got2.TotalAssets == 999 {
		t.Errorf("mutation affected cached item, copy-on-read violated")
	}

	// 29 days later (still valid)
	cache.nowFn = func() time.Time { return fixedNow.Add(29 * 24 * time.Hour) }
	if _, found := cache.Get("7736050003"); !found {
		t.Errorf("expected balance to be cached at 29 days")
	}

	// 31 days later (expired)
	cache.nowFn = func() time.Time { return fixedNow.Add(31 * 24 * time.Hour) }
	if _, found := cache.Get("7736050003"); found {
		t.Errorf("expected balance to be expired after 31 days")
	}
}

func TestFinancialsCache_CapacityGuardrail(t *testing.T) {
	cache := NewFinancialsCache()
	cache.maxEntries = 3
	fixedNow := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	cache.nowFn = func() time.Time { return fixedNow }

	cache.SetWithTTL("inn1", &domain.IssuerBalance{INN: "inn1"}, 1*time.Hour)
	cache.SetWithTTL("inn2", &domain.IssuerBalance{INN: "inn2"}, 1*time.Hour)
	cache.SetWithTTL("inn3", &domain.IssuerBalance{INN: "inn3"}, 1*time.Hour)

	if len(cache.items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(cache.items))
	}

	// Adding 4th item should prune or keep within maxEntries
	cache.SetWithTTL("inn4", &domain.IssuerBalance{INN: "inn4"}, 1*time.Hour)
	if len(cache.items) > 3 {
		t.Errorf("expected <= 3 items, got %d", len(cache.items))
	}
}

func TestFinancialsCache_CopyOnWrite(t *testing.T) {
	cache := NewFinancialsCache()
	balance := &domain.IssuerBalance{
		INN:         "7736050003",
		TotalAssets: 50e9,
	}

	cache.Set("7736050003", balance)

	// Mutate original pointer passed into Set
	balance.TotalAssets = 12345

	got, found := cache.Get("7736050003")
	if !found {
		t.Fatal("expected cached item")
	}
	if got.TotalAssets != 50e9 {
		t.Errorf("mutating original pointer altered cached item: got %v, want 50e9", got.TotalAssets)
	}
}

func TestFinancialsCache_NegativeCaching(t *testing.T) {
	cache := NewFinancialsCache()
	fixedNow := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	cache.nowFn = func() time.Time { return fixedNow }

	if cache.IsNotFound("9999999999") {
		t.Errorf("expected IsNotFound=false for uncached INN")
	}

	cache.SetNotFound("9999999999", 2*time.Hour)

	if !cache.IsNotFound("9999999999") {
		t.Errorf("expected IsNotFound=true after SetNotFound")
	}

	// Normal Get should return nil, false
	if got, found := cache.Get("9999999999"); found || got != nil {
		t.Errorf("Get returned (%v, %v), want (nil, false)", got, found)
	}

	// 3 hours later (expired)
	cache.nowFn = func() time.Time { return fixedNow.Add(3 * time.Hour) }
	if cache.IsNotFound("9999999999") {
		t.Errorf("expected IsNotFound=false after expiration")
	}
}
