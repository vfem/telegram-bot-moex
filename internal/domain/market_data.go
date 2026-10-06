package domain

import (
	"fmt"
	"strings"
	"time"
)

// LiquidityLevel indicates the ease of buying and selling the security on the open market.
type LiquidityLevel string

const (
	LiquidityHigh     LiquidityLevel = "Высокая"
	LiquidityMedium   LiquidityLevel = "Средняя"
	LiquidityLow      LiquidityLevel = "Низкая"
	LiquidityIlliquid LiquidityLevel = "Неликвид"
	LiquidityNoTrades LiquidityLevel = "нет сделок сегодня"
)

// MarketData represents live trading information for a bond.
type MarketData struct {
	ISIN            string         `json:"isin"`
	BoardID         string         `json:"board_id"`
	LastPricePct    float64        `json:"last_price_pct"`    // % of nominal
	LastPriceRub    float64        `json:"last_price_rub"`    // Clean price in currency units
	IsPreviousClose bool           `json:"is_previous_close"` // True if from PREVLEGALCLOSEPRICE
	AccruedCoupon   float64        `json:"accrued_coupon"`    // НКД (in currency units)
	FullPriceRub    float64        `json:"full_price_rub"`    // Dirty price (Clean + НКД)
	Currency        string         `json:"currency"`          // Currency code ("RUB", "USD", "CNY", etc.)
	YTM             float64        `json:"ytm"`               // Yield to maturity (% per annum)
	DurationDays    int            `json:"duration_days"`     // Duration in days
	VolumeTodayRub  float64        `json:"volume_today_rub"`  // Daily turnover in RUB
	TradesCount     int            `json:"trades_count"`      // Number of trades today
	BidPricePct     float64        `json:"bid_price_pct"`     // Best bid (% of nominal)
	OfferPricePct   float64        `json:"offer_price_pct"`   // Best offer (% of nominal)
	SpreadPct       float64        `json:"spread_pct"`        // Relative spread ((Offer - Bid) / MidPrice) * 100 (%)
	HasQuote        bool           `json:"has_quote"`         // True if two-sided quote exists (Bid > 0 && Offer > 0)
	Liquidity       LiquidityLevel `json:"liquidity"`         // Evaluated liquidity rating
	TradingStatus   string         `json:"trading_status"`    // e.g. "Торгуется", "Торги закрыты"
	UpdatedAt       time.Time      `json:"updated_at"`
}

// CurrencySymbol returns the display symbol for the security currency.
func (md *MarketData) CurrencySymbol() string {
	switch md.Currency {
	case "USD":
		return "$"
	case "EUR":
		return "€"
	case "CNY":
		return "¥"
	default:
		return "₽"
	}
}

// CalculateLiquidity evaluates liquidity category based on volume, trades, spread, quote availability, and session state.
func CalculateLiquidity(volTodayRub float64, tradesCount int, spreadPct float64, hasQuote bool, isPreviousClose bool) LiquidityLevel {
	if isPreviousClose && tradesCount == 0 {
		return LiquidityNoTrades
	}
	if volTodayRub < 100_000 || tradesCount == 0 {
		return LiquidityIlliquid
	}
	// Without two-sided quote, cap liquidity at Low (C-06)
	if !hasQuote {
		return LiquidityLow
	}
	if volTodayRub > 10_000_000 && tradesCount >= 100 && spreadPct < 0.5 {
		return LiquidityHigh
	}
	if volTodayRub >= 1_000_000 && tradesCount >= 20 && spreadPct <= 1.5 {
		return LiquidityMedium
	}
	return LiquidityLow
}

// DurationHumanized returns a human-readable duration string with Russian decimal comma and genitive singular (C-14).
func (md *MarketData) DurationHumanized() string {
	if md.DurationDays <= 0 {
		return ""
	}
	if md.DurationDays < 365 {
		return fmt.Sprintf("%d дн.", md.DurationDays)
	}
	years := float64(md.DurationDays) / 365.25
	yearsStr := strings.Replace(fmt.Sprintf("%.1f", years), ".", ",", 1)
	return fmt.Sprintf("%d дн. (~%s года)", md.DurationDays, yearsStr)
}
