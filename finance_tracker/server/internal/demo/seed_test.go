package demo_test

import (
	"path/filepath"
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/db"
	"ft/internal/demo"
	"ft/internal/ledger"
	"ft/internal/wealth"
)

func TestSeedBuildsAUsableDemo(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "demo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	if err := demo.Seed(d, now); err != nil {
		t.Fatal(err)
	}
	txs, _ := ledger.All(d, ledger.Filter{})
	if len(txs) < 600 {
		t.Fatalf("only %d transactions", len(txs))
	}
	for _, x := range txs {
		if x.Date > now.Format("2006-01-02") {
			t.Fatalf("future transaction %+v", x)
		}
	}
	book, _ := wealth.LoadBook(d)
	snap := book.SnapshotAt(now.Format("2006-01-02"), true)
	if snap.NetWorth <= 0 || snap.Liquid <= 0 || snap.Debt <= 0 {
		t.Fatalf("balance sheet %+v", snap)
	}
	o, err := cfo.BuildOverview(d, now, 0)
	if err != nil || o.Avg12.Income <= 0 || o.Plan.IncomeBase <= 0 {
		t.Fatalf("overview %v %+v", err, o)
	}
	loans := wealth.Loans(book, txs, now)
	if len(loans) != 1 || loans[0].Repaid <= 0 || loans[0].Equity <= 0 {
		t.Fatalf("loan %+v", loans)
	}
	trades, _ := wealth.ListTrades(d)
	if len(trades) < 20 {
		t.Fatalf("trades %d", len(trades))
	}
	// Deterministic: a second demo for the same day is identical.
	d2, _ := db.Open(filepath.Join(t.TempDir(), "demo2.db"))
	defer d2.Close()
	demo.Seed(d2, now)
	txs2, _ := ledger.All(d2, ledger.Filter{})
	if len(txs2) != len(txs) {
		t.Errorf("not deterministic: %d vs %d", len(txs), len(txs2))
	}
}
