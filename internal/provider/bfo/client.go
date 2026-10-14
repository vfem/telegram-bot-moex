package bfo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"telegram-bot-moex/internal/domain"
)

// ErrOrganizationNotFound is returned when an organization is not found in BFO by INN.
var ErrOrganizationNotFound = errors.New("organization not found in BFO")

// FinancialsProvider is the interface for retrieving issuer financial statements.
type FinancialsProvider interface {
	GetIssuerBalance(ctx context.Context, inn string) (*domain.IssuerBalance, error)
}

// Client interacts with the public bo.nalog.gov.ru API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new BFO API client with SSRF and timeout protections.
func NewClient(customBaseURL *string, customHTTPClient ...*http.Client) *Client {
	base := "https://bo.nalog.gov.ru"
	if customBaseURL != nil && *customBaseURL != "" {
		base = strings.TrimRight(*customBaseURL, "/")
	}

	var hc *http.Client
	if len(customHTTPClient) > 0 && customHTTPClient[0] != nil {
		hc = customHTTPClient[0]
	} else {
		hc = &http.Client{
			Timeout: 8 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("stopped after 5 redirects")
				}
				// Security: prevent redirects to private IP ranges or metadata service
				host := req.URL.Hostname()
				if isPrivateOrLocalHost(host) {
					return fmt.Errorf("security: redirect to private/internal host blocked: %s", host)
				}
				if req.URL.Scheme != "https" {
					return fmt.Errorf("security: redirect to non-https scheme blocked: %s", req.URL.Scheme)
				}
				return nil
			},
		}
	}

	return &Client{
		baseURL:    base,
		httpClient: hc,
	}
}

// isPrivateOrLocalHost checks whether a host is localhost, link-local, or private RFC1918.
func isPrivateOrLocalHost(host string) bool {
	if host == "localhost" || host == "metadata.google.internal" || host == "169.254.169.254" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() {
		return true
	}
	return false
}

// searchResponse model for /advanced-search/organizations/search
type searchResponse struct {
	Content []struct {
		ID        int64  `json:"id"`
		INN       string `json:"inn"`
		ShortName string `json:"shortName"`
		Bfo       struct {
			IsCb bool `json:"isCb"`
		} `json:"bfo"`
	} `json:"content"`
}

// bfoStatementData represents balance and financial result data
type bfoStatementData struct {
	Period            string `json:"period"`
	CorrectionVersion int    `json:"correctionVersion"`
	Balance           struct {
		Current1600 *float64 `json:"current1600"`
		Current1300 *float64 `json:"current1300"`
		Current1410 *float64 `json:"current1410"`
		Current1510 *float64 `json:"current1510"`
		Current1250 *float64 `json:"current1250"`
	} `json:"balance"`
	FinancialResult struct {
		Current2110 *float64 `json:"current2110"`
		Current2400 *float64 `json:"current2400"`
	} `json:"financialResult"`
	AuditReport *struct {
		Name string `json:"name"`
		INN  string `json:"inn"`
	} `json:"auditReport"`
}

// bfoResponseItem model for /nbo/organizations/{id}/bfo/
type bfoResponseItem struct {
	bfoStatementData
	TypeCorrections []struct {
		Type       int              `json:"type"`
		Correction bfoStatementData `json:"correction"`
	} `json:"typeCorrections"`
	BfoData map[string]bfoStatementData `json:"bfoData"`
}

// GetIssuerBalance retrieves balance sheet and income statement by INN.
func (c *Client) GetIssuerBalance(ctx context.Context, inn string) (*domain.IssuerBalance, error) {
	cleanINN := strings.TrimSpace(inn)
	if cleanINN == "" {
		return nil, errors.New("empty INN provided")
	}

	// 1. Search organization ID by INN
	orgID, shortName, isCb, err := c.searchOrganization(ctx, cleanINN)
	if err != nil {
		return nil, fmt.Errorf("bfo search error: %w", err)
	}

	if isCb {
		balance := &domain.IssuerBalance{
			INN:         cleanINN,
			IssuerName:  shortName,
			Standard:    "ЦБ РФ",
			Source:      "ГИР БО (ФНС)",
			FetchedAt:   time.Now(),
			IsCreditOrg: true,
		}
		balance.CalculateMetrics()
		return balance, nil
	}

	// 2. Fetch BFO statements by organization ID
	statements, err := c.fetchBfoStatements(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("bfo statements error: %w", err)
	}
	if len(statements) == 0 {
		return nil, fmt.Errorf("no financial statements found for INN %s", cleanINN)
	}

	// 3. Find latest closed year with balance
	balance, err := c.extractLatestBalance(statements, cleanINN, shortName)
	if err != nil {
		return nil, err
	}

	balance.CalculateMetrics()
	return balance, nil
}

func (c *Client) searchOrganization(ctx context.Context, inn string) (int64, string, bool, error) {
	reqURL := fmt.Sprintf("%s/advanced-search/organizations/search?query=%s", c.baseURL, url.QueryEscape(inn))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, "", false, err
	}
	req.Header.Set("User-Agent", "BondsBot/1.0 (Mozilla/5.0 compatible)")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://bo.nalog.gov.ru/")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, "", false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, "", false, fmt.Errorf("bfo search returned status %d", resp.StatusCode)
	}

	// Security: limit reader to 4 MB to protect against OOM
	limitedReader := io.LimitReader(resp.Body, 4*1024*1024)

	var sr searchResponse
	if err := json.NewDecoder(limitedReader).Decode(&sr); err != nil {
		return 0, "", false, fmt.Errorf("failed to decode search response: %w", err)
	}

	if len(sr.Content) == 0 {
		return 0, "", false, fmt.Errorf("organization not found in BFO by INN: %s", inn)
	}

	// Match organization by stripped INN (search returns <strong>123</strong> highlighting)
	for _, item := range sr.Content {
		rawINN := strings.ReplaceAll(strings.ReplaceAll(item.INN, "<strong>", ""), "</strong>", "")
		if rawINN == inn {
			return item.ID, item.ShortName, item.Bfo.IsCb, nil
		}
	}

	// Exact match required by stripped INN. Do not fall back to unrelated organizations.
	return 0, "", false, fmt.Errorf("%w: INN %s not found in search results", ErrOrganizationNotFound, inn)
}

func (c *Client) fetchBfoStatements(ctx context.Context, orgID int64) ([]bfoResponseItem, error) {
	reqURL := fmt.Sprintf("%s/nbo/organizations/%d/bfo/", c.baseURL, orgID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "BondsBot/1.0 (Mozilla/5.0 compatible)")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://bo.nalog.gov.ru/")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bfo statements endpoint returned status %d", resp.StatusCode)
	}

	limitedReader := io.LimitReader(resp.Body, 4*1024*1024)

	var items []bfoResponseItem
	if err := json.NewDecoder(limitedReader).Decode(&items); err != nil {
		return nil, fmt.Errorf("failed to decode bfo statements: %w", err)
	}

	return items, nil
}

func (c *Client) extractLatestBalance(items []bfoResponseItem, inn, shortName string) (*domain.IssuerBalance, error) {
	var bestYear int
	var bestCorrectionVersion int
	var bestData *domain.IssuerBalance

	processStatement := func(yearStr string, data bfoStatementData) {
		year, err := strconv.Atoi(yearStr)
		if err != nil {
			return
		}

		// In BFO, Form 1 and Form 2 values are in THOUSANDS OF RUBLES (ОКЕИ: 384)
		// We multiply by 1000 to convert to rubles.
		const scale = 1000.0

		var totalAssets, equity, longDebt, shortDebt, cash, revenue, netProfit float64
		if data.Balance.Current1600 != nil {
			totalAssets = *data.Balance.Current1600 * scale
		}
		if data.Balance.Current1300 != nil {
			equity = *data.Balance.Current1300 * scale
		}
		if data.Balance.Current1410 != nil {
			longDebt = *data.Balance.Current1410 * scale
		}
		if data.Balance.Current1510 != nil {
			shortDebt = *data.Balance.Current1510 * scale
		}
		if data.Balance.Current1250 != nil {
			cash = *data.Balance.Current1250 * scale
		}

		if data.FinancialResult.Current2110 != nil {
			revenue = *data.FinancialResult.Current2110 * scale
		}
		if data.FinancialResult.Current2400 != nil {
			netProfit = *data.FinancialResult.Current2400 * scale
		}

		// Only consider years that have balance data (total assets > 0).
		// Prefer later year, or if same year, prefer later correction version or update.
		isBetter := totalAssets > 0 && (year > bestYear || (year == bestYear && (bestData == nil || data.CorrectionVersion >= bestCorrectionVersion)))
		if isBetter {
			bestYear = year
			bestCorrectionVersion = data.CorrectionVersion
			auditor := ""
			if data.AuditReport != nil {
				auditor = data.AuditReport.Name
			}

			bestData = &domain.IssuerBalance{
				INN:             inn,
				IssuerName:      shortName,
				ReportYear:      year,
				Standard:        "РСБУ",
				TotalAssets:     totalAssets,
				Equity:          equity,
				LongTermDebt:    longDebt,
				ShortTermDebt:   shortDebt,
				CashEquivalents: cash,
				Revenue:         revenue,
				NetProfit:       netProfit,
				AuditorName:     auditor,
				Source:          "ГИР БО (ФНС)",
				FetchedAt:       time.Now(),
			}
		}
	}

	for _, item := range items {
		// 1. Baseline direct balance on item
		if item.Balance.Current1600 != nil {
			period := item.Period
			if item.bfoStatementData.Period != "" {
				period = item.bfoStatementData.Period
			}
			processStatement(period, item.bfoStatementData)
		}

		// 2. Check bfoData map
		for yearStr, data := range item.BfoData {
			processStatement(yearStr, data)
		}

		// 3. Check typeCorrections (live bo.nalog.gov.ru annual reporting, type 12)
		for _, tc := range item.TypeCorrections {
			// In GBO, type 12 is the full annual reporting slice. Skip non-annual statement types.
			if tc.Type != 0 && tc.Type != 12 {
				continue
			}
			period := item.Period
			if tc.Correction.Period != "" {
				period = tc.Correction.Period
			}
			processStatement(period, tc.Correction)
		}
	}

	if bestData == nil {
		return nil, fmt.Errorf("no valid balance sheet data found across reporting years for INN %s", inn)
	}

	return bestData, nil
}
