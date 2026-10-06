//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
