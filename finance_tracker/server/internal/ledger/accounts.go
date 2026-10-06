// Package ledger owns the core records: accounts, categories, transactions,
// tags and categorisation rules.
package ledger

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"ft/internal/db"
)

// Account kinds. Liabilities (loan) carry negative balances.
var AccountKinds = []string{"checking", "savings", "cash", "brokerage", "pension", "crypto", "property", "vehicle", "loan", "other"}

// Groups used for net-worth breakdowns.
func KindGroup(kind string) string {
	switch kind {
	case "checking", "savings", "cash":
		return "cash"
	case "brokerage":
		return "investments"
	case "pension":
		return "pension"
	case "crypto":
		return "crypto"
	case "property", "vehicle":
		return "real_assets"
	case "loan":
		return "debt"
	}
	return "other"
}

type Account struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Institution string          `json:"institution"`
	Kind        string          `json:"kind"`
	Group       string          `json:"group"`
	Currency    string          `json:"currency"`
	Liquid      bool            `json:"liquid"`
	Archived    bool            `json:"archived"`
	Sort        int             `json:"sort"`
	Notes       string          `json:"notes"`
	Details     json.RawMessage `json:"details"`
}

// AssetDetails is the details payload of property / vehicle accounts.
type AssetDetails struct {
	PurchaseDate  string  `json:"purchase_date,omitempty"`
	PurchasePrice float64 `json:"purchase_price,omitempty"`
	Address       string  `json:"address,omitempty"`
	PaidOffDate   string  `json:"paid_off_date,omitempty"`
}

// LoanDetails is the details payload of loan accounts. The balance history
// lives in balances like every other account; these terms drive the
// amortisation schedule and the rate-reset reminder.
type LoanDetails struct {
	Lender         string  `json:"lender,omitempty"`
	AssetID        string  `json:"asset_id,omitempty"` // the account it is secured on
	BaseRate       float64 `json:"base_rate"`          // e.g. 6M EURIBOR, %
	BaseRateName   string  `json:"base_rate_name,omitempty"`
	Margin         float64 `json:"margin"` // %
	RateResetDate  string  `json:"rate_reset_date,omitempty"`
	MonthlyPayment float64 `json:"monthly_payment"`
	PaymentDay     int     `json:"payment_day,omitempty"`
	EndDate        string  `json:"end_date,omitempty"`
	StartDate      string  `json:"start_date,omitempty"`
	StartPrincipal float64 `json:"start_principal,omitempty"`
}

func (a *Account) normalize() {
	a.Group = KindGroup(a.Kind)
	a.Name, a.Institution = strings.TrimSpace(a.Name), strings.TrimSpace(a.Institution)
	if a.Currency == "" {
		a.Currency = "EUR"
	}
	if len(a.Details) == 0 {
		a.Details = json.RawMessage("{}")
	}
}

const accountCols = `id,name,institution,kind,currency,liquid,archived,sort,notes,details`

func scanAccount(s interface{ Scan(...any) error }) (Account, error) {
	var a Account
	var details string
	err := s.Scan(&a.ID, &a.Name, &a.Institution, &a.Kind, &a.Currency, &a.Liquid, &a.Archived, &a.Sort, &a.Notes, &details)
	a.Details = json.RawMessage(details)
	a.normalize()
	return a, err
}

func ListAccounts(q querier) ([]Account, error) {
	rows, err := q.Query(`SELECT ` + accountCols + ` FROM accounts ORDER BY archived, sort, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func GetAccount(q querier, id string) (Account, error) {
	return scanAccount(q.QueryRow(`SELECT `+accountCols+` FROM accounts WHERE id=?`, id))
}

var idRe = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)
var nonID = regexp.MustCompile(`[^a-z0-9]+`)

// SlugID derives an account id from a name.
func SlugID(name string) string {
	s := strings.ToLower(strings.ToLower(foldLT(name)))
	s = strings.Trim(nonID.ReplaceAllString(s, "_"), "_")
	if s == "" {
		s = "account"
	}
	if len(s) > 32 {
		s = strings.TrimRight(s[:32], "_")
	}
	return s
}

func foldLT(s string) string {
	return strings.NewReplacer("ą", "a", "č", "c", "ę", "e", "ė", "e", "į", "i", "š", "s", "ų", "u", "ū", "u", "ž", "z",
		"Ą", "a", "Č", "c", "Ę", "e", "Ė", "e", "Į", "i", "Š", "s", "Ų", "u", "Ū", "u", "Ž", "z").Replace(s)
}

func validKind(k string) bool {
	for _, v := range AccountKinds {
		if v == k {
			return true
		}
	}
	return false
}

// SaveAccount inserts or updates an account.
func SaveAccount(e execer, a Account) (Account, error) {
	a.normalize()
	if strings.TrimSpace(a.Name) == "" {
		return a, errors.New("name is required")
	}
	if !validKind(a.Kind) {
		return a, fmt.Errorf("unknown account kind %q", a.Kind)
	}
	if a.ID == "" {
		a.ID = SlugID(a.Name)
	}
	if !idRe.MatchString(a.ID) {
		return a, fmt.Errorf("invalid account id %q", a.ID)
	}
	if !json.Valid(a.Details) {
		return a, errors.New("details must be JSON")
	}
	now := db.Now()
	_, err := e.Exec(`INSERT INTO accounts(`+accountCols+`,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, institution=excluded.institution, kind=excluded.kind,
		currency=excluded.currency, liquid=excluded.liquid, archived=excluded.archived, sort=excluded.sort,
		notes=excluded.notes, details=excluded.details, updated_at=excluded.updated_at`,
		a.ID, a.Name, a.Institution, a.Kind, a.Currency, a.Liquid, a.Archived, a.Sort, a.Notes, string(a.Details), now, now)
	return a, err
}

// DeleteAccount removes an account that nothing references.
func DeleteAccount(d *sql.DB, id string) error {
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM transactions WHERE account_id=? OR to_account_id=?`, id, id).Scan(&n)
	if n > 0 {
		return fmt.Errorf("%d transactions use this account — archive it instead", n)
	}
	_, err := d.Exec(`DELETE FROM accounts WHERE id=?`, id)
	return err
}
