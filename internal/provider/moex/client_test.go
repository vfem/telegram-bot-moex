package moex

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"telegram-bot-moex/internal/domain"
)

type mockHTTPClient struct {
	responses map[string]string
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	url := req.URL.String()
	for k, v := range m.responses {
		if strings.Contains(url, k) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(v)),
			}, nil
		}
	}
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader("{}")),
	}, nil
}

func TestGetMarketData_Success(t *testing.T) {
	mockJSON := `{
		"securities": {
			"columns": ["SECID", "BOARDID", "FACEVALUE", "ACCRUEDINT", "PREVLEGALCLOSEPRICE"],
			"data": [
				["RU000A107456", "TQCB", 1000.0, 14.20, 98.20]
			]
		},
		"marketdata": {
			"columns": ["SECID", "BOARDID", "LAST", "YIELD", "DURATION", "VALTODAY", "NUMTRADES", "BID", "OFFER", "SPREAD", "TRADINGSTATUS"],
			"data": [
				["RU000A107456", "TQCB", 98.40, 21.45, 420, 14800000.0, 382, 98.30, 98.45, 0.15, "T"]
			]
		}
	}`

	client := NewClientWithBaseURL("https://iss.moex.com/iss", &mockHTTPClient{
		responses: map[string]string{
			"RU000A107456": mockJSON,
		},
	})

	md, err := client.GetMarketData(context.Background(), "RU000A107456")
	require.NoError(t, err)
	require.NotNil(t, md)

	assert.Equal(t, "RU000A107456", md.ISIN)
	assert.Equal(t, "TQCB", md.BoardID)
	assert.Equal(t, 98.40, md.LastPricePct)
	assert.Equal(t, 984.0, md.LastPriceRub)
	assert.Equal(t, 14.20, md.AccruedCoupon)
	assert.Equal(t, 998.20, md.FullPriceRub)
	assert.Equal(t, 21.45, md.YTM)
	assert.Equal(t, 420, md.DurationDays)
	assert.Equal(t, 14800000.0, md.VolumeTodayRub)
	assert.Equal(t, 382, md.TradesCount)
	assert.Equal(t, 0.15, md.SpreadPct)
	assert.Equal(t, domain.LiquidityHigh, md.Liquidity)
	assert.False(t, md.IsPreviousClose)
}

func TestGetMarketData_FallbackToPreviousClose(t *testing.T) {
	mockJSON := `{
		"securities": {
			"columns": ["SECID", "BOARDID", "FACEVALUE", "ACCRUEDINT", "PREVLEGALCLOSEPRICE"],
			"data": [
				["SU26238RMFS4", "TQOB", 1000.0, 33.80, 54.20]
			]
		},
		"marketdata": {
			"columns": ["SECID", "BOARDID", "LAST", "YIELD", "DURATION", "VALTODAY", "NUMTRADES", "BID", "OFFER", "SPREAD", "TRADINGSTATUS"],
			"data": [
				["SU26238RMFS4", "TQOB", 0.0, 17.85, 2450, 0.0, 0, 0.0, 0.0, 0.0, "N"]
			]
		}
	}`

	client := NewClientWithBaseURL("https://iss.moex.com/iss", &mockHTTPClient{
		responses: map[string]string{
			"SU26238RMFS4": mockJSON,
		},
	})

	md, err := client.GetMarketData(context.Background(), "SU26238RMFS4")
	require.NoError(t, err)
	require.NotNil(t, md)

	assert.Equal(t, 54.20, md.LastPricePct)
	assert.True(t, md.IsPreviousClose)
	assert.Equal(t, 542.0, md.LastPriceRub)
	assert.Equal(t, domain.LiquidityNoTrades, md.Liquidity)
}

func TestGetMarketData_EmptyResponseReturnsNil(t *testing.T) {
	mockJSON := `{
		"securities": {"columns": ["SECID", "BOARDID"], "data": []},
		"marketdata": {"columns": ["SECID", "BOARDID"], "data": []}
	}`

	client := NewClientWithBaseURL("https://iss.moex.com/iss", &mockHTTPClient{
		responses: map[string]string{
			"UNKNOWN": mockJSON,
		},
	})

	md, err := client.GetMarketData(context.Background(), "UNKNOWN")
	require.NoError(t, err)
	assert.Nil(t, md)
}

func TestGetMarketData_MultiBoardAndCurrency(t *testing.T) {
	mockJSON := `{
		"securities": {
			"columns": ["SECID", "BOARDID", "FACEVALUE", "FACEUNIT", "ACCRUEDINT", "PREVLEGALCLOSEPRICE"],
			"data": [
				["RU000A10CNY1", "TQCB", 1000.0, "CNY", 5.0, 99.0],
				["RU000A10CNY1", "TQIR", 1000.0, "CNY", 12.50, 101.20]
			]
		},
		"marketdata": {
			"columns": ["SECID", "BOARDID", "LAST", "YIELD", "DURATION", "VALTODAY", "NUMTRADES", "BID", "OFFER", "TRADINGSTATUS"],
			"data": [
				["RU000A10CNY1", "TQCB", 0.0, 0.0, 0, 0.0, 0, 0.0, 0.0, "N"],
				["RU000A10CNY1", "TQIR", 101.50, 10.5, 300, 5000000.0, 45, 101.30, 101.70, "T"]
			]
		}
	}`

	client := NewClientWithBaseURL("https://iss.moex.com/iss", &mockHTTPClient{
		responses: map[string]string{
			"RU000A10CNY1": mockJSON,
		},
	})

	md, err := client.GetMarketData(context.Background(), "RU000A10CNY1")
	require.NoError(t, err)
	require.NotNil(t, md)

	assert.Equal(t, "TQIR", md.BoardID)
	assert.Equal(t, "CNY", md.Currency)
	assert.Equal(t, 101.50, md.LastPricePct)
	assert.Equal(t, 1015.0, md.LastPriceRub)
	assert.Equal(t, 12.50, md.AccruedCoupon)
	assert.Equal(t, 10.5, md.YTM)
	assert.True(t, md.HasQuote)
	assert.Equal(t, domain.LiquidityMedium, md.Liquidity)
}

type countingMockHTTPClient struct {
	responses map[string]string
	callCount int
}

func (m *countingMockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	m.callCount++
	url := req.URL.String()
	for k, v := range m.responses {
		if strings.Contains(url, k) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(v)),
			}, nil
		}
	}
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader("{}")),
	}, nil
}

func TestEnrichIssuerMetadata_ExactMatchAndCache(t *testing.T) {
	// Search returns 2 rows: target is the SECOND row
	searchJSON := `{
		"securities": {
			"columns": ["secid", "isin", "emitent_inn", "emitent_title", "primary_boardid"],
			"data": [
				["OTHER1", "RU000OTHER1", "1111111111", "Other Issuer", "TQCB"],
				["MYBOND", "RU000MYBOND0", "7712345678", "Target Issuer", "TQCB"]
			]
		}
	}`

	counter := &countingMockHTTPClient{
		responses: map[string]string{
			"securities.json": searchJSON,
		},
	}
	client := NewClientWithBaseURL("https://iss.moex.com/iss", counter)

	bond := &domain.Bond{
		ISIN:   "RU000MYBOND0",
		Ticker: "MYBOND",
	}

	// First call: queries network
	client.enrichIssuerMetadata(context.Background(), bond, "MYBOND")
	assert.Equal(t, "7712345678", bond.IssuerINN)
	assert.Equal(t, "Target Issuer", bond.Issuer)
	assert.Equal(t, 1, counter.callCount)

	// Second call with empty metadata: should hit cache and NOT increment callCount
	bond2 := &domain.Bond{
		ISIN:   "RU000MYBOND0",
		Ticker: "MYBOND",
	}
	client.enrichIssuerMetadata(context.Background(), bond2, "MYBOND")
	assert.Equal(t, "7712345678", bond2.IssuerINN)
	assert.Equal(t, "Target Issuer", bond2.Issuer)
	assert.Equal(t, 1, counter.callCount, "second call should be served from cache without HTTP request")
}
