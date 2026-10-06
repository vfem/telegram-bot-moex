package telegram

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"telegram-bot-moex/internal/domain"
)

func assertValidMarkdownV2(t *testing.T, msg string) {
	reserved := "_*[]()~`>#+-=|{}.!"
	isReserved := func(r rune) bool {
		return strings.ContainsRune(reserved, r)
	}

	inCode := false
	runes := []rune(msg)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\\' {
			// Escaped character, skip it and the following rune
			if i+1 < len(runes) {
				i++
				continue
			} else {
				t.Errorf("trailing backslash at index %d in: %s", i, msg)
			}
			continue
		}

		if r == '`' {
			inCode = !inCode
			continue
		}

		if inCode {
			// Inside code span, characters are literal
			continue
		}

		// Formatting delimiters for bold and italic are allowed
		if r == '*' || r == '_' {
			continue
		}

		if isReserved(r) {
			start := i - 15
			if start < 0 {
				start = 0
			}
			end := i + 15
			if end > len(runes) {
				end = len(runes)
			}
			t.Errorf("unescaped reserved MarkdownV2 character %q at index %d (context: %q)",
				string(r), i, string(runes[start:end]))
		}
	}
	assert.False(t, inCode, "unclosed code block in message")
}

func TestFormatBondPassport_WithMarketData(t *testing.T) {
	matDate, _ := time.Parse("2006-01-02", "2041-05-15")
	bond := &domain.Bond{
		ISIN:           "SU26238RMFS4",
		Ticker:         "SU26238",
		Name:           "ОФЗ 26238",
		Issuer:         "Минфин РФ",
		IssuerINN:      "7710168360",
		Exchange:       domain.ExchangeMOEX,
		NominalValue:   1000,
		Currency:       "RUB",
		CouponRatePct:  7.1,
		CouponsPerYear: 2,
		MaturityDate:   matDate,
	}

	md := &domain.MarketData{
		ISIN:           "SU26238RMFS4",
		LastPricePct:   54.20,
		LastPriceRub:   542.00,
		AccruedCoupon:  33.80,
		FullPriceRub:   575.80,
		YTM:            17.85,
		DurationDays:   2450,
		VolumeTodayRub: 142500000,
		TradesCount:    382,
		SpreadPct:      0.08,
		HasQuote:       true,
		Liquidity:      domain.LiquidityHigh,
	}

	payDate, _ := time.Parse("2006-01-02", "2027-04-15")
	payments := []domain.Payment{
		{
			ISIN:   "SU26238RMFS4",
			Date:   payDate,
			Amount: 35.40,
			Type:   domain.PaymentCoupon,
		},
	}

	msg := FormatBondPassport(bond, md, payments)

	// Validate strict MarkdownV2 escaping (C-01)
	assertValidMarkdownV2(t, msg)

	assert.Contains(t, msg, "ОФЗ 26238")
	assert.Contains(t, msg, "ИНН: `7710168360`")
	assert.Contains(t, msg, "📈 *Торги и ликвидность:*")
	assert.Contains(t, msg, "• Цена: *54\\.20%* \\(542\\.00 ₽\\) • НКД: *33\\.80 ₽*")
	assert.Contains(t, msg, "• Доходность \\(YTM\\): *17\\.85%*")
	assert.Contains(t, msg, "2450 дн\\. \\(\\~6,7 года\\)")
	assert.Contains(t, msg, "*142\\.5 млн ₽* \\(382 сделки\\)")
	assert.Contains(t, msg, "🔥 *Высокая* \\(спред: *0\\.08%*\\)")
	assert.Contains(t, msg, "📅 *Ближайшие выплаты:*")
}

func TestFormatBondPassport_WithoutMarketData(t *testing.T) {
	bond := &domain.Bond{
		ISIN:         "RU000A105X64",
		Name:         "КарМани",
		Exchange:     domain.ExchangeSPBE,
		NominalValue: 1000,
		Currency:     "RUB",
	}

	msg := FormatBondPassport(bond, nil, nil)
	assertValidMarkdownV2(t, msg)
	assert.Contains(t, msg, "КарМани")
	assert.NotContains(t, msg, "📈 *Торги и ликвидность:*")
}

func TestFormatBondPassport_ZeroCouponPayment(t *testing.T) {
	// Floating coupon with undetermined amount (C-17)
	bond := &domain.Bond{
		ISIN:     "RU000A107456",
		Name:     "ЕвроТранс",
		Currency: "RUB",
	}

	payDate, _ := time.Parse("2006-01-02", "2026-10-21")
	payments := []domain.Payment{
		{
			ISIN:   "RU000A107456",
			Date:   payDate,
			Amount: 0.0,
			Type:   domain.PaymentCoupon,
		},
	}

	msg := FormatBondPassport(bond, nil, payments)
	assertValidMarkdownV2(t, msg)
	assert.Contains(t, msg, "_размер уточняется_")
	assert.NotContains(t, msg, "*0.00 RUB*")
}

func TestFormatBondPassport_MissingQuoteAndOffHours(t *testing.T) {
	// C-05: Off-hours with no trades today
	bond := &domain.Bond{
		ISIN:     "SU26238RMFS4",
		Name:     "ОФЗ 26238",
		Currency: "RUB",
	}

	md := &domain.MarketData{
		ISIN:            "SU26238RMFS4",
		LastPricePct:    54.20,
		LastPriceRub:    542.0,
		IsPreviousClose: true,
		AccruedCoupon:   33.80,
		YTM:             17.85,
		DurationDays:    2450,
		TradesCount:     0,
		VolumeTodayRub:  0,
		HasQuote:        false,
		Liquidity:       domain.LiquidityNoTrades,
	}

	msg := FormatBondPassport(bond, md, nil)
	assertValidMarkdownV2(t, msg)
	assert.Contains(t, msg, "• Ликвидность: ⏳ *нет сделок сегодня*")
	assert.Contains(t, msg, "\\(цена закр\\.\\)")
}

func TestFormatBondPassport_OnlyYTMWithoutPrice(t *testing.T) {
	// C-11: Valid data with YTM/НКД but LastPricePct == 0
	bond := &domain.Bond{
		ISIN:     "RU000A10NEW0",
		Name:     "Новый Выпуск",
		Currency: "RUB",
	}

	md := &domain.MarketData{
		ISIN:          "RU000A10NEW0",
		LastPricePct:  0,
		AccruedCoupon: 5.50,
		YTM:           19.5,
		DurationDays:  360,
		HasQuote:      false,
		Liquidity:     domain.LiquidityIlliquid,
	}

	msg := FormatBondPassport(bond, md, nil)
	assertValidMarkdownV2(t, msg)
	assert.Contains(t, msg, "• Цена: *н/д* • НКД: *5\\.50 ₽*")
	assert.Contains(t, msg, "• Доходность \\(YTM\\): *19\\.50%*")
	assert.Contains(t, msg, "спред: н/д")
}

func TestFormatDatePaymentsAndAnnouncements(t *testing.T) {
	date, _ := time.Parse("2006-01-02", "2026-10-15")
	payments := []domain.Payment{
		{
			ISIN:     "RU000A107456",
			BondName: "ЕвроТранс",
			Amount:   33.29,
			Type:     domain.PaymentCoupon,
		},
	}

	pMsg := FormatDatePayments(date, payments)
	assertValidMarkdownV2(t, pMsg)

	ann := &domain.Announcement{
		Title:        "Газпром Капитал БО-003P-01",
		Issuer:       "ООО Газпром Капитал",
		Exchange:     domain.ExchangeMOEX,
		VolumeRUB:    15_000_000_000,
		TargetCoupon: "15.5%",
		BookDate:     date,
	}

	aMsg := FormatAnnouncement(ann)
	require.NotEmpty(t, aMsg)
	assertValidMarkdownV2(t, aMsg)
}
