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
	// fillDaily no longer forecasts; fillTimeline does (TestForecast).
	if p.TypicalOneOffs != e(150) { // the €900 dinner over six months
		t.Errorf("one-offs %v", p.TypicalOneOffs)
	}
	// Overspent: no allowance, projection goes negative.
	q := PlanPulse{SafeToSpend: e(-50)}
	q.fillDaily(free, everyday, now)
	if q.PerDayLeft != 0 {
		t.Errorf("overspent: %+v", q)
	}
}

// The rest of the month follows the typical month's own rhythm — a
// spending-heavy second half stays heavy — plus a pro-rata share of the
// one-offs most months have, and the run-out day is where that crosses
// what is left.
func TestForecast(t *testing.T) {
	e := func(v float64) money.Cents { return money.FromFloat(v) }
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC) // 31 days
	// Typical month: €10 a day until the 20th, then €40 a day.
	everyday := map[string]map[int]money.Cents{"2026-09": {}, "2026-08": {}}
	for _, m := range []string{"2026-09", "2026-08"} {
		for d := 1; d <= 30; d++ {
			everyday[m][d] = e(10)
			if d > 20 {
				everyday[m][d] = e(40)
			}
		}
	}
	byDay := map[string]map[int]money.Cents{"2026-09": everyday["2026-09"], "2026-08": everyday["2026-08"], "2026-10": {5: e(100)}}
	p := PlanPulse{SafeToSpend: e(400), TypicalOneOffs: e(310)}
	p.fillTimeline(byDay, everyday, now)
	// Days 11–20 at €10, 21–30 at €40 (day 31 has no history) = €500, plus 21/31 of €310 = €210.
	if p.ExpectedRest != e(710) || p.ProjectedLeft != e(-310) || p.ExpectedDay != e(710)/21 {
		t.Fatalf("rest %v left %v day %v", p.ExpectedRest, p.ProjectedLeft, p.ExpectedDay)
	}
	if got := *p.Days[30].Proj; got != e(100)+e(710) {
		t.Errorf("month-end forecast %v", got)
	}
	if p.Days[8].Proj != nil || *p.Days[9].Proj != e(100) {
		t.Errorf("forecast starts today at what is spent: %v", p.Days[9].Proj)
	}
	// €400 left: €100 + €10/day to the 20th, then €40/day plus one-offs passes it on the 25th.
	if p.RunOutDay != 25 {
		t.Errorf("run out day %d", p.RunOutDay)
	}
	// No history: this month's pace, straight.
	q := PlanPulse{SafeToSpend: e(400), AvgDay: e(10)}
	q.fillTimeline(map[string]map[int]money.Cents{"2026-10": {5: e(100)}}, nil, now)
	if q.ExpectedRest != e(210) || q.ProjectedLeft != e(190) || q.RunOutDay != 0 {
		t.Errorf("no history: rest %v left %v run out %d", q.ExpectedRest, q.ProjectedLeft, q.RunOutDay)
	}
}
