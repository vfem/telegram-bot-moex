package store

import (
	"context"

	"telegram-bot-moex/internal/domain"
)

// Store defines persistence operations for user settings, subscriptions, and custom bond groups.
type Store interface {
	GetUser(ctx context.Context, telegramID int64) (*domain.User, error)
	SaveUser(ctx context.Context, user *domain.User) error
	GetUsersSubscribedToDailyPayments(ctx context.Context) ([]*domain.User, error)
	GetUsersSubscribedToAnnouncements(ctx context.Context) ([]*domain.User, error)
}
