package telegram

import (
	"strings"
	"testing"

	"telegram-bot-moex/internal/domain"
)

func TestFormatIssuerBalance_Corporate(t *testing.T) {
	bal := &domain.IssuerBalance{
		INN:             "7740000076",
		IssuerName:      "ПАО «МТС»",
		ReportYear:      2025,
		TotalAssets:     1023.0e9,
		Equity:          135.2e9,
		LongTermDebt:    377.6e9,
		ShortTermDebt:   166.9e9,
		CashEquivalents: 16.1e9,
		Revenue:         391.6e9,
		NetProfit:       31.3e9,
		AuditorName:     "АО ДРТ",
	}
	bal.CalculateMetrics()

	msg := FormatIssuerBalance(bal)

	expectedSubstrings := []string{
		"ПАО «МТС»",
		"7740000076",
		"2025",
		"1\\.0 трлн ₽",
		"135\\.2 млрд ₽",
		"544\\.5 млрд ₽",
		"528\\.4 млрд ₽",
		"3\\.91",
		"Высокая нагрузка",
		"391\\.6 млрд ₽",
		"31\\.3 млрд ₽",
		"АО ДРТ",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(msg, sub) {
			t.Errorf("expected formatted message to contain %q, but got:\n%s", sub, msg)
		}
	}
}

func TestFormatIssuerBalance_Sovereign(t *testing.T) {
	bal := &domain.IssuerBalance{
		INN:         "7710168360",
		IssuerName:  "Министерство финансов РФ",
		IsSovereign: true,
		RiskLevel:   domain.RiskSovereign,
	}

	msg := FormatIssuerBalance(bal)

	if !strings.Contains(msg, "Суверенный эмитент") {
		t.Errorf("expected 'Суверенный эмитент', got:\n%s", msg)
	}
	if !strings.Contains(msg, "ОФЗ") {
		t.Errorf("expected 'ОФЗ', got:\n%s", msg)
	}
}

func TestFormatIssuerBalance_CreditOrg(t *testing.T) {
	bal := &domain.IssuerBalance{
		INN:         "7702045051",
		IssuerName:  "ПАО МТС-Банк",
		IsCreditOrg: true,
		RiskLevel:   domain.RiskBank,
	}

	msg := FormatIssuerBalance(bal)

	if !strings.Contains(msg, "Кредитная организация") {
		t.Errorf("expected 'Кредитная организация', got:\n%s", msg)
	}
	if !strings.Contains(msg, "Банка России") {
		t.Errorf("expected 'Банка России', got:\n%s", msg)
	}
}

func TestFormatIssuerBalance_NegativeEquity(t *testing.T) {
	bal := &domain.IssuerBalance{
		INN:           "1234567890",
		IssuerName:    "ООО Тест",
		ReportYear:    2024,
		TotalAssets:   10e9,
		Equity:        -5e9,
		LongTermDebt:  8e9,
		ShortTermDebt: 3e9,
	}
	bal.CalculateMetrics()

	msg := FormatIssuerBalance(bal)

	if !strings.Contains(msg, "отриц\\. капитал") {
		t.Errorf("expected 'отриц\\. капитал', got:\n%s", msg)
	}
	if !strings.Contains(msg, "Высокая нагрузка") {
		t.Errorf("expected 'Высокая нагрузка', got:\n%s", msg)
	}
}

func TestFormatIssuerBalance_CashRichNegativeNetDebt(t *testing.T) {
	bal := &domain.IssuerBalance{
		INN:             "7708304947",
		IssuerName:      "ООО Богатый Кэшем",
		ReportYear:      2025,
		TotalAssets:     50e9,
		Equity:          30e9,
		LongTermDebt:    5e9,
		ShortTermDebt:   2e9,
		CashEquivalents: 15e9, // Total debt 7B, Cash 15B -> Net Debt = -8B
	}
	bal.CalculateMetrics()

	msg := FormatIssuerBalance(bal)

	if !strings.Contains(msg, "Чистая ден\\. позиция") {
		t.Errorf("expected 'Чистая ден\\. позиция', got:\n%s", msg)
	}
	if !strings.Contains(msg, "Низкая нагрузка") {
		t.Errorf("expected 'Низкая нагрузка', got:\n%s", msg)
	}
}
