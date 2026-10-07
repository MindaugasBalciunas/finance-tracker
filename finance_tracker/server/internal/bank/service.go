package bank

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
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

// Windows and tolerances — each learned from live syncs in v1.
const (
	firstSyncDays        = 90 // what LT banks serve without a second SCA
	resyncOverlapDays    = 7  // late-posting card rows
	reservationWindow    = 10 * 24 * time.Hour
	reservationTolerance = 0.20 // tips and fuel pre-auths move the amount
	matchDayWindow       = 3    // hand-entered vs bank date drift
	autoLinkDayWindow    = 1
	balanceFreshness     = 24 * time.Hour
)

type Service struct {
	DB *sql.DB
}

var ErrNotConfigured = errors.New("bank connections are not configured — add your Enable Banking application in Settings → Banks")

func (s *Service) client() (*openbanking.Client, Settings, error) {
	set := LoadSettings(s.DB)
	if !set.Configured() {
		return nil, set, ErrNotConfigured
	}
	cl, err := openbanking.New(set.ApplicationID, set.PrivateKeyPEM)
	return cl, set, err
}

// ── connections ─────────────────────────────────────────────────────

// Connections lists every consent with its accounts.
func (s *Service) Connections() ([]Connection, error) {
	conns, err := listConnections(s.DB)
	if err != nil {
		return nil, err
	}
	accts, err := listBankAccounts(s.DB, 0)
	if err != nil {
		return nil, err
	}
	open := map[int64]int{}
	rows, _ := s.DB.Query(`SELECT bank_account_id, COUNT(*) FROM bank_inbox WHERE state='open' GROUP BY 1`)
	if rows != nil {
		for rows.Next() {
			var id int64
			var n int
			rows.Scan(&id, &n)
			open[id] = n
		}
		rows.Close()
	}
	for i := range conns {
		for _, a := range accts {
			if a.ConnectionID == conns[i].ID {
				a.IBAN = openbanking.MaskIBAN(a.IBAN)
				a.Open = open[a.ID]
				conns[i].Accounts = append(conns[i].Accounts, a)
			}
		}
	}
	return conns, nil
}

type ASPSP struct {
	Name     string `json:"name"`
	Country  string `json:"country"`
	Logo     string `json:"logo"`
	Beta     bool   `json:"beta"`
	MaxDays  int    `json:"max_consent_days"`
	Redirect bool   `json:"redirect"`
}

func (s *Service) Banks(ctx context.Context, country string) ([]ASPSP, error) {
	cl, _, err := s.client()
	if err != nil {
		return nil, err
	}
	list, err := cl.ASPSPs(ctx, country)
	if err != nil {
		return nil, err
	}
	var out []ASPSP
	for _, a := range list {
		redirect := false
		for _, m := range a.AuthMethods {
			if m.Approach == "REDIRECT" && !bool(m.Hidden) {
				redirect = true
			}
		}
		out = append(out, ASPSP{Name: a.Name, Country: a.Country, Logo: a.Logo, Beta: bool(a.Beta), MaxDays: a.MaximumConsentValidity / 86400, Redirect: redirect})
	}
	return out, nil
}

func newState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Connect starts a consent and returns the bank's authorisation URL.
func (s *Service) Connect(ctx context.Context, bankName, country string, psu openbanking.PSU) (int64, string, error) {
	cl, set, err := s.client()
	if err != nil {
		return 0, "", err
	}
	if set.RedirectURL == "" {
		return 0, "", errors.New("set the redirect URL in bank settings first — it must match the one registered with Enable Banking exactly")
	}
	if country == "" {
		country = "LT"
	}
	banks, err := cl.ASPSPs(ctx, country)
	if err != nil {
		return 0, "", err
	}
	var bank *openbanking.ASPSP
	for i := range banks {
		if strings.EqualFold(banks[i].Name, bankName) {
			bank = &banks[i]
		}
	}
	if bank == nil {
		return 0, "", fmt.Errorf("bank %q is not available in %s for this application", bankName, country)
	}
	validUntil := time.Now().Add(time.Duration(set.ConsentDays) * 24 * time.Hour)
	if bank.MaximumConsentValidity > 0 {
		if cap := time.Now().Add(time.Duration(bank.MaximumConsentValidity) * time.Second); cap.Before(validUntil) {
			validUntil = cap.Add(-time.Minute)
		}
	}
	c := &Connection{ASPSPName: bank.Name, Country: bank.Country, Status: "pending", ValidUntil: validUntil.UTC().Format(time.RFC3339), AuthState: newState()}
	if err := saveConnection(s.DB, c); err != nil {
		return 0, "", err
	}
	start, err := cl.StartAuth(ctx, openbanking.AuthParams{ASPSPName: bank.Name, ASPSPCountry: bank.Country, State: c.AuthState,
		RedirectURL: set.RedirectURL, ValidUntil: validUntil, PSU: psu, RequiredPSUHeaders: bank.RequiredPSUHeaders})
	if err != nil {
		c.Status, c.LastError = "revoked", err.Error()
		saveConnection(s.DB, c)
		return 0, "", err
	}
	return c.ID, start.URL, nil
}

// ParseCallbackURL pulls code+state out of a pasted redirect address.
func ParseCallbackURL(raw string) (code, state string, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", errors.New("that does not look like a web address")
	}
	q := u.Query()
	if e := q.Get("error"); e != "" {
		if d := q.Get("error_description"); d != "" {
			return "", "", fmt.Errorf("the bank refused the connection: %s", d)
		}
		return "", "", fmt.Errorf("the bank refused the connection (%s)", e)
	}
	code, state = q.Get("code"), q.Get("state")
	if code == "" && u.Fragment != "" {
		if i := strings.IndexByte(u.Fragment, '?'); i >= 0 {
			if fq, perr := url.ParseQuery(u.Fragment[i+1:]); perr == nil {
				code, state = fq.Get("code"), fq.Get("state")
			}
		}
	}
	return code, state, nil
}

// Callback exchanges the authorisation code for a session and records the
// accounts it unlocked. Accounts match on identification_hash, which
// survives re-authorisation (the uid does not).
func (s *Service) Callback(ctx context.Context, code, state string) (int64, error) {
	if code == "" || state == "" {
		return 0, errors.New("the address is missing the code or state parameter — paste the full address you landed on")
	}
	var id int64
	if err := s.DB.QueryRow(`SELECT id FROM bank_connections WHERE auth_state=? AND auth_state != ''`, state).Scan(&id); err != nil {
		return 0, errors.New("this authorisation link has expired or was already used — start the connection again")
	}
	c, err := getConnection(s.DB, id)
	if err != nil {
		return 0, err
	}
	c.AuthState = "" // burn the nonce before the exchange
	saveConnection(s.DB, &c)
	cl, _, err := s.client()
	if err != nil {
		return 0, err
	}
	sess, err := cl.AuthorizeSession(ctx, code)
	if err != nil {
		c.LastError = err.Error()
		saveConnection(s.DB, &c)
		return 0, err
	}
	c.SessionID, c.Status, c.LastError = sess.SessionID, "authorized", ""
	if sess.ASPSP.Name != "" {
		c.ASPSPName = sess.ASPSP.Name
	}
	if vu, err := time.Parse(time.RFC3339, sess.Access.ValidUntil); err == nil {
		c.ValidUntil = vu.UTC().Format(time.RFC3339)
	}
	if err := saveConnection(s.DB, &c); err != nil {
		return 0, err
	}
	for _, a := range sess.Accounts {
		var ba BankAccount
		err := s.DB.QueryRow(`SELECT id FROM bank_accounts WHERE connection_id=? AND identification_hash=?`, c.ID, a.IdentificationHash).Scan(&ba.ID)
		if errors.Is(err, sql.ErrNoRows) {
			// A re-consent to the same bank: carry the mapping from the
			// old connection's account with the same hash.
			s.DB.QueryRow(`SELECT account_id FROM bank_accounts WHERE identification_hash=? AND account_id != '' ORDER BY id DESC LIMIT 1`, a.IdentificationHash).Scan(&ba.AccountID)
			ba.ConnectionID, ba.IdentificationHash = c.ID, a.IdentificationHash
		} else if err == nil {
			ba, _ = getBankAccount(s.DB, ba.ID)
		}
		ba.UID, ba.IBAN, ba.DisplayName = a.UID, a.AccountID.IBAN, a.Label()
		if ba.ID == 0 {
			if err := saveBankAccount(s.DB, &ba); err != nil {
				return 0, err
			}
		}
		if err := saveBankAccount(s.DB, &ba); err != nil {
			return 0, err
		}
	}
	return c.ID, nil
}

// Disconnect revokes upstream (best effort) and deletes locally.
func (s *Service) Disconnect(ctx context.Context, id int64) error {
	c, err := getConnection(s.DB, id)
	if err != nil {
		return errors.New("connection not found")
	}
	if c.SessionID != "" {
		if cl, _, err := s.client(); err == nil {
			_ = cl.DeleteSession(ctx, c.SessionID)
		}
	}
	_, err = s.DB.Exec(`DELETE FROM bank_connections WHERE id=?`, id)
	return err
}

// MapAccount sets which ledger account a bank account feeds (” = don't sync).
func (s *Service) MapAccount(id int64, accountID string) error {
	a, err := getBankAccount(s.DB, id)
	if err != nil {
		return errors.New("bank account not found")
	}
	if accountID != "" {
		if _, err := ledger.GetAccount(s.DB, accountID); err != nil {
			return fmt.Errorf("unknown account %q", accountID)
		}
	}
	a.AccountID = accountID
	return saveBankAccount(s.DB, &a)
}

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
	if hold.Kind != booked.Kind || !strings.EqualFold(hold.RawCurrency, booked.RawCurrency) {
		return false
	}
	na, nb := normPayee(hold.RawPayee), normPayee(booked.RawPayee)
	if na == "" || nb == "" || !(na == nb || strings.HasPrefix(na, nb) || strings.HasPrefix(nb, na)) {
		return false
	}
	hd, _ := time.Parse("2006-01-02", hold.Date)
	bd, _ := time.Parse("2006-01-02", booked.Date)
	if gap := bd.Sub(hd); gap < -24*time.Hour || gap > reservationWindow {
		return false
	}
	held, got := hold.Amount.Float(), booked.Amount.Float()
	return held > 0 && got > 0 && math.Abs(held-got) <= math.Max(held*reservationTolerance, 0.5)
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

// ── inbox ───────────────────────────────────────────────────────────

// Inbox lists rows in a state with likely hand-entered matches attached.
func (s *Service) Inbox(state string) ([]InboxRow, error) {
	if state == "" {
		state = "open"
	}
	rows, err := listInbox(s.DB, state)
	if err != nil {
		return nil, err
	}
	if state != "open" {
		return rows, nil
	}
	cands, err := s.handEntered()
	if err != nil {
		return nil, err
	}
	claimed := map[int64]bool{}
	for i := range rows {
		r := &rows[i]
		if r.Pending || r.Verdict == "duplicate" {
			continue
		}
		if m := findMatch(r, cands, claimed, matchDayWindow); m != nil {
			mm := *m
			r.Match = &mm
			claimed[m.ID] = true
		}
		r.Raw = ""
	}
	return rows, nil
}

// handEntered lists recent ledger rows not linked to any bank row.
func (s *Service) handEntered() ([]ledger.Tx, error) {
	return ledger.All(s.DB, ledger.Filter{From: time.Now().AddDate(0, 0, -150).Format("2006-01-02")})
}

func sameDirection(a, b string) bool {
	out := func(k string) bool { return k == "expense" || k == "transfer" }
	return a == b || (out(a) && out(b))
}

func findMatch(r *InboxRow, cands []ledger.Tx, claimed map[int64]bool, window int) *ledger.Tx {
	var best *ledger.Tx
	bestScore := math.MaxFloat64
	rd, _ := time.Parse("2006-01-02", r.Date)
	for i := range cands {
		t := &cands[i]
		if t.ExternalID != "" || claimed[t.ID] || t.Amount != r.Amount || !sameDirection(t.Kind, r.Kind) {
			continue
		}
		td, _ := time.Parse("2006-01-02", t.Date)
		gap := math.Abs(td.Sub(rd).Hours() / 24)
		if gap > float64(window) {
			continue
		}
		score := gap
		if t.AccountID != r.AccountID {
			score += 10
		}
		if t.Kind != r.Kind {
			score += 0.5
		}
		if score < bestScore {
			best, bestScore = t, score
		}
	}
	return best
}

// autoLink links rows to hand-entered transactions when there is no doubt:
// same kind, amount and account, ≤1 day apart, and no rival either side.
func (s *Service) autoLink() int {
	rows, err := listInbox(s.DB, "open")
	if err != nil {
		return 0
	}
	cands, err := s.handEntered()
	if err != nil {
		return 0
	}
	n := 0
	used := map[int64]bool{}
	for i := range rows {
		r := &rows[i]
		if r.Pending || r.Verdict == "duplicate" {
			continue
		}
		rd, _ := time.Parse("2006-01-02", r.Date)
		var match *ledger.Tx
		rivals := 0
		for j := range cands {
			t := &cands[j]
			if t.ExternalID != "" || used[t.ID] || t.Amount != r.Amount || !sameDirection(t.Kind, r.Kind) {
				continue
			}
			td, _ := time.Parse("2006-01-02", t.Date)
			gap := math.Abs(td.Sub(rd).Hours() / 24)
			if gap > matchDayWindow {
				continue
			}
			rivals++
			if t.Kind == r.Kind && t.AccountID == r.AccountID && gap <= autoLinkDayWindow {
				match = t
			}
		}
		if match == nil || rivals != 1 {
			continue
		}
		competing := 0
		for k := range rows {
			o := &rows[k]
			if o.ID != r.ID && !o.Pending && o.Amount == r.Amount && sameDirection(o.Kind, r.Kind) {
				od, _ := time.Parse("2006-01-02", o.Date)
				md, _ := time.Parse("2006-01-02", match.Date)
				if math.Abs(od.Sub(md).Hours()/24) <= matchDayWindow {
					competing++
				}
			}
		}
		if competing > 0 {
			continue
		}
		if err := s.Link(r.ID, match.ID); err == nil {
			used[match.ID] = true
			match.ExternalID = r.ExternalID
			n++
		}
	}
	return n
}

// Link attaches a bank row to a transaction already in the ledger: the
// ledger row keeps every field the owner wrote, and gains the bank's id so
// later syncs recognise it.
func (s *Service) Link(inboxID, txID int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, err := getInbox(tx, inboxID)
	if err != nil {
		return errors.New("inbox row not found")
	}
	if r.State != "open" {
		return fmt.Errorf("this row was already %s", r.State)
	}
	t, err := ledger.Get(tx, txID)
	if err != nil {
		return errors.New("transaction not found")
	}
	if t.ExternalID != "" {
		return errors.New("that transaction is already linked to a bank row")
	}
	if _, err := tx.Exec(`UPDATE transactions SET external_id=?, updated_at=? WHERE id=?`, r.ExternalID, db.Now(), txID); err != nil {
		return err
	}
	r.State, r.ImportedTxID, r.MatchedTxID, r.Verdict = "imported", txID, txID, "duplicate"
	r.VerdictNote = "linked to a transaction you entered"
	if err := saveInbox(tx, &r); err != nil {
		return err
	}
	return tx.Commit()
}

// Unlink reverses Link: the ledger row loses the bank id and the bank row
// returns to the inbox.
func (s *Service) Unlink(inboxID int64) error {
	r, err := getInbox(s.DB, inboxID)
	if err != nil || r.ImportedTxID == 0 {
		return errors.New("this row is not linked")
	}
	var source string
	s.DB.QueryRow(`SELECT source FROM transactions WHERE id=?`, r.ImportedTxID).Scan(&source)
	if source == "bank" {
		return errors.New("this row was added from the bank, not linked — delete the transaction instead")
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	tx.Exec(`UPDATE transactions SET external_id=NULL, updated_at=? WHERE id=? AND external_id=?`, db.Now(), r.ImportedTxID, r.ExternalID)
	r.State, r.ImportedTxID, r.MatchedTxID, r.Verdict, r.VerdictNote = "open", 0, 0, "new", ""
	saveInbox(tx, &r)
	return tx.Commit()
}

// Edit applies the owner's corrections to an open row.
type Edit struct {
	Date        *string      `json:"date"`
	Kind        *string      `json:"kind"`
	Amount      *money.Cents `json:"amount"`
	AccountID   *string      `json:"account_id"`
	ToAccountID *string      `json:"to_account_id"`
	Category    *string      `json:"category"`
	Merchant    *string      `json:"merchant"`
	Note        *string      `json:"note"`
	Tags        *[]string    `json:"tags"`
}

func (s *Service) Update(id int64, e Edit) (InboxRow, error) {
	r, err := getInbox(s.DB, id)
	if err != nil {
		return r, errors.New("inbox row not found")
	}
	if r.State != "open" {
		return r, fmt.Errorf("this row was already %s", r.State)
	}
	if e.Date != nil {
		r.Date = *e.Date
	}
	if e.Kind != nil {
		r.Kind = *e.Kind
	}
	if e.Amount != nil {
		r.Amount = *e.Amount
	}
	if e.AccountID != nil {
		r.AccountID = *e.AccountID
	}
	if e.ToAccountID != nil {
		r.ToAccountID = *e.ToAccountID
	}
	if e.Category != nil {
		r.Category = *e.Category
		var kind string
		if s.DB.QueryRow(`SELECT kind FROM categories WHERE id=?`, r.Category).Scan(&kind) == nil {
			r.Kind = kind
		}
		r.Guessed = false
	}
	if e.Merchant != nil {
		r.Merchant = strings.TrimSpace(*e.Merchant)
	}
	if e.Note != nil {
		r.Note = strings.TrimSpace(*e.Note)
	}
	if e.Tags != nil {
		r.Tags = ledger.NormalizeTags(*e.Tags)
	}
	if r.Verdict == "needs_review" && r.Amount > 0 && r.RawCurrency != "EUR" && e.Amount != nil {
		r.Verdict, r.VerdictNote = "new", ""
	}
	r.Edited = true
	return r, saveInbox(s.DB, &r)
}

type CommitResult struct {
	Imported []int64  `json:"imported"`
	Skipped  int      `json:"skipped"`
	Notes    []string `json:"notes,omitempty"`
}

// Commit turns open rows into transactions.
func (s *Service) Commit(ids []int64) (*CommitResult, error) {
	res := &CommitResult{Imported: []int64{}}
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, id := range ids {
		r, err := getInbox(tx, id)
		if err != nil {
			res.Skipped++
			continue
		}
		label := firstNonEmpty(r.Merchant, r.Note, r.ExternalID)
		if r.State != "open" {
			res.Skipped++
			res.Notes = append(res.Notes, label+" was already "+r.State)
			continue
		}
		var dup int64
		if tx.QueryRow(`SELECT id FROM transactions WHERE external_id=?`, r.ExternalID).Scan(&dup) == nil {
			r.State, r.ImportedTxID, r.Verdict = "imported", dup, "duplicate"
			saveInbox(tx, &r)
			res.Skipped++
			res.Notes = append(res.Notes, label+" is already in your ledger")
			continue
		}
		// A reservation enters as pending; the booking settles it later.
		t := ledger.Tx{Date: r.Date, Kind: r.Kind, Amount: r.Amount, AccountID: r.AccountID, ToAccountID: r.ToAccountID, Category: r.Category,
			Merchant: r.Merchant, Note: r.Note, Tags: r.Tags, ExternalID: r.ExternalID, Source: "bank", Pending: r.Pending}
		if err := ledger.Validate(tx, &t); err != nil {
			res.Skipped++
			res.Notes = append(res.Notes, label+": "+err.Error())
			continue
		}
		if err := ledger.Insert(tx, &t); err != nil {
			return nil, err
		}
		// A booked transfer to or from one of your accounts the bank doesn't
		// sync (a savings account entered by hand) moves that account's
		// balance, so it doesn't keep showing money already moved out.
		// Synced accounts are skipped: their bank states the truth.
		if t.Kind == "transfer" && !t.Pending {
			for acct, delta := range wealth.TxDeltas(t.Kind, t.AccountID, t.ToAccountID, t.Amount) {
				if _, err := wealth.ApplyDelta(tx, acct, t.Date, delta); err != nil {
					return nil, err
				}
			}
		}
		r.State, r.ImportedTxID = "imported", t.ID
		if err := saveInbox(tx, &r); err != nil {
			return nil, err
		}
		res.Imported = append(res.Imported, t.ID)
	}
	return res, tx.Commit()
}

// SetState dismisses or restores rows.
func (s *Service) SetState(ids []int64, to string) (int, error) {
	from := "open"
	if to == "open" {
		from = "dismissed"
	}
	n := 0
	for _, id := range ids {
		res, err := s.DB.Exec(`UPDATE bank_inbox SET state=?, last_seen_at=? WHERE id=? AND state=?`, to, db.Now(), id, from)
		if err != nil {
			return n, err
		}
		if c, _ := res.RowsAffected(); c > 0 {
			n++
		}
	}
	return n, nil
}

// Raw returns the provider payload of one row (for "show raw bank data").
func (s *Service) Raw(id int64) (string, error) {
	var raw string
	err := s.DB.QueryRow(`SELECT raw FROM bank_inbox WHERE id=?`, id).Scan(&raw)
	return raw, err
}

// OpenCount is the inbox badge.
func (s *Service) OpenCount() int {
	var n int
	s.DB.QueryRow(`SELECT COUNT(*) FROM bank_inbox WHERE state='open'`).Scan(&n)
	return n
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

// ReturnToInbox handles a bank-sourced transaction the owner deleted. A booked
// row goes back to the inbox for review; a reservation is dismissed instead —
// otherwise the next sync would add it straight back.
func (s *Service) ReturnToInbox(e interface {
	Exec(string, ...any) (sql.Result, error)
}, txID int64) error {
	if _, err := e.Exec(`UPDATE bank_inbox SET state='dismissed', imported_tx_id=NULL, matched_tx_id=NULL,
		verdict_note='removed from the ledger while reserved', last_seen_at=? WHERE imported_tx_id=? AND pending=1`, db.Now(), txID); err != nil {
		return err
	}
	_, err := e.Exec(`UPDATE bank_inbox SET state='open', imported_tx_id=NULL, matched_tx_id=NULL, verdict='new', last_seen_at=?
		WHERE imported_tx_id=? AND pending=0`, db.Now(), txID)
	return err
}
