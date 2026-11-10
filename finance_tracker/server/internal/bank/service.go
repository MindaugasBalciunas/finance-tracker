package bank

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"ft/internal/bank/openbanking"
	"ft/internal/ledger"
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
