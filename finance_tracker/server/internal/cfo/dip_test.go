package cfo_test

import (
	"strings"
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/dip"
	"ft/internal/market"
	"ft/internal/money"
	. "ft/internal/testutil"
)

// A fund at two rungs of the dip plan shows on Home's bell (as a note, with
// the amount) and as one phone alert per rung, with no amounts on the phone.
func TestDipRungReachedReachesTheBellAndPhone(t *testing.T) {
	d := DB(t)
	s := dip.Settings{Funds: []dip.Fund{{Symbol: "BELLTEST.DE", Name: "Test fund", Reserve: money.FromFloat(3000)}}}
	if err := dip.SaveSettings(d, s); err != nil {
		t.Fatal(err)
	}
	bars := make([]market.Bar, 1200)
	for i := range bars {
		p := 100.0
		if i >= 1100 {
			p = 60
		}
		bars[i] = market.Bar{Date: time.Date(2022, 1, 3, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i/5*7+i%5).Format("2006-01-02") /* weekdays */, High: p * 1.001, Low: p * 0.999, Close: p}
	}
	dip.Build(s, func(string) (*market.Series, error) { return &market.Series{Currency: "EUR", Bars: bars}, nil })

	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.Local)
	o, err := cfo.BuildOverview(d, now, 0)
	if err != nil {
		t.Fatal(err)
	}
	var bell *cfo.Action
	for i, a := range o.Actions {
		if a.Kind == "dip" {
			bell = &o.Actions[i]
		}
	}
	if bell == nil || bell.Level != "info" || !strings.Contains(bell.Text, "€2,000") || bell.Link != "/wealth/dips" {
		t.Fatalf("bell %+v", bell)
	}
	alerts, err := cfo.Alerts(d, now)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, a := range alerts {
		if strings.HasPrefix(a.Key, "dip:") {
			keys = append(keys, a.Key)
			if strings.Contains(a.Title+a.Message, "€") {
				t.Fatalf("amount on the phone: %+v", a)
			}
		}
	}
	if len(keys) != 2 || !strings.HasPrefix(keys[0], "dip:funds:BELLTEST.DE:t1:") || !strings.HasPrefix(keys[1], "dip:funds:BELLTEST.DE:t2:") {
		t.Fatalf("%v", keys)
	}
}

// A bet at a rung links to the Bets view, on the bell and on the phone.
func TestDipBetLinksToBets(t *testing.T) {
	d := DB(t)
	if err := dip.SavePlan(d, dip.Bets, dip.Settings{Funds: []dip.Fund{{Symbol: "BETTEST", Reserve: money.FromFloat(900)}}}); err != nil {
		t.Fatal(err)
	}
	bars := make([]market.Bar, 1200)
	for i := range bars {
		p := 100.0
		if i >= 1100 {
			p = 60
		}
		bars[i] = market.Bar{Date: time.Date(2022, 1, 3, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i/5*7+i%5).Format("2006-01-02") /* weekdays */, High: p * 1.001, Low: p * 0.999, Close: p}
	}
	dip.Build(dip.LoadPlan(d, dip.Bets), func(string) (*market.Series, error) { return &market.Series{Currency: "EUR", Bars: bars}, nil })
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.Local)
	o, err := cfo.BuildOverview(d, now, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range o.Actions {
		if a.Kind == "dip" && strings.Contains(a.Text, "BETTEST") {
			found = a.Link == "/wealth/bets"
		}
	}
	alerts, _ := cfo.Alerts(d, now)
	push := false
	for _, a := range alerts {
		if strings.HasPrefix(a.Key, "dip:bets:BETTEST:") {
			push = a.URL == "/#/wealth/bets"
		}
	}
	if !found || !push {
		t.Fatalf("bell %v push %v", found, push)
	}
}
