package bdd

import (
	"context"
	"fmt"
	"time"

	"telegram-bot-moex/internal/domain"
	"telegram-bot-moex/internal/provider/bfo"
	"telegram-bot-moex/internal/provider/composite"
	"telegram-bot-moex/internal/provider/moex"
	"telegram-bot-moex/internal/provider/spbe"
	"telegram-bot-moex/internal/service"
	"telegram-bot-moex/internal/store/memory"
	"telegram-bot-moex/internal/telegram"
)

// TestContext holds scenario execution state.
type TestContext struct {
	ctx         context.Context
	memStore    *memory.Store
	moexClient  *moex.Client
	spbeClient  *spbe.Client
	composite   *composite.Provider
	bondService *service.BondService
	subService  *service.SubscriptionService
	notifier    *service.NotifierService
	marketCache *memory.MarketDataCache

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
	mockBfoProv       *mockBddBfoProvider
	lastAnnouncement  domain.Announcement
	returnedPayments  []domain.Payment
	bot               *telegram.Bot
	lastResponseCode  int
	lastResponseBody  string
	finService        *service.FinancialsService
	finCache          *memory.FinancialsCache
}

type mockBddBfoProvider struct {
	reports map[string]*domain.IssuerBalance
}

func (m *mockBddBfoProvider) GetIssuerBalance(ctx context.Context, inn string) (*domain.IssuerBalance, error) {
	if rep, ok := m.reports[inn]; ok {
		return rep, nil
	}
	return nil, fmt.Errorf("no reports found for INN %s: %w", inn, bfo.ErrOrganizationNotFound)
}

func newTestContext() *TestContext {
	memStore := memory.NewStore()
	spbeClient := spbe.NewClient()
	moexClient := moex.NewClient(nil)
	comp := composite.NewProvider(moexClient, spbeClient)
	mockProv := newMockBondProvider()
	marketCache := memory.NewMarketDataCache()
	finCache := memory.NewFinancialsCache()
	mockBfo := &mockBddBfoProvider{reports: make(map[string]*domain.IssuerBalance)}

	bondSvc := service.NewBondService(comp, memStore, marketCache)
	finSvc := service.NewFinancialsService(mockProv, mockBfo, finCache)
	subSvc := service.NewSubscriptionService(memStore)
	notifier := service.NewNotifierService(comp, memStore, nil)

	bot := telegram.NewBot("", "", bondSvc, subSvc, notifier, finSvc)

	return &TestContext{
		ctx:          context.Background(),
		memStore:     memStore,
		moexClient:   moexClient,
		spbeClient:   spbeClient,
		composite:    comp,
		bondService:  bondSvc,
		finService:   finSvc,
		finCache:     finCache,
		mockBfoProv:  mockBfo,
		subService:   subSvc,
		notifier:     notifier,
		marketCache:  marketCache,
		mockBondProv: mockProv,
		bot:          bot,
		mockPayments: make(map[string][]domain.Payment),
		mockBonds:    make(map[string]domain.Bond),
	}
}
