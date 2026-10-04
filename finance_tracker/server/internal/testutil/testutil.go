// Package testutil builds small, realistic fixtures for tests.
package testutil

import (
	"database/sql"
	"encoding/json"
	"testing"

	"ft/internal/db"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/wealth"
)

// DB returns a fresh in-memory database with the category tree and a
// typical set of accounts.
func DB(t testing.TB) *sql.DB {
	t.Helper()
	d, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := ledger.SeedCategories(d); err != nil {
		t.Fatal(err)
	}
	for _, a := range []ledger.Account{
		{ID: "swed", Name: "Swedbank", Institution: "Swedbank", Kind: "checking", Liquid: true},
		{ID: "seb", Name: "SEB", Institution: "SEB", Kind: "checking", Liquid: true},
		{ID: "cash", Name: "Cash", Kind: "cash", Liquid: true},
		{ID: "revolut", Name: "Revolut", Kind: "checking", Liquid: true},
		{ID: "ibkr", Name: "IBKR", Kind: "brokerage", Liquid: true},
		{ID: "artea", Name: "Artea", Kind: "pension"},
		{ID: "btc_m", Name: "BTC", Kind: "crypto", Liquid: true},
		{ID: "house", Name: "House", Kind: "property", Details: json.RawMessage(`{"purchase_price":355000}`)},
		{ID: "mortgage", Name: "Mortgage", Kind: "loan", Details: json.RawMessage(`{"asset_id":"house","base_rate":2.65,"margin":1.3,"monthly_payment":1361.66,"end_date":"2052-07-17"}`)},
	} {
		if _, err := ledger.SaveAccount(d, a); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

// Tx inserts a validated transaction and returns it.
func Tx(t testing.TB, d *sql.DB, tx ledger.Tx) ledger.Tx {
	t.Helper()
	if tx.Source == "" {
		tx.Source = "manual"
	}
	if err := ledger.Validate(d, &tx); err != nil {
		t.Fatalf("validate %+v: %v", tx, err)
	}
	if err := ledger.Insert(d, &tx); err != nil {
		t.Fatal(err)
	}
	return tx
}

// Bal records a balance.
func Bal(t testing.TB, d *sql.DB, account, date string, eur float64) {
	t.Helper()
	if err := wealth.SetBalance(d, account, date, money.FromFloat(eur), nil, nil, "manual"); err != nil {
		t.Fatal(err)
	}
}

// E is shorthand for money from euros.
func E(eur float64) money.Cents { return money.FromFloat(eur) }
