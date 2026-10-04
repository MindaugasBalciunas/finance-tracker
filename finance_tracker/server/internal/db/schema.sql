-- Finance Tracker v2 schema.
--
-- Conventions
--   * Money is INTEGER cents in EUR. Never floats.
--   * Dates are TEXT 'YYYY-MM-DD'; timestamps are TEXT RFC3339 (UTC).
--   * Every own place money can sit — bank account, broker, pension, wallet,
--     house, car, mortgage — is one row in accounts. Net worth is the sum of
--     the latest balance of every non-archived account; loans carry negative
--     balances.

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE accounts (
    id          TEXT PRIMARY KEY,                -- stable slug: swed, seb, ibkr, house…
    name        TEXT NOT NULL,
    institution TEXT NOT NULL DEFAULT '',
    kind        TEXT NOT NULL,                   -- see ledger.AccountKinds
    currency    TEXT NOT NULL DEFAULT 'EUR',
    liquid      INTEGER NOT NULL DEFAULT 1,      -- counts toward liquid net worth / runway
    archived    INTEGER NOT NULL DEFAULT 0,
    sort        INTEGER NOT NULL DEFAULT 0,
    notes       TEXT NOT NULL DEFAULT '',
    details     TEXT NOT NULL DEFAULT '{}',      -- kind-specific JSON (loan terms, asset purchase…)
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

-- One value per account per day. The latest value on or before a date is
-- that account's balance on that date.
CREATE TABLE balances (
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    date       TEXT NOT NULL,
    value      INTEGER NOT NULL,                 -- cents EUR
    quantity   REAL,                             -- units held (BTC), when the account is priced
    price      INTEGER,                          -- cents EUR per unit
    source     TEXT NOT NULL DEFAULT 'manual',   -- manual | bank | import | computed
    updated_at TEXT NOT NULL,
    PRIMARY KEY (account_id, date)
);
CREATE INDEX idx_balances_date ON balances(date);

CREATE TABLE categories (
    id        TEXT PRIMARY KEY,                  -- 'food' or 'food.groceries'
    parent    TEXT REFERENCES categories(id),
    name      TEXT NOT NULL,
    kind      TEXT NOT NULL,                     -- income | expense | transfer
    essential INTEGER NOT NULL DEFAULT 0,        -- needs vs wants (expense only)
    color     TEXT NOT NULL DEFAULT '',
    sort      INTEGER NOT NULL DEFAULT 0,
    archived  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE transactions (
    id            INTEGER PRIMARY KEY,
    date          TEXT NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('income','expense','transfer')),
    amount        INTEGER NOT NULL CHECK (amount > 0),  -- cents, always positive; kind gives the sign
    account_id    TEXT REFERENCES accounts(id),         -- expense: paid from · income: paid into · transfer: from
    to_account_id TEXT REFERENCES accounts(id),         -- transfer: to
    category      TEXT NOT NULL REFERENCES categories(id),
    merchant      TEXT NOT NULL DEFAULT '',
    note          TEXT NOT NULL DEFAULT '',
    tags          TEXT NOT NULL DEFAULT '',             -- ',kids,trip:rome,' — leading/trailing comma for LIKE
    external_id   TEXT,                                 -- bank provider id
    split_of      INTEGER REFERENCES transactions(id) ON DELETE CASCADE,
    source        TEXT NOT NULL DEFAULT 'manual',       -- manual | bank | receipt | import
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);
CREATE INDEX idx_tx_date ON transactions(date);
CREATE INDEX idx_tx_category ON transactions(category);
CREATE INDEX idx_tx_merchant ON transactions(merchant);
CREATE UNIQUE INDEX idx_tx_external ON transactions(external_id) WHERE external_id IS NOT NULL AND external_id != '';

-- Categorisation rules: first enabled match by priority wins for category /
-- merchant; tags accumulate across every match.
CREATE TABLE rules (
    id             INTEGER PRIMARY KEY,
    priority       INTEGER NOT NULL DEFAULT 100,
    pattern        TEXT NOT NULL DEFAULT '',     -- substring of merchant+note, case/diacritic-insensitive; '^' anchors
    when_category  TEXT NOT NULL DEFAULT '',     -- only rows already in this category (or its children)
    when_kind      TEXT NOT NULL DEFAULT '',
    set_category   TEXT NOT NULL DEFAULT '',
    set_merchant   TEXT NOT NULL DEFAULT '',
    add_tags       TEXT NOT NULL DEFAULT '',
    enabled        INTEGER NOT NULL DEFAULT 1,
    created_at     TEXT NOT NULL
);

-- Plan: budgets are monthly envelopes matched by category (incl. children) or tag.
CREATE TABLE budgets (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL,                   -- fixed | spending | saving
    category    TEXT NOT NULL DEFAULT '',
    tag         TEXT NOT NULL DEFAULT '',
    period      TEXT NOT NULL DEFAULT 'monthly', -- monthly | yearly
    fund        INTEGER NOT NULL DEFAULT 0,      -- unspent rolls over (sinking fund)
    start_month TEXT NOT NULL DEFAULT '',
    archived    INTEGER NOT NULL DEFAULT 0,
    sort        INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL
);
CREATE TABLE budget_amounts (
    budget_id  INTEGER NOT NULL REFERENCES budgets(id) ON DELETE CASCADE,
    from_month TEXT NOT NULL DEFAULT '',         -- '' = since forever
    amount     INTEGER NOT NULL,
    PRIMARY KEY (budget_id, from_month)
);

-- Investments.
CREATE TABLE trades (
    id         INTEGER PRIMARY KEY,
    date       TEXT NOT NULL,
    account_id TEXT REFERENCES accounts(id),
    action     TEXT NOT NULL CHECK (action IN ('buy','sell')),
    ticker     TEXT NOT NULL,
    shares     REAL NOT NULL,
    price      REAL NOT NULL,                    -- per share, native currency
    currency   TEXT NOT NULL DEFAULT 'USD',
    notes      TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE TABLE price_cache (
    ticker     TEXT PRIMARY KEY,
    payload    TEXT NOT NULL,
    fetched_at TEXT NOT NULL
);

-- Open banking (Enable Banking).
CREATE TABLE bank_connections (
    id            INTEGER PRIMARY KEY,
    aspsp_name    TEXT NOT NULL,
    aspsp_country TEXT NOT NULL DEFAULT 'LT',
    session_id    TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'pending', -- pending | authorized | expired | revoked
    valid_until   TEXT NOT NULL DEFAULT '',
    auth_state    TEXT NOT NULL DEFAULT '',
    auth_state_at TEXT NOT NULL DEFAULT '',
    last_error    TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);
CREATE TABLE bank_accounts (
    id                   INTEGER PRIMARY KEY,
    connection_id        INTEGER NOT NULL REFERENCES bank_connections(id) ON DELETE CASCADE,
    identification_hash  TEXT NOT NULL DEFAULT '',
    uid                  TEXT NOT NULL DEFAULT '',
    iban                 TEXT NOT NULL DEFAULT '',
    display_name         TEXT NOT NULL DEFAULT '',
    account_id           TEXT NOT NULL DEFAULT '',   -- ledger account this feeds; '' = not synced
    last_synced_at       TEXT NOT NULL DEFAULT '',
    last_tx_date         TEXT NOT NULL DEFAULT '',
    bank_balance         INTEGER,
    bank_balance_currency TEXT NOT NULL DEFAULT '',
    bank_balance_type    TEXT NOT NULL DEFAULT '',
    bank_balance_date    TEXT NOT NULL DEFAULT '',
    bank_balance_fetched TEXT NOT NULL DEFAULT '',
    balance_applied_through TEXT NOT NULL DEFAULT '',
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL
);
-- The inbox: bank rows waiting between "fetched" and "in the ledger".
CREATE TABLE bank_inbox (
    id            INTEGER PRIMARY KEY,
    bank_account_id INTEGER REFERENCES bank_accounts(id) ON DELETE SET NULL,
    external_id   TEXT NOT NULL UNIQUE,
    raw           TEXT NOT NULL DEFAULT '',
    raw_payee     TEXT NOT NULL DEFAULT '',
    raw_details   TEXT NOT NULL DEFAULT '',
    raw_currency  TEXT NOT NULL DEFAULT '',
    booking_date  TEXT NOT NULL DEFAULT '',
    pending       INTEGER NOT NULL DEFAULT 0,     -- card reservation, not booked yet
    -- proposal (editable)
    date          TEXT NOT NULL,
    kind          TEXT NOT NULL,
    amount        INTEGER NOT NULL,
    account_id    TEXT,
    to_account_id TEXT,
    category      TEXT NOT NULL DEFAULT '',
    merchant      TEXT NOT NULL DEFAULT '',
    note          TEXT NOT NULL DEFAULT '',
    tags          TEXT NOT NULL DEFAULT '',
    edited        INTEGER NOT NULL DEFAULT 0,
    guessed       INTEGER NOT NULL DEFAULT 0,     -- category is a placeholder, not a reading
    -- review state
    verdict       TEXT NOT NULL DEFAULT 'new',    -- new | duplicate | internal | needs_review | pending
    verdict_note  TEXT NOT NULL DEFAULT '',
    matched_tx_id INTEGER,
    state         TEXT NOT NULL DEFAULT 'open',   -- open | imported | dismissed | superseded
    imported_tx_id INTEGER,
    superseded_by INTEGER,
    first_seen_at TEXT NOT NULL,
    last_seen_at  TEXT NOT NULL
);
CREATE INDEX idx_inbox_state ON bank_inbox(state);

-- Security. Sessions are stored as SHA-256 of the cookie value.
CREATE TABLE auth_sessions (
    token_hash TEXT PRIMARY KEY,
    expires_at TEXT NOT NULL
);
CREATE TABLE webauthn_credentials (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL DEFAULT '',
    credential BLOB NOT NULL,
    created_at TEXT NOT NULL
);

-- AI.
CREATE TABLE ai_messages (
    id         INTEGER PRIMARY KEY,
    role       TEXT NOT NULL,
    content    TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE TABLE ai_spend (
    id            INTEGER PRIMARY KEY,
    kind          TEXT NOT NULL DEFAULT 'chat',
    model         TEXT NOT NULL DEFAULT '',
    cost_usd      REAL NOT NULL DEFAULT 0,
    input_tokens  INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    estimated     INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL
);
CREATE TABLE ai_topups (
    id          INTEGER PRIMARY KEY,
    amount_usd  REAL NOT NULL,
    note        TEXT NOT NULL DEFAULT '',
    occurred_on TEXT NOT NULL,
    created_at  TEXT NOT NULL
);
