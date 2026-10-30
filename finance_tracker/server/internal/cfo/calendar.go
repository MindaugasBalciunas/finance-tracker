package cfo

import (
	"database/sql"
	"fmt"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

// The cash-flow calendar: cash day by day for the next weeks. It starts from
// today's cash (checking, savings, cash), takes the usual everyday spending
// off each day, and puts every known bill, standing order and income on its
// own date — so the tight week three weeks out shows before it arrives. The
// monthly forecast answers "how does the year look"; this answers "which day".

type CalEvent struct {
	Label  string      `json:"label"`
	Kind   string      `json:"kind"`   // income | bill | transfer
	Amount money.Cents `json:"amount"` // signed: + into cash, − out
}

type CalDay struct {
	Date    string      `json:"date"`
	Events  []CalEvent  `json:"events,omitempty"`
	Balance money.Cents `json:"balance"` // end of the day
	Low     bool        `json:"low"`     // under the emergency floor
	Short   bool        `json:"short"`   // below zero
}

type CashCalendar struct {
	Start      money.Cents `json:"start"`
	Floor      money.Cents `json:"floor"`
	Daily      money.Cents `json:"daily"` // everyday spending taken off each day
	Days       []CalDay    `json:"days"`
	Low        money.Cents `json:"low"`
	LowDate    string      `json:"low_date"`
	FirstBelow string      `json:"first_below,omitempty"` // first day under the floor
	FirstShort string      `json:"first_short,omitempty"` // first day below zero
}

func stepCadence(t time.Time, r insights.Recurring) time.Time {
	switch {
	case r.EveryDays > 0:
		return t.AddDate(0, 0, r.EveryDays)
	case r.Cadence == "quarterly":
		return t.AddDate(0, 3, 0)
	case r.Cadence == "yearly":
		return t.AddDate(1, 0, 0)
	}
	return t.AddDate(0, 1, 0)
}

func CashFlowCalendar(d *sql.DB, now time.Time, days int) (*CashCalendar, error) {
	if days < 7 || days > 120 {
		days = 60
	}
	f, err := ForecastFrom(d, now, 3, nil)
	if err != nil {
		return nil, err
	}
	txs, err := ledger.All(d, ledger.Filter{From: now.AddDate(-1, -1, 0).Format("2006-01-02"), To: now.Format("2006-01-02")})
	if err != nil {
		return nil, err
	}
	all, _, err := insights.RecurringAll(d, txs, now)
	if err != nil {
		return nil, err
	}
	book, err := wealth.LoadBook(d)
	if err != nil {
		return nil, err
	}
	isCash := func(id string) bool { a, ok := book.Accounts[id]; return ok && a.Group == "cash" }

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	end := today.AddDate(0, 0, days)
	events := map[string][]CalEvent{}
	seen := map[string]bool{}
	var smallBills money.Cents // a month: already inside the everyday figure
	salaryKnown := false
	for _, r := range all {
		var amt money.Cents
		switch r.Kind {
		case "bill":
			amt = -r.Amount
			if r.Amount < plan.OneOff && r.Cadence == "monthly" {
				smallBills += r.Amount
			}
		case "income":
			amt = r.Amount
			if ledger.Top(r.Category) == "salary" {
				salaryKnown = true
			}
		case "transfer":
			from, to := isCash(r.FromAccount), isCash(r.ToAccount)
			switch {
			case from && !to:
				amt = -r.Amount
			case !from && to:
				amt = r.Amount
			default:
				continue // between two cash accounts: cash doesn't change
			}
		default:
			continue
		}
		next, err := time.Parse("2006-01-02", r.Next)
		if err != nil || amt == 0 {
			continue
		}
		// Due today or overdue and not paid yet (a standing order the bank
		// hasn't shown, a salary a day late): it still comes, so it goes on
		// the first day rather than a whole cycle later.
		if !r.Done && (r.Overdue || r.Next == today.Format("2006-01-02")) {
			k := today.AddDate(0, 0, 1).Format("2006-01-02")
			id := "due|" + r.Kind + "|" + r.FromAccount + "|" + r.ToAccount + "|" + fmt.Sprint(int64(amt))
			if !seen[id] {
				seen[id] = true
				events[k] = append(events[k], CalEvent{Label: firstNonEmpty(r.Merchant, r.Note, r.Category), Kind: r.Kind, Amount: amt})
			}
		}
		for guard := 0; next.Before(today.AddDate(0, 0, 1)) && guard < 60; guard++ {
			next = stepCadence(next, r)
		}
		for ; !next.After(end); next = stepCadence(next, r) {
			k := next.Format("2006-01-02")
			// The same transfer listed twice (detected from the bank and
			// added by hand) is one payment: same day, accounts and amount.
			id := k + "|" + r.Kind + "|" + r.FromAccount + "|" + r.ToAccount + "|" + fmt.Sprint(int64(amt))
			if seen[id] {
				continue
			}
			seen[id] = true
			events[k] = append(events[k], CalEvent{Label: firstNonEmpty(r.Merchant, r.Note, r.Category), Kind: r.Kind, Amount: amt})
		}
	}
	// A salary paid in parts isn't on the recurring list: put the plan's
	// income on each payday the salary rules give.
	if !salaryKnown && f.Assumed.Income > 0 {
		rules := plan.LoadSettings(d).Salary()
		at := now
		for i := 0; i < days/28+2; i++ {
			p := nextPayday(rules, at)
			if p == "" {
				break
			}
			t, _ := time.Parse("2006-01-02", p)
			if t.After(end) {
				break
			}
			events[p] = append(events[p], CalEvent{Label: "Salary (plan)", Kind: "income", Amount: f.Assumed.Income})
			// One salary a month: an old rule and a new job's rule would
			// otherwise each give a payday in the same month.
			at = time.Date(t.Year(), t.Month()+1, 0, 23, 0, 0, 0, time.UTC)
		}
	}
	daily := max(0, f.Assumed.Everyday-smallBills) * 10 / 304 // a month is 30.4 days
	cal := &CashCalendar{Start: f.Cash, Floor: f.Floor, Daily: daily, Days: []CalDay{}, Low: f.Cash, LowDate: today.Format("2006-01-02")}
	bal := f.Cash
	for i := 1; i <= days; i++ {
		day := today.AddDate(0, 0, i)
		k := day.Format("2006-01-02")
		bal -= daily
		cd := CalDay{Date: k, Events: events[k]}
		for _, e := range cd.Events {
			bal += e.Amount
		}
		cd.Balance = bal
		cd.Low, cd.Short = bal < f.Floor, bal < 0
		if cd.Low && cal.FirstBelow == "" {
			cal.FirstBelow = k
		}
		if cd.Short && cal.FirstShort == "" {
			cal.FirstShort = k
		}
		if bal < cal.Low {
			cal.Low, cal.LowDate = bal, k
		}
		cal.Days = append(cal.Days, cd)
	}
	return cal, nil
}
