---
name: ft-architecture
description: Architecture and coding conventions of finance-tracker v2 (Go server in finance_tracker/server, React web in finance_tracker/web). Load before designing or writing any feature, endpoint, domain function or refactor in this repo — it gives the layering, the money/date types, the handler idiom and the end-to-end checklist of every place a feature must touch.
---

# finance-tracker v2 architecture

A personal CFO for one household, shipped as a Home Assistant add-on: one container, nginx (basic auth + gate) → Go server → SQLite. Single user, mobile-first.

## Layout

| Path | What |
|---|---|
| `finance_tracker/server` | Go module `ft`. stdlib `net/http`, `modernc.org/sqlite` (no CGO), testify only in a few tests |
| `server/cmd/server` | the app. `cmd/mcp` = stdio MCP server over the HTTP API (tools in `internal/mcptools`, also served at `/mcp`). `cmd/v1convert` = offline v1 conversion |
| `server/internal/<domain>` | `ledger` `wealth` `plan` `goals` `insights` `cfo` `bank` `ibkr` `market` `merchant` `ai` `advice` `auth` `backup` `db` `boot` `importv1` `demo` `notify` `usage` `money` `testutil` |
| `server/internal/api` | HTTP surface: `api.go` (middleware, helpers), `routes.go`, one `routes_<domain>.go` per domain |
| `finance_tracker/web` | React 18 + TS, React Router 7 (**HashRouter**), TanStack Query 5, Tailwind 4, Recharts, Vitest |
| `finance_tracker/{Dockerfile,run.sh,nginx.conf,config.yaml}` | add-on packaging; `config.yaml` `version` is the release version |
| `finance_tracker/test/nginx_gate.sh` | real-nginx integration test of the password/passkey gate (runs in CI) |

## Layering rules

1. **Domain packages are plain functions over the database.** `ledger.Validate(q querier, t *Tx)`, `ledger.SaveAccount(e execer, a Account)`, `plan.LoadSettings(d)`. No repository/service/interface layers. Accept the narrowest of `querier` / `execer` / `*sql.Tx` / `*sql.DB` so the function works inside a transaction. Only things holding external clients or state are structs (`bank.Service`, `ibkr.Service`, `ai.Assistant`, `auth.Service`).
2. **Business rules live in the domain package, not the handler.** Handlers decode, call domain functions, return. Invariants (e.g. property/vehicle never "paid from") are enforced in the domain (`ledger.Validate`) *and* mirrored in the UI (`AccountSelect kinds`), never only in the UI.
3. **Cross-domain views go in `cfo`** (Home overview, plan report, cash until payday) or `insights`; they read, they don't write. Computations that depend on "today" take `now time.Time` as a parameter.
4. **`api` imports domains; domains never import `api`.** `advice` holds assistant rules shared by `ai` and `mcptools` — keep one source of truth.
5. **Dependencies: stdlib first.** A new module needs a strong reason (security surface, Docker image size, `govulncheck`). Never `go mod tidy` with a newer local toolchain without checking the `go` directive still matches the Dockerfile's `golang:` image.

## Types and data conventions

- **Money is `money.Cents` (int64 EUR cents) everywhere in Go.** JSON marshals as decimal euros (`12.34`), so the web works in euro numbers. Never do float arithmetic on money in Go; convert at the edge with `money.FromFloat`. Amounts on a `Tx` are always positive; direction comes from `Kind` (`Tx.Signed()`).
- **Dates are strings**: day `YYYY-MM-DD`, month `YYYY-MM`. Timestamps via `db.Now()` (UTC RFC3339). Local day for "today" = `time.Now().Format("2006-01-02")`.
- **Accounts** have a `Kind`: checking, savings, cash, brokerage, pension, crypto, property, vehicle, loan. Net worth = latest balance per account (loans negative). Property/vehicle are valuation-only; loan/pension only receive transfers.
- **Transactions**: income · expense · transfer. Categories are two-level ids (`food.groceries`); a category's kind must equal the tx kind. `transfer.invest/pension/debt/asset` build wealth, `transfer.internal` just moves cash. Mortgage principal = transfer into the loan; interest = housing expense.
- **Tags** are normalized (`ledger.NormalizeTags`); `trip:<name>` marks trips. Merchant = who was paid.
- **Bank rows** dedup by `external_id`; card reservations are `pending` until settled.

## HTTP handler idiom

```go
// routes_<domain>.go
func (s *Server) widgetRoutes() {
	s.handle("GET /api/widgets/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return widgets.Get(s.DB, id) // sql.ErrNoRows → 404 automatically
	})
	s.handle("POST /api/widgets", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in widgets.Widget
		if err := decode(r, &in); err != nil {
			return nil, err // 400 "invalid request body: …"
		}
		in.ID = 0 // never trust ids / server-owned fields from the client
		if err := widgets.Save(s.DB, &in); err != nil {
			return nil, err // plain error → 400 with its message, so write messages for the user
		}
		return in, nil
	})
}
```

- Register the group in `routes.go`. Use Go 1.22+ patterns (`"METHOD /path/{id}"`).
- Errors: `bad(msg)` 400, `notFound(msg)` 404, `&HTTPError{Status, Msg}` for anything else. Error text reaches the user — write it as a plain sentence, no stack/SQL detail.
- Return `nil, nil` only when the handler wrote the response itself (files, streams).
- Query helpers: `qint`, `qlist` (comma or repeated), `parseCents`, `filterFrom`.
- On update, copy server-owned fields from the current row (`ExternalID`, `SplitOf`, `Source`) instead of taking them from the body.

## Code style

- Comments explain *why* in plain English and name the user-visible effect ("a €769 car service … vanished from the month"). Every package has a doc comment. Match the surrounding density; don't narrate obvious code.
- Names are short and domain-y (`Tx`, `Bal`, `moveBalances`). Long one-line struct literals are normal here.
- User-facing wording is short, plain, provider-neutral ("AI", not a vendor name), and uses € with `eur()` formatting.

## Adding a feature end to end

Read `references/feature-checklist.md` — it lists every place a feature may need to touch (migration, backup tables, token scope, AI tool, MCP tool, demo seed, web hooks, docs) with the reason each one exists. Skipping one of those is the most common way a feature ships half-done.

## Related skills

`ft-migration` (schema), `ft-security` (auth/secrets/token scope), `ft-testing`, `ft-web-ui`, `ft-ai-mcp`, `ft-live-verify`, `ft-release`.
