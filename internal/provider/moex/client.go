package moex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"telegram-bot-moex/internal/domain"
)

// HTTPClient represents the minimal interface required for issuing HTTP requests.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client implements the MOEX ISS unauthenticated API client.
type Client struct {
	baseURL    string
	httpClient HTTPClient
}

// NewClient returns a new MOEX ISS client.
func NewClient(httpClient HTTPClient) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{
		baseURL:    "https://iss.moex.com/iss",
		httpClient: httpClient,
	}
}

// NewClientWithBaseURL allows pointing to a mock server in tests.
func NewClientWithBaseURL(baseURL string, httpClient HTTPClient) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

// issResponse represents the generic MOEX ISS tabular JSON schema.
type issResponse struct {
	Description struct {
		Columns []string        `json:"columns"`
		Data    [][]interface{} `json:"data"`
	} `json:"description"`
	Securities struct {
		Columns []string        `json:"columns"`
		Data    [][]interface{} `json:"data"`
	} `json:"securities"`
	Coupons struct {
		Columns []string        `json:"columns"`
		Data    [][]interface{} `json:"data"`
	} `json:"coupons"`
	Amortizations struct {
		Columns []string        `json:"columns"`
		Data    [][]interface{} `json:"data"`
	} `json:"amortizations"`
}

// GetBond fetches bond information from MOEX ISS.
func (c *Client) GetBond(ctx context.Context, identifier string) (*domain.Bond, error) {
	reqURL := fmt.Sprintf("%s/securities/%s.json?iss.meta=off", c.baseURL, url.PathEscape(identifier))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("moex iss error: status %d", resp.StatusCode)
	}

	var data issResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	bond := &domain.Bond{
		Exchange: domain.ExchangeMOEX,
		Currency: "RUB",
	}

	// Parse description table
	colIdx := make(map[string]int)
	for i, col := range data.Description.Columns {
		colIdx[col] = i
	}

	nameIdx, hasName := colIdx["name"]
	valIdx, hasVal := colIdx["value"]

	if hasName && hasVal {
		for _, row := range data.Description.Data {
			if len(row) <= valIdx {
				continue
			}
			key, _ := row[nameIdx].(string)
			val := row[valIdx]

			switch key {
			case "ISIN":
				bond.ISIN, _ = val.(string)
			case "SECID":
				bond.Ticker, _ = val.(string)
			case "NAME", "SHORTNAME":
				if bond.Name == "" {
					bond.Name, _ = val.(string)
				}
			case "EMITTER":
				bond.Issuer, _ = val.(string)
			case "FACEVALUE":
				if v, ok := val.(float64); ok {
					bond.NominalValue = v
				}
			case "COUPONPERCENT":
				if v, ok := val.(float64); ok {
					bond.CouponRatePct = v
				}
			case "COUPONFREQUENCY":
				if v, ok := val.(float64); ok {
					bond.CouponsPerYear = int(v)
				}
			case "MATDATE":
				if s, ok := val.(string); ok && s != "" {
					t, err := time.Parse("2006-01-02", s)
					if err == nil {
						bond.MaturityDate = t
					}
				}
			}
		}
	}

	if bond.ISIN == "" {
		bond.ISIN = identifier
	}
	if bond.Name == "" {
		bond.Name = identifier
	}

	return bond, nil
}

// GetUpcomingPayments parses the bondization coupon & amortization table.
func (c *Client) GetUpcomingPayments(ctx context.Context, isin string) ([]domain.Payment, error) {
	reqURL := fmt.Sprintf("%s/statistics/engines/stock/markets/bonds/bondization/%s.json?iss.meta=off", c.baseURL, url.PathEscape(isin))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("moex bondization error: status %d", resp.StatusCode)
	}

	var data issResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var payments []domain.Payment

	// Parse coupons
	cColIdx := make(map[string]int)
	for i, col := range data.Coupons.Columns {
		cColIdx[col] = i
	}

	dateIdx := cColIdx["coupondate"]
	valIdx := cColIdx["value"]
	pctIdx := cColIdx["valueprcnt"]

	for _, row := range data.Coupons.Data {
		var p domain.Payment
		p.ISIN = isin
		p.Type = domain.PaymentCoupon

		if len(row) > dateIdx {
			if s, ok := row[dateIdx].(string); ok {
				t, err := time.Parse("2006-01-02", s)
				if err == nil {
					p.Date = t
				}
			}
		}
		if len(row) > valIdx {
			if v, ok := row[valIdx].(float64); ok {
				p.Amount = v
			}
		}
		if len(row) > pctIdx {
			if v, ok := row[pctIdx].(float64); ok {
				p.AmountPct = v
			}
		}

		if !p.Date.IsZero() {
			payments = append(payments, p)
		}
	}

	return payments, nil
}

// GetPaymentsForDate returns payments on a specific date for the requested ISINs.
func (c *Client) GetPaymentsForDate(ctx context.Context, date time.Time, isins []string) ([]domain.Payment, error) {
	targetDateStr := date.Format("2006-01-02")
	var matched []domain.Payment

	for _, isin := range isins {
		payments, err := c.GetUpcomingPayments(ctx, isin)
		if err != nil {
			continue
		}
		for _, p := range payments {
			if p.Date.Format("2006-01-02") == targetDateStr {
				matched = append(matched, p)
			}
		}
	}

	return matched, nil
}

// SearchBonds searches for securities by keyword query on MOEX ISS.
func (c *Client) SearchBonds(ctx context.Context, query string) ([]domain.BondSummary, error) {
	reqURL := fmt.Sprintf("%s/securities.json?q=%s&iss.meta=off", c.baseURL, url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data issResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	colIdx := make(map[string]int)
	for i, col := range data.Securities.Columns {
		colIdx[col] = i
	}

	isinIdx := colIdx["isin"]
	secidIdx := colIdx["secid"]
	nameIdx := colIdx["shortname"]
	groupIdx := colIdx["group"]

	var results []domain.BondSummary
	for _, row := range data.Securities.Data {
		// Filter for stock_bonds
		if groupIdx < len(row) {
			if grp, ok := row[groupIdx].(string); ok && grp != "stock_bonds" && grp != "" {
				continue
			}
		}

		var summary domain.BondSummary
		summary.Exchange = domain.ExchangeMOEX
		if isinIdx < len(row) {
			summary.ISIN, _ = row[isinIdx].(string)
		}
		if secidIdx < len(row) {
			summary.Ticker, _ = row[secidIdx].(string)
		}
		if nameIdx < len(row) {
			summary.Name, _ = row[nameIdx].(string)
		}
		if summary.ISIN == "" {
			summary.ISIN = summary.Ticker
		}

		if summary.ISIN != "" {
			results = append(results, summary)
		}
	}

	return results, nil
}

// GetNewAnnouncements returns recent MOEX bond listings.
func (c *Client) GetNewAnnouncements(ctx context.Context, since time.Time) ([]domain.Announcement, error) {
	// Query MOEX corporate news / history
	return nil, nil
}

// AddPaymentHelper for injecting mock test payments
func (c *Client) FormatBondTitle(bond *domain.Bond) string {
	if bond.Name != "" {
		return strings.TrimSpace(bond.Name)
	}
	return bond.ISIN
}
