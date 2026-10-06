package bdd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/telegram"
)

func registerMarketDataSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Step(`^the MOEX provider has market quote for "([^"]*)":$`, func(isin string, table *godog.Table) error {
		md := &domain.MarketData{
			ISIN:      isin,
			BoardID:   "TQCB",
			Currency:  "RUB",
			UpdatedAt: time.Now(),
		}
		if strings.HasPrefix(isin, "SU") {
			md.BoardID = "TQOB"
		}

		var prevClose float64
		var faceValue float64 = 1000

		for _, row := range table.Rows {
			k := strings.TrimSpace(row.Cells[0].Value)
			v := strings.TrimSpace(row.Cells[1].Value)

			switch k {
			case "LastPricePct":
				val, _ := strconv.ParseFloat(v, 64)
				md.LastPricePct = val
			case "PrevClosePrice":
				val, _ := strconv.ParseFloat(v, 64)
				prevClose = val
			case "AccruedCoupon":
				val, _ := strconv.ParseFloat(v, 64)
				md.AccruedCoupon = val
			case "YTM":
				val, _ := strconv.ParseFloat(v, 64)
				md.YTM = val
			case "DurationDays":
				val, _ := strconv.Atoi(v)
				md.DurationDays = val
			case "VolumeTodayRub":
				val, _ := strconv.ParseFloat(v, 64)
				md.VolumeTodayRub = val
			case "TradesCount":
				val, _ := strconv.Atoi(v)
				md.TradesCount = val
			case "BidPricePct":
				val, _ := strconv.ParseFloat(v, 64)
				md.BidPricePct = val
			case "OfferPricePct":
				val, _ := strconv.ParseFloat(v, 64)
				md.OfferPricePct = val
			case "SpreadPct":
				val, _ := strconv.ParseFloat(v, 64)
				md.SpreadPct = val
			}
		}

		if md.LastPricePct == 0 && prevClose > 0 {
			md.LastPricePct = prevClose
			md.IsPreviousClose = true
		}

		if md.LastPricePct > 0 {
			md.LastPriceRub = faceValue * (md.LastPricePct / 100.0)
			md.FullPriceRub = md.LastPriceRub + md.AccruedCoupon
		}

		hasQuote := md.BidPricePct > 0 && md.OfferPricePct > 0
		md.HasQuote = hasQuote
		if hasQuote && md.SpreadPct == 0 && md.OfferPricePct >= md.BidPricePct {
			mid := (md.OfferPricePct + md.BidPricePct) / 2.0
			if mid > 0 {
				md.SpreadPct = ((md.OfferPricePct - md.BidPricePct) / mid) * 100.0
			}
		}

		md.Liquidity = domain.CalculateLiquidity(md.VolumeTodayRub, md.TradesCount, md.SpreadPct, md.HasQuote, md.IsPreviousClose)

		tc.mockBondProv.marketData[isin] = md
		return nil
	})

	ctx.Step(`^the bot response should contain market price "([^"]*)"$`, func(expectedPrice string) error {
		if !messageContains(tc.formattedMessage, expectedPrice) {
			return fmt.Errorf("expected formatted message to contain price %q, got: %s", expectedPrice, tc.formattedMessage)
		}
		return nil
	})

	ctx.Step(`^the bot response should contain clean price in RUB "([^"]*)"$`, func(expectedRub string) error {
		if !messageContains(tc.formattedMessage, expectedRub) {
			return fmt.Errorf("expected message to contain clean price %q, got: %s", expectedRub, tc.formattedMessage)
		}
		return nil
	})

	ctx.Step(`^the bot response should contain YTM "([^"]*)"$`, func(expectedYTM string) error {
		if !messageContains(tc.formattedMessage, expectedYTM) {
			return fmt.Errorf("expected message to contain YTM %q, got: %s", expectedYTM, tc.formattedMessage)
		}
		return nil
	})

	ctx.Step(`^the bot response should display liquidity "([^"]*)"$`, func(expectedLiq string) error {
		if !messageContains(tc.formattedMessage, expectedLiq) {
			return fmt.Errorf("expected message to display liquidity %q, got: %s", expectedLiq, tc.formattedMessage)
		}
		return nil
	})

	ctx.Step(`^the bot response should contain trades count (\d+)$`, func(trades int) error {
		expectedStr := strconv.Itoa(trades)
		if !messageContains(tc.formattedMessage, expectedStr) || !strings.Contains(tc.formattedMessage, "сдел") {
			return fmt.Errorf("expected message to contain trades count %d, got: %s", trades, tc.formattedMessage)
		}
		return nil
	})

	ctx.Step(`^the bot response should indicate "([^"]*)"$`, func(marker string) error {
		if !messageContains(tc.formattedMessage, marker) {
			return fmt.Errorf("expected message to indicate %q, got: %s", marker, tc.formattedMessage)
		}
		return nil
	})

	// C-03: Directly assert against tc.marketCache to verify true cache state
	ctx.Step(`^the market data for "([^"]*)" should be present in the cache$`, func(isin string) error {
		if tc.marketCache == nil {
			return fmt.Errorf("market cache is not initialized in TestContext")
		}
		cached, ok := tc.marketCache.Get(isin)
		if !ok || cached == nil {
			return fmt.Errorf("expected isin %s to be present in cache", isin)
		}
		return nil
	})

	ctx.Step(`^the provider quote for "([^"]*)" changes to (\d+\.\d+)%$`, func(isin string, priceStr string) error {
		newPrice, _ := strconv.ParseFloat(priceStr, 64)
		if md, ok := tc.mockBondProv.marketData[isin]; ok {
			mdCopy := *md
			mdCopy.LastPricePct = newPrice
			mdCopy.LastPriceRub = 1000 * (newPrice / 100.0)
			tc.mockBondProv.marketData[isin] = &mdCopy
		}
		return nil
	})

	ctx.Step(`^the user requests bond details for "([^"]*)" within cache TTL$`, func(isin string) error {
		bond, marketData, payments, err := tc.bondService.GetBondDetails(tc.ctx, isin)
		tc.currentBond = bond
		tc.currentMarketData = marketData
		tc.currentPayments = payments
		tc.lastError = err
		if bond != nil {
			tc.formattedMessage = telegram.FormatBondPassport(bond, marketData, payments)
		}
		return nil
	})

	// C-02: Safe assertion without assert.Contains(nil, ...)
	ctx.Step(`^the bot response should still contain cached price "([^"]*)"$`, func(cachedPrice string) error {
		if !messageContains(tc.formattedMessage, cachedPrice) {
			return fmt.Errorf("expected cached price %s, got message: %s", cachedPrice, tc.formattedMessage)
		}
		return nil
	})
}

func messageContains(msg, substr string) bool {
	if strings.Contains(msg, substr) {
		return true
	}
	escaped := escapeMarkdownText(substr)
	if strings.Contains(msg, escaped) {
		return true
	}
	unescaped := strings.ReplaceAll(msg, "\\", "")
	return strings.Contains(unescaped, substr)
}

func escapeMarkdownText(s string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(s)
}
