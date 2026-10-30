# Finance Tracker

A personal CFO for one household: true net worth (cash, investments, pension, crypto, house, car and mortgage), cash flow and savings rate, a monthly plan with sinking funds, bank sync over PSD2, and an AI adviser that answers from the live data.

Runs as a Home Assistant add-on: one container with nginx (basic auth, TLS headers) in front of a Go server and SQLite.

## Layout

| Path | What |
|---|---|
| `finance_tracker/server` | Go server (stdlib HTTP, SQLite via modernc, no CGO) |
| `finance_tracker/web` | React 18 + TypeScript + Vite + Tailwind 4 + TanStack Query + Recharts |
| `finance_tracker/{Dockerfile,run.sh,nginx.conf,config.yaml}` | Add-on packaging |

Server packages (`finance_tracker/server/internal`):

| Package | Responsibility |
|---|---|
| `ledger` | accounts, two-level categories, transactions (income · expense · transfer), tags, merchants, rules engine |
| `wealth` | balances (one value per account per day), net worth over time, investments, loans |
| `plan` | budget lines (fixed · spending · saving), sinking funds, yearly lines, trips |
| `insights` | cash flow, savings rate, spending breakdown, recurring costs, anomalies, FI projection |
| `cfo` | cross-domain views: Home overview, plan report |
| `bank` | Enable Banking (PSD2) client, adapter, inbox, reservations, bank balances |
| `ai` | Claude Messages API client (Claude API or a gateway), CFO chat with tools, receipt scan, memory |
| `auth` | PIN, passkeys (WebAuthn), persistent sessions, read-only / read-write API tokens |
| `backup` | full JSON backup / restore (table dump, secrets opt-in) |
| `importv1` | converts a v1 database or `finances.json`, and verifies nothing was lost |
| `boot` | first start: convert the v1 database next to the v2 one, verify, refuse to start on any mismatch |

## Data model in one paragraph

Money is integer cents in EUR. Every place money can sit is an **account** (checking, savings, cash, brokerage, pension, crypto, property, vehicle, loan); net worth is the sum of each account's latest **balance** (loans negative). A **transaction** is income, expense or transfer; transfers into investments, pension, debt principal or assets build wealth, `transfer.internal` only moves cash. **Categories** say what the money was for (`food.groceries`), the **merchant** says who was paid, **tags** say who/why/where (`kids`, `trip:rome`, `house`). Mortgage principal is a transfer into the loan, interest is housing cost — so the savings rate is honest.

## Develop

```bash
make install
make dev            # server on :8080 (converts finance_tracker/backend/finance.db on first start) + web on :5175
make test           # go vet + race tests, tsc + vitest
```

`make convert FROM=finances.json TO=/tmp/v2.db` converts a v1 backup offline and prints the reconciliation report (every transaction by id, every balance value, totals per year, secrets, bank state).

## Release

Bump `version` in `finance_tracker/config.yaml`, commit, push to `main`. CI runs the tests, builds the aarch64 image and pushes `ghcr.io/mindaugasbalciunas/finance-tracker:<version>`; update the add-on in Home Assistant.

### Upgrading from v1

The first start of 2.x converts `/data/finance.db` into `/data/finance-v2.db`, checks every transaction and balance against the source, and refuses to start (removing the half-made v2 file) if anything differs. The v1 file is never written, so reinstalling 1.79.0 is a full rollback — anything entered in v2 after the upgrade would then be missing from v1.

## MCP server

### In Claude chat (custom connector)

The app serves its MCP tools itself at `https://your-host/mcp`, with OAuth sign-in, so Claude chat (web, desktop and mobile) can use them — including live, read-only Interactive Brokers data (`ibkr_live`):

1. Set the add-on's `public_url` option to the address the app is reachable at from the internet over https (e.g. `https://budget.example`). Without it the app names itself from each request's host.
2. In Claude: **Settings → Connectors → Add custom connector**, paste `https://your-host/mcp`, press **Connect**.
3. The app opens its approval page (behind its web password and lock): choose **read only** or **read and write**, then **Allow**.

Connections are listed in Settings → Security → AI apps, with when each was last used, and can be disconnected there; they stop working at once. Access tokens last an hour and refresh tokens rotate (30 days from last use); using a refresh token twice ends the connection. Redirects are accepted only to `claude.ai`, `claude.com` or localhost. nginx lets only `/mcp`, `/oauth/authorize|token|register` and the two `/.well-known/oauth-*` documents past the web password; every MCP call checks the connection's own token and scope, exactly like API tokens. A read-only connection isn't offered the write tools at all. The plain API tokens (`ftk_`/`ftkw_`) don't work on `/mcp` — it skips the web password; use the stdio server below for them. With demo mode on, a connection sees the demo data, like API tokens.

### Stdio server

`finance-tracker-mcp` (built into the image at `/app/finance-tracker-mcp`, or `make mcp`) exposes the API as MCP tools for Claude Desktop / Claude Code:

```json
{"mcpServers": {"finance-tracker": {
  "command": "/path/to/finance-tracker-mcp",
  "env": {"FT_API_URL": "https://your-host/api", "FT_API_TOKEN": "ftk_…", "FT_BASIC_AUTH": "user:password"}}}}
```

Mint tokens in Settings → Security. A read-only token (`ftk_`) reads data; a read-write token (`ftkw_`) may also add/edit transactions, rules and tags, improve bank-inbox proposals, and manage the plan (budget lines, plan settings, recurring items, goals, trips and their dates) and the assistant's memory, and write a purchase into the decision journal (`log_decision`) — never accept bank rows, change settings or read backups.

Planning tools (read-only unless noted), the same in Ask CFO:

| Tool | Returns |
|---|---|
| `get_payday_runbook` | This month's pay-yourself-first steps — each saving line's amount, the accounts it usually moves between, done / partial / todo from the bank |
| `get_next_euro` | Where a spare sum is best put: buffer, 3rd-pillar refund room, a goal about to miss its date, loan rate vs expected return — ranked and split |
| `get_lifestyle_creep` | Last 12 months against a year (or `years: 3`) earlier: income vs spending growth, what grew and fell most, new regular bills |
| `get_price_creep` | Regular bills against a year ago, renewals within 45 days, the personal inflation rate and Lithuania's official one |
| `get_decisions` / `get_decision_impact` | The decision journal and its record; what a purchase would do to the month, buffer, wish list and FI date |
| `log_decision` | **Writes** (read-write token): a purchase being considered, with its cooling-off date — never decides, rates or deletes |
| `get_ownership_costs` | The true yearly cost of each property and vehicle — running costs, interest, value lost, money tied up — against renting or a taxi |
| `get_fi_plan` | Independence in levels — obligations, essentials, full, comfortable — with capital, progress and date each; when saving could stop (coast); projected payouts; what more saving buys |
| `get_milestones` | Dated milestones at today's pace: emergency fund, the next round figures of liquid wealth, loans paid off, FI |
| `get_fees` | What investments cost a year and over 20 years, against a 0.2% index fund; funds without a fee are listed as missing |
| `get_subscriptions` | Cancellable regular charges with yearly cost, new ones, free trials, and your keep / cancel-by verdicts |
| `get_cash_calendar` | Cash day by day for the next weeks with every bill, transfer and salary on its date, and the tightest day |

On connect the server sends Claude the same data model, strategy and trust rules Ask CFO follows in the app (`internal/advice`), and offers four ready-made analyses as prompts — `/` commands in Claude Code, one click in Claude Desktop:

| Prompt | Does |
|---|---|
| `month_close` | A finished month: cash flow vs the 6-month average, budget, what drove it, the wish-list funding split |
| `budget_review` | Lines over or under for months, unbudgeted spending, proposed changes (applied only after approval) |
| `wishlist_review` | Goals and planned trips vs the left-over rate, and which budget lines would bring them closer |
| `reconcile_recent` | `get_reconciliation`: recorded bank balances vs the ledger, late bookings and lasting gaps explained |
