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

func cloneUser(u *domain.User) *domain.User {
	if u == nil {
		return nil
	}
	cp := *u
	if u.MonitoredISINs != nil {
		cp.MonitoredISINs = make([]string, len(u.MonitoredISINs))
		copy(cp.MonitoredISINs, u.MonitoredISINs)
	}
	if u.Groups != nil {
		cp.Groups = make(map[string]domain.BondGroup, len(u.Groups))
		for k, g := range u.Groups {
			gCopy := domain.BondGroup{
				Name: g.Name,
			}
			if g.ISINs != nil {
				gCopy.ISINs = make([]string, len(g.ISINs))
				copy(gCopy.ISINs, g.ISINs)
			}
			cp.Groups[k] = gCopy
		}
	}
	return &cp
}

func (s *Store) GetUser(ctx context.Context, telegramID int64) (*domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.users[telegramID]
	if !exists {
		return nil, fmt.Errorf("user not found: %d", telegramID)
	}
	return cloneUser(user), nil
}

func (s *Store) SaveUser(ctx context.Context, user *domain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.users[user.TelegramID] = cloneUser(user)
	return nil
}

func (s *Store) GetUsersSubscribedToDailyPayments(ctx context.Context) ([]*domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*domain.User
	for _, u := range s.users {
		if u.NotifyDailyPayments {
			result = append(result, cloneUser(u))
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
			result = append(result, cloneUser(u))
		}
	}
	return result, nil
}

func (s *Store) GetUsersWithMonitoredBonds(ctx context.Context) ([]*domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*domain.User
	for _, u := range s.users {
		if len(u.MonitoredISINs) > 0 {
			result = append(result, cloneUser(u))
		}
	}
	return result, nil
}

func (s *Store) GetUsersMonitoringISIN(ctx context.Context, isin string) ([]*domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*domain.User
	for _, u := range s.users {
		for _, monitored := range u.MonitoredISINs {
			if monitored == isin {
				result = append(result, cloneUser(u))
				break
			}
		}
	}
	return result, nil
}
