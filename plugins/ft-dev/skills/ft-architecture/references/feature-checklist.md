# Feature checklist — every place a change may need to touch

Go through this list for any new entity, field, endpoint or computed view. Tick each item as either done or "not needed because …". Paths are relative to `finance_tracker/`.

## Server

| # | Where | When | Why |
|---|---|---|---|
| 1 | `server/internal/<domain>/*.go` | always | Business rule + validation live here, testable without HTTP |
| 2 | `server/internal/<domain>/*_test.go` | always | Behaviour test named as a sentence (see `ft-testing`) |
| 3 | `server/internal/db/db.go` `migrations` | new table/column/index or data fix | Append-only, numbered by `PRAGMA user_version` (see `ft-migration`) |
| 4 | `server/internal/backup/backup.go` `tables` | new table | Backups are a table dump of this list only — a missing table is silently lost on restore. Parents before children |
| 5 | `backup.go` `secretTables` / `secretSettings`, `db.go` `scrub` | table or settings key holding credentials, tokens, webhook URLs | Secret-less backups and on-device snapshots must not carry them |
| 6 | `server/internal/api/routes_<domain>.go` + `routes.go` | new endpoint | Thin handler, `s.handle` idiom |
| 7 | `server/internal/api/api.go` `tokenReadPrefixes` / `tokenWriteRoutes` | only if API tokens (MCP, scripts) should reach it | Default is session-only. Writes need a deliberate decision (see `ft-security`) |
| 8 | `server/internal/api/api_test.go` | new endpoint, esp. auth-relevant | `TestAuthBoundaries`, `TestSecretsNeverEchoed` style |
| 9 | `server/internal/ai/tools.go` (+ `writeTools`) and `assistant.go` dispatch | Ask CFO should read/act on it | Mutating tools must be in `writeTools` (confirmation required) |
| 10 | `server/internal/mcptools/tools.go` (+ `WriteTools` for writes) | Claude Desktop/Code should see it | Keep tool names identical to Ask CFO's; route must be inside the token scope from #7 |
| 11 | `server/internal/advice/advice.go` | the way data should be read/advised changes | Shared rules for Ask CFO and MCP |
| 12 | `server/internal/demo/seed.go` | a page would look empty in demo mode | Demo mode must look like a real household; `TestSeedBuildsAUsableDemo` |
| 13 | `server/internal/importv1` | only if v1 data maps to it | v1 → v2 conversion is verified to the cent |
| 14 | `cfo/overview.go` / Home to-dos / `notify` | the owner must act on it (expiring sign-in, inbox) | The Home bell is where to-dos live |

## Web

| # | Where | When | Why |
|---|---|---|---|
| 15 | `web/src/lib/types.ts` | new/changed payload | One shared type per payload |
| 16 | `web/src/lib/hooks.ts` | data read in more than one place | `useQuery` with a stable `queryKey`; set `staleTime` for reference data |
| 17 | mutations | any write | `invalidateQueries` for every key whose numbers change (often `['overview']`, `['plan', month]`, `['transactions']`) |
| 18 | `web/src/pages/**`, `components/**` | UI | Reuse `components/ui.tsx` primitives (see `ft-web-ui`) |
| 19 | `web/src/test/*.test.ts(x)` | logic in `lib/`, non-trivial components | Vitest + Testing Library |

## Repo

| # | Where | When |
|---|---|---|
| 20 | `README.md` | new MCP tool/prompt, setup or API change |
| 21 | `docs/feature-parity.md` | a feature is added or removed |
| 22 | `finance_tracker/config.yaml` `version` | every release (see `ft-release`) |

## Done means

`make test` green (go vet + race, tsc, vitest) → live check on an isolated copy for UI/schema changes (`ft-live-verify`) → backup round-trip for schema changes (`ft-migration`) → release (`ft-release`).
