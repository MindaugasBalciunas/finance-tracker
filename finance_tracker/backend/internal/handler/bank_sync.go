package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/openbanking"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"gorm.io/gorm"
)

// Sync windows.
const (
	// firstSyncDays is how far back the first sync for an account reaches.
	// 90 days is both what the LT banks serve without a second SCA and
	// enough to close the gap left by the last CSV import.
	firstSyncDays = 90
	// resyncOverlapDays re-reads a week either side of where the last sync
	// finished, to catch card rows that post late. Free: a row seen again
	// upserts on its external id rather than duplicating.
	resyncOverlapDays = 7
)

// WithDB makes commits atomic: the transaction rows and the staged-row state
// changes land together or not at all. Without a DB handle (unit tests with
// fakes) the commit runs non-atomically against the handler's repositories.
func (h *BankHandler) WithDB(db *gorm.DB) *BankHandler {
	h.db = db
	return h
}

type syncResult struct {
	Fetched          int    `json:"fetched"`
	StagedNew        int    `json:"staged_new"`
	Unchanged        int    `json:"unchanged"`
	AutoSkipped      int    `json:"auto_skipped"`
	DuplicateExact   int    `json:"duplicate_exact"`
	DuplicateContent int    `json:"duplicate_content"`
	NeedsReview      int    `json:"needs_review"`
	Internal         int    `json:"internal"`
	DateFrom         string `json:"date_from"`
	DateTo           string `json:"date_to"`
}

// SyncAccount pulls recent transactions for one mapped account and stages
// them. Synchronous on purpose: a 90-day window over one personal account is
// a page or two, nowhere near the server's 180s write timeout, and a button
// press that answers inline needs no job table, no polling and no background
// goroutine that can fail silently.
//
// Nothing here writes to the ledger.
func (h *BankHandler) SyncAccount(c *gin.Context) {
	id, err := uintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	link, err := h.repo.GetLink(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
		return
	}
	if !link.Synced() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "map this account to one of your accounts first — otherwise there is nowhere to file its transactions"})
		return
	}
	if link.UID == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "this account has no active session — reconnect the bank", "expired": true})
		return
	}
	conn, err := h.repo.GetConnection(link.ConnectionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
		return
	}
	if effectiveStatus(conn) != domain.BankConnAuthorized {
		c.JSON(http.StatusConflict, gin.H{"error": "the connection to this bank has expired — reconnect it", "expired": true})
		return
	}
	cl, _, err := h.client()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Window: 90 days on a first sync (there is real history to close), a
	// 7-day overlap afterwards.
	now := time.Now()
	from := now.AddDate(0, 0, -firstSyncDays)
	if anchor := syncAnchor(link); !anchor.IsZero() {
		if candidate := anchor.AddDate(0, 0, -resyncOverlapDays); candidate.After(from) {
			from = candidate
		}
	}
	// Allow an explicit narrower window: the first real-data run is meant to
	// be a cautious 7-day look before the full 90.
	if d, derr := strconv.Atoi(c.Query("days")); derr == nil && d > 0 && d <= 730 {
		from = now.AddDate(0, 0, -d)
	}

	txs, err := cl.AllTransactions(c.Request.Context(), openbanking.TxQuery{
		AccountUID:         link.UID,
		DateFrom:           from,
		DateTo:             now,
		PSU:                psuFrom(c),
		RequiredPSUHeaders: h.requiredPSUHeaders(c, cl, conn),
	})
	if err != nil {
		// AllTransactions returns what it collected alongside the error, but
		// a partial window would make "staged_new == 0" meaningless on the
		// next run. Report the failure and stage nothing.
		var apiErr *openbanking.APIError
		if errors.As(err, &apiErr) && apiErr.Expired() {
			conn.Status = domain.BankConnExpired
			conn.LastError = apiErr.Code
			_ = h.repo.SaveConnection(conn)
		}
		writeProviderError(c, err)
		return
	}

	res, err := h.stage(txs, link, from, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// syncAnchor is where the next window starts from. LastTxDate is preferred
// over LastSyncedAt: a sync that returned nothing must not advance the window
// past rows that simply had not posted yet.
func syncAnchor(l *domain.BankAccountLink) time.Time {
	if l.LastTxDate != nil && !l.LastTxDate.IsZero() {
		return *l.LastTxDate
	}
	if l.LastSyncedAt != nil && !l.LastSyncedAt.IsZero() {
		return *l.LastSyncedAt
	}
	return time.Time{}
}

// requiredPSUHeaders looks up what this bank demands. The headers are
// all-or-nothing — a partial set is a 422 — and the client drops the lot if
// any one is missing. A failed lookup is not fatal: the unattended path
// (no headers at all) is the documented fallback.
func (h *BankHandler) requiredPSUHeaders(c *gin.Context, cl *openbanking.Client, conn *domain.BankConnection) []string {
	banks, err := cl.ASPSPs(c.Request.Context(), conn.ASPSPCountry)
	if err != nil {
		return nil
	}
	for _, b := range banks {
		if strings.EqualFold(b.Name, conn.ASPSPName) {
			return b.RequiredPSUHeaders
		}
	}
	return nil
}

// stage converts provider rows into staged rows, assigns verdicts and
// upserts by external id.
func (h *BankHandler) stage(txs []openbanking.Transaction, link *domain.BankAccountLink, from, to time.Time) (syncResult, error) {
	res := syncResult{
		Fetched:  len(txs),
		DateFrom: from.Format("2006-01-02"),
		DateTo:   to.Format("2006-01-02"),
	}

	// Layer-2 baseline. A failed read is fatal for the same reason it is in
	// the CSV importer: with an empty index every row looks new.
	existing, err := h.txRepo.ListAll()
	if err != nil {
		return res, fmt.Errorf("reading existing transactions for dedup: %w", err)
	}
	dedup := newDedupIndex(existing)
	byExternal := make(map[string]*domain.Transaction, len(existing))
	for i := range existing {
		if existing[i].ExternalID != "" {
			byExternal[existing[i].ExternalID] = &existing[i]
		}
	}

	var newest time.Time
	for _, t := range txs {
		// Only booked rows. A pending row is re-issued with a different
		// entry_reference once it books, so staging it would stage it twice;
		// supporting pending needs its own supersede lifecycle.
		if st := strings.ToUpper(strings.TrimSpace(t.Status)); st != "" && st != openbanking.StatusBooked {
			res.AutoSkipped++
			continue
		}
		row := adaptPSD2(t, link)
		// The classifier knows a fixed merchant list; the user's ledger knows
		// the rest. Enriching here (not at commit) means the review queue
		// shows the final proposal, and the user corrects what they can see.
		// Category and labels only — the dedup key is left untouched.
		h.enrich(&row)
		if row.BookingDate.After(newest) {
			newest = row.BookingDate
		}

		// Layer 1 — provider id. Cheap and certain.
		if match, ok := byExternal[row.ExternalID]; ok {
			row.Verdict = domain.VerdictDuplicateExact
			row.MatchedTxID = &match.ID
			row.VerdictNote = describeMatch(match)
		} else if row.Verdict == domain.VerdictNew {
			// Layer 2 — content multiset. Rows imported from statement.csv
			// carry no external id, so this is the only thing that catches
			// them. Consuming (take, not has) so two identical bank rows do
			// not both match one historical transaction.
			if match, dup := dedup.take(row.Date, row.Type, row.Amount, row.Comment); dup {
				row.Verdict = domain.VerdictDuplicateContent
				if match != nil {
					row.MatchedTxID = &match.ID
					row.VerdictNote = describeMatch(match)
				} else {
					row.VerdictNote = "matches a transaction already in your ledger"
				}
			}
		}

		switch row.Verdict {
		case domain.VerdictDuplicateExact:
			res.DuplicateExact++
		case domain.VerdictDuplicateContent:
			res.DuplicateContent++
		case domain.VerdictNeedsReview:
			res.NeedsReview++
		case domain.VerdictInternal:
			res.Internal++
		}

		// Layer 3 — in-batch upsert on the external id.
		prev, gerr := h.repo.GetStagedByExternalID(row.ExternalID)
		switch {
		case errors.Is(gerr, gorm.ErrRecordNotFound):
			row.State = domain.StagedStateStaged
			row.FirstSeenAt = time.Now()
			row.LastSeenAt = row.FirstSeenAt
			if err := h.repo.SaveStaged(&row); err != nil {
				return res, fmt.Errorf("staging %s: %w", row.ExternalID, err)
			}
			res.StagedNew++
		case gerr != nil:
			return res, gerr
		default:
			// Already known. A dismissed row stays dismissed — that
			// persistence IS the dismissal ledger, and re-offering a row the
			// user already rejected is the one thing a re-sync must not do.
			// An imported row stays imported.
			prev.LastSeenAt = time.Now()
			if prev.State == domain.StagedStateStaged {
				// Refresh only the verdict: the editable fields may carry
				// the user's own corrections, made before they walked away.
				prev.Verdict = row.Verdict
				prev.VerdictNote = row.VerdictNote
				prev.MatchedTxID = row.MatchedTxID
			}
			if err := h.repo.SaveStaged(prev); err != nil {
				return res, fmt.Errorf("updating staged %s: %w", row.ExternalID, err)
			}
			res.Unchanged++
		}
	}

	now := time.Now()
	link.LastSyncedAt = &now
	if !newest.IsZero() && (link.LastTxDate == nil || newest.After(*link.LastTxDate)) {
		link.LastTxDate = &newest
	}
	if err := h.repo.SaveLink(link); err != nil {
		return res, err
	}
	return res, nil
}

// describeMatch names the transaction a staged row collided with, in terms
// the user can check: "#1234 · 2026-09-12 · Lidl · 23.40".
func describeMatch(t *domain.Transaction) string {
	comment := t.Comment
	if comment == "" {
		comment = string(t.Category)
	}
	return fmt.Sprintf("#%d · %s · %s · %.2f", t.ID, t.Date.Format("2006-01-02"), comment, t.Amount)
}

// ── staging list ────────────────────────────────────────────────────

func (h *BankHandler) ListStaged(c *gin.Context) {
	f := repository.StagedFilter{
		State:    strings.TrimSpace(c.Query("state")),
		Verdict:  strings.TrimSpace(c.Query("verdict")),
		Page:     atoiDefault(c.Query("page"), 1),
		PageSize: atoiDefault(c.Query("page_size"), 100),
	}
	if f.State == "" {
		f.State = domain.StagedStateStaged
	}
	if f.State == "all" {
		f.State = ""
	}
	if v, err := strconv.ParseUint(c.Query("link_id"), 10, 32); err == nil {
		f.LinkID = uint(v)
	}
	if f.PageSize > 500 {
		f.PageSize = 500
	}
	rows, total, err := h.repo.ListStaged(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	counts, err := h.repo.CountStagedByVerdict()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]stagedResponse, 0, len(rows))
	for i := range rows {
		out = append(out, toStagedResponse(&rows[i]))
	}
	c.JSON(http.StatusOK, gin.H{
		"transactions": out,
		"total":        total,
		"page":         max(f.Page, 1),
		"page_size":    f.PageSize,
		"counts":       counts,
	})
}

type stagedResponse struct {
	domain.BankStagedTx
	// AmountMoney mirrors how transactions are serialised everywhere else,
	// so the UI formats one shape rather than two.
	AmountMoney domain.Money `json:"amount_money"`
	// Preticked tells the UI which rows arrive ticked. Computed server-side
	// so the rule lives in one place.
	Preticked bool `json:"preticked"`
}

func toStagedResponse(t *domain.BankStagedTx) stagedResponse {
	return stagedResponse{
		BankStagedTx: *t,
		AmountMoney:  domain.Money{Value: t.Amount, Currency: domain.CurrencyEUR},
		Preticked:    t.Preticked(),
	}
}

func atoiDefault(s string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && v > 0 {
		return v
	}
	return def
}

// ── commit / dismiss / restore ──────────────────────────────────────

type commitResult struct {
	Imported      int      `json:"imported"`
	Skipped       int      `json:"skipped"`
	ImportedTxIDs []uint   `json:"imported_tx_ids"`
	Notes         []string `json:"notes,omitempty"`
	// BalanceNote is the one honest line about what this did NOT do. Reuses
	// the balance_note surface the transaction create path already has.
	BalanceNote string `json:"balance_note"`
}

// CommitStaged turns ticked staged rows into real transactions. One id is the
// normal case — the review flow is one row at a time.
func (h *BankHandler) CommitStaged(c *gin.Context) {
	var in struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(in.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nothing selected"})
		return
	}
	var res commitResult
	run := func(h *BankHandler) error {
		var err error
		res, err = h.commit(in.IDs)
		return err
	}
	var err error
	if h.db != nil {
		err = h.db.Transaction(func(tx *gorm.DB) error {
			return run(&BankHandler{
				repo:     repository.NewBankRepository(tx),
				txRepo:   repository.NewTransactionRepository(tx),
				labeling: h.labeling,
			})
		})
	} else {
		err = run(h)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *BankHandler) commit(ids []uint) (commitResult, error) {
	res := commitResult{
		ImportedTxIDs: []uint{},
		// Balances are deliberately left alone: a PSD2 row is a flow, and
		// cutting a snapshot per committed row would plant a step in the
		// net-worth trend for every transaction added.
		BalanceNote: "Account balances were not changed — add a balance snapshot if you want the totals to move.",
	}
	rows, err := h.repo.GetStagedByIDs(ids)
	if err != nil {
		return res, err
	}
	existing, err := h.txRepo.ListAll()
	if err != nil {
		return res, fmt.Errorf("reading existing transactions for dedup: %w", err)
	}
	dedup := newDedupIndex(existing)
	byExternal := make(map[string]uint, len(existing))
	for i := range existing {
		if existing[i].ExternalID != "" {
			byExternal[existing[i].ExternalID] = existing[i].ID
		}
	}

	for i := range rows {
		row := &rows[i]
		if row.State != domain.StagedStateStaged {
			res.Skipped++
			res.Notes = append(res.Notes, fmt.Sprintf("%s was already %s", shortLabel(row), row.State))
			continue
		}
		// Re-verify inside the commit: the review list may have been open for
		// an hour, or the row may have been committed from another device.
		if txID, dup := byExternal[row.ExternalID]; dup {
			row.State = domain.StagedStateImported
			row.ImportedTxID = &txID
			row.Verdict = domain.VerdictDuplicateExact
			if err := h.repo.SaveStaged(row); err != nil {
				return res, err
			}
			res.Skipped++
			res.Notes = append(res.Notes, fmt.Sprintf("%s is already in your ledger", shortLabel(row)))
			continue
		}
		// A content match only blocks the commit when the user did NOT see a
		// duplicate warning. If the row was already flagged duplicate_content
		// and they ticked it anyway, that is a deliberate decision — two
		// identical rounds at the same bar are a real thing.
		if match, dup := dedup.has(row.Date, row.Type, row.Amount, row.Comment); dup && row.Verdict == domain.VerdictNew {
			row.Verdict = domain.VerdictDuplicateContent
			if match != nil {
				row.MatchedTxID = &match.ID
				row.VerdictNote = describeMatch(match)
			}
			if err := h.repo.SaveStaged(row); err != nil {
				return res, err
			}
			res.Skipped++
			res.Notes = append(res.Notes, fmt.Sprintf("%s now matches a transaction added since — check it and tick again", shortLabel(row)))
			continue
		}
		if row.Amount <= 0 {
			res.Skipped++
			res.Notes = append(res.Notes, fmt.Sprintf("%s has no amount — fix it first", shortLabel(row)))
			continue
		}
		for _, k := range []string{row.DebitAccount, row.CreditAccount} {
			if k != "" && !domain.IsValidAccountKey(k) {
				return res, fmt.Errorf("staged row %d names an unknown account %q", row.ID, k)
			}
		}

		tx := &domain.Transaction{
			Date:          row.Date,
			Type:          row.Type,
			Amount:        row.Amount,
			Category:      row.Category,
			Comment:       row.Comment,
			Labels:        domain.NormalizeLabels(row.Labels),
			DebitAccount:  row.DebitAccount,
			CreditAccount: row.CreditAccount,
			ExternalID:    row.ExternalID,
		}
		// txRepo.Create, not TransactionService.Create: the service cuts a
		// balance snapshot per transaction, which would show one step in the
		// net-worth trend for every row added here.
		if err := h.txRepo.Create(tx); err != nil {
			return res, fmt.Errorf("creating transaction (%s, %.2f): %w", row.Date.Format("2006-01-02"), row.Amount, err)
		}
		dedup.add(row.Date, row.Type, row.Amount, row.Comment)
		byExternal[row.ExternalID] = tx.ID

		row.State = domain.StagedStateImported
		row.ImportedTxID = &tx.ID
		if err := h.repo.SaveStaged(row); err != nil {
			return res, err
		}
		res.Imported++
		res.ImportedTxIDs = append(res.ImportedTxIDs, tx.ID)
	}
	return res, nil
}

func shortLabel(t *domain.BankStagedTx) string {
	c := t.Comment
	if c == "" {
		c = t.RawPayee
	}
	if c == "" {
		c = "This transaction"
	}
	return fmt.Sprintf("%s (%s)", c, t.Date.Format("2006-01-02"))
}

// UpdateStaged overrides the classifier's proposal on one row before it is
// committed. Only the ledger-bound fields are editable — the raw provider
// columns and the external id stay exactly as the bank sent them, because they
// are the audit trail and the dedup key.
//
// Editing deliberately does NOT re-run the verdict. The verdict answers "have
// I seen this bank row before", which is a question about the provider data;
// retyping the comment does not change the answer, and re-deriving it here
// would let an edit silently flip a reviewed row back to duplicate. Commit
// re-verifies against the ledger anyway.
//
// It DOES re-run the label rules, the same way editing a saved transaction
// does: fixing the category to Food is exactly when the "groceries" rule
// should start applying. Only rules that newly match add a label — see
// reapplyRules for why that asymmetry matters.
func (h *BankHandler) UpdateStaged(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var in struct {
		Date          *string `json:"date"`
		Type          *string `json:"type"`
		Category      *string `json:"category"`
		Comment       *string `json:"comment"`
		Labels        *string `json:"labels"`
		DebitAccount  *string `json:"debit_account"`
		CreditAccount *string `json:"credit_account"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rows, err := h.repo.GetStagedByIDs([]uint{uint(id)})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(rows) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "staged transaction not found"})
		return
	}
	row := &rows[0]
	if row.State != domain.StagedStateStaged {
		c.JSON(http.StatusConflict, gin.H{"error": "only a row still awaiting review can be edited"})
		return
	}
	matchedBefore := h.matchedRules(row)
	if in.Date != nil {
		d, err := time.Parse("2006-01-02", strings.TrimSpace(*in.Date))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "date must be YYYY-MM-DD"})
			return
		}
		row.Date = d
	}
	if in.Type != nil {
		t := domain.TransactionType(strings.TrimSpace(*in.Type))
		if t != domain.TransactionTypeExpense && t != domain.TransactionTypeIncome && t != domain.TransactionTypeInvestment {
			c.JSON(http.StatusBadRequest, gin.H{"error": "type must be expense, income or investment"})
			return
		}
		row.Type = t
	}
	if in.Category != nil {
		row.Category = domain.Category(strings.TrimSpace(*in.Category))
	}
	if in.Comment != nil {
		row.Comment = strings.TrimSpace(*in.Comment)
	}
	if in.Labels != nil {
		row.Labels = strings.TrimSpace(*in.Labels)
	}
	for _, f := range []struct {
		in  *string
		out *string
	}{{in.DebitAccount, &row.DebitAccount}, {in.CreditAccount, &row.CreditAccount}} {
		if f.in == nil {
			continue
		}
		v := strings.TrimSpace(*f.in)
		// Empty is legitimate: an expense has no credit side.
		if v != "" && !domain.IsValidAccountKey(v) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown account: " + v})
			return
		}
		*f.out = v
	}
	// An explicit labels edit is the user's final word on labels — no rule
	// gets to add to it in the same breath.
	if in.Labels == nil {
		h.reapplyRules(row, matchedBefore)
	} else {
		row.Labels = domain.NormalizeLabels(row.Labels)
	}
	if err := h.repo.SaveStaged(row); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toStagedResponse(row))
}

func (h *BankHandler) DismissStaged(c *gin.Context) {
	h.setStagedState(c, domain.StagedStateDismissed, domain.StagedStateStaged)
}

// RestoreStaged undoes a dismissal — a mis-tap has to be recoverable.
func (h *BankHandler) RestoreStaged(c *gin.Context) {
	h.setStagedState(c, domain.StagedStateStaged, domain.StagedStateDismissed)
}

func (h *BankHandler) setStagedState(c *gin.Context, to, from string) {
	var in struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rows, err := h.repo.GetStagedByIDs(in.IDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	changed := 0
	for i := range rows {
		// An imported row is not dismissible: deleting the transaction is
		// how that is undone, and that path restores the staged row.
		if rows[i].State != from {
			continue
		}
		rows[i].State = to
		if err := h.repo.SaveStaged(&rows[i]); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		changed++
	}
	c.JSON(http.StatusOK, gin.H{"changed": changed})
}
