package spbe

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"telegram-bot-moex/internal/domain"
)

// Client handles unauthenticated public data from Saint Petersburg Exchange (SPBE).
type Client struct {
	mu         sync.RWMutex
	securities map[string]domain.Bond // keyed by ISIN
}

// NewClient creates an unauthenticated SPBE client.
func NewClient() *Client {
	return &Client{
		securities: make(map[string]domain.Bond),
	}
}

// LoadFromCSV reads official SPB Exchange listed securities CSV.
func (c *Client) LoadFromCSV(r io.Reader) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	reader := csv.NewReader(r)
	reader.Comma = ';'
	reader.LazyQuotes = true

	// Read header
	header, err := reader.Read()
	if err != nil {
		return err
	}

	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.ToLower(strings.TrimSpace(col))] = i
	}

	isinIdx, hasISIN := colIdx["isin"]
	tickerIdx, _ := colIdx["ticker"]
	nameIdx, _ := colIdx["name"]
	currIdx, _ := colIdx["currency"]

	if !hasISIN {
		// Fallback check alternative headers
		if idx, ok := colIdx["код isin"]; ok {
			isinIdx = idx
			hasISIN = true
		}
	}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		var isin, ticker, name, currency string
		if hasISIN && len(record) > isinIdx {
			isin = strings.TrimSpace(record[isinIdx])
		}
		if len(record) > tickerIdx {
			ticker = strings.TrimSpace(record[tickerIdx])
		}
		if len(record) > nameIdx {
			name = strings.TrimSpace(record[nameIdx])
		}
		if len(record) > currIdx {
			currency = strings.TrimSpace(record[currIdx])
		}

		if isin != "" {
			c.securities[isin] = domain.Bond{
				ISIN:         isin,
				Ticker:       ticker,
				Name:         name,
				Exchange:     domain.ExchangeSPBE,
				Currency:     currency,
				NominalValue: 1000,
			}
		}
	}

	return nil
}

// RegisterBond directly adds a bond to the in-memory SPBE registry (useful for testing & feeds).
func (c *Client) RegisterBond(bond domain.Bond) {
	c.mu.Lock()
	defer c.mu.Unlock()
	bond.Exchange = domain.ExchangeSPBE
	c.securities[bond.ISIN] = bond
}

func (c *Client) GetBond(ctx context.Context, identifier string) (*domain.Bond, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Direct lookup by ISIN
	if bond, exists := c.securities[identifier]; exists {
		return &bond, nil
	}

	// Lookup by Ticker
	for _, bond := range c.securities {
		if strings.EqualFold(bond.Ticker, identifier) {
			return &bond, nil
		}
	}

	return nil, fmt.Errorf("bond not found on SPB Exchange: %s", identifier)
}

func (c *Client) GetUpcomingPayments(ctx context.Context, isin string) ([]domain.Payment, error) {
	return nil, nil
}

func (c *Client) GetPaymentsForDate(ctx context.Context, date time.Time, isins []string) ([]domain.Payment, error) {
	return nil, nil
}

func (c *Client) SearchBonds(ctx context.Context, query string) ([]domain.BondSummary, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var results []domain.BondSummary
	q := strings.ToLower(query)

	for _, bond := range c.securities {
		if strings.Contains(strings.ToLower(bond.ISIN), q) ||
			strings.Contains(strings.ToLower(bond.Ticker), q) ||
			strings.Contains(strings.ToLower(bond.Name), q) {
			results = append(results, domain.BondSummary{
				ISIN:     bond.ISIN,
				Ticker:   bond.Ticker,
				Name:     bond.Name,
				Exchange: domain.ExchangeSPBE,
			})
		}
	}

	return results, nil
}

func (c *Client) GetMarketData(ctx context.Context, identifier string) (*domain.MarketData, error) {
	// SPBE public CSV listing does not provide live order book stream; return nil gracefully
	return nil, nil
}

func (c *Client) GetNewAnnouncements(ctx context.Context, since time.Time) ([]domain.Announcement, error) {
	return nil, nil
}

