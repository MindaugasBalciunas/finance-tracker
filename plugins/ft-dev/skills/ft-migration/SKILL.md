---
name: ft-migration
description: How to change the finance-tracker SQLite schema or fix data safely — append-only numbered migrations in internal/db/db.go, backup table registration, secret classification, restore compatibility and the live backup/restore round-trip the owner requires. Load before adding a table, column, index, CHECK constraint, or any data repair that must reach production.
---

# Schema changes and data fixes

Production runs on the owner's Home Assistant box behind PIN + passkey; it is **not writable from here**. Every schema change and every production data fix ships as a migration that runs on add-on restart. The owner has said backups must never break — treat backup/restore as part of every schema change.

## The migration mechanism (`server/internal/db/db.go`)

- `var migrations = []string{schemaV1, ...}` — `migrations[i]` takes the schema from version `i` to `i+1`, tracked in `PRAGMA user_version`. **Append only.** Never edit, reorder or delete an existing entry; it has already run on production.
- Each migration runs in its own transaction; on a database that already has data, `Open` writes a `pre-migrate-*.db` `VACUUM INTO` snapshot first and refuses to migrate if that fails.
- There are no "repair on every boot" passes. Don't add any.
- `schema.sql` is version 1 — don't edit it for new changes either.

### Writing one

```go
	// 13: <what the owner gets, in a sentence>. Existing rows <what happens to them>.
	`ALTER TABLE goals ADD COLUMN colour TEXT NOT NULL DEFAULT '';`,
```

- Comment states the number and the user-facing reason, like the entries above it.
- Columns: `NOT NULL DEFAULT …` so old rows and old backups stay valid. Money columns are `INTEGER` cents with a `-- cents` comment. Dates `TEXT` `YYYY-MM-DD`; timestamps `TEXT` RFC3339 UTC.
- Use `CREATE TABLE IF NOT EXISTS` / `CREATE INDEX IF NOT EXISTS`, `CHECK (x IN (...))` for enums, `REFERENCES … ON DELETE CASCADE` where children die with the parent (foreign keys are ON).
- SQLite can't drop a CHECK or change a column type with `ALTER`; that needs the create-new / copy / drop / rename dance inside the same migration string. Prefer additive changes.
- Data fixes must be idempotent and narrowly targeted (`WHERE` on exact ids/values), and explain in the comment what was wrong.

## Wire the table everywhere it must go

1. `server/internal/backup/backup.go` → `tables` (dependency order, parents first). A table missing here is **silently lost** by backup/restore.
2. Credentials/tokens/webhook URLs → `secretTables` or `secretSettings`; if it's easy to re-grant, also strip it in `db.scrub` so on-device snapshots don't carry it.
3. `demo/seed.go` if the demo should show it.
4. `importv1` only if v1 had the data.

## Tests to add or keep green

- `internal/db/db_test.go`: `TestMigrateIsIdempotentAndBacksUpExistingData` covers the mechanism; add a focused test for any data-changing migration (see `TestPensionsBecomeLiquid`: build the pre-migration state, migrate, assert).
- `internal/backup/backup_test.go`: `TestRoundTripWithSecretsIsExact`, `TestBackupWithoutSecrets`, `TestOlderBackupRestoresIntoNewerSchema` — extend fixtures so the new table has rows, otherwise round-trip tests prove nothing about it.
- `make test`.

## Live round-trip (required for schema changes)

Unit tests once missed a real restore bug, so the owner wants a live check. Do it on copies, never on the live databases:

1. Build the new binary and the previous release: `git worktree add <scratch>/old <prev-release-sha>`.
2. Make copies with `sqlite3 <src.db> "VACUUM INTO '<scratch>/x.db'"` (plain `cp` drops the WAL and gives stale rows). Source: a recent `finance_tracker/backend/backups/*.db` or the local `finance-v2.db`.
3. Run probe servers on free ports (`lsof -iTCP:<port> -sTCP:LISTEN` first): `DB_PATH=<copy> V1_DB_PATH=/nonexistent PORT=8091 <binary>`.
4. Compare exports section by section (strip `exported_at`, `schema`):
   - old export → restore into a fresh new instance → export → equal;
   - old DB migrated by the new binary → export → equal to old export plus the new columns' defaults;
   - new-feature data → export → restore into fresh → export → equal;
   - rollback: the old binary on a new-migrated DB — does it still start and serve? If not, say so before release.
5. Kill probes by port: `kill $(lsof -t -iTCP:<port> -sTCP:LISTEN)`. Stale probes on deleted DBs give bogus results.

Known noise between runs: `created_at`, row ids of regenerated budget rows, map key order.

See `ft-live-verify` for running probe servers and `ft-security` for secret handling.
