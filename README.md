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

`finance-tracker-mcp` (built into the image at `/app/finance-tracker-mcp`, or `make mcp`) exposes the API as MCP tools for Claude Desktop / Claude Code:

```json
{"mcpServers": {"finance-tracker": {
  "command": "/path/to/finance-tracker-mcp",
  "env": {"FT_API_URL": "https://your-host/api", "FT_API_TOKEN": "ftk_…", "FT_BASIC_AUTH": "user:password"}}}}
```

Mint tokens in Settings → Security. A read-only token (`ftk_`) reads data; a read-write token (`ftkw_`) may also add/edit transactions, rules and tags and improve bank-inbox proposals — never accept bank rows, change settings or read backups.
