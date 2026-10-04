package cfo_test

import (
	"strings"
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/ledger"
	. "ft/internal/testutil"
)

func TestMonthReview(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 7; i++ {
		m := now.AddDate(0, -i, 0)
		day := func(n int) string { return time.Date(m.Year(), m.Month(), n, 0, 0, 0, 0, time.UTC).Format("2006-01-02") }
		Tx(t, d, ledger.Tx{Date: day(15), Amount: E(5000), Category: "salary", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: day(5), Amount: E(400), Category: "food.groceries", AccountID: "swed"})
		if i == 1 { // September: restaurants blow up
			Tx(t, d, ledger.Tx{Date: day(20), Amount: E(900), Category: "food.restaurants", Merchant: "Fancy", AccountID: "swed"})
		}
	}
	Bal(t, d, "swed", "2026-08-31", 10000)
	Bal(t, d, "swed", "2026-09-30", 13000)
	r, err := cfo.BuildMonthReview(d, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if r.Month != "2026-09" || !r.Complete || len(r.Daily) != 30 || len(r.Trend) != 12 {
		t.Fatalf("defaults to last complete month: %+v", r.Month)
	}
	if r.Flow.Spending != E(1300) || r.SixMonthAvg.Spending != E(400) {
		t.Fatalf("%v vs %v", r.Flow.Spending, r.SixMonthAvg.Spending)
	}
	if r.Categories[0].Category != "food" || r.Categories[0].Delta != E(900) || r.Categories[0].Top[0].Merchant != "Fancy" {
		t.Fatalf("%+v", r.Categories[0])
	}
	if r.NetWorth.Change != E(3000) {
		t.Fatal(r.NetWorth)
	}
	if r.Tone == "" || r.Verdict == "" {
		t.Fatal("verdict")
	}
	joined := ""
	for _, h := range r.Highlights {
		joined += h.Text + " | "
	}
	if !strings.Contains(joined, "above your six-month average") || !strings.Contains(joined, "Food ran") {
		t.Fatal(joined)
	}
}

func TestChecks(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	Bal(t, d, "ibkr", "2026-06-01", 100)
	Bal(t, d, "house", "2024-01-01", 300000)
	d.Exec(`INSERT INTO bank_connections(aspsp_name,status,valid_until,created_at,updated_at) VALUES('Swedbank','authorized','2026-10-10T00:00:00Z','x','x')`)
	d.Exec(`INSERT INTO bank_inbox(external_id,date,kind,amount,state,first_seen_at,last_seen_at) VALUES('e1','2026-10-01','expense',100,'open','x','x')`)
	for i := 0; i < 5; i++ {
		Tx(t, d, ledger.Tx{Date: "2026-09-1" + string(rune('0'+i)), Amount: E(5), Category: "leisure", AccountID: "swed"})
	}
	all := ""
	for _, c := range cfo.Checks(d, now) {
		all += c.Level + ":" + c.Text + "\n"
	}
	for _, want := range []string{"IBKR is over 45 days old", "House was last valued", "expires in 6 days", "1 bank row is waiting", "only have a broad category", "No backup downloaded"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in\n%s", want, all)
		}
	}
	if !strings.HasPrefix(all, "warn:") {
		t.Error("warnings first")
	}
}

func TestLateSalaryIsExplained(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	for i := 2; i <= 7; i++ {
		m := now.AddDate(0, -i, 0)
		Tx(t, d, ledger.Tx{Date: time.Date(m.Year(), m.Month(), 15, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), Amount: E(5000), Category: "salary", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: time.Date(m.Year(), m.Month(), 5, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), Amount: E(3000), Category: "food", AccountID: "swed"})
	}
	Tx(t, d, ledger.Tx{Date: "2026-09-15", Amount: E(2000), Category: "salary", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: "2026-09-05", Amount: E(3000), Category: "food", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: "2026-10-01", Amount: E(3000), Category: "salary", AccountID: "swed"})
	r, _ := cfo.BuildMonthReview(d, "2026-09", now)
	if r.Tone == "bad" || !strings.Contains(r.Verdict, "late salary") || !strings.Contains(r.Highlights[0].Text, "€3,000") {
		t.Fatalf("%s %s %+v", r.Tone, r.Verdict, r.Highlights)
	}
}
