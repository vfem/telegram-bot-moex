package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/provider/edisclosure"
	"telegram-bot-moex/internal/store/memory"
)

type stubFactsProvider struct {
	calls int
	facts map[string][]domain.MaterialFact
	err   error
}

func (s *stubFactsProvider) GetRecentFacts(ctx context.Context, inn string, limit int) ([]domain.MaterialFact, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	if list, ok := s.facts[inn]; ok {
		return list, nil
	}
	return nil, edisclosure.ErrNoFactsFound
}

func TestFactsService_GetIssuerFacts_ByISINAndCache(t *testing.T) {
	bondProv := &mockBondProvider{
		bonds: map[string]*domain.Bond{
			"RU000A107456": {
				ISIN:      "RU000A107456",
				Ticker:    "RU000A107456",
				Name:      "ЕвроТранс БО-001Р-03",
				Issuer:    "ПАО «ЕвроТранс»",
				IssuerINN: "7708304947",
			},
		},
	}

	factsProv := &stubFactsProvider{
		facts: map[string][]domain.MaterialFact{
			"7708304947": {
				{
					ID:          "1",
					Title:       "Определение ставки купона и даты начала периода предъявления бумаг к выкупу",
					PublishedAt: time.Date(2026, 9, 25, 18, 30, 0, 0, time.UTC),
				},
				{
					ID:          "2",
					Title:       "Выплата купонного дохода в полном объеме",
					PublishedAt: time.Date(2026, 10, 2, 14, 15, 0, 0, time.UTC),
				},
			},
		},
	}

	cache := memory.NewFactsCache()
	svc := NewFactsService(bondProv, factsProv, cache)

	res, err := svc.GetIssuerFacts(context.Background(), "RU000A107456", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IssuerName != "ПАО «ЕвроТранс»" || res.INN != "7708304947" {
		t.Errorf("unexpected issuer metadata: %+v", res)
	}
	if len(res.Facts) != 2 {
		t.Fatalf("expected 2 facts, got %d", len(res.Facts))
	}
	if res.Facts[0].Category != domain.FactCoupon || res.Facts[0].Severity != domain.SeverityInfo {
		t.Errorf("unexpected first fact classification: %+v", res.Facts[0])
	}
	if res.Facts[1].Category != domain.FactOffer || res.Facts[1].Severity != domain.SeverityWarning {
		t.Errorf("unexpected second fact classification: %+v", res.Facts[1])
	}

	// Second call should hit cache and not invoke factsProv again
	_, err = svc.GetIssuerFacts(context.Background(), "7708304947", 5)
	if err != nil {
		t.Fatalf("unexpected error on cached call: %v", err)
	}
	if factsProv.calls != 1 {
		t.Errorf("expected 1 upstream call due to cache, got %d", factsProv.calls)
	}
}

func TestFactsService_ErrNoFactsFoundMappingAndNegativeCache(t *testing.T) {
	factsProv := &stubFactsProvider{
		facts: map[string][]domain.MaterialFact{},
	}
	cache := memory.NewFactsCache()
	svc := NewFactsService(&mockBondProvider{}, factsProv, cache)

	// 7701234560 has valid INN checksum but no facts -> stub returns edisclosure.ErrNoFactsFound
	_, err := svc.GetIssuerFacts(context.Background(), "7701234560", 5)
	if !errors.Is(err, ErrNoMaterialFacts) {
		t.Fatalf("expected edisclosure.ErrNoFactsFound to map to ErrNoMaterialFacts, got %v", err)
	}
	if !cache.IsNotFound("7701234560") {
		t.Error("expected negative cache entry to be recorded for 7701234560")
	}

	// Second call must be served from negative cache without hitting provider
	_, err2 := svc.GetIssuerFacts(context.Background(), "7701234560", 5)
	if !errors.Is(err2, ErrNoMaterialFacts) {
		t.Fatalf("expected ErrNoMaterialFacts from negative cache, got %v", err2)
	}
	if factsProv.calls != 1 {
		t.Errorf("expected 1 upstream call due to negative cache, got %d", factsProv.calls)
	}
}

func TestFactsService_SovereignBondHandling(t *testing.T) {
	bondProv := &mockBondProvider{
		bonds: map[string]*domain.Bond{
			"SU26238RMFS4": {
				ISIN:      "SU26238RMFS4",
				Ticker:    "SU26238RMFS4",
				Name:      "ОФЗ 26238",
				Issuer:    "Министерство финансов Российской Федерации",
				IssuerINN: "7710168360",
			},
		},
	}
	factsProv := &stubFactsProvider{}
	svc := NewFactsService(bondProv, factsProv, memory.NewFactsCache())

	res, err := svc.GetIssuerFacts(context.Background(), "SU26238RMFS4", 5)
	if err != nil {
		t.Fatalf("unexpected error for sovereign bond: %v", err)
	}
	if !res.IsSovereign || res.INN != "7710168360" {
		t.Errorf("expected IsSovereign=true for OFZ bond, got %+v", res)
	}
	if factsProv.calls != 0 {
		t.Errorf("expected 0 corporate disclosure calls for sovereign bond, got %d", factsProv.calls)
	}
}

func TestFactsService_SecurityValidation(t *testing.T) {
	svc := NewFactsService(&mockBondProvider{}, &stubFactsProvider{}, memory.NewFactsCache())

	_, err := svc.GetIssuerFacts(context.Background(), "../../etc/passwd", 5)
	if !errors.Is(err, ErrInvalidIdentifier) {
		t.Errorf("expected ErrInvalidIdentifier for path traversal input, got %v", err)
	}

	_, err = svc.GetIssuerFacts(context.Background(), "1234567890", 5)
	if !errors.Is(err, ErrInvalidINNChecksum) {
		t.Errorf("expected ErrInvalidINNChecksum for bad INN checksum, got %v", err)
	}
}
