package domain

import "time"

// Open-banking (PSD2) import models.
//
// Transactions are pulled from Enable Banking — a licensed AISP aggregator —
// and land in BankStagedTx, NOT in the ledger. Nothing reaches transactions
// until the user ticks the row. That staging step exists because a bulk
// re-import of statement.csv once surfaced ~150 disguised duplicates: rows
// hand-edited or re-categorised since import, so content dedup no longer
// matched them.

// Bank environments. The sandbox needs no real credentials and is where the
// whole flow is rehearsed before a real bank is linked.
const (
	BankEnvSandbox    = "sandbox"
	BankEnvProduction = "production"
)

// Connection lifecycle. A connection is pending between minting the auth URL
// and exchanging the code; expired/revoked both mean "re-link", and are kept
// apart only so the UI can word the prompt honestly.
const (
	BankConnPending    = "pending"
	BankConnAuthorized = "authorized"
	BankConnExpired    = "expired"
	BankConnRevoked    = "revoked"
)

// Staged-row verdicts, assigned at stage time and re-verified inside the
// commit transaction (the review list may have been open for an hour).
const (
	// VerdictNew — no match anywhere. Pre-ticked.
	VerdictNew = "new"
	// VerdictDuplicateExact — provider id already on a ledger row. Certain,
	// so auto-hidden.
	VerdictDuplicateExact = "duplicate_exact"
	// VerdictDuplicateContent — date+type+amount+comment matched an existing
	// row. A heuristic: this is the ~150-duplicate class, and also what fires
	// on a hand-edited comment. Shown with its reason, left unticked.
	VerdictDuplicateContent = "duplicate_content"
	// VerdictInternal — the classifier returned skip (own-account noise).
	VerdictInternal = "internal"
	// VerdictNeedsReview — non-EUR, or an unparseable credit/debit indicator.
	// Staged rather than dropped: a row we cannot read is still a row.
	VerdictNeedsReview = "needs_review"
	// VerdictPending — a card reservation the bank has authorised but not
	// booked. The money is effectively gone, so it is worth seeing days
	// early, but the amount can still move (fuel pre-auths, tips, FX) and
	// the bank re-issues the row under a new entry_reference once it books.
	// Shown and editable, never committable: see SupersededBy.
	VerdictPending = "pending"
)

// Staged-row lifecycle. A dismissed row stays dismissed forever — that
// persistence *is* the dismissal ledger, which is what stops a re-sync from
// re-offering a row the user already rejected.
const (
	StagedStateStaged    = "staged"
	StagedStateImported  = "imported"
	StagedStateDismissed = "dismissed"
	// StagedStateSuperseded — a reservation that has resolved: the bank
	// booked it (and the booked row carries the corrections made here), or
	// released it without ever booking. Kept rather than deleted, because
	// "where did that row go" deserves an answer.
	StagedStateSuperseded = "superseded"
)

// ValidAccountKeys are the account codes the balance sheet understands. This
// mirrors the switch in service.applyAccountDelta; the two must not drift, so
// the mapping UI and the commit validator both read this list.
var ValidAccountKeys = []string{
	"seb", "swed", "swed_etf", "seb_pen", "luminor",
	"art", "rev_m", "rev_r", "rev_stocks", "ibkr_stocks", "cash",
}

// IsValidAccountKey reports whether key names a real balance-sheet account.
func IsValidAccountKey(key string) bool {
	for _, k := range ValidAccountKeys {
		if k == key {
			return true
		}
	}
	return false
}

// BankSettings holds the Enable Banking application credentials. Singleton
// (ID=1), mirroring AISettings.
type BankSettings struct {
	ID uint `json:"id" gorm:"primaryKey"`
	// ApplicationID travels as the JWT `kid` header on every API call.
	ApplicationID string `json:"-" gorm:"not null;default:''"`
	// PrivateKeyPEM signs the RS256 JWT. Never serialised to JSON, never
	// included in the data export — unlike the AI key, which the backup
	// deliberately embeds. A long-lived bank credential in a downloadable
	// file is materially worse.
	PrivateKeyPEM string `json:"-" gorm:"not null;default:''"`
	// Environment is sandbox|production. Sandbox is the default so a
	// half-configured install cannot touch a real bank.
	Environment string `json:"environment" gorm:"not null;default:'sandbox'"`
	// RedirectURL must byte-match the URL registered with Enable Banking —
	// scheme, port and trailing slash included.
	RedirectURL string    `json:"redirect_url" gorm:"not null;default:''"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Configured reports whether a sync could even be attempted.
func (s *BankSettings) Configured() bool {
	return s.ApplicationID != "" && s.PrivateKeyPEM != ""
}

// HasKey is what the API exposes instead of the PEM itself.
func (s *BankSettings) HasKey() bool { return s.PrivateKeyPEM != "" }

// BankConnection is one authorised consent with one bank.
type BankConnection struct {
	ID uint `json:"id" gorm:"primaryKey"`
	// Enable Banking keys banks by name+country; there is no stable id.
	ASPSPName    string `json:"aspsp_name" gorm:"not null;default:''"`
	ASPSPCountry string `json:"aspsp_country" gorm:"not null;default:'LT'"`
	// SessionID is the credential that reads your accounts — treated like one.
	SessionID  string    `json:"-" gorm:"not null;default:''"`
	Status     string    `json:"status" gorm:"not null;default:'pending'"`
	ValidUntil time.Time `json:"valid_until"`
	// AuthState is a single-use CSRF nonce with a 15-minute TTL. It guards a
	// replayed `code` independently of the session.
	AuthState   string    `json:"-" gorm:"index;default:''"`
	AuthStateAt time.Time `json:"-"`
	// LastError carries the parsed provider error code, never a reflected
	// upstream body.
	LastError string    `json:"last_error" gorm:"not null;default:''"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AuthStateTTL bounds how long a minted auth nonce stays usable.
const AuthStateTTL = 15 * time.Minute

// BankAccountLink maps one bank account to one balance-sheet account key.
type BankAccountLink struct {
	ID           uint `json:"id" gorm:"primaryKey"`
	ConnectionID uint `json:"connection_id" gorm:"index"`
	// IdentificationHash is stable across re-authorisation; UID is NOT — it
	// rotates every time consent is renewed. Re-matching on reconnect keys on
	// the hash, or the mapping silently detaches.
	IdentificationHash string `json:"-" gorm:"index"`
	UID                string `json:"-"`
	IBAN               string `json:"iban"`
	DisplayName        string `json:"display_name"`
	// AccountKey is the balance-sheet account this feeds ("swed", "seb"), or
	// "" meaning do not sync this account.
	AccountKey   string     `json:"account_key" gorm:"not null;default:''"`
	LastSyncedAt *time.Time `json:"last_synced_at"`
	// LastTxDate is the newest booking date seen, and seeds the next sync
	// window. Preferred over LastSyncedAt: a sync that returned nothing must
	// not advance the window past rows that had not posted yet.
	LastTxDate *time.Time `json:"last_tx_date"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Synced reports whether this account is mapped and should be pulled.
func (l *BankAccountLink) Synced() bool { return l.AccountKey != "" }

// BankStagedTx is one fetched bank row awaiting the user's verdict.
type BankStagedTx struct {
	ID     uint `json:"id" gorm:"primaryKey"`
	LinkID uint `json:"link_id" gorm:"index"`
	// ExternalID is "eb:<linkID>:<entry_reference>" — the upsert key, and
	// what layer-1 dedup matches against transactions.external_id.
	ExternalID string `json:"external_id" gorm:"uniqueIndex"`

	// Raw provider fields: the audit trail, and the input to re-classification
	// if the classifier is ever improved.
	Raw         string    `json:"-" gorm:"type:text"`
	RawPayee    string    `json:"raw_payee"`
	RawDetails  string    `json:"raw_details"`
	RawAmount   float64   `json:"raw_amount"`
	RawDK       string    `json:"raw_dk"` // "D" | "K"
	RawCurrency string    `json:"raw_currency"`
	BookingDate time.Time `json:"booking_date"`

	// Classifier output — editable before commit. Amount is kept separate
	// from RawAmount because the classifier (and the user) may adjust it,
	// e.g. a foreign-currency row converted by hand, while RawAmount stays
	// the audit trail of what the bank actually said.
	Amount        float64         `json:"amount"`
	Date          time.Time       `json:"date"`
	Type          TransactionType `json:"type"`
	Category      Category        `json:"category"`
	Comment       string          `json:"comment"`
	Labels        string          `json:"labels" gorm:"not null;default:''"`
	DebitAccount  string          `json:"debit_account" gorm:"not null;default:''"`
	CreditAccount string          `json:"credit_account" gorm:"not null;default:''"`

	// EnrichNote says where a non-obvious proposal came from, in the user's
	// words: "category from 7 similar transactions", "labels from your
	// rules". The classifier's hardcoded guesses are silent; only the
	// learned signals announce themselves, because those are the ones worth
	// double-checking.
	EnrichNote string `json:"enrich_note" gorm:"not null;default:''"`

	// Edited records that a person changed this proposal. It is what lets a
	// re-sync re-run an improved classifier over rows already in the queue
	// without overwriting anybody's corrections.
	Edited bool `json:"edited" gorm:"not null;default:0"`

	// Pending reports that the bank had only reserved this amount, not booked
	// it. Stored rather than derived from Verdict, which the user's edits and
	// the dedup pass both rewrite.
	Pending bool `json:"pending" gorm:"not null;default:0"`
	// SupersededBy names the staged row that replaced this reservation once
	// the bank booked it. Empty on a reservation the bank simply released.
	SupersededBy *uint `json:"superseded_by"`

	// CategoryGuessed marks a row whose category is the classifier's generic
	// fallback rather than a rule hit — the signal enrichment keys on.
	// Transient: it travels from the adapter to the enricher inside one sync
	// and is never stored or serialised.
	CategoryGuessed bool `json:"-" gorm:"-"`

	// Verdict + lifecycle.
	Verdict string `json:"verdict" gorm:"index"`
	// VerdictNote names the match in words the user can act on, e.g.
	// "looks like #1234 · 2026-09-12 · Lidl · 23.40".
	VerdictNote  string `json:"verdict_note" gorm:"not null;default:''"`
	MatchedTxID  *uint  `json:"matched_tx_id"`
	State        string `json:"state" gorm:"index"`
	ImportedTxID *uint  `json:"imported_tx_id"`

	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Preticked reports whether this row should arrive with its checkbox ticked.
// Only an unambiguously new row does; every duplicate verdict, every internal
// row, everything needing review and every unbooked reservation starts
// unticked.
func (t *BankStagedTx) Preticked() bool {
	return t.State == StagedStateStaged && t.Verdict == VerdictNew && !t.Pending
}

// Committable reports whether this row may become a transaction.
//
// A reservation may not. Its amount is not final — a fuel pre-auth is a round
// number, a restaurant adds the tip after you leave — and the bank re-issues
// it under a new entry_reference when it books, which neither dedup layer
// would match. Committing one buys a few days of accuracy and pays for it
// with a wrong amount and a duplicate.
func (t *BankStagedTx) Committable() bool {
	return t.State == StagedStateStaged && !t.Pending
}
