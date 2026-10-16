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
