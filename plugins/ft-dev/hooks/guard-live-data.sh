#!/usr/bin/env bash
# PreToolUse guard for Bash: asks the owner before a command that could change
# live finance data — mutating HTTP calls at the local instance or production,
# SQL writes or file operations on the local databases. Probe copies in a
# scratch directory on other ports pass untouched.
set -uo pipefail

cmd=$(jq -r '.tool_input.command // empty' 2>/dev/null)
[ -z "$cmd" ] && exit 0

ask() {
  jq -n --arg r "$1" '{hookSpecificOutput: {hookEventName: "PreToolUse", permissionDecision: "ask", permissionDecisionReason: $r}}'
  exit 0
}

live_host='(localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1\]):(8080|5175|8099)|duckdns\.org|\.ts\.net|homeassistant'
mutating='(-X|--request)[ =]*["'\'']?(POST|PUT|PATCH|DELETE)|(^|[ ])(-[a-zA-Z]*[dFT]|--data[a-z-]*|--form|--upload-file|--json)([ =]|$)'

if grep -Eq '(^|[^a-z])(curl|wget|http|xh)([^a-z]|$)' <<<"$cmd" && grep -Eq "$live_host" <<<"$cmd" && grep -Eq -- "$mutating" <<<"$cmd"; then
  ask "ft-dev guard: this sends a write to a live finance-tracker instance (local :8080/:5175 or production). Live checks belong on an isolated probe copy (see the ft-live-verify skill). Approve only if the owner asked for exactly this call."
fi

live_db='finance_tracker/backend/(finance|finance-v2)\.db|/data/finance(-v2)?\.db'

if grep -Eq 'sqlite3' <<<"$cmd" && grep -Eq "$live_db" <<<"$cmd" &&
   grep -Eiq '(^|[^a-z_])(update|delete|insert|replace|drop|alter|create|attach|pragma[^;]*=|\.restore|\.import)([^a-z_]|$)' <<<"$cmd"; then
  ask "ft-dev guard: this may write to the owner's local finance database. Make a VACUUM INTO copy in the scratchpad and change that instead (see ft-live-verify). Approve only if the owner asked for this exact change."
fi

if grep -Eq '(^|[;&|[:space:]])(rm|mv|cp|truncate|>)[[:space:]]' <<<"$cmd" && grep -Eq "$live_db|finance_tracker/backend/backups/" <<<"$cmd" &&
   ! grep -Eq "VACUUM INTO" <<<"$cmd"; then
  ask "ft-dev guard: this moves, overwrites or deletes a live finance database or its backups. Approve only if the owner asked for it."
fi

exit 0
