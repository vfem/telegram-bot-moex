# Code Review — FEAT-03: Material Facts & Corporate Events Monitoring (Final Verification)

| | |
|---|---|
| **Date** | 2026-10-18 (Round 3 — Final Verification) |
| **Base** | `bd04c67` (`docs: align roadmap, MVP stages, deployment guide, and feature specs with MVP-2 state`) |
| **Scope** | 20 modified + 13 untracked files (`internal/domain/fact.go`, `internal/provider/edisclosure`, `internal/service/common.go`, `internal/service/facts_service.go`, `internal/service/notifier_service.go`, `internal/store/memory/facts_cache.go`, `internal/telegram`, `features/material_facts.feature`, `tests/integration/live_probe_test.go`) |
| **Verdict** | ✅ **Approved — All 12 initial findings (F1–F12) and all 3 follow-up items (R1–R3) are fully resolved and verified.** |

---

## 1. Verification Performed on Updated Code

| Check | Result |
|---|---|
| `go build ./...` | ✅ Pass |
| `go vet ./...` | ✅ Pass |
| `gofmt -l .` | ✅ Clean (0 unformatted files) |
| `go test -count=1 ./internal/...` | ✅ Pass (`domain`, `bfo`, `edisclosure`, `moex`, `service`, `memory`, `telegram`) |
| `go test -count=1 ./tests/bdd` | ✅ Pass (40 scenarios) |
| `go test -v -tags integration ./tests/integration` | ✅ Pass (`TestLiveMOEXMarketData`, `TestLiveBFOBalance`, `TestLiveMaterialFacts`) |

---

## 2. Status of Initial Review Findings (F1–F12)

| ID | Initial Finding | Status | Resolution Summary |
|---|---|---|---|
| **F1** | Default `e-disclosure.ru/api/events` endpoint returns HTTP 403 / does not exist | ✅ **Resolved** | `edisclosure.NewClient` now defaults to `https://iss.moex.com/iss` (querying `securities.json`, `securities/{SECID}.json`, `bondization.json`, and filtered `sitenews.json`), while preserving RSS 2.0 XML and JSON array support. Verified via `TestLiveMaterialFacts`. |
| **F2** | Proactive alerts (`TASK-03-05`) never invoked in prod; `bot.notifier` wired with `nil` dispatcher in `main.go` | ✅ **Resolved** | `cmd/bot/main.go` wires `notifier.SetDispatcher(bot)` and `notifier.SetFactsProvider(edisclosureClient)` on the active instance, `RunDailyDigest` invokes `CheckMonitoredFacts` using `GetUsersWithMonitoredBonds`, deduplicates per issuer INN, and filters cross-series ISINs. |
| **F3** | False-positive `CRITICAL`/`WARNING` classifications from substring collisions (`спор`, `иск`, `требован`, `предъявлени`, `приостановлени`, `нкр`) | ✅ **Resolved** | Added Unicode word-boundary helpers `containsWord` and `containsStem` in [`internal/domain/fact.go`](internal/domain/fact.go) and 7 regression test cases in [`internal/domain/fact_test.go`](internal/domain/fact_test.go). |
| **F4** | `edisclosure.ErrNoFactsFound` not mapped to `service.ErrNoMaterialFacts`; no negative caching | ✅ **Resolved** | [`FactsService`](internal/service/facts_service.go) maps `edisclosure.ErrNoFactsFound` and empty slices to `ErrNoMaterialFacts` and negatively caches them for 15 minutes (`SetNotFound` / `IsNotFound` in [`FactsCache`](internal/store/memory/facts_cache.go)). |
| **F5** | Unescaped MarkdownV2 in `NotifyMaterialFact` causes HTTP 400 fallback on every alert | ✅ **Resolved** | [`NotifierService`](internal/service/notifier_service.go) now escapes MarkdownV2 characters (`escapeMdV2`, `escapeCodeSpan`) and sanitizes `fact.URL` with `edisclosure.SanitizeURL`. |
| **F6** | Data race and unbounded growth in `NotifierService.sentFactAlerts` and `Outbox` | ✅ **Resolved** | Protected by `sync.Mutex`, capped at `maxSentFactAlerts = 10000` and `maxOutboxMessages = 1000`, and `sentFactAlerts[dedupKey] = true` is only recorded after `disp.SendMessage` succeeds. |
| **F7** | MOEX `sitenews` drops URLs, formats large numeric IDs in scientific notation, and lacks issuer filtering | ✅ **Resolved** | `cellString` formats `float64` via `strconv.FormatFloat(v, 'f', -1, 64)`, constructs `https://www.moex.com/n<id>`, and filters `sitenews` using `buildFilterKeywords`. |
| **F8** | Sovereign ОФЗ bonds routed to corporate disclosure lookup | ✅ **Resolved** | [`FactsService`](internal/service/facts_service.go) and [`FormatMaterialFacts`](internal/telegram/formatters.go) explicitly detect sovereign ОФЗ bonds (`7710168360` / Минфин РФ) and return a dedicated sovereign explanation. |
| **F9** | 9 modified Go files not `gofmt`-formatted | ✅ **Resolved** | `gofmt -l .` is clean. |
| **F10** | Living documentation discrepancies (`FEAT-03-material-facts.md`, `FormatMaterialFacts`, `/help`) | ✅ **Resolved** | Spec, formatter output (`(ИНН ...)` and `HH:MM`), and `/help` (`(алиас /f)`) are aligned. |
| **F11** | Variadic `NewBot`, duplicated `singleflight`/bond lookup, first-caller context cancellation, MSK timezone, HTML tag stripping, `Store` shallow copies | ✅ **Resolved** | Extracted generic `singleflightGroup[T]` and `resolveBondByIdentifier` into [`internal/service/common.go`](internal/service/common.go), added `context.WithoutCancel`, `MSK (UTC+3)` parsing, HTML tag stripping, and `cloneUser`. |
| **F12** | Missing unit, BDD, and live integration tests | ✅ **Resolved** | Added [`notifier_service_test.go`](internal/service/notifier_service_test.go), [`formatters_facts_test.go`](internal/telegram/formatters_facts_test.go), `TestLiveMaterialFacts` in [`live_probe_test.go`](tests/integration/live_probe_test.go), and 2 new BDD scenarios in [`material_facts.feature`](features/material_facts.feature). |

---

## 3. Status of Follow-Up Items (R1–R3)

| ID | Follow-Up Item | Status | Resolution Summary |
|---|---|---|---|
| **R1** | `extractBondizationFacts` synthesizes false-positive `[⚠️ Оферта]` facts for floating-rate bonds (`value: null`), stamps synthetic facts with `time.Now()`, and lacks a recency filter on paid coupons | ✅ **Resolved** | Removed the false-positive `valStr == ""` floating-coupon reset heuristic in [`internal/provider/edisclosure/client.go`](internal/provider/edisclosure/client.go#L455-L490), added the 1-year recency cutoff `cTime.Before(now.AddDate(-1, 0, 0))` to `paidCoupons` ([`L460`](internal/provider/edisclosure/client.go#L460)), and switched `extractDefaultFacts` ([`L385-L410`](internal/provider/edisclosure/client.go#L385-L410)) to a date-truncated `today` timestamp (`00:00:00`). Live output of `TestLiveMaterialFacts` for `RU000A107456` now cleanly returns only recent 2026 coupon facts. |
| **R2** | `CheckMonitoredFacts` only inspects bonds of users with `/sub pay`, makes duplicate `GetRecentFacts` calls per INN, and cross-notifies across different bond series of the same issuer | ✅ **Resolved** | Added `GetUsersWithMonitoredBonds(ctx)` to [`store.Store`](internal/store/store.go#L16) and [`memory.Store`](internal/store/memory/memory.go#L94-L105); updated [`RunDailyDigest`](internal/service/notifier_service.go#L140-L144) and [`CheckMonitoredFacts`](internal/service/notifier_service.go#L158-L216) to inspect all users with monitored bonds, cache `GetRecentFacts` per `inn` (`factsByINN` / `failedINN`), and skip facts where `f.ISIN != ""` and `!strings.EqualFold(f.ISIN, bond.ISIN)` in both `CheckMonitoredFacts` and `NotifyMaterialFact`. |
| **R3** | Minor edge cases in `extractIssuerSecurities`, `cleanText`, and `Store.GetUser` | ✅ **Resolved** | [`extractIssuerSecurities`](internal/provider/edisclosure/client.go#L284-L295) requires `emitentINN == targetINN` when `emitent_inn` column is present; [`cleanText`](internal/provider/edisclosure/client.go#L736-L740) runs `html.UnescapeString` before `htmlTagRegex.ReplaceAllString`; [`memory.Store.GetUser`](internal/store/memory/memory.go#L57) and [`SaveUser`](internal/store/memory/memory.go#L64) clone `*domain.User` via `cloneUser`, and [`Bot.handleGroup`](internal/telegram/bot.go#L396-L412) persists group modifications via `b.subService.AddBondToGroup`. |
