package provider

import (
	"context"
	"time"

	"telegram-bot-moex/internal/domain"
)

// BondProvider specifies the contract for querying bond data from an exchange or aggregator.
type BondProvider interface {
	// GetBond returns full passport information for a bond by ISIN or SECID.
	GetBond(ctx context.Context, identifier string) (*domain.Bond, error)

	// GetUpcomingPayments returns future cash flows (coupons, amortizations, maturities) for a bond.
	GetUpcomingPayments(ctx context.Context, isin string) ([]domain.Payment, error)

	// GetPaymentsForDate returns all payments across given ISINs on a specific date.
	GetPaymentsForDate(ctx context.Context, date time.Time, isins []string) ([]domain.Payment, error)

	// SearchBonds searches for bonds by ticker, name, or ISIN substring.
	SearchBonds(ctx context.Context, query string) ([]domain.BondSummary, error)

	// GetMarketData returns current trading quotes and liquidity for a bond.
	GetMarketData(ctx context.Context, identifier string) (*domain.MarketData, error)

	// GetNewAnnouncements returns recent new bond placement announcements.
	GetNewAnnouncements(ctx context.Context, since time.Time) ([]domain.Announcement, error)
}

// FinancialsProvider specifies the contract for querying issuer financial statements (balance sheet & results).
type FinancialsProvider interface {
	GetIssuerBalance(ctx context.Context, inn string) (*domain.IssuerBalance, error)
}

// FactsProvider specifies the contract for retrieving material facts and corporate disclosures by INN.
type FactsProvider interface {
	GetRecentFacts(ctx context.Context, inn string, limit int) ([]domain.MaterialFact, error)
}
