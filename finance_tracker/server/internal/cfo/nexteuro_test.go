package cfo_test

import (
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/ledger"
	. "ft/internal/testutil"
)

// A short buffer takes the first euros; then the 3rd pillar's 20% refund
// while it has room; investing (5% real + 2.5% − 2 = 5.5%) beats the 3.95%
// mortgage, so the mortgage only gets what's left — nothing here.
func TestNextEuroFillsBufferThenRefundThenBestReturn(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	for m := 1; m <= 12; m++ {
		day := now.AddDate(0, -m, 0).Format("2006-01") + "-20"
		Tx(t, d, ledger.Tx{Date: day, Amount: E(4000), Category: "salary", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: day, Amount: E(1000), Category: "utilities.electricity", AccountID: "swed"})
	}
	Tx(t, d, ledger.Tx{Kind: "transfer", Date: "2026-03-01", Amount: E(1300), Category: "transfer.pension", AccountID: "swed", ToAccountID: "artea"})
	Tx(t, d, ledger.Tx{Kind: "transfer", Date: "2026-04-01", Amount: E(100), Category: "transfer.invest", AccountID: "swed", ToAccountID: "ibkr"})
	Bal(t, d, "swed", "2026-10-14", 2500)
	Bal(t, d, "house", "2022-01-01", 300000)
	Bal(t, d, "mortgage", "2026-10-01", -200000)
	yes := true
	cfo.SaveTaxSettings(d, cfo.TaxSettings{Pillar3Before2025: &yes})

	n, err := cfo.NextEuroFor(d, E(1000), now)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	share := map[string]float64{}
	for _, o := range n.Options {
		keys = append(keys, o.Key)
		share[o.Key] = o.Amount.Float()
	}
	if len(keys) < 4 || keys[0] != "buffer" || keys[1] != "pillar3" || keys[2] != "invest" || keys[3] != "loan:mortgage" {
		t.Fatalf("order %v", keys)
	}
	// Essential spending €1,000 a month × 3 = €3,000; €2,500 in cash.
	if share["buffer"] != 500 || share["pillar3"] != 200 || share["invest"] != 300 || share["loan:mortgage"] != 0 {
		t.Errorf("split %v", share)
	}
	if n.Options[2].To != "ibkr" || n.Options[1].To != "artea" {
		t.Errorf("destinations %+v", n.Options)
	}
}
