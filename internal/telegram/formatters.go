package telegram

import (
	"fmt"
	"strings"
	"time"

	"telegram-bot-moex/internal/domain"
)

// bold wraps text in *...* with inner content escaped for MarkdownV2.
func bold(s string) string {
	return "*" + escapeMarkdown(s) + "*"
}

// italic wraps text in _..._ with inner content escaped for MarkdownV2.
func italic(s string) string {
	return "_" + escapeMarkdown(s) + "_"
}

// code wraps text in `...` with inner content escaped for code spans.
func code(s string) string {
	return "`" + escapeCode(s) + "`"
}

// FormatBondPassport formats bond passport, live market data, and upcoming payments into a clean Telegram Markdown message.
func FormatBondPassport(bond *domain.Bond, md *domain.MarketData, payments []domain.Payment) string {
	var sb strings.Builder

	exchangeIcon := "🏛"
	if bond.Exchange == domain.ExchangeSPBE {
		exchangeIcon = "⚓️"
	}

	sb.WriteString(fmt.Sprintf("%s %s \\(%s\\)\n", exchangeIcon, bold(bond.Name), code(bond.ISIN)))
	if bond.Ticker != "" && bond.Ticker != bond.ISIN {
		sb.WriteString(fmt.Sprintf("Тикер: %s • Биржа: %s\n", code(bond.Ticker), bold(string(bond.Exchange))))
	} else {
		sb.WriteString(fmt.Sprintf("Биржа: %s\n", bold(string(bond.Exchange))))
	}

	if bond.Issuer != "" {
		if bond.IssuerINN != "" {
			sb.WriteString(fmt.Sprintf("Эмитент: %s \\(ИНН: %s\\)\n", escapeMarkdown(bond.Issuer), code(bond.IssuerINN)))
		} else {
			sb.WriteString(fmt.Sprintf("Эмитент: %s\n", escapeMarkdown(bond.Issuer)))
		}
	}

	nominalVal := bond.NominalValue
	curr := bond.Currency
	if nominalVal == 0 {
		nominalVal = 1000
		curr = "RUB"
	}
	nominalStr := fmt.Sprintf("%.0f %s", nominalVal, curr)
	sb.WriteString(fmt.Sprintf("Номинал: %s", bold(nominalStr)))

	if !bond.MaturityDate.IsZero() {
		sb.WriteString(fmt.Sprintf(" • Погашение: %s", bold(bond.MaturityDate.Format("02.01.2006"))))
	}
	sb.WriteString("\n")

	if bond.CouponRatePct > 0 {
		freq := ""
		if bond.CouponsPerYear > 0 {
			freq = fmt.Sprintf(" \\(%d вып\\./год\\)", bond.CouponsPerYear)
		}
		sb.WriteString(fmt.Sprintf("Купон: %s%s\n", bold(fmt.Sprintf("%.2f%%", bond.CouponRatePct)), freq))
	}

	if bond.HasAmortization {
		sb.WriteString(fmt.Sprintf("Амортизация: %s\n", bold("Да")))
	}

	if md != nil {
		sb.WriteString("\n📈 " + bold("Торги и ликвидность:") + "\n")

		currSym := md.CurrencySymbol()

		// Price line (C-11: if LastPricePct == 0, show "н/д")
		if md.LastPricePct > 0 {
			prevMarker := ""
			if md.IsPreviousClose {
				prevMarker = " \\(цена закр\\.\\)"
			}
			sb.WriteString(fmt.Sprintf("• Цена: %s \\(%s %s\\)%s • НКД: %s\n",
				bold(fmt.Sprintf("%.2f%%", md.LastPricePct)),
				escapeMarkdown(fmt.Sprintf("%.2f", md.LastPriceRub)),
				currSym,
				prevMarker,
				bold(fmt.Sprintf("%.2f %s", md.AccruedCoupon, currSym)),
			))
		} else {
			sb.WriteString(fmt.Sprintf("• Цена: %s • НКД: %s\n",
				bold("н/д"),
				bold(fmt.Sprintf("%.2f %s", md.AccruedCoupon, currSym)),
			))
		}

		// YTM & Duration line (C-14: duration with Russian decimal comma)
		durationStr := md.DurationHumanized()
		if md.YTM > 0 && durationStr != "" {
			sb.WriteString(fmt.Sprintf("• Доходность \\(YTM\\): %s • Дюрация: %s\n",
				bold(fmt.Sprintf("%.2f%%", md.YTM)),
				bold(durationStr),
			))
		} else if md.YTM > 0 {
			sb.WriteString(fmt.Sprintf("• Доходность \\(YTM\\): %s\n",
				bold(fmt.Sprintf("%.2f%%", md.YTM)),
			))
		} else if durationStr != "" {
			sb.WriteString(fmt.Sprintf("• Дюрация: %s\n",
				bold(durationStr),
			))
		}

		// Volume line
		var volValStr string
		if md.VolumeTodayRub >= 1e9 {
			volValStr = fmt.Sprintf("%.1f млрд ₽", md.VolumeTodayRub/1e9)
		} else if md.VolumeTodayRub >= 1e6 {
			volValStr = fmt.Sprintf("%.1f млн ₽", md.VolumeTodayRub/1e6)
		} else if md.VolumeTodayRub >= 1e3 {
			volValStr = fmt.Sprintf("%.0f тыс. ₽", md.VolumeTodayRub/1e3)
		} else {
			volValStr = fmt.Sprintf("%.0f ₽", md.VolumeTodayRub)
		}
		sb.WriteString(fmt.Sprintf("• Оборот сегодня: %s \\(%s\\)\n",
			bold(volValStr),
			escapeMarkdown(pluralizeTrades(md.TradesCount)),
		))

		// Liquidity & Spread line (C-05, C-06, C-07)
		if md.Liquidity == domain.LiquidityNoTrades {
			sb.WriteString(fmt.Sprintf("• Ликвидность: ⏳ %s\n",
				bold(string(md.Liquidity)),
			))
		} else {
			liqIcon := "🔥"
			switch md.Liquidity {
			case domain.LiquidityMedium:
				liqIcon = "⚡️"
			case domain.LiquidityLow:
				liqIcon = "⚠️"
			case domain.LiquidityIlliquid:
				liqIcon = "🛑"
			}

			spreadStr := "н/д"
			if md.HasQuote {
				spreadStr = bold(fmt.Sprintf("%.2f%%", md.SpreadPct))
			}
			sb.WriteString(fmt.Sprintf("• Ликвидность: %s %s \\(спред: %s\\)\n",
				liqIcon,
				bold(string(md.Liquidity)),
				spreadStr,
			))
		}
	}

	sb.WriteString("\n📅 " + bold("Ближайшие выплаты:") + "\n")
	if len(payments) == 0 {
		sb.WriteString(italic("График выплат уточняется") + "\n")
	} else {
		now := time.Now()
		count := 0
		for _, p := range payments {
			// Show future payments or up to 5
			if p.Date.After(now.AddDate(0, 0, -1)) && count < 5 {
				typeLabel := "Купон"
				if p.Type == domain.PaymentAmortization {
					typeLabel = "Амортизация"
				} else if p.Type == domain.PaymentMaturity {
					typeLabel = "Погашение"
				}

				var amountStr string
				if p.Amount == 0 {
					amountStr = italic("размер уточняется")
				} else {
					amountStr = bold(fmt.Sprintf("%.2f %s", p.Amount, bond.Currency))
				}

				sb.WriteString(fmt.Sprintf("• %s: %s %s\n",
					code(p.Date.Format("02.01.2006")),
					escapeMarkdown(typeLabel),
					amountStr,
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
				var amountStr string
				if p.Amount == 0 {
					amountStr = italic("размер уточняется")
				} else {
					amountStr = bold(fmt.Sprintf("%.2f %s", p.Amount, bond.Currency))
				}
				sb.WriteString(fmt.Sprintf("• %s: %s\n",
					code(p.Date.Format("02.01.2006")),
					amountStr,
				))
			}
		}
	}

	return sb.String()
}

// FormatDatePayments formats all payments on a specific date.
func FormatDatePayments(date time.Time, payments []domain.Payment) string {
	dateStr := date.Format("02.01.2006")
	if len(payments) == 0 {
		return fmt.Sprintf("На дату %s выплат по облигациям не запланировано.", bold(dateStr))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📅 %s \\(%d\\):\n\n",
		bold(fmt.Sprintf("Выплаты на %s", dateStr)),
		len(payments),
	))

	var totalAmount float64
	for _, p := range payments {
		totalAmount += p.Amount
		name := p.BondName
		if name == "" {
			name = p.ISIN
		}
		sb.WriteString(fmt.Sprintf("• %s \\(%s\\)\n  Выплата: %s \\(%s\\)\n",
			bold(name),
			code(p.ISIN),
			bold(fmt.Sprintf("%.2f ₽", p.Amount)),
			escapeMarkdown(string(p.Type)),
		))
	}

	sb.WriteString(fmt.Sprintf("\n💰 Суммарно на 1 шт\\.: %s",
		bold(fmt.Sprintf("%.2f ₽", totalAmount)),
	))
	return sb.String()
}

// FormatAnnouncement formats a new placement notice.
func FormatAnnouncement(ann *domain.Announcement) string {
	var sb strings.Builder
	sb.WriteString("🔔 " + bold("Новый анонс размещения облигаций") + "\n\n")
	sb.WriteString(fmt.Sprintf("Выпуск: %s\n", bold(ann.Title)))
	if ann.Issuer != "" {
		sb.WriteString(fmt.Sprintf("Эмитент: %s\n", escapeMarkdown(ann.Issuer)))
	}
	sb.WriteString(fmt.Sprintf("Биржа: %s\n", bold(string(ann.Exchange))))

	if ann.VolumeRUB > 0 {
		if ann.VolumeRUB >= 1e9 {
			sb.WriteString(fmt.Sprintf("Объем: %s\n", bold(fmt.Sprintf("%.1f млрд ₽", ann.VolumeRUB/1e9))))
		} else {
			sb.WriteString(fmt.Sprintf("Объем: %s\n", bold(fmt.Sprintf("%.0f млн ₽", ann.VolumeRUB/1e6))))
		}
	}

	if ann.TargetCoupon != "" {
		sb.WriteString(fmt.Sprintf("Ориентир купона: %s\n", bold(ann.TargetCoupon)))
	}
	if !ann.BookDate.IsZero() {
		sb.WriteString(fmt.Sprintf("Сбор заявок: %s\n", bold(ann.BookDate.Format("02.01.2006"))))
	}

	return sb.String()
}

// escapeMarkdown escapes all Telegram MarkdownV2 reserved characters.
func escapeMarkdown(s string) string {
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

// escapeCode escapes backtick and backslash for inline code spans.
func escapeCode(s string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"`", "\\`",
	)
	return replacer.Replace(s)
}

func pluralizeTrades(n int) string {
	absN := n
	if absN < 0 {
		absN = -absN
	}
	mod100 := absN % 100
	mod10 := absN % 10

	if mod100 >= 11 && mod100 <= 19 {
		return fmt.Sprintf("%d сделок", n)
	}
	if mod10 == 1 {
		return fmt.Sprintf("%d сделка", n)
	}
	if mod10 >= 2 && mod10 <= 4 {
		return fmt.Sprintf("%d сделки", n)
	}
	return fmt.Sprintf("%d сделок", n)
}
