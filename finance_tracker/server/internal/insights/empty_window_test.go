package insights_test

import (
	"testing"

	"ft/internal/insights"
	"ft/internal/ledger"
)

// A window with nothing in it (Spending → "This week" on a Monday morning,
// a year before the first transaction) gives empty lists, never nil: they
// reach the web as JSON null, and the pages map over them.
func TestEmptyWindowListsAreNotNil(t *testing.T) {
	var txs []ledger.Tx
	from, to := "2026-11-09", "2026-11-15"
	if insights.Breakdown(txs, from, to) == nil {
		t.Error("Breakdown is nil")
	}
	if insights.MerchantTotals(txs, from, to, 25) == nil {
		t.Error("MerchantTotals is nil")
	}
	if insights.TopExpenses(txs, from, to, 15) == nil {
		t.Error("TopExpenses is nil")
	}
	if insights.TagTotals(txs, from, to) == nil {
		t.Error("TagTotals is nil")
	}
}
