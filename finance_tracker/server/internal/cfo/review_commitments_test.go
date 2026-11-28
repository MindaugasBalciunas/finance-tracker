package cfo_test

import (
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/ledger"
	"ft/internal/money"
	. "ft/internal/testutil"
)

// Month review's "Biggest expenses" are choices: the alimony and anything
// else paid to the ex-partner (here a kids row, under her name in capitals)
// are totalled under the list instead. Each total's link — the Ledger's
// fixed-commitments filter — opens exactly the rows behind it, not every
// kids.general expense.
func TestMonthReviewBiggestExpensesAreChoicesAndCommitmentsLinkToTheirRows(t *testing.T) {
	d := DB(t)
	pay := func(date, cat, merchant string, eur float64) {
		Tx(t, d, ledger.Tx{Date: date, Kind: "expense", Amount: E(eur), AccountID: "swed", Category: cat, Merchant: merchant})
	}
	pay("2026-10-17", "kids.alimony", "Evelina", 1000)
	pay("2026-10-20", "kids.general", "EVELINA", 400) // paid to her: not a shop
	pay("2026-10-22", "kids.general", "Toy shop", 35) // a choice in the same category
	pay("2026-10-26", "food.restaurants", "Smelyne", 1113.5)

	r, err := cfo.BuildMonthReview(d, "2026-10", time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.TopExpenses) != 2 || r.TopExpenses[0].Merchant != "Smelyne" || r.TopExpenses[1].Merchant != "Toy shop" {
		t.Errorf("biggest expenses: %+v", r.TopExpenses)
	}
	got := map[string]money.Cents{}
	for _, c := range r.Commitments {
		got[c.Category] = c.Total
	}
	if len(got) != 2 || got["kids.alimony"] != E(1000) || got["kids.general"] != E(400) {
		t.Errorf("commitments: %+v", r.Commitments)
	}

	rows, err := ledger.All(d, ledger.Filter{From: "2026-10-01", To: "2026-10-31", Categories: []string{"kids.general"}, Commitments: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Amount != E(400) {
		t.Errorf("the €400 link opens %+v", rows)
	}
}
