package cfo_test

import (
	"fmt"
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/ledger"
	"ft/internal/plan"
	. "ft/internal/testutil"
)

// A household's six months, built so every optional-spending signal fires
// once, with the amounts worked out by hand:
//
//   - €1,000 income − €300 alimony (fixed) − €100 Travel fund = €600 a month
//     for day-to-day; groceries are essential, everything else optional.
//   - May–Oct: groceries €200 (2nd), restaurant €100 (5th), a bar €60 two
//     days after the salary (25th → 26th), Wolt delivery €10 a month, then
//     €60 a month from August.
//   - June: a €300 trip evening (tagged trip:rome) — never a "one-off".
//   - October: an extra €450 shop on the 10th tips the month over its €600;
//     afterwards a €40 concert, the €60 bar and a €80 flight paid from the
//     Travel fund. Coffee €4 × 8 on the 1st–8th.
//   - March: a €900 laptop.
//   - November so far: a €25 cinema.
func TestOptionalInsightsFindWhereOptionalMoneyLeaks(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	plan.SaveSettings(d, plan.Settings{IncomeMode: "manual", ManualIncome: E(1000)})
	trav := plan.Budget{Name: "Travel", Kind: "spending", Categories: []string{"travel"}, Amount: E(100), Fund: true, StartMonth: "2026-01"}
	for _, b := range []plan.Budget{
		{Name: "Alimony", Kind: "fixed", Categories: []string{"kids.alimony"}, Amount: E(300)},
		{Name: "Food", Kind: "spending", Categories: []string{"food"}, Amount: E(400)},
		trav,
	} {
		if err := plan.Save(d, &b, ""); err != nil {
			t.Fatal(err)
		}
	}
	tx := func(date, cat, merchant string, eur float64, tags ...string) {
		Tx(t, d, ledger.Tx{Date: date, Amount: E(eur), Category: cat, Merchant: merchant, AccountID: "swed", Tags: tags})
	}
	for _, m := range []string{"2026-05", "2026-06", "2026-07", "2026-08", "2026-09", "2026-10"} {
		tx(m+"-01", "kids.alimony", "", 300)
		tx(m+"-02", "food.groceries", "Maxima", 200)
		tx(m+"-05", "food.restaurants", "Bistro", 100)
		tx(m+"-25", "salary", "Employer", 1000)
		tx(m+"-26", "leisure.going_out", "Bar", 60)
		wolt := 10.0
		if m >= "2026-08" {
			wolt = 60
		}
		tx(m+"-03", "food.delivery", "Wolt", wolt)
	}
	tx("2026-06-10", "leisure.events", "Rome night", 300, "trip:rome")
	tx("2026-10-10", "food.groceries", "Maxima", 450)
	tx("2026-10-20", "leisure.events", "Concert", 40)
	tx("2026-10-22", "travel.flights", "airBaltic", 80)
	for day := 1; day <= 8; day++ {
		tx(fmt.Sprintf("2026-10-%02d", day), "food.coffee", "Caffeine", 4)
	}
	tx("2026-03-15", "shopping.electronics", "Apple", 900)
	tx("2026-11-05", "leisure.events", "Cinema", 25)

	r, err := cfo.OptionalInsights(d, now)
	if err != nil {
		t.Fatal(err)
	}
	// Optional a month: May, Jul 170; Jun 470 (trip night); Aug, Sep 220;
	// Oct 100+60+60+40+80+32 = 372 → median of the six is 220.
	if r.Spent != E(25) || r.Usual != E(220) || r.Free != E(700) {
		t.Fatalf("this month %v, usual %v, free %v", r.Spent, r.Usual, r.Free)
	}
	if r.Year != E(170+470+170+220+220+372+900) {
		t.Errorf("twelve months %v", r.Year)
	}
	if len(r.Months) != 13 || r.Months[12].Month != "2026-11" || r.Months[11].Optional != E(372) || r.Months[11].Essential != E(650) {
		t.Errorf("months %+v", r.Months[11:])
	}
	if c := r.Categories[0]; c.Key != "shopping.electronics" || c.Year != E(900) || c.Count != 1 {
		t.Errorf("top category %+v", c)
	}
	if m := r.Merchants[0]; m.Key != "Apple" {
		t.Errorf("top merchant %+v", m)
	}

	l := r.Leaks
	// June ran out on the 10th (200+10+100+300 > 600): the bar after it, €60.
	// October on the 10th: the concert and the bar, €100; the fund-paid flight doesn't count.
	if l.WhileOver.Year != E(160) || l.WhileOver.ThisMonth != 0 || len(l.WhileOver.Months) != 2 || l.WhileOver.Months[1].From != 11 {
		t.Errorf("while over %+v", l.WhileOver)
	}
	if len(l.SmallOften) != 1 || l.SmallOften[0].Merchant != "Caffeine" || l.SmallOften[0].Count != 8 || l.SmallOften[0].Yearly != E(32)*365/90 {
		t.Errorf("small and often %+v", l.SmallOften)
	}
	// €60 in the three days after each salary; the rest of those months
	// (600 restaurants + 30+180 Wolt + 300 Rome night + 152 October) over
	// 166 days, per 3 days.
	if p := l.Payday; p == nil || p.After != E(60) || p.Usual != E(1262)*3/166 || p.Extra != (E(60)-E(1262)*3/166)*12 || p.Months != 6 {
		t.Errorf("payday %+v", p)
	}
	// Wolt is a habit that grew; the one-off flight, concert and October's
	// coffee are new, not creeping.
	if len(l.Creeping) != 1 || l.Creeping[0].Merchant != "Wolt" || l.Creeping[0].Before != E(10) || l.Creeping[0].Now != E(60) || l.Creeping[0].Yearly != E(600) {
		t.Errorf("creeping %+v", l.Creeping)
	}
	if c := l.Convenience; c == nil || c.Convenience != E(60) || c.Groceries != E(350) || c.Yearly != E(720) {
		t.Errorf("convenience %+v", c)
	}
	if l.OneOffs.Year != E(900) || len(l.OneOffs.Items) != 1 || l.OneOffs.Items[0].Merchant != "Apple" {
		t.Errorf("one-offs %+v", l.OneOffs)
	}
}

// A new install: nothing spent, every list empty and never null.
func TestOptionalInsightsOnAnEmptyBook(t *testing.T) {
	r, err := cfo.OptionalInsights(DB(t), time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if r.Spent != 0 || r.Categories == nil || r.Merchants == nil || r.Leaks.SmallOften == nil || r.Leaks.Creeping == nil ||
		r.Leaks.WhileOver.Items == nil || r.Leaks.OneOffs.Items == nil || r.Leaks.Payday != nil {
		t.Fatalf("%+v", r)
	}
}

// A payday that is only a little busier is no splurge: €8 a year of
// difference stays quiet; and a place first paid this quarter is new, not
// creeping (a trip agency's deposits, a new phone bill).
func TestOptionalLeaksIgnoreNoiseAndNewPlaces(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	plan.SaveSettings(d, plan.Settings{IncomeMode: "manual", ManualIncome: E(3000)})
	tx := func(date, cat, merchant string, eur float64) {
		Tx(t, d, ledger.Tx{Date: date, Amount: E(eur), Category: cat, Merchant: merchant, AccountID: "swed"})
	}
	for _, m := range []string{"2026-05", "2026-06", "2026-07", "2026-08", "2026-09", "2026-10"} {
		tx(m+"-25", "salary", "Employer", 3000)
		for day := 1; day <= 28; day += 3 { // €10 every third day, payday or not
			tx(fmt.Sprintf("%s-%02d", m, day), "leisure.events", "Kino", 10)
		}
		if m >= "2026-08" {
			tx(m+"-12", "travel.trip", "Novaturas", 500)
		}
	}
	r, err := cfo.OptionalInsights(d, now)
	if err != nil {
		t.Fatal(err)
	}
	if p := r.Leaks.Payday; p == nil || p.Extra != 0 {
		t.Errorf("payday %+v", p)
	}
	if len(r.Leaks.Creeping) != 0 {
		t.Errorf("creeping %+v", r.Leaks.Creeping)
	}
}

// A salary on the 30th: the bar on the 1st is still payday spending. And a
// receipt split three ways is one purchase: three such €12 receipts are
// three small purchases (no habit yet), and the place counts 3 payments.
func TestOptionalPaydayAcrossMonthEndAndSplitReceipts(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	plan.SaveSettings(d, plan.Settings{IncomeMode: "manual", ManualIncome: E(3000)})
	for _, m := range []string{"2026-05", "2026-06", "2026-07", "2026-08", "2026-09", "2026-10"} {
		mt, _ := time.Parse("2006-01", m)
		pay := mt.AddDate(0, 1, -1).Format("2006-01-02") // the last day of the month
		Tx(t, d, ledger.Tx{Date: pay, Amount: E(3000), Category: "salary", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: mt.AddDate(0, 1, 0).Format("2006-01-02"), Amount: E(90), Category: "leisure.going_out", Merchant: "Bar", AccountID: "swed"})
	}
	for i := 0; i < 3; i++ {
		parent := Tx(t, d, ledger.Tx{Date: fmt.Sprintf("2026-10-1%d", i), Amount: E(4), Category: "food.coffee", Merchant: "Caffeine", AccountID: "swed"})
		for j := 0; j < 2; j++ {
			Tx(t, d, ledger.Tx{Date: fmt.Sprintf("2026-10-1%d", i), Amount: E(4), Category: "food.coffee", Merchant: "Caffeine", AccountID: "swed", SplitOf: parent.ID})
		}
	}
	r, err := cfo.OptionalInsights(d, now)
	if err != nil {
		t.Fatal(err)
	}
	if p := r.Leaks.Payday; p == nil || p.After != E(90) || p.Extra == 0 {
		t.Errorf("payday %+v", p)
	}
	if len(r.Leaks.SmallOften) != 0 {
		t.Errorf("split parts counted as purchases: %+v", r.Leaks.SmallOften)
	}
	for _, m := range r.Merchants {
		if m.Key == "Caffeine" && m.Count != 3 {
			t.Errorf("Caffeine counted %d payments", m.Count)
		}
	}
}
