// Package boot opens the v2 database, converting a v1 database found next to
// it on the very first start. The v1 file is only ever read: reinstalling the
// v1 add-on is a complete rollback.
package boot

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"ft/internal/db"
	"ft/internal/importv1"
	"ft/internal/ledger"
)

// Open returns the database at path. On first start (no file yet) it
// converts legacy when that file exists, verifies the result against the
// source and refuses to start — deleting the half-made v2 file — if any
// check fails. Otherwise it seeds the default categories.
func Open(path, legacy string) (*sql.DB, *importv1.Verification, error) {
	_, statErr := os.Stat(path)
	fresh := os.IsNotExist(statErr)
	d, err := db.Open(path)
	if err != nil {
		return nil, nil, err
	}
	if !fresh {
		return d, nil, nil
	}
	fail := func(err error) (*sql.DB, *importv1.Verification, error) {
		d.Close()
		os.Remove(path)
		os.Remove(path + "-wal")
		os.Remove(path + "-shm")
		return nil, nil, err
	}
	if _, err := os.Stat(legacy); err != nil || legacy == path || legacy == "" {
		if err := ledger.SeedCategories(d); err != nil {
			return fail(err)
		}
		return d, nil, nil
	}
	log.Printf("first start: converting v1 database %s", legacy)
	src, err := importv1.ReadDB(legacy)
	if err != nil {
		return fail(fmt.Errorf("reading v1 database: %w", err))
	}
	rep, err := importv1.Convert(d, src)
	if err != nil {
		return fail(fmt.Errorf("converting v1 database: %w", err))
	}
	log.Printf("converted: %d transactions, %d balance points, %d accounts, %d rules, %d budgets, %d trades, %d inbox rows",
		rep.Transactions, rep.BalanceRows, rep.Accounts, rep.Rules, rep.Budgets, rep.Trades, rep.InboxRows)
	for _, w := range rep.Warnings {
		log.Printf("  note: %s", w)
	}
	ver := importv1.Verify(d, src)
	for _, c := range ver.Checks {
		log.Printf("  verify %v: %s — %s", c.OK, c.Name, c.Detail)
	}
	if !ver.OK {
		return fail(fmt.Errorf("conversion verification failed — v1 data left untouched, v2 database removed"))
	}
	return d, ver, nil
}
