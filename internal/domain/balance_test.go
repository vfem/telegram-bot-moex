package domain

import (
	"testing"
)

func TestIssuerBalance_CalculateMetrics(t *testing.T) {
	tests := []struct {
		name       string
		balance    IssuerBalance
		wantTotalD float64
		wantNetD   float64
		wantD2E    float64
		wantRisk   DebtRiskLevel
	}{
		{
			name: "Low debt load (<= 1.0)",
			balance: IssuerBalance{
				LongTermDebt:    20e9,
				ShortTermDebt:   10e9,
				CashEquivalents: 5e9,
				Equity:          30e9,
			},
			wantTotalD: 30e9,
			wantNetD:   25e9,
			wantD2E:    0.83,
			wantRisk:   RiskLow,
		},
		{
			name: "Moderate debt load (1.0 to 3.0)",
			balance: IssuerBalance{
				LongTermDebt:    40e9,
				ShortTermDebt:   18.1e9,
				CashEquivalents: 8.8e9,
				Equity:          38.5e9,
			},
			wantTotalD: 58.1e9,
			wantNetD:   49.3e9,
			wantD2E:    1.28,
			wantRisk:   RiskModerate,
		},
		{
			name: "High debt load (> 3.0)",
			balance: IssuerBalance{
				LongTermDebt:    400e9,
				ShortTermDebt:   150e9,
				CashEquivalents: 20e9,
				Equity:          100e9,
			},
			wantTotalD: 550e9,
			wantNetD:   530e9,
			wantD2E:    5.3,
			wantRisk:   RiskHigh,
		},
		{
			name: "Negative equity scenario (distressed/critical)",
			balance: IssuerBalance{
				LongTermDebt:    10e9,
				ShortTermDebt:   5e9,
				CashEquivalents: 1e9,
				Equity:          -2e9,
			},
			wantTotalD: 15e9,
			wantNetD:   14e9,
			wantD2E:    999.0,
			wantRisk:   RiskHigh,
		},
		{
			name: "Sovereign debt",
			balance: IssuerBalance{
				IsSovereign: true,
			},
			wantRisk: RiskSovereign,
		},
		{
			name: "Credit organization / bank",
			balance: IssuerBalance{
				IsCreditOrg: true,
			},
			wantRisk: RiskBank,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := tt.balance
			b.CalculateMetrics()

			if !b.IsSovereign && !b.IsCreditOrg {
				if b.TotalDebt != tt.wantTotalD {
					t.Errorf("TotalDebt = %v, want %v", b.TotalDebt, tt.wantTotalD)
				}
				if b.NetDebt != tt.wantNetD {
					t.Errorf("NetDebt = %v, want %v", b.NetDebt, tt.wantNetD)
				}
				if b.DebtToEquity != tt.wantD2E {
					t.Errorf("DebtToEquity = %v, want %v", b.DebtToEquity, tt.wantD2E)
				}
			}
			if b.RiskLevel != tt.wantRisk {
				t.Errorf("RiskLevel = %v, want %v", b.RiskLevel, tt.wantRisk)
			}
		})
	}
}

func TestFormatMoney(t *testing.T) {
	tests := []struct {
		rubles float64
		want   string
	}{
		{25.7e12, "25.7 трлн ₽"},
		{-1.5e12, "-1.5 трлн ₽"},
		{114.2e9, "114.2 млрд ₽"},
		{49.3e9, "49.3 млрд ₽"},
		{500e6, "500.0 млн ₽"},
		{4.8e9, "4.8 млрд ₽"},
		{250e3, "250.0 тыс. ₽"},
		{850, "850 ₽"},
		{-15e9, "-15.0 млрд ₽"},
	}

	for _, tt := range tests {
		got := FormatMoney(tt.rubles)
		if got != tt.want {
			t.Errorf("FormatMoney(%v) = %q, want %q", tt.rubles, got, tt.want)
		}
	}
}

func TestDebtRiskLevel_Label(t *testing.T) {
	if RiskLow.Label() != "🟢 Низкая нагрузка" {
		t.Errorf("got %q, want '🟢 Низкая нагрузка'", RiskLow.Label())
	}
	if RiskModerate.Label() != "🟡 Умеренная нагрузка" {
		t.Errorf("got %q, want '🟡 Умеренная нагрузка'", RiskModerate.Label())
	}
	if RiskHigh.Label() != "🔴 Высокая нагрузка" {
		t.Errorf("got %q, want '🔴 Высокая нагрузка'", RiskHigh.Label())
	}
	if RiskSovereign.Label() != "⚪️ Суверенный эмитент" {
		t.Errorf("got %q, want '⚪️ Суверенный эмитент'", RiskSovereign.Label())
	}
	if RiskBank.Label() != "⚪️ Кредитная организация (ЦБ РФ)" {
		t.Errorf("got %q, want '⚪️ Кредитная организация (ЦБ РФ)'", RiskBank.Label())
	}
}

func TestIsValidINN(t *testing.T) {
	tests := []struct {
		inn  string
		want bool
	}{
		{"7736050003", true},     // Gazprom (10 digits)
		{"7710168360", true},     // Minfin (10 digits)
		{"7740000076", true},     // MTS (10 digits)
		{"7702045051", true},     // MTS Bank (10 digits)
		{"7708304947", true},     // Valid 10 digits
		{"1234567890", false},    // Invalid checksum
		{"7736050004", false},    // Wrong control digit
		{"500100732259", true},   // Valid 12 digits (individual)
		{"500100732250", false},  // Invalid 12 digits (wrong checksum)
		{"123", false},           // Too short
		{"1234567890123", false}, // Too long
		{"abcdefghij", false},    // Non-digits
	}

	for _, tt := range tests {
		got := IsValidINN(tt.inn)
		if got != tt.want {
			t.Errorf("IsValidINN(%q) = %v, want %v", tt.inn, got, tt.want)
		}
	}
}
