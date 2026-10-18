package telegram

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"telegram-bot-moex/internal/domain"
)

func TestFormatMaterialFacts_CorporateAndTruncationAndEscaping(t *testing.T) {
	var facts []domain.MaterialFact
	for i := 1; i <= 7; i++ {
		facts = append(facts, domain.MaterialFact{
			ID:          fmt.Sprintf("f-%d", i),
			INN:         "7708304947",
			Title:       fmt.Sprintf("Выплата купонного дохода #%d (серия БО-01)", i),
			PublishedAt: time.Date(2026, 10, i, 14, 15, 0, 0, time.UTC),
			URL:         "https://www.e-disclosure.ru/portal/event.aspx?id=1(test)",
		})
	}

	out := FormatMaterialFacts("ПАО «ЕвроТранс»", "7708304947", facts)

	if !strings.Contains(out, `*Существенные факты: ПАО «ЕвроТранс»*`) {
		t.Errorf("expected header with issuer name, got:\n%s", out)
	}
	if !strings.Contains(out, `\(ИНН `+"`7708304947`"+`\)`) {
		t.Errorf("expected header with INN, got:\n%s", out)
	}
	if !strings.Contains(out, "`01.10.2026 14:15`") {
		t.Errorf("expected date with HH:MM when non-zero, got:\n%s", out)
	}
	// Verify URL parentheses escaping in MarkdownV2 link target (only ')' and '\' are escaped in MarkdownV2 URLs)
	if !strings.Contains(out, `[Подробнее](https://www.e-disclosure.ru/portal/event.aspx?id=1(test\))`) {
		t.Errorf("expected escaped ')' inside URL target, got:\n%s", out)
	}
	// Verify truncation to at most 5 items
	if strings.Contains(out, "#6") || strings.Contains(out, "#7") {
		t.Errorf("expected output to truncate at 5 facts, got:\n%s", out)
	}
}

func TestFormatMaterialFacts_EmptyAndSovereign(t *testing.T) {
	emptyOut := FormatMaterialFacts("ПАО «ЕвроТранс»", "7708304947", nil)
	if !strings.Contains(emptyOut, "нет свежих существенных фактов") {
		t.Errorf("expected empty state message, got:\n%s", emptyOut)
	}

	sovOut := FormatMaterialFacts("Министерство финансов РФ", "7710168360", nil)
	if !strings.Contains(sovOut, "Суверенный эмитент") || !strings.Contains(sovOut, "ОФЗ") {
		t.Errorf("expected sovereign OFZ explanation, got:\n%s", sovOut)
	}
}
