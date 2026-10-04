package service

import (
	"context"
	"fmt"
	"time"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/provider"
	"telegram-bot-moex/internal/store"
)

// BondService coordinates bond queries, payments, and user group lookups.
type BondService struct {
	provider provider.BondProvider
	store    store.Store
}

// NewBondService instantiates a new BondService.
func NewBondService(p provider.BondProvider, s store.Store) *BondService {
	return &BondService{
		provider: p,
		store:    s,
	}
}

// GetBondWithPayments fetches bond info and upcoming payments.
func (s *BondService) GetBondWithPayments(ctx context.Context, identifier string) (*domain.Bond, []domain.Payment, error) {
	bond, err := s.provider.GetBond(ctx, identifier)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get bond %s: %w", identifier, err)
	}

	payments, err := s.provider.GetUpcomingPayments(ctx, bond.ISIN)
	if err != nil {
		// Non-fatal: still return the bond passport even if payment schedule fails
		payments = nil
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
