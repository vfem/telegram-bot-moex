package bdd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cucumber/godog"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/provider/composite"
	"telegram-bot-moex/internal/provider/moex"
)

// mockRoundTripper intercepts HTTP requests to return mock MOEX ISS responses.
type mockRoundTripper struct {
	responses map[string]string
}

func (m *mockRoundTripper) Do(req *http.Request) (*http.Response, error) {
	urlStr := req.URL.String()
	for key, body := range m.responses {
		if strings.Contains(urlStr, key) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}
	}
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader(`{}`)),
		Header:     make(http.Header),
	}, nil
}

func registerProviderSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	mockTransport := &mockRoundTripper{responses: make(map[string]string)}
	var rawCSVData bytes.Buffer

	ctx.Step(`^a mock MOEX ISS response for security "([^"]*)" with coupon schedule$`, func(isin string) error {
		// Mock security description
		mockTransport.responses[isin+".json"] = `{
			"description": {
				"columns": ["name", "title", "value", "type"],
				"data": [
					["ISIN", "ISIN", "` + isin + `", "string"],
					["SECID", "Код", "SU26238", "string"],
					["NAME", "Полное наименование", "ОФЗ 26238", "string"],
					["EMITTER", "Эмитент", "Минфин РФ", "string"],
					["FACEVALUE", "Номинал", 1000, "number"],
					["COUPONPERCENT", "Ставка купона", 7.1, "number"]
				]
			}
		}`

		// Mock bondization
		mockTransport.responses["bondization/"+isin+".json"] = `{
			"coupons": {
				"columns": ["coupondate", "value", "valueprcnt"],
				"data": [
					["2026-10-15", 35.40, 7.1],
					["2027-04-15", 35.40, 7.1]
				]
			}
		}`

		tc.moexClient = moex.NewClientWithBaseURL("https://iss.moex.com/iss", mockTransport)
		return nil
	})

	ctx.Step(`^the MOEX provider fetches bond "([^"]*)"$`, func(isin string) error {
		bond, err := tc.moexClient.GetBond(tc.ctx, isin)
		if err != nil {
			return err
		}
		tc.currentBond = bond
		payments, err := tc.moexClient.GetUpcomingPayments(tc.ctx, isin)
		if err != nil {
			return err
		}
		tc.currentPayments = payments
		return nil
	})

	ctx.Step(`^the returned bond exchange should be "([^"]*)"$`, func(exchange string) error {
		if string(tc.currentBond.Exchange) != exchange {
			return fmt.Errorf("expected exchange %s, got %s", exchange, tc.currentBond.Exchange)
		}
		return nil
	})

	ctx.Step(`^the bond name should be "([^"]*)"$`, func(name string) error {
		if tc.currentBond.Name != name {
			return fmt.Errorf("expected name %s, got %s", name, tc.currentBond.Name)
		}
		return nil
	})

	ctx.Step(`^the bond should have (\d+) scheduled payments$`, func(count int) error {
		if len(tc.currentPayments) != count {
			return fmt.Errorf("expected %d payments, got %d", count, len(tc.currentPayments))
		}
		return nil
	})

	ctx.Step(`^an SPB Exchange securities CSV containing:$`, func(table *godog.Table) error {
		rawCSVData.Reset()
		rawCSVData.WriteString("ISIN;Ticker;Name;Currency\n")
		for _, row := range table.Rows[1:] {
			rawCSVData.WriteString(fmt.Sprintf("%s;%s;%s;%s\n",
				row.Cells[0].Value, row.Cells[1].Value, row.Cells[2].Value, row.Cells[3].Value))
		}
		return nil
	})

	ctx.Step(`^the SPB Exchange provider loads the securities registry$`, func() error {
		return tc.spbeClient.LoadFromCSV(&rawCSVData)
	})

	ctx.Step(`^searching for "([^"]*)" yields exchange "([^"]*)"$`, func(query, expectedExchange string) error {
		results, err := tc.spbeClient.SearchBonds(tc.ctx, query)
		if err != nil {
			return err
		}
		if len(results) == 0 {
			return fmt.Errorf("no results for query %s", query)
		}
		if string(results[0].Exchange) != expectedExchange {
			return fmt.Errorf("expected exchange %s, got %s", expectedExchange, results[0].Exchange)
		}
		return nil
	})

	ctx.Step(`^the MOEX provider has bond "([^"]*)"$`, func(isin string) error {
		mockProv := newMockBondProvider()
		mockProv.bonds[isin] = domain.Bond{
			ISIN:     isin,
			Ticker:   "SU26238",
			Name:     "ОФЗ 26238",
			Exchange: domain.ExchangeMOEX,
		}
		tc.composite = composite.NewProvider(mockProv, tc.spbeClient)
		return nil
	})

	ctx.Step(`^the SPB Exchange provider has bond "([^"]*)"$`, func(isin string) error {
		tc.spbeClient.RegisterBond(domain.Bond{
			ISIN:     isin,
			Ticker:   "CARM-01",
			Name:     "КарМани выпуск 1",
			Exchange: domain.ExchangeSPBE,
		})
		return nil
	})

	ctx.Step(`^the composite provider searches for "([^"]*)"$`, func(query string) error {
		results, err := tc.composite.SearchBonds(context.Background(), query)
		if err != nil {
			return err
		}
		if len(results) == 0 {
			return fmt.Errorf("no results found for %s", query)
		}
		tc.resolvedExchange = results[0].Exchange
		return nil
	})

	ctx.Step(`^the resolved bond exchange is "([^"]*)"$`, func(expectedExchange string) error {
		if string(tc.resolvedExchange) != expectedExchange {
			return fmt.Errorf("expected exchange %s, got %s", expectedExchange, tc.resolvedExchange)
		}
		return nil
	})
}
