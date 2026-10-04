// Package openbanking is a client for the Enable Banking API — a licensed
// AISP aggregator that fronts the PSD2 interfaces of European banks. Going
// direct to a bank's PSD2 API needs an AISP licence plus eIDAS QWAC/QSEAL
// certificates, so an aggregator is not optional.
//
// This package is a pure API client: it knows nothing about transaction
// classification, categories or the ledger. Mapping provider rows onto domain
// transactions happens in package handler, next to the classifier.
package openbanking

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// BaseURL is a variable so tests can point the client at an httptest server.
// Sandbox and production applications share one host — which environment you
// are in is a property of the registered application, not of the endpoint.
var BaseURL = "https://api.enablebanking.com"

// Request/response limits.
const (
	// maxBodyBytes caps what we will read from the provider. A 2,000-row
	// transaction page is well under 2 MiB.
	maxBodyBytes = 8 << 20
	// requestTimeout is per HTTP call. Banks behind the aggregator can be
	// slow; the whole sync still has to finish inside the 180s server write
	// timeout, so this is deliberately tighter than that.
	requestTimeout = 45 * time.Second
	// tokenTTL is how long a minted JWT claims to live. The API caps it at
	// 86400s; an hour keeps the blast radius of a leaked token small.
	tokenTTL = time.Hour
	// tokenRefresh is when we mint a fresh one rather than reuse the cached
	// token — well before expiry, so no request races the boundary.
	tokenRefresh = 50 * time.Minute
)

// Client talks to Enable Banking on behalf of one registered application.
type Client struct {
	appID string
	key   *rsa.PrivateKey
	http  *http.Client

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
}

// New builds a client from the application id and its RSA private key in PEM
// form. Both PKCS#1 ("BEGIN RSA PRIVATE KEY") and PKCS#8 ("BEGIN PRIVATE
// KEY") are accepted — Enable Banking hands out the latter.
func New(appID, privateKeyPEM string) (*Client, error) {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return nil, fmt.Errorf("enable banking application id is empty")
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(privateKeyPEM))
	if err != nil {
		// Deliberately not wrapping err: the underlying message can echo key
		// material fragments on a malformed PEM.
		return nil, fmt.Errorf("enable banking private key is not a valid RSA PEM")
	}
	return &Client{
		appID: appID,
		key:   key,
		http:  &http.Client{Timeout: requestTimeout},
	}, nil
}

// bearer returns a cached JWT, minting a new one when it is close to expiry.
func (c *Client) bearer() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExpiry) {
		return c.token, nil
	}
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": "enablebanking.com",
		"aud": "api.enablebanking.com",
		"iat": now.Unix(),
		"exp": now.Add(tokenTTL).Unix(),
	})
	// The application id travels as the `kid` header, not as a claim — this
	// is how the API knows which registered key to verify against.
	tok.Header["kid"] = c.appID
	signed, err := tok.SignedString(c.key)
	if err != nil {
		return "", fmt.Errorf("signing enable banking token: %w", err)
	}
	c.token = signed
	c.tokenExpiry = now.Add(tokenRefresh)
	return signed, nil
}

// PSU holds the end-user context headers. Enable Banking treats these as
// all-or-nothing per bank: send the complete required set, or send none.
type PSU struct {
	IPAddress string
	UserAgent string
}

// psuHeaderNames maps the names a bank asks for in required_psu_headers onto
// what we can actually supply.
func (p PSU) header(name string) string {
	switch strings.ToLower(name) {
	case "psu-ip-address":
		return p.IPAddress
	case "psu-user-agent":
		return p.UserAgent
	}
	return ""
}

// headersFor builds the full PSU header set for a bank, or nil if any
// required header cannot be supplied. Sending a partial set is a 422
// PSU_HEADER_NOT_PROVIDED, so the unattended path (no headers at all) is
// strictly better than a half-filled one.
func (p PSU) headersFor(required []string) map[string]string {
	if len(required) == 0 {
		return nil
	}
	out := make(map[string]string, len(required))
	for _, name := range required {
		v := p.header(name)
		if v == "" {
			return nil
		}
		out[name] = v
	}
	return out
}

// do performs one API call. in is marshalled as JSON when non-nil; out is
// unmarshalled from the response when non-nil.
func (c *Client) do(ctx context.Context, method, path string, in, out any, headers map[string]string) error {
	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encoding request: %w", err)
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, BaseURL+path, body)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	tok, err := c.bearer()
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// net/http decorates errors with the full URL; the path can carry an
		// account uid, so report the operation instead.
		return fmt.Errorf("enable banking request failed (%s)", method)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("reading enable banking response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return parseAPIError(resp.StatusCode, raw)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decoding enable banking response: %w", err)
	}
	return nil
}

// ASPSPs lists the banks available to this application in one country.
func (c *Client) ASPSPs(ctx context.Context, country string) ([]ASPSP, error) {
	var out aspspsResponse
	q := url.Values{}
	if country != "" {
		q.Set("country", country)
	}
	path := "/aspsps"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &out, nil); err != nil {
		return nil, err
	}
	return out.ASPSPs, nil
}

// AuthParams describes the consent to request.
type AuthParams struct {
	ASPSPName    string
	ASPSPCountry string
	State        string
	RedirectURL  string
	// ValidUntil is capped by the bank's maximum_consent_validity.
	ValidUntil time.Time
	PSU        PSU
	// RequiredPSUHeaders comes from the bank's ASPSP entry.
	RequiredPSUHeaders []string
}

// StartAuth returns the URL to send the browser to for strong authentication.
// Both balances and transactions access is requested: Swedbank LT omits the
// account-holder name from the account list without the transactions scope.
func (c *Client) StartAuth(ctx context.Context, p AuthParams) (*AuthStart, error) {
	req := startAuthRequest{
		Access: Access{
			ValidUntil:   p.ValidUntil.UTC().Format(time.RFC3339),
			Balances:     true,
			Transactions: true,
		},
		ASPSP:       aspspRef{Name: p.ASPSPName, Country: p.ASPSPCountry},
		State:       p.State,
		RedirectURL: p.RedirectURL,
		PSUType:     "personal",
	}
	var out AuthStart
	if err := c.do(ctx, http.MethodPost, "/auth", req, &out, p.PSU.headersFor(p.RequiredPSUHeaders)); err != nil {
		return nil, err
	}
	return &out, nil
}

// AuthorizeSession exchanges the code from the redirect for a session and the
// list of accounts it unlocked.
func (c *Client) AuthorizeSession(ctx context.Context, code string) (*Session, error) {
	var out Session
	if err := c.do(ctx, http.MethodPost, "/sessions", authorizeSessionRequest{Code: code}, &out, nil); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteSession revokes a consent upstream. It returns the provider's error
// as-is, including the 404 for a session that is already gone — deciding
// that a 404 is harmless is the caller's call, not the client's, and
// bank_handler.go makes it by discarding the result.
func (c *Client) DeleteSession(ctx context.Context, sessionID string) error {
	return c.do(ctx, http.MethodDelete, "/sessions/"+url.PathEscape(sessionID), nil, nil, nil)
}

// Balances fetches every balance the bank reports for one account.
func (c *Client) Balances(ctx context.Context, accountUID string, psu PSU, requiredPSUHeaders []string) ([]Balance, error) {
	var out balancesResponse
	path := "/accounts/" + url.PathEscape(accountUID) + "/balances"
	if err := c.do(ctx, http.MethodGet, path, nil, &out, psu.headersFor(requiredPSUHeaders)); err != nil {
		return nil, err
	}
	return out.Balances, nil
}

// TxQuery is one page request against one account.
type TxQuery struct {
	AccountUID string
	DateFrom   time.Time
	DateTo     time.Time
	// TransactionStatus asks the bank for one status only (BOOK or PDNG).
	// Empty means whatever the ASPSP returns by default, which is booked
	// rows at most banks — reservations have to be asked for by name.
	TransactionStatus  string
	ContinuationKey    string
	PSU                PSU
	RequiredPSUHeaders []string
}

// TxPage is one page of transactions. ContinuationKey is empty on the last.
type TxPage struct {
	Transactions    []Transaction
	ContinuationKey string
}

// Transactions fetches one page. Paging is the caller's job via
// ContinuationKey, so a caller can bound how much it pulls.
func (c *Client) Transactions(ctx context.Context, q TxQuery) (*TxPage, error) {
	v := url.Values{}
	if !q.DateFrom.IsZero() {
		v.Set("date_from", q.DateFrom.Format("2006-01-02"))
	}
	if !q.DateTo.IsZero() {
		v.Set("date_to", q.DateTo.Format("2006-01-02"))
	}
	if q.TransactionStatus != "" {
		v.Set("transaction_status", q.TransactionStatus)
	}
	if q.ContinuationKey != "" {
		v.Set("continuation_key", q.ContinuationKey)
	}
	path := "/accounts/" + url.PathEscape(q.AccountUID) + "/transactions"
	if len(v) > 0 {
		path += "?" + v.Encode()
	}
	var out halTransactions
	if err := c.do(ctx, http.MethodGet, path, nil, &out, q.PSU.headersFor(q.RequiredPSUHeaders)); err != nil {
		return nil, err
	}
	return &TxPage{Transactions: out.Transactions, ContinuationKey: out.ContinuationKey}, nil
}

// maxPages bounds a single account's walk. A 90-day window over a personal
// account is a page or two; this is a runaway guard, not a limit anyone
// should reach.
const maxPages = 25

// ErrTruncated means the walk hit maxPages with more pages still to come.
//
// This has to be an error, not a quiet short return. The caller decides what
// to do with a partial window by inspecting err; handing it a truncated slice
// with a nil error would let it stage an incomplete window, advance the sync
// anchor past the rows it never saw, and report "nothing new" forever after.
var ErrTruncated = errors.New("transaction history was longer than expected and has been truncated — narrow the date range")

// AllTransactions walks every page for one account, up to maxPages. Returns
// what it managed to collect alongside any error, so a caller that *wants*
// partial results can still use them.
func (c *Client) AllTransactions(ctx context.Context, q TxQuery) ([]Transaction, error) {
	var all []Transaction
	for page := 0; page < maxPages; page++ {
		p, err := c.Transactions(ctx, q)
		if err != nil {
			return all, err
		}
		all = append(all, p.Transactions...)
		if p.ContinuationKey == "" {
			return all, nil
		}
		q.ContinuationKey = p.ContinuationKey
	}
	return all, ErrTruncated
}
