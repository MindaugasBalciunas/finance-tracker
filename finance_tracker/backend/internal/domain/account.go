package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// AccountGroup is where an account's money counts on the balance sheet.
type AccountGroup string

const (
	AccountGroupCash        AccountGroup = "cash"
	AccountGroupInvestments AccountGroup = "investments"
	AccountGroupPensions    AccountGroup = "pensions"
	AccountGroupCrypto      AccountGroup = "crypto"
	AccountGroupOther       AccountGroup = "other"
)

var ValidAccountGroups = []AccountGroup{
	AccountGroupCash, AccountGroupInvestments, AccountGroupPensions, AccountGroupCrypto, AccountGroupOther,
}

func IsValidAccountGroup(g AccountGroup) bool {
	for _, v := range ValidAccountGroups {
		if v == g {
			return true
		}
	}
	return false
}

// Account is one balance-sheet account as a row.
//
// The built-in accounts keep their own columns on Balance — every export,
// chart and AI tool reads those, and moving them would break all of it. An
// account added later has no column: its values live in Balance.Extra under
// its key, so adding or closing an account never needs a schema change.
type Account struct {
	ID    uint         `json:"id" gorm:"primaryKey"`
	Key   string       `json:"key" gorm:"uniqueIndex;not null"`
	Label string       `json:"label" gorm:"not null"`
	Group AccountGroup `json:"group" gorm:"not null;default:'other'"`
	// Builtin accounts are backed by a Balance column and cannot be edited
	// or archived here.
	Builtin bool `json:"builtin" gorm:"not null;default:false"`
	// Archived accounts keep their history but leave the forms and pickers.
	Archived  bool      `json:"archived" gorm:"not null;default:false"`
	SortOrder int       `json:"sort_order" gorm:"not null;default:0"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BuiltinAccounts mirrors the Balance columns, in the order the forms show
// them. Groups match the frontend's balanceGroups.ts.
var BuiltinAccounts = []Account{
	{Key: "seb", Label: "SEB", Group: AccountGroupCash},
	{Key: "swed", Label: "Swedbank", Group: AccountGroupCash},
	{Key: "cash", Label: "Cash", Group: AccountGroupCash},
	{Key: "rev_m", Label: "Revolut M", Group: AccountGroupCash},
	{Key: "rev_r", Label: "Revolut R", Group: AccountGroupCash},
	{Key: "swed_etf", Label: "Swed ETF", Group: AccountGroupInvestments},
	{Key: "rev_stocks", Label: "Rev Stocks", Group: AccountGroupInvestments},
	{Key: "ibkr_stocks", Label: "IBKR", Group: AccountGroupInvestments},
	{Key: "seb_pen", Label: "SEB Pension", Group: AccountGroupPensions},
	{Key: "art", Label: "Artea", Group: AccountGroupPensions},
	{Key: "luminor", Label: "Luminor", Group: AccountGroupOther},
}

// customKeyPrefix marks an account that lives in Balance.Extra. The prefix
// keeps a new key from ever colliding with a column name added later.
const customKeyPrefix = "acc_"

var customKeyRe = regexp.MustCompile(`^acc_[a-z0-9_]{1,40}$`)

// IsCustomAccountKey reports whether key is shaped like an added account's.
// Whether the account exists is the caller's question.
func IsCustomAccountKey(key string) bool { return customKeyRe.MatchString(key) }

var nonKeyChars = regexp.MustCompile(`[^a-z0-9]+`)

// CustomAccountKey derives a key from a label: "Paysera savings" →
// "acc_paysera_savings". Diacritics fold to their base letter.
func CustomAccountKey(label string) string {
	s := strings.ToLower(foldDiacritics(label))
	s = strings.Trim(nonKeyChars.ReplaceAllString(s, "_"), "_")
	if s == "" {
		s = "account"
	}
	if len(s) > 32 {
		s = strings.TrimRight(s[:32], "_")
	}
	return customKeyPrefix + s
}

var diacriticFold = strings.NewReplacer(
	"ą", "a", "č", "c", "ę", "e", "ė", "e", "į", "i", "š", "s", "ų", "u", "ū", "u", "ž", "z",
	"Ą", "a", "Č", "c", "Ę", "e", "Ė", "e", "Į", "i", "Š", "s", "Ų", "u", "Ū", "u", "Ž", "z",
)

func foldDiacritics(s string) string { return diacriticFold.Replace(s) }

// AccountValues holds the EUR values of added accounts on one snapshot,
// keyed by account key. Stored as a JSON object in a text column.
type AccountValues map[string]float64

func (v AccountValues) Value() (driver.Value, error) {
	if len(v) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(map[string]float64(v))
	return string(b), err
}

func (v *AccountValues) Scan(src any) error {
	var raw []byte
	switch s := src.(type) {
	case nil:
		*v = nil
		return nil
	case string:
		raw = []byte(s)
	case []byte:
		raw = s
	default:
		return fmt.Errorf("AccountValues: cannot scan %T", src)
	}
	if len(raw) == 0 {
		*v = nil
		return nil
	}
	m := map[string]float64{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	if len(m) == 0 {
		*v = nil
		return nil
	}
	*v = m
	return nil
}

// Sum is the total of every added account.
func (v AccountValues) Sum() float64 {
	var s float64
	for _, x := range v {
		s += x
	}
	return s
}

// Clone copies the map, so a snapshot cloned from another never shares it.
func (v AccountValues) Clone() AccountValues {
	if v == nil {
		return nil
	}
	out := make(AccountValues, len(v))
	for k, x := range v {
		out[k] = x
	}
	return out
}
