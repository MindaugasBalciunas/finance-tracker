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
	// The list says which rows a fixed line claims, so the Ledger offers no
	// must/optional mark on them: the four obligations, nothing else.
	var month ledger.ListResult
	c.ok("GET", "/transactions?from=2026-10-01&to=2026-10-31", nil, &month)
	claimed := 0
	for _, x := range month.Items {
		if x.Fixed {
			claimed++
		}
		if x.Fixed != (x.Category == "kids.alimony" || x.Category == "subscriptions.media" || x.Category == "transport.car_fees" || x.Category == "leisure.hobbies") {
			t.Errorf("%s fixed=%v", x.Category, x.Fixed)
		}
	}
	if claimed != 4 {
		t.Errorf("%d rows claimed by fixed lines, want 4", claimed)
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
	for i := 0; i < 6; i++ { // a habit: six of the twelve months
		Tx(t, s.DB, ledger.Tx{Date: time.Now().AddDate(0, -1-i, 0).Format("2006-01") + "-10", Amount: E(30), Category: "leisure.going_out", Merchant: "Bar Nr 1", AccountID: "swed"})
	}
	key := merchant.Key("Bar Nr 1")
	c.ok("PUT", "/insights/optional/verdicts", map[string]string{"kind": "merchant", "key": key, "verdict": "cut", "name": "spoofed"}, nil)
	var r cfo.OptionalReport
	c.ok("GET", "/insights/optional", nil, &r)
	if len(r.Judge.Cutting) != 1 || r.Judge.Cutting[0].Name != "Bar Nr 1" || r.Judge.Cutting[0].Usual != E(15) {
		t.Fatalf("cutting %+v", r.Judge.Cutting) // 6 × €30 in the twelve months = €15 a month
	}
	if code, _ := c.do("PUT", "/insights/optional/verdicts", map[string]string{"kind": "merchant", "key": key, "verdict": "maybe"}); code != 400 {
		t.Errorf("unknown verdict: %d", code)
	}
	if code, _ := c.do("PUT", "/insights/optional/verdicts", map[string]string{"kind": "merchant", "key": "NOWHERE", "verdict": "cut"}); code != 400 {
		t.Errorf("a place not in the report: %d", code)
	}
	c.ok("DELETE", "/insights/optional/verdicts?kind=merchant&key="+url.QueryEscape(key), nil, nil)
	c.ok("GET", "/insights/optional", nil, &r)
	if len(r.Judge.Cutting) != 0 || r.Judge.Open != E(180) {
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

// A mark on one payment wins over its category everywhere the split is
// shown: a restaurant marked a must moves from optional to essential in the
// Ledger filter, on the Free money card and in Insights → Optional, and
// groceries marked optional move the other way — the three still agree to
// the cent. Clearing the mark puts the payment back under its category.
// Editing the transaction keeps the mark; only an expense can carry one,
// and an API token can never set it.
func TestMarkingAPaymentMustOrOptionalMovesItInEverySplit(t *testing.T) {
	s, c := newServer(t)
	now := time.Now()
	day := now.Format("2006-01") + "-01"
	from, to := day, now.Format("2006-01")+"-31"
	groceries := Tx(t, s.DB, ledger.Tx{Date: day, Amount: E(120), Category: "food.groceries", Merchant: "Lidl", AccountID: "swed"})
	dinner := Tx(t, s.DB, ledger.Tx{Date: day, Amount: E(45), Category: "food.restaurants", Merchant: "Work lunch", AccountID: "swed"})
	Tx(t, s.DB, ledger.Tx{Date: day, Amount: E(30), Category: "leisure.events", Merchant: "Cinema", AccountID: "swed"})
	move := Tx(t, s.DB, ledger.Tx{Date: day, Amount: E(20), Category: "transfer.internal", AccountID: "swed", ToAccountID: "seb"})

	check := func(when string, essential, optional float64) {
		t.Helper()
		var ess, opt ledger.ListResult
		c.ok("GET", "/transactions?need=essential&from="+from+"&to="+to, nil, &ess)
		c.ok("GET", "/transactions?need=optional&from="+from+"&to="+to, nil, &opt)
		if ess.Expense != E(essential) || opt.Expense != E(optional) {
			t.Fatalf("%s: ledger essential %v optional %v, want %v / %v", when, ess.Expense, opt.Expense, essential, optional)
		}
		r, err := cfo.BudgetReport(s.DB, now.Format("2006-01"), now)
		if err != nil {
			t.Fatal(err)
		}
		if r.FreeMoney.Essential != ess.Expense || r.FreeMoney.Optional != opt.Expense {
			t.Fatalf("%s: card essential %v optional %v ≠ ledger %v / %v", when, r.FreeMoney.Essential, r.FreeMoney.Optional, ess.Expense, opt.Expense)
		}
		var o cfo.OptionalReport
		c.ok("GET", "/insights/optional", nil, &o)
		last := o.Months[len(o.Months)-1]
		if o.Spent != opt.Expense || last.Optional != opt.Expense || last.Essential != ess.Expense {
			t.Fatalf("%s: insights spent %v, month %+v ≠ ledger %v / %v", when, o.Spent, last, ess.Expense, opt.Expense)
		}
	}
	check("unmarked", 120, 75)

	var got ledger.Tx
	c.ok("PUT", "/transactions/"+itoa(dinner.ID)+"/need", map[string]string{"need": "must"}, &got)
	if got.Need != "must" {
		t.Fatalf("mark not returned: %+v", got)
	}
	check("dinner a must", 165, 30)
	c.ok("PUT", "/transactions/"+itoa(groceries.ID)+"/need", map[string]string{"need": "optional"}, nil)
	check("groceries optional", 45, 150)

	// Saving the transaction from the editor keeps the mark.
	got.Note, got.Need = "with the team", ""
	c.ok("PUT", "/transactions/"+itoa(dinner.ID), got, &got)
	if after, _ := ledger.Get(s.DB, dinner.ID); after.Need != "must" || after.Note != "with the team" || got.Need != "must" {
		t.Fatalf("edit dropped the mark: stored %+v, returned %+v", after, got)
	}

	c.ok("PUT", "/transactions/"+itoa(dinner.ID)+"/need", map[string]string{"need": ""}, nil)
	c.ok("PUT", "/transactions/"+itoa(groceries.ID)+"/need", map[string]string{"need": ""}, nil)
	check("cleared", 120, 75)

	if code, _ := c.do("PUT", "/transactions/"+itoa(dinner.ID)+"/need", map[string]string{"need": "maybe"}); code != 400 {
		t.Errorf("unknown mark: %d", code)
	}
	if code, _ := c.do("PUT", "/transactions/"+itoa(move.ID)+"/need", map[string]string{"need": "must"}); code != 400 {
		t.Errorf("a transfer marked: %d", code)
	}
	if code, _ := c.do("PUT", "/transactions/999999/need", map[string]string{"need": "must"}); code != 404 {
		t.Errorf("missing transaction: %d", code)
	}

	a := &auth.Service{DB: s.DB}
	a.SetupPin("", "1234")
	ro, _ := a.MintToken(false)
	rw, _ := a.MintToken(true)
	for _, tok := range []string{ro, rw} {
		if code, _ := c.with("Authorization", "Bearer "+tok).do("PUT", "/transactions/"+itoa(dinner.ID)+"/need", map[string]string{"need": "must"}); code != 403 {
			t.Errorf("token marked a payment: %d", code)
		}
	}
	// The neighbour that is allowed: a read-write token edits the
	// transaction itself, and can't slip a mark in with the edit.
	got.Need = "optional"
	if code, _ := c.with("Authorization", "Bearer "+rw).do("PUT", "/transactions/"+itoa(dinner.ID), got); code != 200 {
		t.Errorf("rw token edit: %d", code)
	}
	if after, _ := ledger.Get(s.DB, dinner.ID); after.Need != "" {
		t.Errorf("an edit set the mark: %q", after.Need)
	}
}
