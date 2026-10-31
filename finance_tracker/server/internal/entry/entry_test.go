package entry_test

import (
	"database/sql"
	"errors"
	"testing"

	"ft/internal/entry"
	"ft/internal/ledger"
	"ft/internal/money"
	. "ft/internal/testutil"
	"ft/internal/wealth"
)

func balance(t *testing.T, d *sql.DB, account string) money.Cents {
	t.Helper()
	book, err := wealth.LoadBook(d)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := book.At(account, "2099-12-31")
	return p.Value
}

func create(t *testing.T, d *sql.DB, tx ledger.Tx) ledger.Tx {
	t.Helper()
	if err := entry.Do(d, func(sq *sql.Tx) error { _, err := entry.Create(sq, &tx); return err }); err != nil {
		t.Fatal(err)
	}
	return tx
}

// A cash expense lowers the cash balance; deleting it gives the money back.
func TestCreateAndDeleteMoveTheBalance(t *testing.T) {
	d := DB(t)
	Bal(t, d, "cash", "2026-09-01", 100)
	tx := create(t, d, ledger.Tx{Date: "2026-09-05", Amount: E(30), Category: "food.groceries", AccountID: "cash"})
	if got := balance(t, d, "cash"); got != E(70) {
		t.Fatalf("after expense: %v", got)
	}
	if err := entry.Do(d, func(sq *sql.Tx) error { return entry.Delete(sq, tx.ID) }); err != nil {
		t.Fatal(err)
	}
	if got := balance(t, d, "cash"); got != E(100) {
		t.Fatalf("after delete: %v", got)
	}
	err := entry.Do(d, func(sq *sql.Tx) error { return entry.Delete(sq, tx.ID) })
	if !errors.Is(err, sql.ErrNoRows) || err.Error() != "transaction not found" {
		t.Fatalf("second delete: %v", err)
	}
}

// Re-filing an expense as income flips its direction on the balance, even
// though only the category changed.
func TestUpdateKindFlipMovesTheBalance(t *testing.T) {
	d := DB(t)
	Bal(t, d, "cash", "2026-09-01", 100)
	cur := create(t, d, ledger.Tx{Date: "2026-09-05", Amount: E(20), Category: "food.groceries", AccountID: "cash"})
	next := cur
	next.Category, next.Kind = "side_income", ""
	if err := entry.Do(d, func(sq *sql.Tx) error { return entry.Update(sq, cur, &next) }); err != nil {
		t.Fatal(err)
	}
	if got := balance(t, d, "cash"); got != E(120) {
		t.Fatalf("100 − 20 reversed + 20 income = 120, got %v", got)
	}
	// Server-owned fields come from the stored row, never the edit.
	evil := next
	evil.Source, evil.ExternalID, evil.SplitOf, evil.Pending = "import", "eb:spoof", 99, true
	if err := entry.Do(d, func(sq *sql.Tx) error { return entry.Update(sq, next, &evil) }); err != nil {
		t.Fatal(err)
	}
	if got, _ := ledger.Get(d, cur.ID); got.Source != "manual" || got.ExternalID != "" || got.SplitOf != 0 || got.Pending {
		t.Fatalf("server-owned fields changed: %+v", got)
	}
}

// When any step fails, nothing lands: the row and its balance move together.
func TestFailedWriteLeavesNothingBehind(t *testing.T) {
	d := DB(t)
	Bal(t, d, "cash", "2026-09-01", 100)
	err := entry.Do(d, func(sq *sql.Tx) error {
		if _, err := entry.Create(sq, &ledger.Tx{Date: "2026-09-05", Amount: E(30), Category: "food.groceries", AccountID: "cash"}); err != nil {
			return err
		}
		_, err := entry.Create(sq, &ledger.Tx{Date: "2026-09-06", Amount: E(5), Category: "no.such", AccountID: "cash"})
		return err
	})
	if err == nil {
		t.Fatal("unknown category accepted")
	}
	if txs, _ := ledger.All(d, ledger.Filter{}); len(txs) != 0 {
		t.Fatalf("rolled back rows remain: %d", len(txs))
	}
	if got := balance(t, d, "cash"); got != E(100) {
		t.Fatalf("balance moved by a rolled back write: %v", got)
	}
}

// A deleted bank row goes back to the inbox inside the same transaction.
func TestDeleteReturnsBankRowToInbox(t *testing.T) {
	d := DB(t)
	tx := Tx(t, d, ledger.Tx{Date: "2026-09-01", Amount: E(9), Category: "food", AccountID: "swed", ExternalID: "eb:1:x", Source: "bank"})
	d.Exec(`INSERT INTO bank_inbox(external_id,date,kind,amount,state,imported_tx_id,first_seen_at,last_seen_at) VALUES('eb:1:x','2026-09-01','expense',900,'imported',?,'x','x')`, tx.ID)
	if err := entry.Do(d, func(sq *sql.Tx) error { return entry.Delete(sq, tx.ID) }); err != nil {
		t.Fatal(err)
	}
	var state string
	d.QueryRow(`SELECT state FROM bank_inbox WHERE external_id='eb:1:x'`).Scan(&state)
	if state != "open" {
		t.Fatalf("inbox row state %q", state)
	}
}

func update(t *testing.T, d *sql.DB, id int64, edit func(*ledger.Tx)) {
	t.Helper()
	err := entry.Do(d, func(sq *sql.Tx) error {
		cur, err := ledger.Get(sq, id)
		if err != nil {
			return err
		}
		next := cur
		edit(&next)
		return entry.Update(sq, cur, &next)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Moving an expense's date keeps it in the balance while it stays after
// the last recorded value; moving it before that value folds it inside.
func TestDateChangesKeepTheBalance(t *testing.T) {
	d := DB(t)
	Bal(t, d, "cash", "2026-09-01", 100)
	tx := create(t, d, ledger.Tx{Date: "2026-09-10", Amount: E(10), Category: "food.groceries", AccountID: "cash"})
	for _, c := range []struct {
		date string
		want float64
	}{{"2026-09-05", 90}, {"2026-09-20", 90}, {"2026-09-12", 90}, {"2026-08-20", 100}, {"2026-09-15", 90}} {
		update(t, d, tx.ID, func(n *ledger.Tx) { n.Date = c.date })
		if got := balance(t, d, "cash"); got != E(c.want) {
			t.Fatalf("moved to %s: balance %v, want %v", c.date, got, c.want)
		}
	}
}

// Paying from another account gives the first one its money back.
func TestAccountChangeMovesBothBalances(t *testing.T) {
	d := DB(t)
	Bal(t, d, "cash", "2026-09-01", 100)
	Bal(t, d, "revolut", "2026-09-01", 500)
	tx := create(t, d, ledger.Tx{Date: "2026-09-10", Amount: E(10), Category: "food.groceries", AccountID: "cash"})
	update(t, d, tx.ID, func(n *ledger.Tx) { n.AccountID = "revolut" })
	if c, r := balance(t, d, "cash"), balance(t, d, "revolut"); c != E(100) || r != E(490) {
		t.Fatalf("cash %v revolut %v", c, r)
	}
}

// Deleting a split gives back the whole original amount; deleting one part
// gives back that share.
func TestDeletingSplitsGivesBackWhatWasPaid(t *testing.T) {
	d := DB(t)
	Bal(t, d, "cash", "2026-09-01", 100)
	tx := create(t, d, ledger.Tx{Date: "2026-09-10", Amount: E(30), Category: "food.groceries", AccountID: "cash"})
	parts, err := ledger.Split(d, tx.ID, []ledger.SplitPart{{Amount: E(10), Category: "food.restaurants"}, {Amount: E(5)}})
	if err != nil {
		t.Fatal(err)
	}
	if got := balance(t, d, "cash"); got != E(70) {
		t.Fatalf("a split moves nothing: %v", got)
	}
	update(t, d, parts[0].ID, func(n *ledger.Tx) { n.Amount = E(15) })
	if got := balance(t, d, "cash"); got != E(65) {
		t.Fatalf("the €10 part raised to €15: %v", got)
	}
	update(t, d, parts[0].ID, func(n *ledger.Tx) { n.Amount = E(10) })
	if err := entry.Do(d, func(sq *sql.Tx) error { return entry.Delete(sq, parts[1].ID) }); err != nil {
		t.Fatal(err)
	}
	if got := balance(t, d, "cash"); got != E(75) {
		t.Fatalf("after deleting the €5 part: %v", got)
	}
	if err := entry.Do(d, func(sq *sql.Tx) error { return entry.Delete(sq, tx.ID) }); err != nil {
		t.Fatal(err)
	}
	if got := balance(t, d, "cash"); got != E(100) {
		t.Fatalf("after deleting the parent with its part: %v", got)
	}
	if txs, _ := ledger.All(d, ledger.Filter{}); len(txs) != 0 {
		t.Fatalf("parts outlived the parent: %d", len(txs))
	}
}

// Editing a transfer into a valued account never touches that account's
// own recorded valuations — even an unchanged one the owner re-confirmed.
func TestEditKeepsValuationsOfUnmovedAccounts(t *testing.T) {
	d := DB(t)
	Bal(t, d, "cash", "2026-08-01", 500)
	Bal(t, d, "btc_m", "2026-08-01", 1000)
	Bal(t, d, "btc_m", "2026-09-10", 1000)
	tx := create(t, d, ledger.Tx{Date: "2026-09-10", Amount: E(100), Category: "transfer.invest", AccountID: "cash", ToAccountID: "btc_m"})
	update(t, d, tx.ID, func(n *ledger.Tx) { n.Amount = E(120) })
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM balances WHERE account_id='btc_m'`).Scan(&n)
	if n != 2 {
		t.Fatalf("crypto valuations: %d points, want 2", n)
	}
	if got := balance(t, d, "cash"); got != E(380) {
		t.Fatalf("cash %v", got)
	}
}
