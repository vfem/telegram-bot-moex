package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/provider"
	"telegram-bot-moex/internal/provider/edisclosure"
	"telegram-bot-moex/internal/store"
)

const (
	maxSentFactAlerts = 10000
	maxOutboxMessages = 1000
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

// NotifierService handles scheduled notification generation (daily digests, announcements, and proactive fact alerts).
type NotifierService struct {
	mu             sync.Mutex
	provider       provider.BondProvider
	factsProvider  provider.FactsProvider
	store          store.Store
	dispatcher     MessageDispatcher
	sentFactAlerts map[string]bool
	// Outbox stores queued messages (useful for testing and async worker queue)
	Outbox []OutgoingMessage
}

// NewNotifierService creates a new NotifierService.
func NewNotifierService(p provider.BondProvider, s store.Store, d MessageDispatcher) *NotifierService {
	return &NotifierService{
		provider:       p,
		store:          s,
		dispatcher:     d,
		sentFactAlerts: make(map[string]bool),
		Outbox:         make([]OutgoingMessage, 0),
	}
}

// SetDispatcher wires or updates the MessageDispatcher on an existing NotifierService instance.
func (s *NotifierService) SetDispatcher(d MessageDispatcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dispatcher = d
}

// SetFactsProvider wires a FactsProvider for polling material facts on monitored bonds.
func (s *NotifierService) SetFactsProvider(fp provider.FactsProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.factsProvider = fp
}

func (s *NotifierService) appendOutbox(msg OutgoingMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Outbox) >= maxOutboxMessages {
		s.Outbox = s.Outbox[1:]
	}
	s.Outbox = append(s.Outbox, msg)
}

func (s *NotifierService) getDispatcher() MessageDispatcher {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dispatcher
}

func (s *NotifierService) getFactsProvider() provider.FactsProvider {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.factsProvider
}

// RunDailyDigest checks for scheduled payments on targetDate for all subscribed users,
// and also polls recent material facts for monitored bonds if a FactsProvider is configured.
func (s *NotifierService) RunDailyDigest(ctx context.Context, targetDate time.Time) ([]OutgoingMessage, error) {
	users, err := s.store.GetUsersSubscribedToDailyPayments(ctx)
	if err != nil {
		return nil, err
	}

	var generated []OutgoingMessage
	disp := s.getDispatcher()

	for _, user := range users {
		if len(user.MonitoredISINs) == 0 {
			continue
		}

		payments, err := s.provider.GetPaymentsForDate(ctx, targetDate, user.MonitoredISINs)
		if err != nil || len(payments) == 0 {
			continue
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("📅 Выплаты по вашим облигациям на %s:\n\n", escapeMdV2(targetDate.Format("02.01.2006"))))

		for _, p := range payments {
			sb.WriteString(fmt.Sprintf("• %s \\(%s\\): %s ₽ \\(%s\\)\n",
				escapeMdV2(p.BondName),
				escapeMdV2(p.ISIN),
				escapeMdV2(fmt.Sprintf("%.2f", p.Amount)),
				escapeMdV2(string(p.Type)),
			))
		}

		msg := OutgoingMessage{
			TelegramID: user.TelegramID,
			ChatID:     user.ChatID,
			Text:       sb.String(),
		}

		s.appendOutbox(msg)
		generated = append(generated, msg)

		if disp != nil {
			_ = disp.SendMessage(ctx, user.ChatID, msg.Text)
		}
	}

	// Poll material facts for all monitored bonds when FactsProvider is configured (F2, R2)
	if s.getFactsProvider() != nil {
		monitoredUsers, err := s.store.GetUsersWithMonitoredBonds(ctx)
		if err == nil {
			factMsgs, _ := s.CheckMonitoredFacts(ctx, monitoredUsers, targetDate)
			generated = append(generated, factMsgs...)
		}
	}

	return generated, nil
}

// CheckMonitoredFacts inspects monitored bonds across the provided users, fetches recent disclosures,
// and dispatches proactive alerts for CRITICAL and WARNING events published within the last 7 days.
func (s *NotifierService) CheckMonitoredFacts(ctx context.Context, users []*domain.User, refTime time.Time) ([]OutgoingMessage, error) {
	fp := s.getFactsProvider()
	if fp == nil || s.provider == nil {
		return nil, nil
	}

	if users == nil && s.store != nil {
		var err error
		users, err = s.store.GetUsersWithMonitoredBonds(ctx)
		if err != nil {
			return nil, err
		}
	}

	seenISINs := make(map[string]bool)
	var uniqueISINs []string
	for _, u := range users {
		for _, isin := range u.MonitoredISINs {
			clean := strings.TrimSpace(isin)
			if clean != "" && !seenISINs[clean] {
				seenISINs[clean] = true
				uniqueISINs = append(uniqueISINs, clean)
			}
		}
	}

	var allAlerts []OutgoingMessage
	cutoff := refTime.AddDate(0, 0, -7)
	factsByINN := make(map[string][]domain.MaterialFact)
	failedINN := make(map[string]bool)

	for _, isin := range uniqueISINs {
		bond, err := s.provider.GetBond(ctx, isin)
		if err != nil || bond == nil || bond.IssuerINN == "" || isFederalSovereignBond(bond) {
			continue
		}
		inn := strings.TrimSpace(bond.IssuerINN)
		if failedINN[inn] {
			continue
		}
		facts, cached := factsByINN[inn]
		if !cached {
			var fErr error
			facts, fErr = fp.GetRecentFacts(ctx, inn, 5)
			if fErr != nil {
				failedINN[inn] = true
				continue
			}
			factsByINN[inn] = facts
		}
		for _, f := range facts {
			if !f.PublishedAt.IsZero() && f.PublishedAt.Before(cutoff) {
				continue
			}
			// Prevent cross-series false alerts when a fact is specific to a different security (R2.2)
			if f.ISIN != "" && !strings.EqualFold(strings.TrimSpace(f.ISIN), strings.TrimSpace(bond.ISIN)) {
				continue
			}
			msgs, err := s.NotifyMaterialFact(ctx, *bond, f)
			if err == nil {
				allAlerts = append(allAlerts, msgs...)
			}
		}
	}

	return allAlerts, nil
}

// BroadcastAnnouncement notifies all users subscribed to announcements about a new bond placement.
func (s *NotifierService) BroadcastAnnouncement(ctx context.Context, announcement domain.Announcement) ([]OutgoingMessage, error) {
	users, err := s.store.GetUsersSubscribedToAnnouncements(ctx)
	if err != nil {
		return nil, err
	}

	var generated []OutgoingMessage
	disp := s.getDispatcher()

	text := fmt.Sprintf("🔔 Новый анонс размещения облигаций\\!\n\n"+
		"Выпуск: %s\n"+
		"Эмитент: %s\n"+
		"Биржа: %s\n"+
		"Объем: %s ₽\n"+
		"Ориентир купона: %s\n",
		escapeMdV2(announcement.Title),
		escapeMdV2(announcement.Issuer),
		escapeMdV2(string(announcement.Exchange)),
		escapeMdV2(fmt.Sprintf("%.0f", announcement.VolumeRUB)),
		escapeMdV2(announcement.TargetCoupon),
	)

	for _, user := range users {
		msg := OutgoingMessage{
			TelegramID: user.TelegramID,
			ChatID:     user.ChatID,
			Text:       text,
		}

		s.appendOutbox(msg)
		generated = append(generated, msg)

		if disp != nil {
			_ = disp.SendMessage(ctx, user.ChatID, msg.Text)
		}
	}

	return generated, nil
}

// NotifyMaterialFact sends a proactive push alert to all users monitoring bond.ISIN
// when a CRITICAL or WARNING material fact is detected. Routine INFO facts are ignored.
func (s *NotifierService) NotifyMaterialFact(ctx context.Context, bond domain.Bond, fact domain.MaterialFact) ([]OutgoingMessage, error) {
	if fact.Category == "" || fact.Severity == "" {
		fact.Classify()
	}

	// Only push alerts for CRITICAL or WARNING events
	if !fact.IsAlertWorthy() {
		return nil, nil
	}

	bondISIN := strings.TrimSpace(bond.ISIN)
	factISIN := strings.TrimSpace(fact.ISIN)
	if bondISIN != "" && factISIN != "" && !strings.EqualFold(bondISIN, factISIN) {
		return nil, nil
	}

	isin := bondISIN
	if isin == "" {
		isin = factISIN
	}
	if isin == "" {
		return nil, nil
	}

	users, err := s.store.GetUsersMonitoringISIN(ctx, isin)
	if err != nil {
		return nil, err
	}

	bondTitle := bond.Name
	if bondTitle == "" {
		bondTitle = isin
	}

	dateStr := "н/д"
	if !fact.PublishedAt.IsZero() {
		dateStr = fact.PublishedAt.Format("02.01.2006")
	}

	safeURL := edisclosure.SanitizeURL(fact.URL)

	var sb strings.Builder
	sb.WriteString("🚨 *ВНИМАНИЕ: Срочное событие по вашей облигации\\!*\n")
	sb.WriteString(fmt.Sprintf("Облигация: *%s* \\(`%s`\\)\n", escapeMdV2(bondTitle), escapeCodeSpan(isin)))
	if bond.Issuer != "" {
		sb.WriteString(fmt.Sprintf("Эмитент: %s\n", escapeMdV2(bond.Issuer)))
	}
	sb.WriteString(fmt.Sprintf("\n%s Тип: *%s* \\[%s\\]\n", fact.Severity.Emoji(), escapeMdV2(string(fact.Category)), escapeMdV2(string(fact.Severity))))
	sb.WriteString(fmt.Sprintf("Дата события: `%s`\n", escapeCodeSpan(dateStr)))
	sb.WriteString(fmt.Sprintf("Суть: %s\n", escapeMdV2(fact.Title)))
	if safeURL != "" {
		sb.WriteString(fmt.Sprintf("Ссылка: %s\n", escapeMdV2(safeURL)))
	}

	text := sb.String()
	disp := s.getDispatcher()
	var generated []OutgoingMessage

	for _, user := range users {
		dedupKey := fmt.Sprintf("%d:%s:%s:%s", user.TelegramID, isin, fact.ID, fact.Title)

		s.mu.Lock()
		alreadySent := s.sentFactAlerts != nil && s.sentFactAlerts[dedupKey]
		s.mu.Unlock()
		if alreadySent {
			continue
		}

		msg := OutgoingMessage{
			TelegramID: user.TelegramID,
			ChatID:     user.ChatID,
			Text:       text,
		}

		if disp != nil {
			if sendErr := disp.SendMessage(ctx, user.ChatID, msg.Text); sendErr != nil {
				// Do not mark deduplication key on transient delivery error (F6.3)
				continue
			}
		}

		s.mu.Lock()
		if s.sentFactAlerts == nil {
			s.sentFactAlerts = make(map[string]bool)
		}
		if len(s.sentFactAlerts) >= maxSentFactAlerts {
			for k := range s.sentFactAlerts {
				delete(s.sentFactAlerts, k)
				break
			}
		}
		s.sentFactAlerts[dedupKey] = true
		s.mu.Unlock()

		s.appendOutbox(msg)
		generated = append(generated, msg)
	}

	return generated, nil
}

func escapeMdV2(s string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(s)
}

func escapeCodeSpan(s string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"`", "\\`",
	)
	return replacer.Replace(s)
}
