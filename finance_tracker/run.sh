#!/bin/sh
set -e

# Read options from HAOS supervisor (written to /data/options.json)
OPTIONS_FILE="/data/options.json"
if [ -f "$OPTIONS_FILE" ]; then
    ANTHROPIC_API_KEY=$(jq -r '.anthropic_api_key // ""' "$OPTIONS_FILE")
    export ANTHROPIC_API_KEY

    # nexos.ai gateway seed: applied at boot only when the database has no
    # key, so a wiped/reinstalled instance comes back with AI working.
    NEXOS_API_KEY=$(jq -r '.nexos_api_key // ""' "$OPTIONS_FILE")
    NEXOS_MODEL=$(jq -r '.nexos_model // ""' "$OPTIONS_FILE")
    NEXOS_GATEWAY_URL=$(jq -r '.nexos_gateway_url // ""' "$OPTIONS_FILE")
    export NEXOS_API_KEY NEXOS_MODEL NEXOS_GATEWAY_URL

    AUTH_PASSWORD=$(jq -r '.auth_password // ""' "$OPTIONS_FILE")
    AUTH_USER=$(jq -r '.auth_user // "admin"' "$OPTIONS_FILE")
else
    AUTH_PASSWORD=""
    AUTH_USER="admin"
fi

# Generate nginx basic-auth config
if [ -n "$AUTH_PASSWORD" ]; then
    printf '%s:%s\n' "$AUTH_USER" "$(openssl passwd -apr1 "$AUTH_PASSWORD")" \
        > /etc/nginx/.htpasswd
    # Either check lets a request in: the web password, or the app's gate —
    # which says yes only once a passkey is registered (app shell, passkey
    # login, and anything with a live session). PIN login keeps the password.
    printf 'satisfy any;\nauth_basic "Finance Tracker";\nauth_basic_user_file /etc/nginx/.htpasswd;\nauth_request /_gate;\n' \
        > /etc/nginx/auth.conf
    echo "[finance-tracker] Basic auth enabled for user: $AUTH_USER (skipped after a passkey login)"
else
    printf '# auth disabled\n' > /etc/nginx/auth.conf
    echo "[finance-tracker] Basic auth disabled (no auth_password set)"
fi

# v2 keeps its own database. On the very first start it converts the v1
# database next to it (/data/finance.db), verifies every row and balance,
# and refuses to start if anything does not match — the v1 file is only read,
# so reinstalling the previous add-on version is a complete rollback.
export DB_PATH="/data/finance-v2.db"
export V1_DB_PATH="/data/finance.db"
export PORT="8080"
export TZ="${TZ:-Europe/Vilnius}"

echo "[finance-tracker] Starting backend on port $PORT, DB at $DB_PATH"
/app/finance-tracker &
BACKEND_PID=$!

# Wait for the backend to actually answer (startup runs migrations, which can
# take a while) instead of hoping one second is enough.
i=0
until wget -q -O /dev/null "http://127.0.0.1:8080/api/health" 2>/dev/null; do
    if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
        echo "[finance-tracker] Backend exited during startup — aborting"
        exit 1
    fi
    i=$((i + 1))
    if [ "$i" -gt 600 ]; then
        echo "[finance-tracker] Backend did not become healthy in 5 minutes — aborting"
        exit 1
    fi
    sleep 0.5
done

echo "[finance-tracker] Starting nginx"
nginx -g "daemon off;" &
NGINX_PID=$!

# Supervise BOTH processes: if either dies, stop the container so the
# supervisor restarts it. Previously a crashed backend left nginx serving
# 502s while the container looked healthy.
trap 'kill -TERM "$BACKEND_PID" "$NGINX_PID" 2>/dev/null' TERM INT
while kill -0 "$BACKEND_PID" 2>/dev/null && kill -0 "$NGINX_PID" 2>/dev/null; do
    sleep 5 &
    wait $!
done

echo "[finance-tracker] A process exited — shutting down container"
kill -TERM "$BACKEND_PID" "$NGINX_PID" 2>/dev/null
wait
exit 1
