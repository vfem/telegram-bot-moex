package domain

import "time"

// PaymentType describes the nature of cash flow (coupon, amortization, or principal repayment).
type PaymentType string

const (
	PaymentCoupon       PaymentType = "COUPON"
	PaymentAmortization PaymentType = "AMORTIZATION"
	PaymentMaturity     PaymentType = "MATURITY"
)

// Payment represents a single scheduled cash flow event for a bond.
type Payment struct {
	ISIN       string      `json:"isin"`
	BondName   string      `json:"bond_name"`
	Type       PaymentType `json:"type"`
	Date       time.Time   `json:"date"`
	Amount     float64     `json:"amount"`     // in currency (e.g. RUB) per 1 bond
	AmountPct  float64     `json:"amount_pct"` // % of nominal
	RecordDate time.Time   `json:"record_date"`
}
