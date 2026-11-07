package cfo

import (
	"database/sql"
	"math"
	"time"

	"ft/internal/goals"
	"ft/internal/insights"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

// EmergencyPlan is the emergency fund as the plan should treat it: not all
// the cash there is today, but what would still be there if income stopped
// — after what already has to leave before the next payday, and not the
// money set aside for the wish list. On 7 Nov the cash was €12,459 (4.3
// months), but €3,450 of it was due out by 10 Dec and €6,110 of it was
// notes in the Cash account, while the payday checklist offered the wish
// list €3,376 more with no look at the floor.
type EmergencyPlan struct {
	Bank         money.Cents `json:"bank"`        // current and savings accounts
	Cash         money.Cents `json:"cash"`        // Cash accounts: notes and coins
	CountsCash   bool        `json:"counts_cash"` // the Cash accounts count (a plan setting)
	Committed    money.Cents `json:"committed"`   // leaving before the next payday
	SetAside     money.Cents `json:"set_aside"`   // held for wish-list goals
	Available    money.Cents `json:"available"`   // what an emergency could use
	Monthly      money.Cents `json:"monthly_essential"`
	TargetMonths float64     `json:"target_months"`
	Target       money.Cents `json:"target"`
	Months       float64     `json:"months"` // Available in months of essentials
	Gap          money.Cents `json:"gap"`    // short of the target
	Spare        money.Cents `json:"spare"`  // above the target: free for the wish list
	Payday       string      `json:"payday,omitempty"`
}

// buildEmergencyPlan works it out from the cash plan until payday (cp, nil
// when there is none) and the essentials the FI figures use.
func buildEmergencyPlan(d *sql.DB, book *wealth.Book, em insights.Emergency, cp *CashPlan, now time.Time) *EmergencyPlan {
	settings := plan.LoadSettings(d)
	today := now.Format("2006-01-02")
	e := &EmergencyPlan{CountsCash: !settings.EmergencyExcludeCash, Monthly: em.MonthlyEssential, TargetMonths: em.TargetMonths, Target: em.Target}
	for id, a := range book.Accounts {
		if a.Group != "cash" {
			continue
		}
		p, ok := book.At(id, today)
		if !ok {
			continue
		}
		if a.Kind == "cash" {
			e.Cash += p.Value
		} else {
			e.Bank += p.Value
		}
	}
	// What the cash plan already sees leaving before pay comes back in:
	// obligations, bills, standing orders and day-to-day spending. Small bills
	// on the everyday account are inside the day-to-day allowance (Within) and
	// income is left out — the fund is what is there if income stops. A move
	// between two cash accounts is on both and cancels.
	if cp != nil {
		e.Payday = cp.Payday
		var net money.Cents
		for _, a := range cp.Accounts {
			for _, it := range a.Items {
				if it.Within || it.Kind == "income" {
					continue
				}
				net += it.Amount
			}
		}
		e.Committed = max(0, -net)
	}
	// Wish-list money set aside: notes held for a goal leave the fund only
	// when the Cash account counts toward it in the first place.
	if pots, err := goals.Pots(d, now); err == nil {
		for _, p := range pots {
			if book.Accounts[p.AccountID].Kind == "cash" && !e.CountsCash {
				continue
			}
			e.SetAside += min(p.SetAside, max(0, p.Balance))
		}
	}
	e.Available = e.Bank - e.Committed - e.SetAside
	if e.CountsCash {
		e.Available += e.Cash
	}
	if e.Monthly > 0 {
		e.Months = math.Round(max(0, e.Available.Float())/e.Monthly.Float()*10) / 10
	}
	e.Gap, e.Spare = max(0, e.Target-e.Available), max(0, e.Available-e.Target)
	return e
}
