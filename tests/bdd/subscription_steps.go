package bdd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/service"
)

func registerSubscriptionSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	mockProv := newMockBondProvider()

	ctx.Step(`^a clean subscription storage$`, func() error {
		tc.memStore = tc.memStore // already fresh per scenario in InitializeScenario
		tc.notifier = service.NewNotifierService(mockProv, tc.memStore, nil)
		tc.subService = service.NewSubscriptionService(tc.memStore)
		return nil
	})

	ctx.Step(`^a user with Telegram ID (\d+)$`, func(id int64) error {
		user, err := tc.subService.EnsureUser(tc.ctx, id, id)
		if err != nil {
			return err
		}
		tc.currentUser = user
		return nil
	})

	ctx.Step(`^the user subscribes to "([^"]*)"$`, func(subType string) error {
		user, err := tc.subService.Subscribe(tc.ctx, tc.currentUser.TelegramID, tc.currentUser.ChatID, subType)
		if err != nil {
			return err
		}
		tc.currentUser = user
		return nil
	})

	ctx.Step(`^the user adds bond "([^"]*)" to their monitored bonds$`, func(isin string) error {
		user, err := tc.subService.AddMonitoredBond(tc.ctx, tc.currentUser.TelegramID, tc.currentUser.ChatID, isin)
		if err != nil {
			return err
		}
		tc.currentUser = user
		return nil
	})

	ctx.Step(`^the user subscription state should have "([^"]*)" enabled$`, func(subType string) error {
		switch subType {
		case "daily_payments":
			if !tc.currentUser.NotifyDailyPayments {
				return fmt.Errorf("expected NotifyDailyPayments to be true")
			}
		case "new_announcements":
			if !tc.currentUser.NotifyAnnouncements {
				return fmt.Errorf("expected NotifyAnnouncements to be true")
			}
		}
		return nil
	})

	ctx.Step(`^the monitored bonds count for user (\d+) should be (\d+)$`, func(userID int64, count int) error {
		user, err := tc.memStore.GetUser(tc.ctx, userID)
		if err != nil {
			return err
		}
		if len(user.MonitoredISINs) != count {
			return fmt.Errorf("expected %d monitored bonds, got %d", count, len(user.MonitoredISINs))
		}
		return nil
	})

	ctx.Step(`^user (\d+) is subscribed to "([^"]*)" with bonds:$`, func(id int64, subType string, table *godog.Table) error {
		user, err := tc.subService.Subscribe(tc.ctx, id, id, subType)
		if err != nil {
			return err
		}
		for _, row := range table.Rows[1:] {
			isin := row.Cells[0].Value
			user.AddMonitoredISIN(isin)
		}
		tc.currentUser = user
		return tc.memStore.SaveUser(tc.ctx, user)
	})

	ctx.Step(`^today's date is "([^"]*)"$`, func(dateStr string) error {
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return err
		}
		tc.todayDate = t
		return nil
	})

	ctx.Step(`^bond "([^"]*)" has a coupon payment of ([0-9.]+) on "([^"]*)"$`, func(isin string, amountStr, dateStr string) error {
		amount, _ := strconv.ParseFloat(amountStr, 64)
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return err
		}

		payment := domain.Payment{
			ISIN:     isin,
			BondName: "ОФЗ 26238",
			Date:     t,
			Amount:   amount,
			Type:     domain.PaymentCoupon,
		}

		mockProv.datePayments[dateStr] = append(mockProv.datePayments[dateStr], payment)
		mockProv.payments[isin] = append(mockProv.payments[isin], payment)
		return nil
	})

	ctx.Step(`^the daily digest scheduler runs for "([^"]*)"$`, func(dateStr string) error {
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return err
		}
		_, err = tc.notifier.RunDailyDigest(tc.ctx, t)
		return err
	})

	ctx.Step(`^a notification message is queued for user (\d+)$`, func(userID int64) error {
		for _, msg := range tc.notifier.Outbox {
			if msg.TelegramID == userID {
				return nil
			}
		}
		return fmt.Errorf("no queued message found for user %d (total in outbox: %d)", userID, len(tc.notifier.Outbox))
	})

	ctx.Step(`^the notification message contains "([^"]*)"$`, func(expectedText string) error {
		if len(tc.notifier.Outbox) == 0 {
			return fmt.Errorf("outbox is empty")
		}
		lastMsg := tc.notifier.Outbox[len(tc.notifier.Outbox)-1]
		if !strings.Contains(lastMsg.Text, expectedText) {
			return fmt.Errorf("expected text %q in message: %s", expectedText, lastMsg.Text)
		}
		return nil
	})

	ctx.Step(`^the notification message mentions "([^"]*)"$`, func(mention string) error {
		if len(tc.notifier.Outbox) == 0 {
			return fmt.Errorf("outbox is empty")
		}
		lastMsg := tc.notifier.Outbox[len(tc.notifier.Outbox)-1]
		if !strings.Contains(lastMsg.Text, mention) {
			return fmt.Errorf("expected mention of %q in message: %s", mention, lastMsg.Text)
		}
		return nil
	})

	ctx.Step(`^a new bond placement is detected:$`, func(table *godog.Table) error {
		var ann domain.Announcement
		for _, row := range table.Rows {
			field := row.Cells[0].Value
			val := row.Cells[1].Value
			switch field {
			case "Title":
				ann.Title = val
				ann.Issuer = "ГМК Норильский Никель"
			case "Exchange":
				ann.Exchange = domain.Exchange(val)
			case "VolumeRUB":
				ann.VolumeRUB, _ = strconv.ParseFloat(val, 64)
			case "TargetCoupon":
				ann.TargetCoupon = val
			}
		}
		tc.lastAnnouncement = ann
		return nil
	})

	ctx.Step(`^the announcement monitor runs$`, func() error {
		_, err := tc.notifier.BroadcastAnnouncement(tc.ctx, tc.lastAnnouncement)
		return err
	})
}
