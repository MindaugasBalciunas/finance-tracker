package cfo

import (
	"database/sql"
	"sort"
	"strings"
	"time"

	"ft/internal/ledger"
	"ft/internal/merchant"
	"ft/internal/money"
	"ft/internal/plan"
)

// Optional spending insights: where the money for things you could do
// without goes, and the patterns that usually mean it leaks rather than buys
// a better life. "Optional" is the Free money card's split — an expense no
// fixed line claims, under a category not marked essential — so every figure
// here is a slice of the card's Optional and the Ledger's need=optional view.

// Thresholds, named so the page can say them.
const (
	SmallPurchase = money.Cents(15 * 100)  // "small" is under €15
	SmallMinCount = 6                      // that many in 90 days makes it a habit
	PaydayDays    = 3                      // payday and the two days after it
	OneOffMin     = money.Cents(250 * 100) // a big one-off, as plan.OneOff
)

type OptMonth struct {
	Month     string      `json:"month"`
	Essential money.Cents `json:"essential"`
	Optional  money.Cents `json:"optional"`
}

// OptLine is a category or merchant: this month, a usual month (average of
// the last six complete ones) and the last twelve complete months.
type OptLine struct {
	Key   string      `json:"key"`
	Month money.Cents `json:"month"`
	Usual money.Cents `json:"usual"`
	Year  money.Cents `json:"year"`
	Count int         `json:"count"` // payments in the twelve months
}

type TxBrief struct {
	ID       int64       `json:"id"`
	Date     string      `json:"date"`
	Merchant string      `json:"merchant"`
	Category string      `json:"category"`
	Amount   money.Cents `json:"amount"`
}

// WhileOver: optional spending after the day a month had used up its free
// money — money the month didn't have. Paid-from-fund rows don't count.
type WhileOver struct {
	Year      money.Cents     `json:"year"`
	ThisMonth money.Cents     `json:"this_month"`
	Months    []WhileOverDays `json:"months"`
	Items     []TxBrief       `json:"items"`
}

type WhileOverDays struct {
	Month  string      `json:"month"`
	From   int         `json:"from"` // the first day after the month ran out
	Amount money.Cents `json:"amount"`
}

// SmallOften: under SmallPurchase, SmallMinCount+ times in 90 days at one place.
type SmallOften struct {
	Merchant string      `json:"merchant"`
	Count    int         `json:"count"` // in the last 90 days
	Spent    money.Cents `json:"spent"`
	Yearly   money.Cents `json:"yearly"`
}

// Payday: optional spending in the PaydayDays after salary arrives against
// any other PaydayDays of the month, averaged over the last six salaries.
type Payday struct {
	After  money.Cents `json:"after"`
	Usual  money.Cents `json:"usual"`
	Extra  money.Cents `json:"extra"` // a year, when After is clearly higher (≥25% and €100 a year)
	Months int         `json:"months"`
}

// Creeping: a place you now spend more at (last three complete months)
// than in the three before.
type Creeping struct {
	Merchant string      `json:"merchant"`
	Before   money.Cents `json:"before"` // a month, months 4–6 ago
	Now      money.Cents `json:"now"`    // a month, last 3 complete months
	Yearly   money.Cents `json:"yearly"` // (now − before) × 12
}

// Convenience: delivery and fast food against groceries, a month on average
// over the last three complete months.
type Convenience struct {
	Convenience money.Cents `json:"convenience"`
	Groceries   money.Cents `json:"groceries"`
	Yearly      money.Cents `json:"yearly"`
}

type OneOffs struct {
	Year  money.Cents `json:"year"` // since the first of the twelve months, this month included
	Items []TxBrief   `json:"items"`
}

type Leaks struct {
	WhileOver   WhileOver    `json:"while_over"`
	SmallOften  []SmallOften `json:"small_often"`
	Payday      *Payday      `json:"payday,omitempty"`
	Creeping    []Creeping   `json:"creeping"`
	Convenience *Convenience `json:"convenience,omitempty"`
	OneOffs     OneOffs      `json:"one_offs"`
}

type OptionalReport struct {
	Month      string      `json:"month"`
	Spent      money.Cents `json:"spent"` // optional so far this month (before refunds, as the card)
	Usual      money.Cents `json:"usual"` // median of the last six complete months
	Free       money.Cents `json:"free"`  // this month's free money (income − fixed − saving)
	Year       money.Cents `json:"year"`  // last twelve complete months
	Months     []OptMonth  `json:"months"`
	Categories []OptLine   `json:"categories"`
	Merchants  []OptLine   `json:"merchants"`
	Leaks      Leaks       `json:"leaks"`
}

// optRow is an expense with what the insights need to know about it.
type optRow struct {
	*ledger.Tx
	month    string
	day      int
	optional bool // not essential, not claimed by a fixed line
	free     bool // not claimed by a fixed line
	fund     bool // paid by a fund that month
}

// OptionalInsights reads the last twelve complete months and this one.
func OptionalInsights(d *sql.DB, now time.Time) (*OptionalReport, error) {
	thisM := now.Format("2006-01")
	first := ledger.AddMonths(now, -12)
	// To the end of this month, as the plan engine reads it, so this month's
	// figure is the Free money card's even with a row dated ahead.
	monthEnd := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	txs, err := ledger.All(d, ledger.Filter{From: first + "-01", To: monthEnd})
	if err != nil {
		return nil, err
	}
	budgets, err := plan.List(d, false)
	if err != nil {
		return nil, err
	}
	cats, err := ledger.CategoryMap(d)
	if err != nil {
		return nil, err
	}
	months := append(lastMonths(now, 12), thisM)
	reports, err := BudgetReports(d, months, now)
	if err != nil {
		return nil, err
	}
	return optionalReport(txs, budgets, cats, reports, months, now), nil
}

func optionalReport(txs []ledger.Tx, budgets []plan.Budget, cats map[string]ledger.Category, reports map[string]*plan.Report, months []string, now time.Time) *OptionalReport {
	thisM := months[len(months)-1]
	complete := months[:len(months)-1]
	r := &OptionalReport{Month: thisM, Months: []OptMonth{}, Categories: []OptLine{}, Merchants: []OptLine{},
		Leaks: Leaks{WhileOver: WhileOver{Months: []WhileOverDays{}, Items: []TxBrief{}}, SmallOften: []SmallOften{}, Creeping: []Creeping{}, OneOffs: OneOffs{Items: []TxBrief{}}}}
	if fm := reports[thisM]; fm != nil && fm.FreeMoney != nil {
		r.Free = fm.FreeMoney.Free
	}

	var rows []optRow
	var salaryDays []string
	for i := range txs {
		t := &txs[i]
		if t.Kind == "income" && ledger.Top(t.Category) == "salary" {
			salaryDays = append(salaryDays, t.Date)
		}
		if t.Kind != "expense" {
			continue
		}
		o := optRow{Tx: t, month: t.Date[:7], day: dayFrom(t.Date), free: true}
		for _, b := range budgets {
			if !b.Matches(t) {
				continue
			}
			if b.Kind == "fixed" {
				o.free = false
			}
			if b.Kind == "spending" && b.FundIn(o.month) {
				o.fund = true
			}
		}
		o.optional = o.free && !cats[t.Category].Essential
		rows = append(rows, o)
	}

	// Months, and the usual month.
	byMonth := map[string]*OptMonth{}
	for _, m := range months {
		byMonth[m] = &OptMonth{Month: m}
	}
	for _, o := range rows {
		mm := byMonth[o.month]
		if mm == nil || !o.free {
			continue
		}
		if o.optional {
			mm.Optional += o.Amount
		} else {
			mm.Essential += o.Amount
		}
	}
	var last6 []money.Cents
	for i, m := range months {
		r.Months = append(r.Months, *byMonth[m])
		if m != thisM {
			r.Year += byMonth[m].Optional
			if i >= len(complete)-6 {
				last6 = append(last6, byMonth[m].Optional)
			}
		}
	}
	r.Spent = byMonth[thisM].Optional
	r.Usual = medianCents(last6)

	// Where it goes: categories and merchants.
	recent6 := map[string]bool{}
	for _, m := range complete[max(0, len(complete)-6):] {
		recent6[m] = true
	}
	cat, mer := map[string]*OptLine{}, map[string]*OptLine{}
	merName := map[string]string{}
	counted := map[*OptLine]map[int64]bool{} // purchases already counted per line
	add := func(set map[string]*OptLine, key string, o optRow) {
		l := set[key]
		if l == nil {
			l = &OptLine{Key: key}
			set[key] = l
			counted[l] = map[int64]bool{}
		}
		switch {
		case o.month == thisM:
			l.Month += o.Amount
		default:
			l.Year += o.Amount
			if !counted[l][o.purchase()] {
				counted[l][o.purchase()] = true
				l.Count++
			}
			if recent6[o.month] {
				l.Usual += o.Amount
			}
		}
	}
	for _, o := range rows {
		if !o.optional {
			continue
		}
		add(cat, o.Category, o)
		if k := merchant.Key(o.Merchant); k != "" {
			if merName[k] == "" {
				merName[k] = o.Merchant
			}
			add(mer, k, o)
		}
	}
	top := func(set map[string]*OptLine, n int, name func(string) string) []OptLine {
		out := []OptLine{}
		for _, l := range set {
			l.Usual /= 6
			l.Key = name(l.Key)
			out = append(out, *l)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Year != out[j].Year {
				return out[i].Year > out[j].Year
			}
			return out[i].Key < out[j].Key
		})
		return out[:min(n, len(out))]
	}
	r.Categories = top(cat, 8, func(k string) string { return k })
	r.Merchants = top(mer, 10, func(k string) string { return merName[k] })

	r.Leaks.WhileOver = whileOver(rows, reports, months, thisM)
	r.Leaks.SmallOften = smallOften(rows, now)
	r.Leaks.Payday = payday(rows, salaryDays, complete)
	r.Leaks.Creeping = creeping(rows, merName, complete)
	r.Leaks.Convenience = convenience(rows, complete)
	r.Leaks.OneOffs = oneOffs(rows, thisM, complete)
	return r
}

// purchase is the receipt a row belongs to: the parts of a split are one
// purchase, as Habits and Merchants count them.
func (o optRow) purchase() int64 {
	if o.SplitOf > 0 {
		return o.SplitOf
	}
	return o.ID
}

func brief(o optRow) TxBrief {
	return TxBrief{ID: o.ID, Date: o.Date, Merchant: o.Merchant, Category: o.Category, Amount: o.Amount}
}

func topItems(items []optRow, n int) []TxBrief {
	sort.SliceStable(items, func(i, j int) bool { return items[i].Amount > items[j].Amount })
	out := []TxBrief{}
	for _, o := range items[:min(n, len(items))] {
		out = append(out, brief(o))
	}
	return out
}

// whileOver finds, each month, the first day free spending passed the free
// money planned for day-to-day (income − fixed − saving − fund set-asides),
// and adds up the optional payments made after it.
func whileOver(rows []optRow, reports map[string]*plan.Report, months []string, thisM string) WhileOver {
	w := WhileOver{Months: []WhileOverDays{}, Items: []TxBrief{}}
	var items []optRow
	for _, m := range months {
		rep := reports[m]
		if rep == nil || rep.IncomeBase <= 0 {
			continue // no income to measure against (a new book): no verdict
		}
		planned := rep.IncomeBase - rep.FixedPlanned - rep.SavingPlanned - rep.FundContributions
		ran, cum := 0, money.Cents(0)
		if planned <= 0 {
			ran = -1 // nothing was free: every optional payment that month
		} else {
			for day := 1; day <= 31; day++ {
				if cum += rep.FreeSpentByDay[m][day]; cum > planned {
					ran = day
					break
				}
			}
		}
		if ran == 0 {
			continue
		}
		var amt money.Cents
		for _, o := range rows {
			if o.month == m && o.optional && !o.fund && o.day > ran {
				amt += o.Amount
				items = append(items, o)
			}
		}
		if amt == 0 {
			continue
		}
		w.Months = append(w.Months, WhileOverDays{Month: m, From: max(ran+1, 1), Amount: amt})
		if m == thisM {
			w.ThisMonth = amt
		} else {
			w.Year += amt
		}
	}
	w.Items = topItems(items, 5)
	return w
}

func smallOften(rows []optRow, now time.Time) []SmallOften {
	from := now.AddDate(0, 0, -90).Format("2006-01-02")
	type acc struct {
		name  string
		count int
		spent money.Cents
	}
	// Whole purchases first: three parts of one €12 receipt are one small
	// purchase, not three.
	type buy struct {
		key, name string
		amount    money.Cents
	}
	buys := map[int64]*buy{}
	var order []int64
	for _, o := range rows {
		k := merchant.Key(o.Merchant)
		if !o.optional || k == "" || o.Date < from {
			continue
		}
		p := o.purchase()
		if buys[p] == nil {
			buys[p] = &buy{key: k, name: o.Merchant}
			order = append(order, p)
		}
		buys[p].amount += o.Amount
	}
	by := map[string]*acc{}
	for _, p := range order {
		b := buys[p]
		if b.amount >= SmallPurchase {
			continue
		}
		if by[b.key] == nil {
			by[b.key] = &acc{name: b.name}
		}
		by[b.key].count++
		by[b.key].spent += b.amount
	}
	out := []SmallOften{}
	for _, a := range by {
		if a.count >= SmallMinCount {
			out = append(out, SmallOften{Merchant: a.name, Count: a.count, Spent: a.spent, Yearly: a.spent * 365 / 90})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Yearly > out[j].Yearly })
	return out[:min(6, len(out))]
}

// payday compares payday and the days after it (PaydayDays in all, running
// into the next month when salary comes on the 30th) with the rest of the
// salary's month, per PaydayDays, over the last six salaries in complete
// months.
func payday(rows []optRow, salaryDays []string, complete []string) *Payday {
	in := map[string]bool{}
	for _, m := range complete[max(0, len(complete)-6):] {
		in[m] = true
	}
	// The first salary of each month, by the month it was paid in.
	first := map[string]string{}
	for _, s := range salaryDays {
		if m := s[:7]; in[m] && (first[m] == "" || s < first[m]) {
			first[m] = s
		}
	}
	if len(first) < 3 {
		return nil
	}
	var after, rest money.Cents
	var restDays int
	for m, pay := range first {
		start, _ := time.Parse("2006-01-02", pay)
		end := start.AddDate(0, 0, PaydayDays).Format("2006-01-02")
		dim := time.Date(start.Year(), start.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		restDays += dim - min(PaydayDays, dim-start.Day()+1) // the window's days inside the month
		for _, o := range rows {
			if !o.optional {
				continue
			}
			switch {
			case o.Date >= pay && o.Date < end:
				after += o.Amount
			case o.month == m:
				rest += o.Amount
			}
		}
	}
	p := &Payday{Months: len(first)}
	p.After = after / money.Cents(len(first))
	if restDays > 0 {
		p.Usual = rest * PaydayDays / money.Cents(restDays)
	}
	// A splurge is clearly more — a quarter above a usual three days and
	// €100 a year — not €8 of noise (a real book's first run).
	if p.After*4 >= p.Usual*5 && (p.After-p.Usual)*12 >= 100*100 {
		p.Extra = (p.After - p.Usual) * 12
	}
	return p
}

func creeping(rows []optRow, names map[string]string, complete []string) []Creeping {
	n := len(complete)
	if n < 6 {
		return []Creeping{}
	}
	now3, before3 := map[string]bool{}, map[string]bool{}
	for _, m := range complete[n-3:] {
		now3[m] = true
	}
	for _, m := range complete[n-6 : n-3] {
		before3[m] = true
	}
	nowS, beforeS := map[string]money.Cents{}, map[string]money.Cents{}
	seen := map[string]map[string]bool{} // months a place was paid in, of the last three
	for _, o := range rows {
		k := merchant.Key(o.Merchant)
		if !o.optional || k == "" {
			continue
		}
		if now3[o.month] {
			nowS[k] += o.Amount
			if seen[k] == nil {
				seen[k] = map[string]bool{}
			}
			seen[k][o.month] = true
		}
		if before3[o.month] {
			beforeS[k] += o.Amount
		}
	}
	out := []Creeping{}
	for k, v := range nowS {
		nowM, beforeM := v/3, beforeS[k]/3
		// A habit that grew: paid before, and in at least two of the last
		// three months — a new trip agency or phone bill is new, not creeping,
		// and one flight or concert isn't a habit — now half as much again,
		// and at least €10 a month more.
		if beforeM > 0 && len(seen[k]) >= 2 && nowM-beforeM >= 10*100 && nowM*2 >= beforeM*3 {
			out = append(out, Creeping{Merchant: names[k], Before: beforeM, Now: nowM, Yearly: (nowM - beforeM) * 12})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Yearly > out[j].Yearly })
	return out[:min(6, len(out))]
}

func convenience(rows []optRow, complete []string) *Convenience {
	n := len(complete)
	in := map[string]bool{}
	for _, m := range complete[max(0, n-3):] {
		in[m] = true
	}
	under := func(cat, c string) bool { return cat == c || strings.HasPrefix(cat, c+".") }
	var conv, groc money.Cents
	for _, o := range rows {
		if !in[o.month] || !o.free {
			continue
		}
		switch {
		case o.optional && (under(o.Category, "food.delivery") || under(o.Category, "food.fast_food")):
			conv += o.Amount
		case under(o.Category, "food.groceries"):
			groc += o.Amount
		}
	}
	// Worth a card from €20 a month — €5 of fast food is not a pattern.
	if conv/3 < 20*100 {
		return nil
	}
	c := &Convenience{Convenience: conv / 3, Groceries: groc / 3}
	c.Yearly = c.Convenience * 12
	return c
}

func oneOffs(rows []optRow, thisM string, complete []string) OneOffs {
	in := map[string]bool{thisM: true}
	for _, m := range complete {
		in[m] = true
	}
	var items []optRow
	o := OneOffs{Items: []TxBrief{}}
	for _, r := range rows {
		if !in[r.month] || !r.optional || r.fund || r.Amount < OneOffMin || hasTrip(r.Tags) {
			continue
		}
		items = append(items, r)
		o.Year += r.Amount // the twelve months and this one, like the list
	}
	o.Items = topItems(items, 6)
	return o
}

func hasTrip(tags []string) bool {
	for _, t := range tags {
		if strings.HasPrefix(t, "trip:") {
			return true
		}
	}
	return false
}
