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

// What a purchase would do, in the owner's own numbers, before it is made:
// how much of this month's free money it takes, the buffer after it, how far
// it pushes the wish list back, and how much later financial independence
// comes. Each line is a plain sentence; the page shows them as they are.

type Impact struct {
	Amount  money.Cents  `json:"amount"`
	Cooling int          `json:"cooling_days"`
	Lines   []ImpactLine `json:"lines"`
}

type ImpactLine struct {
	Key  string `json:"key"`  // month | buffer | wishlist | fi | days
	Tone string `json:"tone"` // good | warn | bad
	Text string `json:"text"`
}

func DecisionImpact(d *sql.DB, amount money.Cents, now time.Time) (*Impact, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("the price must be more than zero")
	}
	out := &Impact{Amount: amount, Lines: []ImpactLine{}}
	add := func(key, tone, text string) { out.Lines = append(out.Lines, ImpactLine{key, tone, text}) }

	// This month's free money.
	if r, err := BudgetReport(d, now.Format("2006-01"), now); err == nil {
		switch left := r.SafeToSpend; {
		case left <= 0:
			add("month", "bad", fmt.Sprintf("Nothing is left to spend this month (%s), so it would come from savings.", eur(left)))
		case amount > left:
			add("month", "bad", fmt.Sprintf("It's %s more than the %s left to spend this month.", eur(amount-left), eur(left)))
		default:
			tone := "good"
			if amount*2 > left {
				tone = "warn"
			}
			add("month", tone, fmt.Sprintf("Takes %.0f%% of the %s left to spend this month.", amount.Float()/left.Float()*100, eur(left)))
		}
	}

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

	// Days of an ordinary life.
	if fi.AnnualSpend > 0 {
		days := amount.Float() / (fi.AnnualSpend.Float() / 365)
		add("days", "good", fmt.Sprintf("As much as %.0f days of your usual spending.", math.Max(1, math.Round(days))))
	}

	// The buffer, if it is paid from cash.
	if em := fi.Emergency; em.MonthlyEssential > 0 && em.TargetMonths > 0 {
		after := (em.Cash - amount).Float() / em.MonthlyEssential.Float()
		tone := "good"
		switch {
		case after < em.TargetMonths*0.75:
			tone = "bad"
		case after < em.TargetMonths:
			tone = "warn"
		}
		add("buffer", tone, fmt.Sprintf("Paid from cash, the buffer goes from %.1f to %.1f months of essentials (you aim for %.0f).", em.Months, math.Max(0, after), em.TargetMonths))
	}

	// The wish list: it is funded from what's left at month end.
	if gp, err := goals.Build(d, "", now); err == nil && gp.AvgLeftOver > 0 {
		months := amount.Float() / gp.AvgLeftOver.Float()
		next := ""
		for _, g := range gp.Goals {
			if g.Status == "active" && g.Remaining > 0 {
				next = g.Name
				break
			}
		}
		text := fmt.Sprintf("Pushes the wish list back about %s (you have %s left over in a typical month).", span(months), eur(gp.AvgLeftOver))
		if next != "" {
			text = fmt.Sprintf("Pushes %s — and everything after it on the wish list — back about %s.", next, span(months))
		}
		tone := "good"
		if months >= 1 {
			tone = "warn"
		}
		add("wishlist", tone, text)
	}

	// Financial independence: what the money would have grown into, in
	// months of saving at today's pace.
	if fi.MonthlyInvested > 0 && fi.YearsToFI > 0 {
		grown := amount.Float() * math.Pow(1+fi.ExpectedReturn/100, fi.YearsToFI)
		months := grown / fi.MonthlyInvested.Float()
		add("fi", "good", fmt.Sprintf("Invested instead, it would be about %s by your FI date — independence about %s later.", eur(money.FromFloat(grown)), span(months)))
	}
	return out, nil
}

func span(months float64) string {
	switch weeks := months * 4.35; {
	case weeks < 1.5:
		return "a week"
	case months < 1.5:
		return fmt.Sprintf("%.0f weeks", weeks)
	case months < 18:
		return fmt.Sprintf("%.0f months", months)
	}
	return fmt.Sprintf("%.1f years", months/12)
}
