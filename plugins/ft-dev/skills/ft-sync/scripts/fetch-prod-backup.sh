#!/usr/bin/env bash
# Downloads today's production backup (no secrets) with the sync token, so a
# sync needs nothing from the owner. Prints the saved file's path.
#
# The token and prod's address live in the macOS Keychain, put there once by
# the owner (the command prompts for the token, so it never lands in shell
# history, a file or the repo):
#
#   security add-generic-password -U -s finance-tracker-prod-sync -a https://<prod host> -w
#
# The token is an ftsy_ "sync token" from prod's Settings → Security. It
# opens GET /sync/backup.json and nothing else, and can be revoked there.
#
# Exit: 0 saved · 6 no Keychain entry (owner hasn't set it up) · 7 download
# refused or failed (token revoked, prod down) · 8 not a plain backup.
set -euo pipefail
umask 077 # the backup is the whole household's finances: readable by the owner only

REPO=$(cd "$(dirname "$0")/../../../../.." && pwd)
OUT_DIR="$REPO/finance_tracker/backend/tmp"
ITEM=finance-tracker-prod-sync

entry=$(security find-generic-password -s "$ITEM" 2>/dev/null) || {
  echo "no Keychain entry '$ITEM' — the owner sets it up once (see the top of this script)" >&2; exit 6; }
base=$(awk -F'"' '/"acct"<blob>=/{print $4}' <<<"$entry")
base=${base%/}
case "$base" in https://*) ;; *) echo "Keychain entry's account must be prod's https:// address, got '$base'" >&2; exit 6;; esac

mkdir -p "$OUT_DIR"
out="$OUT_DIR/finance-backup-$(date +%Y-%m-%d)-auto.json"
tmp="$out.part"
# The header goes in on stdin, so the token never shows in a process list.
if ! security find-generic-password -s "$ITEM" -w | sed 's/^/Authorization: Bearer /' |
    curl -sS --fail --proto '=https' --max-time 120 -H @- -o "$tmp" "$base/sync/backup.json"; then
  rm -f "$tmp"; echo "download failed — revoked token, or prod unreachable" >&2; exit 7
fi
python3 -I - "$tmp" <<'EOF' || { rm -f "$tmp"; exit 8; }
import json, sys
d = json.load(open(sys.argv[1]))
assert d.get("format") == "finance-tracker-backup" and not d.get("include_secrets"), "not a plain finance-tracker backup"
EOF
mv "$tmp" "$out"
echo "$out"
