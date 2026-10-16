package ledger

import (
	"database/sql"
	"sort"
	"time"

	"ft/internal/merchant"
	"ft/internal/money"
)

// QuickPicks is what a new expense most likely is, so entering one is a tap
// and an amount: the account usually paid from by hand, and the merchants
// usually paid on this kind of day (weekday or weekend) with their category
// and account. Transactions carry no time of day, so the day type is the
// finest signal there is.
type QuickPicks struct {
	AccountID string      `json:"account_id"`
	Merchants []QuickPick `json:"merchants"`
}

type QuickPick struct {
	Merchant  string      `json:"merchant"`
	Category  string      `json:"category"`
	AccountID string      `json:"account_id"`
	Typical   money.Cents `json:"typical"` // median amount, a hint — never filled in
	Count     int         `json:"count"`
}

func Quick(d *sql.DB, now time.Time) (*QuickPicks, error) {
	txs, err := All(d, Filter{From: now.AddDate(0, 0, -120).Format("2006-01-02"), Kind: "expense"})
	if err != nil {
		return nil, err
	}
	weekend := func(t time.Time) bool { return t.Weekday() == time.Saturday || t.Weekday() == time.Sunday }
	today := weekend(now)
	accounts := map[string]int{}
	type agg struct {
		name, cat, acct string
		amounts         []money.Cents
		cats, accts     map[string]int
		n               int
	}
	by := map[string]*agg{}
	for _, t := range txs {
		if t.Source == "manual" && t.AccountID != "" {
			accounts[t.AccountID]++
		}
		day, err := time.Parse("2006-01-02", t.Date)
		if err != nil || t.Merchant == "" || weekend(day) != today || t.Amount >= money.FromFloat(250) {
			continue
		}
		// Bills and fees arrive by themselves; quick-add is for what you buy.
		switch Top(t.Category) {
		case "utilities", "subscriptions", "finance", "housing":
			continue
		}
		k := merchant.Fold(t.Merchant)
		a := by[k]
		if a == nil {
			a = &agg{name: t.Merchant, cats: map[string]int{}, accts: map[string]int{}}
			by[k] = a
		}
		a.n++
		a.amounts = append(a.amounts, t.Amount)
		a.cats[t.Category]++
		a.accts[t.AccountID]++
	}
	top := func(m map[string]int) string {
		best, bn := "", 0
		for k, n := range m {
			if n > bn || n == bn && k < best {
				best, bn = k, n
			}
		}
		return best
	}
	q := &QuickPicks{AccountID: top(accounts), Merchants: []QuickPick{}}
	for _, a := range by {
		if a.n < 2 {
			continue
		}
		sort.Slice(a.amounts, func(i, j int) bool { return a.amounts[i] < a.amounts[j] })
		q.Merchants = append(q.Merchants, QuickPick{Merchant: a.name, Category: top(a.cats), AccountID: top(a.accts), Typical: a.amounts[len(a.amounts)/2], Count: a.n})
	}
	sort.Slice(q.Merchants, func(i, j int) bool {
		if q.Merchants[i].Count != q.Merchants[j].Count {
			return q.Merchants[i].Count > q.Merchants[j].Count
		}
		return q.Merchants[i].Merchant < q.Merchants[j].Merchant
	})
	if len(q.Merchants) > 6 {
		q.Merchants = q.Merchants[:6]
	}
	return q, nil
}
