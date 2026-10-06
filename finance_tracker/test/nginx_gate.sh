#!/usr/bin/env bash
# Integration test for the web-password / passkey gate: the real nginx.conf and
# the way run.sh writes auth.conf, in front of the real server on a fresh
# database. Needs nginx, openssl, sqlite3, curl and a built server binary.
#   test/nginx_gate.sh <path-to-server-binary>
set -euo pipefail
BIN=$(realpath "$1")
HERE=$(cd "$(dirname "$0")/.." && pwd)
T=$(mktemp -d)
trap 'kill $(cat "$T/server.pid" 2>/dev/null) 2>/dev/null; nginx -p "$T" -c "$T/main.conf" -s stop 2>/dev/null || true; rm -rf "$T"' EXIT

# Server on 8080 (nginx.conf proxies there), fresh database.
DB_PATH="$T/finance-v2.db" V1_DB_PATH=/nonexistent PORT=8080 "$BIN" > "$T/server.log" 2>&1 &
echo $! > "$T/server.pid"
for _ in $(seq 60); do curl -sf http://127.0.0.1:8080/api/health >/dev/null && break; sleep 0.5; done

# Site config as shipped, on a free port, with the password on.
mkdir -p "$T/html" "$T/logs"
echo '<!doctype html><title>app</title>' > "$T/html/index.html"
echo '{}' > "$T/html/manifest.webmanifest"
printf 'admin:%s\n' "$(openssl passwd -apr1 secret)" > "$T/htpasswd"
printf 'satisfy any;\nauth_basic "Finance Tracker";\nauth_basic_user_file %s;\nauth_request /_gate;\n' "$T/htpasswd" > "$T/auth.conf"
sed -e "s#listen 80 default_server#listen 8088#" -e "s#/etc/nginx/auth.conf#$T/auth.conf#" -e "s#/usr/share/nginx/html#$T/html#" \
  -e "s#access_log /dev/stdout#access_log $T/logs/access.log#" -e "s#error_log /dev/stderr warn#error_log $T/logs/error.log warn#" \
  "$HERE/nginx.conf" > "$T/site.conf"
cat > "$T/main.conf" <<CONF
pid $T/nginx.pid;
error_log $T/logs/error.log warn;
events {}
http {
  client_body_temp_path $T/body; proxy_temp_path $T/proxy; fastcgi_temp_path $T/fcgi; uwsgi_temp_path $T/uwsgi; scgi_temp_path $T/scgi;
  include $T/site.conf;
}
CONF
nginx -p "$T" -c "$T/main.conf"
sleep 0.5

B=http://127.0.0.1:8088
fail=0
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
expect() { # expect <want> <label> <curl args...>
  local want=$1 label=$2; shift 2
  local got; got=$(code "$@")
  if [ "$got" = "$want" ]; then echo "ok   $label ($got)"; else echo "FAIL $label: got $got, want $want"; fail=1; fi
}

echo "— no PIN, no passkey: the web password guards everything"
expect 401 "app without password" $B/
expect 200 "app with password" -u admin:secret $B/
expect 200 "icons stay public" $B/manifest.webmanifest
curl -s -D - -o /dev/null $B/ | grep -qi '^www-authenticate: basic' && echo "ok   browser is asked for the password" || { echo "FAIL no Basic challenge"; fail=1; }

echo "— PIN set, still no passkey: unchanged"
curl -sf -u admin:secret -H 'Content-Type: application/json' -d '{"pin":"1234"}' $B/api/auth/pin/setup >/dev/null
expect 401 "app without password" $B/
expect 401 "status without password" $B/api/auth/status

echo "— passkey registered: fingerprint instead of the password"
sqlite3 "$T/finance-v2.db" "INSERT INTO webauthn_credentials(name,credential,created_at) VALUES('phone',x'00','x')"
expect 200 "app without password" $B/
expect 200 "lock screen status without password" $B/api/auth/status
got=$(code -X POST $B/api/auth/passkey/login/begin)
[ "$got" != 401 ] && echo "ok   passkey login reachable ($got)" || { echo "FAIL passkey login needs the password"; fail=1; }
expect 401 "data without a session" $B/api/overview
expect 401 "PIN login still needs the password" -X POST -H 'Content-Type: application/json' -d '{"pin":"1234"}' $B/api/auth/pin/login
expect 401 "path tricks don't help" "$B/api/auth/status/../../api/overview" --path-as-is
curl -s -D - -o /dev/null -X POST $B/api/auth/pin/login | grep -qi '^www-authenticate: basic' && echo "ok   PIN login asks for the password" || { echo "FAIL PIN login gives no Basic challenge"; fail=1; }

echo "— after logging in: the session alone is enough"
curl -s -c "$T/jar" -u admin:secret -H 'Content-Type: application/json' -d '{"pin":"1234"}' $B/api/auth/pin/login >/dev/null
expect 200 "data with the session, no password" -b "$T/jar" $B/api/overview
expect 401 "a made-up session is refused" -b "ft_session=forged" $B/api/overview

[ $fail = 0 ] && echo "nginx gate: all good" || { echo "nginx gate: FAILED"; tail -20 "$T/logs/error.log"; exit 1; }
