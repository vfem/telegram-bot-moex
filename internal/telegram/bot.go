package telegram

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"telegram-bot-moex/internal/service"
)

// Update represents an incoming Telegram webhook update.
type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

// Message represents a Telegram message.
type Message struct {
	MessageID int64  `json:"message_id"`
	From      *User  `json:"from"`
	Chat      *Chat  `json:"chat"`
	Text      string `json:"text"`
}

// User represents a Telegram user.
type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

// Chat represents a Telegram chat.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// Bot handles incoming webhooks and dispatches Telegram messages.
type Bot struct {
	token         string
	secretToken   string
	bondService   *service.BondService
	finService    *service.FinancialsService
	subService    *service.SubscriptionService
	notifier      *service.NotifierService
	httpClient    *http.Client
	messageSender func(ctx context.Context, chatID int64, text string) error
}

// NewBot instantiates a Bot with all required services.
func NewBot(token, secretToken string, bondSvc *service.BondService, subSvc *service.SubscriptionService, notifier *service.NotifierService, finSvc *service.FinancialsService) *Bot {
	return &Bot{
		token:       token,
		secretToken: secretToken,
		bondService: bondSvc,
		finService:  finSvc,
		subService:  subSvc,
		notifier:    notifier,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// SetMessageSender allows hooking into SendMessage for tests.
func (b *Bot) SetMessageSender(fn func(ctx context.Context, chatID int64, text string) error) {
	b.messageSender = fn
}

// HandleIncomingMessage dispatches an incoming user message directly (e.g. in test suites).
func (b *Bot) HandleIncomingMessage(ctx context.Context, chatID, userID int64, text string) {
	msg := &Message{
		Chat: &Chat{ID: chatID},
		From: &User{ID: userID},
		Text: text,
	}
	b.handleMessage(ctx, msg)
}

// SendMessage sends an outgoing message to a Telegram chat.
func (b *Bot) SendMessage(ctx context.Context, chatID int64, text string) error {
	if b.messageSender != nil {
		return b.messageSender(ctx, chatID, text)
	}

	if b.token == "" {
		log.Printf("[MOCK SEND] Chat %d: %s\n", chatID, text)
		return nil
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", b.token)
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "MarkdownV2",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("Telegram SendMessage error (status %d): %s; falling back to plain text", resp.StatusCode, string(respBody))
		// Fallback to plain text if MarkdownV2 formatting error occurs
		payload["parse_mode"] = ""
		body, _ = json.Marshal(payload)
		fallbackReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
		fallbackReq.Header.Set("Content-Type", "application/json")
		fallbackResp, err := b.httpClient.Do(fallbackReq)
		if err == nil {
			defer fallbackResp.Body.Close()
		}
	}

	return nil
}

// WebhookHandler processes incoming POST requests from Telegram.
func (b *Bot) WebhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Fail closed if bot token is configured (production mode) but secret token is unset
	if b.secretToken == "" && b.token != "" {
		http.Error(w, "Webhook secret not configured", http.StatusServiceUnavailable)
		return
	}

	// Validate secret token if configured (constant-time)
	if b.secretToken != "" {
		tokenHeader := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		if subtle.ConstantTimeCompare([]byte(tokenHeader), []byte(b.secretToken)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	var update Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if update.Message != nil && update.Message.Text != "" {
		// Process synchronously so Cloud Run CPU throttling does not freeze response dispatch
		msgCtx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		b.handleMessage(msgCtx, update.Message)
	}

	w.WriteHeader(http.StatusOK)
}

func (b *Bot) handleMessage(ctx context.Context, msg *Message) {
	text := strings.TrimSpace(msg.Text)
	chatID := msg.Chat.ID
	userID := msg.From.ID

	if strings.HasPrefix(text, "/start") || strings.HasPrefix(text, "/help") {
		b.sendHelp(ctx, chatID)
		return
	}

	if strings.HasPrefix(text, "/bond") || strings.HasPrefix(text, "/b ") {
		parts := strings.Fields(text)
		if len(parts) < 2 {
			_ = b.SendMessage(ctx, chatID, "Использование: `/bond <ISIN / Тикер / Название>`\nПример: `/bond SU26238RMFS4`")
			return
		}
		query := strings.Join(parts[1:], " ")
		bond, marketData, payments, err := b.bondService.GetBondDetails(ctx, query)
		if err != nil {
			// Try fuzzy search
			bond, err = b.bondService.SearchBond(ctx, query)
			if err != nil {
				_ = b.SendMessage(ctx, chatID, fmt.Sprintf("Облигация по запросу *%s* не найдена\\.", escapeMarkdown(query)))
				return
			}
			marketData, _ = b.bondService.GetMarketData(ctx, bond.ISIN)
			payments, _ = b.bondService.GetPaymentsForDate(ctx, time.Now(), []string{bond.ISIN})
		}
		_ = b.SendMessage(ctx, chatID, FormatBondPassport(bond, marketData, payments))
		return
	}

	parts := strings.Fields(text)
	if len(parts) > 0 {
		cmd := strings.ToLower(strings.SplitN(parts[0], "@", 2)[0])
		if cmd == "/balance" || cmd == "/bal" {
			if len(parts) < 2 {
				_ = b.SendMessage(ctx, chatID, "Использование: `/balance <ISIN/ИНН/Тикер>`\nПример: `/balance RU000A107456` или `/balance 7736050003`")
				return
			}
			query := parts[1]
			if b.finService == nil {
				_ = b.SendMessage(ctx, chatID, "⚠️ Сервис финансовой отчетности временно недоступен\\.")
				return
			}
			balance, err := b.finService.GetIssuerBalance(ctx, query)
			if err != nil {
				if errors.Is(err, service.ErrInvalidIdentifier) {
					_ = b.SendMessage(ctx, chatID, "⚠️ Неверный формат запроса\\. Укажите корректный ISIN \\(12 знаков\\), ИНН \\(10 или 12 цифр\\) или тикер\\.")
					return
				}
				if errors.Is(err, service.ErrInvalidINNChecksum) {
					_ = b.SendMessage(ctx, chatID, "⚠️ Неверная контрольная сумма ИНН\\. Проверьте правильность введенных цифр\\.")
					return
				}
				if errors.Is(err, service.ErrIssuerNotFound) {
					_ = b.SendMessage(ctx, chatID, fmt.Sprintf("⚠️ Эмитент по запросу *%s* не найден в базе ГИР БО \\(ФНС\\)\\.", escapeMarkdown(query)))
					return
				}
				if errors.Is(err, service.ErrBondNotFound) {
					_ = b.SendMessage(ctx, chatID, fmt.Sprintf("⚠️ Облигация *%s* не найдена\\.", escapeMarkdown(query)))
					return
				}
				log.Printf("GetIssuerBalance error for query %q: %v", query, err)
				_ = b.SendMessage(ctx, chatID, fmt.Sprintf("⚠️ Не удалось получить баланс для *%s*\\. Проверьте правильность запроса или повторите позже\\.", escapeMarkdown(query)))
				return
			}
			_ = b.SendMessage(ctx, chatID, FormatIssuerBalance(balance))
			return
		}
	}

	if text == "/today" {
		user, err := b.subService.EnsureUser(ctx, userID, chatID)
		if err != nil || len(user.MonitoredISINs) == 0 {
			_ = b.SendMessage(ctx, chatID, "У вас пока нет отслеживаемых облигаций\\.\nДобавьте облигацию: `/sub add <ISIN>`")
			return
		}
		payments, err := b.bondService.GetPaymentsForDate(ctx, time.Now(), user.MonitoredISINs)
		if err != nil {
			_ = b.SendMessage(ctx, chatID, "Ошибка получения выплат\\.")
			return
		}
		_ = b.SendMessage(ctx, chatID, FormatDatePayments(time.Now(), payments))
		return
	}

	if strings.HasPrefix(text, "/cal") || strings.HasPrefix(text, "/calendar") {
		parts := strings.Fields(text)
		targetDate := time.Now()
		if len(parts) > 1 {
			parsed, err := time.Parse("2006-01-02", parts[1])
			if err == nil {
				targetDate = parsed
			} else {
				parsedDM, err2 := time.Parse("02.01", parts[1])
				if err2 == nil {
					targetDate = time.Date(time.Now().Year(), parsedDM.Month(), parsedDM.Day(), 0, 0, 0, 0, time.UTC)
				}
			}
		}

		user, _ := b.subService.EnsureUser(ctx, userID, chatID)
		payments, err := b.bondService.GetPaymentsForDate(ctx, targetDate, user.MonitoredISINs)
		if err != nil {
			_ = b.SendMessage(ctx, chatID, "Ошибка при запросе календаря\\.")
			return
		}
		_ = b.SendMessage(ctx, chatID, FormatDatePayments(targetDate, payments))
		return
	}

	if strings.HasPrefix(text, "/sub") {
		b.handleSubscription(ctx, userID, chatID, text)
		return
	}

	if strings.HasPrefix(text, "/group") {
		b.handleGroup(ctx, userID, chatID, text)
		return
	}

	// Default fallback: treat text as a bond search query
	bond, marketData, payments, err := b.bondService.GetBondDetails(ctx, text)
	if err == nil && bond != nil {
		_ = b.SendMessage(ctx, chatID, FormatBondPassport(bond, marketData, payments))
		return
	}

	_ = b.SendMessage(ctx, chatID, "Неизвестная команда\\. Введите /help для справки\\.")
}

func (b *Bot) handleSubscription(ctx context.Context, userID, chatID int64, text string) {
	parts := strings.Fields(text)
	if len(parts) < 2 {
		_ = b.SendMessage(ctx, chatID, "Команды подписки:\n• `/sub pay` — утренние уведомления о выплатах\n• `/sub new` — анонсы новых размещений\n• `/sub add <ISIN>` — добавить бумагу в портфель\n• `/unsub` — отписаться")
		return
	}

	switch parts[1] {
	case "pay", "payments":
		_, _ = b.subService.Subscribe(ctx, userID, chatID, "daily_payments")
		_ = b.SendMessage(ctx, chatID, "✅ Вы подписались на утренние уведомления о выплатах купонов\\.")
	case "new", "announcements":
		_, _ = b.subService.Subscribe(ctx, userID, chatID, "new_announcements")
		_ = b.SendMessage(ctx, chatID, "✅ Вы подписались на анонсы новых выпусков облигаций\\.")
	case "add":
		if len(parts) < 3 {
			_ = b.SendMessage(ctx, chatID, "Использование: `/sub add <ISIN>`")
			return
		}
		isin := strings.ToUpper(parts[2])
		_, err := b.subService.AddMonitoredBond(ctx, userID, chatID, isin)
		if err != nil {
			_ = b.SendMessage(ctx, chatID, "Ошибка добавления бумаги\\.")
			return
		}
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("✅ Бумага `%s` добавлена в список отслеживаемых\\.", isin))
	}
}

func (b *Bot) handleGroup(ctx context.Context, userID, chatID int64, text string) {
	parts := strings.Fields(text)
	if len(parts) < 2 {
		_ = b.SendMessage(ctx, chatID, "Команды групп:\n• `/group create <имя>`\n• `/group add <имя> <ISIN>`\n• `/group show <имя>`")
		return
	}

	user, err := b.subService.EnsureUser(ctx, userID, chatID)
	if err != nil {
		_ = b.SendMessage(ctx, chatID, "Ошибка пользователя\\.")
		return
	}

	action := parts[1]
	switch action {
	case "create":
		if len(parts) < 3 {
			_ = b.SendMessage(ctx, chatID, "Использование: `/group create <имя>`")
			return
		}
		name := parts[2]
		user.AddBondToGroup(name, "")
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("✅ Группа *%s* создана\\.", escapeMarkdown(name)))

	case "add":
		if len(parts) < 4 {
			_ = b.SendMessage(ctx, chatID, "Использование: `/group add <имя> <ISIN>`")
			return
		}
		name := parts[2]
		isin := strings.ToUpper(parts[3])
		user.AddBondToGroup(name, isin)
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("✅ Облигация `%s` добавлена в группу *%s*\\.", isin, escapeMarkdown(name)))

	case "show":
		if len(parts) < 3 {
			_ = b.SendMessage(ctx, chatID, "Использование: `/group show <имя>`")
			return
		}
		name := parts[2]
		payments, err := b.bondService.GetPaymentsForUserGroup(ctx, userID, name)
		if err != nil {
			_ = b.SendMessage(ctx, chatID, fmt.Sprintf("Группа *%s* не найдена\\.", escapeMarkdown(name)))
			return
		}
		_ = b.SendMessage(ctx, chatID, fmt.Sprintf("📊 *Выплаты для группы %s*:\n\n%s", escapeMarkdown(name), FormatDatePayments(time.Now(), payments)))
	}
}

func (b *Bot) sendHelp(ctx context.Context, chatID int64) {
	helpText := "🤖 *Бот данных по облигациям MOEX и СПБ Биржи*\n\n" +
		"🔍 *Поиск и информация:*\n" +
		"• `/bond <ISIN/Тикер>` — паспорт облигации и график выплат\n" +
		"• `/balance <ISIN/ИНН/Тикер>` — финансовый баланс и долговая нагрузка \\(ФНС ГИР БО\\)\n" +
		"• `/today` — выплаты на сегодня по вашим бумагам\n" +
		"• `/cal 2026-10-15` — выплаты на конкретную дату\n\n" +
		"📁 *Группы и портфели:*\n" +
		"• `/group create <название>` — создать группу\n" +
		"• `/group add <группа> <ISIN>` — добавить бумагу\n" +
		"• `/group show <группа>` — график выплат группы\n\n" +
		"🔔 *Подписки:*\n" +
		"• `/sub pay` — утренний дайджест купонов\n" +
		"• `/sub new` — анонсы новых размещений\n" +
		"• `/sub add <ISIN>` — добавить в мониторинг\n"
	_ = b.SendMessage(ctx, chatID, helpText)
}

// CronDailyDigestHandler is triggered by Cloud Scheduler.
func (b *Bot) CronDailyDigestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	msgs, err := b.notifier.RunDailyDigest(r.Context(), time.Now())
	if err != nil {
		log.Printf("Cron daily digest execution error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf(`{"status":"ok","sent":%d}`, len(msgs))))
}
