package bdd

import (
	"fmt"
	"net/http/httptest"
	"strings"

	"github.com/cucumber/godog"
	"telegram-bot-moex/internal/telegram"
)

func registerSecuritySteps(sc *godog.ScenarioContext, tc *TestContext) {
	sc.Step(`^a bot initialized with secret token "([^"]*)"$`, func(secret string) error {
		tc.bot = telegram.NewBot("", secret, tc.bondService, tc.subService, tc.notifier)
		return nil
	})

	sc.Step(`^a bot initialized in production mode with bot token "([^"]*)" and no secret token$`, func(token string) error {
		tc.bot = telegram.NewBot(token, "", tc.bondService, tc.subService, tc.notifier)
		return nil
	})

	sc.Step(`^an HTTP "([^"]*)" request is sent to "([^"]*)"$`, func(method, path string) error {
		return executeRequest(tc, method, path, nil, "")
	})

	sc.Step(`^an HTTP "([^"]*)" request is sent to "([^"]*)" without secret token header$`, func(method, path string) error {
		return executeRequest(tc, method, path, nil, "")
	})

	sc.Step(`^an HTTP "([^"]*)" request is sent to "([^"]*)" with body "([^"]*)"$`, func(method, path, body string) error {
		return executeRequest(tc, method, path, strings.NewReader(body), "")
	})

	sc.Step(`^an HTTP "([^"]*)" request is sent to "([^"]*)" with secret token header "([^"]*)"$`, func(method, path, secret string) error {
		return executeRequest(tc, method, path, strings.NewReader(`{"update_id":1}`), secret)
	})

	sc.Step(`^the response status should be (\d+)$`, func(expectedStatus int) error {
		if tc.lastResponseCode != expectedStatus {
			return fmt.Errorf("expected response status %d, got %d (body: %s)", expectedStatus, tc.lastResponseCode, tc.lastResponseBody)
		}
		return nil
	})
}

func executeRequest(tc *TestContext, method, path string, body *strings.Reader, secretHeader string) error {
	var bodyReader *strings.Reader
	if body != nil {
		bodyReader = body
	} else {
		bodyReader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if secretHeader != "" {
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", secretHeader)
	}
	rec := httptest.NewRecorder()

	if tc.bot == nil {
		tc.bot = telegram.NewBot("", "", tc.bondService, tc.subService, tc.notifier)
	}

	switch path {
	case "/webhook":
		tc.bot.WebhookHandler(rec, req)
	case "/cron/daily-digest":
		tc.bot.CronDailyDigestHandler(rec, req)
	default:
		return fmt.Errorf("unknown test path: %s", path)
	}

	tc.lastResponseCode = rec.Code
	tc.lastResponseBody = rec.Body.String()
	return nil
}
