# finance-tracker

Personal CFO web app shipped as a Home Assistant add-on. Go server in `finance_tracker/server`, React web in `finance_tracker/web`. See `README.md` for the domain model.

The `ft-dev` plugin (`plugins/ft-dev`, enabled in `.claude/settings.json`) holds the working rules. Load the matching skill before the work, not after:

| Work | Skill |
|---|---|
| Any feature, endpoint, refactor | `ft-dev:ft-architecture` (+ its feature checklist) |
| Schema, migration, data fix, backup | `ft-dev:ft-migration` |
| Auth, tokens, secrets, uploads, release audit | `ft-dev:ft-security` |
| Writing or fixing tests | `ft-dev:ft-testing` |
| Pages, components, charts, forms | `ft-dev:ft-web-ui` |
| Ask CFO tools, MCP, advice | `ft-dev:ft-ai-mcp` |
| Running or screenshotting the app | `ft-dev:ft-live-verify` |
| Version bump, commit, push, CI | `ft-dev:ft-release` |

Before shipping a non-trivial change, run the `ft-reviewer` agent on the diff.

Hard rules:
- `make test` must be green before anything is called done.
- Never mutate the owner's live data (local `finance_tracker/backend/*.db`, `:8080`, production). Verify on a `VACUUM INTO` copy on other ports. The plugin's hook asks before risky commands.
- Migrations in `internal/db/db.go` are append-only; new tables go into `backup.tables`.
- Money is `money.Cents` in Go; new routes are session-only unless deliberately added to the token scope.
