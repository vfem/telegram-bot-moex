package telegram

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/service"
	"telegram-bot-moex/internal/store/memory"
)

type dummyBondProvider struct{}

func (d *dummyBondProvider) GetBond(ctx context.Context, identifier string) (*domain.Bond, error) {
	return nil, nil
}
func (d *dummyBondProvider) GetUpcomingPayments(ctx context.Context, isin string) ([]domain.Payment, error) {
	return nil, nil
}
func (d *dummyBondProvider) GetPaymentsForDate(ctx context.Context, date time.Time, isins []string) ([]domain.Payment, error) {
	return nil, nil
}
func (d *dummyBondProvider) SearchBonds(ctx context.Context, query string) ([]domain.BondSummary, error) {
	return nil, nil
}
func (d *dummyBondProvider) GetMarketData(ctx context.Context, identifier string) (*domain.MarketData, error) {
	return nil, nil
}
func (d *dummyBondProvider) GetNewAnnouncements(ctx context.Context, since time.Time) ([]domain.Announcement, error) {
	return nil, nil
}

type failingNotifierStore struct {
	*memory.Store
}

func (f *failingNotifierStore) GetUsersSubscribedToDailyPayments(ctx context.Context) ([]*domain.User, error) {
	return nil, errors.New("sensitive database connection failed with details")
}

func setupTestBot(token, secretToken string) *Bot {
	memStore := memory.NewStore()
	marketCache := memory.NewMarketDataCache()
	prov := &dummyBondProvider{}
	bondSvc := service.NewBondService(prov, memStore, marketCache)
	subSvc := service.NewSubscriptionService(memStore)
	notifier := service.NewNotifierService(prov, memStore, nil)
	return NewBot(token, secretToken, bondSvc, subSvc, notifier)
}

func TestWebhookHandler_MethodNotAllowed(t *testing.T) {
	bot := setupTestBot("bot-token", "secret-token")
	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	rec := httptest.NewRecorder()

	bot.WebhookHandler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 Method Not Allowed, got %d", rec.Code)
	}
}

func TestWebhookHandler_FailClosed_WhenSecretUnsetInProd(t *testing.T) {
	bot := setupTestBot("bot-token-prod", "") // bot token set, secret unset
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	bot.WebhookHandler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable, got %d", rec.Code)
	}
}

func TestWebhookHandler_SecretTokenValidation(t *testing.T) {
	secret := "correct-secret-token"
	bot := setupTestBot("bot-token-prod", secret)

	t.Run("Missing Secret Header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{}`))
		rec := httptest.NewRecorder()

		bot.WebhookHandler(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("Wrong Secret Header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{}`))
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "wrong-secret-token")
		rec := httptest.NewRecorder()

		bot.WebhookHandler(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("Valid Secret Header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{"update_id":1}`))
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
		rec := httptest.NewRecorder()

		bot.WebhookHandler(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})
}

func TestWebhookHandler_MockModeAllowed(t *testing.T) {
	bot := setupTestBot("", "") // local/mock mode: both empty
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{"update_id":1}`))
	rec := httptest.NewRecorder()

	bot.WebhookHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK in mock mode, got %d", rec.Code)
	}
}

func TestCronDailyDigestHandler_MethodNotAllowed(t *testing.T) {
	bot := setupTestBot("bot-token", "secret-token")
	req := httptest.NewRequest(http.MethodGet, "/cron/daily-digest", nil)
	rec := httptest.NewRecorder()

	bot.CronDailyDigestHandler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 Method Not Allowed on GET, got %d", rec.Code)
	}
}

func TestCronDailyDigestHandler_Success(t *testing.T) {
	bot := setupTestBot("bot-token", "secret-token")
	req := httptest.NewRequest(http.MethodPost, "/cron/daily-digest", nil)
	rec := httptest.NewRecorder()

	bot.CronDailyDigestHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on POST, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("expected response body with status ok, got %s", body)
	}
}

func TestCronDailyDigestHandler_ErrorSanitization(t *testing.T) {
	prov := &dummyBondProvider{}
	failingStore := &failingNotifierStore{Store: memory.NewStore()}
	marketCache := memory.NewMarketDataCache()
	bondSvc := service.NewBondService(prov, failingStore, marketCache)
	subSvc := service.NewSubscriptionService(failingStore)
	notifier := service.NewNotifierService(prov, failingStore, nil)
	bot := NewBot("bot-token", "secret-token", bondSvc, subSvc, notifier)

	req := httptest.NewRequest(http.MethodPost, "/cron/daily-digest", bytes.NewReader(nil))
	rec := httptest.NewRecorder()

	bot.CronDailyDigestHandler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error, got %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "sensitive database") {
		t.Fatalf("expected error body to be sanitized, but found sensitive details: %s", body)
	}
	if !strings.Contains(body, "Internal server error") {
		t.Fatalf("expected 'Internal server error', got: %s", body)
	}
}
