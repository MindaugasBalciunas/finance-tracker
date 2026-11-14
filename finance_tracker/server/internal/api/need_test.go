package api_test

import (
	"net/url"
	"testing"
	"time"

	"ft/internal/auth"

	"ft/internal/cfo"
	"ft/internal/ledger"
	"ft/internal/merchant"
	"ft/internal/plan"
	. "ft/internal/testutil"
)

// The Ledger's essential/optional view lists exactly the payments behind the
// Free money card's split: free-spending expenses by their own category,
// with what the fixed lines claim left out — alimony (an essential category),
// a subscription kept as a fixed line (an optional one), the lease rows of a
// category+tag line (while untagged fuel stays) and a tag-only line's rows
// appear in neither — so its totals match the card to the cent.
func TestLedgerNeedFilterMatchesTheFreeMoneySplit(t *testing.T) {
	s, c := newServer(t)
	for _, b := range []plan.Budget{
		{Name: "Alimony", Kind: "fixed", Categories: []string{"kids.alimony"}, Amount: E(1000)},
		{Name: "Streaming", Kind: "fixed", Categories: []string{"subscriptions.media"}, Amount: E(15)},
		// A lease: only transport rows tagged "lease" are the obligation — fuel stays a choice.
		{Name: "Car lease", Kind: "fixed", Categories: []string{"transport"}, Tag: "lease", Amount: E(300)},
		// A tag-only line: every row tagged "school" is the obligation.
		{Name: "School", Kind: "fixed", Tag: "school", Amount: E(80)},
		{Name: "Food", Kind: "spending", Categories: []string{"food"}, Amount: E(400)},
	} {
		if err := plan.Save(s.DB, &b, ""); err != nil {
			t.Fatal(err)
		}
	}
	for _, x := range []struct {
		cat string
		eur float64
	}{{"kids.alimony", 1000}, {"subscriptions.media", 15}, {"food.groceries", 120}, {"food.restaurants", 45}, {"leisure.events", 30}, {"transport.fuel", 60}} {
		Tx(t, s.DB, ledger.Tx{Date: "2026-10-10", Amount: E(x.eur), Category: x.cat, AccountID: "swed"})
	}
	Tx(t, s.DB, ledger.Tx{Date: "2026-10-11", Amount: E(300), Category: "transport.car_fees", AccountID: "swed", Tags: []string{"lease"}})
	Tx(t, s.DB, ledger.Tx{Date: "2026-10-12", Amount: E(80), Category: "leisure.hobbies", AccountID: "swed", Tags: []string{"school"}})
	Tx(t, s.DB, ledger.Tx{Date: "2026-09-30", Amount: E(99), Category: "leisure.events", AccountID: "swed"}) // other month
	Tx(t, s.DB, ledger.Tx{Date: "2026-10-05", Amount: E(20), Category: "transfer.internal", AccountID: "swed", ToAccountID: "seb"})

	var ess, opt ledger.ListResult
	c.ok("GET", "/transactions?need=essential&from=2026-10-01&to=2026-10-31", nil, &ess)
	c.ok("GET", "/transactions?need=optional&from=2026-10-01&to=2026-10-31", nil, &opt)
	if ess.Total != 2 || ess.Expense != E(180) || opt.Total != 2 || opt.Expense != E(75) {
		t.Fatalf("essential %d rows %v, optional %d rows %v", ess.Total, ess.Expense, opt.Total, opt.Expense)
	}
	for _, x := range append(ess.Items, opt.Items...) {
		if x.Category == "kids.alimony" || x.Category == "subscriptions.media" || x.Category == "transport.car_fees" || x.Category == "leisure.hobbies" || x.Kind != "expense" {
			t.Errorf("%s shouldn't be in the split", x.Category)
		}
	}
	r, err := cfo.BudgetReport(s.DB, "2026-10", time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if r.FreeMoney.Essential != ess.Expense || r.FreeMoney.Optional != opt.Expense {
		t.Fatalf("card essential %v optional %v ≠ ledger %v / %v", r.FreeMoney.Essential, r.FreeMoney.Optional, ess.Expense, opt.Expense)
	}
	// Anything else is no filter at all.
	var all ledger.ListResult
	c.ok("GET", "/transactions?need=nonsense&from=2026-10-01&to=2026-10-31", nil, &all)
	if all.Total != 9 {
		t.Errorf("unknown need value filtered: %d rows", all.Total)
	}
}

// The Optional insights page reads one route; on a fresh book every list is
// empty, never null, and an API token may read it like the other insights.
func TestOptionalInsightsRoute(t *testing.T) {
	s, c := newServer(t)
	Tx(t, s.DB, ledger.Tx{Date: time.Now().Format("2006-01-02"), Amount: E(12), Category: "leisure.events", Merchant: "Cinema", AccountID: "swed"})
	var r cfo.OptionalReport
	c.ok("GET", "/insights/optional", nil, &r)
	if r.Spent != E(12) || r.Leaks.SmallOften == nil || r.Leaks.Creeping == nil || len(r.Merchants) != 1 || r.Leaks.WhileOver.ThisMonth != 0 {
		t.Fatalf("%+v", r)
	}
	a := &auth.Service{DB: s.DB}
	a.SetupPin("", "1234")
	ro, _ := a.MintToken(false)
	if code, _ := c.with("Authorization", "Bearer "+ro).do("GET", "/insights/optional", nil); code != 200 {
		t.Errorf("read token: %d", code)
	}
}

// Verdicts: set with a session, changed, cleared; the server fills the name
// and usual month itself; a read token can see them but never set one.
func TestOptionalVerdictsAreTheOwnersToSet(t *testing.T) {
	s, c := newServer(t)
	for i := 0; i < 4; i++ {
		Tx(t, s.DB, ledger.Tx{Date: time.Now().AddDate(0, -1-i, 0).Format("2006-01") + "-10", Amount: E(30), Category: "leisure.going_out", Merchant: "Bar Nr 1", AccountID: "swed"})
	}
	key := merchant.Key("Bar Nr 1")
	c.ok("PUT", "/insights/optional/verdicts", map[string]string{"kind": "merchant", "key": key, "verdict": "cut", "name": "spoofed"}, nil)
	var r cfo.OptionalReport
	c.ok("GET", "/insights/optional", nil, &r)
	if len(r.Judge.Cutting) != 1 || r.Judge.Cutting[0].Name != "Bar Nr 1" || r.Judge.Cutting[0].Usual != E(20) {
		t.Fatalf("cutting %+v", r.Judge.Cutting) // 4 × €30 over the last six months = €20 a month
	}
	if code, _ := c.do("PUT", "/insights/optional/verdicts", map[string]string{"kind": "merchant", "key": key, "verdict": "maybe"}); code != 400 {
		t.Errorf("unknown verdict: %d", code)
	}
	if code, _ := c.do("PUT", "/insights/optional/verdicts", map[string]string{"kind": "merchant", "key": "NOWHERE", "verdict": "cut"}); code != 400 {
		t.Errorf("a place not in the report: %d", code)
	}
	c.ok("DELETE", "/insights/optional/verdicts?kind=merchant&key="+url.QueryEscape(key), nil, nil)
	c.ok("GET", "/insights/optional", nil, &r)
	if len(r.Judge.Cutting) != 0 || r.Judge.Open != E(120) {
		t.Fatalf("after clearing: %+v", r.Judge)
	}

	a := &auth.Service{DB: s.DB}
	a.SetupPin("", "1234")
	ro, _ := a.MintToken(false)
	rw, _ := a.MintToken(true)
	for _, tok := range []string{ro, rw} {
		if code, _ := c.with("Authorization", "Bearer "+tok).do("PUT", "/insights/optional/verdicts", map[string]string{"kind": "merchant", "key": key, "verdict": "worth"}); code != 403 {
			t.Errorf("token set a verdict: %d", code)
		}
		if code, _ := c.with("Authorization", "Bearer "+tok).do("DELETE", "/insights/optional/verdicts?kind=merchant&key="+url.QueryEscape(key), nil); code != 403 {
			t.Errorf("token cleared a verdict: %d", code)
		}
	}
	if code, _ := c.with("Authorization", "Bearer "+ro).do("GET", "/insights/optional", nil); code != 200 {
		t.Errorf("read token can't read: %d", code)
	}
}
