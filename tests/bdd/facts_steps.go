package bdd

import (
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"telegram-bot-moex/internal/domain"
)

func registerFactsSteps(sc *godog.ScenarioContext, tc *TestContext) {
	sc.Step(`^a new disclosure fact for issuer INN "([^"]*)":$`, func(inn string, table *godog.Table) error {
		fact := domain.MaterialFact{
			INN: inn,
		}
		for _, row := range table.Rows {
			k := strings.TrimSpace(row.Cells[0].Value)
			v := strings.TrimSpace(row.Cells[1].Value)
			switch k {
			case "Title":
				fact.Title = v
			case "PublishedAt":
				t, _ := time.Parse("2006-01-02", v)
				fact.PublishedAt = t
			}
		}
		tc.currentFact = fact
		return nil
	})

	sc.Step(`^the facts classifier processes the announcement$`, func() error {
		tc.currentFact.Classify()
		return nil
	})

	sc.Step(`^the severity should be "([^"]*)"$`, func(wantSeverity string) error {
		if string(tc.currentFact.Severity) != wantSeverity {
			return fmt.Errorf("expected severity %q, got %q", wantSeverity, tc.currentFact.Severity)
		}
		return nil
	})

	sc.Step(`^the category should be "([^"]*)"$`, func(wantCategory string) error {
		if string(tc.currentFact.Category) != wantCategory {
			return fmt.Errorf("expected category %q, got %q", wantCategory, tc.currentFact.Category)
		}
		return nil
	})

	sc.Step(`^the disclosure provider has facts for INN "([^"]*)":$`, func(inn string, table *godog.Table) error {
		var list []domain.MaterialFact
		for idx, row := range table.Rows {
			if idx == 0 && strings.EqualFold(strings.TrimSpace(row.Cells[0].Value), "PublishedAt") {
				continue
			}
			dateStr := strings.TrimSpace(row.Cells[0].Value)
			title := strings.TrimSpace(row.Cells[1].Value)
			var urlStr string
			if len(row.Cells) > 2 {
				urlStr = strings.TrimSpace(row.Cells[2].Value)
			}
			t, _ := time.Parse("2006-01-02", dateStr)
			f := domain.MaterialFact{
				ID:          fmt.Sprintf("%s-%d", inn, idx),
				INN:         inn,
				Title:       title,
				PublishedAt: t,
				URL:         urlStr,
			}
			f.Classify()
			list = append(list, f)
		}
		tc.mockFactsProv.facts[inn] = list
		return nil
	})

	sc.Step(`^the material facts for "([^"]*)" should be stored in the cache$`, func(inn string) error {
		if tc.factsCache == nil {
			return fmt.Errorf("factsCache is nil")
		}
		if _, ok := tc.factsCache.Get(inn); !ok {
			return fmt.Errorf("expected facts for INN %s to be stored in cache", inn)
		}
		return nil
	})

	sc.Step(`^the disclosure provider facts for "([^"]*)" are cleared$`, func(inn string) error {
		delete(tc.mockFactsProv.facts, inn)
		return nil
	})

	sc.Step(`^the disclosure provider has no facts for INN "([^"]*)"$`, func(inn string) error {
		delete(tc.mockFactsProv.facts, inn)
		return nil
	})

	sc.Step(`^a material fact is detected for bond "([^"]*)":$`, func(isin string, table *godog.Table) error {
		fact := domain.MaterialFact{
			ID:   "alert-" + isin,
			ISIN: isin,
		}
		for _, row := range table.Rows {
			k := strings.TrimSpace(row.Cells[0].Value)
			v := strings.TrimSpace(row.Cells[1].Value)
			switch k {
			case "Title":
				fact.Title = v
			case "PublishedAt":
				t, _ := time.Parse("2006-01-02", v)
				fact.PublishedAt = t
			}
		}
		fact.Classify()

		bond, ok := tc.mockBondProv.bonds[isin]
		if !ok {
			bond = domain.Bond{ISIN: isin, Name: isin}
		}

		_, err := tc.notifier.NotifyMaterialFact(tc.ctx, bond, fact)
		return err
	})

	sc.Step(`^no notification message is queued for user (\d+)$`, func(userID int64) error {
		for _, msg := range tc.notifier.Outbox {
			if msg.TelegramID == userID {
				return fmt.Errorf("expected no notification for user %d, but found message: %s", userID, msg.Text)
			}
		}
		return nil
	})
}
