package plan_test

import (
	"testing"
	"time"

	"ft/internal/ledger"
	"ft/internal/plan"
	. "ft/internal/testutil"
)

func cats(essential ...string) map[string]ledger.Category {
	m := map[string]ledger.Category{}
	for _, id := range []string{"food", "food.groceries", "food.restaurants", "kids", "kids.alimony", "leisure", "shopping", "travel", "gifts", "transfer.invest"} {
		m[id] = ledger.Category{ID: id}
	}
	for _, id := range essential {
		m[id] = ledger.Category{ID: id, Essential: true}
	}
	return m
}

// The free budget is the spending lines only: alimony (fixed) and the ETF
// (saving) are decided already, so they never show as money you can choose.
// Needs are lines whose every category is essential; a line that mixes in a
// non-essential one (leisure + shopping) is a want.
func TestFreeBudgetLeavesOutFixedAndSavingAndSplitsNeedsFromWants(t *testing.T) {
	now := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	budgets := []plan.Budget{
		line(1, "Alimony", "fixed", []string{"kids.alimony"}, 1000),
		line(2, "ETF", "saving", []string{"transfer.invest"}, 800),
		line(3, "Groceries", "spending", []string{"food.groceries"}, 500),
		line(4, "Kids", "spending", []string{"kids"}, 200),
		line(5, "Leisure & shopping", "spending", []string{"leisure", "shopping"}, 300),
		line(6, "Food out", "spending", []string{"food.restaurants"}, 100),
	}
	txs := []ledger.Tx{
		tx("2026-11-03", "kids.alimony", 1000),
		tx("2026-11-01", "transfer.invest", 800),
		tx("2026-11-05", "food.groceries", 220),
		tx("2026-11-06", "kids.general", 50),
		tx("2026-11-07", "shopping.clothing", 340), // €40 over
		tx("2026-11-08", "food.restaurants", 30),
	}
	r := plan.Compute(budgets, txs, "2026-11", now, plan.Settings{IncomeMode: "manual", ManualIncome: E(4000)})
	f := plan.Free(r, cats("food", "food.groceries", "kids", "kids.alimony"))

	if f.Budgeted != E(1100) || f.Spent != E(220+50+340+30) || f.Left != E(1100-640) {
		t.Fatalf("free budget %v spent %v left %v", f.Budgeted, f.Spent, f.Left)
	}
	if f.Needs.Budgeted != E(700) || f.Needs.Spent != E(270) || f.Needs.Left != E(430) {
		t.Fatalf("needs %+v", f.Needs)
	}
	if f.Wants.Budgeted != E(400) || f.Wants.Spent != E(370) || f.Wants.Left != E(30) {
		t.Fatalf("wants %+v", f.Wants)
	}
	if len(f.Wants.Lines) != 2 || f.Wants.Lines[0] != "Leisure & shopping" || f.Wants.Lines[1] != "Food out" {
		t.Fatalf("want lines %v", f.Wants.Lines)
	}
	if f.Needs.Budgeted+f.Wants.Budgeted != f.Budgeted || f.Needs.Spent+f.Wants.Spent != f.Spent {
		t.Fatal("needs and wants must add up to the free budget")
	}
}

// A fund counts what it holds this month (carry-over + this month's share)
// and a yearly line only its monthly share against this month's spend — a
// €1,200-a-year gifts line must not put €1,200 into November's budget.
func TestFreeBudgetCountsFundsAsHeldAndYearlyLinesAsAMonthlyShare(t *testing.T) {
	now := time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC)
	vac := line(1, "Travel", "spending", []string{"travel"}, 300)
	vac.Fund, vac.StartMonth = true, "2026-10"
	gifts := line(2, "Gifts", "spending", []string{"gifts"}, 1200)
	gifts.Period = "yearly"
	txs := []ledger.Tx{
		tx("2026-11-20", "travel.flights", 200), tx("2026-12-02", "travel.lodging", 500),
		tx("2026-03-01", "gifts", 300), tx("2026-12-10", "gifts", 40),
	}
	r := plan.Compute([]plan.Budget{vac, gifts}, txs, "2026-12", now, plan.Settings{})
	f := plan.Free(r, cats())
	// Travel: opens at 400 + 300 = 700 held, 500 paid, 200 left. Gifts: 100 share, 40 spent.
	if f.Budgeted != E(800) || f.Spent != E(540) || f.Left != E(260) {
		t.Fatalf("free %v / %v / %v", f.Budgeted, f.Spent, f.Left)
	}
	if f.Needs.Budgeted != 0 || len(f.Needs.Lines) != 0 || f.Wants.Budgeted != f.Budgeted {
		t.Fatalf("all wants: %+v", f)
	}
	// What is left splits into what the Travel fund still holds and what the
	// day-to-day (here the yearly gifts share) has left.
	if f.InFunds != E(200) || f.FundSpent != E(500) || f.Left-f.InFunds != E(60) {
		t.Fatalf("in funds %v, fund spent %v, day-to-day %v", f.InFunds, f.FundSpent, f.Left-f.InFunds)
	}
}

// A month with no spending lines gives zeros and empty lists, never null —
// Home maps over the names.
func TestFreeBudgetWithNoLinesIsEmptyNotNull(t *testing.T) {
	r := plan.Compute(nil, nil, "2026-11", time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC), plan.Settings{})
	f := plan.Free(r, nil)
	if f.Budgeted != 0 || f.Lines == nil || f.Needs.Lines == nil || f.Wants.Lines == nil {
		t.Fatalf("%+v", f)
	}
}

func TestEssentialNeedsEveryCategoryEssential(t *testing.T) {
	c := cats("food", "kids")
	for _, tc := range []struct {
		b    plan.Budget
		want bool
	}{
		{line(1, "Food", "spending", []string{"food"}, 1), true},
		{line(2, "Food & kids", "spending", []string{"food", "kids"}, 1), true},
		{line(3, "Food & fun", "spending", []string{"food", "leisure"}, 1), false},
		{line(4, "Out", "spending", []string{"food.restaurants"}, 1), false},
		{plan.Budget{Name: "Trip", Kind: "spending", Tag: "trip:rome"}, false},
		{line(5, "Unknown", "spending", []string{"nope"}, 1), false},
	} {
		if got := plan.Essential(tc.b, c); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.b.Name, got, tc.want)
		}
	}
}

// An overdrawn fund opens the month below zero; that overdraft already came
// out of last month's free money, so this month it holds just its share and
// never puts a negative budget into the needs/wants split.
func TestFreeBudgetDoesNotCarryAFundOverdraftIntoTheMonth(t *testing.T) {
	now := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	vac := line(1, "Travel", "spending", []string{"travel"}, 300)
	vac.Fund, vac.StartMonth = true, "2026-10"
	txs := []ledger.Tx{tx("2026-10-12", "travel.flights", 800), tx("2026-11-03", "travel.lodging", 100)}
	r := plan.Compute([]plan.Budget{vac}, txs, "2026-11", now, plan.Settings{})
	if fs := find(r, "Travel").FundState; fs.Opening != E(-500) {
		t.Fatalf("setup: fund should open at −500, got %v", fs.Opening)
	}
	f := plan.Free(r, cats())
	if f.Budgeted != E(300) || f.Spent != E(100) || f.Left != E(200) || f.Wants.Budgeted != E(300) || f.InFunds != E(200) {
		t.Fatalf("free %v / %v / %v, wants %+v", f.Budgeted, f.Spent, f.Left, f.Wants)
	}
}

// A fund that pays more this month than it holds keeps nothing, and the
// overdraft comes out of the day-to-day money — as Left to spend takes it out
// of free money — instead of making "kept in funds" negative.
func TestFreeBudgetFundOverspentThisMonthKeepsNothing(t *testing.T) {
	now := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	food := line(1, "Food", "spending", []string{"food"}, 400)
	trav := line(2, "Travel", "spending", []string{"travel"}, 300)
	trav.Fund, trav.StartMonth = true, "2026-11"
	gifts := line(3, "Gifts", "spending", []string{"gifts"}, 1000)
	gifts.Fund, gifts.StartMonth = true, "2026-11"
	r := plan.Compute([]plan.Budget{food, trav, gifts}, []ledger.Tx{tx("2026-11-04", "travel.flights", 500)}, "2026-11", now, plan.Settings{})
	f := plan.Free(r, cats("food"))
	if f.Left != E(1200) || f.InFunds != E(1000) || f.Left-f.InFunds != E(200) || f.FundSpent != E(300) {
		t.Fatalf("left %v, in funds %v, day-to-day %v, fund spent %v", f.Left, f.InFunds, f.Left-f.InFunds, f.FundSpent)
	}
}
