package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/provider"
	"telegram-bot-moex/internal/store"
)

// OutgoingMessage represents a message prepared for dispatch to a user.
type OutgoingMessage struct {
	TelegramID int64
	ChatID     int64
	Text       string
}

// MessageDispatcher defines the contract for sending messages to Telegram.
type MessageDispatcher interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
}

// NotifierService handles scheduled notification generation (daily digests, announcements).
type NotifierService struct {
	provider   provider.BondProvider
	store      store.Store
	dispatcher MessageDispatcher
	// Outbox stores queued messages (useful for testing and async worker queue)
	Outbox []OutgoingMessage
}

// NewNotifierService creates a new NotifierService.
func NewNotifierService(p provider.BondProvider, s store.Store, d MessageDispatcher) *NotifierService {
	return &NotifierService{
		provider:   p,
		store:      s,
		dispatcher: d,
		Outbox:     make([]OutgoingMessage, 0),
	}
}

// RunDailyDigest checks for scheduled payments on targetDate for all subscribed users.
func (s *NotifierService) RunDailyDigest(ctx context.Context, targetDate time.Time) ([]OutgoingMessage, error) {
	users, err := s.store.GetUsersSubscribedToDailyPayments(ctx)
	if err != nil {
		return nil, err
	}

	var generated []OutgoingMessage

	for _, user := range users {
		if len(user.MonitoredISINs) == 0 {
			continue
		}

		payments, err := s.provider.GetPaymentsForDate(ctx, targetDate, user.MonitoredISINs)
		if err != nil || len(payments) == 0 {
			continue
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("📅 Выплаты по вашим облигациям на %s:\n\n", targetDate.Format("02.01.2006")))

		for _, p := range payments {
			sb.WriteString(fmt.Sprintf("• %s (%s): %.2f ₽ (%s)\n", p.BondName, p.ISIN, p.Amount, p.Type))
		}

		msg := OutgoingMessage{
			TelegramID: user.TelegramID,
			ChatID:     user.ChatID,
			Text:       sb.String(),
		}

		s.Outbox = append(s.Outbox, msg)
		generated = append(generated, msg)

		if s.dispatcher != nil {
			_ = s.dispatcher.SendMessage(ctx, user.ChatID, msg.Text)
		}
	}

	return generated, nil
}

// BroadcastAnnouncement notifies all users subscribed to announcements about a new bond placement.
func (s *NotifierService) BroadcastAnnouncement(ctx context.Context, announcement domain.Announcement) ([]OutgoingMessage, error) {
	users, err := s.store.GetUsersSubscribedToAnnouncements(ctx)
	if err != nil {
		return nil, err
	}

	var generated []OutgoingMessage

	text := fmt.Sprintf("🔔 Новый анонс размещения облигаций!\n\n"+
		"Выпуск: %s\n"+
		"Эмитент: %s\n"+
		"Биржа: %s\n"+
		"Объем: %.0f ₽\n"+
		"Ориентир купона: %s\n",
		announcement.Title,
		announcement.Issuer,
		announcement.Exchange,
		announcement.VolumeRUB,
		announcement.TargetCoupon,
	)

	for _, user := range users {
		msg := OutgoingMessage{
			TelegramID: user.TelegramID,
			ChatID:     user.ChatID,
			Text:       text,
		}

		s.Outbox = append(s.Outbox, msg)
		generated = append(generated, msg)

		if s.dispatcher != nil {
			_ = s.dispatcher.SendMessage(ctx, user.ChatID, msg.Text)
		}
	}

	return generated, nil
}
