package cfo

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

// MonthReview answers "how did the month go, and is anything in the data
// stale or waiting on me?" — deterministic, free and instant.
type MonthReview struct {
	Month       string           `json:"month"`
	Complete    bool             `json:"complete"`
	Flow        insights.Flow    `json:"flow"`
	Previous    insights.Flow    `json:"previous"`
	SixMonthAvg insights.Flow    `json:"six_month_avg"`
	Verdict     string           `json:"verdict"`
	Tone        string           `json:"tone"` // good | neutral | bad
	Highlights  []Highlight      `json:"highlights"`
	NetWorth    NetWorthMove     `json:"net_worth"`
	Categories  []CategoryVsNorm `json:"categories"`
	TopExpenses []ledger.Tx      `json:"top_expenses"`
	TopIncome   []ledger.Tx      `json:"top_income"`
	Daily       []DaySpend       `json:"daily"`
	Trend       []insights.Flow  `json:"trend"` // twelve months ending with this one
	Budget      *BudgetBrief     `json:"budget,omitempty"`
	OwedToYou   money.Cents      `json:"owed_to_you"`
	Checks      []Check          `json:"checks"`
	lateSalary  money.Cents
}

type Highlight struct {
	Tone string `json:"tone"`
	Text string `json:"text"`
}

type NetWorthMove struct {
	Start  money.Cents  `json:"start"`
	End    money.Cents  `json:"end"`
	Change money.Cents  `json:"change"`
	Points []SparkPoint `json:"points"`
}

type CategoryVsNorm struct {
	Category string      `json:"category"`
	Spent    money.Cents `json:"spent"`
	Average  money.Cents `json:"average"` // previous six months
	Delta    money.Cents `json:"delta"`
	Count    int         `json:"count"`
	Top      []ledger.Tx `json:"top"`
}

type DaySpend struct {
	Date  string      `json:"date"`
	Spent money.Cents `json:"spent"`
}

type BudgetBrief struct {
	Over        []LineBrief `json:"over"`
	WithinCount int         `json:"within_count"`
	Lines       []LineBrief `json:"lines"`
	SafeToSpend money.Cents `json:"safe_to_spend"`
}

// Check is one item of the data-health list (also shown on Home).
type Check struct {
	Level string `json:"level"` // warn | info
	Text  string `json:"text"`
	Link  string `json:"link"`
}

func monthBounds(m string) (string, string) {
	t, _ := time.Parse("2006-01", m)
	return t.Format("2006-01-02"), t.AddDate(0, 1, -1).Format("2006-01-02")
}

// BuildMonthReview reviews one month (default: the last complete one).
func BuildMonthReview(d *sql.DB, month string, now time.Time) (*MonthReview, error) {
	if _, err := time.Parse("2006-01", month); err != nil {
		month = now.AddDate(0, -1, 0).Format("2006-01")
	}
	sel, _ := time.Parse("2006-01", month)
	from, to := monthBounds(month)
	r := &MonthReview{Month: month, Complete: to < now.Format("2006-01-02")}
	// A few days past the month, so a salary paid early next month counts here.
	salary := plan.LoadSettings(d).Salary()
	toPlus := to
	if t, err := time.Parse("2006-01-02", to); err == nil {
		toPlus = t.AddDate(0, 0, plan.MaxSalaryDays(salary)).Format("2006-01-02")
	}
	txs, err := ledger.All(d, ledger.Filter{From: sel.AddDate(0, -11, 0).Format("2006-01-02"), To: toPlus})
	if err != nil {
		return nil, err
	}
	cats, _ := ledger.CategoryMap(d)
	flows := insights.CashFlow(txs, cats, "month", salary)
	byPeriod := map[string]insights.Flow{}
	for _, f := range flows {
		byPeriod[f.Period] = f
	}
	for i := 11; i >= 0; i-- {
		p := sel.AddDate(0, -i, 0).Format("2006-01")
		f, ok := byPeriod[p]
		if !ok {
			f = insights.Flow{Period: p}
		}
		r.Trend = append(r.Trend, f)
	}
	r.Flow = byPeriod[month]
	r.Flow.Period = month
	r.Previous = byPeriod[sel.AddDate(0, -1, 0).Format("2006-01")]
	var avg insights.Flow
	for i := 1; i <= 6; i++ {
		f := byPeriod[sel.AddDate(0, -i, 0).Format("2006-01")]
		avg.Income += f.Income
		avg.Spending += f.Spending
		avg.Saved += f.Saved
		avg.Invested += f.Invested
		avg.Essential += f.Essential
		avg.Discretionary += f.Discretionary
	}
	avg.Income, avg.Spending, avg.Saved, avg.Invested = avg.Income/6, avg.Spending/6, avg.Saved/6, avg.Invested/6
	avg.Essential, avg.Discretionary = avg.Essential/6, avg.Discretionary/6
	if avg.Income > 0 {
		avg.SavingsRate = math.Round(avg.Saved.Float()/avg.Income.Float()*1000) / 1000
	}
	r.SixMonthAvg = avg

	// Categories this month vs their own six-month norm.
	type agg struct {
		spent, prev6 money.Cents
		n            int
		top          []ledger.Tx
	}
	byCat := map[string]*agg{}
	daily := map[string]money.Cents{}
	sixFrom := sel.AddDate(0, -6, 0).Format("2006-01-02")
	var monthTx []ledger.Tx
	for _, t := range txs {
		if t.Kind != "expense" {
			continue
		}
		c := ledger.Top(t.Category)
		a := byCat[c]
		if a == nil {
			a = &agg{}
			byCat[c] = a
		}
		switch {
		case t.Date >= from:
			a.spent += t.Amount
			a.n++
			a.top = append(a.top, t)
			daily[t.Date] += t.Amount
			monthTx = append(monthTx, t)
		case t.Date >= sixFrom:
			a.prev6 += t.Amount
		}
	}
	for c, a := range byCat {
		if a.spent == 0 && a.prev6 == 0 {
			continue
		}
		sort.Slice(a.top, func(i, j int) bool { return a.top[i].Amount > a.top[j].Amount })
		if len(a.top) > 3 {
			a.top = a.top[:3]
		}
		av := a.prev6 / 6
		r.Categories = append(r.Categories, CategoryVsNorm{Category: c, Spent: a.spent, Average: av, Delta: a.spent - av, Count: a.n, Top: a.top})
	}
	sort.Slice(r.Categories, func(i, j int) bool { return r.Categories[i].Spent > r.Categories[j].Spent })
	for t := sel; t.Format("2006-01") == month; t = t.AddDate(0, 0, 1) {
		ds := t.Format("2006-01-02")
		r.Daily = append(r.Daily, DaySpend{Date: ds, Spent: daily[ds]})
	}
	r.TopExpenses = insights.TopExpenses(monthTx, from, to, 8)
	for _, t := range txs {
		if t.Kind == "income" && t.Date >= from && ledger.Top(t.Category) != "refunds" {
			r.TopIncome = append(r.TopIncome, t)
		}
	}
	sort.Slice(r.TopIncome, func(i, j int) bool { return r.TopIncome[i].Amount > r.TopIncome[j].Amount })
	if len(r.TopIncome) > 5 {
		r.TopIncome = r.TopIncome[:5]
	}

	// Net worth across the month.
	if book, err := wealth.LoadBook(d); err == nil {
		start := book.SnapshotAt(sel.AddDate(0, 0, -1).Format("2006-01-02"), false)
		end := book.SnapshotAt(minDate(to, now.Format("2006-01-02")), false)
		r.NetWorth = NetWorthMove{Start: start.NetWorth, End: end.NetWorth, Change: end.NetWorth - start.NetWorth}
		for _, h := range book.History(from, minDate(to, now.Format("2006-01-02")), "week") {
			r.NetWorth.Points = append(r.NetWorth.Points, SparkPoint{h.Date, h.NetWorth, h.Liquid})
		}
	}

	// Budget results for the month.
	if rep, err := BudgetReport(d, month, now); err == nil && len(rep.Lines) > 0 {
		b := &BudgetBrief{SafeToSpend: rep.SafeToSpend}
		for _, l := range rep.Lines {
			if l.Kind != "spending" || l.Period != "monthly" || l.Fund {
				continue
			}
			lb := LineBrief{Name: l.Name, Spent: l.Spent, Budgeted: l.Budgeted, Remaining: l.Remaining}
			b.Lines = append(b.Lines, lb)
			if l.Remaining < 0 {
				b.Over = append(b.Over, lb)
			} else {
				b.WithinCount++
			}
		}
		sort.Slice(b.Lines, func(i, j int) bool { return b.Lines[i].Remaining < b.Lines[j].Remaining })
		r.Budget = b
	}
	r.OwedToYou = OwedTotal(d)
	// Cash flow already counts a salary paid on the 1st–3rd of next month here;
	// say so, since the bank statement shows it in the other month.
	for i := range txs {
		if t := &txs[i]; t.Date > to && plan.FlowDate(t, salary) <= to && t.Kind == "income" {
			r.lateSalary += t.Amount
		}
	}
	r.Verdict, r.Tone, r.Highlights = verdict(r)
	r.Checks = Checks(d, now)
	return r, nil
}

func minDate(a, b string) string {
	if a < b {
		return a
	}
	return b
}

// verdict turns the month into one sentence and a few plain observations.
func verdict(r *MonthReview) (string, string, []Highlight) {
	f, avg := r.Flow, r.SixMonthAvg
	var hs []Highlight
	tone := "neutral"
	head := "An ordinary month."
	if r.lateSalary > 0 {
		hs = append(hs, Highlight{"neutral", fmt.Sprintf("Salary of %s arrived in the first days of next month and is counted in this month.", eurS(r.lateSalary))})
	}
	switch {
	case f.Income == 0 && f.Spending == 0:
		return "Nothing recorded for this month yet.", "neutral", nil
	case f.Saved < 0:
		tone, head = "bad", fmt.Sprintf("You spent %s more than you earned.", eurS(-f.Saved))
	case avg.Income > 0 && f.SavingsRate >= avg.SavingsRate+0.05:
		tone, head = "good", fmt.Sprintf("A strong month — you kept %s of your income.", pctS(f.SavingsRate))
	case avg.Income > 0 && f.SavingsRate < avg.SavingsRate-0.1:
		tone, head = "bad", fmt.Sprintf("Savings fell to %s, below your usual %s.", pctS(f.SavingsRate), pctS(avg.SavingsRate))
	default:
		head = fmt.Sprintf("You kept %s of your income — about your usual %s.", pctS(f.SavingsRate), pctS(avg.SavingsRate))
	}
	return head, tone, append(hs, tail(r)...)
}

// tail is the list of observations that follow the headline.
func tail(r *MonthReview) []Highlight {
	f, avg := r.Flow, r.SixMonthAvg
	var hs []Highlight
	if avg.Spending > 0 {
		d := f.Spending - avg.Spending
		switch {
		case d.Float() > 0.15*avg.Spending.Float():
			hs = append(hs, Highlight{"bad", fmt.Sprintf("Spending was %s above your six-month average.", eurS(d))})
		case d.Float() < -0.1*avg.Spending.Float():
			hs = append(hs, Highlight{"good", fmt.Sprintf("Spending was %s below your six-month average.", eurS(-d))})
		}
	}
	for _, c := range r.Categories {
		if c.Average > 0 && c.Delta.Float() > math.Max(100, 0.4*c.Average.Float()) {
			hs = append(hs, Highlight{"bad", fmt.Sprintf("%s ran %s over its usual %s.", titleCase(c.Category), eurS(c.Delta), eurS(c.Average))})
			if len(hs) >= 4 {
				break
			}
		}
	}
	if f.Invested > 0 {
		hs = append(hs, Highlight{"good", fmt.Sprintf("%s went into investments, pension and mortgage principal.", eurS(f.Invested))})
	}
	if r.NetWorth.Change != 0 {
		t := "good"
		if r.NetWorth.Change < 0 {
			t = "bad"
		}
		hs = append(hs, Highlight{t, fmt.Sprintf("Net worth moved %s to %s.", signedS(r.NetWorth.Change), eurS(r.NetWorth.End))})
	}
	if r.Budget != nil && len(r.Budget.Over) > 0 {
		var names []string
		for _, o := range r.Budget.Over {
			names = append(names, o.Name)
		}
		hs = append(hs, Highlight{"bad", "Over budget: " + strings.Join(names, ", ") + "."})
	}
	return hs
}

func titleCase(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func eurS(c money.Cents) string {
	v := int64(math.Round(c.Abs().Float()))
	s := fmt.Sprint(v)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return "€" + s
}

func signedS(c money.Cents) string {
	if c < 0 {
		return "−" + eurS(c)
	}
	return "+" + eurS(c)
}

func pctS(v float64) string { return fmt.Sprintf("%.0f%%", v*100) }

// OwedTotal is what other people owe the owner (owed:* parts minus repayments).
func OwedTotal(d *sql.DB) money.Cents {
	txs, err := ledger.All(d, ledger.Filter{Query: "owed:"})
	if err != nil {
		return 0
	}
	var total money.Cents
	for _, t := range txs {
		for _, tag := range t.Tags {
			if strings.HasPrefix(tag, "owed:") {
				if t.Kind == "income" {
					total -= t.Amount
				} else {
					total += t.Amount
				}
			}
		}
	}
	return total
}

// Checks is the data-health list: stale balances, valuations, bank consent,
// waiting inbox rows, vague categories, overdue backups, rate resets.
func Checks(d *sql.DB, now time.Time) []Check {
	out := []Check{}
	add := func(level, link, format string, a ...any) {
		out = append(out, Check{Level: level, Text: fmt.Sprintf(format, a...), Link: link})
	}
	days := func(date string) int {
		t, err := time.Parse("2006-01-02", date[:min(10, len(date))])
		if err != nil {
			return 0
		}
		return int(now.Sub(t).Hours() / 24)
	}
	if book, err := wealth.LoadBook(d); err == nil {
		snap := book.SnapshotAt(now.Format("2006-01-02"), true)
		var stale []string
		for id := range snap.Stale {
			stale = append(stale, book.Accounts[id].Name)
		}
		sort.Strings(stale)
		if len(stale) > 0 {
			add("warn", "/wealth?update=1", "%s %s over 45 days old — update so net worth stays true.", strings.Join(stale, ", "), plural(len(stale), "is", "are"))
		}
		for id, a := range book.Accounts {
			if a.Archived || (a.Kind != "property" && a.Kind != "vehicle") {
				continue
			}
			s := book.Series(id)
			if len(s) > 0 && days(s[len(s)-1].Date) > 365 {
				add("info", "/wealth", "%s was last valued %s — over a year ago.", a.Name, s[len(s)-1].Date)
			}
		}
		for _, a := range book.Accounts {
			if a.Kind != "loan" || a.Archived {
				continue
			}
			var ld ledger.LoanDetails
			json.Unmarshal(a.Details, &ld)
			if ld.RateResetDate != "" {
				if dd := -days(ld.RateResetDate); dd >= 0 && dd <= 45 {
					add("info", "/wealth/loans", "%s rate resets on %s (in %d days).", a.Name, ld.RateResetDate, dd)
				}
			}
		}
	}
	rows, err := d.Query(`SELECT aspsp_name, status, valid_until FROM bank_connections WHERE status IN ('authorized','expired')`)
	if err == nil {
		for rows.Next() {
			var name, status, until string
			rows.Scan(&name, &status, &until)
			t, err := time.Parse(time.RFC3339, until)
			left := int(t.Sub(now).Hours() / 24)
			switch {
			case status == "expired" || (err == nil && left < 0):
				add("warn", "/settings/banks", "%s bank access has expired — reconnect it.", name)
			case err == nil && left <= 14:
				add("warn", "/settings/banks", "%s bank access expires in %d days — reconnect before it lapses.", name, left)
			}
		}
		rows.Close()
	}
	var waiting int
	d.QueryRow(`SELECT COUNT(*) FROM bank_inbox WHERE state='open' AND pending=0`).Scan(&waiting)
	if waiting > 0 {
		add("warn", "/ledger/inbox", "%d bank %s waiting for review.", waiting, plural(waiting, "row is", "rows are"))
	}
	var vague int
	d.QueryRow(`SELECT COUNT(*) FROM transactions WHERE kind='expense' AND category IN ('other','leisure','shopping','food','housing','finance','health','transport','utilities','subscriptions') AND date >= ?`,
		now.AddDate(0, -3, 0).Format("2006-01-02")).Scan(&vague)
	if vague >= 5 {
		add("info", "/ledger/tidy", "%d recent expenses only have a broad category — tidy them up for sharper insights.", vague)
	}
	var last string
	d.QueryRow(`SELECT value FROM settings WHERE key='last_backup_download'`).Scan(&last)
	last = strings.Trim(last, `"`)
	if last == "" {
		add("info", "/settings/data", "No backup downloaded yet — keep one off this device.")
	} else if days(last) > 30 {
		add("info", "/settings/data", "Last backup downloaded %d days ago.", days(last))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Level == "warn" && out[j].Level != "warn" })
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
