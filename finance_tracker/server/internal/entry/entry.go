// Package entry writes a transaction together with everything the write
// moves — the account balances it shifts and the bank inbox row behind it —
// in one database transaction. Every create, edit and delete of a
// transaction (the form, bulk edits, Ask CFO) goes through here, so a
// balance can't drift because one path forgot a step: bulk-deleting cash
// expenses used to leave the cash balance short by their total.
package entry

import (
	"database/sql"

	"ft/internal/bank"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/wealth"
)

// Do runs f in one database transaction: all of it lands or none of it.
func Do(d *sql.DB, f func(tx *sql.Tx) error) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := f(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// Create validates and inserts t, then moves the balances it implies.
// Returns the accounts whose balance moved.
func Create(tx *sql.Tx, t *ledger.Tx) ([]string, error) {
	if err := ledger.Validate(tx, t); err != nil {
		return nil, err
	}
	if err := ledger.Insert(tx, t); err != nil {
		return nil, err
	}
	return wealth.MoveForTx(tx, *t, 1)
}

// Update rewrites cur with next's editable fields. Server-owned fields (id,
// bank id, split parent, source) are kept from cur, never taken from next.
// When the money side changes — date, amount, accounts or kind (a category
// moved from expense to income flips the direction) — cur's balance effect
// is reversed and next's applied.
func Update(tx *sql.Tx, cur ledger.Tx, next *ledger.Tx) error {
	next.ID, next.ExternalID, next.SplitOf, next.Source, next.Pending = cur.ID, cur.ExternalID, cur.SplitOf, cur.Source, cur.Pending
	if err := ledger.Validate(tx, next); err != nil {
		return err
	}
	if err := ledger.Update(tx, next); err != nil {
		return err
	}
	if cur.Date == next.Date && cur.Amount == next.Amount && cur.AccountID == next.AccountID && cur.ToAccountID == next.ToAccountID && cur.Kind == next.Kind {
		return nil
	}
	// A part counts on its own here, as when it is deleted: the parent holds
	// only what the parts left, so a part's change is real money.
	was, now := cur, *next
	if cur.SplitOf != 0 {
		was.SplitOf, now.SplitOf = 0, 0
	}
	return wealth.MoveForEdit(tx, was, now)
}

// ErrNotFound is returned when the transaction to change doesn't exist. It
// unwraps to sql.ErrNoRows, so the API answers 404 with this message.
var ErrNotFound error = notFound{}

type notFound struct{}

func (notFound) Error() string { return "transaction not found" }
func (notFound) Unwrap() error { return sql.ErrNoRows }

// Delete removes a transaction and reverses its balance effect. A bank row
// goes back to the inbox rather than vanishing (a reservation is dismissed,
// so sync doesn't re-add it). A split's parts go with their parent, so the
// whole original amount comes back — the parent alone holds only what the
// parts left; a part deleted on its own gives back its share.
func Delete(tx *sql.Tx, id int64) error {
	cur, err := ledger.Get(tx, id)
	if err != nil {
		return ErrNotFound
	}
	if cur.SplitOf == 0 {
		var parts int64
		if err := tx.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM transactions WHERE split_of=?`, id).Scan(&parts); err != nil {
			return err
		}
		cur.Amount += money.Cents(parts)
	} else {
		cur.SplitOf = 0
	}
	if err := bank.ReturnToInbox(tx, id); err != nil {
		return err
	}
	if err := ledger.Delete(tx, id); err != nil {
		return err
	}
	_, err = wealth.MoveForTx(tx, cur, -1)
	return err
}
