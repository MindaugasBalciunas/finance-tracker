package testutil

// V1Schema is the v1 (1.79) database layout, for conversion tests.
const V1Schema = `
CREATE TABLE transactions (id integer PRIMARY KEY AUTOINCREMENT, date datetime NOT NULL, type text NOT NULL, amount real NOT NULL, comment text, category text,
 labels text NOT NULL DEFAULT "", source_account text DEFAULT "", debit_account text NOT NULL DEFAULT "", credit_account text NOT NULL DEFAULT "",
 external_id text DEFAULT "", split_of integer NOT NULL DEFAULT 0, created_at datetime, updated_at datetime);
CREATE TABLE balances (id integer PRIMARY KEY AUTOINCREMENT, date datetime NOT NULL, total real, seb real, swed real, swed_etf real, seb_pen real, luminor real,
 art real, cash real, rev_m real, rev_r real, rbtc real, mbtc real, btc_price real, rev_stocks real, ibkr_stocks real, extra text NOT NULL DEFAULT "{}", created_at datetime, updated_at datetime);
CREATE TABLE accounts (id integer PRIMARY KEY AUTOINCREMENT, key text NOT NULL, label text NOT NULL, "group" text NOT NULL DEFAULT "other", institution text NOT NULL DEFAULT "",
 builtin numeric NOT NULL DEFAULT false, archived numeric NOT NULL DEFAULT false, sort_order integer NOT NULL DEFAULT 0, created_at datetime, updated_at datetime);
CREATE TABLE stock_trades (id integer PRIMARY KEY AUTOINCREMENT, date datetime NOT NULL, action text NOT NULL, ticker text NOT NULL, shares real NOT NULL,
 price_per_share real NOT NULL, currency text DEFAULT "USD", source text NOT NULL DEFAULT "Revolut", notes text, created_at datetime, updated_at datetime);
CREATE TABLE assets (id integer PRIMARY KEY AUTOINCREMENT, name text NOT NULL, type text NOT NULL DEFAULT "other", purchase_date datetime, purchase_price real NOT NULL,
 current_value real, valuation_date datetime, notes text, loan_remaining real, loan_remaining_date datetime, loan_rate text, loan_account text, loan_paid_off_date datetime,
 loan_margin real, loan_label text, loan_base_rate real, loan_rate_reset_date datetime, loan_monthly_payment real, created_at datetime, updated_at datetime);
CREATE TABLE budgets (id integer PRIMARY KEY AUTOINCREMENT, name text NOT NULL, kind text NOT NULL, label text NOT NULL DEFAULT "", category text NOT NULL DEFAULT "",
 amount real NOT NULL, period text NOT NULL DEFAULT "monthly", fund numeric NOT NULL DEFAULT false, start_month text NOT NULL DEFAULT "", created_at datetime, updated_at datetime);
CREATE TABLE budget_amounts (id integer PRIMARY KEY AUTOINCREMENT, budget_id integer NOT NULL, from_month text NOT NULL DEFAULT "", amount real NOT NULL);
CREATE TABLE budget_settings (id integer PRIMARY KEY AUTOINCREMENT, income_mode text NOT NULL DEFAULT "median", manual_income real NOT NULL DEFAULT 0, gross_salary real NOT NULL DEFAULT 0, monthly_deductions real NOT NULL DEFAULT 0, updated_at datetime);
CREATE TABLE label_rules (id integer PRIMARY KEY AUTOINCREMENT, label text NOT NULL, category text NOT NULL DEFAULT "", comment_match text NOT NULL DEFAULT "", created_at datetime);
CREATE TABLE ai_settings (id integer PRIMARY KEY AUTOINCREMENT, gateway_url text NOT NULL DEFAULT "", api_key text NOT NULL DEFAULT "", model text NOT NULL DEFAULT "", provider text NOT NULL DEFAULT "", disabled numeric NOT NULL DEFAULT false, updated_at datetime);
CREATE TABLE ai_contexts (id integer PRIMARY KEY AUTOINCREMENT, content text NOT NULL DEFAULT "", updated_at datetime);
CREATE TABLE ai_chat_messages (id integer PRIMARY KEY AUTOINCREMENT, role text NOT NULL, content text NOT NULL, created_at datetime);
CREATE TABLE ai_spends (id integer PRIMARY KEY AUTOINCREMENT, kind text NOT NULL DEFAULT "other", model text NOT NULL DEFAULT "", provider text NOT NULL DEFAULT "", cost_usd real NOT NULL DEFAULT 0, input_tokens integer NOT NULL DEFAULT 0, output_tokens integer NOT NULL DEFAULT 0, estimated numeric NOT NULL DEFAULT false, created_at datetime);
CREATE TABLE ai_top_ups (id integer PRIMARY KEY AUTOINCREMENT, amount_usd real NOT NULL, note text NOT NULL DEFAULT "", occurred_on datetime, created_at datetime);
CREATE TABLE auth_settings (id integer PRIMARY KEY AUTOINCREMENT, pin_hash text, enabled numeric, api_token_hash text, api_token_rw_hash text, updated_at datetime);
CREATE TABLE webauthn_credentials (id integer PRIMARY KEY AUTOINCREMENT, name text, credential blob, created_at datetime);
CREATE TABLE bank_settings (id integer PRIMARY KEY AUTOINCREMENT, application_id text NOT NULL DEFAULT "", private_key_pem text NOT NULL DEFAULT "", environment text NOT NULL DEFAULT "sandbox", redirect_url text NOT NULL DEFAULT "", updated_at datetime);
CREATE TABLE bank_connections (id integer PRIMARY KEY AUTOINCREMENT, aspsp_name text NOT NULL DEFAULT "", aspsp_country text NOT NULL DEFAULT "LT", session_id text NOT NULL DEFAULT "", status text NOT NULL DEFAULT "pending", valid_until datetime, auth_state text DEFAULT "", auth_state_at datetime, last_error text NOT NULL DEFAULT "", created_at datetime, updated_at datetime);
CREATE TABLE bank_account_links (id integer PRIMARY KEY AUTOINCREMENT, connection_id integer, identification_hash text, uid text, iban text, display_name text, account_key text NOT NULL DEFAULT "", last_synced_at datetime, last_tx_date datetime, bank_balance real, bank_balance_currency text NOT NULL DEFAULT "", bank_balance_type text NOT NULL DEFAULT "", bank_balance_date datetime, bank_balance_fetched datetime, balance_applied_through datetime, created_at datetime, updated_at datetime);
CREATE TABLE bank_staged_txes (id integer PRIMARY KEY AUTOINCREMENT, link_id integer, external_id text, raw text, raw_payee text, raw_details text, raw_amount real, raw_dk text, raw_currency text, booking_date datetime, amount real, date datetime, type text, category text, comment text, labels text NOT NULL DEFAULT "", debit_account text NOT NULL DEFAULT "", credit_account text NOT NULL DEFAULT "", enrich_note text NOT NULL DEFAULT "", edited numeric NOT NULL DEFAULT false, pending numeric NOT NULL DEFAULT false, superseded_by integer, verdict text, verdict_note text NOT NULL DEFAULT "", matched_tx_id integer, state text, imported_tx_id integer, first_seen_at datetime, last_seen_at datetime, created_at datetime, updated_at datetime);
`

