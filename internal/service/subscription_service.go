package service

import (
	"context"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/store"
)

// SubscriptionService manages user subscriptions and monitored bonds.
type SubscriptionService struct {
	store store.Store
}

// NewSubscriptionService creates a SubscriptionService.
func NewSubscriptionService(s store.Store) *SubscriptionService {
	return &SubscriptionService{store: s}
}

// EnsureUser retrieves existing user or initializes a new one.
func (s *SubscriptionService) EnsureUser(ctx context.Context, telegramID, chatID int64) (*domain.User, error) {
	user, err := s.store.GetUser(ctx, telegramID)
	if err == nil {
		return user, nil
	}

	newUser := domain.NewUser(telegramID, chatID)
	if err := s.store.SaveUser(ctx, newUser); err != nil {
		return nil, err
	}
	return newUser, nil
}

// Subscribe updates subscription flags for a user.
func (s *SubscriptionService) Subscribe(ctx context.Context, telegramID, chatID int64, subType string) (*domain.User, error) {
	user, err := s.EnsureUser(ctx, telegramID, chatID)
	if err != nil {
		return nil, err
	}

	switch subType {
	case "daily_payments":
		user.NotifyDailyPayments = true
	case "new_announcements":
		user.NotifyAnnouncements = true
	}

	if err := s.store.SaveUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// Unsubscribe disables subscription flags for a user.
func (s *SubscriptionService) Unsubscribe(ctx context.Context, telegramID int64, subType string) (*domain.User, error) {
	user, err := s.store.GetUser(ctx, telegramID)
	if err != nil {
		return nil, err
	}

	switch subType {
	case "daily_payments":
		user.NotifyDailyPayments = false
	case "new_announcements":
		user.NotifyAnnouncements = false
	case "all":
		user.NotifyDailyPayments = false
		user.NotifyAnnouncements = false
	}

	if err := s.store.SaveUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// AddMonitoredBond adds an ISIN to the user's monitored list.
func (s *SubscriptionService) AddMonitoredBond(ctx context.Context, telegramID, chatID int64, isin string) (*domain.User, error) {
	user, err := s.EnsureUser(ctx, telegramID, chatID)
	if err != nil {
		return nil, err
	}

	user.AddMonitoredISIN(isin)
	if err := s.store.SaveUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}
