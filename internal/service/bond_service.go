package service

import (
	"context"
	"fmt"
	"time"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/provider"
	"telegram-bot-moex/internal/store"
)

// BondService coordinates bond queries, payments, market data, and user group lookups.
type BondService struct {
	provider    provider.BondProvider
	store       store.Store
	marketCache store.MarketDataCache
}

// NewBondService instantiates a new BondService.
func NewBondService(p provider.BondProvider, s store.Store, cache ...store.MarketDataCache) *BondService {
	var mc store.MarketDataCache
	if len(cache) > 0 && cache[0] != nil {
		mc = cache[0]
	}
	return &BondService{
		provider:    p,
		store:       s,
		marketCache: mc,
	}
}

// SetMarketCache allows overriding the market data cache (e.g. for testing).
func (s *BondService) SetMarketCache(cache store.MarketDataCache) {
	s.marketCache = cache
}

// GetBondDetails fetches bond passport, live market data (with caching), and upcoming payments.
func (s *BondService) GetBondDetails(ctx context.Context, identifier string) (*domain.Bond, *domain.MarketData, []domain.Payment, error) {
	bond, err := s.provider.GetBond(ctx, identifier)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get bond %s: %w", identifier, err)
	}

	// Determine lookup key: prefer ticker (e.g. SU26238RMFS4 on MOEX), fallback to ISIN
	lookupKey := bond.Ticker
	if lookupKey == "" {
		lookupKey = bond.ISIN
	}

	// Fetch or retrieve market data from cache
	var marketData *domain.MarketData
	if s.marketCache != nil {
		if cached, ok := s.marketCache.Get(bond.ISIN); ok {
			marketData = cached
		} else if lookupKey != bond.ISIN {
			if cached, ok := s.marketCache.Get(lookupKey); ok {
				marketData = cached
			}
		}
	}

	if marketData == nil {
		// Attempt primary lookupKey first (C-11: accept any non-nil result)
		if md, err := s.provider.GetMarketData(ctx, lookupKey); err == nil && md != nil {
			marketData = md
		} else if lookupKey != bond.ISIN {
			// Fallback to ISIN
			if md2, err2 := s.provider.GetMarketData(ctx, bond.ISIN); err2 == nil && md2 != nil {
				marketData = md2
			}
		}

		if marketData != nil && s.marketCache != nil {
			s.marketCache.Set(bond.ISIN, marketData)
			if bond.Ticker != "" {
				s.marketCache.Set(bond.Ticker, marketData)
			}
		}
	}

	// Upcoming payments: try lookupKey, then ISIN
	payments, err := s.provider.GetUpcomingPayments(ctx, lookupKey)
	if err != nil || len(payments) == 0 {
		if lookupKey != bond.ISIN {
			if p2, err2 := s.provider.GetUpcomingPayments(ctx, bond.ISIN); err2 == nil && len(p2) > 0 {
				payments = p2
			}
		}
	}

	return bond, marketData, payments, nil
}

// GetMarketData returns cached or live market data for an ISIN.
func (s *BondService) GetMarketData(ctx context.Context, isin string) (*domain.MarketData, error) {
	if s.marketCache != nil {
		if cached, ok := s.marketCache.Get(isin); ok {
			return cached, nil
		}
	}

	md, err := s.provider.GetMarketData(ctx, isin)
	if err != nil {
		return nil, err
	}

	if md != nil && s.marketCache != nil {
		s.marketCache.Set(isin, md)
	}
	return md, nil
}

// GetBondWithPayments fetches bond info and upcoming payments (for backwards compatibility).
func (s *BondService) GetBondWithPayments(ctx context.Context, identifier string) (*domain.Bond, []domain.Payment, error) {
	bond, _, payments, err := s.GetBondDetails(ctx, identifier)
	if err != nil {
		return nil, nil, err
	}
	return bond, payments, nil
}

// SearchBond resolves a name or partial ticker to a bond.
func (s *BondService) SearchBond(ctx context.Context, query string) (*domain.Bond, error) {
	results, err := s.provider.SearchBonds(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no bonds found for query: %s", query)
	}

	// Fetch primary match
	return s.provider.GetBond(ctx, results[0].ISIN)
}

// GetPaymentsForDate returns all payments on a specific date for a list of ISINs.
func (s *BondService) GetPaymentsForDate(ctx context.Context, date time.Time, isins []string) ([]domain.Payment, error) {
	return s.provider.GetPaymentsForDate(ctx, date, isins)
}

// GetPaymentsForUserGroup returns upcoming payments for all bonds belonging to a specific user group.
func (s *BondService) GetPaymentsForUserGroup(ctx context.Context, telegramID int64, groupName string) ([]domain.Payment, error) {
	user, err := s.store.GetUser(ctx, telegramID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	group, exists := user.Groups[groupName]
	if !exists {
		return nil, fmt.Errorf("group %q not found", groupName)
	}

	var allPayments []domain.Payment
	for _, isin := range group.ISINs {
		payments, err := s.provider.GetUpcomingPayments(ctx, isin)
		if err != nil {
			continue
		}
		allPayments = append(allPayments, payments...)
	}

	return allPayments, nil
}
