package bdd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
	"telegram-bot-moex/internal/domain"
)

func registerBalanceSteps(sc *godog.ScenarioContext, tc *TestContext) {
	sc.Step(`^the bot is running with financial reporting support$`, func() error {
		if tc.bot != nil {
			tc.bot.SetMessageSender(func(ctx context.Context, chatID int64, text string) error {
				tc.formattedMessage = text
				return nil
			})
		}
		return nil
	})

	sc.Step(`^bond "([^"]*)" has issuer INN "([^"]*)" and name "([^"]*)"$`, func(identifier, inn, name string) error {
		b := domain.Bond{
			ISIN:      identifier,
			Ticker:    identifier,
			Name:      name,
			Issuer:    name,
			IssuerINN: inn,
			Exchange:  domain.ExchangeMOEX,
		}
		tc.mockBondProv.bonds[identifier] = b
		return nil
	})

	sc.Step(`^the financial provider has reports for INN "([^"]*)":$`, func(inn string, table *godog.Table) error {
		bal := &domain.IssuerBalance{
			INN:      inn,
			Standard: "РСБУ",
			Source:   "ГИР БО (ФНС)",
		}

		for _, row := range table.Rows {
			k := strings.TrimSpace(row.Cells[0].Value)
			v := strings.TrimSpace(row.Cells[1].Value)

			switch k {
			case "ReportYear":
				y, _ := strconv.Atoi(v)
				bal.ReportYear = y
			case "TotalAssets":
				val, _ := strconv.ParseFloat(v, 64)
				bal.TotalAssets = val
			case "Equity":
				val, _ := strconv.ParseFloat(v, 64)
				bal.Equity = val
			case "LongTermDebt":
				val, _ := strconv.ParseFloat(v, 64)
				bal.LongTermDebt = val
			case "ShortTermDebt":
				val, _ := strconv.ParseFloat(v, 64)
				bal.ShortTermDebt = val
			case "CashEquivalents":
				val, _ := strconv.ParseFloat(v, 64)
				bal.CashEquivalents = val
			case "Revenue":
				val, _ := strconv.ParseFloat(v, 64)
				bal.Revenue = val
			case "NetProfit":
				val, _ := strconv.ParseFloat(v, 64)
				bal.NetProfit = val
			case "AuditorName":
				bal.AuditorName = v
			}
		}

		bal.CalculateMetrics()
		tc.mockBfoProv.reports[inn] = bal
		return nil
	})

	sc.Step(`^the user sends command "([^"]*)"$`, func(cmd string) error {
		tc.formattedMessage = ""
		tc.bot.HandleIncomingMessage(tc.ctx, 12345, 67890, cmd)
		return nil
	})

	sc.Step(`^the bot response should contain "([^"]*)"$`, func(substring string) error {
		if !messageContains(tc.formattedMessage, substring) {
			return fmt.Errorf("expected bot response to contain %q, but got:\n%s", substring, tc.formattedMessage)
		}
		return nil
	})

	sc.Step(`^the bot response should contain ratio "([^"]*)"$`, func(ratio string) error {
		if !messageContains(tc.formattedMessage, ratio) {
			return fmt.Errorf("expected bot response to contain ratio %q, but got:\n%s", ratio, tc.formattedMessage)
		}
		return nil
	})

	sc.Step(`^the bot response should contain risk evaluation "([^"]*)"$`, func(risk string) error {
		if !messageContains(tc.formattedMessage, risk) {
			return fmt.Errorf("expected bot response to contain risk %q, but got:\n%s", risk, tc.formattedMessage)
		}
		return nil
	})

	sc.Step(`^the bot response should contain auditor "([^"]*)"$`, func(auditor string) error {
		if !messageContains(tc.formattedMessage, auditor) {
			return fmt.Errorf("expected bot response to contain auditor %q, but got:\n%s", auditor, tc.formattedMessage)
		}
		return nil
	})

	sc.Step(`^the financial statement for "([^"]*)" should be stored in the cache$`, func(inn string) error {
		if tc.finCache == nil {
			return fmt.Errorf("finCache is nil")
		}
		if _, ok := tc.finCache.Get(inn); !ok {
			return fmt.Errorf("expected report for INN %s to be in cache", inn)
		}
		return nil
	})

	sc.Step(`^the provider report for "([^"]*)" is cleared$`, func(inn string) error {
		delete(tc.mockBfoProv.reports, inn)
		return nil
	})

	sc.Step(`^issuer INN "([^"]*)" is registered as a credit organization$`, func(inn string) error {
		tc.mockBfoProv.reports[inn] = &domain.IssuerBalance{
			INN:         inn,
			IssuerName:  "ПАО Сбербанк",
			IsCreditOrg: true,
			RiskLevel:   domain.RiskBank,
			Standard:    "РСБУ (ЦБ РФ)",
			Source:      "Банк России (101/102)",
		}
		return nil
	})

	sc.Step(`^the financial provider has no reports for INN "([^"]*)"$`, func(inn string) error {
		delete(tc.mockBfoProv.reports, inn)
		return nil
	})

	sc.Step(`^the bot response should not contain "([^"]*)"$`, func(substring string) error {
		if messageContains(tc.formattedMessage, substring) {
			return fmt.Errorf("expected bot response NOT to contain %q, but got:\n%s", substring, tc.formattedMessage)
		}
		return nil
	})
}
