#!/bin/sh
set -e

# Read ANTHROPIC_API_KEY from HAOS add-on options (written to /data/options.json by supervisor)
OPTIONS_FILE="/data/options.json"
if [ -f "$OPTIONS_FILE" ]; then
    ANTHROPIC_API_KEY=$(jq -r '.anthropic_api_key // ""' "$OPTIONS_FILE")
    export ANTHROPIC_API_KEY
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
