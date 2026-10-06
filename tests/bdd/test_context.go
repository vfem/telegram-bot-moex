package bdd

import (
	"context"
	"time"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/provider/composite"
	"telegram-bot-moex/internal/provider/moex"
	"telegram-bot-moex/internal/provider/spbe"
	"telegram-bot-moex/internal/service"
	"telegram-bot-moex/internal/store/memory"
)

// TestContext holds scenario execution state.
type TestContext struct {
	ctx          context.Context
	memStore     *memory.Store
	moexClient   *moex.Client
	spbeClient   *spbe.Client
	composite    *composite.Provider
	bondService  *service.BondService
	subService   *service.SubscriptionService
	notifier     *service.NotifierService
	marketCache  *memory.MarketDataCache

	// Scenario state
	currentUser       *domain.User
	currentBond       *domain.Bond
	currentMarketData *domain.MarketData
	currentPayments   []domain.Payment
	formattedMessage  string
	lastError         error
	todayDate         time.Time
	resolvedISIN      string
	resolvedExchange  domain.Exchange
	mockPayments      map[string][]domain.Payment
	mockBonds         map[string]domain.Bond
	mockBondProv      *mockBondProvider
	lastAnnouncement  domain.Announcement
	returnedPayments  []domain.Payment
}

func newTestContext() *TestContext {
	memStore := memory.NewStore()
	spbeClient := spbe.NewClient()
	moexClient := moex.NewClient(nil)
	comp := composite.NewProvider(moexClient, spbeClient)
	mockProv := newMockBondProvider()
	marketCache := memory.NewMarketDataCache()

	bondSvc := service.NewBondService(comp, memStore, marketCache)
	subSvc := service.NewSubscriptionService(memStore)
	notifier := service.NewNotifierService(comp, memStore, nil)

	return &TestContext{
		ctx:          context.Background(),
		memStore:     memStore,
		moexClient:   moexClient,
		spbeClient:   spbeClient,
		composite:    comp,
		bondService:  bondSvc,
		subService:   subSvc,
		notifier:     notifier,
		marketCache:  marketCache,
		mockBondProv: mockProv,
		mockPayments: make(map[string][]domain.Payment),
		mockBonds:    make(map[string]domain.Bond),
	}
}
