package domain

import (
	"testing"
)

func TestClassifyFact(t *testing.T) {
	tests := []struct {
		name         string
		title        string
		summary      string
		wantCategory FactCategory
		wantSeverity FactSeverity
		wantAlert    bool
	}{
		{
			name:         "Critical non-performance of coupon obligations",
			title:        "Неисполнение обязательств эмитента перед владельцами его эмиссионных ценных бумаг",
			wantCategory: FactDefault,
			wantSeverity: SeverityCritical,
			wantAlert:    true,
		},
		{
			name:         "Critical technical default",
			title:        "Сообщение о техническом дефолте по облигациям серии БО-01",
			wantCategory: FactDefault,
			wantSeverity: SeverityCritical,
			wantAlert:    true,
		},
		{
			name:         "Critical bankruptcy claim",
			title:        "Заявление о признании должника банкротом (несостоятельности)",
			wantCategory: FactDefault,
			wantSeverity: SeverityCritical,
			wantAlert:    true,
		},
		{
			name:         "Critical trading suspension",
			title:        "О приостановлении торгов ценными бумагами эмитента",
			wantCategory: FactDefault,
			wantSeverity: SeverityCritical,
			wantAlert:    true,
		},
		{
			name:         "Warning offer buyback and coupon rate determination",
			title:        "Определение ставки купона и даты начала периода предъявления бумаг к выкупу",
			wantCategory: FactOffer,
			wantSeverity: SeverityWarning,
			wantAlert:    true,
		},
		{
			name:         "Warning acquisition of securities by issuer (put offer)",
			title:        "О приобретении ценных бумаг эмитентом по требованию владельцев (оферта)",
			wantCategory: FactOffer,
			wantSeverity: SeverityWarning,
			wantAlert:    true,
		},
		{
			name:         "Warning litigation / arbitration dispute",
			title:        "Возбуждение арбитражного судебного спора на сумму свыше 10% активов",
			wantCategory: FactLitigation,
			wantSeverity: SeverityWarning,
			wantAlert:    true,
		},
		{
			name:         "Warning lawsuit at end of string without trailing space",
			title:        "Против эмитента подан иск",
			wantCategory: FactLitigation,
			wantSeverity: SeverityWarning,
			wantAlert:    true,
		},
		{
			name:         "Warning lawsuit before punctuation",
			title:        "О предъявлении иска, связанного с договором поставки",
			wantCategory: FactLitigation,
			wantSeverity: SeverityWarning,
			wantAlert:    true,
		},
		{
			name:         "Warning rating downgrade",
			title:        "АКРА сообщило о понижении кредитного рейтинга эмитента до уровня BB(RU)",
			wantCategory: FactRatingChange,
			wantSeverity: SeverityWarning,
			wantAlert:    true,
		},
		{
			name:         "Info rating assignment",
			title:        "Эксперт РА подтвердило кредитный рейтинг на уровне ruA- со стабильным прогнозом",
			wantCategory: FactRatingChange,
			wantSeverity: SeverityInfo,
			wantAlert:    false,
		},
		{
			name:         "Info coupon payment in full",
			title:        "Выплата купонного дохода по облигациям серии БО-001Р-03 в полном объеме",
			wantCategory: FactCoupon,
			wantSeverity: SeverityInfo,
			wantAlert:    false,
		},
		{
			name:         "Info board of directors decisions",
			title:        "Решения заседания совета директоров эмитента",
			wantCategory: FactMeeting,
			wantSeverity: SeverityInfo,
			wantAlert:    false,
		},
		{
			name:         "Info general shareholders meeting",
			title:        "О созыве внеочередного общего собрания акционеров (ВОСА)",
			wantCategory: FactMeeting,
			wantSeverity: SeverityInfo,
			wantAlert:    false,
		},
		{
			name:         "Info generic corporate announcement",
			title:        "Публикация ежеквартального отчета эмитента за 3 квартал",
			wantCategory: FactOther,
			wantSeverity: SeverityInfo,
			wantAlert:    false,
		},
		// False-positive regression cases (F3)
		{
			name:         "Regression F3.1: транспорт in coupon payment is not FactLitigation",
			title:        "Выплата купонного дохода ПАО Государственная транспортная лизинговая компания",
			wantCategory: FactCoupon,
			wantSeverity: SeverityInfo,
			wantAlert:    false,
		},
		{
			name:         "Regression F3.1: распоряжение in board meeting is not FactLitigation",
			title:        "Решения совета директоров о распоряжении имуществом и паспорт сделки",
			wantCategory: FactMeeting,
			wantSeverity: SeverityInfo,
			wantAlert:    false,
		},
		{
			name:         "Regression F3.2: риска in rating action is not FactLitigation",
			title:        "АКРА отметило снижение кредитного риска эмитента и поиск новых рынков",
			wantCategory: FactRatingChange,
			wantSeverity: SeverityInfo,
			wantAlert:    false,
		},
		{
			name:         "Regression F3.3: требованиям Положения ЦБ is not FactLitigation",
			title:        "Раскрытие отчетности в соответствии с требованиями Положения Банка России № 714-П",
			wantCategory: FactOther,
			wantSeverity: SeverityInfo,
			wantAlert:    false,
		},
		{
			name:         "Regression F3.4: предъявлении иска is FactLitigation not FactOffer",
			title:        "О предъявлении иска в арбитражный суд к дочернему обществу",
			wantCategory: FactLitigation,
			wantSeverity: SeverityWarning,
			wantAlert:    true,
		},
		{
			name:         "Regression F3.5: приостановлении полномочий гендиректора is FactMeeting not FactDefault",
			title:        "Решения совета директоров о приостановлении полномочий генерального директора",
			wantCategory: FactMeeting,
			wantSeverity: SeverityInfo,
			wantAlert:    false,
		},
		{
			name:         "Regression F3.6: конкретному выпуску is not FactRatingChange (нкр substring)",
			title:        "Выплата купонного дохода по конкретному выпуску облигаций",
			wantCategory: FactCoupon,
			wantSeverity: SeverityInfo,
			wantAlert:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fact := MaterialFact{
				Title:   tc.title,
				Summary: tc.summary,
			}
			fact.Classify()

			if fact.Category != tc.wantCategory {
				t.Errorf("Category = %q, want %q", fact.Category, tc.wantCategory)
			}
			if fact.Severity != tc.wantSeverity {
				t.Errorf("Severity = %q, want %q", fact.Severity, tc.wantSeverity)
			}
			if fact.IsAlertWorthy() != tc.wantAlert {
				t.Errorf("IsAlertWorthy() = %v, want %v", fact.IsAlertWorthy(), tc.wantAlert)
			}
		})
	}
}

func TestFactShortTagAndEmoji(t *testing.T) {
	if FactDefault.ShortTag() != "Дефолт" {
		t.Errorf("unexpected ShortTag for FactDefault: %s", FactDefault.ShortTag())
	}
	if FactOffer.ShortTag() != "Оферта" {
		t.Errorf("unexpected ShortTag for FactOffer: %s", FactOffer.ShortTag())
	}
	if FactCoupon.ShortTag() != "Купон" {
		t.Errorf("unexpected ShortTag for FactCoupon: %s", FactCoupon.ShortTag())
	}
	if FactMeeting.ShortTag() != "СД" {
		t.Errorf("unexpected ShortTag for FactMeeting: %s", FactMeeting.ShortTag())
	}
	if FactLitigation.ShortTag() != "Суд" {
		t.Errorf("unexpected ShortTag for FactLitigation: %s", FactLitigation.ShortTag())
	}
	if FactRatingChange.ShortTag() != "Рейтинг" {
		t.Errorf("unexpected ShortTag for FactRatingChange: %s", FactRatingChange.ShortTag())
	}
	if FactOther.ShortTag() != "Событие" {
		t.Errorf("unexpected ShortTag for FactOther: %s", FactOther.ShortTag())
	}
	if SeverityCritical.Emoji() != "🚨" {
		t.Errorf("unexpected Emoji for SeverityCritical: %s", SeverityCritical.Emoji())
	}
	if SeverityWarning.Emoji() != "⚠️" {
		t.Errorf("unexpected Emoji for SeverityWarning: %s", SeverityWarning.Emoji())
	}
	if SeverityInfo.Emoji() != "ℹ️" {
		t.Errorf("unexpected Emoji for SeverityInfo: %s", SeverityInfo.Emoji())
	}
}
