package bank_test

import (
	"context"
	"database/sql"
	"testing"

	"ft/internal/bank"
	"ft/internal/bank/openbanking"
	"ft/internal/db"
	"ft/internal/entry"
	"ft/internal/ledger"
	. "ft/internal/testutil"
)

// withSEB links a second bank: bank account 3 feeds the ledger's "seb".
func withSEB(t *testing.T, d *sql.DB, fb *fakeBank) {
	t.Helper()
	now := db.Now()
	if _, err := d.Exec(`INSERT INTO bank_accounts(id,connection_id,identification_hash,uid,iban,display_name,account_id,created_at,updated_at) VALUES(3,1,'h3','u3','LT3','SEB','seb',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	fb.balances["u3"] = "500.00"
}

// sent is a transfer as the sending bank lists it; received is the same
// money as the receiving bank lists it.
func sent(ref, date, amount, toIBAN string) openbanking.Transaction {
	return openbanking.Transaction{EntryReference: ref, Status: "BOOK", CreditDebitIndicator: "DBIT", BookingDate: date,
		TransactionAmount: openbanking.Amount{Currency: "EUR", Amount: amount}, Creditor: openbanking.PartyName{Name: "MINDAUGAS BALCIUNAS"},
		CreditorAccount: openbanking.PartyAccount{IBAN: toIBAN}, RemittanceInformation: []string{"Paskolos aptarnavimas"}}
}

func received(ref, date, amount, fromIBAN string) openbanking.Transaction {
	return openbanking.Transaction{EntryReference: ref, Status: "BOOK", CreditDebitIndicator: "CRDT", BookingDate: date,
		TransactionAmount: openbanking.Amount{Currency: "EUR", Amount: amount}, Debtor: openbanking.PartyName{Name: "MINDAUGAS BALCIUNAS"},
		DebtorAccount: openbanking.PartyAccount{IBAN: fromIBAN}, RemittanceInformation: []string{"RO2512107976L02 Paskolos aptarnavimas"}}
}

func transfers(t *testing.T, d *sql.DB) []ledger.Tx {
	t.Helper()
	all, err := ledger.All(d, ledger.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	out := []ledger.Tx{}
	for _, x := range all {
		if x.Kind == "transfer" {
			out = append(out, x)
		}
	}
	return out
}

func syncNow(t *testing.T, s *bank.Service) *bank.SyncResult {
	t.Helper()
	res, err := s.SyncAll(context.Background(), openbanking.PSU{}, 0, 0)
	if err != nil || res.Failed != 0 {
		t.Fatalf("sync: %+v %v", res, err)
	}
	return res
}

// €1,400 sent from Swedbank to SEB is listed by both banks. Accepting one
// side must take the other with it: one transfer in the ledger, nothing left
// in the inbox, and a later sync doesn't bring the second side back.
func TestTransferReportedByBothBanksBecomesOneTransaction(t *testing.T) {
	d, s, fb := setup(t)
	withSEB(t, d, fb)
	fb.booked["u1"] = []openbanking.Transaction{sent("out-1", today(-1), "1400.00", "LT3")}
	fb.booked["u3"] = []openbanking.Transaction{received("in-1", today(0), "1400.00", "LT1")}
	syncNow(t, s)
	rows := inbox(t, s, "open")
	if len(rows) != 2 {
		t.Fatalf("both sides should wait in the inbox until one is accepted: %+v", rows)
	}
	res, err := s.Commit([]int64{rows[0].ID})
	if err != nil || len(res.Imported) != 1 || res.Merged != 1 {
		t.Fatalf("commit %+v %v", res, err)
	}
	got := transfers(t, d)
	if len(got) != 1 || got[0].AccountID != "swed" || got[0].ToAccountID != "seb" || got[0].Amount != E(1400) {
		t.Fatalf("want one swed → seb transfer, got %+v", got)
	}
	if left := inbox(t, s, "open"); len(left) != 0 {
		t.Fatalf("the other side stayed in the inbox: %+v", left)
	}
	if r := syncNow(t, s); r.New != 0 || len(inbox(t, s, "open")) != 0 || len(transfers(t, d)) != 1 {
		t.Fatalf("resync brought the second side back: %+v", r)
	}
	// Deleting the transfer hands both bank rows back for review.
	if err := entry.Do(d, func(tx *sql.Tx) error { return entry.Delete(tx, got[0].ID) }); err != nil {
		t.Fatal(err)
	}
	if back := inbox(t, s, "open"); len(back) != 2 {
		t.Fatalf("both sides should return to the inbox, got %+v", back)
	}
}

func TestAcceptingBothSidesTogetherAddsOneTransfer(t *testing.T) {
	d, s, fb := setup(t)
	withSEB(t, d, fb)
	fb.booked["u1"] = []openbanking.Transaction{sent("out-1", today(-1), "1400.00", "LT3")}
	fb.booked["u3"] = []openbanking.Transaction{received("in-1", today(-1), "1400.00", "LT1")}
	syncNow(t, s)
	rows := inbox(t, s, "open")
	res, err := s.Commit([]int64{rows[0].ID, rows[1].ID})
	if err != nil || len(res.Imported) != 1 || res.Merged != 1 || len(transfers(t, d)) != 1 {
		t.Fatalf("commit %+v %v, transfers %+v", res, err, transfers(t, d))
	}
}

// The duplicates made before this rule existed: both sides already sit in the
// ledger as two transfers. The next sync keeps the older one, removes the
// other, and files the removed side's bank row so it isn't offered again.
func TestSyncMergesATransferAlreadyInTheLedgerTwice(t *testing.T) {
	d, s, fb := setup(t)
	withSEB(t, d, fb)
	fb.booked["u1"] = []openbanking.Transaction{sent("out-1", today(-2), "1400.00", "LT3")}
	fb.booked["u3"] = []openbanking.Transaction{received("in-1", today(-2), "1400.00", "LT1")}
	err := entry.Do(d, func(tx *sql.Tx) error {
		for _, x := range []ledger.Tx{
			{Date: today(-2), Kind: "transfer", Amount: E(1400), AccountID: "swed", ToAccountID: "seb", Category: "transfer.internal", Note: "Paskolos aptarnavimas", ExternalID: "eb:1:out-1", Source: "bank"},
			{Date: today(-2), Kind: "transfer", Amount: E(1400), ToAccountID: "seb", Category: "transfer.internal", Tags: []string{"house"}, ExternalID: "eb:3:in-1", Source: "bank"},
		} {
			if _, err := entry.Create(tx, &x); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	res := syncNow(t, s)
	got := transfers(t, d)
	if res.Merged != 1 || len(got) != 1 {
		t.Fatalf("merged %d, transfers %+v", res.Merged, got)
	}
	if got[0].ExternalID != "eb:1:out-1" || got[0].ToAccountID != "seb" || len(got[0].Tags) != 1 || got[0].Tags[0] != "house" {
		t.Fatalf("the older transfer should stay and keep the other's tag: %+v", got[0])
	}
	if len(inbox(t, s, "open")) != 0 {
		t.Fatalf("the removed side came back to the inbox: %+v", inbox(t, s, "open"))
	}
	if r := syncNow(t, s); r.Merged != 0 || len(transfers(t, d)) != 1 || len(inbox(t, s, "open")) != 0 {
		t.Fatalf("second sync changed something: %+v", r)
	}
}

// Same amount, same day, is not enough. Two payments out of the same ledger
// account (here both Swedbank accounts feed "swed") are two payments, and two
// real transfers to SEB stay two — each takes exactly one mirror.
func TestSeparateTransfersOfTheSameAmountStaySeparate(t *testing.T) {
	d, s, fb := setup(t)
	withSEB(t, d, fb)
	err := entry.Do(d, func(tx *sql.Tx) error {
		for _, ext := range []string{"eb:1:a", "eb:2:b"} {
			x := ledger.Tx{Date: today(-1), Kind: "transfer", Amount: E(500), AccountID: "swed", ToAccountID: "ibkr", Category: "transfer.invest", ExternalID: ext, Source: "bank"}
			if _, err := entry.Create(tx, &x); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	fb.booked["u1"] = []openbanking.Transaction{sent("out-1", today(-1), "300.00", "LT3"), sent("out-2", today(-1), "300.00", "LT3")}
	fb.booked["u3"] = []openbanking.Transaction{received("in-1", today(-1), "300.00", "LT1"), received("in-2", today(-1), "300.00", "LT1")}
	if r := syncNow(t, s); r.Merged != 0 {
		t.Fatalf("merged unrelated payments: %+v", r)
	}
	var ids []int64
	for _, r := range inbox(t, s, "open") {
		ids = append(ids, r.ID)
	}
	res, err := s.Commit(ids)
	if err != nil || len(res.Imported) != 2 || res.Merged != 2 {
		t.Fatalf("commit %+v %v", res, err)
	}
	if got := transfers(t, d); len(got) != 4 {
		t.Fatalf("want 2 to IBKR + 2 to SEB, got %d: %+v", len(got), got)
	}
}

// What is not proof of one transfer must stay two rows for the owner: the
// sender naming a third account, neither side naming the other, and sides
// more than three days apart.
func TestLookalikeTransfersAreNotMerged(t *testing.T) {
	for name, c := range map[string]struct {
		out, in openbanking.Transaction
	}{
		"sent to a third account": {sent("o", today(-1), "500.00", "LT9"), received("i", today(-1), "500.00", "LT1")},
		"neither names the other": {sent("o", today(-1), "500.00", ""), received("i", today(-1), "500.00", "")},
		"four days apart":         {sent("o", today(-5), "500.00", "LT3"), received("i", today(-1), "500.00", "LT1")},
		"different amounts":       {sent("o", today(-1), "500.00", "LT3"), received("i", today(-1), "500.01", "LT1")},
	} {
		t.Run(name, func(t *testing.T) {
			d, s, fb := setup(t)
			withSEB(t, d, fb)
			if _, err := d.Exec(`UPDATE accounts SET details='{"iban":"LT9"}' WHERE id='revolut'`); err != nil {
				t.Fatal(err)
			}
			fb.booked["u1"] = []openbanking.Transaction{c.out}
			fb.booked["u3"] = []openbanking.Transaction{c.in}
			syncNow(t, s)
			var ids []int64
			for _, r := range inbox(t, s, "open") {
				ids = append(ids, r.ID)
			}
			res, err := s.Commit(ids)
			if err != nil || res.Merged != 0 || len(res.Imported) != 2 {
				t.Fatalf("commit %+v %v", res, err)
			}
			if r := syncNow(t, s); r.Merged != 0 || len(transfers(t, d)) != 2 {
				t.Fatalf("merged lookalikes: %+v %+v", r, transfers(t, d))
			}
		})
	}
}

// The receiving bank often can't say where the money came from. Folding the
// sender's row in completes the transfer; and when the owner says the two
// are not the same transfer after all, the row comes back, can be added on
// its own, and is never folded again. Synced balances move in none of this.
func TestMergeCompletesTheTransferAndCanBeUndone(t *testing.T) {
	d, s, fb := setup(t)
	withSEB(t, d, fb)
	fb.booked["u1"] = []openbanking.Transaction{sent("out-1", today(-1), "1400.00", "LT3")}
	fb.booked["u3"] = []openbanking.Transaction{received("in-1", today(-1), "1400.00", "")}
	syncNow(t, s)
	var in, out bank.InboxRow
	for _, r := range inbox(t, s, "open") {
		if r.BankAccountID == 3 {
			in = r
		} else {
			out = r
		}
	}
	if in.AccountID != "" || in.ToAccountID != "seb" {
		t.Fatalf("receiving side should not know the sender: %+v", in)
	}
	if res, err := s.Commit([]int64{in.ID}); err != nil || len(res.Imported) != 1 || res.Merged != 1 {
		t.Fatalf("commit %+v %v", res, err)
	}
	got := transfers(t, d)
	if len(got) != 1 || got[0].AccountID != "swed" || got[0].ToAccountID != "seb" {
		t.Fatalf("the sender's row should have filled in Swedbank: %+v", got)
	}
	var manual int
	d.QueryRow(`SELECT COUNT(*) FROM balances WHERE account_id IN ('swed','seb') AND source != 'bank'`).Scan(&manual)
	if manual != 0 {
		t.Fatalf("a synced balance was moved by the merge (%d rows)", manual)
	}
	if err := s.Unlink(out.ID); err != nil {
		t.Fatal(err)
	}
	if res, err := s.Commit([]int64{out.ID}); err != nil || len(res.Imported) != 1 || res.Merged != 0 {
		t.Fatalf("a row kept apart should be added on its own: %+v %v", res, err)
	}
	if r := syncNow(t, s); r.Merged != 0 || len(transfers(t, d)) != 2 {
		t.Fatalf("kept-apart transfers were merged again: %+v", r)
	}
}
