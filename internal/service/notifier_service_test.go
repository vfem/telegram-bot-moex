package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/store/memory"
)

type recordingDispatcher struct {
	mu       sync.Mutex
	failNext bool
	sent     []string
}

func (r *recordingDispatcher) SendMessage(ctx context.Context, chatID int64, text string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failNext {
		r.failNext = false
		return errors.New("temporary telegram API failure")
	}
	r.sent = append(r.sent, text)
	return nil
}

func TestNotifierService_NotifyMaterialFact_MarkdownV2AndRetryAndConcurrency(t *testing.T) {
	memStore := memory.NewStore()
	u := domain.NewUser(1001, 1001)
	u.AddMonitoredISIN("RU000A107456")
	_ = memStore.SaveUser(context.Background(), u)

	disp := &recordingDispatcher{failNext: true}
	notifier := NewNotifierService(&mockBondProvider{}, memStore, nil)
	notifier.SetDispatcher(disp)

	bond := domain.Bond{
		ISIN:   "RU000A107456",
		Name:   "ЕвроТранс БО-001Р-03",
		Issuer: "ПАО «ЕвроТранс»",
	}
	fact := domain.MaterialFact{
		ID:          "evt-55",
		ISIN:        "RU000A107456",
		Title:       "Определение ставки купона и даты выкупа (оферта)",
		PublishedAt: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC),
		URL:         "javascript:alert(1)",
	}

	// 1. First attempt fails in dispatcher -> should NOT mark deduplicated (F6.3)
	msgs1, err := notifier.NotifyMaterialFact(context.Background(), bond, fact)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs1) != 0 {
		t.Fatalf("expected 0 delivered messages on failed dispatch, got %d", len(msgs1))
	}

	// 2. Second attempt succeeds -> should deliver and deduplicate subsequent calls
	msgs2, err := notifier.NotifyMaterialFact(context.Background(), bond, fact)
	if err != nil {
		t.Fatalf("unexpected error on retry: %v", err)
	}
	if len(msgs2) != 1 {
		t.Fatalf("expected 1 delivered message on retry, got %d", len(msgs2))
	}

	text := msgs2[0].Text
	// Verify MarkdownV2 escaping of '!', '-', '(', ')', '[', ']' (F5)
	if !strings.Contains(text, `\!`) || !strings.Contains(text, `\[WARNING\]`) || !strings.Contains(text, `БО\-001Р\-03`) {
		t.Errorf("expected properly escaped MarkdownV2 text, got:\n%s", text)
	}
	// Verify unsafe javascript: URL was stripped (F5)
	if strings.Contains(text, "javascript:") {
		t.Errorf("expected unsafe javascript: URL to be stripped, got:\n%s", text)
	}

	// 3. Concurrent calls should be thread-safe and deduplicated (F6.1)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = notifier.NotifyMaterialFact(context.Background(), bond, fact)
		}()
	}
	wg.Wait()

	disp.mu.Lock()
	sentCount := len(disp.sent)
	disp.mu.Unlock()
	if sentCount != 1 {
		t.Errorf("expected exactly 1 dispatched message after deduplication, got %d", sentCount)
	}
}

func TestNotifierService_CheckMonitoredFacts_WithoutSubPayAndNoCrossSeriesAlerts(t *testing.T) {
	memStore := memory.NewStore()
	// User 2001 monitors Series 03 (RU000A107456) and User 2002 monitors Series 01 (RU000A1034T9),
	// BOTH have NotifyDailyPayments == false (did not run /sub pay, only /sub add).
	u1 := domain.NewUser(2001, 2001)
	u1.AddMonitoredISIN("RU000A107456")
	_ = memStore.SaveUser(context.Background(), u1)

	u2 := domain.NewUser(2002, 2002)
	u2.AddMonitoredISIN("RU000A1034T9")
	_ = memStore.SaveUser(context.Background(), u2)

	bondProv := &mockBondProvider{
		bonds: map[string]*domain.Bond{
			"RU000A107456": {
				ISIN:      "RU000A107456",
				Name:      "МТС-Банк ПАО 001P-03",
				Issuer:    "ПАО МТС-Банк",
				IssuerINN: "7702045051",
			},
			"RU000A1034T9": {
				ISIN:      "RU000A1034T9",
				Name:      "МТС-Банк ПАО 001P-01",
				Issuer:    "ПАО МТС-Банк",
				IssuerINN: "7702045051",
			},
		},
	}

	factsProv := &stubFactsProvider{
		facts: map[string][]domain.MaterialFact{
			"7702045051": {
				{
					ID:          "offer-series-01",
					INN:         "7702045051",
					ISIN:        "RU000A1034T9", // Specific to Series 01!
					Title:       "Оферта по облигациям МТС-Банк ПАО 001P-01: дата выкупа 20.10.2026",
					PublishedAt: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC),
				},
			},
		},
	}

	disp := &recordingDispatcher{}
	notifier := NewNotifierService(bondProv, memStore, disp)
	notifier.SetFactsProvider(factsProv)

	msgs, err := notifier.RunDailyDigest(context.Background(), time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1. GetRecentFacts should only be called once for INN 7702045051 across both bonds (R2.3)
	if factsProv.calls != 1 {
		t.Errorf("expected 1 GetRecentFacts call for shared issuer INN, got %d", factsProv.calls)
	}

	// 2. Only User 2002 (holding Series 01 RU000A1034T9) should receive the Series 01 offer alert;
	// User 2001 (holding Series 03 RU000A107456) must NOT receive a cross-series false alert (R2.1, R2.2)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 alert (only for Series 01 holder), got %d", len(msgs))
	}
	if msgs[0].TelegramID != 2002 {
		t.Errorf("expected alert for user 2002, got %d", msgs[0].TelegramID)
	}
}
