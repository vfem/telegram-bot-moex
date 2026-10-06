package main

import (
	"log"
	"net/http"
	"os"

	"telegram-bot-moex/internal/provider/composite"
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

	log.Printf("Starting MOEX & SPBE Bonds Telegram Bot on port %s...", port)

	// In-memory store (or Firestore on GCP) and market cache
	store := memory.NewStore()
	marketCache := memory.NewMarketDataCache()

	// Unauthenticated exchange providers
	moexClient := moex.NewClient(nil)
	spbeClient := spbe.NewClient()
	compositeProv := composite.NewProvider(moexClient, spbeClient)

	// Business services
	bondSvc := service.NewBondService(compositeProv, store, marketCache)
	subSvc := service.NewSubscriptionService(store)

	var bot *telegram.Bot
	notifier := service.NewNotifierService(compositeProv, store, nil)
	bot = telegram.NewBot(botToken, secretToken, bondSvc, subSvc, notifier)

	// Wire dispatcher back to bot
	notifier = service.NewNotifierService(compositeProv, store, bot)

	// HTTP Routing for Cloud Run
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", bot.WebhookHandler)
	mux.HandleFunc("/cron/daily-digest", bot.CronDailyDigestHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"moex-spbe-bonds-bot"}`))
	})

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	log.Printf("Bot server ready to accept webhooks at :%s/webhook", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}
