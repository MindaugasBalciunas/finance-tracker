package cfo

import (
	"database/sql"
	"fmt"
	"math"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

// The independence plan: not one finish line but levels of freedom — the
// day your investments' safe payout covers your fixed obligations, then your
// essentials, then the whole lifestyle, then a comfortable one — plus the day
// you could stop putting money in and still get there by your target age
// (coast), what the money could pay you each year, and what more saving buys.
// All in today's euros at the plan's real expected return and withdrawal rate.

type FreedomLevel struct {
	Key      string      `json:"key"` // obligations | essentials | lifestyle | comfortable
	Title    string      `json:"title"`
	Detail   string      `json:"detail"`
	Monthly  money.Cents `json:"monthly"` // what the payout must cover a month
	Capital  money.Cents `json:"capital"` // invested money that pays it forever at the withdrawal rate
	Progress float64     `json:"progress"`
	Date     string      `json:"date,omitempty"` // YYYY-MM at this pace
	Months   int         `json:"months"`
	Age      float64     `json:"age,omitempty"`
	Done     bool        `json:"done"`
}

type Coast struct {
	Age    int         `json:"age"`            // the target age it is measured against
	Needed money.Cents `json:"needed"`         // invested today that grows to FI by that age untouched
	Date   string      `json:"date,omitempty"` // when contributions could stop
	Months int         `json:"months"`
	Done   bool        `json:"done"`
	Detail string      `json:"detail"`
	// Later: the same for later finishing ages, so a target age out of reach
	// still answers "when could I stop adding money?".
	Later []CoastAt `json:"later,omitempty"`
}

type CoastAt struct {
	Age  int    `json:"age"`
	Date string `json:"date,omitempty"` // when contributions could stop ("" = not before that age)
	Done bool   `json:"done"`
}

// coastDate: the first month investable money, still growing with the monthly
// addition, is enough to reach target by month toAge untouched.
func coastDate(start, monthly, r, target float64, toAge int, now time.Time) (string, bool) {
	v := start
	for m := 0; m <= toAge && m <= 720; m++ {
		if m > 0 {
			v = v*(1+r) + monthly
		}
		if v >= target/math.Pow(1+r, float64(toAge-m)) {
			if m == 0 {
				return "", true
			}
			return ledger.AddMonths(now, m), false
		}
	}
	return "", false
}

type PayoutPoint struct {
	Year       int         `json:"year"`
	Age        int         `json:"age,omitempty"`
	Investable money.Cents `json:"investable"`
	Monthly    money.Cents `json:"monthly"` // safe payout a month if you stopped then
}

type WhatIf struct {
	Extra  money.Cents `json:"extra"` // more invested a month
	Date   string      `json:"date,omitempty"`
	Sooner float64     `json:"sooner"` // years earlier than today's pace
}

type FIPlan struct {
	insights.FI
	Levels      []FreedomLevel `json:"levels"`
	PassiveNow  money.Cents    `json:"passive_now"` // safe payout a month from today's investable money
	Obligations money.Cents    `json:"obligations"` // fixed obligations a month (plan)
	Essentials  money.Cents    `json:"essentials"`
	Spending    money.Cents    `json:"spending"` // a month, what FI is measured on
	Coast       Coast          `json:"coast"`
	Payouts     []PayoutPoint  `json:"payouts"`
	WhatIfs     []WhatIf       `json:"what_ifs"`
	SavingsRate float64        `json:"savings_rate"` // last 12 months, 0..1
	Locked      money.Cents    `json:"locked"`       // in pensions: not reachable before retirement age
	Notes       []string       `json:"notes"`
}

// monthsTo grows start by monthly at real rate r (a month) until it reaches
// target; -1 when not within 60 years.
func monthsTo(start, monthly, r, target float64) int {
	if start >= target {
		return 0
	}
	v := start
	for m := 1; m <= 720; m++ {
		v = v*(1+r) + monthly
		if v >= target {
			return m
		}
	}
	return -1
}

func BuildFIPlan(d *sql.DB, now time.Time) (*FIPlan, error) {
	settings := plan.LoadSettings(d)
	book, err := wealth.LoadBook(d)
	if err != nil {
		return nil, err
	}
	cats, err := ledger.CategoryMap(d)
	if err != nil {
		return nil, err
	}
	txs, err := ledger.All(d, ledger.Filter{From: now.AddDate(-1, -1, 0).Format("2006-01-02"), To: now.Format("2006-01-02")})
	if err != nil {
		return nil, err
	}
	fi := insights.ComputeFI(txs, cats, book, settings, now)
	p := &FIPlan{FI: fi, Levels: []FreedomLevel{}, Payouts: []PayoutPoint{}, WhatIfs: []WhatIf{}, Notes: []string{}}
	wr := settings.WithdrawalRate
	if wr <= 0 {
		wr = 4
	}
	r := settings.ExpectedReturn / 100 / 12 // real: everything in today's euros
	start, monthly := fi.Investable.Float(), math.Max(fi.MonthlyInvested.Float(), 0)
	capital := func(perMonth money.Cents) money.Cents { return money.FromFloat(perMonth.Float() * 12 * 100 / wr) }
	month := func(m int) string { return ledger.AddMonths(now, m) }
	age := func(m int) float64 {
		if settings.BirthYear <= 0 {
			return 0
		}
		return math.Round((float64(now.Year()-settings.BirthYear)+float64(m)/12)*10) / 10
	}
	p.PassiveNow = money.FromFloat(start * wr / 100 / 12)

	// What each level must cover a month.
	if rep, err := BudgetReport(d, now.Format("2006-01"), now); err == nil {
		p.Obligations = rep.FixedPlanned
	}
	p.Essentials = fi.Emergency.MonthlyEssential
	p.Spending = fi.AnnualSpend / 12
	levels := []struct {
		key, title string
		monthly    money.Cents
		why        string
	}{
		{"obligations", "Obligations covered", p.Obligations, "the mortgage, alimony and other fixed lines in your plan"},
		{"essentials", "Essentials covered", p.Essentials, "food, housing, transport, health — the essential categories"},
		{"lifestyle", "Financial independence", p.Spending, fi.SpendSource},
		{"comfortable", "Comfortable independence", money.FromFloat(p.Spending.Float() * 1.25), "your spending with a 25% cushion for a fuller life"},
	}
	for _, l := range levels {
		if l.monthly <= 0 {
			continue
		}
		fl := FreedomLevel{Key: l.key, Title: l.title, Monthly: l.monthly, Capital: capital(l.monthly)}
		fl.Progress = math.Min(1, math.Round(start/fl.Capital.Float()*1000)/1000)
		m := monthsTo(start, monthly, r, fl.Capital.Float())
		switch {
		case m == 0:
			fl.Done = true
			fl.Detail = fmt.Sprintf("Your investments can pay %s a month — %s.", eurN(l.monthly), l.why)
		case m > 0:
			fl.Months, fl.Date, fl.Age = m, month(m), age(m)
			fl.Detail = fmt.Sprintf("%s a month for %s needs %s invested.", eurN(l.monthly), l.why, eurN(fl.Capital))
		default:
			fl.Detail = fmt.Sprintf("%s invested needed — not within 60 years at today's pace.", eurN(fl.Capital))
		}
		p.Levels = append(p.Levels, fl)
	}

	// Coast: when what is invested, left alone, grows to FI by the target age.
	targetAge := settings.TargetAge
	if targetAge <= 0 {
		targetAge = 60
		p.Notes = append(p.Notes, "No target age set — coast is measured to age 60 (Plan → Income & goals).")
	}
	if settings.BirthYear <= 0 {
		p.Coast = Coast{Age: targetAge, Detail: "Set your birth year in Plan → Income & goals to see when you could stop adding money."}
	} else {
		toAge := func(a int) int { return (settings.BirthYear+a-now.Year())*12 - int(now.Month()) + 12 }
		c := Coast{Age: targetAge}
		target := fi.Target.Float()
		c.Needed = money.FromFloat(target / math.Pow(1+r, float64(toAge(targetAge))))
		c.Date, c.Done = coastDate(start, monthly, r, target, toAge(targetAge), now)
		if c.Date != "" {
			t, _ := time.Parse("2006-01", c.Date)
			c.Months = (t.Year()-now.Year())*12 + int(t.Month()-now.Month())
		}
		for _, a := range []int{55, 60, 65} {
			if a > targetAge {
				at := CoastAt{Age: a}
				at.Date, at.Done = coastDate(start, monthly, r, target, toAge(a), now)
				c.Later = append(c.Later, at)
			}
		}
		switch {
		case c.Done:
			c.Detail = fmt.Sprintf("Already there: what's invested grows to %s by age %d without another euro.", eurN(fi.Target), targetAge)
		case c.Date != "":
			c.Detail = fmt.Sprintf("Keep adding %s a month until then; after that, what's invested grows to %s by age %d on its own — saving becomes optional.", eurN(fi.MonthlyInvested), eurN(fi.Target), targetAge)
		default:
			c.Detail = fmt.Sprintf("Not before age %d at today's pace — more saving or a later target age brings it in.", targetAge)
		}
		p.Coast = c
	}

	// Payouts: what the money could pay a month if you stopped in each year.
	v := start
	for y := 0; y <= 30; y++ {
		pt := PayoutPoint{Year: now.Year() + y, Investable: money.FromFloat(v), Monthly: money.FromFloat(v * wr / 100 / 12)}
		if settings.BirthYear > 0 {
			pt.Age = now.Year() + y - settings.BirthYear
		}
		p.Payouts = append(p.Payouts, pt)
		for k := 0; k < 12; k++ {
			v = v*(1+r) + monthly
		}
	}

	// What more saving buys.
	base := monthsTo(start, monthly, r, fi.Target.Float())
	for _, extra := range []float64{100, 250, 500} {
		w := WhatIf{Extra: money.FromFloat(extra)}
		if m := monthsTo(start, monthly+extra, r, fi.Target.Float()); m > 0 {
			w.Date = month(m)
			if base > 0 {
				w.Sooner = math.Round(float64(base-m)/12*10) / 10
			}
		}
		p.WhatIfs = append(p.WhatIfs, w)
	}

	// Savings rate, last 12 months; money locked in pensions.
	var inc, spend money.Cents
	yearAgo := now.AddDate(-1, 0, 0).Format("2006-01-02")
	var recent []ledger.Tx
	for _, t := range txs {
		if t.Date > yearAgo {
			recent = append(recent, t)
		}
	}
	for _, f := range insights.CashFlow(recent, cats, "year", settings.Salary()) {
		inc += f.Income
		spend += f.Spending
	}
	if inc > 0 {
		p.SavingsRate = math.Round((inc-spend).Float()/inc.Float()*1000) / 1000
	}
	p.Locked = book.SnapshotAt(now.Format("2006-01-02"), false).ByGroup["pension"]
	if p.Locked > 0 && fi.Investable > 0 {
		p.Notes = append(p.Notes, fmt.Sprintf("%s of the investable money is in pensions, reachable only from retirement age — the rest has to bridge the years before.", eurN(p.Locked)))
	}
	return p, nil
}
