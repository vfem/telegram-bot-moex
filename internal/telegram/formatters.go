package telegram

import (
	"fmt"
	"strings"
	"time"

	"telegram-bot-moex/internal/domain"
)

// FormatBondPassport formats bond passport and upcoming payments into a clean Telegram Markdown message.
func FormatBondPassport(bond *domain.Bond, payments []domain.Payment) string {
	var sb strings.Builder

	exchangeIcon := "🏛"
	if bond.Exchange == domain.ExchangeSPBE {
		exchangeIcon = "⚓️"
	}

	sb.WriteString(fmt.Sprintf("%s *%s* (`%s`)\n", exchangeIcon, escapeMarkdown(bond.Name), bond.ISIN))
	if bond.Ticker != "" && bond.Ticker != bond.ISIN {
		sb.WriteString(fmt.Sprintf("Тикер: `%s` • Биржа: *%s*\n", bond.Ticker, bond.Exchange))
	} else {
		sb.WriteString(fmt.Sprintf("Биржа: *%s*\n", bond.Exchange))
	}

	if bond.Issuer != "" {
		sb.WriteString(fmt.Sprintf("Эмитент: %s\n", escapeMarkdown(bond.Issuer)))
	}

	nominalStr := fmt.Sprintf("%.0f %s", bond.NominalValue, bond.Currency)
	if bond.NominalValue == 0 {
		nominalStr = "1 000 RUB"
	}
	sb.WriteString(fmt.Sprintf("Номинал: *%s*", nominalStr))

	if !bond.MaturityDate.IsZero() {
		sb.WriteString(fmt.Sprintf(" • Погашение: *%s*", bond.MaturityDate.Format("02.01.2006")))
	}
	sb.WriteString("\n")

	if bond.CouponRatePct > 0 {
		freq := ""
		if bond.CouponsPerYear > 0 {
			freq = fmt.Sprintf(" (%d вып./год)", bond.CouponsPerYear)
		}
		sb.WriteString(fmt.Sprintf("Купон: *%.2f%%*%s\n", bond.CouponRatePct, freq))
	}

	if bond.HasAmortization {
		sb.WriteString("Амортизация: *Да*\n")
	}

	sb.WriteString("\n📅 *Ближайшие выплаты:*\n")
	if len(payments) == 0 {
		sb.WriteString("_График выплат уточняется_\n")
	} else {
		now := time.Now()
		count := 0
		for _, p := range payments {
			// Show future payments or up to 4
			if p.Date.After(now.AddDate(0, 0, -1)) && count < 5 {
				typeLabel := "Купон"
				if p.Type == domain.PaymentAmortization {
					typeLabel = "Амортизация"
				} else if p.Type == domain.PaymentMaturity {
					typeLabel = "Погашение"
				}

				sb.WriteString(fmt.Sprintf("• `%s`: %s *%.2f %s*\n",
					p.Date.Format("02.01.2006"),
					typeLabel,
					p.Amount,
					bond.Currency,
				))
				count++
			}
		}
		if count == 0 {
			// Show last scheduled
			for i, p := range payments {
				if i >= 3 {
					break
				}
				sb.WriteString(fmt.Sprintf("• `%s`: *%.2f %s*\n", p.Date.Format("02.01.2006"), p.Amount, bond.Currency))
			}
		}
	}

	return sb.String()
}

// FormatDatePayments formats all payments on a specific date.
func FormatDatePayments(date time.Time, payments []domain.Payment) string {
	if len(payments) == 0 {
		return fmt.Sprintf("На дату *%s* выплат по облигациям не запланировано.", date.Format("02.01.2006"))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📅 *Выплаты на %s* (%d):\n\n", date.Format("02.01.2006"), len(payments)))

	var totalAmount float64
	for _, p := range payments {
		totalAmount += p.Amount
		name := p.BondName
		if name == "" {
			name = p.ISIN
		}
		sb.WriteString(fmt.Sprintf("• *%s* (`%s`)\n  Выплата: *%.2f ₽* (%s)\n",
			escapeMarkdown(name),
			p.ISIN,
			p.Amount,
			p.Type,
		))
	}

	sb.WriteString(fmt.Sprintf("\n💰 Суммарно на 1 шт.: *%.2f ₽*", totalAmount))
	return sb.String()
}

// FormatAnnouncement formats a new placement notice.
func FormatAnnouncement(ann *domain.Announcement) string {
	var sb strings.Builder
	sb.WriteString("🔔 *Новый анонс размещения облигаций*\n\n")
	sb.WriteString(fmt.Sprintf("Выпуск: *%s*\n", escapeMarkdown(ann.Title)))
	if ann.Issuer != "" {
		sb.WriteString(fmt.Sprintf("Эмитент: %s\n", escapeMarkdown(ann.Issuer)))
	}
	sb.WriteString(fmt.Sprintf("Биржа: *%s*\n", ann.Exchange))

	if ann.VolumeRUB > 0 {
		if ann.VolumeRUB >= 1e9 {
			sb.WriteString(fmt.Sprintf("Объем: *%.1f млрд ₽*\n", ann.VolumeRUB/1e9))
		} else {
			sb.WriteString(fmt.Sprintf("Объем: *%.0f млн ₽*\n", ann.VolumeRUB/1e6))
		}
	}

	if ann.TargetCoupon != "" {
		sb.WriteString(fmt.Sprintf("Ориентир купона: *%s*\n", ann.TargetCoupon))
	}
	if !ann.BookDate.IsZero() {
		sb.WriteString(fmt.Sprintf("Сбор заявок: *%s*\n", ann.BookDate.Format("02.01.2006")))
	}

	return sb.String()
}

// Helper to escape basic markdown chars
func escapeMarkdown(s string) string {
	replacer := strings.NewReplacer(
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
