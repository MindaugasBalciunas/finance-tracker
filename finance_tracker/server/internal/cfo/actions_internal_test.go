package cfo

import (
	"math"
	"strings"
	"testing"
	"time"

	"ft/internal/money"
)

// The emergency fund under its target is a "Needs you" item (amber well
// below, a note just under), rounded down so 2.95 never reads "3 of 3"; the
// budgets-vs-history note shows only in the month's first week.
func TestNeedsYouHasTheEmergencyFundAndBudgetsOnlyEarlyInTheMonth(t *testing.T) {
	// As buildEmergencyPlan leaves it: Months rounded to a tenth, Available
	// and Monthly exact.
	e := func(months float64, gap float64) *Overview {
		monthly := money.FromFloat(3000)
		avail := money.FromFloat(months * 3000)
		return &Overview{EmergencyPlan: &EmergencyPlan{Months: math.Round(months*10) / 10, Available: avail, Monthly: monthly, TargetMonths: 3, Gap: money.FromFloat(gap), Target: money.FromFloat(9038.25)}}
	}
	late := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	find := func(as []Action, kind string) *Action {
		for i := range as {
			if as[i].Kind == kind {
				return &as[i]
			}
		}
		return nil
	}
	a := find(buildActions(e(2.96, 150), 10, nil, late), "emergency")
	if a == nil || a.Level != "info" || !strings.Contains(a.Text, "2.9 of 3 months") || !strings.Contains(a.Detail, "€150 short of €9,038.25") || a.Link != "/wealth/fi" {
		t.Fatalf("near target: %+v", a)
	}
	if a := find(buildActions(e(2, 3000), 0, nil, late), "emergency"); a == nil || a.Level != "warn" {
		t.Fatalf("well below: %+v", a)
	}
	if a := find(buildActions(e(3.2, 0), 0, nil, late), "emergency"); a != nil {
		t.Fatalf("at target: %+v", a)
	}
	if a := find(buildActions(e(3.2, 0), 10, nil, late), "budgets"); a != nil {
		t.Fatalf("budgets note on the 24th: %+v", a)
	}
	if a := find(buildActions(e(3.2, 0), 10, nil, time.Date(2026, 12, 3, 9, 0, 0, 0, time.UTC)), "budgets"); a == nil {
		t.Fatal("no budgets note on the 3rd")
	}
	// A top-up due today is still only a note: the app shows money, the
	// banks move it (owner, 2026-11-24).
	o := e(3.2, 0)
	o.Cash = &CashPlan{Moves: []CashMove{{From: "swed_sav", FromName: "Swedbank savings", To: "swed", ToName: "Swedbank", Amount: money.FromFloat(320), By: "2026-11-24", For: "Day-to-day spending"}}}
	if a := find(buildActions(o, 0, nil, late), "move"); a == nil || a.Level != "info" {
		t.Fatalf("top-up due today: %+v", a)
	}
}
