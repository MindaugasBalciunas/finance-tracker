package bank

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"ft/internal/db"
	"ft/internal/ledger"
	"ft/internal/money"
)

// Settings are the Enable Banking application credentials.
type Settings struct {
	ApplicationID string   `json:"application_id"`
	PrivateKeyPEM string   `json:"private_key_pem"`
	Environment   string   `json:"environment"`
	RedirectURL   string   `json:"redirect_url"`
	OwnerNames    []string `json:"owner_names"`
	ConsentDays   int      `json:"consent_days"`
}

func (s Settings) Configured() bool {
	return strings.TrimSpace(s.ApplicationID) != "" && strings.TrimSpace(s.PrivateKeyPEM) != ""
}

var defaultOwnerNames = []string{"MINDAUGAS BALCIUNAS", "MINDAUGAS BALČIŪNAS", "BALCIUNAS MINDAUGAS", "BALČIŪNAS MINDAUGAS"}

func LoadSettings(q interface{ QueryRow(string, ...any) *sql.Row }) Settings {
	s := Settings{Environment: "production", ConsentDays: 180}
	var raw string
	if q.QueryRow(`SELECT value FROM settings WHERE key='bank'`).Scan(&raw) == nil {
		json.Unmarshal([]byte(raw), &s)
	}
	if len(s.OwnerNames) == 0 {
		s.OwnerNames = defaultOwnerNames
	}
	if s.ConsentDays <= 0 {
		s.ConsentDays = 180
	}
	return s
}

func SaveSettings(e interface {
	Exec(string, ...any) (sql.Result, error)
}, s Settings) error {
	raw, _ := json.Marshal(s)
	_, err := e.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('bank',?)`, string(raw))
	return err
}

type Connection struct {
	ID          int64          `json:"id"`
	ASPSPName   string         `json:"aspsp_name"`
	Country     string         `json:"aspsp_country"`
	SessionID   string         `json:"-"`
	Status      string         `json:"status"`
	ValidUntil  string         `json:"valid_until"`
	AuthState   string         `json:"-"`
	LastError   string         `json:"last_error,omitempty"`
	DaysLeft    int            `json:"days_left"`
	Accounts    []BankAccount  `json:"accounts"`
}

// Effective reports a consent past its validity as expired.
func (c *Connection) Effective() string {
	if c.Status == "authorized" && c.ValidUntil != "" {
		if t, err := time.Parse(time.RFC3339, c.ValidUntil); err == nil && time.Now().After(t) {
			return "expired"
		}
	}
	return c.Status
}

type BankAccount struct {
	ID                 int64        `json:"id"`
	ConnectionID       int64        `json:"connection_id"`
	IdentificationHash string       `json:"-"`
	UID                string       `json:"-"`
	IBAN               string       `json:"iban"`
	DisplayName        string       `json:"display_name"`
	AccountID          string       `json:"account_id"`
	LastSyncedAt       string       `json:"last_synced_at"`
	LastTxDate         string       `json:"last_tx_date"`
	BankBalance        *money.Cents `json:"bank_balance"`
	BalanceCurrency    string       `json:"bank_balance_currency"`
	BalanceType        string       `json:"-"`
	BalanceDate        string       `json:"bank_balance_date"`
	BalanceFetched     string       `json:"bank_balance_fetched"`
	AppliedThrough     string       `json:"-"`
	Open               int          `json:"open"`
}

func listConnections(q *sql.DB) ([]Connection, error) {
	rows, err := q.Query(`SELECT id,aspsp_name,aspsp_country,session_id,status,valid_until,auth_state,last_error FROM bank_connections ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Connection
	for rows.Next() {
		var c Connection
		rows.Scan(&c.ID, &c.ASPSPName, &c.Country, &c.SessionID, &c.Status, &c.ValidUntil, &c.AuthState, &c.LastError)
		c.Status = c.Effective()
		if t, err := time.Parse(time.RFC3339, c.ValidUntil); err == nil {
			c.DaysLeft = int(time.Until(t).Hours() / 24)
		}
		c.Accounts = []BankAccount{}
		out = append(out, c)
	}
	return out, nil
}

func getConnection(q querier, id int64) (Connection, error) {
	var c Connection
	err := q.QueryRow(`SELECT id,aspsp_name,aspsp_country,session_id,status,valid_until,auth_state,last_error FROM bank_connections WHERE id=?`, id).
		Scan(&c.ID, &c.ASPSPName, &c.Country, &c.SessionID, &c.Status, &c.ValidUntil, &c.AuthState, &c.LastError)
	return c, err
}

func saveConnection(e execer, c *Connection) error {
	now := db.Now()
	if c.ID == 0 {
		res, err := e.Exec(`INSERT INTO bank_connections(aspsp_name,aspsp_country,session_id,status,valid_until,auth_state,auth_state_at,last_error,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?)`, c.ASPSPName, c.Country, c.SessionID, c.Status, c.ValidUntil, c.AuthState, now, c.LastError, now, now)
		if err != nil {
			return err
		}
		c.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := e.Exec(`UPDATE bank_connections SET aspsp_name=?,aspsp_country=?,session_id=?,status=?,valid_until=?,auth_state=?,last_error=?,updated_at=? WHERE id=?`,
		c.ASPSPName, c.Country, c.SessionID, c.Status, c.ValidUntil, c.AuthState, c.LastError, now, c.ID)
	return err
}

const bankAccountCols = `id,connection_id,identification_hash,uid,iban,display_name,account_id,last_synced_at,last_tx_date,bank_balance,
	bank_balance_currency,bank_balance_type,bank_balance_date,bank_balance_fetched,balance_applied_through`

func scanBankAccount(s interface{ Scan(...any) error }) (BankAccount, error) {
	var a BankAccount
	var bal sql.NullInt64
	err := s.Scan(&a.ID, &a.ConnectionID, &a.IdentificationHash, &a.UID, &a.IBAN, &a.DisplayName, &a.AccountID, &a.LastSyncedAt, &a.LastTxDate,
		&bal, &a.BalanceCurrency, &a.BalanceType, &a.BalanceDate, &a.BalanceFetched, &a.AppliedThrough)
	if bal.Valid {
		v := money.Cents(bal.Int64)
		a.BankBalance = &v
	}
	return a, err
}

func listBankAccounts(q querier, connID int64) ([]BankAccount, error) {
	query := `SELECT ` + bankAccountCols + ` FROM bank_accounts`
	var args []any
	if connID > 0 {
		query += ` WHERE connection_id=?`
		args = append(args, connID)
	}
	rows, err := q.Query(query+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BankAccount
	for rows.Next() {
		a, err := scanBankAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func getBankAccount(q querier, id int64) (BankAccount, error) {
	return scanBankAccount(q.QueryRow(`SELECT `+bankAccountCols+` FROM bank_accounts WHERE id=?`, id))
}

func saveBankAccount(e execer, a *BankAccount) error {
	now := db.Now()
	var bal any
	if a.BankBalance != nil {
		bal = int64(*a.BankBalance)
	}
	if a.ID == 0 {
		res, err := e.Exec(`INSERT INTO bank_accounts(connection_id,identification_hash,uid,iban,display_name,account_id,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`,
			a.ConnectionID, a.IdentificationHash, a.UID, a.IBAN, a.DisplayName, a.AccountID, now, now)
		if err != nil {
			return err
		}
		a.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := e.Exec(`UPDATE bank_accounts SET uid=?,iban=?,display_name=?,account_id=?,last_synced_at=?,last_tx_date=?,bank_balance=?,bank_balance_currency=?,
		bank_balance_type=?,bank_balance_date=?,bank_balance_fetched=?,balance_applied_through=?,updated_at=? WHERE id=?`,
		a.UID, a.IBAN, a.DisplayName, a.AccountID, a.LastSyncedAt, a.LastTxDate, bal, a.BalanceCurrency, a.BalanceType, a.BalanceDate,
		a.BalanceFetched, a.AppliedThrough, now, a.ID)
	return err
}

type querier interface {
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}

type execer interface {
	Exec(string, ...any) (sql.Result, error)
}

// InboxRow is one bank row awaiting a decision.
type InboxRow struct {
	ID            int64       `json:"id"`
	BankAccountID int64       `json:"bank_account_id"`
	ExternalID    string      `json:"external_id"`
	Raw           string      `json:"raw,omitempty"`
	RawPayee      string      `json:"raw_payee"`
	RawDetails    string      `json:"raw_details"`
	RawCurrency   string      `json:"raw_currency"`
	BookingDate   string      `json:"booking_date"`
	Pending       bool        `json:"pending"`
	Date          string      `json:"date"`
	Kind          string      `json:"kind"`
	Amount        money.Cents `json:"amount"`
	AccountID     string      `json:"account_id"`
	ToAccountID   string      `json:"to_account_id"`
	Category      string      `json:"category"`
	Merchant      string      `json:"merchant"`
	Note          string      `json:"note"`
	Tags          []string    `json:"tags"`
	Edited        bool        `json:"edited"`
	Guessed       bool        `json:"guessed"`
	Verdict       string      `json:"verdict"`
	VerdictNote   string      `json:"verdict_note"`
	MatchedTxID   int64       `json:"matched_tx_id,omitempty"`
	State         string      `json:"state"`
	ImportedTxID  int64       `json:"imported_tx_id,omitempty"`
	SupersededBy  int64       `json:"superseded_by,omitempty"`
	FirstSeenAt   string      `json:"first_seen_at"`
	// Match is the hand-entered transaction this row probably is.
	Match *ledger.Tx `json:"match,omitempty"`
}

// Ready reports whether a row may become a transaction.
func (r *InboxRow) Ready() bool { return r.State == "open" && !r.Pending && r.Amount > 0 && r.Category != "" }

// Preticked: the rows a one-tap "Accept all" should take.
func (r *InboxRow) Preticked() bool {
	return r.Ready() && r.Verdict == "new" && r.MatchedTxID == 0 && !r.Guessed
}

const inboxCols = `id,COALESCE(bank_account_id,0),external_id,raw,raw_payee,raw_details,raw_currency,booking_date,pending,date,kind,amount,
	COALESCE(account_id,''),COALESCE(to_account_id,''),category,merchant,note,tags,edited,guessed,verdict,verdict_note,COALESCE(matched_tx_id,0),
	state,COALESCE(imported_tx_id,0),COALESCE(superseded_by,0),first_seen_at`

func scanInbox(s interface{ Scan(...any) error }) (InboxRow, error) {
	var r InboxRow
	var tags string
	err := s.Scan(&r.ID, &r.BankAccountID, &r.ExternalID, &r.Raw, &r.RawPayee, &r.RawDetails, &r.RawCurrency, &r.BookingDate, &r.Pending, &r.Date,
		&r.Kind, &r.Amount, &r.AccountID, &r.ToAccountID, &r.Category, &r.Merchant, &r.Note, &tags, &r.Edited, &r.Guessed, &r.Verdict, &r.VerdictNote,
		&r.MatchedTxID, &r.State, &r.ImportedTxID, &r.SupersededBy, &r.FirstSeenAt)
	r.Tags = ledger.SplitTags(tags)
	if r.Tags == nil {
		r.Tags = []string{}
	}
	return r, err
}

func listInbox(q querier, state string) ([]InboxRow, error) {
	query := `SELECT ` + inboxCols + ` FROM bank_inbox`
	var args []any
	if state != "" && state != "all" {
		query += ` WHERE state=?`
		args = append(args, state)
	}
	rows, err := q.Query(query+` ORDER BY date DESC, id DESC LIMIT 1000`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InboxRow
	for rows.Next() {
		r, err := scanInbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func getInbox(q querier, id int64) (InboxRow, error) {
	return scanInbox(q.QueryRow(`SELECT `+inboxCols+` FROM bank_inbox WHERE id=?`, id))
}

func getInboxByExternal(q querier, ext string) (InboxRow, error) {
	return scanInbox(q.QueryRow(`SELECT `+inboxCols+` FROM bank_inbox WHERE external_id=?`, ext))
}

func nz(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func ns(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func saveInbox(e execer, r *InboxRow) error {
	tags := ledger.JoinTags(r.Tags)
	if r.ID == 0 {
		now := db.Now()
		res, err := e.Exec(`INSERT INTO bank_inbox(bank_account_id,external_id,raw,raw_payee,raw_details,raw_currency,booking_date,pending,date,kind,amount,
			account_id,to_account_id,category,merchant,note,tags,edited,guessed,verdict,verdict_note,matched_tx_id,state,imported_tx_id,superseded_by,first_seen_at,last_seen_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			nz(r.BankAccountID), r.ExternalID, r.Raw, r.RawPayee, r.RawDetails, r.RawCurrency, r.BookingDate, r.Pending, r.Date, r.Kind, int64(r.Amount),
			ns(r.AccountID), ns(r.ToAccountID), r.Category, r.Merchant, r.Note, tags, r.Edited, r.Guessed, r.Verdict, r.VerdictNote, nz(r.MatchedTxID),
			r.State, nz(r.ImportedTxID), nz(r.SupersededBy), now, now)
		if err != nil {
			return err
		}
		r.ID, _ = res.LastInsertId()
		r.FirstSeenAt = now
		return nil
	}
	_, err := e.Exec(`UPDATE bank_inbox SET raw=?,raw_payee=?,raw_details=?,raw_currency=?,booking_date=?,pending=?,date=?,kind=?,amount=?,account_id=?,to_account_id=?,
		category=?,merchant=?,note=?,tags=?,edited=?,guessed=?,verdict=?,verdict_note=?,matched_tx_id=?,state=?,imported_tx_id=?,superseded_by=?,last_seen_at=? WHERE id=?`,
		r.Raw, r.RawPayee, r.RawDetails, r.RawCurrency, r.BookingDate, r.Pending, r.Date, r.Kind, int64(r.Amount), ns(r.AccountID), ns(r.ToAccountID),
		r.Category, r.Merchant, r.Note, tags, r.Edited, r.Guessed, r.Verdict, r.VerdictNote, nz(r.MatchedTxID), r.State, nz(r.ImportedTxID),
		nz(r.SupersededBy), db.Now(), r.ID)
	return err
}
