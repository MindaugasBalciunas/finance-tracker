package insights_test

import (
	"testing"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/money"
)

// Fun facts rank employers by what they paid, find the most visited shop,
// count little habits — and never turn a person paid an obligation (an
// ex-partner) into a "favourite shop".
func TestFunFacts(t *testing.T) {
	e := money.FromFloat
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	var txs []ledger.Tx
	add := func(date, kind, cat, m string, eur float64, tags ...string) {
		txs = append(txs, ledger.Tx{Date: date, Kind: kind, Category: cat, Merchant: m, Amount: e(eur), Tags: tags})
	}
	add("2013-03-18", "income", "salary", "App Camp", 174)
	for _, d := range []string{"2024-01-10", "2024-02-10", "2024-03-10"} {
		add(d, "income", "salary", "Big Co", 5000)
	}
	add("2026-09-10", "income", "salary", "New Co", 5000)
	for _, d := range []string{"2026-09-02", "2026-09-09", "2026-09-16", "2026-09-23"} {
		add(d, "expense", "food.groceries", "Lidl", 40)
		add(d, "expense", "food.coffee", "Caffeine", 4)
	}
	add("2026-09-30", "expense", "kids.alimony", "Ex", 1000)
	add("2026-08-30", "expense", "kids.general", "Ex", 3000) // a person, not a shop
	add("2026-07-01", "expense", "travel.lodging", "Hotel", 900, "trip:rome")

	by := map[string]insights.Fact{}
	for _, f := range insights.FunFacts(txs, nil, now, "", "") {
		by[f.Key] = f
	}
	if f := by["employers"]; f.Value != "Big Co" || len(f.Items) != 3 || f.Items[0].Value != "€15,000" {
		t.Errorf("employers %+v", f)
	}
	if f := by["first_pay"]; f.Value != "€174" {
		t.Errorf("first pay %+v", f)
	}
	if f := by["visits"]; f.Value != "Caffeine" && f.Value != "Lidl" {
		t.Errorf("visits %+v", f)
	}
	for _, it := range by["spent_most"].Items {
		if it.Label == "Ex" {
			t.Errorf("a person paid an obligation shows as a shop: %+v", by["spent_most"])
		}
	}
	if f := by["biggest"]; f.Value != "€900" {
		t.Errorf("biggest purchase leaves obligations and people out: %+v", f)
	}
	found := false
	for _, it := range by["habits"].Items {
		if it.Label == "Coffees" && it.Value == "4" {
			found = true
		}
	}
	if !found {
		t.Errorf("habits %+v", by["habits"])
	}
	if f := by["trips"]; len(f.Items) != 1 || f.Items[0].Label != "Rome" {
		t.Errorf("trips %+v", f)
	}
}

// With a period picked, only its transactions count; the lifetime-only
// facts (first pay cheque, this year's favourite) are left out and the
// story counts days.
func TestFunFactsInAPeriod(t *testing.T) {
	e := money.FromFloat
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	txs := []ledger.Tx{
		{Date: "2013-03-18", Kind: "income", Category: "salary", Merchant: "App Camp", Amount: e(174)},
		{Date: "2026-09-10", Kind: "income", Category: "salary", Merchant: "New Co", Amount: e(5000)},
		{Date: "2025-05-02", Kind: "expense", Category: "food.groceries", Merchant: "Old Shop", Amount: e(40)},
		{Date: "2026-09-12", Kind: "expense", Category: "food.groceries", Merchant: "Lidl", Amount: e(40)},
		{Date: "2026-09-19", Kind: "expense", Category: "food.groceries", Merchant: "Lidl", Amount: e(30)},
	}
	by := map[string]insights.Fact{}
	for _, f := range insights.FunFacts(txs, nil, now, "2026-09-01", "2026-09-30") {
		by[f.Key] = f
	}
	if _, ok := by["first_pay"]; ok {
		t.Error("first pay cheque in a period")
	}
	if f := by["employers"]; f.Value != "New Co" || len(f.Items) != 1 {
		t.Errorf("employers in September %+v", f)
	}
	if f := by["visits"]; f.Value != "Lidl" || len(f.Items) != 1 {
		t.Errorf("visits in September %+v", f)
	}
	l := by["lifetime"]
	if l.Title != "Earned in this period" || l.Value != "€5,000" || l.Items[0].Label != "days" || l.Items[0].N != 30 {
		t.Errorf("story %+v", l)
	}
}
