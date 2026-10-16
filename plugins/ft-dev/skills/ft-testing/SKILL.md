---
name: ft-testing
description: Testing patterns for finance-tracker — Go tests with internal/testutil fixtures (DB, Tx, Bal, E), black-box API tests through httptest with the api_test client, fakes for Claude/bank/IBKR, fixed clocks, auth-boundary tests, and Vitest + Testing Library for the web with stubbed fetch. Load before writing or fixing tests, or when deciding what a change needs to be covered by.
---

# Testing patterns

Gate: `make test` from the repo root = `go vet ./... && go test -race ./...` (server) + `npx tsc --noEmit && npx vitest run` (web). CI additionally runs `npm run build` and `test/nginx_gate.sh`. Never call a change done on a red or skipped suite; report failures with their output.

Locally the toolchain is pinned in `go.mod`; if `go` complains, run with `GOTOOLCHAIN=<toolchain from go.mod>`.

## Go

### Shape

- Prefer black-box tests: `package ledger_test`, import the package plus `. "ft/internal/testutil"`. Use `*_internal_test.go` (`package ai`) only to test unexported logic.
- **Name tests as the behaviour, in a sentence**: `TestDeletingABankRowReturnsItToTheInbox`, `TestOlderBackupRestoresIntoNewerSchema`. Put a one-paragraph comment above complex tests saying what must hold and why.
- Plain `if … { t.Fatalf("got %v want %v") }` is the norm; testify exists but isn't the house style.
- Table tests for validation and parsing (`TestValidate` in `ledger_test.go`).

### Fixtures (`internal/testutil`)

```go
d := DB(t)                         // in-memory SQLite, all migrations, category tree,
                                   // accounts: swed seb cash revolut (checking), ibkr, artea (pension),
                                   // btc_m, house (property), mortgage (loan)
Tx(t, d, ledger.Tx{Date: "2026-09-01", Amount: E(10), Category: "food", AccountID: "swed"}) // validated insert
Bal(t, d, "swed", "2026-10-06", 500)  // balance in euros
E(12.34)                              // money.Cents from euros
```

Add new fixture helpers to `testutil` when two packages need them; keep them small and realistic.

### Time

Anything depending on "today" takes `now time.Time`; tests pass `time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)`. Never let a test depend on the wall clock or the month it runs in.

### Read-only computations

Views like cash-until-payday must change nothing. Pattern (`cfo/cash_test.go`): fingerprint the tables before and after, assert equal.

### HTTP / API

`internal/api/api_test.go` has the harness:

```go
s, c := newServer(t)                  // api.New over testutil.DB, httptest server, cookie jar
c.ok("POST", "/transactions", body, &out) // fails the test on non-200
code, raw := c.do("DELETE", "/rules/1", nil)
tok := c.with("Authorization", "Bearer "+rw) // fresh jar + header
```

- Auth: enable the lock in-process (`a := &auth.Service{DB: s.DB}; a.SetupPin("", "1234"); a.MintToken(true)`), then assert 401/403/200 per route. Every token-scope change gets an allowed **and** a denied neighbour case in `TestAuthBoundaries`.
- Secrets: assert the response body does not contain the secret string (`TestSecretsNeverEchoed`).
- CSRF: send `Sec-Fetch-Site: cross-site` and expect 403 (`TestCrossSiteWritesBlocked`).

### External services

Never reach a real bank, IBKR, Yahoo or Claude from a test. Each client takes a base URL; tests stand up `httptest.NewServer` fakes: `fakeClaude` in `ai/ai_test.go` (scripted replies, records requests and headers), the fake bank in `bank/sync_test.go`, IBKR fakes in `ibkr/ibkr_test.go`. Extend those fakes rather than inventing new ones. Assert on what was *sent* too (headers, no secrets leaked, tool schemas).

### Migrations and backups

See `ft-migration`: new tables need rows in the backup round-trip fixtures, data-changing migrations get a before/after test.

## Web (Vitest 4 + Testing Library, jsdom, globals on)

- Tests live in `web/src/test/*.test.ts(x)`; `setup.ts` loads jest-dom matchers.
- Pure logic in `src/lib` (format, periods, categories) → `lib.test.ts` unit tests; keep money formatting cases (cents kept under €1,000, `—` for null).
- Components: `render`, `screen`, `fireEvent`/`user-event`, `waitFor`. Assert what the user sees (text, role, `aria-invalid`), and semantic classes (`text-good`, `bg-bad`) when colour carries meaning.
- Network: `vi.stubGlobal('fetch', vi.fn(async (url, init) => new Response(JSON.stringify(body), { status })))`, record calls to assert request bodies; `afterEach(() => vi.restoreAllMocks())`. Components using queries need a `QueryClientProvider` with `retry: false`.
- `npx tsc --noEmit` is part of the gate — no `any` sprawl to silence it.

## What a change needs

| Change | Minimum tests |
|---|---|
| Domain rule / calculation | Go unit test in the package, incl. an edge case (month boundary, refund, transfer, zero) |
| New endpoint | API test through `newServer`; auth case if token-reachable |
| Secret / auth / token scope | `internal/auth` or `api_test` boundary tests with the lock on |
| Schema | migration test + backup round-trip fixtures + live round-trip (`ft-migration`) |
| Bank / IBKR / AI client | fake-server test incl. error and expiry paths |
| UI logic | Vitest; plus live check at 390px in Firefox (`ft-live-verify`) |
| Bug fix | a test that fails before the fix |
