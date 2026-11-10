package bank

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"ft/internal/bank/openbanking"
	"ft/internal/db"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/wealth"
)

// ── sync ────────────────────────────────────────────────────────────

type SyncResult struct {
	Accounts      []AccountSync `json:"accounts"`
	Fetched       int           `json:"fetched"`
	New           int           `json:"new"`
	Updated       int           `json:"updated"`
	Duplicates    int           `json:"duplicates"`
	Pending       int           `json:"pending"`
	Superseded    int           `json:"superseded"`
	Released      int           `json:"released"`
	AutoLinked    int           `json:"auto_linked"`
	ReservedAdded int           `json:"reserved_added"` // card reservations put straight into the ledger
	Settled       int           `json:"settled"`        // ledger reservations the bank has now booked
	Balances      []BalanceSet  `json:"balances"`
	Failed        int           `json:"failed"`
}

type AccountSync struct {
	BankAccountID int64  `json:"bank_account_id"`
	Name          string `json:"name"`
	Bank          string `json:"bank"`
	Fetched       int    `json:"fetched"`
	New           int    `json:"new"`
	From          string `json:"from"`
	Error         string `json:"error,omitempty"`
	Skipped       string `json:"skipped,omitempty"`
}

type BalanceSet struct {
	AccountID string      `json:"account_id"`
	Before    money.Cents `json:"before"`
	After     money.Cents `json:"after"`
	Skipped   string      `json:"skipped,omitempty"`
}

// SyncAll pulls every mapped account on every live connection. days > 0
// forces a window; otherwise first sync = 90 days, then a 7-day overlap.
func (s *Service) SyncAll(ctx context.Context, psu openbanking.PSU, days int, only int64) (*SyncResult, error) {
	cl, set, err := s.client()
	if err != nil {
		return nil, err
	}
	conns, err := listConnections(s.DB)
	if err != nil {
		return nil, err
	}
	res := &SyncResult{Accounts: []AccountSync{}}
	owned := s.ownIBANs()
	synced := map[string]bool{}
	for i := range conns {
		c := &conns[i]
		accts, err := listBankAccounts(s.DB, c.ID)
		if err != nil {
			return nil, err
		}
		var headers []string
		headersLoaded := false
		for j := range accts {
			a := &accts[j]
			if a.AccountID == "" || (only > 0 && a.ID != only) {
				continue
			}
			row := AccountSync{BankAccountID: a.ID, Name: firstNonEmpty(a.DisplayName, openbanking.MaskIBAN(a.IBAN)), Bank: c.ASPSPName}
			switch {
			case c.Status != "authorized":
				row.Skipped = "the connection to this bank has expired — reconnect it"
			case a.UID == "":
				row.Skipped = "no active session — reconnect the bank"
			}
			if row.Skipped != "" {
				res.Accounts = append(res.Accounts, row)
				continue
			}
			if !headersLoaded {
				headers = requiredHeaders(ctx, cl, c)
				headersLoaded = true
			}
			n, err := s.syncAccount(ctx, cl, c, a, psu, headers, days, set, owned, res, &row)
			if err != nil {
				row.Error = err.Error()
				res.Failed++
				var apiErr *openbanking.APIError
				if errors.As(err, &apiErr) && apiErr.Expired() {
					c.Status, c.LastError = "expired", apiErr.Code
					saveConnection(s.DB, c)
				}
			} else {
				synced[a.AccountID] = true
				row.New = n
			}
			res.Accounts = append(res.Accounts, row)
		}
	}
	res.Balances = s.applyBankBalances(synced)
	res.AutoLinked = s.autoLink()
	res.ReservedAdded = s.addReservations()
	return res, nil
}

func requiredHeaders(ctx context.Context, cl *openbanking.Client, c *Connection) []string {
	banks, err := cl.ASPSPs(ctx, c.Country)
	if err != nil {
		return nil
	}
	for _, b := range banks {
		if strings.EqualFold(b.Name, c.ASPSPName) {
			return b.RequiredPSUHeaders
		}
	}
	return nil
}

func (s *Service) ownIBANs() map[string]string {
	out := map[string]string{}
	// Accounts the bank doesn't link (a savings account entered by hand) can
	// carry their IBAN in the account details, so transfers to them are
	// recognised as yours and filed against them.
	if rows, err := s.DB.Query(`SELECT id, json_extract(details, '$.iban') FROM accounts WHERE json_valid(details) AND json_extract(details, '$.iban') IS NOT NULL`); err == nil {
		for rows.Next() {
			var id, iban string
			if rows.Scan(&id, &iban) == nil && iban != "" {
				out[strings.ToUpper(strings.ReplaceAll(iban, " ", ""))] = id
			}
		}
		rows.Close()
	}
	accts, _ := listBankAccounts(s.DB, 0)
	for _, a := range accts {
		if a.IBAN != "" && a.AccountID != "" {
			out[strings.ReplaceAll(a.IBAN, " ", "")] = a.AccountID
		}
	}
	return out
}

func (s *Service) syncAccount(ctx context.Context, cl *openbanking.Client, c *Connection, a *BankAccount, psu openbanking.PSU,
	headers []string, days int, set Settings, owned map[string]string, res *SyncResult, row *AccountSync) (int, error) {
	now := time.Now()
	from := now.AddDate(0, 0, -firstSyncDays)
	if days > 0 {
		from = now.AddDate(0, 0, -days)
	} else if a.LastTxDate != "" {
		if t, err := time.Parse("2006-01-02", a.LastTxDate); err == nil {
			if cand := t.AddDate(0, 0, -resyncOverlapDays); cand.After(from) {
				from = cand
			}
		}
	}
	row.From = from.Format("2006-01-02")
	q := openbanking.TxQuery{AccountUID: a.UID, DateFrom: from, DateTo: now, PSU: psu, RequiredPSUHeaders: headers}
	booked, err := cl.AllTransactions(ctx, q)
	if err != nil {
		return 0, err
	}
	// Balance: best-effort, a bank that will not state one must not fail
	// a sync whose transactions arrived.
	if bs, err := cl.Balances(ctx, a.UID, psu, headers); err == nil {
		if b := openbanking.PickBalance(bs); b != nil {
			types := make([]string, 0, len(bs))
			for _, x := range bs {
				types = append(types, x.BalanceType+"="+x.BalanceAmount.Amount)
			}
			log.Printf("bank sync: account %d balances %v, using %s", a.ID, types, b.BalanceType)
			if v, err := strconv.ParseFloat(strings.TrimSpace(b.BalanceAmount.Amount), 64); err == nil {
				if strings.EqualFold(b.CreditDebitIndicator, "DBIT") && v > 0 {
					v = -v
				}
				c := money.FromFloat(v)
				a.BankBalance = &c
				a.BalanceCurrency = strings.ToUpper(b.BalanceAmount.Currency)
				a.BalanceType = strings.ToUpper(b.BalanceType)
				a.BalanceDate = firstNonEmpty(isoDate(b.ReferenceDate), now.Format("2006-01-02"))
				a.BalanceFetched = db.Now()
			}
		}
	} else {
		log.Printf("bank sync: balance unavailable for account %d: %v", a.ID, err)
	}
	// Reservations are asked for by name; best effort.
	q.TransactionStatus, q.ContinuationKey = openbanking.StatusPending, ""
	all := booked
	if pending, err := cl.AllTransactions(ctx, q); err == nil {
		seen := map[string]bool{}
		for _, t := range booked {
			seen[strings.ToUpper(t.Status)+"|"+firstNonEmpty(t.EntryReference, t.TransactionID)] = true
		}
		for _, t := range pending {
			ref := firstNonEmpty(t.EntryReference, t.TransactionID)
			if ref != "" && seen[strings.ToUpper(t.Status)+"|"+ref] {
				continue
			}
			all = append(all, t)
		}
	}
	row.Fetched = len(all)
	res.Fetched += len(all)
	n, err := s.stage(all, a, from, Context{OwnerNames: set.OwnerNames, OwnIBANs: owned}, res)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// stage upserts provider rows into the inbox.
func (s *Service) stage(txs []openbanking.Transaction, a *BankAccount, from time.Time, ctx Context, res *SyncResult) (int, error) {
	eng, err := ledger.LoadEngine(s.DB)
	if err != nil {
		return 0, err
	}
	existing := map[string]int64{}
	rows, err := s.DB.Query(`SELECT external_id, id FROM transactions WHERE external_id IS NOT NULL AND external_id != ''`)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var ext string
		var id int64
		rows.Scan(&ext, &id)
		existing[ext] = id
	}
	rows.Close()
	open, err := listInbox(s.DB, "open")
	if err != nil {
		return 0, err
	}
	var holds []*InboxRow
	for i := range open {
		if open[i].Pending && open[i].BankAccountID == a.ID {
			holds = append(holds, &open[i])
		}
	}
	// Reservations already in the ledger (pending) can be settled or released too.
	imported, err := listInbox(s.DB, "imported")
	if err != nil {
		return 0, err
	}
	for i := range imported {
		if imported[i].Pending && imported[i].BankAccountID == a.ID && imported[i].ImportedTxID > 0 {
			holds = append(holds, &imported[i])
		}
	}
	taken := map[int64]bool{}
	seen := map[string]bool{}
	newest := a.LastTxDate
	created := 0
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, t := range txs {
		st := strings.ToUpper(strings.TrimSpace(t.Status))
		if st != "" && st != openbanking.StatusBooked && st != openbanking.StatusPending {
			continue
		}
		p := Adapt(t, a.ID, a.AccountID, ctx)
		seen[p.ExternalID] = true
		if !p.Pending && p.Date > newest {
			newest = p.Date
		}
		r := InboxRow{BankAccountID: a.ID, ExternalID: p.ExternalID, Raw: p.Raw, RawPayee: p.RawPayee, RawDetails: p.RawDetails,
			RawCurrency: p.RawCurrency, BookingDate: p.BookingDate, Pending: p.Pending, Date: p.Date, Kind: p.Kind, Amount: p.Amount,
			AccountID: p.AccountID, ToAccountID: p.ToAccountID, Category: p.Category, Merchant: p.Merchant, Note: p.Note, Tags: []string{},
			Guessed: p.Guessed, Verdict: p.Verdict, VerdictNote: p.VerdictNote, State: "open"}
		// Rules and the ledger's history refine what the classifier knew.
		lt := ledger.Tx{Kind: r.Kind, Category: r.Category, Merchant: r.Merchant, Note: r.Note, Tags: r.Tags}
		out := eng.Apply(&lt, r.Guessed, nil)
		r.Category, r.Merchant, r.Tags = lt.Category, lt.Merchant, lt.Tags
		if r.Kind != lt.Kind && lt.Kind != "" {
			r.Kind = lt.Kind
		}
		if out.Category != "" {
			r.Guessed = false
		}
		if id, ok := existing[r.ExternalID]; ok && !r.Pending {
			r.Verdict, r.MatchedTxID, r.VerdictNote = "duplicate", id, "already in your ledger"
		}

		prev, err := getInboxByExternal(tx, r.ExternalID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			// A booking claims the reservation it settles, inheriting the
			// corrections made while it was pending.
			var claimed *InboxRow
			if !r.Pending {
				bestGap := math.MaxFloat64
				for _, h := range holds {
					if taken[h.ID] || !reservationMatches(h, &r) {
						continue
					}
					if g := math.Abs(float64(h.Amount - r.Amount)); g < bestGap {
						claimed, bestGap = h, g
					}
				}
				// The bank often rewrites the payee text when it books a card
				// payment ("WL*GOOGLE Dublin IE" → "WL*GOOGLE YouTubePrem…"):
				// failing a name match, the one reservation with exactly this
				// amount in the window is the same payment.
				if claimed == nil {
					var only *InboxRow
					n := 0
					for _, h := range holds {
						if !taken[h.ID] && h.Amount == r.Amount && reservationInWindow(h, &r) {
							only = h
							n++
						}
					}
					if n == 1 {
						claimed = only
					}
				}
				if claimed != nil {
					taken[claimed.ID] = true
					if claimed.Edited {
						r.Category, r.Kind, r.Tags, r.Merchant = claimed.Category, claimed.Kind, claimed.Tags, claimed.Merchant
						r.AccountID, r.ToAccountID, r.Guessed = claimed.AccountID, claimed.ToAccountID, false
					}
				}
			}
			if r.Verdict == "duplicate" {
				r.State = "imported"
				r.ImportedTxID = r.MatchedTxID
			}
			// The reservation is already in the ledger: settle that transaction
			// with the bank's final amount instead of adding a second one.
			if claimed != nil && claimed.State == "imported" && claimed.ImportedTxID > 0 {
				settled, err := ledger.SettleReservation(tx, claimed.ImportedTxID, r.Amount, r.Date, r.ExternalID)
				if err != nil {
					return 0, err
				}
				if settled {
					r.State, r.ImportedTxID, r.MatchedTxID = "imported", claimed.ImportedTxID, claimed.ImportedTxID
					r.Verdict, r.VerdictNote = "duplicate", "booked — settled the transaction added while it was reserved"
					res.Settled++
				}
				// Not settled: the pending transaction was deleted meanwhile, so
				// the booking stays open in the inbox rather than vanishing.
			}
			if err := saveInbox(tx, &r); err != nil {
				return 0, fmt.Errorf("staging %s: %w", r.ExternalID, err)
			}
			if claimed != nil {
				claimed.State, claimed.SupersededBy = "superseded", r.ID
				claimed.VerdictNote = "booked by the bank on " + firstNonEmpty(r.BookingDate, r.Date)
				saveInbox(tx, claimed)
				res.Superseded++
			}
			if r.State == "open" {
				created++
				res.New++
				if r.Pending {
					res.Pending++
				}
			} else {
				res.Duplicates++
			}
		case err != nil:
			return 0, err
		default:
			// Known row. Dismissed stays dismissed, imported stays imported;
			// an untouched open row is re-proposed so classifier fixes reach it.
			if prev.State == "open" && !prev.Edited {
				r.ID, r.FirstSeenAt = prev.ID, prev.FirstSeenAt
				if r.Verdict == "duplicate" {
					r.State, r.ImportedTxID = "imported", r.MatchedTxID
				}
				saveInbox(tx, &r)
				res.Updated++
			} else {
				tx.Exec(`UPDATE bank_inbox SET last_seen_at=? WHERE id=?`, db.Now(), prev.ID)
			}
		}
	}
	// A reservation the bank stopped reporting inside the window we re-read
	// was released, not booked.
	for _, h := range holds {
		if taken[h.ID] || seen[h.ExternalID] || h.Date < from.Format("2006-01-02") {
			continue
		}
		if h.State == "imported" && h.ImportedTxID > 0 {
			// Added while reserved, never booked: take it back out.
			if _, err := ledger.DropReservation(tx, h.ImportedTxID); err != nil {
				return 0, err
			}
		}
		h.State, h.VerdictNote = "superseded", "the bank released this reservation without booking it"
		saveInbox(tx, h)
		res.Released++
	}
	a.LastSyncedAt, a.LastTxDate = db.Now(), newest
	if err := saveBankAccount(tx, a); err != nil {
		return 0, err
	}
	return created, tx.Commit()
}

func normPayee(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= 'A' && r <= 'Z') || r > 127 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func reservationMatches(hold, booked *InboxRow) bool {
	if !reservationInWindow(hold, booked) || !samePayee(hold.RawPayee, booked.RawPayee) {
		return false
	}
	held, got := hold.Amount.Float(), booked.Amount.Float()
	return held > 0 && got > 0 && math.Abs(held-got) <= math.Max(held*reservationTolerance, 0.5)
}

// reservationInWindow: same kind and currency, booked from the day before
// the reservation up to reservationWindow after it.
func reservationInWindow(hold, booked *InboxRow) bool {
	if hold.Kind != booked.Kind || !strings.EqualFold(hold.RawCurrency, booked.RawCurrency) {
		return false
	}
	hd, _ := time.Parse("2006-01-02", hold.Date)
	bd, _ := time.Parse("2006-01-02", booked.Date)
	gap := bd.Sub(hd)
	return gap >= -24*time.Hour && gap <= reservationWindow
}

// payeeNoise are words card terminals add that say nothing about who was
// paid: places, countries, legal forms.
var payeeNoise = map[string]bool{"VILNIUS": true, "KAUNAS": true, "KLAIPEDA": true, "DUBLIN": true, "LONDON": true, "LUXEMBOURG": true,
	"UAB": true, "LIMITED": true, "IRELAND": true, "LITHUANIA": true, "LIETUVA": true, "WWW": true, "COM": true, "INTERNET": true}

// samePayee: the same text (one may be cut short), or a shared distinctive
// word of four letters or more — "UNIPARK Vilnius VL LT" and
// "UNIPARK LT-08105 Vilnius" share UNIPARK.
func samePayee(a, b string) bool {
	na, nb := normPayee(a), normPayee(b)
	if na == "" || nb == "" {
		return false
	}
	if na == nb || strings.HasPrefix(na, nb) || strings.HasPrefix(nb, na) {
		return true
	}
	words := func(s string) map[string]bool {
		out := map[string]bool{}
		for _, w := range strings.FieldsFunc(strings.ToUpper(s), func(r rune) bool { return !((r >= 'A' && r <= 'Z') || r > 127) }) {
			if len([]rune(w)) >= 4 && !payeeNoise[w] {
				out[w] = true
			}
		}
		return out
	}
	wb := words(b)
	for w := range words(a) {
		if wb[w] {
			return true
		}
	}
	return false
}

// applyBankBalances sets each synced ledger account to the sum of the bank
// balances of every bank account feeding it, as today's balance.
func (s *Service) applyBankBalances(synced map[string]bool) []BalanceSet {
	if len(synced) == 0 {
		return nil
	}
	accts, _ := listBankAccounts(s.DB, 0)
	book, _ := wealth.LoadBook(s.DB)
	today := time.Now().Format("2006-01-02")
	ids := make([]string, 0, len(synced))
	for id := range synced {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []BalanceSet
	for _, id := range ids {
		if book != nil && book.Accounts[id].Archived {
			continue // hidden = closed at €0; a still-linked bank account doesn't reopen it
		}
		var sum money.Cents
		skip := ""
		for _, a := range accts {
			if a.AccountID != id || a.UID == "" {
				continue
			}
			fetched, _ := time.Parse(time.RFC3339, a.BalanceFetched)
			switch {
			case a.BankBalance == nil || time.Since(fetched) > balanceFreshness:
				skip = firstNonEmpty(a.DisplayName, a.IBAN) + " has no fresh balance from the bank"
			case a.BalanceCurrency != "" && a.BalanceCurrency != "EUR":
				skip = firstNonEmpty(a.DisplayName, a.IBAN) + " is in " + a.BalanceCurrency + " — only EUR balances are applied"
			default:
				sum += *a.BankBalance
			}
		}
		set := BalanceSet{AccountID: id, After: sum, Skipped: skip}
		if book != nil {
			if p, ok := book.At(id, today); ok {
				set.Before = p.Value
			}
		}
		if skip == "" {
			if err := wealth.SetBalance(s.DB, id, today, sum, nil, nil, "bank"); err != nil {
				set.Skipped = err.Error()
			}
		}
		out = append(out, set)
	}
	return out
}

// addReservations puts fresh card reservations straight into the ledger as
// pending transactions, so today's spending counts today. Only rows that are
// ready (category known, not flagged for review or as duplicates).
func (s *Service) addReservations() int {
	if LoadSettings(s.DB).KeepReservedInInbox {
		return 0
	}
	open, err := listInbox(s.DB, "open")
	if err != nil {
		return 0
	}
	var ids []int64
	for i := range open {
		r := &open[i]
		if r.Pending && r.Ready() && r.Verdict == "pending" {
			ids = append(ids, r.ID)
		}
	}
	if len(ids) == 0 {
		return 0
	}
	res, err := s.Commit(ids)
	if err != nil {
		return 0
	}
	return len(res.Imported)
}
