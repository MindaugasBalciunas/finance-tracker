# v1 → v2 feature parity

Every v1 capability, where it lives in v2, and what was deliberately retired. "Covered by" names the test that guards it.

## Kept (reworked)

| v1 | v2 | Covered by |
|---|---|---|
| Dashboard (net worth, cash flow card, AI insight card, AI spend card) | **Home**: true net worth incl. house/car/mortgage with 30d/YTD/12m deltas and sparkline, month vs typical, saved & savings rate, safe to spend, emergency fund, plan pulse, running-hot categories, upcoming recurring charges, stale balances, recent transactions; "Brief me" asks the CFO | `cfo/TestBuildOverview` |
| Transactions page: filters, labels any/all, amount range, bulk label, delete | **Ledger → Transactions**: search, period chips, category/account/tag (any/all)/merchant/amount filters, sort, bulk category/tags/delete | `ledger/TestListFilters`, `api/TestBulkSplitOwed` |
| Add/edit transaction, category & label suggestions, suppressed rule labels | Transaction editor: merchant → category + tags from rules and history, suppressed tags respected; property/car/loans never offered as "paid from" | `ledger/TestEngineApply`, `ledger/TestValidate` |
| Manual transaction moves the account balance (balance note) | Same, for accounts not synced with a bank; update/delete reverse it | `wealth/TestApplyDelta`, `api/TestTransactionLifecycleMovesCashBalance` |
| Receipt scan (AI vision) | Scan button (camera) → prefilled editor; only real money accounts accepted | `ai/TestScanReceipt` |
| Splits, owed money, repayments | Split in the editor (parts, owed-by), "Owed to you" with one-tap repayment | `ledger/TestSplitAndUnsplit`, `api/TestBulkSplitOwed` |
| Categories (flat) + 135 labels | Two-level categories + merchant field + tags (people, properties, trips) | `importv1/TestMapTx` |
| Label rules, preview, reapply, rename/merge/delete labels, label stats | **Settings → Rules** (preview against the ledger, save & apply to history), **Settings → Tags** (rename/merge/remove incl. rules and budgets) | `ledger/TestPreviewAndApplyRuleToHistory`, `ledger/TestRenameTagUpdatesRulesAndBudgets` |
| Balances page, snapshots, built-in + added accounts, allocation, trend | **Wealth → Net worth**: stacked history by group (liquid-only toggle), accounts by group with stale flags, account history, quick "Update balances" (BTC by quantity × price) | `wealth/TestSnapshotCarriesValuesForward`, `api/TestWealthPlanInsightsEndpoints` |
| Assets page (house, car, solar, loan fields) | Property/vehicle accounts with valuation history; **Wealth → Loans**: balance, rate, next interest/principal, payoff, equity/LTV, rate-reset warning, 12-month paid, reconstructed history | `wealth/TestLoans` |
| Stocks: portfolio, live prices, history, analyst targets, trades | **Wealth → Investments**: positions (avg cost, live value, EUR gains), position chart with your average, analyst targets, trades CRUD | `wealth/TestPortfolioAverageCost`, `market/*` |
| Budget engine (fixed/investment/spending, funds, yearly, dated amounts, suggestions, unbudgeted, safe to spend, income modes incl. LT net from gross) | **Plan** — same engine semantics on the new categories; one-tap "Apply from this month" / "Make it a fund" | `plan/TestCompute*`, `plan/TestIncomeBaseModes` |
| Trips (suggest, assign, rename, summarise) | **Plan → Trips**: per-trip totals and per-day cost (prepaid bookings don't stretch the trip), suggestions with one-tap tagging; rename via Tags | `plan/TestTrips` |
| Reports (category charts, cumulative spending, growth, label chart, balance trend) | **Insights**: cash flow (12M/24M/5Y + every year table), savings rate by month, spending breakdown with change vs previous window and drill-down, month-to-date pace vs typical, monthly trends by category or subcategory, recurring costs, FI projection, year review | `insights/*` |
| Review page (monthly highlights) | **Insights → Review (Month/Year)**: verdict, highlights vs last month and the 6-month norm (a salary booked on the 1st–3rd of next month is counted, not reported as a bad month), net-worth move, categories vs norm, spend calendar, top expenses/income, budget brief, owed, checks; Home nudges during the first week of a month | `cfo/TestMonthReview`, `cfo/TestLateSalaryIsExplained` |
| Bank (PSD2): settings, bank list, consent, callback (auto + pasted URL), account mapping, sync one/all, 90-day first window + 7-day overlap, reservations, released holds, staged queue, verdicts, edit, dismiss/restore, merge/unmerge, auto-link certain matches, bank balances summed per account | **Ledger → Bank inbox** + **Settings → Banks** — all of it, v1 external ids preserved so nothing re-imports | `bank/sync_test.go` (staging, idempotency, dismissals, reservations, linking, expiry), `bank/adapter_test.go` (captured narratives) |
| AI chat with tools, images, charts, history, web search, market buzz | **Ask CFO**: tools redesigned for v2 (overview, search, cash flow, breakdown, net worth, plan, recurring, FI, loans, portfolio, quote, buzz, trips, reference, inbox) + confirmed writes | `ai/TestChatToolLoopAndHistory`, `ai/TestReadToolsAnswer` |
| AI memory of decisions | `remember` tool → "Remembered decisions" (editable), injected into every chat and MCP context | `ai/TestWriteToolsAndMemory` |
| AI rule review / labeling / view summaries / forecast / generated insights | Ask CFO with the data tools; "Ask CFO" / "Brief me" buttons on Home, Plan, Wealth and Insights open the chat with the question prefilled | `ai/TestReadToolsAnswer` |
| AI settings (Claude API or gateway, model list, test, on/off), context document, spend + top-ups | **Settings → AI** (key never echoed) | `ai/TestGatewayAuthAndErrors`, `api/TestSecretsNeverEchoed` |
| MCP server (read tools, read-write tools incl. the plan) | `finance-tracker-mcp` v2 (same token model; works behind nginx basic auth via `FT_BASIC_AUTH`) | `api/TestAuthBoundaries` (token scopes) |
| PIN lock, brute-force throttle, passkeys, API tokens (ro/rw) | Same; sessions now persist across restarts (stored hashed) | `auth/*` |
| Cross-origin write guard, body limits | Same | `api/TestCrossSiteWritesBlocked` |
| Full backup / restore (finances.json), CSV export, AI zip, nightly + pre-migration snapshots | Backup (table dump; secrets only on request; restore keeps this instance's secrets), CSV, AI dataset zip, nightly/pre-migration/pre-restore/manual snapshots, v1 backup import | `backup/*`, `api/TestBackupRestoreThroughAPI`, `api/TestImportV1AndExports`, `db/TestBackupRetention` |
| Startup data repairs | Replaced by a one-time verified conversion (`boot`), then numbered migrations | `boot/*`, `importv1/TestConvertFromV1DatabaseCarriesSecretsAndBankState` |

## Gaps found in the second audit and closed (v2.1)

| v1 | v2.1 | Covered by |
|---|---|---|
| Balance-page allocation and "what changed" | **Wealth → Net worth**: allocation bar + per-account movement over the selected range | `wealth/TestMovementAndScenarios` |
| Stock forecast (portfolio at analyst targets) | **Wealth → Investments → Scenarios**: low/mean/high in EUR, coverage share | `wealth/TestMovementAndScenarios` |
| Budget year view | **Plan → The year at a glance**: line × month grid, over-budget flagged | `plan/*` |
| Label chart / label stats | Spending "By tag" card; tag merge suggestions (plural, separator, one-letter typos) in Settings → Tags | `insights/TestTagTotals`, `ledger/TestSuggestTagMerges` |
| AI labeling / rule review | Editor "describe it" AI fill, "Always file X as Y" rule checkbox; **Ledger → Tidy up** queue for vague categories with history and AI proposals | `ai/TestAssistAndTidy`, `api/TestNewInsightAndTidyEndpoints` |
| Data health warnings | `Checks`: stale balances, old valuations, loan rate reset, consent expiry, inbox, vague categories, backup age — on Home and in the month review | `cfo/TestChecks` |
| AI credit top-ups | Settings → AI: top-ups list (add/delete), spend vs credit | `api/TestNewInsightAndTidyEndpoints` |
| Bank setup help | Settings → Banks step-by-step guide | — |
| Export for sync without touching the backup reminder | `?purpose=export` | `api/TestNewInsightAndTidyEndpoints` |

## Added in v2.1.1–2.2

| Area | Change | Covered by |
|---|---|---|
| Settings | Absolute tab links (stacked `/settings/a/b/c` paths fixed, unknown paths recover); same tab bar as other pages; **Export for AI** zip (PROMPT.md, loans.csv, today.json) with phone share sheet | `test/settings-nav.test.tsx`, `api/TestImportV1AndExports` |
| Net worth | "Liquid only" on Home and Wealth, remembered on the server (`/api/prefs`); II/III pillar pensions count as liquid (migration 2, also applied to older backups on restore) | `api/TestPrefsPersist`, `cfo/TestBuildOverview`, `db/TestPensionsBecomeLiquid` |
| Home | Balance-sheet breakdown (assets − debt = net worth) instead of a sideways-scrolling chip row | — |
| Wealth | Structured loan editor (rates, reset, payment, dates, owed balance); property & car valuations folded away in "Update balances" | — |
| Ask CFO | Model picker in the chat header | — |
| UI | Firefox/mobile: sheet footer padding, consistent select chevrons, tab bars without vertical scrollbars, stable page width (`scrollbar-gutter`) | Firefox Playwright sweep |

## Added in v2.3

| Area | Change | Covered by |
|---|---|---|
| Recurring | Add, edit (amount, cadence incl. quarterly, category, next date, note), mark not recurring, restore, reset to detected; same list feeds Home "Coming up" and the AI (migration 3, `recurring_items`, in backups) | `insights/TestRecurringEditsHideAndManual`, `api/TestRecurringCRUD` |
| Charts | Donuts: spending by category; "Where my money is" — stacked balances per account over time + today's split, with a switch per account (remembered); net worth line over the stacked groups | — |
| Periods | 3M / 6M / YTD added to net worth, cash flow, trends, spending and positions; each chart's choice remembered on the server | `api/TestPrefsPersist` |
| Accounts | Settings → Accounts: hide closed/unused accounts (Luminor) from Update balances, pickers and Wealth lists | — |

## Where v2 is better

- One ledger model (income/expense/transfer, two-level categories, merchant, tags) instead of categories + 135 overlapping labels; mortgage principal counts as invested, refunds reduce spending.
- Property, car and loans can no longer be picked as "paid from"; every spend form offers only real money accounts.
- Month review explains itself (verdict + reasons) and doesn't panic over a late salary.
- Sessions survive restarts; read-only vs read-write tokens are route-exact; 0 known npm vulnerabilities, Go 1.26.8.
- Integer-cent money end to end; first start converts and verifies v1 to the cent or refuses to run.
- One Go binary + static web app; unit + integration tests in CI on every push.

## Retired by design

| v1 | Why |
|---|---|
| Swedbank statement CSV import (normal + enrich modes), INVL pension import, Danske salary recovery | History 2010→2026 is complete; new rows arrive over PSD2. A v1 `finances.json` can still be imported. |
| CSV-parity content dedup (byte-identical comments) | Only needed to dedup against CSV-era rows; v2 dedups on bank ids and links hand-entered rows by amount/date/account. |
| Per-boot data repairs (`applyDataCleanups`, `applyCategoryMigrations`, `CanonicalCategory`) | Applied once during conversion; the new category tree makes them unnecessary. |
| Bulk AI label re-index | Superseded by rules with preview + apply-to-history, and the CFO chat's confirmed edits. |
| Partial JSON export, separate balances.csv | The AI dataset zip carries both. |
| Swagger UI | Internal API; the MCP server and tests document it. |

## Data reconciliation (2026-10-04)

Re-checked against production at 18:00 (prod still on v1): 16/16 conversion checks; 6,422 transactions identical to local plus 2 new PSD2 bank fees on prod; every latest balance equal; net worth identical at all 118 month-ends since 2017; 12 integrity checks clean (no orphan accounts/categories, no self-transfers, no spending from property, no duplicate bank ids, no sign errors). Found and fixed in 2.4.2: a v1 JSON export omits zero balances, which made an emptied BTC wallet carry €509 forward for years when importing from the export (the database path was already right).


`importv1.Verify` runs on every first start and in `make convert`. Results on real data:

| Source | Transactions | Balance values | Yearly totals | Latest balance sheet | Settings/secrets/bank state |
|---|---|---|---|---|---|
| Local v1 database | 6,422 / 6,422, 0 altered | 2,416 checked, 0 differ (exact cents) | 17 years preserved | €58,174.36 = €58,174.37 (BTC rounding) | AI key/model, brief, PIN, passkey, bank credentials, 8 connections, 6 bank ids — all carried |
| Production export (finances.json) | 6,424 / 6,424, 0 altered | 2,199 checked, 0 differ | 17 years preserved | €58,174.36 = €58,174.37 | AI settings + brief, 166 bank ids carried (bank/auth secrets are not in the export; the production database itself is converted and verified on the add-on's first start) |
