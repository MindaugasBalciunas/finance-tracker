// Package plan holds the monthly plan: budget lines (fixed obligations,
// spending envelopes, saving targets), sinking funds and trips.
package plan

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"ft/internal/db"
	"ft/internal/ledger"
	"ft/internal/money"
)

type Step struct {
	FromMonth string      `json:"from_month"` // '' = since forever
	Amount    money.Cents `json:"amount"`
}

type Budget struct {
	ID         int64       `json:"id"`
	Name       string      `json:"name"`
	Kind       string      `json:"kind"` // fixed | spending | saving
	Categories []string    `json:"categories"`
	Tag        string      `json:"tag"`
	Period     string      `json:"period"` // monthly | yearly
	Fund       bool        `json:"fund"`
	StartMonth string      `json:"start_month"`
	Archived   bool        `json:"archived"`
	Sort       int         `json:"sort"`
	Amount     money.Cents `json:"amount"` // current amount (latest step)
	Steps      []Step      `json:"steps"`
}

// Matches reports whether a transaction counts toward the line.
func (b Budget) Matches(t *ledger.Tx) bool {
	switch b.Kind {
	case "spending":
		if t.Kind != "expense" {
			return false
		}
	case "saving":
		if t.Kind != "transfer" {
			return false
		}
	case "fixed":
		if t.Kind == "income" {
			return false
		}
	}
	if b.Tag != "" && !containsTag(t.Tags, b.Tag) {
		return false
	}
	if len(b.Categories) == 0 {
		return b.Tag != ""
	}
	for _, c := range b.Categories {
		if t.Category == c || strings.HasPrefix(t.Category, c+".") {
			return true
		}
	}
	return false
}

func containsTag(tags []string, t string) bool {
	for _, x := range tags {
		if x == t {
			return true
		}
	}
	return false
}

// AmountFor resolves the line's amount in a month from its steps.
func (b Budget) AmountFor(ym string) money.Cents {
	if len(b.Steps) == 0 {
		return b.Amount
	}
	amount, found := b.Steps[0].Amount, false
	for _, s := range b.Steps {
		if s.FromMonth <= ym {
			amount, found = s.Amount, true
		}
	}
	if !found {
		return b.Steps[0].Amount
	}
	return amount
}

// MonthlyShare is what the line claims of one month.
func (b Budget) MonthlyShare(amount money.Cents) money.Cents {
	if b.Period == "yearly" {
		return amount / 12
	}
	return amount
}

func List(q *sql.DB, includeArchived bool) ([]Budget, error) {
	rows, err := q.Query(`SELECT id,name,kind,category,tag,period,fund,start_month,archived,sort FROM budgets ORDER BY archived, sort, id`)
	if err != nil {
		return nil, err
	}
	var out []Budget
	for rows.Next() {
		var b Budget
		var cats string
		if err := rows.Scan(&b.ID, &b.Name, &b.Kind, &cats, &b.Tag, &b.Period, &b.Fund, &b.StartMonth, &b.Archived, &b.Sort); err != nil {
			rows.Close()
			return nil, err
		}
		for _, c := range strings.Split(cats, ",") {
			if c = strings.TrimSpace(c); c != "" {
				b.Categories = append(b.Categories, c)
			}
		}
		if b.Categories == nil {
			b.Categories = []string{}
		}
		if !b.Archived || includeArchived {
			out = append(out, b)
		}
	}
	rows.Close()
	steps, err := q.Query(`SELECT budget_id, from_month, amount FROM budget_amounts ORDER BY budget_id, from_month`)
	if err != nil {
		return nil, err
	}
	defer steps.Close()
	byID := map[int64][]Step{}
	for steps.Next() {
		var id int64
		var s Step
		steps.Scan(&id, &s.FromMonth, &s.Amount)
		byID[id] = append(byID[id], s)
	}
	for i := range out {
		out[i].Steps = byID[out[i].ID]
		if out[i].Steps == nil {
			out[i].Steps = []Step{}
		}
		if n := len(out[i].Steps); n > 0 {
			out[i].Amount = out[i].Steps[n-1].Amount
		}
	}
	return out, nil
}

// Save creates or updates a budget line. When FromMonth is set the amount
// applies from that month on (history before it keeps its old amount);
// otherwise it replaces the amount everywhere.
func Save(d *sql.DB, b *Budget, fromMonth string) error {
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" {
		return errors.New("name is required")
	}
	switch b.Kind {
	case "fixed", "spending", "saving":
	default:
		return errors.New("kind must be fixed, spending or saving")
	}
	if len(b.Categories) == 0 && b.Tag == "" {
		return errors.New("match a category or a tag")
	}
	if b.Period != "yearly" {
		b.Period = "monthly"
	}
	if b.Kind != "spending" {
		b.Fund = false
	}
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	cats := strings.Join(b.Categories, ",")
	if b.ID == 0 {
		res, err := tx.Exec(`INSERT INTO budgets(name,kind,category,tag,period,fund,start_month,archived,sort,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			b.Name, b.Kind, cats, b.Tag, b.Period, b.Fund, b.StartMonth, b.Archived, b.Sort, db.Now())
		if err != nil {
			return err
		}
		b.ID, _ = res.LastInsertId()
		fromMonth = ""
	} else if _, err := tx.Exec(`UPDATE budgets SET name=?,kind=?,category=?,tag=?,period=?,fund=?,start_month=?,archived=?,sort=? WHERE id=?`,
		b.Name, b.Kind, cats, b.Tag, b.Period, b.Fund, b.StartMonth, b.Archived, b.Sort, b.ID); err != nil {
		return err
	}
	if fromMonth == "" {
		if _, err := tx.Exec(`DELETE FROM budget_amounts WHERE budget_id=?`, b.ID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO budget_amounts(budget_id,from_month,amount) VALUES(?,?,?)`, b.ID, fromMonth, int64(b.Amount)); err != nil {
		return err
	}
	return tx.Commit()
}

func Delete(d *sql.DB, id int64) error {
	_, err := d.Exec(`DELETE FROM budgets WHERE id=?`, id)
	return err
}

// Settings drive the income base the plan measures against.
type Settings struct {
	IncomeMode        string  `json:"income_mode"` // median | manual | gross
	ManualIncome      float64 `json:"manual_income"`
	GrossSalary       float64 `json:"gross_salary"`
	MonthlyDeductions float64 `json:"monthly_deductions"`
	// FI planning
	TargetAge      int     `json:"target_age"`
	BirthYear      int     `json:"birth_year"`
	FIMonthlySpend float64 `json:"fi_monthly_spend"` // target spend in FI; 0 = use trailing 12-month spend
	WithdrawalRate float64 `json:"withdrawal_rate"`  // e.g. 4
	ExpectedReturn float64 `json:"expected_return"`  // real, % p.a.
	EmergencyMonths float64 `json:"emergency_months"`
}

func LoadSettings(q interface{ QueryRow(string, ...any) *sql.Row }) Settings {
	s := Settings{IncomeMode: "median", WithdrawalRate: 4, ExpectedReturn: 5, EmergencyMonths: 3}
	var raw string
	if q.QueryRow(`SELECT value FROM settings WHERE key='plan'`).Scan(&raw) == nil {
		json.Unmarshal([]byte(raw), &s)
	}
	if s.WithdrawalRate <= 0 {
		s.WithdrawalRate = 4
	}
	if s.EmergencyMonths <= 0 {
		s.EmergencyMonths = 3
	}
	return s
}

func SaveSettings(e interface {
	Exec(string, ...any) (sql.Result, error)
}, s Settings) error {
	raw, _ := json.Marshal(s)
	_, err := e.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('plan',?)`, string(raw))
	return err
}

// LTNetSalary estimates Lithuanian take-home pay (2026 employee rates):
// Sodra 19.5%, GPM 20% on (gross − NPD), NPD tapering from €747 above MMA.
func LTNetSalary(gross, deductions float64) float64 {
	const sodraRate, gpmRate, mma, npdMax, npdSlope = 0.195, 0.20, 1038.0, 747.0, 0.49
	sodra := gross * sodraRate
	npd := npdMax
	if gross > mma {
		npd = npdMax - npdSlope*(gross-mma)
		if npd < 0 {
			npd = 0
		}
	}
	gpm := (gross - npd) * gpmRate
	if gpm < 0 {
		gpm = 0
	}
	return gross - sodra - gpm - deductions
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	m := len(s) / 2
	if len(s)%2 == 1 {
		return s[m]
	}
	return (s[m-1] + s[m]) / 2
}
