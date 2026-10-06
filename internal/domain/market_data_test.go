package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateLiquidity(t *testing.T) {
	tests := []struct {
		name        string
		volumeRub   float64
		trades      int
		spread      float64
		hasQuote    bool
		isPrevClose bool
		expected    LiquidityLevel
	}{
		{
			name:        "Illiquid when zero trades in regular session",
			volumeRub:   50_000_000,
			trades:      0,
			spread:      0.1,
			hasQuote:    true,
			isPrevClose: false,
			expected:    LiquidityIlliquid,
		},
		{
			name:        "No trades today when using previous close off-hours",
			volumeRub:   0,
			trades:      0,
			spread:      0,
			hasQuote:    false,
			isPrevClose: true,
			expected:    LiquidityNoTrades,
		},
		{
			name:        "Illiquid when low volume",
			volumeRub:   50_000,
			trades:      10,
			spread:      0.2,
			hasQuote:    true,
			isPrevClose: false,
			expected:    LiquidityIlliquid,
		},
		{
			name:        "High liquidity",
			volumeRub:   15_000_000,
			trades:      150,
			spread:      0.2,
			hasQuote:    true,
			isPrevClose: false,
			expected:    LiquidityHigh,
		},
		{
			name:        "Capped at Low liquidity when no two-sided quote",
			volumeRub:   15_000_000,
			trades:      150,
			spread:      0,
			hasQuote:    false,
			isPrevClose: false,
			expected:    LiquidityLow,
		},
		{
			name:        "Medium liquidity",
			volumeRub:   5_000_000,
			trades:      30,
			spread:      1.0,
			hasQuote:    true,
			isPrevClose: false,
			expected:    LiquidityMedium,
		},
		{
			name:        "Low liquidity due to high spread",
			volumeRub:   15_000_000,
			trades:      150,
			spread:      2.0,
			hasQuote:    true,
			isPrevClose: false,
			expected:    LiquidityLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := CalculateLiquidity(tt.volumeRub, tt.trades, tt.spread, tt.hasQuote, tt.isPrevClose)
			assert.Equal(t, tt.expected, res)
		})
	}
}

func TestDurationHumanized(t *testing.T) {
	md := &MarketData{DurationDays: 180}
	assert.Equal(t, "180 дн.", md.DurationHumanized())

	md.DurationDays = 2450
	assert.Contains(t, md.DurationHumanized(), "2450 дн. (~6,7 года)")

	md.DurationDays = 0
	assert.Equal(t, "", md.DurationHumanized())
}

func TestCurrencySymbol(t *testing.T) {
	assert.Equal(t, "₽", (&MarketData{Currency: "RUB"}).CurrencySymbol())
	assert.Equal(t, "₽", (&MarketData{}).CurrencySymbol())
	assert.Equal(t, "$", (&MarketData{Currency: "USD"}).CurrencySymbol())
	assert.Equal(t, "€", (&MarketData{Currency: "EUR"}).CurrencySymbol())
	assert.Equal(t, "¥", (&MarketData{Currency: "CNY"}).CurrencySymbol())
}
