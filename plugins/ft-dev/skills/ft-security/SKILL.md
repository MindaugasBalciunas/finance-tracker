---
name: ft-security
description: Security model and checklist for finance-tracker — middleware chain, app lock, API token scopes (read prefixes / write allowlist), CSRF guard, body limits, secrets in backups/snapshots/responses, AI write tools, nginx gate, dependency audits, and rules for never mutating live data. Load before adding or changing any endpoint, auth/token logic, secret storage, AI tool, upload, or before a release.
---

# Security model and rules

This app holds a household's complete finances, bank PSD2 sessions, broker OAuth tokens and an AI API key. The owner asks for a security pass on every release.

## Request path

`nginx` (basic auth, `auth_request` gate, TLS headers) → Go `Handler()`:

```
recoverer → crossOriginGuard → limitBodies → authenticate → routes
```

- **recoverer** — panics become `500 internal error`; the panic is logged, not returned.
- **crossOriginGuard** — `http.CrossOriginProtection`: blocks browser state-changing requests from another site. Consequence: **GET handlers must never change state** (they're not CSRF-protected and read tokens can call them).
- **limitBodies** — 2 MB JSON; 200 MB only for `/api/import/`, `/api/ai/chat`, `/api/ai/scan`. A new upload route must be added there deliberately.
- **authenticate** — skipped for `/api/auth/*` and `/api/health`; when the lock is enabled requires a valid `ft_session` cookie (HttpOnly, SameSite=Lax, Secure on HTTPS) or an API token.
- **Outside `/api/`** `authenticate` checks nothing: those routes guard themselves. The AI-connector paths (`/mcp`, `/oauth/authorize|token|register`, `/.well-known/oauth-*`) are the only ones nginx lets past the web password (exact regex location, `test/nginx_gate.sh` probes neighbours and traversals). `/mcp` accepts only an OAuth connection's `ftoa_` token and runs tools in process through `tokenAllows` with that scope; approving a connection (`/api/oauth/approve`) is session-only and locked in demo mode. Never let API tokens (`ftk_`/`ftkw_`) work on a path that skips the password.

## API tokens (MCP, scripts)

- `ftk_` read-only, `ftkw_` read-write, minted in Settings → Security; checked by `auth.TokenScope`.
- Reads: GET only, and only under `tokenReadPrefixes` (data — never settings, secrets, exports, chat).
- Writes (rw only): exact method + path in `tokenWriteRoutes`. `pathUnder` semantics: `"…$"` exact path, trailing `/` any sub-path, otherwise path or `path/…`.
- **A new route is session-only by default. Leave it that way** unless an agent genuinely needs it. Never open to tokens: accepting bank inbox rows into the ledger, bulk delete, settings, keys, security, bank connections, backups/exports, the chat.
- Any change here → extend `TestAuthBoundaries` (`internal/api/api_test.go`) with both the allowed and a denied neighbour.

## Secrets

- Stored in `settings` keys (`auth`, `bank`, `ibkr`, `notify`, the AI key under `ai`) and tables (`webauthn_credentials`, `bank_*`).
- **Never return a secret from an endpoint.** Return presence (`"configured": true`) or a masked hint. `TestSecretsNeverEchoed` / `TestIBKRTokensStayPrivate` are the patterns — add a case for every new secret.
- Backups exclude secrets unless `include_secrets`; a restore without secrets keeps the instance's own. New secret → `backup.secretTables`/`secretSettings`; re-grantable → strip in `db.scrub` too.
- Never log tokens, PINs, cookies, API keys, full bank payloads or AI request bodies. Never put secrets in URLs/query strings, test fixtures copied from real data, commits, or the AI export.
- `.env` is gitignored; keep it that way.

## Code-level rules

- SQL: placeholders (`?`) for every value. Only constant identifiers (e.g. table names from `backup.tables`) may be concatenated.
- Decode into typed structs; zero server-owned fields from client input (`ID`, `ExternalID`, `SplitOf`, `Source`).
- Validate in the domain package; the UI restriction is a convenience, not the control.
- Outbound HTTP (bank, IBKR, Yahoo, Claude, notify): every `http.Client` has a `Timeout` (keep it that way), no following user-provided URLs server-side, errors mapped to plain messages (bank `APIError` → 502/409 "reconnect").
- Web: AI/markdown output goes through `react-markdown` without raw HTML (no `rehype-raw`, no `dangerouslySetInnerHTML`). Only same-origin `api…` fetches.
- PIN brute-force throttle, idle session expiry and passkey verification have tests in `internal/auth` — keep them green and add to them when touching auth.

## AI and MCP

- Every mutating Ask CFO tool must be in `writeTools` (`internal/ai/tools.go`); confirmation before a write is required by the system prompt (not by code), so a write tool's description must say plainly what it changes.
- MCP tools run with the user's token — their reach is exactly the token scope above. Don't widen the scope to make an MCP tool work; reconsider the tool.
- Amounts in answers not found in tool results are flagged (`unchecked`) — don't remove that.

## Live instances — never test destructively

- The owner's local instance (`:8080` API / Vite) has a PIN and passkey; never change its auth state or data for testing. Production (HA add-on) is not writable from here.
- Never send "should be rejected" calls (DELETE, PUT, bulk) at a live instance to prove a guard: if the lock is off the guard is skipped and the call **executes** (this once wiped every transaction). Prove auth boundaries with in-memory tests that enable the lock (`a.SetupPin("", "1234")`).
- Live checks run on an isolated copy (see `ft-live-verify`). The plugin's guard hook asks before mutating HTTP calls or SQL writes against live databases.

## Release security pass

From `finance_tracker/server`: `go vet ./...`, `go test -race ./...`, `~/go/bin/govulncheck ./...` (report any *called* vulnerability as a blocker; uncalled ones as notes). From `finance_tracker/web`: `npm audit --omit=dev` (0 is the bar). Check the `Dockerfile` `golang:` image matches `go.mod`'s toolchain and nginx base is current. `config.yaml` keeps `8080/tcp: null` (the debug port bypasses nginx auth). CI also runs `test/nginx_gate.sh` against real nginx.
