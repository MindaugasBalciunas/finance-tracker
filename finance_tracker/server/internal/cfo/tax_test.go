package cfo_test

import (
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/ledger"
	"ft/internal/plan"
	. "ft/internal/testutil"
)

// The declaration ahead: salary above the 2026 25% bracket that 20%
// withholding leaves to pay, interest above its €500 exemption, and the
// 3rd-pillar refund only once the owner says the contract predates 2025.
// The net lands in the forecast in May.
func TestTaxYear(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	st := plan.LoadSettings(d)
	st.IncomeMode, st.GrossSalary = "gross", 8400
	if err := plan.SaveSettings(d, st); err != nil {
		t.Fatal(err)
	}
	Tx(t, d, ledger.Tx{Date: "2026-06-30", Amount: E(600), Category: "investment_income", AccountID: "swed"})
	for _, m := range []string{"2026-04", "2026-05", "2026-06", "2026-07", "2026-08", "2026-09"} {
		Tx(t, d, ledger.Tx{Date: m + "-10", Kind: "transfer", Amount: E(200), Category: "transfer.pension", AccountID: "swed", ToAccountID: "artea"})
	}
	tx, err := cfo.Tax(d, 2026, now)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]cfo.TaxItem{}
	for _, it := range tx.Items {
		by[it.Key] = it
	}
	// (100,800 − 83,237) × 5% = 878.15; interest (600 − 500) × 15% = 15.
	if by["bracket"].Amount != E(878.15) || by["interest"].Amount != E(15) || tx.Due != E(893.15) {
		t.Errorf("due %v, items %+v", tx.Due, tx.Items)
	}
	if p := by["pillar3"]; p.Kind != "info" || tx.Refund != 0 || tx.Deadline != "2027-05-01" {
		t.Errorf("unknown contract date must not assume a refund: %+v / %v", p, tx.Refund)
	}
	yes := true
	cfo.SaveTaxSettings(d, cfo.TaxSettings{Pillar3Before2025: &yes})
	tx, _ = cfo.Tax(d, 2026, now)
	if tx.Refund != E(240) { // 20% of 1,200
		t.Errorf("refund %v", tx.Refund)
	}
	// Booked salary wins over the setting: €60,500 net paid (≈ €100,000
	// gross, final pay included) plus November and December at €8,400.
	d2 := DB(t)
	plan.SaveSettings(d2, st)
	Tx(t, d2, ledger.Tx{Date: "2026-04-01", Amount: E(60500), Category: "salary", AccountID: "swed"})
	t2, _ := cfo.Tax(d2, 2026, now)
	for _, it := range t2.Items {
		if it.Key == "bracket" && it.Amount != E(1678.15) { // (116,800 − 83,237) × 5%
			t.Errorf("from booked salary: %+v", it)
		}
	}

	f, err := cfo.ForecastFrom(d, now, 12, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range f.Months {
		for _, it := range m.Dated {
			if it.Kind == "tax" {
				found = m.Month == "2027-05" && it.Amount == E(893.15-240)
			}
		}
	}
	if !found {
		t.Error("the 2026 tax isn't in May 2027")
	}
}

// The 3rd-pillar refund is on what was paid in; a payout the same year
// doesn't cancel it.
func TestTaxPillarIgnoresPayout(t *testing.T) {
	d := DB(t)
	year := time.Now().Year() - 1
	day := func(m int) string { return time.Date(year, time.Month(m), 10, 0, 0, 0, 0, time.UTC).Format("2006-01-02") }
	Tx(t, d, ledger.Tx{Date: day(3), Kind: "transfer", Amount: E(1500), Category: "transfer.pension", AccountID: "swed", ToAccountID: "artea"})
	Tx(t, d, ledger.Tx{Date: day(12), Kind: "transfer", Amount: E(17197.92), Category: "transfer.pension", AccountID: "artea", ToAccountID: "swed"})
	r, err := cfo.Tax(d, year, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range r.Items {
		if it.Key == "pillar3" {
			if it.Amount <= 0 {
				t.Fatalf("pillar3 refund %v", it.Amount)
			}
			return
		}
	}
	t.Fatalf("no 3rd-pillar item: %+v", r.Items)
}
