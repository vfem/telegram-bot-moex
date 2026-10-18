package main

import (
	"log"
	"net/http"
	"os"

	"telegram-bot-moex/internal/provider/bfo"
	"telegram-bot-moex/internal/provider/composite"
	"telegram-bot-moex/internal/provider/edisclosure"
	"telegram-bot-moex/internal/provider/moex"
	"telegram-bot-moex/internal/provider/spbe"
	"telegram-bot-moex/internal/service"
	"telegram-bot-moex/internal/store/memory"
	"telegram-bot-moex/internal/telegram"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	secretToken := os.Getenv("TELEGRAM_SECRET_TOKEN")

	// Fail closed: if bot token is set (production mode), secret token must be configured
	if botToken != "" && secretToken == "" {
		log.Fatal("Fatal configuration error: TELEGRAM_SECRET_TOKEN must be configured when TELEGRAM_BOT_TOKEN is set")
	}

	log.Printf("Starting MOEX & SPBE Bonds Telegram Bot on port %s...", port)

	// In-memory store (or Firestore on GCP) and caches
	store := memory.NewStore()
	marketCache := memory.NewMarketDataCache()
	finCache := memory.NewFinancialsCache()
	factsCache := memory.NewFactsCache()

	// Unauthenticated exchange providers, BFO client, and disclosure client
	moexClient := moex.NewClient(nil)
	spbeClient := spbe.NewClient()
	compositeProv := composite.NewProvider(moexClient, spbeClient)
	bfoClient := bfo.NewClient(nil)
	edisclosureClient := edisclosure.NewClient(nil)

	// Business services
	bondSvc := service.NewBondService(compositeProv, store, marketCache)
	finSvc := service.NewFinancialsService(compositeProv, bfoClient, finCache)
	factsSvc := service.NewFactsService(compositeProv, edisclosureClient, factsCache)
	subSvc := service.NewSubscriptionService(store)

	notifier := service.NewNotifierService(compositeProv, store, nil)
	notifier.SetFactsProvider(edisclosureClient)
	bot := telegram.NewBot(botToken, secretToken, bondSvc, subSvc, notifier, finSvc, factsSvc)

	// Wire dispatcher and facts provider on the existing notifier instance (F2)
	notifier.SetDispatcher(bot)

	// HTTP Routing for Cloud Run
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", bot.WebhookHandler)
	mux.HandleFunc("/cron/daily-digest", bot.CronDailyDigestHandler)
	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"moex-spbe-bonds-bot"}`))
	}
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/ping", healthHandler)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	log.Printf("Bot server ready to accept webhooks at :%s/webhook", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}
