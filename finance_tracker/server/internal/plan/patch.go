package plan

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"ft/internal/ledger"
	"ft/internal/money"
)

// BudgetPatch changes a budget line field by field (nil = keep), or creates
// one when ID is 0. It is what the assistants use: an amount change applies
// from FromMonth (default: this month), so past months keep what they were
// planned with — unless AllMonths rewrites the amount everywhere.
type BudgetPatch struct {
	ID         int64     `json:"id"`
	Name       *string   `json:"name"`
	Kind       *string   `json:"kind"`
	Categories *[]string `json:"categories"`
	Tag        *string   `json:"tag"`
	Period     *string   `json:"period"`
	Fund       *bool     `json:"fund"`
	StartMonth *string   `json:"start_month"`
	Archived   *bool     `json:"archived"`
	Amount     *float64  `json:"amount"` // EUR per period
	FromMonth  string    `json:"from_month"`
	AllMonths  bool      `json:"all_months"`
}

// Get returns one budget line, archived or not.
func Get(d *sql.DB, id int64) (Budget, error) {
	all, err := List(d, true)
	if err != nil {
		return Budget{}, err
	}
	for _, b := range all {
		if b.ID == id {
			return b, nil
		}
	}
	return Budget{}, errors.New("no budget line with that id")
}

// Patch applies p and returns the saved line.
func Patch(d *sql.DB, p BudgetPatch, now time.Time) (Budget, error) {
	var b Budget
	if p.ID != 0 {
		cur, err := Get(d, p.ID)
		if err != nil {
			return Budget{}, err
		}
		b = cur
	} else if p.Amount == nil {
		return Budget{}, errors.New("a new budget line needs an amount")
	}
	if p.Name != nil {
		b.Name = *p.Name
	}
	if p.Kind != nil {
		b.Kind = *p.Kind
	}
	if p.Categories != nil {
		b.Categories = *p.Categories
	}
	if p.Tag != nil {
		b.Tag = strings.TrimSpace(*p.Tag)
	}
	if p.Period != nil {
		b.Period = *p.Period
	}
	if p.Fund != nil {
		b.Fund = *p.Fund
	}
	if p.StartMonth != nil {
		b.StartMonth = *p.StartMonth
	}
	if p.Archived != nil {
		b.Archived = *p.Archived
	}
	for _, m := range []string{b.StartMonth, p.FromMonth} {
		if _, err := time.Parse("2006-01", m); m != "" && err != nil {
			return Budget{}, errors.New("months are YYYY-MM")
		}
	}
	if cats, err := ledger.CategoryMap(d); err == nil {
		for _, c := range b.Categories {
			if _, ok := cats[c]; !ok {
				return Budget{}, errors.New("unknown category " + c)
			}
		}
	}
	// Keep the amount history unless the amount changes; a change starts in
	// FromMonth (this month by default) or, with AllMonths, replaces it all.
	from := ""
	if n := len(b.Steps); n > 0 {
		from = b.Steps[n-1].FromMonth
	}
	if p.Amount != nil {
		if *p.Amount < 0 {
			return Budget{}, errors.New("amount must be positive")
		}
		amt := money.FromFloat(*p.Amount)
		if b.ID != 0 && amt != b.Amount {
			switch {
			case p.AllMonths:
				from = ""
			case p.FromMonth != "":
				from = p.FromMonth
			default:
				from = now.Format("2006-01")
			}
		}
		b.Amount = amt
	}
	if err := Save(d, &b, from); err != nil {
		return Budget{}, err
	}
	return Get(d, b.ID)
}

// ValidateSettings checks plan settings against the database.
func ValidateSettings(d *sql.DB, st Settings) error {
	if err := ValidSalaryRules(st.SalaryRules); err != nil {
		return err
	}
	switch st.IncomeMode {
	case "", "median", "manual", "gross":
	default:
		return errors.New("income_mode is median, manual or gross")
	}
	if st.SalaryAccount != "" {
		if _, err := ledger.GetAccount(d, st.SalaryAccount); err != nil {
			return errors.New("unknown salary account")
		}
	}
	for id, v := range st.Buffers {
		if _, err := ledger.GetAccount(d, id); err != nil {
			return errors.New("unknown account in buffers: " + id)
		}
		if v < 0 || v > 1e6 {
			return errors.New("a buffer is between €0 and €1,000,000")
		}
	}
	return nil
}

// PatchSettings merges a partial JSON object into the saved settings (absent
// fields keep their value; buffers merge per account, 0 removes one).
func PatchSettings(d *sql.DB, patch json.RawMessage) (Settings, error) {
	st := LoadSettings(d)
	if err := json.Unmarshal(patch, &st); err != nil {
		return Settings{}, errors.New("invalid settings: " + err.Error())
	}
	for id, v := range st.Buffers {
		if v == 0 {
			delete(st.Buffers, id)
		}
	}
	if err := ValidateSettings(d, st); err != nil {
		return Settings{}, err
	}
	return st, SaveSettings(d, st)
}

// TagTrip puts the transactions under one trip:<name> tag, replacing any
// other trip tag they had.
func TagTrip(d *sql.DB, name string, ids []int64) (string, int, error) {
	name = strings.ReplaceAll(strings.Trim(ledger.SlugID(name), "_"), "_", "-")
	if name == "" {
		return "", 0, errors.New("name the trip")
	}
	tag := "trip:" + name
	tx, err := d.Begin()
	if err != nil {
		return "", 0, err
	}
	defer tx.Rollback()
	n := 0
	for _, id := range ids {
		t, err := ledger.Get(tx, id)
		if err != nil {
			continue
		}
		var keep []string
		for _, x := range t.Tags {
			if !ledger.TripTag(x) {
				keep = append(keep, x)
			}
		}
		t.Tags = append(keep, tag)
		if err := ledger.Update(tx, &t); err != nil {
			return "", 0, err
		}
		n++
	}
	return tag, n, tx.Commit()
}
