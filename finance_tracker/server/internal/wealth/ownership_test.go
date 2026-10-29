package wealth_test

import (
	"testing"
	"time"

	"ft/internal/ledger"
	. "ft/internal/testutil"
	"ft/internal/wealth"
)

// A car worth €15,000 a year ago and €12,000 now, with €2,400 of fuel and
// insurance tagged "car": €3,000 lost + €2,400 + 5% on €12,000 = €6,000 a
// year; at 10,000 km that's €0.60/km against €0.35 by taxi and rentals.
func TestOwnershipCountsLostValueAndTheMoneyTiedUp(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	if _, err := ledger.SaveAccount(d, ledger.Account{ID: "golf", Name: "Golf", Kind: "vehicle"}); err != nil {
		t.Fatal(err)
	}
	Bal(t, d, "golf", "2025-10-01", 15000)
	Bal(t, d, "golf", "2026-10-01", 12000)
	for m := 1; m <= 12; m++ {
		Tx(t, d, ledger.Tx{Date: now.AddDate(0, -m, 1).Format("2006-01-02"), Amount: E(200), Category: "transport.fuel", AccountID: "swed", Tags: []string{"car"}})
	}
	if err := wealth.SaveOwnSettings(d, "golf", wealth.OwnSettings{KmYear: 10000, AltPerKm: 0.35}); err != nil {
		t.Fatal(err)
	}
	if err := wealth.SaveOwnSettings(d, "swed", wealth.OwnSettings{}); err == nil {
		t.Error("a bank account has no ownership cost")
	}
	book, _ := wealth.LoadBook(d)
	txs, _ := ledger.All(d, ledger.Filter{From: "2025-01-01"})
	all, err := wealth.Ownership(d, book, txs, 5, now)
	if err != nil {
		t.Fatal(err)
	}
	var car wealth.OwnCost
	for _, c := range all {
		if c.AccountID == "golf" {
			car = c
		}
	}
	if car.Settings.Tag != "car" || car.Year != E(6000) || car.PerKm != 0.6 || car.AltYear != E(3500) || car.Tone != "warn" {
		t.Fatalf("%+v", car)
	}
}

// The house's mortgage interest is its own line, not counted again from a
// tagged interest payment, and the loan comes off the money tied up.
func TestOwnershipHouseInterestOnce(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	Bal(t, d, "house", "2025-10-01", 300000)
	Bal(t, d, "house", "2026-10-01", 310000)
	Bal(t, d, "mortgage", "2026-10-01", -200000)
	Tx(t, d, ledger.Tx{Date: "2026-09-17", Amount: E(600), Category: "housing.mortgage_interest", AccountID: "seb", Tags: []string{"house"}})
	Tx(t, d, ledger.Tx{Date: "2026-09-20", Amount: E(1000), Category: "housing.maintenance", AccountID: "seb", Tags: []string{"house"}})
	wealth.SaveOwnSettings(d, "house", wealth.OwnSettings{Rent: 1200})
	book, _ := wealth.LoadBook(d)
	txs, _ := ledger.All(d, ledger.Filter{From: "2025-01-01"})
	all, _ := wealth.Ownership(d, book, txs, 0, now)
	h := all[0]
	if h.AccountID != "house" || h.Equity != E(110000) {
		t.Fatalf("%+v", h)
	}
	var running, value int64
	for _, l := range h.Lines {
		switch l.Key {
		case "running":
			running = int64(l.Amount)
		case "value":
			value = int64(l.Amount)
		}
	}
	if running != int64(E(1000)) || value != -int64(E(10000)) {
		t.Errorf("lines %+v", h.Lines)
	}
}
