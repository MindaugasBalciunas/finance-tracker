package cfo

import (
	"testing"
	"time"

	"ft/internal/money"
)

func TestFillDaily(t *testing.T) {
	e := func(v float64) money.Cents { return money.FromFloat(v) }
	// 10 October (31 days): €300 free money spent so far, €1,100 still free.
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	free := map[string]money.Cents{
		"2026-10": e(300),
		"2026-09": e(600), "2026-08": e(620), "2026-07": e(620), // 30 + 31 + 31 days
		"2026-06": e(600), "2026-05": e(620), "2026-04": e(600), // 30 + 31 + 30 days
		"2026-03": e(9999), // seventh month back: ignored
	}
	// Everyday spending (one-offs out): each month's total on its 1st, plus
	// a €900 dinner in September that the typical month must ignore.
	everyday := map[string]map[int]money.Cents{}
	for m, v := range free {
		if m != "2026-10" {
			everyday[m] = map[int]money.Cents{1: v}
		}
	}
	free["2026-09"] += e(900)
	p := PlanPulse{SafeToSpend: e(1100)}
	p.fillDaily(free, everyday, now)
	if p.DaysLeft != 22 || p.PerDayLeft != e(50) {
		t.Fatalf("days left %d, per day %v", p.DaysLeft, p.PerDayLeft)
	}
	if p.FreeSpent != e(300) || p.AvgDay != e(30) {
		t.Errorf("spent %v avg %v", p.FreeSpent, p.AvgDay)
	}
	if p.TypicalDay != e(20) { // €3,660 over 183 days — the €900 one-off left out
		t.Errorf("typical %v", p.TypicalDay)
	}
	// Expected pace: 10/31 of this month's €30 + 21/31 of the typical €20.
	pace := 10.0/31*30 + 21.0/31*20
	if p.ExpectedDay != e(pace) || p.ProjectedLeft != e(1100)-money.FromFloat(pace*21) {
		t.Errorf("expected %v projected %v", p.ExpectedDay, p.ProjectedLeft)
	}
	// Early in a month a quiet start doesn't promise a surplus: typical dominates.
	early := PlanPulse{SafeToSpend: e(1262)}
	early.fillDaily(map[string]money.Cents{"2026-10": e(20), "2026-09": e(2490)}, map[string]map[int]money.Cents{"2026-09": {1: e(2490)}}, time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC))
	if early.ProjectedLeft >= 0 {
		t.Errorf("day 1 at €20 vs typical €83/day should warn: %+v", early)
	}
	// Overspent: no allowance, projection goes negative.
	q := PlanPulse{SafeToSpend: e(-50)}
	q.fillDaily(free, everyday, now)
	if q.PerDayLeft != 0 || q.ProjectedLeft >= 0 {
		t.Errorf("overspent: %+v", q)
	}
}
