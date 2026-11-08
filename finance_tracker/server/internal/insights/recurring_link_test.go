package insights_test

import (
	"strings"
	"testing"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	. "ft/internal/testutil"
)

// A payment marked as a recurring item counts as that item's payment for
// its month whatever the bank calls the merchant ("TELIA LIETUVA AB" for
// the "Telia" item), and never as another item's — even one it would match
// by name.
func TestMarkedPaymentCountsForItsItemOnly(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	phone := insights.RecurringItem{Merchant: "Telia", Note: "phone", Category: "subscriptions", Cadence: "monthly", Amount: E(13.97), Kind: "bill", FromAccount: "swed", Day: 10}
	net := insights.RecurringItem{Merchant: "Telia", Note: "internet", Category: "subscriptions", Cadence: "monthly", Amount: E(14.95), Kind: "bill", FromAccount: "swed", Day: 10}
	for _, it := range []*insights.RecurringItem{&phone, &net} {
		if err := insights.SaveRecurringItem(d, it); err != nil {
			t.Fatal(err)
		}
	}
	octPhone := Tx(t, d, ledger.Tx{Date: "2026-10-10", Amount: E(13.97), Category: "subscriptions", AccountID: "swed", Merchant: "Telia", Note: "phone"})
	// Filed under another category, so only the mark can tie it to an item.
	novNet := Tx(t, d, ledger.Tx{Date: "2026-11-10", Amount: E(13.97), Category: "utilities.electricity", AccountID: "swed", Merchant: "TELIA LIETUVA AB"})
	_ = octPhone
	state := func() map[int64][2]any {
		txs, _ := ledger.All(d, ledger.Filter{})
		list, _, err := insights.RecurringAll(d, txs, now)
		if err != nil {
			t.Fatal(err)
		}
		out := map[int64][2]any{}
		for _, r := range list {
			out[r.ID] = [2]any{r.Done, r.Last}
		}
		return out
	}
	// Unmarked, November's row matches neither by name: phone not paid this month.
	if s := state(); s[phone.ID][0] != false || s[phone.ID][1] != "2026-10-10" {
		t.Fatalf("before marking: %v", s)
	}
	if err := ledger.SetRecurring(d, novNet.ID, phone.ID); err != nil {
		t.Fatal(err)
	}
	if s := state(); s[phone.ID][0] != true || s[phone.ID][1] != "2026-11-10" || s[net.ID][0] != false {
		t.Errorf("marked as the phone: %v", s)
	}
	// Marked as the phone, a row named "Telia · internet" still isn't the internet's.
	tx, _ := ledger.Get(d, novNet.ID)
	tx.Merchant, tx.Note = "Telia", "internet"
	items, _ := insights.ListRecurringItems(d)
	if id, by := insights.MatchingItem(&tx, items); id != phone.ID || by != "marked" {
		t.Errorf("matching item %d by %q", id, by)
	}
	// Deleting the item makes it a plain transaction again.
	if err := insights.DeleteRecurringItem(d, phone.ID); err != nil {
		t.Fatal(err)
	}
	if tx, _ = ledger.Get(d, novNet.ID); tx.RecurringID != 0 {
		t.Errorf("link kept after delete: %d", tx.RecurringID)
	}
}

// Paid this month, an item without a fixed day comes next a month on — not
// again later this month (Argus paid on 5 Nov, next date 10 Nov).
func TestPaidItemMovesToNextPeriod(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	argus := insights.RecurringItem{Merchant: "Argus", Category: "utilities.electricity", Cadence: "monthly", Amount: E(17.88), Kind: "bill", FromAccount: "swed", NextDate: "2026-11-10"}
	if err := insights.SaveRecurringItem(d, &argus); err != nil {
		t.Fatal(err)
	}
	Tx(t, d, ledger.Tx{Date: "2026-11-05", Amount: E(89.40), Category: "utilities.electricity", AccountID: "swed", Merchant: "Argus"})
	txs, _ := ledger.All(d, ledger.Filter{})
	list, _, _ := insights.RecurringAll(d, txs, now)
	for _, r := range list {
		if r.ID == argus.ID && (!r.Done || r.Next != "2026-12-10") {
			t.Errorf("argus done %v next %s", r.Done, r.Next)
		}
	}
}

// Linking is the domain's rule: "new" on a merchant whose item was once
// dismissed brings that item back rather than hiding the payment in it, an
// income can't be marked as paying a bill, a transfer with no name is named
// after where it goes, and a marked payment no longer shows up as a second
// recurring line under the bank's name.
func TestLinkingRules(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	barber := insights.RecurringItem{Merchant: "Barbara beauty", Category: "health", Cadence: "monthly", Amount: E(25), Kind: "bill", Hidden: true}
	if err := insights.SaveRecurringItem(d, &barber); err != nil {
		t.Fatal(err)
	}
	cut := Tx(t, d, ledger.Tx{Date: "2026-11-14", Amount: E(27), Category: "health", AccountID: "swed", Merchant: "Barbara beauty"})
	it, err := insights.LinkTx(d, &cut, true, 0, nil)
	if err != nil || it.ID != barber.ID || it.Hidden {
		t.Fatalf("reuse the dismissed item: %+v %v", it, err)
	}
	pay := Tx(t, d, ledger.Tx{Date: "2026-11-02", Kind: "income", Amount: E(25), Category: "salary", AccountID: "swed", Merchant: "Barbara beauty"})
	if _, err := insights.LinkTx(d, &pay, false, barber.ID, nil); err == nil {
		t.Error("an income marked as paying a bill")
	}
	move := Tx(t, d, ledger.Tx{Date: "2026-11-10", Kind: "transfer", Amount: E(200), Category: "transfer.pension", AccountID: "swed", ToAccountID: "artea"})
	if it, err := insights.LinkTx(d, &move, true, 0, map[string]string{"artea": "Artea"}); err != nil || it.Merchant != "To Artea" || it.Kind != "transfer" {
		t.Errorf("unnamed transfer: %+v %v", it, err)
	}
	// Three months of "TELIA LIETUVA AB", all marked as Telia: one Telia line, not two.
	telia := insights.RecurringItem{Merchant: "Telia", Category: "subscriptions", Cadence: "monthly", Amount: E(13.97), Kind: "bill", FromAccount: "swed"}
	insights.SaveRecurringItem(d, &telia)
	for _, m := range []string{"2026-08", "2026-09", "2026-10", "2026-11"} {
		row := Tx(t, d, ledger.Tx{Date: m + "-10", Amount: E(13.97), Category: "subscriptions", AccountID: "swed", Merchant: "TELIA LIETUVA AB"})
		if err := ledger.SetRecurring(d, row.ID, telia.ID); err != nil {
			t.Fatal(err)
		}
	}
	txs, _ := ledger.All(d, ledger.Filter{})
	list, _, _ := insights.RecurringAll(d, txs, now)
	for _, r := range list {
		if strings.EqualFold(r.Merchant, "TELIA LIETUVA AB") {
			t.Errorf("counted twice under the bank's name: %+v", r)
		}
		if r.ID == telia.ID && (!r.Done || r.Last != "2026-11-10") {
			t.Errorf("telia %+v", r)
		}
	}
}
