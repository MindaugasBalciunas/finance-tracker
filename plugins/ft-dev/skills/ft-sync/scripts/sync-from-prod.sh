#!/usr/bin/env bash
# One-way sync: production → local dev database, from a production backup
# the owner downloaded (Settings → Data → Download backup, no secrets).
#
#   sync-from-prod.sh --fetch            download today's backup with the sync
#                                        token, then as below
#   sync-from-prod.sh BACKUP.json        prepare, verify, then swap into local
#   sync-from-prod.sh --swap PREPARED.db swap an already verified copy
#
# What it does, and why each step exists:
#   1. Snapshot local into backend/backups/pre-prod-sync-*.db — the way back.
#   2. Restore the backup into a VACUUM INTO copy of local (not a blank DB),
#      so local-only secrets (AI key, bank sessions) survive, using a server
#      built from the current checkout on a free probe port.
#   3. Restart it (startup migrations run, as on prod after an update), then
#      re-export and compare every table row for row with the backup. Any
#      difference stops the sync — local is left as it was.
#   4. Swap the verified copy in — only when nothing holds the local DB open.
#      If a server does, it prints the PIDs and commands and exits 4: stop
#      that server, run --swap with the printed path, start it again.
#   5. Record backend/tmp/last-sync.json (which backup, when, counts).
#
# Exit: 0 synced · 2 bad input · 3 tables differ (nothing swapped) ·
#       4 local DB in use (verified copy kept, see message) · 5 server failed.
set -euo pipefail

REPO=$(cd "$(dirname "$0")/../../../../.." && pwd)
FT="$REPO/finance_tracker"
# FT_SYNC_LIVE points it at another database (a scratch copy) to test the script.
LIVE=${FT_SYNC_LIVE:-$FT/backend/finance-v2.db}
BACKEND=$(cd "$(dirname "$LIVE")" && pwd)
BACKUPS="$BACKEND/backups"
MARK="$BACKEND/tmp/last-sync.json"

swap() {
  local prepared=$1
  [ -f "$prepared" ] || { echo "no prepared copy at $prepared" >&2; exit 2; }
  local users
  users=$(lsof -t "$LIVE" "$LIVE-wal" 2>/dev/null | sort -u || true)
  if [ -n "$users" ]; then
    echo "LOCAL DB IN USE — not swapped. Holding $LIVE:"
    for p in $users; do echo "  PID $p: $(ps -o command= -p "$p")"; lsof -a -p "$p" -iTCP -sTCP:LISTEN -n -P 2>/dev/null | awk 'NR>1{print "    listening on "$9}'; done
    echo "Stop it (kill by port), run: $0 --swap $prepared"
    echo "then start it again with the same command and environment (ps eww <pid> shows it)."
    exit 4
  fi
  sqlite3 "$prepared" "VACUUM INTO '$LIVE.new'"
  rm -f "$LIVE-wal" "$LIVE-shm"
  mv "$LIVE.new" "$LIVE"
  echo "swapped: $LIVE ($(sqlite3 "$LIVE" 'SELECT count(*) FROM transactions') transactions, schema $(sqlite3 "$LIVE" 'PRAGMA user_version'))"
}

if [ "${1:-}" = "--swap" ]; then
  swap "${2:?--swap needs the prepared .db path}"
  [ -f "${2%.db}.mark.json" ] && cp "${2%.db}.mark.json" "$MARK"
  rm -f "$2" "${2%.db}.mark.json"
  exit 0
fi

if [ "${1:-}" = "--fetch" ]; then
  # Download today's backup with the sync token (see fetch-prod-backup.sh);
  # its exit code (6–8) says why when it can't.
  fetched=$("$(dirname "$0")/fetch-prod-backup.sh") || exit $?
  set -- "$fetched"
  echo "fetched: $1"
fi
BACKUP=${1:?usage: sync-from-prod.sh --fetch | BACKUP.json | --swap PREPARED.db}
[ -f "$BACKUP" ] || { echo "no such file: $BACKUP" >&2; exit 2; }
python3 -I - "$BACKUP" <<'EOF' || exit 2
import json, sys
d = json.load(open(sys.argv[1]))
assert d.get("format") == "finance-tracker-backup", "not a finance-tracker backup"
assert not d.get("include_secrets"), "a backup WITH secrets — use the plain one"
print(f"prod backup: exported {d['exported_at']} · schema {d['schema']} · {d['counts'].get('transactions', 0)} transactions")
EOF

WORK=$(mktemp -d "${TMPDIR:-/tmp}/ft-sync.XXXXXX")
STAMP=$(date +%Y%m%d-%H%M%S)
mkdir -p "$BACKUPS" "$BACKEND/tmp"

# 1. the way back
sqlite3 "$LIVE" "VACUUM INTO '$BACKUPS/pre-prod-sync-$STAMP.db'"
echo "snapshot: $BACKUPS/pre-prod-sync-$STAMP.db"

# 2. restore into a copy of local, on a server built from this checkout
sqlite3 "$LIVE" "VACUUM INTO '$WORK/sync.db'"
(cd "$FT/server" && go build -o "$WORK/server" ./cmd/server)
PORT=""
for p in 8095 8096 8097 8098; do lsof -iTCP:$p -sTCP:LISTEN -n -P >/dev/null 2>&1 || { PORT=$p; break; }; done
[ -n "$PORT" ] || { echo "no free probe port in 8095–8098" >&2; exit 5; }
PID=""
start() {
  DB_PATH="$WORK/sync.db" V1_DB_PATH=/nonexistent PORT=$PORT "$WORK/server" >>"$WORK/server.log" 2>&1 &
  PID=$!
  for _ in $(seq 1 60); do curl -sf "http://127.0.0.1:$PORT/api/health" >/dev/null && return 0; sleep 0.5; done
  echo "probe server did not start — $WORK/server.log" >&2; exit 5
}
stop() { [ -n "$PID" ] && kill "$PID" 2>/dev/null && wait "$PID" 2>/dev/null; PID=""; }
trap stop EXIT
# Local has no app lock; if it ever gets one, the restore below answers 401
# and the sync stops here rather than clearing the lock on the copy (which
# would then be swapped in).
start
curl -sf -X POST "http://127.0.0.1:$PORT/api/import/backup?confirm=replace" -H 'Content-Type: application/json' --data-binary @"$BACKUP" >"$WORK/restore.json" \
  || { echo "restore failed: $(cat "$WORK/restore.json" 2>/dev/null)" >&2; exit 5; }

# 3. restart, re-export, compare every table
stop; start
curl -sf "http://127.0.0.1:$PORT/api/export/backup.json?purpose=export" >"$WORK/after.json"
stop
python3 -I - "$BACKUP" "$WORK/after.json" "$MARK.tmp" <<'EOF' || exit 3
import json, sys, datetime
a, b = (json.load(open(p)) for p in sys.argv[1:3])
# Tables are {columns, rows}: zip them, or the diff compares nothing. Compare
# on prod's columns only — a newer checkout's migrations add columns and
# tables (schema 19 added transactions.recurring_id) that prod hasn't got yet.
def rows(t, cols):
    return sorted(json.dumps({c: r.get(c) for c in cols}, sort_keys=True) for r in (dict(zip(t["columns"], x)) for x in t["rows"]))
bad, newer = [], []
for name in sorted(set(a["tables"]) | set(b["tables"])):
    x, y = a["tables"].get(name), b["tables"].get(name)
    if x is None:
        newer.append(f"{name} (new table)")
    elif y is None:
        bad.append(f"{name}: in prod, missing after restore")
    else:
        if set(y["columns"]) - set(x["columns"]):
            newer.append(f"{name}.{', '.join(sorted(set(y['columns']) - set(x['columns'])))}")
        if rows(x, x["columns"]) != rows(y, x["columns"]):
            bad.append(f"{name}: {len(x['rows'])} prod rows vs {len(y['rows'])} after restore")
if b["schema"] < a["schema"]:
    bad.append(f"prod is on schema {a['schema']}, this checkout only {b['schema']} — pull main first")
if newer:
    print("new in this checkout (not on prod yet): " + "; ".join(newer))
if bad:
    print("TABLES DIFFER — local left as it was:\n  " + "\n  ".join(bad))
    sys.exit(1)
print(f"verified: {len(a['tables'])}/{len(a['tables'])} tables identical, schema {a['schema']} → {b['schema']}")
json.dump({"backup": sys.argv[1].rsplit("/", 1)[-1], "exported_at": a["exported_at"], "prod_schema": a["schema"],
           "local_schema": b["schema"], "transactions": a["counts"].get("transactions", 0),
           "synced_at": datetime.datetime.now().astimezone().isoformat(timespec="seconds")}, open(sys.argv[3], "w"), indent=1)
EOF
# Keep the verified copy outside the temp dir so --swap can find it later.
PREP="$BACKEND/tmp/prod-sync-$STAMP.db"
sqlite3 "$WORK/sync.db" "VACUUM INTO '$PREP'"
mv "$MARK.tmp" "${PREP%.db}.mark.json"
echo "verified copy: $PREP"
rm -rf "$WORK"

# 4–5. swap and record
swap "$PREP"
cp "${PREP%.db}.mark.json" "$MARK"
rm -f "$PREP" "${PREP%.db}.mark.json"
echo "synced from $(basename "$BACKUP")"
