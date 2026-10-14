//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/provider/bfo"
	"telegram-bot-moex/internal/provider/composite"
	"telegram-bot-moex/internal/provider/moex"
	"telegram-bot-moex/internal/provider/spbe"
	"telegram-bot-moex/internal/service"
	"telegram-bot-moex/internal/store/memory"
	"telegram-bot-moex/internal/telegram"
)

func TestLiveMOEXMarketData(t *testing.T) {
	moexClient := moex.NewClient(nil)
	spbeClient := spbe.NewClient()
	comp := composite.NewProvider(moexClient, spbeClient)
	store := memory.NewStore()
	svc := service.NewBondService(comp, store)

	for _, isin := range []string{"SU26238RMFS4", "RU000A107456"} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		bond, md, payments, err := svc.GetBondDetails(ctx, isin)
		cancel()

		require.NoError(t, err, "failed to get bond details for %s", isin)
		require.NotNil(t, bond)
		require.NotNil(t, md)

		msg := telegram.FormatBondPassport(bond, md, payments)
		fmt.Printf("\n--- Live Formatted Message for %s ---\n%s\n", isin, msg)

		require.NotEmpty(t, bond.IssuerINN, "expected issuer INN for %s", isin)
		require.True(t, md.LastPricePct > 0, "expected last price > 0")
	}
}

func TestLiveBFOBalance(t *testing.T) {
	moexClient := moex.NewClient(nil)
	spbeClient := spbe.NewClient()
	comp := composite.NewProvider(moexClient, spbeClient)
	bfoClient := bfo.NewClient(nil)
	finCache := memory.NewFinancialsCache()
	finSvc := service.NewFinancialsService(comp, bfoClient, finCache)

	// Test 1: Corporate issuer (Gazprom by INN)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	bal, err := finSvc.GetIssuerBalance(ctx, "7736050003")
	cancel()

	require.NoError(t, err, "failed to get balance for Gazprom")
	require.NotNil(t, bal)
	require.Equal(t, "7736050003", bal.INN)
	require.True(t, bal.TotalAssets > 0, "expected total assets > 0")
	require.True(t, bal.ReportYear >= 2023, "expected report year >= 2023")

	msg := telegram.FormatIssuerBalance(bal)
	fmt.Printf("\n--- Live Formatted Balance for Gazprom ---\n%s\n", msg)

	// Test 2: Sovereign issuer (Minfin OFZ)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	sovBal, err2 := finSvc.GetIssuerBalance(ctx2, "SU26238RMFS4")
	cancel2()

	require.NoError(t, err2, "failed to get sovereign balance")
	require.NotNil(t, sovBal)
	require.True(t, sovBal.IsSovereign)
	require.Equal(t, domain.RiskSovereign, sovBal.RiskLevel)

	sovMsg := telegram.FormatIssuerBalance(sovBal)
	fmt.Printf("\n--- Live Formatted Balance for Sovereign OFZ ---\n%s\n", sovMsg)
}
