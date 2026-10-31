package cfo

import (
	"database/sql"
	"fmt"
	"math"
	"time"

	"ft/internal/goals"
	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

// Milestones are the dates the money is heading for at today's pace: the
// emergency fund full, the next round figures of liquid wealth, the mortgage
// paid off, financial independence. Each is a projection from the app's own
// figures (what is saved and invested a month, the plan's expected return,
// the loan schedule), never a promise — the page says so.

type Milestone struct {
	Key     string      `json:"key"` // emergency | liquid:<target> | debt:<id> | fi
	Title   string      `json:"title"`
	Detail  string      `json:"detail"`
	Target  money.Cents `json:"target,omitempty"`
	Current money.Cents `json:"current,omitempty"`
	Date    string      `json:"date,omitempty"` // YYYY-MM it is reached ("" = not at this pace)
	Months  int         `json:"months"`         // from now; 0 = reached
	Done    bool        `json:"done"`
}

// liquidSteps are the round figures worth marking.
var liquidSteps = []float64{10000, 25000, 50000, 75000, 100000, 150000, 200000, 250000, 400000, 500000, 750000, 1000000}

func Milestones(d *sql.DB, now time.Time) ([]Milestone, error) {
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
	snap := book.SnapshotAt(now.Format("2006-01-02"), false)
	month := func(m int) string { return ledger.AddMonths(now, m) }
	out := []Milestone{}

	// The emergency fund fills from what is left at month end.
	em := fi.Emergency
	if em.Target > 0 {
		m := Milestone{Key: "emergency", Title: fmt.Sprintf("Emergency fund: %.0f months", em.TargetMonths), Target: em.Target, Current: em.Cash}
		if em.Cash >= em.Target {
			m.Done, m.Detail = true, fmt.Sprintf("%.1f months of essentials in cash.", em.Months)
		} else if gp, err := goals.Build(d, "", now); err == nil && gp.AvgLeftOver > 0 {
			m.Months = int(math.Ceil((em.Target - em.Cash).Float() / gp.AvgLeftOver.Float()))
			m.Date = month(m.Months)
			m.Detail = fmt.Sprintf("%s short; at the %s left over in a usual month.", eurN(em.Target-em.Cash), eurN(gp.AvgLeftOver))
		} else {
			m.Detail = fmt.Sprintf("%s short, and nothing is left over in a usual month to fill it.", eurN(em.Target-em.Cash))
		}
		out = append(out, m)
	}

	// Liquid wealth: the next two round figures, growing at what is put in a
	// month plus the expected return on it.
	r := (settings.ExpectedReturn + inflation) / 100 / 12
	monthly := math.Max(fi.MonthlyInvested.Float(), 0)
	cur := snap.Liquid.Float()
	shown := 0
	for _, step := range liquidSteps {
		if step <= cur || shown == 2 {
			continue
		}
		shown++
		m := Milestone{Key: fmt.Sprintf("liquid:%.0f", step), Title: fmt.Sprintf("%s liquid", eurN(money.FromFloat(step))), Target: money.FromFloat(step), Current: snap.Liquid}
		v := cur
		for n := 1; n <= 600; n++ {
			v = v*(1+r) + monthly
			if v >= step {
				m.Months, m.Date = n, month(n)
				break
			}
		}
		m.Detail = fmt.Sprintf("From %s, adding %s a month at about %.1f%% a year.", eurN(snap.Liquid), eurN(money.FromFloat(monthly)), (settings.ExpectedReturn + inflation))
		if m.Date == "" {
			m.Detail = "Not within 50 years at today's pace."
		}
		out = append(out, m)
	}

	// Debts: the schedule's own payoff date.
	for _, l := range wealth.Loans(book, txs, now) {
		if l.Balance <= 0 || l.PayoffDate == "" {
			continue
		}
		m := Milestone{Key: "debt:" + l.AccountID, Title: l.Name + " paid off", Target: 0, Current: l.Balance, Date: l.PayoffDate[:7]}
		if t, err := time.Parse("2006-01", m.Date); err == nil {
			m.Months = (t.Year()-now.Year())*12 + int(t.Month()-now.Month())
		}
		m.Detail = fmt.Sprintf("%s left at %.2f%% on the payment schedule — extra principal brings it forward.", eurN(l.Balance), l.Rate)
		out = append(out, m)
	}

	// Financial independence.
	fm := Milestone{Key: "fi", Title: "Financial independence", Target: fi.Target, Current: fi.Investable}
	switch {
	case fi.YearsToFI == 0:
		fm.Done, fm.Detail = true, "Investable assets cover your spending at your withdrawal rate."
	case fi.YearsToFI > 0:
		fm.Date, fm.Months = fi.FIDate, int(math.Round(fi.YearsToFI*12))
		fm.Detail = fmt.Sprintf("%s of %s needed (%.0f%%).", eurN(fi.Investable), eurN(fi.Target), fi.Progress*100)
		if fi.FIAge > 0 {
			fm.Detail += fmt.Sprintf(" At age %.0f.", fi.FIAge)
		}
	default:
		fm.Detail = "Not within 60 years at today's pace — see Wealth → Independence."
	}
	out = append(out, fm)
	return out, nil
}
