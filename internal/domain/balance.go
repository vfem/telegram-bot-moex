package domain

import (
	"fmt"
	"math"
	"time"
)

// DebtRiskLevel represents the risk assessment based on debt load.
type DebtRiskLevel string

const (
	RiskLow       DebtRiskLevel = "low"
	RiskModerate  DebtRiskLevel = "moderate"
	RiskHigh      DebtRiskLevel = "high"
	RiskSovereign DebtRiskLevel = "sovereign"
	RiskBank      DebtRiskLevel = "bank"
	RiskUnknown   DebtRiskLevel = "unknown"
)

// Label returns the Russian user-facing representation with emoji for the risk level.
func (r DebtRiskLevel) Label() string {
	switch r {
	case RiskLow:
		return "🟢 Низкая нагрузка"
	case RiskModerate:
		return "🟡 Умеренная нагрузка"
	case RiskHigh:
		return "🔴 Высокая нагрузка"
	case RiskSovereign:
		return "⚪️ Суверенный эмитент"
	case RiskBank:
		return "⚪️ Кредитная организация (ЦБ РФ)"
	default:
		return "н/д"
	}
}

// IssuerBalance encapsulates balance sheet and financial results of an issuer.
type IssuerBalance struct {
	INN             string        `json:"inn"`
	IssuerName      string        `json:"issuer_name"`
	ReportYear      int           `json:"report_year"`
	Standard        string        `json:"standard"` // "РСБУ"
	TotalAssets     float64       `json:"total_assets"`
	Equity          float64       `json:"equity"`
	LongTermDebt    float64       `json:"long_term_debt"`
	ShortTermDebt   float64       `json:"short_term_debt"`
	CashEquivalents float64       `json:"cash_equivalents"`
	TotalDebt       float64       `json:"total_debt"`
	NetDebt         float64       `json:"net_debt"`
	Revenue         float64       `json:"revenue"`
	NetProfit       float64       `json:"net_profit"`
	DebtToEquity    float64       `json:"debt_to_equity"`
	RiskLevel       DebtRiskLevel `json:"risk_level"`
	AuditorName     string        `json:"auditor_name,omitempty"`
	AuditOpinion    string        `json:"audit_opinion,omitempty"`
	Source          string        `json:"source"`
	FetchedAt       time.Time     `json:"fetched_at"`
	IsSovereign     bool          `json:"is_sovereign"`
	IsCreditOrg     bool          `json:"is_credit_org"`
}

// CalculateMetrics computes TotalDebt, NetDebt, DebtToEquity, and RiskLevel.
func (b *IssuerBalance) CalculateMetrics() {
	if b.IsSovereign {
		b.RiskLevel = RiskSovereign
		return
	}
	if b.IsCreditOrg {
		b.RiskLevel = RiskBank
		return
	}

	b.TotalDebt = b.LongTermDebt + b.ShortTermDebt
	b.NetDebt = b.TotalDebt - b.CashEquivalents

	// In negative or zero equity scenarios, debt load is considered high risk
	if b.Equity <= 0 {
		b.DebtToEquity = 999.0
		b.RiskLevel = RiskHigh
		return
	}

	b.DebtToEquity = b.NetDebt / b.Equity

	// Round to two decimal places for stable representation
	b.DebtToEquity = math.Round(b.DebtToEquity*100) / 100

	switch {
	case b.DebtToEquity <= 1.0:
		b.RiskLevel = RiskLow
	case b.DebtToEquity <= 3.0:
		b.RiskLevel = RiskModerate
	default:
		b.RiskLevel = RiskHigh
	}
}

// FormatMoney converts ruble values into readable Russian trillions, billions, millions or thousands string.
func FormatMoney(rubles float64) string {
	abs := math.Abs(rubles)
	if abs >= 1e12 {
		trillions := rubles / 1e12
		return fmt.Sprintf("%.1f трлн ₽", trillions)
	}
	if abs >= 1e9 {
		billions := rubles / 1e9
		return fmt.Sprintf("%.1f млрд ₽", billions)
	}
	if abs >= 1e6 {
		millions := rubles / 1e6
		return fmt.Sprintf("%.1f млн ₽", millions)
	}
	if abs >= 1e3 {
		thousands := rubles / 1e3
		return fmt.Sprintf("%.1f тыс. ₽", thousands)
	}
	return fmt.Sprintf("%.0f ₽", rubles)
}

// IsValidINN checks whether the provided string is a valid Russian 10- or 12-digit INN
// using the official Federal Tax Service (FNS) checksum algorithm.
func IsValidINN(inn string) bool {
	if len(inn) != 10 && len(inn) != 12 {
		return false
	}
	digits := make([]int, len(inn))
	for i, r := range inn {
		if r < '0' || r > '9' {
			return false
		}
		digits[i] = int(r - '0')
	}

	if len(digits) == 10 {
		// 10-digit INN for legal entities
		weights := []int{2, 4, 10, 3, 5, 9, 4, 6, 8}
		sum := 0
		for i := 0; i < 9; i++ {
			sum += digits[i] * weights[i]
		}
		check := (sum % 11) % 10
		return digits[9] == check
	}

	// 12-digit INN for individuals and sole proprietors
	weights11 := []int{7, 2, 4, 10, 3, 5, 9, 4, 6, 8}
	sum11 := 0
	for i := 0; i < 10; i++ {
		sum11 += digits[i] * weights11[i]
	}
	check11 := (sum11 % 11) % 10
	if digits[10] != check11 {
		return false
	}

	weights12 := []int{3, 7, 2, 4, 10, 3, 5, 9, 4, 6, 8}
	sum12 := 0
	for i := 0; i < 11; i++ {
		sum12 += digits[i] * weights12[i]
	}
	check12 := (sum12 % 11) % 10
	return digits[11] == check12
}
