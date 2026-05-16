#!/bin/sh
set -e

# Read options from HAOS supervisor (written to /data/options.json)
OPTIONS_FILE="/data/options.json"
if [ -f "$OPTIONS_FILE" ]; then
    ANTHROPIC_API_KEY=$(jq -r '.anthropic_api_key // ""' "$OPTIONS_FILE")
    export ANTHROPIC_API_KEY

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
    printf 'auth_basic "Finance Tracker";\nauth_basic_user_file /etc/nginx/.htpasswd;\n' \
        > /etc/nginx/auth.conf
    echo "[finance-tracker] Basic auth enabled for user: $AUTH_USER"
else
    printf '# auth disabled\n' > /etc/nginx/auth.conf
    echo "[finance-tracker] Basic auth disabled (no auth_password set)"
fi

# Start Go backend in background
export DB_PATH="/data/finance.db"
export PORT="8080"

echo "[finance-tracker] Starting backend on port $PORT, DB at $DB_PATH"
/app/finance-tracker &

# Give backend a moment to initialize before nginx starts accepting requests
sleep 1

echo "[finance-tracker] Starting nginx"
# Run nginx in foreground so the container stays alive
nginx -g "daemon off;"
