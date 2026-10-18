package edisclosure

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"telegram-bot-moex/internal/domain"
)

func TestGetRecentFacts_RSSFeed_AndHTMLCleanup(t *testing.T) {
	rssPayload := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>ПАО «ЕвроТранс»</title>
    <item>
      <guid>evt-101</guid>
      <title>Выплата купонного дохода по облигациям серии БО-001Р-03 в полном объеме</title>
      <description>&lt;p&gt;Купонный доход &lt;b&gt;выплачен&lt;/b&gt; полностью&lt;/p&gt;</description>
      <link>https://www.e-disclosure.ru/portal/event.aspx?EventId=101</link>
      <pubDate>02.10.2026 14:15</pubDate>
    </item>
    <item>
      <guid>evt-102</guid>
      <title>Определение ставки купона и даты начала периода предъявления бумаг к выкупу</title>
      <description>Оферта по выпуску</description>
      <link>https://www.e-disclosure.ru/portal/event.aspx?EventId=102</link>
      <pubDate>25.09.2026 18:30</pubDate>
    </item>
    <item>
      <guid>evt-103</guid>
      <title>Неисполнение обязательств эмитента по выплате купона</title>
      <link>javascript:alert(1)</link>
      <pubDate>06.10.2026 09:00</pubDate>
    </item>
  </channel>
</rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(rssPayload))
	}))
	defer srv.Close()

	baseURL := srv.URL
	client := NewClient(&baseURL, srv.Client())

	facts, err := client.GetRecentFacts(context.Background(), "7708304947", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(facts) != 3 {
		t.Fatalf("expected 3 facts, got %d", len(facts))
	}

	// Sorted newest first: 06.10.2026 (evt-103) -> 02.10.2026 (evt-101) -> 25.09.2026 (evt-102)
	if facts[0].ID != "evt-103" {
		t.Errorf("expected newest fact evt-103 first, got %s", facts[0].ID)
	}
	if facts[0].Severity != domain.SeverityCritical || facts[0].Category != domain.FactDefault {
		t.Errorf("expected CRITICAL default for evt-103, got %s / %s", facts[0].Severity, facts[0].Category)
	}
	if facts[0].URL != "" {
		t.Errorf("expected unsafe javascript: URL to be sanitized to empty string, got %q", facts[0].URL)
	}

	// Verify HTML tags were stripped from summary (F11.5)
	if facts[1].Summary != "Купонный доход выплачен полностью" {
		t.Errorf("expected HTML tags to be stripped from summary, got %q", facts[1].Summary)
	}
	// Verify MSK timezone was used (F11.4: UTC+3)
	_, offset := facts[1].PublishedAt.Zone()
	if offset != 3*3600 {
		t.Errorf("expected MSK timezone offset (10800s), got %d", offset)
	}
}

func TestGetRecentFacts_JSONArray(t *testing.T) {
	jsonArr := `[
		{
			"id": "j-1",
			"inn": "7708304947",
			"isin": "RU000A107456",
			"issuer_name": "ПАО ЕвроТранс",
			"title": "Решения заседания совета директоров",
			"published_at": "2026-08-12 10:00:00",
			"url": "https://www.e-disclosure.ru/portal/event.aspx?EventId=1"
		}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(jsonArr))
	}))
	defer srv.Close()

	baseURL := srv.URL
	client := NewClient(&baseURL, srv.Client())

	facts, err := client.GetRecentFacts(context.Background(), "7708304947", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(facts) != 1 || facts[0].Category != domain.FactMeeting {
		t.Fatalf("expected 1 FactMeeting item, got %+v", facts)
	}
}

func TestGetRecentFacts_MOEXJsonFormat_LargeNumericIDAndConstructedURL(t *testing.T) {
	// Real MOEX sitenews has no "url" column and numeric IDs > 1,000,000 (F7.1, F7.2)
	moexJSON := `{
		"sitenews": {
			"columns": ["id", "tag", "title", "published_at", "modified_at"],
			"data": [
				[1052061, "site", "Решения заседания совета директоров эмитента", "2026-08-12 10:00:00", "2026-08-12 10:05:00"],
				[1052062, "site", "Выплата купонного дохода в полном объеме", "2026-10-02 12:00:00", "2026-10-02 12:05:00"]
			]
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(moexJSON))
	}))
	defer srv.Close()

	baseURL := srv.URL
	client := NewClient(&baseURL, srv.Client())

	facts, err := client.GetRecentFacts(context.Background(), "7708304947", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d", len(facts))
	}
	if facts[0].ID != "1052062" {
		t.Errorf("expected integer ID '1052062' (not scientific notation), got %q", facts[0].ID)
	}
	if facts[0].URL != "https://www.moex.com/n1052062" {
		t.Errorf("expected constructed MOEX URL 'https://www.moex.com/n1052062', got %q", facts[0].URL)
	}
}

func TestGetRecentFacts_MOEXBondizationAndFilteredSiteNews(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/securities.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"securities": {
				"columns": ["secid", "shortname", "name", "isin", "is_traded", "emitent_title", "emitent_inn"],
				"data": [
					["UNRELATED1", "ЧужойВыпуск", "Чужой БО-01", "RU000A000001", 1, "ПАО \"Чужой\"", ""],
					["RU000A107456", "ЕвроТранс3", "ЕвроТранс БО-001Р-03", "RU000A107456", 1, "ПАО \"ЕвроТранс\"", "7708304947"]
				]
			}
		}`))
	})
	mux.HandleFunc("/securities/RU000A107456.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"description": {
				"columns": ["name", "title", "value"],
				"data": [
					["HASDEFAULT", "Допущен дефолт", "0"],
					["HASTECHNICALDEFAULT", "Допущен технический дефолт", "0"]
				]
			}
		}`))
	})
	mux.HandleFunc("/securities/RU000A107456/bondization.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"offers": {
				"columns": ["isin", "name", "offerdate", "offertype", "secid"],
				"data": [
					["RU000A107456", "ЕвроТранс БО-001Р-03", "2026-09-25", "Оферта", "RU000A107456"]
				]
			},
			"coupons": {
				"columns": ["isin", "name", "coupondate", "faceunit", "value", "secid"],
				"data": [
					["RU000A107456", "ЕвроТранс БО-001Р-03", "2023-05-24", "RUB", 37.15, "RU000A107456"],
					["RU000A107456", "ЕвроТранс БО-001Р-03", "2026-10-02", "RUB", 33.29, "RU000A107456"],
					["RU000A107456", "ЕвроТранс БО-001Р-03", "2026-10-25", "RUB", null, "RU000A107456"]
				]
			},
			"amortizations": {
				"columns": ["isin", "name", "amortdate", "faceunit", "value", "secid"],
				"data": []
			}
		}`))
	})
	mux.HandleFunc("/sitenews.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"sitenews": {
				"columns": ["id", "title", "published_at"],
				"data": [
					[2001001, "Общие торги на срочном рынке", "2026-10-10 10:00:00"],
					[2001002, "ЕвроТранс: &lt;b&gt;Решения&lt;/b&gt; заседания совета директоров", "2026-08-12 10:00:00"]
				]
			}
		}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	baseURL := srv.URL
	client := NewClient(&baseURL, srv.Client())

	facts, err := client.GetRecentFacts(context.Background(), "7708304947", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should include 2026 coupon, offer, and matching sitenews, and exclude:
	// - 2023 coupon (> 1 year old, R1.3)
	// - 2026-10-25 floater coupon with value: null (R1.1)
	// - unrelated "Общие торги на срочном рынке"
	if len(facts) != 3 {
		t.Fatalf("expected 3 facts (coupon + offer + filtered sitenews), got %d: %+v", len(facts), facts)
	}
	for _, f := range facts {
		if strings.Contains(f.Title, "срочном рынке") {
			t.Errorf("unfiltered exchange-wide news should not be attributed to issuer: %+v", f)
		}
		if strings.Contains(f.Title, "37.15") {
			t.Errorf("coupon older than 1 year should be excluded (R1.3): %+v", f)
		}
		if strings.Contains(f.Title, "<b>") || strings.Contains(f.Title, "&lt;b&gt;") {
			t.Errorf("entity-encoded HTML tags should be stripped in cleanText (R3.2): %+v", f)
		}
	}
}

func TestGetRecentFacts_HTTP404AndNon200AndSSRFRedirect(t *testing.T) {
	t.Run("HTTP 404 returns ErrNoFactsFound", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()
		baseURL := srv.URL
		client := NewClient(&baseURL, srv.Client())

		_, err := client.GetRecentFacts(context.Background(), "7708304947", 5)
		if !errors.Is(err, ErrNoFactsFound) {
			t.Errorf("expected ErrNoFactsFound on 404, got %v", err)
		}
	})

	t.Run("HTTP 500 returns status error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		baseURL := srv.URL
		client := NewClient(&baseURL, srv.Client())

		_, err := client.GetRecentFacts(context.Background(), "7708304947", 5)
		if err == nil || !strings.Contains(err.Error(), "status 500") {
			t.Errorf("expected status 500 error, got %v", err)
		}
	})

	t.Run("SSRF redirect check blocks localhost and private IPs", func(t *testing.T) {
		reqPrivate, _ := http.NewRequest(http.MethodGet, "https://169.254.169.254/computeMetadata/v1/", nil)
		if err := checkSafeRedirect(reqPrivate, []*http.Request{{}}); err == nil {
			t.Error("expected redirect to metadata IP 169.254.169.254 to be blocked")
		}
		reqHTTP, _ := http.NewRequest(http.MethodGet, "http://iss.moex.com/iss", nil)
		if err := checkSafeRedirect(reqHTTP, []*http.Request{{}}); err == nil {
			t.Error("expected redirect to non-https scheme to be blocked")
		}
	})
}
