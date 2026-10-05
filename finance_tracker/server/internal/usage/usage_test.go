package usage_test

import (
	"testing"
	"time"

	. "ft/internal/testutil"
	"ft/internal/usage"
)

func TestHuntsBouncesAndPrivacy(t *testing.T) {
	d := DB(t)
	base := time.Now().Add(-2 * time.Hour).UTC()
	at := func(sec int) string { return base.Add(time.Duration(sec) * time.Second).Format(time.RFC3339) }
	x := 0.25
	evs := []usage.Event{
		// Session 1: Home, then three quick hops before settling on Balance history.
		{At: at(0), Kind: "view", Path: "/", DwellMS: 30_000},
		{At: at(30), Kind: "view", Path: "/wealth?tab=x", From: "/", DwellMS: 3_000},
		{At: at(33), Kind: "view", Path: "/wealth/investments", DwellMS: 2_000},
		{At: at(35), Kind: "view", Path: "/wealth/loans", DwellMS: 2_000},
		{At: at(37), Kind: "view", Path: "/wealth/history", DwellMS: 60_000},
		{At: at(40), Kind: "action", Path: "/wealth/history", Label: "Add 12 to ledger €19.58", X: &x},
		// Session 2 (an hour later): same hunt; plus a bounce on Trips.
		{At: at(3600), Kind: "view", Path: "/plan", DwellMS: 25_000},
		{At: at(3625), Kind: "view", Path: "/plan/trips", DwellMS: 1_500},
		{At: at(3627), Kind: "view", Path: "/plan", DwellMS: 2_000},
		{At: at(3629), Kind: "view", Path: "/wealth", DwellMS: 2_000},
		{At: at(3631), Kind: "view", Path: "/wealth/loans", DwellMS: 2_000},
		{At: at(3633), Kind: "view", Path: "/wealth/history", DwellMS: 45_000},
		{At: at(3700), Kind: "view", Path: "bad path!", DwellMS: 1},
	}
	n, err := usage.Record(d, evs)
	if err != nil || n != 12 {
		t.Fatalf("recorded %d %v", n, err)
	}
	s, err := usage.Summarise(d, 30, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if s.Sessions != 2 || s.Views != 11 {
		t.Fatalf("sessions %d views %d", s.Sessions, s.Views)
	}
	if len(s.Hunts) == 0 || s.Hunts[0].Target != "/wealth/history" || s.Hunts[0].Count != 2 {
		t.Fatalf("hunts %+v", s.Hunts)
	}
	var trips usage.PageStat
	for _, p := range s.Pages {
		if p.Path == "/plan/trips" {
			trips = p
		}
		if p.Path == "/wealth?tab=x" {
			t.Error("query strings must be stripped")
		}
	}
	if trips.Bounces != 1 {
		t.Errorf("Trips bounce: %+v", trips)
	}
	if len(s.Actions) != 1 || s.Actions[0].Label != "Add # to ledger €#" {
		t.Errorf("numbers masked in labels: %+v", s.Actions)
	}
	if len(s.Suggestions) == 0 {
		t.Error("a repeated hunt should produce a suggestion")
	}
	raw, _ := usage.Events(d, 30, time.Now())
	if len(raw) != 12 || raw[5].X == nil || *raw[5].X != 0.25 {
		t.Errorf("raw events keep click positions: %+v", raw[5])
	}
	usage.Clear(d)
	if s2, _ := usage.Summarise(d, 30, time.Now()); s2.Views != 0 || s2.Suggestions == nil || s2.Hunts == nil {
		t.Errorf("clear, and empty lists are lists: %+v", s2)
	}
}
