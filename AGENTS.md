# AI Agent Guidelines & Repository Context

Welcome! This file (`AGENTS.md`) defines the operating principles, architectural conventions, and mandatory procedures for AI agents (Antigravity, Gemini, etc.) working on the **MOEX & SPB Bonds Telegram Bot** codebase.

---

## 1. Project Purpose & Architecture

- **Stack:** Go 1.27+, Docker (Alpine multi-stage <20MB), Telegram Bot API.
- **Hosting Target:** Google Cloud Platform (GCP Cloud Run) under the GCP Free Tier (**Target Cost: $0.00 / month**).
- **Core Value:** Providing Russian debt market intelligence (quotes, YTM, payment schedules, balance sheets, material facts, and risk scoring) to Telegram users before taking actions with securities.
- **Data Policy:** 100% free, unauthenticated public data providers:
  - **MOEX ISS API** (`iss.moex.com`): Quotes, specifications, and coupon cash flows.
  - **SPB Exchange**: Public listing CSV ingestion and feeds.
  - **ГИР БО (bo.nalog.ru)**: Official public accounting reports (Form 1 & Form 2) by issuer INN.
  - **e-disclosure.ru / MOEX News**: Public corporate actions and disclosures.

---

## 2. Living Documentation Policy (Mandatory Rule)

> [!IMPORTANT]
> **All agents must maintain and update documentation alongside code changes.**  
> Code changes and documentation updates are considered a single unit of work.

The project maintains structured living documentation in [`docs/`](docs/):

1. **[`docs/ROADMAP.md`](docs/ROADMAP.md)**:
   - High-level initiative overview and user feedback origin.
2. **[`docs/MVP_STAGES.md`](docs/MVP_STAGES.md)**:
   - Phased delivery milestones: **Stage 0** (Sanity Check) ➔ **MVP-1** (Live Market) ➔ **MVP-2** (Balance Sheet) ➔ **MVP-3** (Facts & Ratings) ➔ **MVP-4** (Decision Memo Hub).
3. **[`docs/FEATURES_DECOMPOSITION.md`](docs/FEATURES_DECOMPOSITION.md)**:
   - Central registry and status tracking for all features (`STAGE-00`, `FEAT-01` through `FEAT-05`) and their 30 sub-tasks.
4. **[`docs/features/`](docs/features/)**:
   - Detailed functional and technical specifications for each feature:
     - [`STAGE-00-sanity-check.md`](docs/features/STAGE-00-sanity-check.md)
     - [`FEAT-01-market-data-liquidity.md`](docs/features/FEAT-01-market-data-liquidity.md)
     - [`FEAT-02-balance-sheet.md`](docs/features/FEAT-02-balance-sheet.md)
     - [`FEAT-03-material-facts.md`](docs/features/FEAT-03-material-facts.md)
     - [`FEAT-04-credit-ratings-audit.md`](docs/features/FEAT-04-credit-ratings-audit.md)
     - [`FEAT-05-decision-memo-ux.md`](docs/features/FEAT-05-decision-memo-ux.md)

### Agent Documentation Checklist:
- **Before implementing a task:** Consult the corresponding specification in `docs/features/` and check dependencies.
- **After implementing a task:** 
  - Update checkboxes in [`docs/FEATURES_DECOMPOSITION.md`](docs/FEATURES_DECOMPOSITION.md) and [`docs/MVP_STAGES.md`](docs/MVP_STAGES.md).
  - Document any non-obvious design decisions, API quirks, or data model adjustments in the feature spec.
  - If a command syntax, environment variable, or configuration changes, update [`README.md`](README.md).

---

## 3. Engineering Guidelines & Conventions

### Directory Layout:
```text
├── cmd/bot/                # Entrypoint, HTTP mux, webhooks, and cron handlers
├── features/               # Gherkin BDD specification files (.feature)
├── internal/
│   ├── domain/             # Pure domain entities (Bond, MarketData, IssuerBalance, Payment)
│   ├── provider/           # External API adapters (moex, spbe, bfo, edisclosure, composite)
│   ├── service/            # Application logic (BondService, SubscriptionService, Notifier)
│   ├── store/              # Storage interfaces & in-memory / Firestore implementations
│   └── telegram/           # Webhook parsing, command routing, keyboards, formatters
├── scripts/                # Sanity and deployment scripts (sanity_check.ps1 / .sh)
├── tests/bdd/              # Godog BDD test suite and step definitions
└── docs/                   # Specifications, roadmaps, and task decompositions
```

### Development Rules:
1. **Behavior-Driven Development (BDD):**
   - The test suite uses Cucumber/Godog in `tests/bdd`.
   - Always run tests before and after modifications:
     ```bash
     go test -count=1 ./tests/bdd
     ```
   - All tests must pass 100% of the time.
2. **Domain Isolation:**
   - Domain models in `internal/domain` must not import transport, HTTP, or storage packages.
3. **Caching Strategy:**
   - Respect external services and free quotas by caching:
     - Real-time market data: 2 minutes TTL.
     - Corporate disclosures & facts: 30–60 minutes TTL.
     - Financial statements (Balance Sheet): 30 days TTL.
4. **Sanity Check Pre-flight:**
   - Execute [`scripts/sanity_check.ps1`](scripts/sanity_check.ps1) (Windows) or [`scripts/sanity_check.sh`](scripts/sanity_check.sh) (Linux) to verify build, tests, and live network reachability to MOEX ISS.
