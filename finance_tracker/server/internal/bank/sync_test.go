package bank_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ft/internal/bank"
	"ft/internal/bank/openbanking"
	"ft/internal/db"
	"ft/internal/ledger"
	. "ft/internal/testutil"
	"ft/internal/wealth"
)

// fakeBank serves the Enable Banking endpoints a sync uses.
type fakeBank struct {
	mu       sync.Mutex
	booked   map[string][]openbanking.Transaction // uid → rows
	pending  map[string][]openbanking.Transaction
	balances map[string]string
	expired  bool
}

func (f *fakeBank) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		w.WriteHeader(401)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	parts := strings.Split(r.URL.Path, "/")
	switch {
	case r.URL.Path == "/aspsps":
		json.NewEncoder(w).Encode(map[string]any{"aspsps": []map[string]any{{"name": "Swedbank", "country": "LT"}}})
	case len(parts) == 4 && parts[1] == "accounts" && parts[3] == "transactions":
		if f.expired {
			w.WriteHeader(401)
			w.Write([]byte(`{"error":"EXPIRED_SESSION","message":"expired"}`))
			return
		}
		rows := f.booked[parts[2]]
		if r.URL.Query().Get("transaction_status") == "PDNG" {
			rows = f.pending[parts[2]]
		}
		json.NewEncoder(w).Encode(map[string]any{"transactions": rows})
	case len(parts) == 4 && parts[1] == "accounts" && parts[3] == "balances":
		json.NewEncoder(w).Encode(map[string]any{"balances": []map[string]any{{"balance_type": "ITBD", "balance_amount": map[string]string{"currency": "EUR", "amount": f.balances[parts[2]]}}}})
	default:
		w.WriteHeader(404)
	}
}

func card(ref, status, date, amount, merchant string) openbanking.Transaction {
	d, _ := time.Parse("2006-01-02", date)
	return openbanking.Transaction{EntryReference: ref, Status: status, CreditDebitIndicator: "DBIT", BookingDate: date,
		TransactionAmount:     openbanking.Amount{Currency: "EUR", Amount: amount},
		RemittanceInformation: []string{"PIRKINYS 516793******2669 " + d.Format("02.01.06") + " 12:00 " + amount + " EUR (1) " + merchant + "/X-1 " + merchant + " Vilnius"}}
}

func setup(t *testing.T) (*sql.DB, *bank.Service, *fakeBank) {
	d := DB(t)
	fb := &fakeBank{booked: map[string][]openbanking.Transaction{}, pending: map[string][]openbanking.Transaction{}, balances: map[string]string{"u1": "1000.00", "u2": "250.50"}}
	srv := httptest.NewServer(http.HandlerFunc(fb.handler))
	t.Cleanup(srv.Close)
	old := openbanking.BaseURL
	openbanking.BaseURL = srv.URL
	t.Cleanup(func() { openbanking.BaseURL = old })

	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	bank.SaveSettings(d, bank.Settings{ApplicationID: "app", PrivateKeyPEM: string(pemKey), RedirectURL: "https://x/", OwnerNames: []string{"MINDAUGAS BALCIUNAS"}})
	now := db.Now()
	d.Exec(`INSERT INTO bank_connections(id,aspsp_name,session_id,status,valid_until,created_at,updated_at) VALUES(1,'Swedbank','s1','authorized',?,?,?)`,
		time.Now().Add(90*24*time.Hour).UTC().Format(time.RFC3339), now, now)
	d.Exec(`INSERT INTO bank_accounts(id,connection_id,identification_hash,uid,iban,display_name,account_id,created_at,updated_at) VALUES(1,1,'h1','u1','LT1','Main','swed',?,?)`, now, now)
	d.Exec(`INSERT INTO bank_accounts(id,connection_id,identification_hash,uid,iban,display_name,account_id,created_at,updated_at) VALUES(2,1,'h2','u2','LT2','Second','swed',?,?)`, now, now)
	return d, &bank.Service{DB: d}, fb
}

func today(off int) string { return time.Now().AddDate(0, 0, off).Format("2006-01-02") }

func inbox(t *testing.T, s *bank.Service, state string) []bank.InboxRow {
	rows, err := s.Inbox(state)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestSyncStagesIdempotentlyAndAppliesBalances(t *testing.T) {
	d, s, fb := setup(t)
	fb.booked["u1"] = []openbanking.Transaction{card("a", "BOOK", today(-2), "23.40", "LIDL"), card("b", "BOOK", today(-1), "9.99", "WOLT")}
	res, err := s.SyncAll(context.Background(), openbanking.PSU{}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.New != 2 || res.Failed != 0 || len(inbox(t, s, "open")) != 2 {
		t.Fatalf("%+v", res)
	}
	// Both bank accounts feed "swed": the balance sheet gets their sum, today.
	book, _ := wealth.LoadBook(d)
	if p, ok := book.At("swed", today(0)); !ok || p.Value != E(1250.50) || p.Source != "bank" {
		t.Fatalf("bank balance %+v", p)
	}
	// Re-sync: nothing new.
	res, _ = s.SyncAll(context.Background(), openbanking.PSU{}, 0, 0)
	if res.New != 0 || len(inbox(t, s, "open")) != 2 {
		t.Fatalf("resync %+v", res)
	}
}

func TestDismissedStaysDismissedAndCommitMarksDuplicate(t *testing.T) {
	d, s, fb := setup(t)
	fb.booked["u1"] = []openbanking.Transaction{card("a", "BOOK", today(-2), "23.40", "LIDL"), card("b", "BOOK", today(-1), "9.99", "WOLT")}
	s.SyncAll(context.Background(), openbanking.PSU{}, 0, 0)
	rows := inbox(t, s, "open")
	var lidl, wolt bank.InboxRow
	for _, r := range rows {
		if r.Merchant == "Lidl" {
			lidl = r
		} else {
			wolt = r
		}
	}
	s.SetState([]int64{wolt.ID}, "dismissed")
	cat := "food.groceries"
	if _, err := s.Update(lidl.ID, bank.Edit{Category: &cat}); err != nil {
		t.Fatal(err)
	}
	res, err := s.Commit([]int64{lidl.ID})
	if err != nil || len(res.Imported) != 1 {
		t.Fatal(res, err)
	}
	tx, _ := ledger.Get(d, res.Imported[0])
	if tx.Category != "food.groceries" || tx.ExternalID != "eb:1:a" || tx.Source != "bank" || tx.AccountID != "swed" {
		t.Fatalf("%+v", tx)
	}
	// Committing again is a no-op with a note.
	again, _ := s.Commit([]int64{lidl.ID})
	if len(again.Imported) != 0 || again.Skipped != 1 {
		t.Fatal(again)
	}
	s.SyncAll(context.Background(), openbanking.PSU{}, 0, 0)
	if len(inbox(t, s, "open")) != 0 {
		t.Fatal("dismissed or imported rows came back")
	}
	if len(inbox(t, s, "dismissed")) != 1 {
		t.Fatal("dismissal lost")
	}
	// Restore brings it back.
	s.SetState([]int64{wolt.ID}, "open")
	if len(inbox(t, s, "open")) != 1 {
		t.Fatal("restore")
	}
}

func TestReservationSupersededOrReleased(t *testing.T) {
	d, s, fb := setup(t)
	st := bank.LoadSettings(d)
	st.KeepReservedInInbox = true // the review-first path
	bank.SaveSettings(d, st)
	fb.pending["u1"] = []openbanking.Transaction{card("hold-1", "PDNG", today(-3), "60.00", "NESTE"), card("hold-2", "PDNG", today(-3), "15.00", "CAFE")}
	res, _ := s.SyncAll(context.Background(), openbanking.PSU{}, 7, 0)
	if res.Pending != 2 {
		t.Fatalf("%+v", res)
	}
	// Kept in the inbox: nothing reached the ledger on its own.
	rows := inbox(t, s, "open")
	if res.ReservedAdded != 0 || len(rows) != 2 {
		t.Fatalf("review-first: %+v", res)
	}
	// The fuel hold books for a different amount under a new reference; the
	// café hold disappears.
	var holdID int64
	for _, r := range rows {
		if r.RawPayee == "NESTE" {
			holdID = r.ID
		}
	}
	tags := []string{"car"}
	s.Update(holdID, bank.Edit{Tags: &tags})
	fb.pending["u1"] = nil
	fb.booked["u1"] = []openbanking.Transaction{card("book-1", "BOOK", today(-1), "58.20", "NESTE")}
	res, _ = s.SyncAll(context.Background(), openbanking.PSU{}, 7, 0)
	if res.Superseded != 1 || res.Released != 1 || res.New != 1 {
		t.Fatalf("%+v", res)
	}
	open := inbox(t, s, "open")
	if len(open) != 1 || open[0].Amount != E(58.20) || len(open[0].Tags) == 0 || open[0].Tags[0] != "car" {
		t.Fatalf("booking inherits the edits made on its hold: %+v", open)
	}
}

func TestLinksToHandEnteredTransaction(t *testing.T) {
	d, s, fb := setup(t)
	mine := Tx(t, d, ledger.Tx{Date: today(-2), Amount: E(23.40), Category: "food.groceries", Merchant: "Lidl", Note: "weekly shop", AccountID: "swed"})
	fb.booked["u1"] = []openbanking.Transaction{card("a", "BOOK", today(-2), "23.40", "LIDL")}
	res, _ := s.SyncAll(context.Background(), openbanking.PSU{}, 0, 0)
	if res.AutoLinked != 1 || len(inbox(t, s, "open")) != 0 {
		t.Fatalf("certain match must auto-link: %+v", res)
	}
	got, _ := ledger.Get(d, mine.ID)
	if got.ExternalID != "eb:1:a" || got.Note != "weekly shop" {
		t.Fatalf("the hand-entered row keeps its fields and gains the bank id: %+v", got)
	}
	// Unlink returns the row to the inbox.
	imp := inbox(t, s, "imported")
	if err := s.Unlink(imp[0].ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := ledger.Get(d, mine.ID); got.ExternalID != "" {
		t.Error("unlink kept the bank id")
	}
	if open := inbox(t, s, "open"); len(open) != 1 || open[0].Match == nil {
		t.Fatalf("back in the inbox with the match suggested: %+v", open)
	}
}

func TestAmbiguousMatchIsOnlySuggested(t *testing.T) {
	d, s, fb := setup(t)
	Tx(t, d, ledger.Tx{Date: today(-2), Amount: E(10), Category: "food", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: today(-3), Amount: E(10), Category: "food", AccountID: "swed"})
	fb.booked["u1"] = []openbanking.Transaction{card("a", "BOOK", today(-2), "10.00", "KIOSK")}
	res, _ := s.SyncAll(context.Background(), openbanking.PSU{}, 0, 0)
	if res.AutoLinked != 0 {
		t.Fatal("two candidates: never auto-link")
	}
	if open := inbox(t, s, "open"); open[0].Match == nil || open[0].Preticked() {
		t.Fatal("suggested, not preticked")
	}
}

func TestRulesAndHistoryRefineProposals(t *testing.T) {
	d, s, fb := setup(t)
	Tx(t, d, ledger.Tx{Date: today(-40), Amount: E(5), Category: "food.coffee", Merchant: "Caffeine", AccountID: "swed"})
	r := ledger.Rule{Pattern: "caffeine", AddTags: []string{"work"}, Enabled: true}
	ledger.SaveRule(d, &r)
	fb.booked["u1"] = []openbanking.Transaction{card("a", "BOOK", today(-1), "4.50", "CAFFEINE")}
	s.SyncAll(context.Background(), openbanking.PSU{}, 0, 0)
	open := inbox(t, s, "open")
	if open[0].Category != "food.coffee" || open[0].Guessed || len(open[0].Tags) != 1 {
		t.Fatalf("history category + rule tags: %+v", open[0])
	}
}

func TestExpiredConsentReported(t *testing.T) {
	d, s, fb := setup(t)
	fb.expired = true
	res, err := s.SyncAll(context.Background(), openbanking.PSU{}, 0, 0)
	// The first account learns the consent is dead; the second is skipped
	// with a reason rather than failing again.
	if err != nil || res.Failed != 1 || res.Accounts[1].Skipped == "" {
		t.Fatalf("%+v %v", res, err)
	}
	var status string
	d.QueryRow(`SELECT status FROM bank_connections WHERE id=1`).Scan(&status)
	if status != "expired" {
		t.Fatal(status)
	}
	res, _ = s.SyncAll(context.Background(), openbanking.PSU{}, 0, 0)
	if res.Accounts[0].Skipped == "" {
		t.Fatal("expired connection skipped with a reason")
	}
}

func TestNotConfigured(t *testing.T) {
	d := DB(t)
	s := &bank.Service{DB: d}
	if _, err := s.SyncAll(context.Background(), openbanking.PSU{}, 0, 0); err != bank.ErrNotConfigured {
		t.Fatal(err)
	}
	if bank.LoadSettings(d).ConsentDays != 180 {
		t.Error("defaults")
	}
}

// Default: reservations go straight into the ledger as pending, the booking
// settles the same transaction (owner's edits kept), a release removes it.
func TestReservationsAddedSettledReleased(t *testing.T) {
	d, s, fb := setup(t)
	fb.pending["u1"] = []openbanking.Transaction{card("hold-1", "PDNG", today(-3), "60.00", "NESTE"), card("hold-2", "PDNG", today(-3), "15.00", "CAFE")}
	res, err := s.SyncAll(context.Background(), openbanking.PSU{}, 7, 0)
	if err != nil {
		t.Fatal(err)
	}
	pending, _ := ledger.All(d, ledger.Filter{})
	if res.ReservedAdded != 2 || len(pending) != 2 || !pending[0].Pending || !pending[1].Pending {
		t.Fatalf("reservations should be in the ledger at once: %+v / %+v", res, pending)
	}
	var fuel ledger.Tx
	for _, x := range pending {
		if x.Amount == E(60) {
			fuel = x
		}
	}
	// The owner files the fuel hold while it is pending.
	fuel.Category, fuel.Tags = "transport.fuel", []string{"car"}
	if err := ledger.Update(d, &fuel); err != nil {
		t.Fatal(err)
	}
	// It books for less under a new reference; the café hold is released.
	fb.pending["u1"] = nil
	fb.booked["u1"] = []openbanking.Transaction{card("book-1", "BOOK", today(-1), "58.20", "NESTE")}
	res, err = s.SyncAll(context.Background(), openbanking.PSU{}, 7, 0)
	if err != nil {
		t.Fatal(err)
	}
	all, _ := ledger.All(d, ledger.Filter{})
	if res.Settled != 1 || res.Released != 1 || len(all) != 1 {
		t.Fatalf("settle + release: %+v / %+v", res, all)
	}
	got := all[0]
	if got.ID != fuel.ID || got.Pending || got.Amount != E(58.20) || got.Date != today(-1) || got.Category != "transport.fuel" || len(got.Tags) != 1 {
		t.Fatalf("booking settles the same transaction, keeping edits: %+v", got)
	}
	if n := len(inbox(t, s, "open")); n != 0 {
		t.Fatalf("nothing left to review, got %d", n)
	}
	// A later sync changes nothing.
	res, _ = s.SyncAll(context.Background(), openbanking.PSU{}, 7, 0)
	if again, _ := ledger.All(d, ledger.Filter{}); len(again) != 1 || res.New != 0 {
		t.Fatalf("idempotent: %+v %+v", res, again)
	}
}
