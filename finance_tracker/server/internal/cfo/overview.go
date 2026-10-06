// Package cfo assembles the cross-domain views — the Home overview, the
// plan report and the monthly briefing — from ledger, wealth, plan and
// insights. The API and the AI assistant both read these, so a number on
// the Home screen and in a chat answer is the same number.
package cfo

import (
	"database/sql"
	"sort"
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
	FreeSpent     money.Cents `json:"free_spent"`     // free spending so far this month
	DaysLeft      int         `json:"days_left"`      // including today
	PerDayLeft    money.Cents `json:"per_day_left"`   // safe to spend ÷ days left
	AvgDay        money.Cents `json:"avg_day"`        // this month's free spending per elapsed day
	TypicalDay    money.Cents `json:"typical_day"`    // same, averaged over the previous 6 complete months
	ExpectedDay   money.Cents `json:"expected_day"`   // pace used for the projection (this month blended with typical)
	ProjectedLeft money.Cents `json:"projected_left"` // what is left at month end at the expected pace
	Over          []LineBrief `json:"over"`           // spending lines over budget
	Lines         []LineBrief `json:"lines"`          // busiest spending lines (most of budget used first)
	Spent         money.Cents `json:"spent"`
	Budgeted      money.Cents `json:"budgeted"`
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

	if r, err := BudgetReport(d, thisM, now); err == nil {
		o.Plan = PlanPulse{SafeToSpend: r.SafeToSpend, IncomeBase: r.IncomeBase}
		o.Plan.fillDaily(r.FreeSpentByMonth, now)
		for _, l := range r.Lines {
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
	for _, r := range recurring {
		if r.Next >= today && r.Next <= soon {
			o.Upcoming = append(o.Upcoming, r)
		}
	}
	sort.Slice(o.Upcoming, func(i, j int) bool { return o.Upcoming[i].Next < o.Upcoming[j].Next })
	o.Checks = Checks(d, now)
	if now.Day() <= 7 {
		o.ReviewMonth = now.AddDate(0, -1, 0).Format("2006-01")
	}
	rec, _ := ledger.List(d, ledger.Filter{Limit: 8})
	o.Recent = rec.Items
	return o, nil
}

// fillDaily turns the month's free money into a daily allowance and pace.
func (p *PlanPulse) fillDaily(free map[string]money.Cents, now time.Time) {
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
	var total money.Cents
	var days int
	for i := 1; i <= 6; i++ {
		m := time.Date(now.Year(), now.Month()-time.Month(i), 1, 0, 0, 0, 0, time.UTC)
		v, ok := free[m.Format("2006-01")]
		if !ok {
			continue
		}
		total += v
		days += time.Date(m.Year(), m.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	}
	if days > 0 {
		p.TypicalDay = total / money.Cents(days)
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
