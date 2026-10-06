package insights_test

import (
	"testing"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	. "ft/internal/testutil"
)

func TestTagStats(t *testing.T) {
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	mk := func(date, kind, cat, merchant string, eur float64, tags ...string) ledger.Tx {
		x := tx(date, kind, cat, eur, "swed")
		x.Merchant, x.Tags = merchant, tags
		return x
	}
	txs := []ledger.Tx{
		mk("2026-09-10", "expense", "travel.trip", "Hotel", 300, "trip:rome"),
		mk("2026-09-12", "expense", "food.restaurants", "Trattoria", 100, "trip:rome", "kristina"),
		mk("2026-09-20", "income", "income.refunds", "Hotel", 50, "trip:rome"),
		mk("2024-01-05", "expense", "kids.general", "Toys", 40, "kids"),
		mk("2026-09-01", "expense", "food.groceries", "Lidl", 200),
	}
	rules := []ledger.Rule{{AddTags: []string{"kids"}, Enabled: true}}
	r := insights.TagStats(txs, rules, now)
	if len(r.Tags) != 3 || r.Tags[0].Tag != "trip:rome" {
		t.Fatalf("%+v", r.Tags)
	}
	rome := r.Tags[0]
	if !rome.Trip || rome.Count != 3 || rome.Spent != E(400) || rome.Income != E(50) || rome.Net != E(350) || rome.Year != E(350) || rome.Months[11] != 0 || rome.Months[10] != E(350) {
		t.Errorf("rome %+v", rome)
	}
	if rome.Cats[0].Name != "travel.trip" || rome.Merch[0].Name != "Hotel" || rome.PerMonth != E(350) {
		t.Errorf("rome breakdown %+v %+v %v", rome.Cats, rome.Merch, rome.PerMonth)
	}
	var kids insights.TagStat
	for _, s := range r.Tags {
		if s.Tag == "kids" {
			kids = s
		}
	}
	if kids.Year != 0 || kids.Rules != 1 || kids.Net != E(40) {
		t.Errorf("kids %+v", kids)
	}
	// 400 of the year's 600 spending carries a tag.
	if r.YearSpent != E(600) || r.TaggedShare < 0.66 || r.TaggedShare > 0.67 {
		t.Errorf("share %v of %v", r.TaggedShare, r.YearSpent)
	}
}

func TestRuleStats(t *testing.T) {
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	mk := func(date, cat, merchant string) ledger.Tx {
		x := tx(date, "expense", cat, 5, "swed")
		x.Merchant = merchant
		return x
	}
	txs := []ledger.Tx{
		mk("2026-09-01", "food.coffee", "Coffee Inn"),
		mk("2026-09-02", "dating", "Coffee Inn"), // re-filed by hand
		mk("2026-09-03", "dating", "Coffee Inn"),
		mk("2025-01-01", "food.groceries", "Lidl"),
	}
	rules := []ledger.Rule{
		{ID: 1, Pattern: "coffee", SetCategory: "food.coffee", Enabled: true},
		{ID: 2, Pattern: "COFFEE", SetCategory: "food.restaurants", Enabled: true}, // always beaten by 1
		{ID: 3, Pattern: "coffee", SetCategory: "food.coffee", Enabled: true},      // duplicate of 1
		{ID: 4, Pattern: "starbucks", SetCategory: "food.coffee", Enabled: true},   // never matches
		{ID: 5, Pattern: "lidl", AddTags: []string{"house"}, Enabled: true},
	}
	r := insights.RuleStats(txs, rules, now)
	st := map[int64]insights.RuleStat{}
	for _, s := range r.Rules {
		st[s.ID] = s
	}
	if s := st[1]; s.Matches != 3 || s.Recent != 3 || s.Decides != 3 || s.Overridden != 2 || s.Last != "2026-09-03" {
		t.Errorf("rule 1 %+v", s)
	}
	if !st[2].Shadowed || st[2].Decides != 0 || st[3].Duplicate != 1 || st[4].Matches != 0 || st[5].Shadowed || st[5].Recent != 0 {
		t.Errorf("2 %+v 3 %+v 4 %+v 5 %+v", st[2], st[3], st[4], st[5])
	}
	// Rule 3 is shadowed as well as a duplicate; the report counts each once per kind.
	if r.Unused != 1 || r.Dupes != 1 || r.Overrode != 1 || r.Coverage != 1 || len(r.Top) != 3 || r.Top[0] != 1 {
		t.Errorf("report %+v", r)
	}
}

func TestCategoryAndAccountUsage(t *testing.T) {
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	txs := []ledger.Tx{tx("2026-09-01", "expense", "food.coffee", 3, "swed"), tx("2020-01-01", "expense", "food.coffee", 4, "swed"), tx("2026-09-02", "transfer", "transfer.own", 100, "swed", "seb")}
	u := insights.CategoryUsage(txs, []ledger.Rule{{SetCategory: "food.coffee"}}, map[string]bool{"food": true}, now)
	if c := u["food.coffee"]; c.Count != 2 || c.Year != E(3) || c.Last != "2026-09-01" || c.Rules != 1 || c.Budgeted {
		t.Errorf("coffee %+v", c)
	}
	if !u["food"].Budgeted || u["food"].Count != 0 {
		t.Errorf("food %+v", u["food"])
	}
	a := insights.AccountUsage(txs)
	if a["swed"].Count != 3 || a["seb"].Count != 1 || a["seb"].Last != "2026-09-02" {
		t.Errorf("accounts %+v", a)
	}
}

// Standing orders are found; irregular top-ups are not. Saved movements keep
// their accounts and day, and know whether this month's one has happened.
func TestRecurringTransfers(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	mk := func(date, from, to, cat, merchant string, eur float64) {
		Tx(t, d, ledger.Tx{Date: date, Kind: "transfer", Amount: E(eur), AccountID: from, ToAccountID: to, Category: cat, Merchant: merchant})
	}
	for _, m := range []string{"2026-07", "2026-08", "2026-09", "2026-10"} {
		mk(m+"-10", "swed", "artea", "transfer.pension", "Artea", 200)
	}
	for _, day := range []string{"2026-07-03", "2026-07-29", "2026-08-17", "2026-09-01"} { // whenever there's spare cash
		mk(day, "swed", "ibkr", "transfer.invest", "IBKR", 1000)
	}
	txs, _ := ledger.All(d, ledger.Filter{})
	det := insights.DetectTransfers(txs, now, map[string]string{"artea": "Artea"})
	if len(det) != 1 || det[0].Merchant != "Artea" || det[0].Day != 10 || det[0].FromAccount != "swed" || det[0].ToAccount != "artea" || det[0].Amount != E(200) {
		t.Fatalf("detected %+v", det)
	}

	// A planned transfer (no history yet) and validation.
	bad := []insights.RecurringItem{
		{Merchant: "x", Kind: "loan"},
		{Merchant: "x", Kind: "transfer", FromAccount: "nope"},
		{Merchant: "x", Kind: "transfer", FromAccount: "swed", ToAccount: "swed"},
		{Merchant: "x", Day: 40},
	}
	for _, it := range bad {
		if err := insights.SaveRecurringItem(d, &it); err == nil {
			t.Errorf("accepted %+v", it)
		}
	}
	ibkr := insights.RecurringItem{Merchant: "IBKR monthly", Kind: "transfer", FromAccount: "swed", ToAccount: "ibkr", Amount: E(1000), Day: 11}
	if err := insights.SaveRecurringItem(d, &ibkr); err != nil {
		t.Fatal(err)
	}
	all, _, err := insights.RecurringAll(d, txs, now)
	if err != nil {
		t.Fatal(err)
	}
	get := func(name string) insights.Recurring {
		for _, r := range all {
			if r.Merchant == name {
				return r
			}
		}
		t.Fatalf("%s missing from %+v", name, all)
		return insights.Recurring{}
	}
	if a := get("Artea"); a.Kind != "transfer" || !a.Done || a.Next != "2026-11-10" {
		t.Errorf("Artea %+v", a)
	}
	if p := get("IBKR monthly"); p.Kind != "transfer" || p.Done || p.Next != "2026-10-11" || !p.Overdue {
		t.Errorf("planned IBKR %+v", p)
	}
	// Costs stay costs: transfers never reach the bills list.
	bills, _, _ := insights.RecurringCosts(d, txs, now)
	for _, b := range bills {
		if b.Kind != "bill" {
			t.Errorf("transfer among costs: %+v", b)
		}
	}
}
