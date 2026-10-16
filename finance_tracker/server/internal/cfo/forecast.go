package cfo

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"ft/internal/goals"
	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/merchant"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

// Forecast is cash (checking, savings, cash) month by month for the year
// ahead: the plan's income, obligations and saving, everyday spending at
// its usual level, the one-offs that keep coming, and what is already
// dated — yearly bills on their next date and wish-list goals in their
// target month. A Scenario reruns it with changes, so "what if" is the same
// arithmetic as "what is".
type Forecast struct {
	Start       string          `json:"start"`                 // first month projected (YYYY-MM)
	Cash        money.Cents     `json:"cash"`                  // cash today
	Floor       money.Cents     `json:"floor"`                 // emergency target: months of essentials
	Months      []ForecastMonth `json:"months"`                // the year ahead
	Assumed     Assumptions     `json:"assumed"`               // the inputs, so the screen can show and change them
	Lowest      money.Cents     `json:"lowest"`                // lowest month-end cash
	LowestOn    string          `json:"lowest_on"`             // its month
	BelowFloor  string          `json:"below_floor,omitempty"` // first month under the emergency target
	Negative    string          `json:"negative,omitempty"`    // first month cash would run out
	Invested    money.Cents     `json:"invested"`              // saving lines over the horizon (money moved to investments)
	Unscheduled []ForecastItem  `json:"unscheduled,omitempty"` // goals with no date — not in the projection
}

type Assumptions struct {
	Income   money.Cents `json:"income"`          // per month, net
	Gross    float64     `json:"gross,omitempty"` // when income comes from a gross salary
	Fixed    money.Cents `json:"fixed"`           // obligations per month
	Saving   money.Cents `json:"saving"`          // saving lines per month (leave cash)
	Everyday money.Cents `json:"everyday"`        // usual everyday spending per month (payments under €250)
	OneOffs  money.Cents `json:"one_offs"`        // allowance for unplanned one-offs per month (beyond the dated ones)
}

type ForecastMonth struct {
	Month    string         `json:"month"`
	Income   money.Cents    `json:"income"`
	Fixed    money.Cents    `json:"fixed"`
	Saving   money.Cents    `json:"saving"`
	Everyday money.Cents    `json:"everyday"`
	OneOffs  money.Cents    `json:"one_offs"`
	Dated    []ForecastItem `json:"dated,omitempty"` // yearly bills, goals, scenario purchases
	Net      money.Cents    `json:"net"`
	End      money.Cents    `json:"end"` // cash at month end
}

type ForecastItem struct {
	ID     int64       `json:"id,omitempty"` // the goal, for leaving it out of a scenario
	Label  string      `json:"label"`
	Kind   string      `json:"kind"` // bill | yearly | goal | purchase
	Month  string      `json:"month,omitempty"`
	Amount money.Cents `json:"amount"`
}

// Scenario changes the forecast's inputs. Zero values keep the baseline.
type Scenario struct {
	Gross       float64        `json:"gross,omitempty"`        // a new gross salary (net worked out with LT taxes)
	Income      *float64       `json:"income,omitempty"`       // or a net income per month
	Saving      *float64       `json:"saving,omitempty"`       // saving lines per month
	EverydayPct float64        `json:"everyday_pct,omitempty"` // −20 = everyday spending 20% lower
	From        string         `json:"from,omitempty"`         // first month the change applies (default: first projected)
	Purchases   []ForecastItem `json:"purchases,omitempty"`    // extra one-off buys, by month
	SkipGoals   []int64        `json:"skip_goals,omitempty"`   // leave these goals out
}

// ForecastFrom projects horizon months from the month after now (the
// current month's money is already in today's cash, apart from what is
// still to come this month, which the first month covers).
func ForecastFrom(d *sql.DB, now time.Time, horizon int, sc *Scenario) (*Forecast, error) {
	if horizon <= 0 || horizon > 36 {
		horizon = 12
	}
	book, err := wealth.LoadBook(d)
	if err != nil {
		return nil, err
	}
	txs, err := ledger.All(d, ledger.Filter{})
	if err != nil {
		return nil, err
	}
	cats, _ := ledger.CategoryMap(d)
	settings := plan.LoadSettings(d)
	cur, err := BudgetReport(d, now.Format("2006-01"), now)
	if err != nil {
		return nil, err
	}
	budgets, _ := plan.List(d, false)
	isFixed := func(t *ledger.Tx) bool {
		for _, b := range budgets {
			if b.Kind == "fixed" && b.Matches(t) {
				return true
			}
		}
		return false
	}

	f := &Forecast{Start: now.Format("2006-01"), Months: []ForecastMonth{}}
	f.Cash = book.SnapshotAt(now.Format("2006-01-02"), false).ByGroup["cash"]
	f.Floor = insights.ComputeFI(txs, cats, book, settings, now).Emergency.Target

	// Usual everyday spending and the one-off allowance, from the last
	// complete months: everyday is the median month of payments under
	// €250 outside fixed costs; one-offs the average of the rest.
	from6 := time.Date(now.Year(), now.Month()-6, 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
	from12 := time.Date(now.Year(), now.Month()-12, 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
	thisM := now.Format("2006-01")
	everyday := map[string]money.Cents{}
	var oneOffs money.Cents
	for i := range txs {
		t := &txs[i]
		m := t.Date[:7]
		if t.Kind != "expense" || m < from12 || m >= thisM || isFixed(t) {
			continue
		}
		if t.Amount >= plan.OneOff {
			oneOffs += t.Amount
		} else if m >= from6 {
			everyday[m] += t.Amount
		}
	}
	var ev []money.Cents
	for _, v := range everyday {
		ev = append(ev, v)
	}

	// Dated items: yearly and quarterly bills on their next dates (monthly
	// ones are in everyday spending already), and goals in their month.
	horizonEnd := time.Date(now.Year(), now.Month()+time.Month(horizon)+1, 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	dated := map[string][]ForecastItem{}
	var datedOneOffs money.Cents // dated bills big enough to be one-offs: don't count them twice
	if rec, _, err := insights.RecurringCosts(d, txs, now); err == nil {
		for _, r := range rec {
			if (r.Kind != "" && r.Kind != "bill") || (r.Cadence != "yearly" && r.Cadence != "quarterly") || r.Next == "" {
				continue
			}
			if isFixed(&ledger.Tx{Kind: "expense", Category: r.Category, Merchant: r.Merchant}) {
				continue
			}
			step := 12
			if r.Cadence == "quarterly" {
				step = 3
			}
			next, err := time.Parse("2006-01-02", r.Next)
			if err != nil {
				continue
			}
			for ; next.Format("2006-01-02") <= horizonEnd; next = next.AddDate(0, step, 0) {
				m := next.Format("2006-01")
				dated[m] = append(dated[m], ForecastItem{Label: r.Merchant, Kind: "bill", Amount: r.Amount})
			}
			if r.Amount >= plan.OneOff {
				datedOneOffs += r.Amount * money.Cents(12/step)
			}
		}
	}
	// Once-a-year costs (the car service every October) in their month,
	// unless a recurring bill already put them there.
	billed := map[string]bool{}
	for _, items := range dated {
		for _, it := range items {
			billed[merchant.Fold(it.Label)] = true
		}
	}
	for _, y := range insights.DetectYearly(txs, now, isFixed) {
		if billed[merchant.Fold(y.Merchant)] {
			continue
		}
		next, err := time.Parse("2006-01", y.Next)
		if err != nil {
			continue
		}
		for ; next.Format("2006-01-02") <= horizonEnd; next = next.AddDate(1, 0, 0) {
			m := next.Format("2006-01")
			dated[m] = append(dated[m], ForecastItem{Label: y.Merchant, Kind: "yearly", Amount: y.Typical})
		}
		datedOneOffs += y.Typical
	}
	skip := map[int64]bool{}
	if sc != nil {
		for _, id := range sc.SkipGoals {
			skip[id] = true
		}
	}
	if gs, err := goals.List(d); err == nil {
		for _, g := range gs {
			left := g.Target - g.Spent
			if g.Status != "active" || left <= 0 || skip[g.ID] {
				continue
			}
			if g.TargetDate == "" || g.TargetDate > horizonEnd {
				f.Unscheduled = append(f.Unscheduled, ForecastItem{ID: g.ID, Label: g.Name, Kind: "goal", Month: monthOf(g.TargetDate), Amount: left})
				continue
			}
			m := g.TargetDate[:7]
			if m < thisM {
				m = thisM
			}
			dated[m] = append(dated[m], ForecastItem{ID: g.ID, Label: g.Name, Kind: "goal", Amount: left})
		}
	}

	a := Assumptions{Income: cur.IncomeBase, Fixed: cur.FixedPlanned, Saving: cur.SavingPlanned, Everyday: medianCents(ev),
		OneOffs: max(0, (oneOffs-datedOneOffs)/12)}
	if settings.IncomeMode == "gross" && settings.GrossSalary > 0 {
		a.Gross = settings.GrossSalary
	}
	f.Assumed = a

	// The scenario, from its month on.
	changed := a
	scFrom := ""
	if sc != nil {
		scFrom = sc.From
		if sc.Gross > 0 {
			changed.Gross = sc.Gross
			changed.Income = money.FromFloat(plan.LTNetSalary(sc.Gross, settings.MonthlyDeductions))
		}
		if sc.Income != nil {
			changed.Income = money.FromFloat(*sc.Income)
		}
		if sc.Saving != nil {
			changed.Saving = money.FromFloat(*sc.Saving)
		}
		if sc.EverydayPct != 0 {
			changed.Everyday = money.FromFloat(a.Everyday.Float() * (1 + sc.EverydayPct/100))
		}
		for _, p := range sc.Purchases {
			if p.Amount > 0 && p.Month != "" {
				dated[p.Month] = append(dated[p.Month], ForecastItem{Label: p.Label, Kind: "purchase", Amount: p.Amount})
			}
		}
	}

	// This month: what is still to come of it — the obligations and saving
	// not done yet, and the rest of the usual month's spending.
	cash := f.Cash
	dim := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	restShare := float64(dim-now.Day()) / float64(dim)
	f.Lowest, f.LowestOn = cash, thisM
	for i := 0; i < horizon; i++ {
		m := time.Date(now.Year(), now.Month()+time.Month(i), 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
		in := a
		if sc != nil && (scFrom == "" || m >= scFrom) {
			in = changed
		}
		fm := ForecastMonth{Month: m, Income: in.Income, Fixed: in.Fixed, Saving: in.Saving, Everyday: in.Everyday, OneOffs: in.OneOffs, Dated: dated[m]}
		if i == 0 {
			// Income already received this month is in today's cash.
			fm.Income = max(0, in.Income-cur.IncomeActual)
			fm.Fixed = max(0, in.Fixed-cur.FixedSpent)
			fm.Saving = max(0, in.Saving-cur.SavedActual)
			fm.Everyday = money.FromFloat(in.Everyday.Float() * restShare)
			fm.OneOffs = money.FromFloat(in.OneOffs.Float() * restShare)
		}
		fm.Net = fm.Income - fm.Fixed - fm.Saving - fm.Everyday - fm.OneOffs
		for _, it := range fm.Dated {
			fm.Net -= it.Amount
		}
		cash += fm.Net
		fm.End = cash
		f.Invested += fm.Saving
		if cash < f.Lowest {
			f.Lowest, f.LowestOn = cash, m
		}
		if f.BelowFloor == "" && f.Floor > 0 && cash < f.Floor {
			f.BelowFloor = m
		}
		if f.Negative == "" && cash < 0 {
			f.Negative = m
		}
		f.Months = append(f.Months, fm)
	}
	sort.SliceStable(f.Unscheduled, func(i, j int) bool { return f.Unscheduled[i].Amount > f.Unscheduled[j].Amount })
	return f, nil
}

// Compare runs the baseline and the scenario together.
type Comparison struct {
	Baseline *Forecast `json:"baseline"`
	Scenario *Forecast `json:"scenario,omitempty"`
	Summary  []string  `json:"summary,omitempty"` // what the scenario changes, in plain words
}

func CompareForecast(d *sql.DB, now time.Time, horizon int, sc *Scenario) (*Comparison, error) {
	base, err := ForecastFrom(d, now, horizon, nil)
	if err != nil {
		return nil, err
	}
	c := &Comparison{Baseline: base}
	if sc == nil {
		return c, nil
	}
	alt, err := ForecastFrom(d, now, horizon, sc)
	if err != nil {
		return nil, err
	}
	c.Scenario = alt
	last := func(f *Forecast) money.Cents { return f.Months[len(f.Months)-1].End }
	n := len(base.Months)
	c.Summary = append(c.Summary, fmt.Sprintf("Cash in %d months: %s instead of %s (%s).", n, eurN(last(alt)), eurN(last(base)), signedN(last(alt)-last(base))))
	if alt.Invested != base.Invested {
		c.Summary = append(c.Summary, fmt.Sprintf("Invested over the period: %s instead of %s.", eurN(alt.Invested), eurN(base.Invested)))
	}
	switch {
	case alt.Negative != "" && base.Negative == "":
		c.Summary = append(c.Summary, fmt.Sprintf("Cash would run out in %s.", monthName(alt.Negative)))
	case alt.Negative == "" && base.Negative != "":
		c.Summary = append(c.Summary, fmt.Sprintf("Cash no longer runs out (it would in %s today).", monthName(base.Negative)))
	}
	if alt.BelowFloor != base.BelowFloor {
		switch {
		case alt.BelowFloor == "":
			c.Summary = append(c.Summary, "Cash stays above the emergency target all year.")
		case base.BelowFloor == "":
			c.Summary = append(c.Summary, fmt.Sprintf("Cash drops below the emergency target in %s.", monthName(alt.BelowFloor)))
		default:
			c.Summary = append(c.Summary, fmt.Sprintf("Below the emergency target from %s instead of %s.", monthName(alt.BelowFloor), monthName(base.BelowFloor)))
		}
	}
	return c, nil
}

func signedN(c money.Cents) string {
	if c >= 0 {
		return "+" + eurS(c)
	}
	return "−" + eurS(c)
}

func monthOf(date string) string {
	if len(date) >= 7 {
		return date[:7]
	}
	return ""
}
