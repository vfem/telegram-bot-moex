package bfo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"telegram-bot-moex/internal/domain"
)

func TestClient_GetIssuerBalance_Success(t *testing.T) {
	searchJSON := `{
		"content": [
			{
				"id": 6622458,
				"inn": "<strong>7736050003</strong>",
				"shortName": "ПАО \"ГАЗПРОМ\"",
				"bfo": { "isCb": false }
			}
		]
	}`

	bfoJSON := `[
		{
			"period": "2024",
			"bfoData": {
				"2024": {
					"period": "2024",
					"balance": {
						"current1600": 26347838.0,
						"current1300": 17415464.0,
						"current1410": 2809651.0,
						"current1510": 1500000.0,
						"current1250": 500000.0
					},
					"financialResult": {
						"current2110": 5620061.0,
						"current2400": 695570.0
					},
					"auditReport": {
						"name": "ООО Финансовые и бухгалтерские консультанты"
					}
				}
			}
		}
	]`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/advanced-search/organizations/search" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(searchJSON))
			return
		}
		if r.URL.Path == "/nbo/organizations/6622458/bfo/" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(bfoJSON))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(&server.URL, server.Client())
	balance, err := client.GetIssuerBalance(context.Background(), "7736050003")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if balance.ReportYear != 2024 {
		t.Errorf("ReportYear = %d, want 2024", balance.ReportYear)
	}
	if balance.TotalAssets != 26347838.0*1000 {
		t.Errorf("TotalAssets = %v, want %v", balance.TotalAssets, 26347838.0*1000)
	}
	if balance.TotalDebt != (2809651.0+1500000.0)*1000 {
		t.Errorf("TotalDebt = %v, want %v", balance.TotalDebt, (2809651.0+1500000.0)*1000)
	}
	if balance.NetDebt != ((2809651.0+1500000.0)-500000.0)*1000 {
		t.Errorf("NetDebt = %v, want %v", balance.NetDebt, ((2809651.0+1500000.0)-500000.0)*1000)
	}
	if balance.AuditorName != "ООО Финансовые и бухгалтерские консультанты" {
		t.Errorf("AuditorName = %q, want 'ООО Финансовые и бухгалтерские консультанты'", balance.AuditorName)
	}
	if balance.RiskLevel != domain.RiskLow {
		t.Errorf("RiskLevel = %v, want %v", balance.RiskLevel, domain.RiskLow)
	}
}

func TestClient_GetIssuerBalance_Bank(t *testing.T) {
	searchJSON := `{
		"content": [
			{
				"id": 12345,
				"inn": "7702045051",
				"shortName": "ПАО \"МТС-Банк\"",
				"bfo": { "isCb": true }
			}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/advanced-search/organizations/search" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(searchJSON))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(&server.URL, server.Client())
	balance, err := client.GetIssuerBalance(context.Background(), "7702045051")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !balance.IsCreditOrg {
		t.Errorf("IsCreditOrg = false, want true")
	}
	if balance.RiskLevel != domain.RiskBank {
		t.Errorf("RiskLevel = %v, want %v", balance.RiskLevel, domain.RiskBank)
	}
}

func TestClient_GetIssuerBalance_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"content":[]}`))
	}))
	defer server.Close()

	client := NewClient(&server.URL, server.Client())
	_, err := client.GetIssuerBalance(context.Background(), "0000000000")
	if err == nil {
		t.Fatal("expected error when organization not found, got nil")
	}
}

func TestClient_GetIssuerBalance_TypeCorrections(t *testing.T) {
	searchJSON := `{
		"content": [
			{
				"id": 6622458,
				"inn": "7736050003",
				"shortName": "ПАО \"ГАЗПРОМ\"",
				"bfo": { "isCb": false }
			}
		]
	}`

	bfoJSON := `[
		{
			"period": "2024",
			"typeCorrections": [
				{
					"type": 12,
					"correction": {
						"period": "2024",
						"balance": {
							"current1600": 26162827.0,
							"current1300": 16330362.0,
							"current1410": 3137801.0,
							"current1510": 1450341.0,
							"current1250": 267111.0
						},
						"financialResult": {
							"current2110": 6256625.0,
							"current2400": -1076329.0
						},
						"auditReport": {
							"name": "ООО ФБК"
						}
					}
				}
			]
		}
	]`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/advanced-search/organizations/search" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(searchJSON))
			return
		}
		if r.URL.Path == "/nbo/organizations/6622458/bfo/" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(bfoJSON))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(&server.URL, server.Client())
	balance, err := client.GetIssuerBalance(context.Background(), "7736050003")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if balance.ReportYear != 2024 {
		t.Errorf("ReportYear = %d, want 2024", balance.ReportYear)
	}
	if balance.TotalAssets != 26162827.0*1000 {
		t.Errorf("TotalAssets = %v, want %v", balance.TotalAssets, 26162827.0*1000)
	}
	if balance.NetProfit != -1076329.0*1000 {
		t.Errorf("NetProfit = %v, want %v", balance.NetProfit, -1076329.0*1000)
	}
	if balance.AuditorName != "ООО ФБК" {
		t.Errorf("AuditorName = %q, want 'ООО ФБК'", balance.AuditorName)
	}
}

func TestClient_GetIssuerBalance_SearchReturnsNonMatchingINN(t *testing.T) {
	// F1 check: search returns an organization with a DIFFERENT INN.
	// Client must NOT fall back to the first item and must return ErrOrganizationNotFound.
	searchJSON := `{
		"content": [
			{
				"id": 99999,
				"inn": "<strong>9999999999</strong>",
				"shortName": "ООО \"ДРУГАЯ КОМПАНИЯ\"",
				"bfo": { "isCb": false }
			}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/advanced-search/organizations/search" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(searchJSON))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(&server.URL, server.Client())
	_, err := client.GetIssuerBalance(context.Background(), "7736050003")
	if err == nil {
		t.Fatal("expected error when search returns non-matching INN, got nil")
	}
}

func TestClient_GetIssuerBalance_MultipleCorrectionsAndNonAnnualType(t *testing.T) {
	searchJSON := `{
		"content": [
			{
				"id": 6622458,
				"inn": "7736050003",
				"shortName": "ПАО \"ГАЗПРОМ\"",
				"bfo": { "isCb": false }
			}
		]
	}`

	bfoJSON := `[
		{
			"period": "2024",
			"typeCorrections": [
				{
					"type": 12,
					"correction": {
						"period": "2024",
						"correctionVersion": 0,
						"balance": {
							"current1600": 20000000.0,
							"current1300": 10000000.0
						}
					}
				},
				{
					"type": 11,
					"correction": {
						"period": "2024",
						"correctionVersion": 99,
						"balance": {
							"current1600": 99999999.0,
							"current1300": 99999999.0
						}
					}
				},
				{
					"type": 12,
					"correction": {
						"period": "2024",
						"correctionVersion": 1,
						"balance": {
							"current1600": 25000000.0,
							"current1300": 15000000.0
						}
					}
				}
			]
		}
	]`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/advanced-search/organizations/search" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(searchJSON))
			return
		}
		if r.URL.Path == "/nbo/organizations/6622458/bfo/" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(bfoJSON))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient(&server.URL, server.Client())
	balance, err := client.GetIssuerBalance(context.Background(), "7736050003")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Must have picked correctionVersion 1 (25000000*1000), not version 0 (20000000*1000) or non-annual type 11 (99999999*1000)
	if balance.TotalAssets != 25000000.0*1000 {
		t.Errorf("TotalAssets = %v, want %v", balance.TotalAssets, 25000000.0*1000)
	}
}

func TestIsPrivateOrLocalHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"metadata.google.internal", true},
		{"169.254.169.254", true},
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"192.168.1.1", true},
		{"172.16.5.4", true},
		{"bo.nalog.gov.ru", false},
		{"bo.nalog.ru", false},
		{"google.com", false},
	}

	for _, tt := range tests {
		got := isPrivateOrLocalHost(tt.host)
		if got != tt.want {
			t.Errorf("isPrivateOrLocalHost(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}
