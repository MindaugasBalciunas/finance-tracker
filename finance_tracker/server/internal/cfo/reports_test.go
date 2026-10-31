package cfo_test

import (
	"reflect"
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/ledger"
	"ft/internal/plan"
	. "ft/internal/testutil"
)

// One read for several months gives each month exactly the report it gets
// on its own — a fund's history, a late salary and this month's included.
func TestBudgetReportsMatchOneByOne(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	for m := 1; m <= 22; m++ {
		day := time.Date(2025, time.Month(m), 1, 0, 0, 0, 0, time.UTC)
		if day.After(now) {
			break
		}
		Tx(t, d, ledger.Tx{Date: day.AddDate(0, 0, 1).Format("2006-01-02"), Amount: E(3000), Category: "salary", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: day.AddDate(0, 0, 9).Format("2006-01-02"), Amount: E(float64(300 + 10*m)), Category: "food.groceries", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: day.AddDate(0, 0, 14).Format("2006-01-02"), Amount: E(500), Category: "transfer.invest", AccountID: "swed", ToAccountID: "ibkr"})
		if m%5 == 0 {
			Tx(t, d, ledger.Tx{Date: day.AddDate(0, 0, 20).Format("2006-01-02"), Amount: E(400), Category: "transport.car", AccountID: "swed"})
		}
	}
	for _, b := range []plan.Budget{
		{Name: "Food", Kind: "spending", Categories: []string{"food"}, Amount: E(450)},
		{Name: "Car", Kind: "spending", Categories: []string{"transport.car"}, Amount: E(100), Fund: true, StartMonth: "2025-02"},
		{Name: "ETF", Kind: "saving", Categories: []string{"transfer.invest"}, Amount: E(500)},
	} {
		if err := plan.Save(d, &b, ""); err != nil {
			t.Fatal(err)
		}
	}
	months := []string{"2026-04", "2026-05", "2026-06", "2026-07", "2026-08", "2026-09", "2026-10", "2025-12"}
	all, err := cfo.BudgetReports(d, months, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range months {
		one, err := cfo.BudgetReport(d, m, now)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(one, all[m]) {
			t.Errorf("%s differs:\none by one %+v\ntogether  %+v", m, one, all[m])
		}
	}
	if all["2026-09"].IncomeActual == 0 || all["2026-10"].Lines[1].FundState == nil || len(all["2026-10"].Lines) != 3 {
		t.Fatalf("fixture too thin to prove anything: %+v", all["2026-10"])
	}
}
