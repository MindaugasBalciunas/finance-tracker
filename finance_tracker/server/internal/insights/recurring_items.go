package insights

import (
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"ft/internal/db"
	"ft/internal/ledger"
	"ft/internal/money"
)

// RecurringItem is the owner's word on a recurring cost: one added by hand,
// a detected one with its amount/cadence/date corrected, or a detected one
// hidden because it isn't really recurring. Matched to detection by merchant.
type RecurringItem struct {
	ID       int64       `json:"id"`
	Merchant string      `json:"merchant"`
	Category string      `json:"category"`
	Cadence  string      `json:"cadence"`
	Amount   money.Cents `json:"amount"`
	NextDate string      `json:"next_date"`
	Note     string      `json:"note"`
	Hidden   bool        `json:"hidden"`
}

var cadenceMonths = map[string]int{"monthly": 1, "quarterly": 3, "yearly": 12}

func ListRecurringItems(q *sql.DB) ([]RecurringItem, error) {
	rows, err := q.Query(`SELECT id,merchant,category,cadence,amount,next_date,note,hidden FROM recurring_items ORDER BY lower(merchant)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RecurringItem{}
	for rows.Next() {
		var it RecurringItem
		if err := rows.Scan(&it.ID, &it.Merchant, &it.Category, &it.Cadence, &it.Amount, &it.NextDate, &it.Note, &it.Hidden); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// SaveRecurringItem inserts or updates; a second edit for the same merchant
// updates the first.
func SaveRecurringItem(d *sql.DB, it *RecurringItem) error {
	it.Merchant = strings.TrimSpace(it.Merchant)
	if it.Merchant == "" {
		return errors.New("merchant is required")
	}
	if it.Cadence == "" {
		it.Cadence = "monthly"
	}
	if _, ok := cadenceMonths[it.Cadence]; !ok {
		return errors.New("cadence must be monthly, quarterly or yearly")
	}
	if it.Amount < 0 {
		return errors.New("amount must be positive")
	}
	if it.NextDate != "" {
		if _, err := time.Parse("2006-01-02", it.NextDate); err != nil {
			return errors.New("invalid next date")
		}
	}
	now := db.Now()
	if it.ID == 0 {
		err := d.QueryRow(`INSERT INTO recurring_items(merchant,category,cadence,amount,next_date,note,hidden,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?)
			ON CONFLICT(lower(merchant)) DO UPDATE SET category=excluded.category, cadence=excluded.cadence, amount=excluded.amount,
				next_date=excluded.next_date, note=excluded.note, hidden=excluded.hidden, updated_at=excluded.updated_at
			RETURNING id`, it.Merchant, it.Category, it.Cadence, it.Amount, it.NextDate, it.Note, it.Hidden, now, now).Scan(&it.ID)
		return err
	}
	res, err := d.Exec(`UPDATE recurring_items SET merchant=?,category=?,cadence=?,amount=?,next_date=?,note=?,hidden=?,updated_at=? WHERE id=?`,
		it.Merchant, it.Category, it.Cadence, it.Amount, it.NextDate, it.Note, it.Hidden, now, it.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func DeleteRecurringItem(d *sql.DB, id int64) error {
	_, err := d.Exec(`DELETE FROM recurring_items WHERE id=?`, id)
	return err
}

// MergeRecurring applies the owner's items to what detection found. Hidden
// items drop out (returned separately so they can be restored); edits
// override amount, cadence, category and next date; hand-added items appear
// even without matching transactions.
func MergeRecurring(detected []Recurring, items []RecurringItem, now time.Time) (list, hidden []Recurring) {
	byKey := map[string]RecurringItem{}
	for _, it := range items {
		byKey[strings.ToLower(it.Merchant)] = it
	}
	today := now.Format("2006-01-02")
	apply := func(r Recurring, it RecurringItem) Recurring {
		r.ID, r.Note = it.ID, it.Note
		if it.Category != "" {
			r.Category = it.Category
		}
		if it.Cadence != "" {
			r.Cadence = it.Cadence
		}
		if it.Amount > 0 {
			r.Amount = it.Amount
		}
		if it.NextDate != "" {
			r.Next = it.NextDate
		}
		r.Monthly = r.Amount / money.Cents(cadenceMonths[r.Cadence])
		r.Next = rollForward(r.Next, r.Cadence, today)
		return r
	}
	seen := map[string]bool{}
	for _, r := range detected {
		k := strings.ToLower(r.Merchant)
		seen[k] = true
		it, ok := byKey[k]
		if !ok {
			r.Next = rollForward(r.Next, r.Cadence, today)
			list = append(list, r)
			continue
		}
		r = apply(r, it)
		r.Source = "edited"
		if it.Hidden {
			hidden = append(hidden, r)
		} else {
			list = append(list, r)
		}
	}
	for _, it := range items {
		k := strings.ToLower(it.Merchant)
		if seen[k] {
			continue
		}
		r := apply(Recurring{Source: "manual", Merchant: it.Merchant, Cadence: "monthly"}, it)
		r.LastAmount = r.Amount
		if it.Hidden {
			r.Source = "edited" // a hidden detection that no longer shows up
			hidden = append(hidden, r)
		} else {
			list = append(list, r)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Monthly > list[j].Monthly })
	return list, hidden
}

// rollForward moves a past next-date on by its cadence until it is today or later.
func rollForward(next, cadence, today string) string {
	t, err := time.Parse("2006-01-02", next)
	if err != nil {
		return next
	}
	m := cadenceMonths[cadence]
	if m == 0 {
		m = 1
	}
	for i := 0; t.Format("2006-01-02") < today && i < 600; i++ {
		t = t.AddDate(0, m, 0)
	}
	return t.Format("2006-01-02")
}

// RecurringCosts is the recurring list everyone uses: detection plus edits.
func RecurringCosts(d *sql.DB, txs []ledger.Tx, now time.Time) (list, hidden []Recurring, err error) {
	items, err := ListRecurringItems(d)
	if err != nil {
		return nil, nil, err
	}
	list, hidden = MergeRecurring(DetectRecurring(txs, now), items, now)
	return list, hidden, nil
}
