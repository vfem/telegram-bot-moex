package composite

import (
	"context"
	"fmt"
	"time"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/provider"
)

// Provider combines MOEX and SPB Exchange data providers into a unified interface.
type Provider struct {
	moex provider.BondProvider
	spbe provider.BondProvider
}

// NewProvider creates a new composite provider.
func NewProvider(moex, spbe provider.BondProvider) *Provider {
	return &Provider{
		moex: moex,
		spbe: spbe,
	}
}

// GetBond looks up a bond across MOEX and SPBE.
func (p *Provider) GetBond(ctx context.Context, identifier string) (*domain.Bond, error) {
	if p.moex != nil {
		bond, err := p.moex.GetBond(ctx, identifier)
		if err == nil && bond != nil && bond.Name != identifier {
			return bond, nil
		}
	}

	if p.spbe != nil {
		bond, err := p.spbe.GetBond(ctx, identifier)
		if err == nil && bond != nil {
			return bond, nil
		}
	}

	// If neither found a complete profile, fallback to whichever returned an error
	if p.moex != nil {
		return p.moex.GetBond(ctx, identifier)
	}

	return nil, fmt.Errorf("bond %s not found on any exchange", identifier)
}

// GetUpcomingPayments resolves future payments for an ISIN.
func (p *Provider) GetUpcomingPayments(ctx context.Context, isin string) ([]domain.Payment, error) {
	if p.moex != nil {
		payments, err := p.moex.GetUpcomingPayments(ctx, isin)
		if err == nil && len(payments) > 0 {
			return payments, nil
		}
	}

	if p.spbe != nil {
		return p.spbe.GetUpcomingPayments(ctx, isin)
	}

	return nil, nil
}

// GetPaymentsForDate queries payments across all specified ISINs for a target date.
func (p *Provider) GetPaymentsForDate(ctx context.Context, date time.Time, isins []string) ([]domain.Payment, error) {
	var results []domain.Payment

	if p.moex != nil {
		payments, err := p.moex.GetPaymentsForDate(ctx, date, isins)
		if err == nil {
			results = append(results, payments...)
		}
	}

	if p.spbe != nil {
		payments, err := p.spbe.GetPaymentsForDate(ctx, date, isins)
		if err == nil {
			results = append(results, payments...)
		}
	}

	return results, nil
}

// SearchBonds queries both MOEX and SPB Exchange, deduplicating by ISIN.
func (p *Provider) SearchBonds(ctx context.Context, query string) ([]domain.BondSummary, error) {
	seen := make(map[string]bool)
	var merged []domain.BondSummary

	if p.moex != nil {
		results, _ := p.moex.SearchBonds(ctx, query)
		for _, r := range results {
			if !seen[r.ISIN] {
				seen[r.ISIN] = true
				merged = append(merged, r)
			}
		}
	}

	if p.spbe != nil {
		results, _ := p.spbe.SearchBonds(ctx, query)
		for _, r := range results {
			if !seen[r.ISIN] {
				seen[r.ISIN] = true
				merged = append(merged, r)
			}
		}
	}

	return merged, nil
}

// GetMarketData delegates market data lookup to MOEX, then SPBE if needed.
func (p *Provider) GetMarketData(ctx context.Context, identifier string) (*domain.MarketData, error) {
	if p.moex != nil {
		md, err := p.moex.GetMarketData(ctx, identifier)
		if err == nil && md != nil {
			return md, nil
		}
	}

	if p.spbe != nil {
		return p.spbe.GetMarketData(ctx, identifier)
	}

	return nil, nil
}

// GetNewAnnouncements aggregates new placement announcements from both exchanges.
func (p *Provider) GetNewAnnouncements(ctx context.Context, since time.Time) ([]domain.Announcement, error) {
	var merged []domain.Announcement

	if p.moex != nil {
		if items, err := p.moex.GetNewAnnouncements(ctx, since); err == nil {
			merged = append(merged, items...)
		}
	}

	if p.spbe != nil {
		if items, err := p.spbe.GetNewAnnouncements(ctx, since); err == nil {
			merged = append(merged, items...)
		}
	}

	return merged, nil
}
