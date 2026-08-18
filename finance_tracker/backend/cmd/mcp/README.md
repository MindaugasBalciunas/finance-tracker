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

## Connecting to Google Gemini

The server is a standard stdio MCP server, so Gemini's tooling attaches to it
the same way Claude's does.

**Gemini CLI** — either register it with one command:

```bash
gemini mcp add finance-tracker /Users/you/bin/finance-tracker-mcp \
  -e FT_API_URL=http://homeassistant.local:8098/api/v1 \
  -e FT_API_TOKEN=ftk_...
```

or add it to `~/.gemini/settings.json` (same shape as the Claude config):

```json
{
  "mcpServers": {
    "finance-tracker": {
      "command": "/Users/you/bin/finance-tracker-mcp",
      "env": {
        "FT_API_URL": "http://homeassistant.local:8098/api/v1",
        "FT_API_TOKEN": "ftk_..."
      },
      "timeout": 30000,
      "trust": false
    }
  }
}
```

Check it with `/mcp` inside the CLI — the twelve tools should list. Keep
`trust: false` so tool calls stay confirm-first; everything is read-only
regardless, enforced server-side by the token scope.

**Gemini API (google-genai SDK)** — the SDK accepts a live MCP client
session as a tool, so Gemini models can call this server from your own code:

```python
from google import genai
from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

params = StdioServerParameters(
    command="/Users/you/bin/finance-tracker-mcp",
    env={"FT_API_URL": "http://homeassistant.local:8098/api/v1",
         "FT_API_TOKEN": "ftk_..."})

async with stdio_client(params) as (r, w):
    async with ClientSession(r, w) as session:
        await session.initialize()
        client = genai.Client()
        resp = await client.aio.models.generate_content(
            model="gemini-2.5-pro",
            contents="How am I tracking against my budgets this month?",
            config=genai.types.GenerateContentConfig(tools=[session]))
        print(resp.text)
```

The same safeguards apply to any client: GET-only allowlisted routes, capped
row counts, and a revocable token — nothing a model can call mutates data.

## Tools

| Tool | What it returns |
|---|---|
| `get_overview` | The full financial report: balances, summaries, this month vs budgets, safe-to-spend, stock positions with live prices |
| `search_transactions` | Filtered transaction rows (comment substring, labels any/all, category, type, date range, **amount range**, sort; capped) |
| `get_summary` | Aggregated totals + per-category, per-month **and per-label** breakdowns, same filters as search |
| `get_label_stats` | Every label's footprint (count, volume, rules, budgets) |
| `get_label_rules` | The auto-labeling rules (pattern → label), i.e. *why* rows carry a label |
| `get_budgets` | The monthly plan: fixed obligations, investment targets, spending limits |
| `get_budget_status` | Month-to-date progress per budget line: budgeted / spent / remaining, discretionary total, safe-to-spend (any month via `month=YYYY-MM`) |
| `get_balances` | Account balance snapshots over time |
| `get_stock_portfolio` | Holdings with cost basis and realized gains |
| `get_stock_trades` | The individual trade ledger, newest first, optional ticker filter |
| `get_stock_quote` | Live market price for one ticker |
| `get_assets` | Physical assets with loans, interest structure and equity |

The same twelve tools are available to the in-app AI chat (executed
in-process), so the web chat and any MCP client answer with identical
query power.
