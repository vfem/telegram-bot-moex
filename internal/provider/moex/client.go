package moex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"telegram-bot-moex/internal/domain"
)

// HTTPClient represents the minimal interface required for issuing HTTP requests.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type issuerMeta struct {
	inn          string
	issuer       string
	primaryBoard string
}

// Client implements the MOEX ISS unauthenticated API client.
type Client struct {
	baseURL       string
	httpClient    HTTPClient
	issuerCacheMu sync.RWMutex
	issuerCache   map[string]issuerMeta
}

// NewClient returns a new MOEX ISS client.
func NewClient(httpClient HTTPClient) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{
		baseURL:     "https://iss.moex.com/iss",
		httpClient:  httpClient,
		issuerCache: make(map[string]issuerMeta),
	}
}

// NewClientWithBaseURL allows pointing to a mock server in tests.
func NewClientWithBaseURL(baseURL string, httpClient HTTPClient) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{
		baseURL:     baseURL,
		httpClient:  httpClient,
		issuerCache: make(map[string]issuerMeta),
	}
}

// issResponse represents the generic MOEX ISS tabular JSON schema.
type issResponse struct {
	Description struct {
		Columns []string        `json:"columns"`
		Data    [][]interface{} `json:"data"`
	} `json:"description"`
	Boards struct {
		Columns []string        `json:"columns"`
		Data    [][]interface{} `json:"data"`
	} `json:"boards"`
	Securities struct {
		Columns []string        `json:"columns"`
		Data    [][]interface{} `json:"data"`
	} `json:"securities"`
	Marketdata struct {
		Columns []string        `json:"columns"`
		Data    [][]interface{} `json:"data"`
	} `json:"marketdata"`
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
			case "EMITENT_INN", "INN":
				bond.IssuerINN = toString(val)
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

	// Parse boards table to identify primary trading board
	bColIdx := make(map[string]int)
	for i, col := range data.Boards.Columns {
		bColIdx[col] = i
	}
	boardIdx, hasBoard := bColIdx["boardid"]
	primaryIdx, hasPrimary := bColIdx["is_primary"]
	if hasBoard {
		for _, row := range data.Boards.Data {
			if hasPrimary && len(row) > primaryIdx {
				if toFloat(row[primaryIdx]) == 1 {
					bond.PrimaryBoard = toString(row[boardIdx])
					break
				}
			}
		}
	}

	if bond.PrimaryBoard == "" {
		if strings.HasPrefix(bond.ISIN, "SU") || strings.Contains(bond.Name, "ОФЗ") || strings.HasPrefix(bond.Ticker, "SU") {
			bond.PrimaryBoard = "TQOB"
		} else {
			bond.PrimaryBoard = "TQCB"
		}
	}

	if bond.ISIN == "" {
		bond.ISIN = identifier
	}
	if bond.Name == "" {
		bond.Name = identifier
	}

	// Attempt fallback fetch of INN and issuer if missing
	if (bond.Issuer == "" || bond.IssuerINN == "") && (bond.ISIN != "" || bond.Ticker != "") {
		c.enrichIssuerMetadata(ctx, bond, identifier)
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

// GetMarketData queries MOEX ISS market data for the security.
func (c *Client) GetMarketData(ctx context.Context, identifier string) (*domain.MarketData, error) {
	// First attempt: general engines/stock/markets/bonds/securities endpoint
	reqURL := fmt.Sprintf("%s/engines/stock/markets/bonds/securities/%s.json?iss.meta=off", c.baseURL, url.PathEscape(identifier))
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
		// Fallback attempt: board-specific endpoint (TQCB for corporate, TQOB for OFZ)
		board := "TQCB"
		if strings.HasPrefix(identifier, "SU") {
			board = "TQOB"
		}
		fallbackURL := fmt.Sprintf("%s/engines/stock/markets/bonds/boards/%s/securities/%s.json?iss.meta=off", c.baseURL, board, url.PathEscape(identifier))
		fallbackReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fallbackURL, nil)
		if err == nil {
			fallbackResp, err := c.httpClient.Do(fallbackReq)
			if err == nil {
				defer fallbackResp.Body.Close()
				if fallbackResp.StatusCode == http.StatusOK {
					return c.parseMarketDataResponse(identifier, fallbackResp.Body)
				}
			}
		}
		return nil, fmt.Errorf("moex marketdata error: status %d", resp.StatusCode)
	}

	return c.parseMarketDataResponse(identifier, resp.Body)
}

func (c *Client) parseMarketDataResponse(identifier string, r io.Reader) (*domain.MarketData, error) {
	var data issResponse
	if err := json.NewDecoder(r).Decode(&data); err != nil {
		return nil, err
	}

	// C-08: If both tables are empty, return nil, nil
	if len(data.Securities.Data) == 0 && len(data.Marketdata.Data) == 0 {
		return nil, nil
	}

	md := &domain.MarketData{
		ISIN:      identifier,
		UpdatedAt: time.Now(),
	}

	// 1. Parse marketdata table first to select the best trading board (C-09)
	mColIdx := make(map[string]int)
	for i, col := range data.Marketdata.Columns {
		mColIdx[strings.ToLower(col)] = i
	}

	// Board selection preference:
	// 1. Preferred trading board (TQCB/TQOB/TQIR/TQOD/TQOE) with trades or quotes
	// 2. Preferred trading board without trades
	// 3. First available row
	var bestRow []interface{}
	preferredBoards := []string{"TQCB", "TQOB", "TQIR", "TQOD", "TQOE"}

	// Pass 1: Preferred board with trades or last price
	for _, row := range data.Marketdata.Data {
		bIdx, hasB := mColIdx["boardid"]
		if !hasB || bIdx >= len(row) {
			continue
		}
		b := toString(row[bIdx])
		numTrades := 0
		if ntIdx, ok := mColIdx["numtrades"]; ok && ntIdx < len(row) {
			numTrades = toInt(row[ntIdx])
		}
		last := 0.0
		if lIdx, ok := mColIdx["last"]; ok && lIdx < len(row) {
			last = toFloat(row[lIdx])
		}
		for _, pref := range preferredBoards {
			if b == pref && (numTrades > 0 || last > 0) {
				bestRow = row
				break
			}
		}
		if bestRow != nil {
			break
		}
	}

	// Pass 2: Preferred board without trades
	if bestRow == nil {
		for _, row := range data.Marketdata.Data {
			bIdx, hasB := mColIdx["boardid"]
			if !hasB || bIdx >= len(row) {
				continue
			}
			b := toString(row[bIdx])
			for _, pref := range preferredBoards {
				if b == pref {
					bestRow = row
					break
				}
			}
			if bestRow != nil {
				break
			}
		}
	}

	// Pass 3: First row
	if bestRow == nil && len(data.Marketdata.Data) > 0 {
		bestRow = data.Marketdata.Data[0]
	}

	if bestRow != nil {
		if idx, ok := mColIdx["boardid"]; ok && idx < len(bestRow) {
			md.BoardID = toString(bestRow[idx])
		}
		if idx, ok := mColIdx["last"]; ok && idx < len(bestRow) {
			md.LastPricePct = toFloat(bestRow[idx])
		}
		if idx, ok := mColIdx["yield"]; ok && idx < len(bestRow) {
			md.YTM = toFloat(bestRow[idx])
		}
		if idx, ok := mColIdx["duration"]; ok && idx < len(bestRow) {
			md.DurationDays = toInt(bestRow[idx])
		}
		if idx, ok := mColIdx["valtoday"]; ok && idx < len(bestRow) {
			md.VolumeTodayRub = toFloat(bestRow[idx])
		}
		if idx, ok := mColIdx["numtrades"]; ok && idx < len(bestRow) {
			md.TradesCount = toInt(bestRow[idx])
		}
		if idx, ok := mColIdx["bid"]; ok && idx < len(bestRow) {
			md.BidPricePct = toFloat(bestRow[idx])
		}
		if idx, ok := mColIdx["offer"]; ok && idx < len(bestRow) {
			md.OfferPricePct = toFloat(bestRow[idx])
		}
		if idx, ok := mColIdx["tradingstatus"]; ok && idx < len(bestRow) {
			md.TradingStatus = toString(bestRow[idx])
		}
	}

	// 2. Parse securities table matching the selected board (C-09)
	sColIdx := make(map[string]int)
	for i, col := range data.Securities.Columns {
		sColIdx[strings.ToLower(col)] = i
	}

	var sRow []interface{}
	if md.BoardID != "" {
		for _, row := range data.Securities.Data {
			if bIdx, ok := sColIdx["boardid"]; ok && bIdx < len(row) {
				if toString(row[bIdx]) == md.BoardID {
					sRow = row
					break
				}
			}
		}
	}
	if sRow == nil && len(data.Securities.Data) > 0 {
		sRow = data.Securities.Data[0]
	}

	var faceValue float64 = 1000
	var prevClose float64
	var accruedInt float64
	currency := "RUB"

	if sRow != nil {
		if idx, ok := sColIdx["facevalue"]; ok && idx < len(sRow) {
			if fv := toFloat(sRow[idx]); fv > 0 {
				faceValue = fv
			}
		}
		if idx, ok := sColIdx["prevlegalcloseprice"]; ok && idx < len(sRow) {
			prevClose = toFloat(sRow[idx])
		}
		if idx, ok := sColIdx["accruedint"]; ok && idx < len(sRow) {
			accruedInt = toFloat(sRow[idx])
		}
		if md.BoardID == "" {
			if idx, ok := sColIdx["boardid"]; ok && idx < len(sRow) {
				md.BoardID = toString(sRow[idx])
			}
		}
		// Currency detection (C-09)
		if idx, ok := sColIdx["faceunit"]; ok && idx < len(sRow) {
			u := strings.ToUpper(toString(sRow[idx]))
			if u == "SUR" || u == "RUB" {
				currency = "RUB"
			} else if u != "" {
				currency = u
			}
		} else if idx, ok := sColIdx["currencyid"]; ok && idx < len(sRow) {
			c := strings.ToUpper(toString(sRow[idx]))
			if c == "SUR" || c == "RUB" {
				currency = "RUB"
			} else if c != "" {
				currency = c
			}
		}
	}

	md.AccruedCoupon = accruedInt
	md.Currency = currency

	// Fallback to previous close price if market is closed or no trades today
	if md.LastPricePct == 0 && prevClose > 0 {
		md.LastPricePct = prevClose
		md.IsPreviousClose = true
	}

	// C-08: If completely empty (no price, no prev close, no yield, no coupon, no volume, no trades)
	if md.LastPricePct == 0 && md.YTM == 0 && md.AccruedCoupon == 0 && md.VolumeTodayRub == 0 && md.TradesCount == 0 {
		return nil, nil
	}

	// Relative spread calculation (C-06, C-07)
	if md.BidPricePct > 0 && md.OfferPricePct > 0 && md.OfferPricePct >= md.BidPricePct {
		mid := (md.OfferPricePct + md.BidPricePct) / 2.0
		if mid > 0 {
			md.SpreadPct = roundFloat(((md.OfferPricePct-md.BidPricePct)/mid)*100.0, 2)
		}
		md.HasQuote = true
	} else {
		md.SpreadPct = 0
		md.HasQuote = false
	}

	// Clean and full prices in currency units
	if md.LastPricePct > 0 {
		md.LastPriceRub = roundFloat(faceValue*(md.LastPricePct/100.0), 2)
		md.FullPriceRub = roundFloat(md.LastPriceRub+md.AccruedCoupon, 2)
	}

	// Compute liquidity rating (C-05, C-06)
	md.Liquidity = domain.CalculateLiquidity(md.VolumeTodayRub, md.TradesCount, md.SpreadPct, md.HasQuote, md.IsPreviousClose)

	return md, nil
}

func roundFloat(val float64, precision int) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}

func (c *Client) enrichIssuerMetadata(ctx context.Context, bond *domain.Bond, identifier string) {
	searchTarget := identifier
	if bond.Ticker != "" {
		searchTarget = bond.Ticker
	} else if bond.ISIN != "" {
		searchTarget = bond.ISIN
	}

	// 1. Check in-memory cache (C-10)
	c.issuerCacheMu.RLock()
	cached, found := c.issuerCache[searchTarget]
	if !found && bond.ISIN != "" {
		cached, found = c.issuerCache[bond.ISIN]
	}
	if !found && bond.Ticker != "" {
		cached, found = c.issuerCache[bond.Ticker]
	}
	c.issuerCacheMu.RUnlock()

	if found {
		if bond.IssuerINN == "" {
			bond.IssuerINN = cached.inn
		}
		if bond.Issuer == "" {
			bond.Issuer = cached.issuer
		}
		if bond.PrimaryBoard == "" {
			bond.PrimaryBoard = cached.primaryBoard
		}
		return
	}

	reqURL := fmt.Sprintf("%s/securities.json?q=%s&iss.meta=off", c.baseURL, url.QueryEscape(searchTarget))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}

	var data issResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return
	}

	colIdx := make(map[string]int)
	for i, col := range data.Securities.Columns {
		colIdx[strings.ToLower(col)] = i
	}
	innIdx, hasINN := colIdx["emitent_inn"]
	titleIdx, hasTitle := colIdx["emitent_title"]
	boardIdx, hasBoard := colIdx["primary_boardid"]
	secidIdx, hasSecid := colIdx["secid"]
	isinIdx, hasISIN := colIdx["isin"]

	var targetRow []interface{}
	for _, row := range data.Securities.Data {
		var rSecid, rISIN string
		if hasSecid && secidIdx < len(row) {
			rSecid = toString(row[secidIdx])
		}
		if hasISIN && isinIdx < len(row) {
			rISIN = toString(row[isinIdx])
		}

		if strings.EqualFold(rSecid, searchTarget) || strings.EqualFold(rISIN, searchTarget) ||
			(bond.ISIN != "" && strings.EqualFold(rISIN, bond.ISIN)) ||
			(bond.Ticker != "" && strings.EqualFold(rSecid, bond.Ticker)) {
			targetRow = row
			break
		}
	}

	// Fallback to first row if only 1 row returned
	if targetRow == nil && len(data.Securities.Data) == 1 {
		targetRow = data.Securities.Data[0]
	}

	if targetRow != nil {
		var meta issuerMeta
		if hasINN && innIdx < len(targetRow) {
			meta.inn = toString(targetRow[innIdx])
		}
		if hasTitle && titleIdx < len(targetRow) {
			meta.issuer = toString(targetRow[titleIdx])
		}
		if hasBoard && boardIdx < len(targetRow) {
			meta.primaryBoard = toString(targetRow[boardIdx])
		}

		if bond.IssuerINN == "" {
			bond.IssuerINN = meta.inn
		}
		if bond.Issuer == "" {
			bond.Issuer = meta.issuer
		}
		if bond.PrimaryBoard == "" {
			bond.PrimaryBoard = meta.primaryBoard
		}

		// Store in cache (C-10)
		c.issuerCacheMu.Lock()
		c.issuerCache[searchTarget] = meta
		if bond.ISIN != "" {
			c.issuerCache[bond.ISIN] = meta
		}
		if bond.Ticker != "" {
			c.issuerCache[bond.Ticker] = meta
		}
		c.issuerCacheMu.Unlock()
	}
}

func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		var f float64
		fmt.Sscanf(val, "%f", &f)
		return f
	default:
		return 0
	}
}

func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		var i int
		fmt.Sscanf(val, "%d", &i)
		return i
	default:
		return 0
	}
}

func toString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

