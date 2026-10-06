# MOEX & SPB Exchange Bonds Telegram Bot

A lightweight, serverless Telegram bot in **Go** that delivers Russian debt market data (Moscow Exchange and Saint Petersburg Exchange) on demand and via automated scheduled digests.

Designed for low-load personal or group use on **Google Cloud Platform (GCP)** with an operational cost target of **$0.00 / month** (well within GCP Free Tier).

---

## Features

- **Bond Profiles & Passports:** Instant lookup by ISIN, ticker, or Russian issuer name (`/bond SU26238RMFS4`, `/bond ЕвроТранс`).
- **Payment Calendars:**
  - `/today` — view coupon and amortization payments scheduled for today across watched bonds.
  - `/calendar 2026-10-15` — inspect all scheduled cash flows for any given date.
- **Custom Groups & Portfolios:**
  - Create and manage bond collections: `/group create ОФЗ`, `/group add ОФЗ SU26238RMFS4`, `/group show ОФЗ`.
- **Subscriptions & Push Digests:**
  - `/sub pay` — daily morning digest (09:00 MSK) of coupon payments for monitored bonds.
  - `/sub new` — automated notifications for new primary bond placement announcements (сбор заявок).
  - `/sub add <ISIN>` — add a bond to your monitored list.
- **100% Unauthenticated Public Providers:**
  - **MOEX ISS API:** Official unauthenticated JSON REST endpoints (`iss.moex.com`) for bond specifications, quotes, and bondization cash flows.
  - **SPB Exchange:** Public listing CSV ingestion and public disclosure feed tracking.

---

## BDD Test Suite (Gherkin / Godog)

The project follows Behavior-Driven Development (BDD). Scenarios are defined in `.feature` files:

- `features/bonds_on_demand.feature`: On-demand bond queries, searches, and calendar payments.
- `features/bonds_subscription.feature`: User subscriptions, scheduled daily digests, and placement alerts.
- `features/dual_exchange_providers.feature`: Dual-exchange data ingestion and resolution.

### Running BDD Tests

```bash
go test -v ./tests/bdd
```

---

## Project Structure

```text
├── cmd/
│   └── bot/
│       └── main.go                 # Webhook and cron HTTP server
├── features/                       # Gherkin BDD specification files
│   ├── bonds_on_demand.feature
│   ├── bonds_subscription.feature
│   └── dual_exchange_providers.feature
├── internal/
│   ├── domain/                     # Core domain entities (Bond, Payment, User, Group)
│   ├── provider/                   # Unauthenticated market data providers
│   │   ├── moex/                   # MOEX ISS REST client
│   │   ├── spbe/                   # SPB Exchange public feeds
│   │   └── composite/              # Multi-exchange federated provider
│   ├── service/                    # Application services (bonds, subscriptions, notifier)
│   ├── store/                      # Storage abstraction & in-memory / Firestore store
│   └── telegram/                   # Webhook parser, command handlers, and formatters
├── tests/
│   └── bdd/                        # Godog BDD test suite & step definitions
├── Dockerfile                      # Ultra-light Alpine multi-stage build (<20 MB)
└── go.mod
```

---

## Local Development

1. Copy `.env.example` to `.env` and fill in your `TELEGRAM_BOT_TOKEN`.
2. Run locally:
   ```bash
   go run ./cmd/bot
   ```
3. Health check:
   ```bash
   curl http://localhost:8080/healthz
   ```

---

## GCP Cloud Run Deployment ($0.00 / month Free Tier)

The bot is engineered to run 100% within the **GCP Free Tier** (2M requests, 360k vCPU-s, 180k GiB-s, 1GB egress per month).

See the full [**GCP Cloud Run Deployment Guide & Free Tier Audit**](docs/DEPLOYMENT.md) for detailed mathematical verification, guardrails, and operational runbook.

### Automated Deployment

```powershell
# Windows PowerShell
.\scripts\deploy_cloud_run.ps1 -Region "europe-west1"
```

```bash
# Linux / macOS
./scripts/deploy_cloud_run.sh "<PROJECT_ID>" "europe-west1"
```

### Manual Deployment

1. Build and deploy container to Cloud Run with Free Tier guardrails:
   ```bash
   gcloud run deploy moex-bonds-bot \
     --source . \
     --region europe-west1 \
     --platform managed \
     --allow-unauthenticated \
     --memory 128Mi \
     --cpu 1 \
     --min-instances 0 \
     --max-instances 2 \
     --concurrency 80 \
     --timeout 15s \
     --set-env-vars TELEGRAM_BOT_TOKEN="your_token",TELEGRAM_SECRET_TOKEN="your_secret"
   ```

2. Configure Telegram Webhook:
   ```bash
   curl -s -X POST "https://api.telegram.org/bot<YOUR_TOKEN>/setWebhook" \
     -H "Content-Type: application/json" \
     -d '{"url":"https://<YOUR_CLOUD_RUN_URL>/webhook","secret_token":"your_secret","drop_pending_updates":true}'
   ```

3. Setup Cloud Scheduler for Daily Digests (Free Tier: 1 of 3 free jobs):
   ```bash
   gcloud scheduler jobs create http moex-daily-digest \
     --location europe-west1 \
     --schedule="0 6 * * *" \
     --uri="https://<YOUR_CLOUD_RUN_URL>/cron/daily-digest" \
     --http-method=POST
   ```

---

## Product Roadmap & Documentation

- **Initiative Roadmap:** [`docs/ROADMAP.md`](docs/ROADMAP.md)
- **MVP Delivery Stages:** [`docs/MVP_STAGES.md`](docs/MVP_STAGES.md)
- **Features Decomposition & Task Registry:** [`docs/FEATURES_DECOMPOSITION.md`](docs/FEATURES_DECOMPOSITION.md)
- **Deployment Guide & Free Tier Audit:** [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md)
- **AI Agent Guidelines & Maintenance Policy:** [`AGENTS.md`](AGENTS.md)


