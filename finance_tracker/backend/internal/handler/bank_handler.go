package handler

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/openbanking"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/mindaugas/finance-tracker/internal/service"
	"gorm.io/gorm"
)

// BankHandler owns the PSD2 open-banking surface: credentials, bank consent,
// account mapping, and the staging list that sits between a fetched bank row
// and the ledger.
//
// Nothing this handler fetches is written to transactions. A sync stages
// rows; only an explicit commit creates a transaction. That separation is the
// whole point of the feature — a bulk CSV re-import once produced ~150
// disguised duplicates, and there was no preview to catch them.
type BankHandler struct {
	repo   repository.BankRepository
	txRepo repository.TransactionRepository
	// db is used only to wrap a commit in one transaction, so the created
	// rows and the staged-row state changes land together. Optional: see
	// WithDB.
	db *gorm.DB
	// consentDays is how long a consent is requested for, capped per-bank by
	// the ASPSP's maximum_consent_validity.
	consentDays int
	// labeling supplies the learned signals a hand-typed transaction gets —
	// the user's label rules and the category similar rows were filed under.
	// Optional: see WithLabeling in bank_enrich.go.
	labeling labelingSource
	// balances moves the balance sheet when rows are committed. Built per
	// database handle so the snapshot lands inside the commit's transaction;
	// nil disables it (unit tests on fake repositories).
	balances BalanceSnapshotter
}

// BalanceSnapshotter applies dated per-account movements as a single new
// balance snapshot. service.BalanceService satisfies it.
type BalanceSnapshotter interface {
	SnapshotFromDeltas(deltas []service.AccountDelta) (string, error)
}

func NewBankHandler(repo repository.BankRepository, txRepo repository.TransactionRepository) *BankHandler {
	return &BankHandler{repo: repo, txRepo: txRepo, consentDays: 180}
}

func (h *BankHandler) RegisterRoutes(rg *gin.RouterGroup) {
	// Settings stay reachable with no credentials stored — they are how the
	// credentials get there in the first place.
	s := rg.Group("/banking")
	s.GET("/settings", h.GetSettings)
	s.PUT("/settings", h.SaveSettings)

	// Everything that talks to the provider needs credentials to exist.
	g := rg.Group("/banking", h.requireConfigured)
	g.GET("/aspsps", h.ListASPSPs)
	g.GET("/connections", h.ListConnections)
	g.POST("/connections", h.CreateConnection)
	g.POST("/connections/callback", h.Callback)
	g.DELETE("/connections/:id", h.DeleteConnection)
	g.PUT("/accounts/:id", h.UpdateAccountLink)
	g.POST("/accounts/:id/sync", h.SyncAccount)
	// Sync every mapped account at once — what the review queue on the
	// transactions page presses.
	g.POST("/sync", h.SyncAll)
	g.GET("/staged", h.ListStaged)
	g.PUT("/staged/:id", h.UpdateStaged)
	g.POST("/staged/commit", h.CommitStaged)
	g.POST("/staged/dismiss", h.DismissStaged)
	g.POST("/staged/restore", h.RestoreStaged)
	// Link a bank row to a transaction already entered by hand, and back out
	// again — a merge is a judgement call and has to be reversible.
	g.POST("/staged/:id/merge", h.MergeStaged)
	g.POST("/staged/:id/unmerge", h.UnmergeStaged)
}

// requireConfigured hides the whole feature until credentials are stored.
// 404 rather than 403, same reasoning as the AI switch: with no application
// registered there is nothing here to forbid.
func (h *BankHandler) requireConfigured(c *gin.Context) {
	s, err := h.repo.GetSettings()
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !s.Configured() {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error": "bank connections are not configured — add your Enable Banking application in bank settings",
		})
		return
	}
	c.Next()
}

// client builds an API client from the stored credentials.
func (h *BankHandler) client() (*openbanking.Client, *domain.BankSettings, error) {
	s, err := h.repo.GetSettings()
	if err != nil {
		return nil, nil, err
	}
	if !s.Configured() {
		return nil, nil, errors.New("bank connections are not configured")
	}
	cl, err := openbanking.New(s.ApplicationID, s.PrivateKeyPEM)
	if err != nil {
		return nil, nil, err
	}
	return cl, s, nil
}

// ── settings ────────────────────────────────────────────────────────

type bankSettingsResponse struct {
	Environment string `json:"environment"`
	RedirectURL string `json:"redirect_url"`
	// The application id is shown back (it is an identifier, not a secret);
	// the private key never is — only whether one is stored.
	ApplicationID string    `json:"application_id"`
	HasKey        bool      `json:"has_key"`
	Configured    bool      `json:"configured"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func toBankSettingsResponse(s *domain.BankSettings) bankSettingsResponse {
	return bankSettingsResponse{
		Environment:   s.Environment,
		RedirectURL:   s.RedirectURL,
		ApplicationID: s.ApplicationID,
		HasKey:        s.HasKey(),
		Configured:    s.Configured(),
		UpdatedAt:     s.UpdatedAt,
	}
}

func (h *BankHandler) GetSettings(c *gin.Context) {
	s, err := h.repo.GetSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toBankSettingsResponse(s))
}

func (h *BankHandler) SaveSettings(c *gin.Context) {
	var in struct {
		ApplicationID *string `json:"application_id"`
		// PrivateKeyPEM blank means "keep what is stored" — the UI never
		// receives the key, so it cannot echo it back on an unrelated edit.
		PrivateKeyPEM string  `json:"private_key_pem"`
		ClearKey      bool    `json:"clear_key"`
		Environment   *string `json:"environment"`
		RedirectURL   *string `json:"redirect_url"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	s, err := h.repo.GetSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if in.ApplicationID != nil {
		s.ApplicationID = strings.TrimSpace(*in.ApplicationID)
	}
	if in.Environment != nil {
		switch env := strings.ToLower(strings.TrimSpace(*in.Environment)); env {
		case domain.BankEnvSandbox, domain.BankEnvProduction:
			s.Environment = env
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unknown environment %q — use %q or %q", env, domain.BankEnvSandbox, domain.BankEnvProduction)})
			return
		}
	}
	if in.RedirectURL != nil {
		s.RedirectURL = strings.TrimSpace(*in.RedirectURL)
	}
	switch {
	case in.ClearKey:
		s.PrivateKeyPEM = ""
	case strings.TrimSpace(in.PrivateKeyPEM) != "":
		pem := strings.TrimSpace(in.PrivateKeyPEM)
		// Validate before storing: a key that cannot be parsed would
		// otherwise fail at the first API call with a confusing error.
		if _, err := openbanking.New(orPlaceholder(s.ApplicationID), pem); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		s.PrivateKeyPEM = pem
	}
	if err := h.repo.SaveSettings(s); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toBankSettingsResponse(s))
}

// orPlaceholder lets the key be validated before an application id is saved —
// openbanking.New rejects an empty id, which is not what is being checked here.
func orPlaceholder(id string) string {
	if id == "" {
		return "pending"
	}
	return id
}

// ── banks ───────────────────────────────────────────────────────────

type aspspResponse struct {
	Name     string `json:"name"`
	Country  string `json:"country"`
	Logo     string `json:"logo"`
	Beta     bool   `json:"beta"`
	Sandbox  bool   `json:"sandbox"`
	MaxDays  int    `json:"max_consent_days"`
	Redirect bool   `json:"redirect"`
}

func (h *BankHandler) ListASPSPs(c *gin.Context) {
	cl, _, err := h.client()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	country := strings.ToUpper(strings.TrimSpace(c.Query("country")))
	if country == "" {
		country = "LT"
	}
	list, err := cl.ASPSPs(c.Request.Context(), country)
	if err != nil {
		writeProviderError(c, err)
		return
	}
	out := make([]aspspResponse, 0, len(list))
	for _, a := range list {
		out = append(out, aspspResponse{
			Name: a.Name, Country: a.Country, Logo: a.Logo,
			Beta: bool(a.Beta), Sandbox: bool(a.Sandbox),
			MaxDays:  a.MaximumConsentValidity / 86400,
			Redirect: hasRedirectAuth(a),
		})
	}
	c.JSON(http.StatusOK, gin.H{"aspsps": out})
}

// hasRedirectAuth reports whether the bank offers a browser-redirect flow for
// a personal user. The decoupled and embedded approaches need a different UI
// and are not supported here.
func hasRedirectAuth(a openbanking.ASPSP) bool {
	for _, m := range a.AuthMethods {
		if m.Approach == "REDIRECT" && !m.Hidden {
			return true
		}
	}
	return false
}

// ── connections ─────────────────────────────────────────────────────

type accountLinkResponse struct {
	ID           uint       `json:"id"`
	ConnectionID uint       `json:"connection_id"`
	IBAN         string     `json:"iban"` // masked
	DisplayName  string     `json:"display_name"`
	AccountKey   string     `json:"account_key"`
	LastSyncedAt *time.Time `json:"last_synced_at"`
	LastTxDate   *time.Time `json:"last_tx_date"`
	Pending      int        `json:"pending"`
	// BankBalance is what the bank last stated; nil until it has said.
	BankBalance         *float64   `json:"bank_balance"`
	BankBalanceCurrency string     `json:"bank_balance_currency,omitempty"`
	BankBalanceAt       *time.Time `json:"bank_balance_at"`
}

type connectionResponse struct {
	ID              uint                  `json:"id"`
	ASPSPName       string                `json:"aspsp_name"`
	ASPSPCountry    string                `json:"aspsp_country"`
	Status          string                `json:"status"`
	ValidUntil      time.Time             `json:"valid_until"`
	DaysUntilExpiry int                   `json:"days_until_expiry"`
	LastError       string                `json:"last_error,omitempty"`
	Accounts        []accountLinkResponse `json:"accounts"`
}

func (h *BankHandler) ListConnections(c *gin.Context) {
	conns, err := h.repo.ListConnections()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	pending, err := h.repo.CountPendingByLink()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]connectionResponse, 0, len(conns))
	for i := range conns {
		conn := conns[i]
		links, err := h.repo.ListLinks(conn.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		accs := make([]accountLinkResponse, 0, len(links))
		for _, l := range links {
			acc := accountLinkResponse{
				ID: l.ID, ConnectionID: l.ConnectionID,
				IBAN:        openbanking.MaskIBAN(l.IBAN),
				DisplayName: l.DisplayName, AccountKey: l.AccountKey,
				LastSyncedAt: l.LastSyncedAt, LastTxDate: l.LastTxDate,
				Pending: pending[l.ID],
			}
			if l.BankBalanceFetched != nil {
				bal := l.BankBalance
				acc.BankBalance = &bal
				acc.BankBalanceCurrency = l.BankBalanceCurrency
				acc.BankBalanceAt = l.BankBalanceFetched
			}
			accs = append(accs, acc)
		}
		out = append(out, connectionResponse{
			ID: conn.ID, ASPSPName: conn.ASPSPName, ASPSPCountry: conn.ASPSPCountry,
			Status: effectiveStatus(&conn), ValidUntil: conn.ValidUntil,
			// Expiry is computed on read. There is no background loop, so
			// nothing else would ever notice a consent going stale.
			DaysUntilExpiry: daysUntil(conn.ValidUntil),
			LastError:       conn.LastError,
			Accounts:        accs,
		})
	}
	c.JSON(http.StatusOK, gin.H{"connections": out, "valid_account_keys": domain.ValidAccountKeys})
}

// effectiveStatus reports a consent past its validity as expired even if
// nothing has tried to use it since.
func effectiveStatus(c *domain.BankConnection) string {
	if c.Status == domain.BankConnAuthorized && !c.ValidUntil.IsZero() && time.Now().After(c.ValidUntil) {
		return domain.BankConnExpired
	}
	return c.Status
}

func daysUntil(t time.Time) int {
	if t.IsZero() {
		return 0
	}
	return int(time.Until(t).Hours() / 24)
}

func (h *BankHandler) CreateConnection(c *gin.Context) {
	var in struct {
		ASPSPName string `json:"aspsp_name" binding:"required"`
		Country   string `json:"country"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cl, settings, err := h.client()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if settings.RedirectURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "set the redirect URL in bank settings first — it must match the one registered with Enable Banking exactly"})
		return
	}
	country := strings.ToUpper(strings.TrimSpace(in.Country))
	if country == "" {
		country = "LT"
	}

	// Look the bank up to honour its consent cap and its PSU-header
	// requirements — both are per-bank and guessing either one is a 422.
	banks, err := cl.ASPSPs(c.Request.Context(), country)
	if err != nil {
		writeProviderError(c, err)
		return
	}
	var bank *openbanking.ASPSP
	for i := range banks {
		if strings.EqualFold(banks[i].Name, in.ASPSPName) {
			bank = &banks[i]
			break
		}
	}
	if bank == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("bank %q is not available in %s for this application", in.ASPSPName, country)})
		return
	}

	validUntil := time.Now().Add(time.Duration(h.consentDays) * 24 * time.Hour)
	if bank.MaximumConsentValidity > 0 {
		if cap := time.Now().Add(time.Duration(bank.MaximumConsentValidity) * time.Second); cap.Before(validUntil) {
			// Step just inside the cap: a request landing exactly on it is
			// rejected by some banks for clock skew.
			validUntil = cap.Add(-1 * time.Minute)
		}
	}

	state, err := newAuthState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	conn := &domain.BankConnection{
		ASPSPName: bank.Name, ASPSPCountry: bank.Country,
		Status: domain.BankConnPending, ValidUntil: validUntil,
		AuthState: state, AuthStateAt: time.Now(),
	}
	if err := h.repo.SaveConnection(conn); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	start, err := cl.StartAuth(c.Request.Context(), openbanking.AuthParams{
		ASPSPName: bank.Name, ASPSPCountry: bank.Country,
		State: state, RedirectURL: settings.RedirectURL, ValidUntil: validUntil,
		PSU:                psuFrom(c),
		RequiredPSUHeaders: bank.RequiredPSUHeaders,
	})
	if err != nil {
		conn.Status = domain.BankConnRevoked
		conn.LastError = providerMessage(err)
		_ = h.repo.SaveConnection(conn)
		writeProviderError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"connection_id": conn.ID, "url": start.URL, "state": state})
}

// psuFrom builds the end-user headers. Behind the DuckDNS terminator nginx
// sets X-Real-IP but not X-Forwarded-For, so ClientIP() is the terminator's
// address rather than the user's. Every sync here is a button press, so this
// is cosmetic — but it is what the bank is told, so it is sent honestly
// rather than fabricated.
func psuFrom(c *gin.Context) openbanking.PSU {
	return openbanking.PSU{
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}
}

func newAuthState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// Callback exchanges the authorisation code for a session. It is called by
// the SPA, not by the bank: the redirect is a browser navigation, so the code
// arrives in the user's browser and is POSTed back from there. That is why
// this route sits behind auth like everything else — no public endpoint is
// needed anywhere.
func (h *BankHandler) Callback(c *gin.Context) {
	var in struct {
		Code  string `json:"code"`
		State string `json:"state"`
		// URL is the manual-paste fallback: the whole address the browser
		// landed on. This is the guaranteed path when the auto-redirect
		// cannot reach the app (Tailscale, LAN-only).
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	code, state := strings.TrimSpace(in.Code), strings.TrimSpace(in.State)
	if raw := strings.TrimSpace(in.URL); raw != "" {
		pc, ps, perr := parseCallbackURL(raw)
		if perr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": perr.Error()})
			return
		}
		code, state = pc, ps
	}
	if code == "" || state == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the address is missing the code or state parameter — paste the full address you landed on"})
		return
	}

	conn, err := h.repo.GetConnectionByState(state)
	if err != nil {
		// Wrong, reused or expired nonce. Deliberately one message for all
		// three: distinguishing them tells an attacker which guess was close.
		c.JSON(http.StatusBadRequest, gin.H{"error": "this authorisation link has expired or was already used — start the connection again"})
		return
	}
	// Burn the nonce before the exchange. A replayed code then finds no
	// connection to attach to, whether or not the exchange below succeeds.
	conn.AuthState = ""
	if err := h.repo.SaveConnection(conn); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	cl, _, err := h.client()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sess, err := cl.AuthorizeSession(c.Request.Context(), code)
	if err != nil {
		conn.LastError = providerMessage(err)
		_ = h.repo.SaveConnection(conn)
		writeProviderError(c, err)
		return
	}

	conn.SessionID = sess.SessionID
	conn.Status = domain.BankConnAuthorized
	conn.LastError = ""
	if sess.ASPSP.Name != "" {
		conn.ASPSPName = sess.ASPSP.Name
	}
	if vu, err := time.Parse(time.RFC3339, sess.Access.ValidUntil); err == nil {
		conn.ValidUntil = vu
	}
	if err := h.repo.SaveConnection(conn); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	linked, err := h.syncAccountLinks(conn.ID, sess.Accounts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"connection_id": conn.ID, "accounts": linked})
}

// parseCallbackURL pulls code+state out of a pasted redirect address. The app
// is a HashRouter, so the query sits before the "#" — but a hand-copied
// address can carry it either side, and both are accepted.
func parseCallbackURL(raw string) (code, state string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", errors.New("that does not look like a web address")
	}
	q := u.Query()
	if e := q.Get("error"); e != "" {
		// The bank reports refusal on the redirect, not as an API error.
		if d := q.Get("error_description"); d != "" {
			return "", "", fmt.Errorf("the bank refused the connection: %s", d)
		}
		return "", "", fmt.Errorf("the bank refused the connection (%s)", e)
	}
	code, state = q.Get("code"), q.Get("state")
	if code == "" && u.Fragment != "" {
		// Query after the fragment: "…/#/banking?code=…".
		if i := strings.IndexByte(u.Fragment, '?'); i >= 0 {
			if fq, perr := url.ParseQuery(u.Fragment[i+1:]); perr == nil {
				code, state = fq.Get("code"), fq.Get("state")
			}
		}
	}
	return code, state, nil
}

// syncAccountLinks reconciles the accounts an authorised session unlocked
// against the links already stored. Matching is on identification_hash:
// the uid rotates on every re-authorisation, so a uid match would create a
// duplicate link and silently orphan the account mapping on every reconnect.
func (h *BankHandler) syncAccountLinks(connID uint, accounts []openbanking.Account) ([]accountLinkResponse, error) {
	out := make([]accountLinkResponse, 0, len(accounts))
	for _, a := range accounts {
		link, err := h.repo.GetLinkByHash(connID, a.IdentificationHash)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			link = &domain.BankAccountLink{
				ConnectionID:       connID,
				IdentificationHash: a.IdentificationHash,
			}
		} else if err != nil {
			return nil, err
		}
		link.UID = a.UID
		link.IBAN = a.AccountID.IBAN
		link.DisplayName = a.Label()
		if err := h.repo.SaveLink(link); err != nil {
			return nil, err
		}
		out = append(out, accountLinkResponse{
			ID: link.ID, ConnectionID: connID,
			IBAN:        openbanking.MaskIBAN(link.IBAN),
			DisplayName: link.DisplayName, AccountKey: link.AccountKey,
			LastSyncedAt: link.LastSyncedAt, LastTxDate: link.LastTxDate,
		})
	}
	return out, nil
}

func (h *BankHandler) DeleteConnection(c *gin.Context) {
	id, err := uintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	conn, err := h.repo.GetConnection(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
		return
	}
	// Revoke upstream too, best-effort: leaving a live consent behind after
	// the user pressed Disconnect would be dishonest. A failure here must not
	// block the local delete, or a dead session becomes undeletable.
	if conn.SessionID != "" {
		if cl, _, cerr := h.client(); cerr == nil {
			_ = cl.DeleteSession(c.Request.Context(), conn.SessionID)
		}
	}
	if err := h.repo.DeleteConnection(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// UpdateAccountLink maps one bank account onto a balance-sheet account, or
// unmaps it with an empty key.
func (h *BankHandler) UpdateAccountLink(c *gin.Context) {
	id, err := uintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var in struct {
		AccountKey  string `json:"account_key"`
		DisplayName string `json:"display_name"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	key := strings.TrimSpace(in.AccountKey)
	if key != "" && !domain.IsValidAccountKey(key) {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unknown account %q — expected one of %s", key, strings.Join(domain.ValidAccountKeys, ", "))})
		return
	}
	link, err := h.repo.GetLink(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
		return
	}
	link.AccountKey = key
	if n := strings.TrimSpace(in.DisplayName); n != "" {
		link.DisplayName = n
	}
	if err := h.repo.SaveLink(link); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, accountLinkResponse{
		ID: link.ID, ConnectionID: link.ConnectionID,
		IBAN:        openbanking.MaskIBAN(link.IBAN),
		DisplayName: link.DisplayName, AccountKey: link.AccountKey,
		LastSyncedAt: link.LastSyncedAt, LastTxDate: link.LastTxDate,
	})
}

// ── shared helpers ──────────────────────────────────────────────────

func uintParam(c *gin.Context, name string) (uint, error) {
	v, err := strconv.ParseUint(c.Param(name), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid %s", name)
	}
	return uint(v), nil
}

// writeProviderError maps a provider failure onto an HTTP status and a
// message worth showing. Branching is on the error *code*, never the status:
// EXPIRED_SESSION arrives as a 401 and means "re-link", not "bad credentials".
func writeProviderError(c *gin.Context, err error) {
	var apiErr *openbanking.APIError
	if !errors.As(err, &apiErr) {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	switch {
	case apiErr.Expired():
		c.JSON(http.StatusConflict, gin.H{
			"error":   providerReason(err),
			"code":    apiErr.Code,
			"expired": true,
		})
	case apiErr.RateLimited():
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": providerReason(err),
			"code":  apiErr.Code,
		})
	default:
		c.JSON(http.StatusBadGateway, gin.H{"error": providerReason(err), "code": apiErr.Code})
	}
}

// providerReason is the sentence a user reads. providerMessage is the terse
// code persisted on the connection; this is its counterpart for the screen,
// shared so a failure worded one way in a single sync is not worded another
// way in the Sync all report.
func providerReason(err error) string {
	var apiErr *openbanking.APIError
	if !errors.As(err, &apiErr) {
		return err.Error()
	}
	switch {
	case apiErr.Expired():
		return "the connection to this bank has expired — reconnect it"
	case apiErr.RateLimited():
		return "the bank is rate-limiting requests — try again in a few minutes"
	}
	return apiErr.Error()
}

// providerMessage is what gets persisted on the connection. Short, parsed,
// and never the raw upstream body.
func providerMessage(err error) string {
	var apiErr *openbanking.APIError
	if errors.As(err, &apiErr) {
		if apiErr.Code != "" {
			return apiErr.Code
		}
		return fmt.Sprintf("http %d", apiErr.Status)
	}
	return "request failed"
}
