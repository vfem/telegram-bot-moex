package bdd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/stretchr/testify/assert"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/service"
	"telegram-bot-moex/internal/telegram"
)

// mockProvider implements provider.BondProvider for controllable testing in BDD scenarios.
type mockBondProvider struct {
	bonds        map[string]domain.Bond
	payments     map[string][]domain.Payment
	datePayments map[string][]domain.Payment
	marketData   map[string]*domain.MarketData
}

func newMockBondProvider() *mockBondProvider {
	return &mockBondProvider{
		bonds:        make(map[string]domain.Bond),
		payments:     make(map[string][]domain.Payment),
		datePayments: make(map[string][]domain.Payment),
		marketData:   make(map[string]*domain.MarketData),
	}
}

func (m *mockBondProvider) GetMarketData(ctx context.Context, identifier string) (*domain.MarketData, error) {
	if md, ok := m.marketData[identifier]; ok {
		return md, nil
	}
	return nil, nil
}

func (m *mockBondProvider) GetBond(ctx context.Context, identifier string) (*domain.Bond, error) {
	if b, ok := m.bonds[identifier]; ok {
		return &b, nil
	}
	// Try match by name
	for _, b := range m.bonds {
		if strings.Contains(strings.ToLower(b.Name), strings.ToLower(identifier)) ||
			strings.EqualFold(b.Ticker, identifier) {
			return &b, nil
		}
	}
	return nil, fmt.Errorf("mock bond not found: %s", identifier)
}

func (m *mockBondProvider) GetUpcomingPayments(ctx context.Context, isin string) ([]domain.Payment, error) {
	return m.payments[isin], nil
}

func (m *mockBondProvider) GetPaymentsForDate(ctx context.Context, date time.Time, isins []string) ([]domain.Payment, error) {
	dateStr := date.Format("2006-01-02")
	allForDate := m.datePayments[dateStr]
	if len(isins) == 0 {
		return allForDate, nil
	}

	isinSet := make(map[string]bool)
	for _, isin := range isins {
		isinSet[isin] = true
	}

	var filtered []domain.Payment
	for _, p := range allForDate {
		if isinSet[p.ISIN] {
			filtered = append(filtered, p)
		}
	}
	return filtered, nil
}

func (m *mockBondProvider) SearchBonds(ctx context.Context, query string) ([]domain.BondSummary, error) {
	var results []domain.BondSummary
	q := strings.ToLower(query)
	for _, b := range m.bonds {
		if strings.Contains(strings.ToLower(b.Name), q) ||
			strings.Contains(strings.ToLower(b.ISIN), q) ||
			strings.Contains(strings.ToLower(b.Ticker), q) {
			results = append(results, domain.BondSummary{
				ISIN:     b.ISIN,
				Ticker:   b.Ticker,
				Name:     b.Name,
				Exchange: b.Exchange,
			})
		}
	}
	return results, nil
}

func (m *mockBondProvider) GetNewAnnouncements(ctx context.Context, since time.Time) ([]domain.Announcement, error) {
	return nil, nil
}

func registerOnDemandSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	mockProv := tc.mockBondProv

	ctx.Step(`^the bot is running with market data for MOEX and SPB Exchange$`, func() error {
		// Populate standard mock bonds
		mockProv.bonds["RU000A107456"] = domain.Bond{
			ISIN:          "RU000A107456",
			Ticker:        "EUTR-03",
			Name:          "ЕвроТранс БО-001Р-03",
			Exchange:      domain.ExchangeMOEX,
			NominalValue:  1000,
			CouponRatePct: 13.5,
		}
		mockProv.payments["RU000A107456"] = []domain.Payment{
			{
				ISIN:   "RU000A107456",
				Type:   domain.PaymentCoupon,
				Date:   time.Now().AddDate(0, 1, 0),
				Amount: 33.29,
			},
		}

		mockProv.bonds["SU26238RMFS4"] = domain.Bond{
			ISIN:          "SU26238RMFS4",
			Ticker:        "SU26238",
			Name:          "ОФЗ 26238",
			Exchange:      domain.ExchangeMOEX,
			NominalValue:  1000,
			CouponRatePct: 7.1,
		}

		// Rebuild service with the mock provider for on-demand steps
		tc.bondService = service.NewBondService(mockProv, tc.memStore, tc.marketCache)
		return nil
	})

	ctx.Step(`^the user requests bond details for "([^"]*)"$`, func(identifier string) error {
		bond, marketData, payments, err := tc.bondService.GetBondDetails(tc.ctx, identifier)
		tc.currentBond = bond
		tc.currentMarketData = marketData
		tc.currentPayments = payments
		tc.lastError = err
		if bond != nil {
			tc.formattedMessage = telegram.FormatBondPassport(bond, marketData, payments)
		}
		return nil
	})

	ctx.Step(`^the bot should return a bond passport with:$`, func(table *godog.Table) error {
		if tc.currentBond == nil {
			return fmt.Errorf("expected bond to be found, got nil (err: %v)", tc.lastError)
		}

		for _, row := range table.Rows[1:] { // skip header
			field := row.Cells[0].Value
			val := row.Cells[1].Value

			switch field {
			case "Name":
				if tc.currentBond.Name != val {
					return fmt.Errorf("expected Name %s, got %s", val, tc.currentBond.Name)
				}
			case "Exchange":
				if string(tc.currentBond.Exchange) != val {
					return fmt.Errorf("expected Exchange %s, got %s", val, tc.currentBond.Exchange)
				}
			case "Nominal":
				expectedNominal, _ := strconv.ParseFloat(val, 64)
				if tc.currentBond.NominalValue != expectedNominal {
					return fmt.Errorf("expected Nominal %f, got %f", expectedNominal, tc.currentBond.NominalValue)
				}
			case "CouponRate":
				expectedRate, _ := strconv.ParseFloat(val, 64)
				if tc.currentBond.CouponRatePct != expectedRate {
					return fmt.Errorf("expected CouponRate %f, got %f", expectedRate, tc.currentBond.CouponRatePct)
				}
			}
		}
		return nil
	})

	ctx.Step(`^the response should list upcoming coupon payments$`, func() error {
		if len(tc.currentPayments) == 0 {
			return fmt.Errorf("expected payments list to not be empty")
		}
		return nil
	})

	ctx.Step(`^the user searches for bond "([^"]*)"$`, func(query string) error {
		bond, err := tc.bondService.SearchBond(tc.ctx, query)
		if err != nil {
			return err
		}
		tc.currentBond = bond
		tc.resolvedISIN = bond.ISIN
		tc.resolvedExchange = bond.Exchange
		return nil
	})

	ctx.Step(`^the bot resolves the search to ISIN "([^"]*)"$`, func(expectedISIN string) error {
		if tc.resolvedISIN != expectedISIN {
			return fmt.Errorf("expected ISIN %s, got %s", expectedISIN, tc.resolvedISIN)
		}
		return nil
	})

	ctx.Step(`^the bond exchange is identified as "([^"]*)"$`, func(expectedExchange string) error {
		if string(tc.resolvedExchange) != expectedExchange {
			return fmt.Errorf("expected exchange %s, got %s", expectedExchange, tc.resolvedExchange)
		}
		return nil
	})

	ctx.Step(`^the following payments are scheduled:$`, func(table *godog.Table) error {
		for _, row := range table.Rows[1:] {
			isin := row.Cells[0].Value
			bondName := row.Cells[1].Value
			dateStr := row.Cells[2].Value
			amount, _ := strconv.ParseFloat(row.Cells[3].Value, 64)
			pType := domain.PaymentType(row.Cells[4].Value)

			t, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				return err
			}

			p := domain.Payment{
				ISIN:     isin,
				BondName: bondName,
				Date:     t,
				Amount:   amount,
				Type:     pType,
			}

			mockProv.datePayments[dateStr] = append(mockProv.datePayments[dateStr], p)
			mockProv.payments[isin] = append(mockProv.payments[isin], p)
		}
		return nil
	})

	ctx.Step(`^the user queries payments for date "([^"]*)"$`, func(dateStr string) error {
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return err
		}
		// Query across all known scheduled payments
		payments, err := tc.bondService.GetPaymentsForDate(tc.ctx, t, nil)
		if err != nil {
			return err
		}
		tc.returnedPayments = payments
		return nil
	})

	ctx.Step(`^the bot response should contain (\d+) payments:$`, func(count int, table *godog.Table) error {
		if len(tc.returnedPayments) != count {
			return fmt.Errorf("expected %d payments, got %d", count, len(tc.returnedPayments))
		}
		for _, row := range table.Rows[1:] {
			expectedName := row.Cells[0].Value
			expectedAmount, _ := strconv.ParseFloat(row.Cells[1].Value, 64)

			found := false
			for _, p := range tc.returnedPayments {
				if p.BondName == expectedName && p.Amount == expectedAmount {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("payment %s (%.2f) not found in response", expectedName, expectedAmount)
			}
		}
		return nil
	})

	ctx.Step(`^the response should not contain "([^"]*)"$`, func(unexpectedName string) error {
		for _, p := range tc.returnedPayments {
			if p.BondName == unexpectedName {
				return fmt.Errorf("unexpectedly found payment for %s", unexpectedName)
			}
		}
		return nil
	})

	ctx.Step(`^the user has created a group named "([^"]*)" with bonds:$`, func(groupName string, table *godog.Table) error {
		testUser := domain.NewUser(555, 555)
		for _, row := range table.Rows[1:] {
			isin := row.Cells[0].Value
			testUser.AddBondToGroup(groupName, isin)
		}
		return tc.memStore.SaveUser(tc.ctx, testUser)
	})

	ctx.Step(`^the user requests upcoming payments for group "([^"]*)"$`, func(groupName string) error {
		payments, err := tc.bondService.GetPaymentsForUserGroup(tc.ctx, 555, groupName)
		if err != nil {
			return err
		}
		tc.returnedPayments = payments
		return nil
	})

	ctx.Step(`^the bot returns payments scheduled only for the bonds in "([^"]*)"$`, func(groupName string) error {
		user, _ := tc.memStore.GetUser(tc.ctx, 555)
		group := user.Groups[groupName]
		validISINs := make(map[string]bool)
		for _, isin := range group.ISINs {
			validISINs[isin] = true
		}

		assert.NotEmpty(nil, tc.returnedPayments)
		for _, p := range tc.returnedPayments {
			if !validISINs[p.ISIN] {
				return fmt.Errorf("payment with ISIN %s not in group %s", p.ISIN, groupName)
			}
		}
		return nil
	})
}
