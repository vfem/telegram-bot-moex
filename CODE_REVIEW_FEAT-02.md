# Code Review — Uncommitted Changes (FEAT-02: Issuer Balance Sheet)

| | |
|---|---|
| **Date** | 2026-10-14 |
| **Base** | `39244b1` (feat(security): harden cloud run secrets…) |
| **Scope** | 11 modified + 11 new files (~244 lines added to tracked files, plus new packages) |
| **Verdict** | ⚠️ **Request changes.** Fix 2 must-fix issues before committing. Everything else can be a follow-up. |

---

## 1. Summary of the Change

This adds `/balance <ISIN|ИНН|Тикер>` (alias `/bal`). It fetches the issuer's RSBU Form 1 and Form 2 figures from ГИР БО (`bo.nalog.gov.ru`), computes Net Debt and the Net Debt / Equity ratio, assigns a risk level, caches the result for 30 days, and renders it as MarkdownV2.

| Layer | Files |
|---|---|
| Domain | `internal/domain/balance.go` (+ test) |
| Provider | `internal/provider/bfo/client.go` (+ test) |
| Service | `internal/service/financials_service.go` (+ test) |
| Store | `internal/store/store.go`, `internal/store/memory/financials_cache.go` (+ test) |
| Telegram | `internal/telegram/bot.go`, `internal/telegram/formatters.go`, `formatters_balance_test.go` |
| Wiring | `cmd/bot/main.go` |
| Tests | `features/issuer_balance_sheet.feature`, `tests/bdd/balance_steps.go`, `tests/bdd/test_context.go`, `tests/bdd/bdd_test.go`, `tests/integration/live_probe_test.go` |
| Docs | `README.md`, `docs/FEATURES_DECOMPOSITION.md`, `docs/MVP_STAGES.md`, `docs/features/FEAT-02-balance-sheet.md` |

## 2. Verification Performed

| Check | Result |
|---|---|
| `go build ./...` | ✅ Pass |
| `go vet ./...` | ✅ Pass |
| `go test -count=1 ./internal/...` | ✅ Pass (domain, bfo, moex, service, memory, telegram) |
| `go test -count=1 ./tests/bdd` | ✅ Pass |
| `go test -tags integration -run TestLiveBFOBalance ./tests/integration` | ✅ Pass (live data: Gazprom 2025: assets 25 736.3 bn ₽, ratio 0.24; OFZ shown as sovereign) |
| `gofmt -l .` | ❌ Not clean. Changed files: `internal/domain/balance_test.go`, `internal/provider/bfo/client.go`, `internal/telegram/bot.go`, `tests/bdd/test_context.go`. (`moex/client.go` and `spbe/client.go` were already unformatted before this change.) |

The live run confirms that the ×1000 scaling (ОКЕИ 384, thousands of rubles) is correct: Gazprom's standalone RSBU assets come out at about 25.7 trillion ₽.

---

## 3. Findings

Severity: 🔴 must fix · 🟠 should fix · 🟡 nice to have / nit

### 🔴 F1 — Wrong issuer's financials can be shown under the requested INN

[`client.go#L216-L225`](internal/provider/bfo/client.go#L216-L225)

```go
// Fallback to first item if exact match not found
return sr.Content[0].ID, sr.Content[0].ShortName, sr.Content[0].Bfo.IsCb, nil
```

`/advanced-search/organizations/search?query=` is a full-text search. If the INN isn't in ГИР БО (no filings, a foreign SPV, a typo), the search can still return other organizations. The client then takes the first one. [`extractLatestBalance`](internal/provider/bfo/client.go#L305-L307) labels that data with the user's INN, the service caches it for 30 days, and the user sees another company's balance sheet as if it were the issuer's.

For a decision-support tool, wrong numbers are worse than no numbers.

**Fix:** Drop the fallback and return a "not found" error when no item's INN matches exactly. Add a unit test where the search returns a different INN.

---

### 🔴 F2 — `/help` breaks MarkdownV2 (unescaped parentheses)

[`bot.go#L384`](internal/telegram/bot.go#L384)

```go
"• `/balance <ISIN/ИНН>` — финансовый баланс и долговая нагрузка (ФНС ГИР БО)\n" +
```

In MarkdownV2, `(` and `)` outside code spans must be escaped. Telegram will reject the message with 400. [`SendMessage`](internal/telegram/bot.go#L130-L142) then resends it as plain text, so **every** `/start` and `/help` reply shows raw `*` and backticks. Before this change the help text had no unescaped reserved characters.

**Fix:** `… долговая нагрузка \\(ФНС ГИР БО\\)\n`. Consider a unit test that checks static help and usage strings for unescaped MarkdownV2 reserved characters outside code spans.

---

### 🟠 F3 — `typeCorrections` handling ignores `type` and may pick a superseded report

[`client.go#L298`](internal/provider/bfo/client.go#L298), [`client.go#L326-L332`](internal/provider/bfo/client.go#L326-L332)

- The spec notes (§4) say the annual slice is `type = 12`, but the code never checks `tc.Type`. Every correction is treated as an annual statement.
- `year > bestYear` is a strict comparison, so the **first** statement found for a year wins. If ГИР БО lists the original filing before the corrected one (or the `typeCorrections` → direct `balance` → `bfoData` order puts stale data first), the user sees pre-correction figures.

**Fix:** Filter by the annual type. For the same year, prefer the latest correction (by array position, or by a correction date or ID if the API provides one). Add a test with two corrections for the same year.

---

### 🟠 F4 — Sovereign detection is too broad

[`financials_service.go#L82`](internal/service/financials_service.go#L82)

```go
if bond.IssuerINN == sovereignINN || strings.Contains(strings.ToLower(bond.Issuer), "минфин") {
```

The INN check alone is exact and enough. The substring check can also match regional or municipal finance ministries ("Минфин Республики …"). Those issuers would get a reply saying the bonds are "обеспечены суверенным бюджетом Российской Федерации", which is factually wrong for sub-federal debt.

**Fix:** Rely on `IssuerINN == "7710168360"` only. If you need a name-based fallback, require an exact federal name match.

Related: when `GetBond` fails, the [search fallback](internal/service/financials_service.go#L70-L78) picks `results[0]`. For an ambiguous ticker that may be a different bond or issuer. Consider requiring an exact ISIN or ticker match in the results.

---

### 🟠 F5 — No negative caching or throttling toward ФНС

[`financials_service.go#L117-L139`](internal/service/financials_service.go#L117-L139)

Only successful results are cached. Any 10-digit number passes `innRegex` and triggers two upstream calls (search + statements). Repeated or scripted `/balance <random INN>` requests will hit `bo.nalog.gov.ru` without limit and could get the shared Cloud Run egress IP throttled or blocked. That would conflict with the project's "respect external services and free quotas" rule. Concurrent requests for the same INN also each go upstream.

**Suggestions:**
- Cache "not found" results for a short time (e.g. 1–6 h).
- Wrap upstream fetches in `golang.org/x/sync/singleflight`.
- Optionally validate the INN checksum before calling upstream. This rejects about 90% of random 10-digit inputs for free.

---

### 🟠 F6 — Living documentation doesn't match the code (AGENTS.md §2 is mandatory)

| Doc says | Code does |
|---|---|
| FEAT-02 §4: validation regex `^[A-Za-z0-9]{9,12}$` | Three regexes: `innRegex`, `isinRegex`, and a broad `tickerRegex` `^[A-Z0-9.\-_]{2,20}$` ([L19-L21](internal/service/financials_service.go#L19-L21)) |
| FEAT-02 §4 and TASK-02-04: High risk when `Equity < 0` | `Equity <= 0` ([balance.go#L61](internal/domain/balance.go#L61)) |
| MVP_STAGES.md MVP-2: 🟡 1.0–2.5 / 🔴 > 2.5 | 🟡 ≤ 3.0 / 🔴 > 3.0 (matches FEAT-02, contradicts MVP_STAGES) |
| FEAT-02 §4: annual slice `type = 12` is used | `type` is ignored (see F3) |
| FEAT-02 §2: endpoint `bo.nalog.ru/nws/api/v1/organizations/{INN}/financial-statements` | `bo.nalog.gov.ru/advanced-search/…` + `/nbo/organizations/{id}/bfo/`. §2 was never updated. |
| TASK-02-05 (marked ✅): "и Firestore-адаптер для персистентности" | Only an in-memory cache exists |
| FEAT-02 §4: "Anti-SSRF … блокирует" | Only **redirect** targets are checked, by literal hostname or IP. DNS is not resolved, so a public hostname pointing to a private IP isn't blocked. The risk is low (the base URL is a constant), but the wording overstates the protection. |
| MVP-2 scope: Form 2 line `2330` (interest payable) | Not parsed. Fine if descoped, but say so. |

**Fix:** Align the docs, or the code, and state that the Firestore adapter and line `2330` are deferred.

---

### 🟡 F7 — Minor code-quality items

1. **`gofmt`**: run `gofmt -w` on the four changed files listed in §2.
2. **`/bal` prefix match** ([bot.go#L217](internal/telegram/bot.go#L217)): `HasPrefix(text, "/bal")` also catches `/ballast` and `/bal123`, and the `"/balance"` check is redundant. Match on the command token instead: `cmd := strings.SplitN(parts[0], "@", 2)[0]; cmd == "/balance" || cmd == "/bal"`. This also handles `/balance@BotName` in group chats.
3. **Hidden dependency in `NewBot`** ([bot.go#L59](internal/telegram/bot.go#L59)): the variadic `finSvc ...*service.FinancialsService` makes a real dependency optional and easy to forget. `SetFinancialsService` is unused. Prefer an explicit parameter, or the functional options pattern, and update the 5 call sites.
4. **Test seams in the production API**: `SetMessageSender` and `HandleIncomingMessage` are exported only for BDD. That's acceptable, but document them as test-only, or move them behind an interface (e.g. a `Sender` field).
5. **Interface location** ([financials_service.go#L13](internal/service/financials_service.go#L13)): the service imports the concrete `provider/bfo` package just for `FinancialsProvider`. For consistency with `provider.BondProvider`, define it in `internal/provider/provider.go`.
6. **Duplicated structs** ([client.go#L94-L136](internal/provider/bfo/client.go#L94-L136)): `bfoResponseItem` repeats every field of `bfoStatementData`. Embed `bfoStatementData` instead.
7. **Cache eviction race** ([financials_cache.go#L48-L54](internal/store/memory/financials_cache.go#L48-L54)): between `RUnlock` and `Lock`, another goroutine can `Set` a fresh entry, and the code then deletes it. Re-check `expiresAt` under the write lock before deleting.
8. **Aliasing**: the service caches the same pointer it returns ([L135-L139](internal/service/financials_service.go#L135-L139)). The cache copies on read but not on write, so if a future caller mutates the result, it mutates the cache. Store a copy in `Set`.
9. **Presentation in the domain** ([balance.go#L13-L18](internal/domain/balance.go#L13-L18)): `DebtRiskLevel` values are Russian labels with emoji. Prefer stable codes (`low`, `moderate`, …) and map them to labels in `internal/telegram`.
10. **`FormatMoney`** ([balance.go#L83-L103](internal/domain/balance.go#L83-L103)): add a trillion (`трлн`) tier, since live Gazprom output reads `25736.3 млрд ₽`. The `sign` variable is used only in the `< 1e3` branch.
11. **Negative net debt**: a cash-rich issuer gets a negative ratio (e.g. `-0.25`, 🟢). That's correct but confusing. Consider showing "чистая денежная позиция" instead of the ratio.
12. **Error UX** ([bot.go#L234-L235](internal/telegram/bot.go#L234-L235)): "issuer not found", "bond not found", and "ФНС unavailable" all produce the same message. Sentinel errors such as `ErrIssuerNotFound` and `ErrUpstreamUnavailable` would allow clearer replies.
13. **Usage text vs. validation**: the usage message advertises `Тикер`, but the help line only says `<ISIN/ИНН>`. Pick one.

---

## 4. Test Coverage Gaps

| Missing test | Why it matters |
|---|---|
| BFO search returns a non-matching INN | Would have caught F1 |
| Two corrections for the same year, and a non-annual `type` | Covers F3 |
| Non-200 response and >4 MB body from BFO | Checks the error path and `LimitReader` truncation |
| BDD: credit organization (`isCb`) via `/balance` | Only covered at the client and formatter unit level |
| BDD: provider error → user message | The generic failure path is untested end to end |
| BDD: `/bal` alias | Alias is documented but untested |
| MarkdownV2 validity of static texts (help, usage) | Would have caught F2 |
| Regional "Минфин …" issuer is **not** sovereign | Covers F4 |

---

## 5. What's Good

- Clean layering (domain → provider → service → telegram) that follows AGENTS.md. The domain package has no transport imports.
- The 30-day TTL matches the caching policy. The cache is bounded (5 000 entries) and copies on read.
- Defensive HTTP client: timeout, `io.LimitReader` (4 MB), redirect limit, https-only redirects.
- Input whitelisting before any upstream call, and internal errors aren't leaked to users (they're logged with `%q`).
- Banks (`isCb`) and the sovereign case are explicit, so the bot doesn't show meaningless RSBU numbers for them.
- The integration test is behind the `integration` build tag, so CI stays offline and deterministic.
- Living docs, README, and the BDD feature file were updated in the same change.

---

## 6. Recommended Action Plan

1. **Before commit:** F1 (remove the wrong-issuer fallback), F2 (escape the help text), and run `gofmt -w`.
2. **Same PR, if possible:** F3, F4, and the F6 doc alignment (required by the Living Documentation policy).
3. **Follow-up:** F5 (negative cache, singleflight, INN checksum), the F7 items, and the §4 test gaps.
