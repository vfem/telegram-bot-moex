package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/provider"
	"telegram-bot-moex/internal/store/memory"
)

type mockBondProvider struct {
	bonds map[string]*domain.Bond
}

func (m *mockBondProvider) GetBond(ctx context.Context, isin string) (*domain.Bond, error) {
	if b, ok := m.bonds[isin]; ok {
		return b, nil
	}
	return nil, errors.New("not found")
}

func (m *mockBondProvider) SearchBonds(ctx context.Context, query string) ([]domain.BondSummary, error) {
	for k, b := range m.bonds {
		if k == query || b.Ticker == query {
			return []domain.BondSummary{{ISIN: b.ISIN, Ticker: b.Ticker, Name: b.Name}}, nil
		}
	}
	return nil, nil
}

func (m *mockBondProvider) GetUpcomingPayments(ctx context.Context, isin string) ([]domain.Payment, error) {
	return nil, nil
}

func (m *mockBondProvider) GetPaymentsForDate(ctx context.Context, date time.Time, isins []string) ([]domain.Payment, error) {
	return nil, nil
}

func (m *mockBondProvider) GetNewAnnouncements(ctx context.Context, since time.Time) ([]domain.Announcement, error) {
	return nil, nil
}

func (m *mockBondProvider) GetMarketData(ctx context.Context, identifier string) (*domain.MarketData, error) {
	return nil, nil
}

type mockBfoProvider struct {
	reports map[string]*domain.IssuerBalance
}

func (m *mockBfoProvider) GetIssuerBalance(ctx context.Context, inn string) (*domain.IssuerBalance, error) {
	if rep, ok := m.reports[inn]; ok {
		return rep, nil
	}
	return nil, errors.New("no reports for inn")
}

func TestFinancialsService_InputValidationSecurity(t *testing.T) {
	svc := NewFinancialsService(nil, nil, nil)
	ctx := context.Background()

	maliciousInputs := []string{
		"../../etc/passwd",
		"http://169.254.169.254",
		"<script>alert(1)</script>",
		"7736050003; DROP TABLE",
		"RU000A107456\n123",
		"INVALID ISIN!",
		"   ",
	}

	for _, input := range maliciousInputs {
		_, err := svc.GetIssuerBalance(ctx, input)
		if !errors.Is(err, ErrInvalidIdentifier) {
			t.Errorf("expected ErrInvalidIdentifier for input %q, got %v", input, err)
		}
	}
}

func TestFinancialsService_GetIssuerBalance_ByINNAndISIN(t *testing.T) {
	ctx := context.Background()

	bonds := map[string]*domain.Bond{
		"RU000A107456": {
			ISIN:      "RU000A107456",
			Ticker:    "RU000A107456",
			Name:      "ЕвроТранс выпуск 3",
			Issuer:    "ПАО ЕвроТранс",
			IssuerINN: "7708304947",
		},
		"SU26238RMFS4": {
			ISIN:      "RU000A1038V6",
			Ticker:    "SU26238RMFS4",
			Name:      "ОФЗ 26238",
			Issuer:    "Минфин РФ",
			IssuerINN: "7710168360",
		},
		"RU000A0ZZZZ0": {
			ISIN:      "RU000A0ZZZZ0",
			Ticker:    "RU000A0ZZZZ0",
			Name:      "Облигации Нижегородской области",
			Issuer:    "Министерство финансов Нижегородской области",
			IssuerINN: "5260000000",
		},
	}

	reports := map[string]*domain.IssuerBalance{
		"7708304947": {
			INN:          "7708304947",
			IssuerName:   "ПАО ЕвроТранс",
			ReportYear:   2025,
			TotalAssets:  114.2e9,
			Equity:       38.5e9,
			DebtToEquity: 1.28,
			RiskLevel:    domain.RiskModerate,
		},
	}

	cache := memory.NewFinancialsCache()
	bp := &mockBondProvider{bonds: bonds}
	bfo := &mockBfoProvider{reports: reports}
	svc := NewFinancialsService(bp, bfo, cache)

	// 1. Direct INN query
	bal1, err := svc.GetIssuerBalance(ctx, "7708304947")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bal1.TotalAssets != 114.2e9 {
		t.Errorf("TotalAssets = %v, want %v", bal1.TotalAssets, 114.2e9)
	}

	// Verify cached
	if _, ok := cache.Get("7708304947"); !ok {
		t.Errorf("expected balance to be in cache")
	}

	// 2. Query by ISIN
	bal2, err := svc.GetIssuerBalance(ctx, "RU000A107456")
	if err != nil {
		t.Fatalf("unexpected error by ISIN: %v", err)
	}
	if bal2.INN != "7708304947" {
		t.Errorf("INN = %s, want 7708304947", bal2.INN)
	}

	// 3. Federal Sovereign bond
	sovBal, err := svc.GetIssuerBalance(ctx, "SU26238RMFS4")
	if err != nil {
		t.Fatalf("unexpected error for sovereign bond: %v", err)
	}
	if !sovBal.IsSovereign || sovBal.RiskLevel != domain.RiskSovereign {
		t.Errorf("expected sovereign balance, got %v", sovBal)
	}

	// 4. Regional "Минфин ..." bond is NOT federal sovereign debt (F4 check)
	_, regErr := svc.GetIssuerBalance(ctx, "RU000A0ZZZZ0")
	if regErr == nil {
		t.Fatal("expected error because regional INN has no reports, but got nil (it was erroneously treated as sovereign)")
	}

	// 5. Invalid INN checksum rejection (F5 check)
	_, chkErr := svc.GetIssuerBalance(ctx, "1234567890")
	if !errors.Is(chkErr, ErrInvalidINNChecksum) {
		t.Errorf("expected ErrInvalidINNChecksum for 1234567890, got %v", chkErr)
	}

	// 6. Negative caching for not-found issuer (F5 check)
	_, notFoundErr := svc.GetIssuerBalance(ctx, "7736050003") // valid INN but not in mock reports
	if !errors.Is(notFoundErr, ErrIssuerNotFound) {
		t.Errorf("expected ErrIssuerNotFound, got %v", notFoundErr)
	}
	if !cache.IsNotFound("7736050003") {
		t.Errorf("expected 7736050003 to be stored in negative cache")
	}
}

func TestFinancialsService_SingleflightDeduplication(t *testing.T) {
	cache := memory.NewFinancialsCache()
	callCount := 0
	bfo := &mockBfoProvider{
		reports: map[string]*domain.IssuerBalance{
			"7736050003": {INN: "7736050003", TotalAssets: 100e9},
		},
	}
	wrappedBFO := &countingBfoProvider{
		inner: bfo,
		calls: &callCount,
		delay: 50 * time.Millisecond,
	}

	svc := NewFinancialsService(&mockBondProvider{}, wrappedBFO, cache)

	// Launch 10 concurrent requests for the same INN
	const concurrency = 10
	errChan := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			_, err := svc.GetIssuerBalance(context.Background(), "7736050003")
			errChan <- err
		}()
	}

	for i := 0; i < concurrency; i++ {
		if err := <-errChan; err != nil {
			t.Errorf("concurrent lookup failed: %v", err)
		}
	}

	// Only 1 upstream call should have been made
	if callCount != 1 {
		t.Errorf("expected 1 upstream call via singleflight, got %d", callCount)
	}
}

type countingBfoProvider struct {
	inner provider.FinancialsProvider
	calls *int
	delay time.Duration
}

func (c *countingBfoProvider) GetIssuerBalance(ctx context.Context, inn string) (*domain.IssuerBalance, error) {
	time.Sleep(c.delay)
	*c.calls++
	return c.inner.GetIssuerBalance(ctx, inn)
}
