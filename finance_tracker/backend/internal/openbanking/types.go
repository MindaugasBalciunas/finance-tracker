package openbanking

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ASPSP is one bank as Enable Banking describes it. There is no stable id —
// a bank is addressed by name+country everywhere in the API.
type ASPSP struct {
	Name     string   `json:"name"`
	Country  string   `json:"country"`
	Logo     string   `json:"logo"`
	BIC      string   `json:"bic"`
	Beta     bool     `json:"beta"`
	Sandbox  bool     `json:"sandbox"`
	PSUTypes []string `json:"psu_types"`
	// MaximumConsentValidity is in seconds and is per-bank (commonly 180
	// days). Asking for longer than this is rejected.
	MaximumConsentValidity int `json:"maximum_consent_validity"`
	// RequiredPSUHeaders is all-or-nothing: send the complete set or none at
	// all. A partial set is a 422 PSU_HEADER_NOT_PROVIDED.
	RequiredPSUHeaders []string     `json:"required_psu_headers"`
	AuthMethods        []AuthMethod `json:"auth_methods"`
}

type AuthMethod struct {
	Name     string `json:"name"`
	Title    string `json:"title"`
	PSUType  string `json:"psu_type"`
	Approach string `json:"approach"` // REDIRECT | DECOUPLED | EMBEDDED
	Hidden   bool   `json:"hidden_method"`
}

type aspspsResponse struct {
	ASPSPs []ASPSP `json:"aspsps"`
}

// Access is the consent being requested. Both Balances and Transactions are
// asked for: Swedbank LT omits the account-holder name from the account list
// without the transactions scope, which degrades identification at exactly
// the moment you are mapping accounts.
type Access struct {
	ValidUntil   string `json:"valid_until"` // RFC3339 with offset
	Balances     bool   `json:"balances,omitempty"`
	Transactions bool   `json:"transactions,omitempty"`
}

type aspspRef struct {
	Name    string `json:"name"`
	Country string `json:"country"`
}

type startAuthRequest struct {
	Access      Access   `json:"access"`
	ASPSP       aspspRef `json:"aspsp"`
	State       string   `json:"state"`
	RedirectURL string   `json:"redirect_url"`
	PSUType     string   `json:"psu_type"` // "personal"
	Language    string   `json:"language,omitempty"`
}

// AuthStart is where to send the browser.
type AuthStart struct {
	URL             string `json:"url"`
	AuthorizationID string `json:"authorization_id"`
	PSUIDHash       string `json:"psu_id_hash"`
}

type authorizeSessionRequest struct {
	Code string `json:"code"`
}

// AccountID carries the account number. IBAN is the only scheme in use here.
type AccountID struct {
	IBAN  string `json:"iban"`
	Other string `json:"other,omitempty"`
}

// Account is one bank account inside an authorised session.
type Account struct {
	UID string `json:"uid"`
	// IdentificationHash is stable across re-authorisation; UID is not.
	IdentificationHash string    `json:"identification_hash"`
	AccountID          AccountID `json:"account_id"`
	Name               string    `json:"name"`
	Details            string    `json:"details"`
	Product            string    `json:"product"`
	Currency           string    `json:"currency"`
	CashAccountType    string    `json:"cash_account_type"`
	Usage              string    `json:"usage"`
}

// Label is the best human name available for an account, in descending order
// of usefulness. Accounts frequently come back with every name field empty,
// which is why the IBAN is the last resort rather than the first choice.
func (a Account) Label() string {
	for _, s := range []string{a.Name, a.Product, a.Details} {
		if t := strings.TrimSpace(s); t != "" {
			return t
		}
	}
	if a.AccountID.IBAN != "" {
		return MaskIBAN(a.AccountID.IBAN)
	}
	return "Account"
}

// Session is an authorised consent plus the accounts it unlocked.
type Session struct {
	SessionID string    `json:"session_id"`
	Accounts  []Account `json:"accounts"`
	ASPSP     aspspRef  `json:"aspsp"`
	PSUType   string    `json:"psu_type"`
	Access    struct {
		ValidUntil string `json:"valid_until"`
	} `json:"access"`
}

// Amount is Enable Banking's money shape. The amount travels as a *string* —
// decimal, never a float on the wire.
type Amount struct {
	Currency string `json:"currency"`
	Amount   string `json:"amount"`
}

// PartyName is a creditor/debtor. Only the name is used.
type PartyName struct {
	Name string `json:"name"`
}

type PartyAccount struct {
	IBAN  string `json:"iban"`
	Other struct {
		Identification string `json:"identification"`
	} `json:"other"`
}

type BankTransactionCode struct {
	Description string `json:"description"`
	Code        string `json:"code"`
	SubCode     string `json:"sub_code"`
}

// Transaction statuses. Only BOOK is staged: a pending row gets a new
// entry_reference when it books, so staging it would stage it twice.
const (
	StatusBooked  = "BOOK"
	StatusPending = "PDNG"
)

// Credit/debit indicators.
const (
	IndicatorCredit = "CRDT"
	IndicatorDebit  = "DBIT"
)

// Transaction is one bank row.
type Transaction struct {
	EntryReference        string              `json:"entry_reference"`
	TransactionID         string              `json:"transaction_id"`
	TransactionAmount     Amount              `json:"transaction_amount"`
	CreditDebitIndicator  string              `json:"credit_debit_indicator"`
	Status                string              `json:"status"`
	BookingDate           string              `json:"booking_date"`     // YYYY-MM-DD
	ValueDate             string              `json:"value_date"`       // YYYY-MM-DD
	TransactionDate       string              `json:"transaction_date"` // YYYY-MM-DD — card purchase date
	Creditor              PartyName           `json:"creditor"`
	Debtor                PartyName           `json:"debtor"`
	CreditorAccount       PartyAccount        `json:"creditor_account"`
	DebtorAccount         PartyAccount        `json:"debtor_account"`
	BankTransactionCode   BankTransactionCode `json:"bank_transaction_code"`
	RemittanceInformation []string            `json:"remittance_information"`
	ReferenceNumber       string              `json:"reference_number"`
	Note                  string              `json:"note"`
	MerchantCategoryCode  string              `json:"merchant_category_code"`
	ExchangeRate          json.RawMessage     `json:"exchange_rate"`
}

type halTransactions struct {
	Transactions    []Transaction `json:"transactions"`
	ContinuationKey string        `json:"continuation_key"`
}

// Provider error codes we branch on. Branching on the *code* and never on the
// HTTP status matters: EXPIRED_SESSION arrives as a 401, and treating that as
// "bad credentials" would mark a live application dead on a transient blip.
const (
	CodeExpiredSession    = "EXPIRED_SESSION"
	CodeSessionNotFound   = "SESSION_NOT_FOUND"
	CodeWrongSessionState = "WRONG_SESSION_STATE"
	CodePSUHeaderMissing  = "PSU_HEADER_NOT_PROVIDED"
)

// APIError is a parsed provider error. The upstream body is never surfaced
// whole: only the `message`/`detail` field is kept, capped at 300 chars.
//
// That is a bound, not a redaction — if the provider puts an internal
// hostname in the first 300 characters of its message, it reaches the error
// string. Accepted here: the alternative is dropping the one field that says
// what actually went wrong, and the audience is the single authenticated
// owner of the instance. The transport-level path is stricter (client.go):
// a dial or TLS failure never carries the URL.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("enable banking %s (http %d): %s", e.Code, e.Status, e.Message)
	}
	return fmt.Sprintf("enable banking http %d: %s", e.Status, e.Message)
}

// Expired reports whether the consent needs re-linking, as opposed to the
// request being wrong. This is the only distinction the UI really needs.
func (e *APIError) Expired() bool {
	switch e.Code {
	case CodeExpiredSession, CodeSessionNotFound, CodeWrongSessionState:
		return true
	}
	return false
}

// RateLimited reports a 429. Enable Banking rate-limits per application.
func (e *APIError) RateLimited() bool { return e.Status == 429 }

// errorEnvelope is deliberately lenient. The documented ErrorResponse schema
// is not pinned down, and different endpoints have been seen to use `code`,
// `error` or `error_code` for the same thing.
type errorEnvelope struct {
	Code      json.RawMessage `json:"code"`
	Error     string          `json:"error"`
	ErrorCode string          `json:"error_code"`
	Message   string          `json:"message"`
	Detail    string          `json:"detail"`
}

// parseAPIError turns a non-2xx body into an APIError, keeping only the fields
// that are safe to show. A body that is not JSON at all (an HTML error page
// from a proxy, say) yields a status-only error rather than a parse failure.
func parseAPIError(status int, body []byte) *APIError {
	e := &APIError{Status: status, Message: "request failed"}
	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return e
	}
	// `code` is a string code on some endpoints and a numeric status on
	// others — only a string is a code worth branching on.
	var codeStr string
	if len(env.Code) > 0 {
		_ = json.Unmarshal(env.Code, &codeStr)
	}
	for _, c := range []string{codeStr, env.ErrorCode, env.Error} {
		if c = strings.TrimSpace(c); c != "" {
			e.Code = c
			break
		}
	}
	for _, m := range []string{env.Message, env.Detail} {
		if m = strings.TrimSpace(m); m != "" {
			e.Message = truncate(m, 300)
			break
		}
	}
	return e
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// MaskIBAN keeps the country prefix and the last four digits. Account numbers
// are shown in the UI and logged on errors; the middle is never needed.
func MaskIBAN(iban string) string {
	s := strings.ReplaceAll(strings.TrimSpace(iban), " ", "")
	if len(s) <= 8 {
		return s
	}
	return s[:4] + "…" + s[len(s)-4:]
}
