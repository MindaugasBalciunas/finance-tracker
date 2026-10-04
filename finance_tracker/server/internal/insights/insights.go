// Package insights is the CFO view: cash flow, savings rate, spending
// structure, recurring costs, net-worth drivers and progress to financial
// independence. Every figure is computed from the ledger and balances on
// request — nothing here is stored.
package insights

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"ft/internal/ledger"
	"ft/internal/money"
)

// Definitions used everywhere:
//
//   income      income rows except refunds (refunds reduce spending instead)
//   spending    expense rows − refunds
//   saved       income − spending
//   invested    transfers into investments, pension and debt principal paid
//               from your own accounts (asset purchases count too)
//   savings %   saved / income
//
// Mortgage principal is "invested": it moves money into home equity. That is
// why the v1 savings rate (which counted the whole mortgage as spending)
// read low.

func isRefund(t *ledger.Tx) bool { return t.Kind == "income" && ledger.Top(t.Category) == "refunds" }

// IsInvesting reports whether a transfer builds wealth (vs shuffling cash).
func IsInvesting(t *ledger.Tx) bool {
	if t.Kind != "transfer" {
		return false
	}
	switch t.Category {
	case "transfer.invest", "transfer.pension", "transfer.debt", "transfer.asset":
		return true
	}
	return false
}

// Flow is one period of cash flow.
type Flow struct {
	Period        string                 `json:"period"` // 2026-09 or 2026
	Income        money.Cents            `json:"income"`
	Spending      money.Cents            `json:"spending"`
	Essential     money.Cents            `json:"essential"`
	Discretionary money.Cents            `json:"discretionary"`
	Saved         money.Cents            `json:"saved"`
	SavingsRate   float64                `json:"savings_rate"` // 0..1, NaN-safe
	Invested      money.Cents            `json:"invested"`
	Principal     money.Cents            `json:"principal"`      // debt principal (part of invested)
	PayrollPension money.Cents           `json:"payroll_pension"` // pension paid by payroll, never touched a bank
	IncomeBy      map[string]money.Cents `json:"income_by"`
	SpendingBy    map[string]money.Cents `json:"spending_by"` // by top-level category
}

func periodKey(date, granularity string) string {
	if granularity == "year" {
		return date[:4]
	}
	return date[:7]
}

// CashFlow aggregates by month or year.
func CashFlow(txs []ledger.Tx, cats map[string]ledger.Category, granularity string) []Flow {
	byP := map[string]*Flow{}
	get := func(p string) *Flow {
		f := byP[p]
		if f == nil {
			f = &Flow{Period: p, IncomeBy: map[string]money.Cents{}, SpendingBy: map[string]money.Cents{}}
			byP[p] = f
		}
		return f
	}
	for i := range txs {
		t := &txs[i]
		f := get(periodKey(t.Date, granularity))
		switch {
		case isRefund(t):
			f.Spending -= t.Amount
			f.SpendingBy["refunds"] -= t.Amount
		case t.Kind == "income":
			f.Income += t.Amount
			f.IncomeBy[t.Category] += t.Amount
		case t.Kind == "expense":
			f.Spending += t.Amount
			f.SpendingBy[ledger.Top(t.Category)] += t.Amount
			if c, ok := cats[t.Category]; ok && c.Essential {
				f.Essential += t.Amount
			} else {
				f.Discretionary += t.Amount
			}
		case IsInvesting(t):
			if t.AccountID == "" && t.Category == "transfer.pension" {
				f.PayrollPension += t.Amount
				continue
			}
			f.Invested += t.Amount
			if t.Category == "transfer.debt" {
				f.Principal += t.Amount
			}
		}
	}
	out := make([]Flow, 0, len(byP))
	for _, f := range byP {
		f.Saved = f.Income - f.Spending
		if f.Income > 0 {
			f.SavingsRate = round3(f.Saved.Float() / f.Income.Float())
		}
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Period < out[j].Period })
	return out
}

func round3(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*1000) / 1000
}

// CategoryStat is one category over a window, compared with the window
// before it and with its own typical month.
type CategoryStat struct {
	Category    string      `json:"category"`
	Total       money.Cents `json:"total"`
	Share       float64     `json:"share"`
	Count       int         `json:"count"`
	MonthlyAvg  money.Cents `json:"monthly_avg"`
	Previous    money.Cents `json:"previous"` // same-length window before
	Change      float64     `json:"change"`   // vs previous, ratio
	TopMerchant string      `json:"top_merchant,omitempty"`
	Children    []CategoryStat `json:"children,omitempty"`
}

// Breakdown splits spending in [from,to] by category (top level with leaf
// children), compared with the equal window before.
func Breakdown(txs []ledger.Tx, from, to string) []CategoryStat {
	f, _ := time.Parse("2006-01-02", from)
	t, _ := time.Parse("2006-01-02", to)
	span := t.Sub(f) + 24*time.Hour
	pf := f.Add(-span).Format("2006-01-02")
	pt := f.Add(-24 * time.Hour).Format("2006-01-02")
	months := span.Hours() / 24 / 30.44
	if months < 1 {
		months = 1
	}
	type agg struct {
		total, prev money.Cents
		count       int
		merchants   map[string]money.Cents
	}
	top := map[string]*agg{}
	leaf := map[string]*agg{}
	get := func(m map[string]*agg, k string) *agg {
		a := m[k]
		if a == nil {
			a = &agg{merchants: map[string]money.Cents{}}
			m[k] = a
		}
		return a
	}
	var total money.Cents
	for i := range txs {
		x := &txs[i]
		if x.Kind != "expense" && !isRefund(x) {
			continue
		}
		amt := x.Amount
		cat := x.Category
		if isRefund(x) {
			amt, cat = -amt, "refunds"
		}
		inCur := x.Date >= from && x.Date <= to
		inPrev := x.Date >= pf && x.Date <= pt
		if !inCur && !inPrev {
			continue
		}
		ta, la := get(top, ledger.Top(cat)), get(leaf, cat)
		if inCur {
			ta.total += amt
			la.total += amt
			ta.count++
			la.count++
			total += amt
			if x.Merchant != "" {
				ta.merchants[x.Merchant] += amt
				la.merchants[x.Merchant] += amt
			}
		} else {
			ta.prev += amt
			la.prev += amt
		}
	}
	stat := func(cat string, a *agg) CategoryStat {
		s := CategoryStat{Category: cat, Total: a.total, Count: a.count, Previous: a.prev,
			MonthlyAvg: money.FromFloat(a.total.Float() / months)}
		if total > 0 {
			s.Share = round3(a.total.Float() / total.Float())
		}
		if a.prev > 0 {
			s.Change = round3(a.total.Float()/a.prev.Float() - 1)
		}
		var best money.Cents
		for m, v := range a.merchants {
			if v > best {
				best, s.TopMerchant = v, m
			}
		}
		return s
	}
	var out []CategoryStat
	for cat, a := range top {
		if a.total == 0 && a.prev == 0 {
			continue
		}
		s := stat(cat, a)
		for lc, la := range leaf {
			if ledger.Top(lc) == cat && lc != cat && (la.total != 0 || la.prev != 0) {
				s.Children = append(s.Children, stat(lc, la))
			}
		}
		if la, ok := leaf[cat]; ok && len(s.Children) > 0 && la.total != 0 {
			s.Children = append(s.Children, stat(cat, la))
		}
		sort.Slice(s.Children, func(i, j int) bool { return s.Children[i].Total > s.Children[j].Total })
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Total > out[j].Total })
	return out
}

// Recurring is a cost that repeats on a schedule (subscriptions, bills,
// obligations), detected from the ledger rather than declared.
type Recurring struct {
	ID        int64       `json:"id,omitempty"`     // saved owner edit, if any
	Source    string      `json:"source"`           // detected | edited | manual
	Note      string      `json:"note,omitempty"`
	Merchant  string      `json:"merchant"`
	Category  string      `json:"category"`
	Cadence   string      `json:"cadence"` // monthly | quarterly | yearly
	Amount    money.Cents `json:"amount"`  // typical charge
	Monthly   money.Cents `json:"monthly"` // monthly equivalent
	Last      string      `json:"last"`
	Next      string      `json:"next"`
	Count     int         `json:"count"`
	Changed   bool        `json:"changed"` // last charge differs from typical by >10%
	LastAmount money.Cents `json:"last_amount"`
}

// DetectRecurring finds merchants charged in most of the last 6 months at a
// steady amount, plus yearly renewals.
func DetectRecurring(txs []ledger.Tx, now time.Time) []Recurring {
	cut := now.AddDate(-1, -1, 0).Format("2006-01-02")
	type hit struct {
		date string
		amt  money.Cents
		cat  string
	}
	by := map[string][]hit{}
	for _, t := range txs {
		if t.Kind != "expense" || t.Date < cut {
			continue
		}
		key := t.Merchant
		if key == "" {
			continue
		}
		by[key] = append(by[key], hit{t.Date, t.Amount, t.Category})
	}
	var out []Recurring
	recent := now.AddDate(0, -6, 0).Format("2006-01")
	for m, hs := range by {
		sort.Slice(hs, func(i, j int) bool { return hs[i].date < hs[j].date })
		months := map[string]money.Cents{}
		for _, h := range hs {
			if h.date[:7] >= recent {
				months[h.date[:7]] += h.amt
			}
		}
		last := hs[len(hs)-1]
		var amts []float64
		for _, v := range months {
			amts = append(amts, v.Float())
		}
		sort.Float64s(amts)
		if len(months) >= 5 && len(hs) <= len(months)*3 {
			med := amts[len(amts)/2]
			// Steady: most months within 35% of the median.
			steady := 0
			for _, a := range amts {
				if math.Abs(a-med) <= 0.35*med+1 {
					steady++
				}
			}
			if steady*10 < len(amts)*7 {
				continue
			}
			lastMonth := months[last.date[:7]]
			lt, _ := time.Parse("2006-01-02", last.date)
			out = append(out, Recurring{Source: "detected", Merchant: m, Category: last.cat, Cadence: "monthly", Amount: money.FromFloat(med),
				Monthly: money.FromFloat(med), Last: last.date, Next: lt.AddDate(0, 1, 0).Format("2006-01-02"), Count: len(hs),
				LastAmount: lastMonth, Changed: math.Abs(lastMonth.Float()-med) > 0.1*med+1})
			continue
		}
		// Yearly: two charges 11–13 months apart with similar amounts.
		if len(hs) >= 2 && len(hs) <= 3 {
			a, b := hs[len(hs)-2], hs[len(hs)-1]
			ta, _ := time.Parse("2006-01-02", a.date)
			tb, _ := time.Parse("2006-01-02", b.date)
			gap := tb.Sub(ta).Hours() / 24
			if gap > 330 && gap < 400 && math.Abs(a.amt.Float()-b.amt.Float()) <= 0.25*b.amt.Float() && b.amt >= 1000 {
				out = append(out, Recurring{Source: "detected", Merchant: m, Category: b.cat, Cadence: "yearly", Amount: b.amt, Monthly: b.amt / 12,
					Last: b.date, Next: tb.AddDate(1, 0, 0).Format("2006-01-02"), Count: len(hs), LastAmount: b.amt})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Monthly > out[j].Monthly })
	return out
}

// Anomaly is a category running well above its own normal this month.
type Anomaly struct {
	Category string      `json:"category"`
	Spent    money.Cents `json:"spent"`
	Typical  money.Cents `json:"typical"` // median of the previous 12 months
	Ratio    float64     `json:"ratio"`
	Projected money.Cents `json:"projected"` // month-end pace (current month only)
}

// Anomalies compares month's top-level spending with each category's
// median month over the 12 before it.
func Anomalies(txs []ledger.Tx, month string, now time.Time) []Anomaly {
	by := map[string]map[string]money.Cents{}
	for i := range txs {
		t := &txs[i]
		if t.Kind != "expense" {
			continue
		}
		c := ledger.Top(t.Category)
		if by[c] == nil {
			by[c] = map[string]money.Cents{}
		}
		by[c][t.Date[:7]] += t.Amount
	}
	mt, _ := time.Parse("2006-01", month)
	frac := 1.0
	if month == now.Format("2006-01") {
		dim := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		frac = float64(now.Day()) / float64(dim)
	}
	var out []Anomaly
	for c, m := range by {
		var hist []float64
		for i := 1; i <= 12; i++ {
			hist = append(hist, m[mt.AddDate(0, -i, 0).Format("2006-01")].Float())
		}
		sort.Float64s(hist)
		med := (hist[5] + hist[6]) / 2
		cur := m[month]
		proj := money.FromFloat(cur.Float() / math.Max(frac, 0.2))
		if med < 30 || cur.Float() < 50 {
			continue
		}
		ratio := proj.Float() / med
		if ratio >= 1.4 {
			out = append(out, Anomaly{Category: c, Spent: cur, Typical: money.FromFloat(med), Ratio: round3(ratio), Projected: proj})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ratio > out[j].Ratio })
	return out
}

// TopExpenses lists the largest single expenses in a window.
func TopExpenses(txs []ledger.Tx, from, to string, n int) []ledger.Tx {
	var out []ledger.Tx
	for _, t := range txs {
		if t.Kind == "expense" && t.Date >= from && t.Date <= to {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Amount > out[j].Amount })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// MerchantTotals ranks merchants by spend in a window.
func MerchantTotals(txs []ledger.Tx, from, to string, n int) []NamedCount {
	m := map[string]*NamedCount{}
	for _, t := range txs {
		if t.Kind != "expense" || t.Date < from || t.Date > to || t.Merchant == "" {
			continue
		}
		x := m[t.Merchant]
		if x == nil {
			x = &NamedCount{Name: t.Merchant, Category: t.Category}
			m[t.Merchant] = x
		}
		x.Amount += t.Amount
		x.Count++
	}
	out := make([]NamedCount, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Amount > out[j].Amount })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

type NamedCount struct {
	Name     string      `json:"name"`
	Category string      `json:"category"`
	Amount   money.Cents `json:"amount"`
	Count    int         `json:"count"`
}

// Window returns [from,to] for common presets relative to now.
func Window(preset string, now time.Time) (string, string) {
	d := func(t time.Time) string { return t.Format("2006-01-02") }
	switch strings.ToLower(preset) {
	case "month":
		return d(time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)), d(now)
	case "last_month":
		f := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, time.UTC)
		return d(f), d(f.AddDate(0, 1, -1))
	case "3m":
		return d(now.AddDate(0, -3, 0).AddDate(0, 0, 1)), d(now)
	case "6m":
		return d(now.AddDate(0, -6, 0).AddDate(0, 0, 1)), d(now)
	case "ytd":
		return d(time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)), d(now)
	case "last_year":
		return d(time.Date(now.Year()-1, 1, 1, 0, 0, 0, 0, time.UTC)), d(time.Date(now.Year()-1, 12, 31, 0, 0, 0, 0, time.UTC))
	case "all":
		return "2000-01-01", d(now)
	}
	return d(now.AddDate(-1, 0, 0).AddDate(0, 0, 1)), d(now)
}

// PacePoint is cumulative spending by day of month.
type PacePoint struct {
	Day       int          `json:"day"`
	Current   *money.Cents `json:"current,omitempty"` // this month (only up to today)
	LastMonth money.Cents  `json:"last_month"`
	Typical   money.Cents  `json:"typical"` // average of the previous six months
}

// Pace compares this month's cumulative spending with last month and the
// six-month norm, day by day — "am I spending faster than usual?".
func Pace(txs []ledger.Tx, now time.Time) []PacePoint {
	month := now.Format("2006-01")
	byMonthDay := map[string]map[int]money.Cents{}
	for i := range txs {
		t := &txs[i]
		if t.Kind != "expense" && !isRefund(t) {
			continue
		}
		amt := t.Amount
		if isRefund(t) {
			amt = -amt
		}
		m := t.Date[:7]
		if byMonthDay[m] == nil {
			byMonthDay[m] = map[int]money.Cents{}
		}
		var d int
		fmt.Sscanf(t.Date[8:], "%d", &d)
		byMonthDay[m][d] += amt
	}
	cum := func(m string, day int) money.Cents {
		var s money.Cents
		for d := 1; d <= day; d++ {
			s += byMonthDay[m][d]
		}
		return s
	}
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, -1, 0).Format("2006-01")
	var prev []string
	for i := 1; i <= 6; i++ {
		prev = append(prev, first.AddDate(0, -i, 0).Format("2006-01"))
	}
	out := make([]PacePoint, 0, 31)
	for d := 1; d <= 31; d++ {
		p := PacePoint{Day: d, LastMonth: cum(last, d)}
		var s money.Cents
		for _, m := range prev {
			s += cum(m, d)
		}
		p.Typical = s / money.Cents(len(prev))
		if d <= now.Day() {
			c := cum(month, d)
			p.Current = &c
		}
		out = append(out, p)
	}
	return out
}

// TagTotals sums expenses (net of refunds) per tag in a window — who and
// what the money was for, across categories.
func TagTotals(txs []ledger.Tx, from, to string) []NamedCount {
	m := map[string]*NamedCount{}
	for i := range txs {
		t := &txs[i]
		if t.Date < from || t.Date > to || (t.Kind != "expense" && !isRefund(t)) {
			continue
		}
		for _, tag := range t.Tags {
			x := m[tag]
			if x == nil {
				x = &NamedCount{Name: tag}
				m[tag] = x
			}
			if isRefund(t) {
				x.Amount -= t.Amount
			} else {
				x.Amount += t.Amount
				x.Count++
			}
		}
	}
	out := make([]NamedCount, 0, len(m))
	for _, v := range m {
		if v.Amount > 0 {
			out = append(out, *v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Amount > out[j].Amount })
	return out
}
