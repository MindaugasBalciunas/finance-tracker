---
name: ft-ai-mcp
description: How Ask CFO (internal/ai) and the MCP server (cmd/mcp) are built and how to add or change an AI tool, MCP tool or prompt, the shared advice rules, write-tool confirmation, unchecked-amount warnings, model settings and the fake Claude used in tests. Load before touching internal/ai, internal/advice, internal/mcptools, internal/oauth, cmd/mcp, the receipt scan or the AI export.
---

# Ask CFO and the MCP server

Two assistants read the same data and must follow the same rules:

- **Ask CFO** (`internal/ai`): in-app chat over the Claude Messages API (Claude API or a gateway; key + model in Settings → AI, default `ai.DefaultModel`). `assistant.go` runs the tool loop, `tools.go` defines tools, `client.go` talks HTTP and records spend in `ai_spend`. Chats can finish in the background (`api/chatjob.go`).
- **MCP tools** (`internal/mcptools`): every tool, the shared instructions (`prompts.go`) and the ready-made prompts (`month_close`, `budget_review`, `wishlist_review`, `reconcile_recent`), built over a `Caller`. Two transports use them: `cmd/mcp` (stdio, HTTP API with a user-minted token: `FT_API_URL`, `FT_API_TOKEN`, optional `FT_BASIC_AUTH`) and the app's own `/mcp` endpoint (`api/routes_mcp.go`, Claude custom connectors) which calls the routes in process with an OAuth connection's scope (`internal/oauth`, `ftoa_` tokens only). Write tools are listed in `mcptools.WriteTools` and hidden from read-only connections (`TestWriteToolsListed` checks it).
- **`internal/advice`**: `DataModel`, `Strategy`, `Trust` — the single source of how to read the data and advise. Both system prompts embed it. Change rules here, not in two places.

## Adding a tool

1. Domain function first (testable, takes `now` if time-dependent). Reuse the app's own figures — the Trust rules forbid assistants adding up rows for totals, so give them a tool that returns the total.
2. **Ask CFO**: add to `toolDefs()` with `obj/str/num/arr` schema helpers and a description that says what it returns and the units/ids it expects; dispatch it in `runTool`'s `switch name` (same file); if it changes data, add the name to `writeTools` — that marks the reply `Changed` so the web refetches, and the system prompt requires the owner's explicit confirmation before any write tool (prompt-enforced, so keep the tool's description explicit about what it changes).
3. **MCP**: `mcp.AddTool` in `internal/mcptools/tools.go` (a write tool's description starts with `WRITES` and its name goes into `WriteTools`) with the **same name** and a matching description; it calls the HTTP API, so the route must be inside the token scope — reads in `tokenReadPrefixes`, writes in `tokenWriteRoutes` for `ftkw_` tokens only. Do not widen the scope for convenience (see `ft-security`); bank-row acceptance, settings, backups and chat stay out.
4. Results: compact JSON, euros as numbers (via `money.Cents` marshalling), ids included so follow-up tools can act; cap row counts and say so (search returns ≤200 rows plus totals over the whole match).
5. Tests: `ai/ai_test.go` `fakeClaude` scripts replies and records requests — assert the tool appears in the request, the result is fed back as a `tool_result`, errors come back with `is_error`, and every mutating tool is in `writeTools` (`ibkr_tool_test.go` checks the map). `cmd/mcp/main_test.go` (stdio, prompts) and `api/mcp_test.go` (remote endpoint, OAuth, scopes) for the MCP side.
6. Docs: README's MCP section if a tool or prompt is user-visible.

## Guards to keep

- `unchecked`: euro amounts in an answer that appear in no tool result are stored on the message and shown as a warning. Keep new tools' outputs in the set that is checked.
- Truncation and refusals produce a plain note (`answer truncated…`, `the model declined…`), never a silent cut.
- Cost tracking per call (`ai_spend`) must stay accurate when adding new request kinds (scan, chat, summary).
- The AI key is a secret: never returned, logged or included in exports (see `ft-security`).
- Wording in UI and exports is provider-neutral ("AI").

## Models

Use current Claude model ids (`claude-opus-5-5`, `claude-sonnet-5-5`, `claude-haiku-4-5-20251001`); the Settings list comes from the provider's `/models`. For anything about the Messages API itself (params, tools, caching, streaming), load the `claude-api` skill rather than relying on memory.
