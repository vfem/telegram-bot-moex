package domain

import "time"

// Exchange represents the trading venue for the bond.
type Exchange string

const (
	ExchangeMOEX Exchange = "MOEX"
	ExchangeSPBE Exchange = "SPBE"
)

// Bond represents a sovereign, municipal, or corporate bond.
type Bond struct {
	ISIN            string    `json:"isin"`
	Ticker          string    `json:"ticker"`
	Name            string    `json:"name"`
	Issuer          string    `json:"issuer"`
	IssuerINN       string    `json:"issuer_inn,omitempty"`
	Exchange        Exchange  `json:"exchange"`
	PrimaryBoard    string    `json:"primary_board,omitempty"`
	NominalValue    float64   `json:"nominal_value"`
	Currency        string    `json:"currency"`
	CouponRatePct   float64   `json:"coupon_rate_pct"`
	CouponsPerYear  int       `json:"coupons_per_year"`
	MaturityDate    time.Time `json:"maturity_date"`
	HasAmortization bool      `json:"has_amortization"`
}

// BondSummary contains basic info for search results.
type BondSummary struct {
	ISIN     string   `json:"isin"`
	Ticker   string   `json:"ticker"`
	Name     string   `json:"name"`
	Exchange Exchange `json:"exchange"`
}
