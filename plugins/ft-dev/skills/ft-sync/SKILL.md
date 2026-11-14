---
name: ft-sync
description: Bring local finance-tracker in line with production before any task — local main to origin/main (code), then the local database to the owner's latest production backup (data), verified table by table before it is swapped in. Load FIRST at the start of every task in this repo, before any other ft-dev skill, and whenever asked to sync local and prod.
---

# Sync local with production — first step of every task

The owner's standing instruction (2026-11-14): **always sync local and prod before doing a task**, without asking each time. Production is the source of truth for data and runs what `main` released. Local drifts (bank syncs, edits on the phone), and work done on stale local data has given wrong answers before. The sync is one-way: prod → local. Nothing is ever written to prod.

Skip it only when `finance_tracker/backend/tmp/last-sync.json` already names a backup exported today, or the owner says to skip. Say which in one line.

## 1. Code: local `main` = `origin/main`

```bash
git fetch -q origin && git status -sb | head -1
```

- **Behind, clean tree, on `main`:** `git pull --ff-only`.
- **Ahead:** these are local commits waiting for the owner's "push". Leave them.
- **Diverged or dirty:** report it and don't touch it. Other sessions may share this folder, so never stash, reset or switch branches.

## 2. Data: get today's production backup

Claude never signs in to production, even when the owner pastes the password or PIN. Prod is not a test host, and entering credentials there is off-limits. The owner downloads the backup:

> Prod → Settings → Data → **Download backup** (the plain one, without secrets)

The app can't read `~/Downloads` (macOS privacy: Bash, Read and request_directory all get EPERM). A copy run **in the Terminal panel** works (`mcp__terminal__run_in_terminal`, cwd = repo root):

```bash
f=$(ls -t ~/Downloads/finance-backup-*.json | grep -v with-secrets | head -1) && cp -p "$f" finance_tracker/backend/tmp/ && ls -l "finance_tracker/backend/tmp/$(basename "$f")"
```

Read `exported_at` from the copy. If it isn't from today, or it is the backup `last-sync.json` already names, ask the owner in one line to download a fresh one and say "done". Wait for that, or for "skip". `backend/tmp/` is git-ignored.

## 3. Run the sync

```bash
plugins/ft-dev/skills/ft-sync/scripts/sync-from-prod.sh "finance_tracker/backend/tmp/<backup>.json"
```

It snapshots local to `backend/backups/pre-prod-sync-<stamp>.db`. Then it restores the backup into a `VACUUM INTO` copy of local, so local-only secrets survive. It does that with a server built from this checkout, on a free port in 8095–8098. It restarts that server so startup migrations run, re-exports, and compares every table on prod's columns. Only when all of them match does it swap the copy into `backend/finance-v2.db` and write `backend/tmp/last-sync.json`. Columns and tables that a newer checkout adds (prod not updated yet) are listed and allowed.

| Exit | Meaning | Do |
|---|---|---|
| 0 | synced | report the line it printed |
| 2 | not a plain backup (or one with secrets) | ask for the plain download |
| 3 | tables differ, or prod's schema is newer than this checkout | local is untouched; pull `main` (step 1) or report the tables, don't force it |
| 4 | a server holds the local DB | see below |
| 5 | probe server failed | read the log path it names |

**Exit 4.** The owner's local server has `finance-v2.db` open; the script prints its PID, command and port. Save its command and environment with `ps eww <pid>`, stop it by port (`kill $(lsof -t -iTCP:<port> -sTCP:LISTEN)`), and run `sync-from-prod.sh --swap <the printed verified copy>`. Then start it again, in the background, with the same command and environment. Only processes holding the live file are listed. Other sessions' probes use their own copies, so leave them alone.

The ft-dev guard hook doesn't prompt for this script. The owner has authorized the sync itself, and the snapshot plus the verify-before-swap check are the safety net. This is the one sanctioned write to the local database; every other check still runs on a `VACUUM INTO` copy (`ft-live-verify`). If a sync ever looks wrong, the way back is to stop the server and copy back the `pre-prod-sync-*.db` it printed.

## 4. Report, then start the task

One line, e.g. "Synced: main = origin/main (2.76.0); local data = prod backup of 14 Nov 09:12, 19/19 tables, 6,610 transactions." Then load the skills the task needs.

To test the script itself, point `FT_SYNC_LIVE` at a scratch copy. It then snapshots, verifies and swaps that copy, never the real one.
