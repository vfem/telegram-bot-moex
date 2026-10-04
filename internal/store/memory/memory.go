package memory

import (
	"context"
	"fmt"
	"sync"

	"telegram-bot-moex/internal/domain"
)

// Store implements store.Store using an in-memory map protected by a mutex.
type Store struct {
	mu    sync.RWMutex
	users map[int64]*domain.User
}

// NewStore creates a new in-memory store.
func NewStore() *Store {
	return &Store{
		users: make(map[int64]*domain.User),
	}
}

func (s *Store) GetUser(ctx context.Context, telegramID int64) (*domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.users[telegramID]
	if !exists {
		return nil, fmt.Errorf("user not found: %d", telegramID)
	}
	return user, nil
}

func (s *Store) SaveUser(ctx context.Context, user *domain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.users[user.TelegramID] = user
	return nil
}

func (s *Store) GetUsersSubscribedToDailyPayments(ctx context.Context) ([]*domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*domain.User
	for _, u := range s.users {
		if u.NotifyDailyPayments {
			result = append(result, u)
		}
	}
	return result, nil
}

func (s *Store) GetUsersSubscribedToAnnouncements(ctx context.Context) ([]*domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*domain.User
	for _, u := range s.users {
		if u.NotifyAnnouncements {
			result = append(result, u)
		}
	}
	return result, nil
}
