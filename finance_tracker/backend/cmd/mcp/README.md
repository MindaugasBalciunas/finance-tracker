# finance-tracker-mcp

A read-only [MCP](https://modelcontextprotocol.io) server that gives MCP
clients (Claude Desktop, Claude Code, or an AI gateway that attaches MCP
servers) safe, typed access to your Finance Tracker data for deeper insights.

## Safeguards

- **Read-only by construction** — every tool is an HTTP `GET` against the
  app's REST API. The backend independently enforces the same rule: the API
  token is rejected on anything but `GET`, and only on an allowlist of data
  routes. Backups (which contain the AI key), settings, auth and chat history
  are never reachable with the token, even if this binary were modified.
- **Scoped credential** — the token is minted in the app (Security → API
  token), stored server-side only as a SHA-256 hash, shown once, and
  revocable/rotatable at any time without touching your PIN.
- **Bounded output** — list tools cap rows (default 100, max 500) and
  responses are hard-capped at 4 MB, so a broad query can't dump the ledger.
- **No secrets in transit to the model** — tool output is your financial
  data only; the token itself is read from `FT_API_TOKEN` and never echoed.

## Setup

1. Build: `go build -o ~/bin/finance-tracker-mcp ./cmd/mcp` (from `backend/`).
2. In the app: **Security → API token → Generate**, copy the `ftk_…` token.
3. Register the server, e.g. for Claude Code:

```bash
claude mcp add finance-tracker ~/bin/finance-tracker-mcp \
  -e FT_API_URL=http://homeassistant.local:8098/api/v1 \
  -e FT_API_TOKEN=ftk_...
```

or in Claude Desktop's `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "finance-tracker": {
      "command": "/Users/you/bin/finance-tracker-mcp",
      "env": {
        "FT_API_URL": "http://homeassistant.local:8098/api/v1",
        "FT_API_TOKEN": "ftk_..."
      }
    }
  }
}
```

Port 8098 is the add-on's LAN-exposed API port (see `config.yaml` ports);
for local development use `http://localhost:8080/api/v1`.

## Tools

| Tool | What it returns |
|---|---|
| `get_overview` | The full financial report: balances, summaries, this month vs budgets, safe-to-spend, stock positions with live prices |
| `search_transactions` | Filtered transaction rows (comment, labels, category, type, dates; capped) |
| `get_summary` | Aggregated totals + per-category and per-month breakdowns for a period |
| `get_label_stats` | Every label's footprint (count, volume, rules, budgets) |
| `get_budgets` | The monthly plan: fixed obligations, investment targets, spending limits |
| `get_balances` | Account balance snapshots over time |
| `get_stock_portfolio` | Holdings with cost basis and realized gains |
| `get_stock_quote` | Live market price for one ticker |
| `get_assets` | Physical assets with loans, interest structure and equity |
