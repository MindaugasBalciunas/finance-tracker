#!/bin/bash
# Export finance-tracker SQLite database tables to CSV files

DB_PATH="${1:-./backend/finance.db}"
OUT_DIR="${2:-./exports}"

if [ ! -f "$DB_PATH" ]; then
  echo "Database not found at: $DB_PATH"
  echo "Usage: $0 [db_path] [output_dir]"
  exit 1
fi

mkdir -p "$OUT_DIR"

echo "Exporting from: $DB_PATH"
echo "Output directory: $OUT_DIR"

# Export transactions
sqlite3 -header -csv "$DB_PATH" \
  "SELECT id, date(date) as date, type, amount, category, comment, created_at, updated_at FROM transactions ORDER BY date DESC;" \
  > "$OUT_DIR/transactions.csv"
echo "  transactions.csv  ($(wc -l < "$OUT_DIR/transactions.csv") rows)"

# Export balances
sqlite3 -header -csv "$DB_PATH" \
  "SELECT id, date(date) as date, total, seb, swed, swed_etf, swed_pen, luminor, art, cash, rev_m, rev_r, rbtc, mbtc, btc_price, rev_stocks, created_at, updated_at FROM balances ORDER BY date DESC;" \
  > "$OUT_DIR/balances.csv"
echo "  balances.csv      ($(wc -l < "$OUT_DIR/balances.csv") rows)"

# Export ai_insights
sqlite3 -header -csv "$DB_PATH" \
  "SELECT id, content, created_at FROM ai_insights ORDER BY created_at DESC;" \
  > "$OUT_DIR/ai_insights.csv"
echo "  ai_insights.csv   ($(wc -l < "$OUT_DIR/ai_insights.csv") rows)"

echo ""
echo "Done. Files saved to $OUT_DIR/"
