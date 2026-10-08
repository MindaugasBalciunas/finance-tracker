// Package cfo assembles the cross-domain views — the Home overview, the
// plan report and the monthly briefing — from ledger, wealth, plan and
// insights. The API and the AI assistant both read these, so a number on
// the Home screen and in a chat answer is the same number.
package cfo

import (
	"database/sql"
	"sort"
	"strings"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

type Change struct {
	Value money.Cents `json:"value"`
	Delta money.Cents `json:"delta"`
}

type Overview struct {
	Cash          *CashPlan              `json:"cash,omitempty"` // per-account cash until the next payday
	Actions       []Action               `json:"actions"`        // "needs you": one list of things to do, most urgent first
	Date          string                 `json:"date"`
	NetWorth      money.Cents            `json:"net_worth"`
	Liquid        money.Cents            `json:"liquid"`
	Debt          money.Cents            `json:"debt"`
	ByGroup       map[string]money.Cents `json:"by_group"`
	NetWorth30d   money.Cents            `json:"net_worth_30d"` // change vs 30 days ago
	NetWorthYTD   money.Cents            `json:"net_worth_ytd"` // change since 31 Dec
	NetWorth12m   money.Cents            `json:"net_worth_12m"` // change vs a year ago
	Liquid30d     money.Cents            `json:"liquid_30d"`
	LiquidYTD     money.Cents            `json:"liquid_ytd"`
	Liquid12m     money.Cents            `json:"liquid_12m"`
	Spark         []SparkPoint           `json:"spark"` // 24 month-ends
	Month         insights.Flow          `json:"month"` // this month so far
	LastMonth     insights.Flow          `json:"last_month"`
	Avg12         insights.Flow          `json:"avg12"`          // trailing 12 complete months, monthly average
	Year          insights.Flow          `json:"year"`           // year to date
	MonthProgress float64                `json:"month_progress"` // share of the month elapsed
	Plan          PlanPulse              `json:"plan"`
	Emergency     insights.Emergency     `json:"emergency"`
	FIProgress    float64                `json:"fi_progress"`
	YearsToFI     float64                `json:"years_to_fi"`
	Anomalies     []insights.Anomaly     `json:"anomalies"`
	Upcoming      []insights.Recurring   `json:"upcoming"` // recurring charges expected in the next 14 days
	Stale         map[string]string      `json:"stale"`
	InboxOpen     int                    `json:"inbox_open"`
	Checks        []Check                `json:"checks"`
	ReviewMonth   string                 `json:"review_month,omitempty"` // first week: last month's review is ready
	Recent        []ledger.Tx            `json:"recent"`
}

type SparkPoint struct {
	Date   string      `json:"date"`
	Value  money.Cents `json:"value"`
	Liquid money.Cents `json:"liquid"`
}

type PlanPulse struct {
	SafeToSpend money.Cents `json:"safe_to_spend"` // free money left this month
	IncomeBase  money.Cents `json:"income_base"`
	// Daily view of the same money.
	FreeSpent      money.Cents `json:"free_spent"`       // free spending so far this month
	DaysLeft       int         `json:"days_left"`        // including today
	PerDayLeft     money.Cents `json:"per_day_left"`     // safe to spend ÷ days left
	AvgDay         money.Cents `json:"avg_day"`          // this month's free spending per elapsed day
	TypicalOneOffs money.Cents `json:"typical_one_offs"` // one-offs (€250+) per month over the same months, left out of typical
	TypicalDay     money.Cents `json:"typical_day"`      // everyday spending per day (one-offs of €250+ left out), previous 6 complete months
	ExpectedDay    money.Cents `json:"expected_day"`     // pace used for the projection (this month blended with typical)
	ProjectedLeft  money.Cents `json:"projected_left"`   // what is left at month end at the expected pace
	Over           []LineBrief `json:"over"`             // spending lines over budget
	Lines          []LineBrief `json:"lines"`            // busiest spending lines (most of budget used first)
	Spent          money.Cents `json:"spent"`
	Budgeted       money.Cents `json:"budgeted"`
	// How safe-to-spend is reached: income base − fixed − saving − fund
	// set-asides − free spending so far.
	IncomeActual  money.Cents  `json:"income_actual"`
	FixedPlanned  money.Cents  `json:"fixed_planned"`
	FixedSpent    money.Cents  `json:"fixed_spent"`
	SavingPlanned money.Cents  `json:"saving_planned"`
	SavedActual   money.Cents  `json:"saved_actual"`
	FundSetAside  money.Cents  `json:"fund_set_aside"`
	Fixed         []FixedBrief `json:"fixed"` // each obligation: paid, or when it is due
	// The month as a timeline: free spending day by day (cumulative), the
	// typical month's curve, and dated events (obligations, income).
	Days   []DayPoint   `json:"days"`
	Events []MonthEvent `json:"events"`
}

type DayPoint struct {
	Day     int          `json:"day"`
	Spent   money.Cents  `json:"spent"`         // free spending that day (0 after today)
	Cum     *money.Cents `json:"cum,omitempty"` // cumulative, up to today only
	Typical money.Cents  `json:"typical"`       // average cumulative everyday spending (no one-offs) by this day, last 6 months
}

type MonthEvent struct {
	Day    int         `json:"day"`
	Kind   string      `json:"kind"` // fixed | income
	Label  string      `json:"label"`
	Amount money.Cents `json:"amount"`
	Done   bool        `json:"done"`
}

type FixedBrief struct {
	Name     string      `json:"name"`
	Budgeted money.Cents `json:"budgeted"`
	Spent    money.Cents `json:"spent"`
	Due      string      `json:"due,omitempty"`  // next expected payment this month, from recurring costs
	Paid     string      `json:"paid,omitempty"` // last payment this month
}

type LineBrief struct {
	Name      string      `json:"name"`
	Spent     money.Cents `json:"spent"`
	Budgeted  money.Cents `json:"budgeted"`
	Remaining money.Cents `json:"remaining"`
}

// BudgetReport runs the plan engine for a month.
func BudgetReport(d *sql.DB, month string, now time.Time) (*plan.Report, error) {
	budgets, err := plan.List(d, false)
	if err != nil {
		return nil, err
	}
	sel, _ := time.Parse("2006-01", month)
	from := sel.AddDate(-1, 0, 0).Format("2006-01") + "-01"
	// Funds need their whole history since their start month.
	for _, b := range budgets {
		if b.Fund && b.StartMonth != "" && b.StartMonth+"-01" < from {
			from = b.StartMonth + "-01"
		}
	}
	// A few days into the next month, so its early salary reaches this month.
	extra := plan.MaxSalaryDays(plan.LoadSettings(d).Salary())
	txs, err := ledger.All(d, ledger.Filter{From: from, To: sel.AddDate(0, 1, extra-1).Format("2006-01-02")})
	if err != nil {
		return nil, err
	}
	// The income base looks back 12 complete months from today.
	inc, err := ledger.All(d, ledger.Filter{From: now.AddDate(-1, -1, 0).Format("2006-01") + "-01", Kind: "income"})
	if err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	for _, t := range txs {
		seen[t.ID] = true
	}
	for _, t := range inc {
		if !seen[t.ID] {
			txs = append(txs, t)
		}
	}
	return plan.Compute(budgets, txs, month, now, plan.LoadSettings(d)), nil
}

// BuildOverview is the Home screen.
func BuildOverview(d *sql.DB, now time.Time, inboxOpen int) (*Overview, error) {
	book, err := wealth.LoadBook(d)
	if err != nil {
		return nil, err
	}
	today := now.Format("2006-01-02")
	snap := book.SnapshotAt(today, true)
	o := &Overview{Date: today, NetWorth: snap.NetWorth, Liquid: snap.Liquid, Debt: snap.Debt, ByGroup: snap.ByGroup, Stale: snap.Stale, InboxOpen: inboxOpen}
	ago30 := book.SnapshotAt(now.AddDate(0, 0, -30).Format("2006-01-02"), false)
	yearEnd := book.SnapshotAt(time.Date(now.Year()-1, 12, 31, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), false)
	ago12 := book.SnapshotAt(now.AddDate(-1, 0, 0).Format("2006-01-02"), false)
	o.NetWorth30d, o.Liquid30d = snap.NetWorth-ago30.NetWorth, snap.Liquid-ago30.Liquid
	o.NetWorthYTD, o.LiquidYTD = snap.NetWorth-yearEnd.NetWorth, snap.Liquid-yearEnd.Liquid
	o.NetWorth12m, o.Liquid12m = snap.NetWorth-ago12.NetWorth, snap.Liquid-ago12.Liquid
	for _, h := range book.History(now.AddDate(-2, 0, 0).Format("2006-01-02"), today, "month") {
		o.Spark = append(o.Spark, SparkPoint{h.Date, h.NetWorth, h.Liquid})
	}

	txs, err := ledger.All(d, ledger.Filter{From: now.AddDate(-2, 0, 0).Format("2006-01") + "-01"})
	if err != nil {
		return nil, err
	}
	cats, _ := ledger.CategoryMap(d)
	flows := insights.CashFlow(txs, cats, "month", plan.LoadSettings(d).Salary())
	thisM := now.Format("2006-01")
	lastM := now.AddDate(0, -1, 0).Format("2006-01")
	var avg insights.Flow
	n := 0
	for _, f := range flows {
		switch {
		case f.Period == thisM:
			o.Month = f
		case f.Period == lastM:
			o.LastMonth = f
		}
		if f.Period < thisM && f.Period >= now.AddDate(-1, 0, 0).Format("2006-01") {
			avg.Income += f.Income
			avg.Spending += f.Spending
			avg.Essential += f.Essential
			avg.Discretionary += f.Discretionary
			avg.Saved += f.Saved
			avg.Invested += f.Invested
			avg.Principal += f.Principal
			n++
		}
		if f.Period[:4] == thisM[:4] {
			o.Year.Income += f.Income
			o.Year.Spending += f.Spending
			o.Year.Saved += f.Saved
			o.Year.Invested += f.Invested
			o.Year.Essential += f.Essential
			o.Year.Discretionary += f.Discretionary
		}
	}
	if n > 0 {
		div := money.Cents(n)
		avg.Income, avg.Spending, avg.Essential, avg.Discretionary = avg.Income/div, avg.Spending/div, avg.Essential/div, avg.Discretionary/div
		avg.Saved, avg.Invested, avg.Principal = avg.Saved/div, avg.Invested/div, avg.Principal/div
		if avg.Income > 0 {
			avg.SavingsRate = float64(avg.Saved) / float64(avg.Income)
		}
	}
	avg.Period = "avg12"
	o.Avg12 = avg
	o.Year.Period = thisM[:4]
	if o.Year.Income > 0 {
		o.Year.SavingsRate = float64(o.Year.Saved) / float64(o.Year.Income)
	}
	dim := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	o.MonthProgress = float64(now.Day()) / float64(dim)

	var fixedCats [][]string
	if r, err := BudgetReport(d, thisM, now); err == nil {
		o.Plan = PlanPulse{SafeToSpend: r.SafeToSpend, IncomeBase: r.IncomeBase, IncomeActual: r.IncomeActual,
			FixedPlanned: r.FixedPlanned, FixedSpent: r.FixedSpent, SavingPlanned: r.SavingPlanned, SavedActual: r.SavedActual, FundSetAside: r.FundContributions}
		o.Plan.fillDaily(r.FreeSpentByMonth, r.EverydayByDay, now)
		o.Plan.fillTimeline(r.FreeSpentByDay, r.EverydayByDay, now)
		for _, l := range r.Lines {
			if l.Kind == "fixed" {
				o.Plan.Fixed = append(o.Plan.Fixed, FixedBrief{Name: l.Name, Budgeted: l.Budgeted, Spent: l.Spent})
				fixedCats = append(fixedCats, l.Categories)
			}
			if l.Kind != "spending" {
				continue
			}
			o.Plan.Spent += l.Spent
			o.Plan.Budgeted += l.Budgeted
			b := LineBrief{l.Name, l.Spent, l.Budgeted, l.Remaining}
			if l.Remaining < 0 {
				o.Plan.Over = append(o.Plan.Over, b)
			}
			if l.Budgeted > 0 {
				o.Plan.Lines = append(o.Plan.Lines, b)
			}
		}
		sort.SliceStable(o.Plan.Lines, func(i, j int) bool {
			return float64(o.Plan.Lines[i].Spent)/float64(o.Plan.Lines[i].Budgeted) > float64(o.Plan.Lines[j].Spent)/float64(o.Plan.Lines[j].Budgeted)
		})
		if len(o.Plan.Lines) > 5 {
			o.Plan.Lines = o.Plan.Lines[:5]
		}
	}
	fi := insights.ComputeFI(txs, cats, book, plan.LoadSettings(d), now)
	o.Emergency, o.FIProgress, o.YearsToFI = fi.Emergency, fi.Progress, fi.YearsToFI
	o.Anomalies = insights.Anomalies(txs, thisM, now)
	soon := now.AddDate(0, 0, 14).Format("2006-01-02")
	recurring, _, _ := insights.RecurringCosts(d, txs, now)
	var obligationCats []string // shown under "still to pay" already
	for _, c := range fixedCats {
		obligationCats = append(obligationCats, c...)
	}
	for _, r := range recurring {
		if r.Next >= today && r.Next <= soon && !within(r.Category, obligationCats) {
			o.Upcoming = append(o.Upcoming, r)
		}
	}
	sort.Slice(o.Upcoming, func(i, j int) bool { return o.Upcoming[i].Next < o.Upcoming[j].Next })
	// When each unpaid obligation is due: the first recurring cost this month
	// that falls under the line's categories.
	monthEnd := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	for i := range o.Plan.Fixed {
		f := &o.Plan.Fixed[i]
		if f.Spent >= f.Budgeted {
			continue
		}
		for _, r := range recurring {
			if r.Next < today || r.Next > monthEnd || (f.Due != "" && r.Next >= f.Due) {
				continue
			}
			for _, c := range fixedCats[i] {
				if r.Category == c || strings.HasPrefix(r.Category, c+".") {
					f.Due = r.Next
				}
			}
		}
		if f.Due == "" {
			f.Due = dueFromHistory(txs, fixedCats[i], now)
		}
	}
	o.Plan.addEvents(txs, fixedCats, plan.LoadSettings(d).Salary(), now)
	o.Cash = buildCashPlan(d, txs, &o.Plan, fixedCats, book, now)
	budgetDiffs := 0
	if r, err := BudgetReport(d, thisM, now); err == nil {
		for _, l := range r.Lines {
			if l.Suggestion != nil && l.Kind != "saving" {
				budgetDiffs++
			}
		}
	}
	plannedLate := []insights.Recurring{}
	if all, _, err := insights.RecurringAll(d, txs, now); err == nil {
		for _, r := range all {
			if r.Kind == "transfer" && r.Overdue && !r.Done {
				plannedLate = append(plannedLate, r)
			}
		}
	}
	defer func() { o.Actions = buildActions(o, budgetDiffs, plannedLate, now) }()
	o.Checks = Checks(d, now)
	if now.Day() <= 7 {
		o.ReviewMonth = now.AddDate(0, -1, 0).Format("2006-01")
	}
	rec, _ := ledger.List(d, ledger.Filter{Limit: 8})
	o.Recent = rec.Items
	return o, nil
}

// fillDaily turns the month's free money into a daily allowance and pace.
func (p *PlanPulse) fillDaily(free map[string]money.Cents, everyday map[string]map[int]money.Cents, now time.Time) {
	month := now.Format("2006-01")
	dim := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	day := now.Day()
	p.FreeSpent = free[month]
	p.DaysLeft = dim - day + 1
	if p.SafeToSpend > 0 {
		p.PerDayLeft = p.SafeToSpend / money.Cents(p.DaysLeft)
	}
	if day > 0 {
		p.AvgDay = p.FreeSpent / money.Cents(day)
	}
	var total, all money.Cents
	var days, months int
	for i := 1; i <= 6; i++ {
		m := time.Date(now.Year(), now.Month()-time.Month(i), 1, 0, 0, 0, 0, time.UTC)
		// A typical month is everyday spending: one-offs (a single payment
		// of €250 or more) are left out so one big dinner doesn't set the pace.
		byDay, ok := everyday[m.Format("2006-01")]
		if !ok {
			if _, any := free[m.Format("2006-01")]; !any {
				continue
			}
		}
		for _, v := range byDay {
			total += v
		}
		all += free[m.Format("2006-01")]
		months++
		days += time.Date(m.Year(), m.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	}
	if days > 0 {
		p.TypicalDay = total / money.Cents(days)
		p.TypicalOneOffs = (all - total) / money.Cents(months)
	}
	// The rest of the month at an expected daily pace: early on this month's
	// few days say little, so lean on the typical day and shift to the actual
	// pace as the month fills in. Today counts as already spent.
	progress := float64(day) / float64(dim)
	pace := p.AvgDay.Float()
	if p.TypicalDay > 0 {
		pace = progress*p.AvgDay.Float() + (1-progress)*p.TypicalDay.Float()
	}
	p.ExpectedDay = money.FromFloat(pace)
	p.ProjectedLeft = p.SafeToSpend - money.FromFloat(pace*float64(p.DaysLeft-1))
}

// dueFromHistory guesses this month's due date for an obligation with no
// detected rhythm (mortgage interest: no merchant, a new amount every month)
// from the day of its last payment in the past two months; "" if unknown or
// that day has already passed.
func dueFromHistory(txs []ledger.Tx, cats []string, now time.Time) string {
	since := now.AddDate(0, -2, 0).Format("2006-01-02")
	thisMonth := now.Format("2006-01")
	last := ""
	for i := range txs {
		t := &txs[i]
		if t.Date < since || t.Date[:7] == thisMonth || t.Date <= last {
			continue
		}
		for _, c := range cats {
			if t.Category == c || strings.HasPrefix(t.Category, c+".") {
				last = t.Date
			}
		}
	}
	if last == "" {
		return ""
	}
	day, _ := time.Parse("2006-01-02", last)
	dim := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	d := min(day.Day(), dim)
	if d < now.Day() {
		return ""
	}
	return time.Date(now.Year(), now.Month(), d, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

// fillTimeline lays this month's free spending out by day, next to the
// typical month: for each day, the average of the last six complete months'
// cumulative free spending by that day (shorter months count their total).
func (p *PlanPulse) fillTimeline(byDay, everyday map[string]map[int]money.Cents, now time.Time) {
	month := now.Format("2006-01")
	dim := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	var prev []string
	for i := 1; i <= 6; i++ {
		m := time.Date(now.Year(), now.Month()-time.Month(i), 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
		if byDay[m] != nil {
			prev = append(prev, m)
		}
	}
	cumPrev := make([]money.Cents, len(prev))
	var cum money.Cents
	p.Days = make([]DayPoint, 0, dim)
	for d := 1; d <= dim; d++ {
		pt := DayPoint{Day: d}
		if d <= now.Day() {
			pt.Spent = byDay[month][d]
			cum += pt.Spent
			c := cum
			pt.Cum = &c
		}
		var sum money.Cents
		for i, m := range prev {
			cumPrev[i] += everyday[m][d]
			sum += cumPrev[i]
		}
		if len(prev) > 0 {
			pt.Typical = sum / money.Cents(len(prev))
		}
		p.Days = append(p.Days, pt)
	}
}

// addEvents dates what happens to the month's money: each fixed obligation
// (paid, or when it is due) and each day this month's income arrived.
func (p *PlanPulse) addEvents(txs []ledger.Tx, fixedCats [][]string, salary []plan.SalaryRule, now time.Time) {
	month := now.Format("2006-01")
	in := func(cat string, cats []string) bool {
		for _, c := range cats {
			if cat == c || strings.HasPrefix(cat, c+".") {
				return true
			}
		}
		return false
	}
	income := map[int]money.Cents{}
	for i := range txs {
		t := &txs[i]
		if t.Date[:7] != month {
			continue
		}
		// Income the plan counts in this month (a salary landing on the 1st
		// belongs to the month before, as everywhere else).
		if t.Kind == "income" && ledger.Top(t.Category) != "refunds" && plan.FlowDate(t, salary)[:7] == month {
			income[dayFrom(t.Date)] += t.Amount
		}
		for j := range p.Fixed {
			if j < len(fixedCats) && t.Kind != "income" && in(t.Category, fixedCats[j]) && t.Date > p.Fixed[j].Paid {
				p.Fixed[j].Paid = t.Date
			}
		}
	}
	for _, f := range p.Fixed {
		switch {
		case f.Spent >= f.Budgeted && f.Paid != "":
			p.Events = append(p.Events, MonthEvent{Day: dayFrom(f.Paid), Kind: "fixed", Label: f.Name, Amount: f.Spent, Done: true})
		case f.Due != "":
			p.Events = append(p.Events, MonthEvent{Day: dayFrom(f.Due), Kind: "fixed", Label: f.Name, Amount: f.Budgeted - f.Spent})
		}
	}
	for d, v := range income {
		p.Events = append(p.Events, MonthEvent{Day: d, Kind: "income", Label: "Income", Amount: v, Done: true})
	}
	sort.Slice(p.Events, func(i, j int) bool {
		return p.Events[i].Day < p.Events[j].Day || p.Events[i].Day == p.Events[j].Day && p.Events[i].Label < p.Events[j].Label
	})
}

func dayFrom(date string) int {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0
	}
	return t.Day()
}
