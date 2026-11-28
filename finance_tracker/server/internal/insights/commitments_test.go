package insights_test

import (
	"testing"

	"ft/internal/insights"
	"ft/internal/ledger"
	. "ft/internal/testutil"
)

// Alimony every month filled Year review's "Biggest expenses" and topped
// "Top merchants". Fixed commitments — obligation categories and anything
// else paid to the people paid one — leave the lists and are totalled apart.
func TestFixedCommitmentsLeaveTheBiggestListsAndAreTotalledApart(t *testing.T) {
	alimony := func(d string) ledger.Tx {
		x := tx(d, "expense", "kids.alimony", 1000)
		x.Merchant = "Evelina"
		return x
	}
	kidsToEx := tx("2026-03-02", "expense", "kids.general", 400) // paid to her, not a shop
	kidsToEx.Merchant = "EVELINA"
	interest := tx("2026-03-15", "expense", "housing.mortgage_interest", 950)
	interest.Merchant = "Seb"
	dinner := tx("2026-09-26", "expense", "food.restaurants", 1113.5)
	tv := tx("2026-02-08", "expense", "shopping.electronics", 800)
	tv.Merchant = "bigbox"
	fee := tx("2026-04-01", "expense", "finance.bank_fees", 2)
	txs := []ledger.Tx{alimony("2026-01-15"), alimony("2026-02-15"), alimony("2026-03-15"), kidsToEx, interest, dinner, tv, fee}

	people := insights.CommitmentPeople(txs)
	choices := insights.Choices(txs, people)
	top := insights.TopExpenses(choices, "2026-01-01", "2026-12-31", 10)
	if len(top) != 2 || top[0].Category != "food.restaurants" || top[1].Merchant != "bigbox" {
		t.Errorf("biggest expenses: %+v", top)
	}
	if m := insights.MerchantTotals(choices, "2026-01-01", "2026-12-31", 10); len(m) != 1 || m[0].Name != "bigbox" {
		t.Errorf("top merchants: %+v", m)
	}

	got := insights.Commitments(txs, people, "2026-01-01", "2026-12-31")
	want := []insights.CommitmentTotal{
		{Category: "kids.alimony", Total: E(3000), Count: 3},
		{Category: "housing.mortgage_interest", Total: E(950), Count: 1},
		{Category: "kids.general", Total: E(400), Count: 1},
		{Category: "finance.bank_fees", Total: E(2), Count: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("commitments: %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("commitments[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	// The window holds: February is one alimony payment.
	if feb := insights.Commitments(txs, people, "2026-02-01", "2026-02-28"); len(feb) != 1 || feb[0].Total != E(1000) {
		t.Errorf("February: %+v", feb)
	}
}
