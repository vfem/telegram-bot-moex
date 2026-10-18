package store

import (
	"context"
	"time"

	"telegram-bot-moex/internal/domain"
)

// Store defines persistence operations for user settings, subscriptions, and custom bond groups.
type Store interface {
	GetUser(ctx context.Context, telegramID int64) (*domain.User, error)
	SaveUser(ctx context.Context, user *domain.User) error
	GetUsersSubscribedToDailyPayments(ctx context.Context) ([]*domain.User, error)
	GetUsersSubscribedToAnnouncements(ctx context.Context) ([]*domain.User, error)
	GetUsersWithMonitoredBonds(ctx context.Context) ([]*domain.User, error)
	GetUsersMonitoringISIN(ctx context.Context, isin string) ([]*domain.User, error)
}

// MarketDataCache defines operations for caching real-time bond quotes and liquidity data.
type MarketDataCache interface {
	Get(isin string) (*domain.MarketData, bool)
	Set(isin string, data *domain.MarketData)
}

// FinancialsCache defines operations for caching annual financial statements of issuers.
type FinancialsCache interface {
	Get(inn string) (*domain.IssuerBalance, bool)
	Set(inn string, balance *domain.IssuerBalance)
	SetNotFound(inn string, ttl time.Duration)
	IsNotFound(inn string) bool
}

// FactsCache defines operations for caching corporate disclosures and material facts.
type FactsCache interface {
	Get(inn string) ([]domain.MaterialFact, bool)
	Set(inn string, facts []domain.MaterialFact)
	SetNotFound(inn string, ttl time.Duration)
	IsNotFound(inn string) bool
}
