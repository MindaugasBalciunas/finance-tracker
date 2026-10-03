package handler

import (
	"errors"
	"fmt"
	"log"
	"net/http"
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
	Fetched          int `json:"fetched"`
	StagedNew        int `json:"staged_new"`
	Unchanged        int `json:"unchanged"`
	AutoSkipped      int `json:"auto_skipped"`
	DuplicateExact   int `json:"duplicate_exact"`
	DuplicateContent int `json:"duplicate_content"`
	NeedsReview      int `json:"needs_review"`
	Internal         int `json:"internal"`
	// Pending counts card reservations — authorised, not booked. Shown days
	// before the booked row exists; never committable.
	Pending int `json:"pending"`
	// Superseded counts reservations the bank booked during this sync.
	Superseded int `json:"superseded"`
	// Released counts reservations the bank dropped without booking.
	Released int    `json:"released"`
	DateFrom string `json:"date_from"`
	DateTo   string `json:"date_to"`
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

	txs, window, err := h.fetchWindow(c, cl, link, conn, requestedDays(c))
	if err != nil {
		writeProviderError(c, err)
		return
	}
	res, err := h.stage(txs, link, window.from, window.to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// requestedDays reads an explicit, narrower window off the query string. The
// first real-data run is meant to be a cautious 7-day look before the full
// 90; 0 means "use the normal window".
func requestedDays(c *gin.Context) int {
	if d, err := strconv.Atoi(c.Query("days")); err == nil && d > 0 && d <= 730 {
		return d
	}
	return 0
}

// syncWindow is how far back this account's next sync reaches: 90 days on a
// first sync (there is real history to close), a 7-day overlap afterwards,
// or an explicit override.
func syncWindow(link *domain.BankAccountLink, now time.Time, days int) time.Time {
	if days > 0 {
		return now.AddDate(0, 0, -days)
	}
	from := now.AddDate(0, 0, -firstSyncDays)
	if anchor := syncAnchor(link); !anchor.IsZero() {
		if candidate := anchor.AddDate(0, 0, -resyncOverlapDays); candidate.After(from) {
			from = candidate
		}
	}
	return from
}

type window struct{ from, to time.Time }

// fetchWindow pulls one mapped account's window from the provider. Shared by
// the single-account sync and Sync all, so the two cannot drift in what a
// sync actually reads.
//
// The error is returned, not written: the caller decides whether it fails the
// request (one account) or becomes one line in a report (all of them). It is
// kept separate from staging so a database failure is still a 500 and only a
// provider failure is a 502.
func (h *BankHandler) fetchWindow(
	c *gin.Context, cl *openbanking.Client,
	link *domain.BankAccountLink, conn *domain.BankConnection, days int,
) ([]openbanking.Transaction, window, error) {
	w := window{to: time.Now()}
	w.from = syncWindow(link, w.to, days)

	headers := h.requiredPSUHeaders(c, cl, conn)
	base := openbanking.TxQuery{
		AccountUID:         link.UID,
		DateFrom:           w.from,
		DateTo:             w.to,
		PSU:                psuFrom(c),
		RequiredPSUHeaders: headers,
	}

	txs, err := cl.AllTransactions(c.Request.Context(), base)
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
		return nil, w, err
	}

	// Card reservations, asked for by name.
	//
	// Most ASPSPs return booked rows only unless transaction_status says
	// otherwise, so a reservation is invisible until it books — two or three
	// days during which the money is gone and the app says nothing. This is a
	// second call because the parameter takes one status.
	//
	// Deliberately best-effort: a bank that rejects the parameter, or does
	// not serve pending rows at all, must not fail a sync whose booked half
	// succeeded. Booked rows are the ledger; reservations are a preview.
	pending, perr := cl.AllTransactions(c.Request.Context(), withStatus(base, openbanking.StatusPending))
	if perr != nil {
		log.Printf("bank sync: reservations unavailable for account %d: %v", link.ID, perr)
		return txs, w, nil
	}
	return mergeByReference(txs, pending), w, nil
}

func withStatus(q openbanking.TxQuery, status string) openbanking.TxQuery {
	q.TransactionStatus = status
	q.ContinuationKey = ""
	return q
}

// mergeByReference appends rows the first call did not already return.
//
// Some banks ignore transaction_status and answer both calls with everything,
// so the same row can arrive twice. Keying on the provider's own reference
// (plus the status, since the pending and booked forms of one purchase are
// different rows) keeps the fetched count honest.
func mergeByReference(booked, pending []openbanking.Transaction) []openbanking.Transaction {
	seen := make(map[string]bool, len(booked))
	key := func(t openbanking.Transaction) string {
		return strings.ToUpper(t.Status) + "|" + firstNonEmpty(t.EntryReference, t.TransactionID)
	}
	for _, t := range booked {
		if ref := firstNonEmpty(t.EntryReference, t.TransactionID); ref != "" {
			seen[key(t)] = true
		}
	}
	out := booked
	for _, t := range pending {
		ref := firstNonEmpty(t.EntryReference, t.TransactionID)
		if ref != "" && seen[key(t)] {
			continue
		}
		out = append(out, t)
	}
	return out
}

// ── sync all ────────────────────────────────────────────────────────

// syncAllResult reports every mapped account separately, plus the totals.
//
// One report rather than one request per account: the review queue is a
// single list, so "what arrived just now" is a single question. Per-account
// detail stays because the answer is rarely uniform — one bank's consent
// expires while the other syncs fine, and a single "failed" would hide that.
type syncAllResult struct {
	Accounts []accountSyncResult `json:"accounts"`
	// Totals sums the accounts that actually synced.
	Totals syncResult `json:"totals"`
	Synced int        `json:"synced"`
	// Skipped counts accounts that were never going to sync — unmapped, or
	// behind a dead consent. Not a failure, but not silence either.
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

type accountSyncResult struct {
	LinkID uint   `json:"link_id"`
	Bank   string `json:"bank"`
	Name   string `json:"name"`
	// Exactly one of result / skipped / error is set.
	Result  *syncResult `json:"result,omitempty"`
	Skipped string      `json:"skipped,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// SyncAll syncs every mapped account on every live connection.
//
// Pressing Sync once per account was the only way to fill the review queue,
// which made the common case — "pull whatever is new everywhere" — a tour of
// the settings page. One account failing does not stop the rest: each gets a
// line, and the user sees exactly which bank is unhappy.
func (h *BankHandler) SyncAll(c *gin.Context) {
	cl, _, err := h.client()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	conns, err := h.repo.ListConnections()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	days := requestedDays(c)
	out := syncAllResult{Accounts: []accountSyncResult{}}
	for i := range conns {
		conn := &conns[i]
		links, lerr := h.repo.ListLinks(conn.ID)
		if lerr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": lerr.Error()})
			return
		}
		for j := range links {
			link := &links[j]
			// An account the user chose not to sync is not worth a line —
			// "Don't sync" is an answer, and repeating it every time would
			// bury the accounts that do have something to say.
			if !link.Synced() {
				continue
			}
			row := accountSyncResult{
				LinkID: link.ID,
				Bank:   conn.ASPSPName,
				Name:   firstNonEmpty(link.DisplayName, link.IBAN, fmt.Sprintf("Account %d", link.ID)),
			}
			switch {
			case effectiveStatus(conn) != domain.BankConnAuthorized:
				row.Skipped = "the connection to this bank has expired — reconnect it"
			case link.UID == "":
				row.Skipped = "no active session — reconnect the bank"
			}
			if row.Skipped != "" {
				out.Skipped++
				out.Accounts = append(out.Accounts, row)
				continue
			}

			txs, w, ferr := h.fetchWindow(c, cl, link, conn, days)
			if ferr == nil {
				var res syncResult
				res, ferr = h.stage(txs, link, w.from, w.to)
				if ferr == nil {
					row.Result = &res
					out.Synced++
					out.Totals.Fetched += res.Fetched
					out.Totals.StagedNew += res.StagedNew
					out.Totals.Unchanged += res.Unchanged
					out.Totals.AutoSkipped += res.AutoSkipped
					out.Totals.DuplicateExact += res.DuplicateExact
					out.Totals.DuplicateContent += res.DuplicateContent
					out.Totals.NeedsReview += res.NeedsReview
					out.Totals.Internal += res.Internal
					out.Totals.Pending += res.Pending
					out.Totals.Superseded += res.Superseded
					out.Totals.Released += res.Released
					if out.Totals.DateFrom == "" || res.DateFrom < out.Totals.DateFrom {
						out.Totals.DateFrom = res.DateFrom
					}
					if res.DateTo > out.Totals.DateTo {
						out.Totals.DateTo = res.DateTo
					}
				}
			}
			if ferr != nil {
				row.Error = providerReason(ferr)
				out.Failed++
			}
			out.Accounts = append(out.Accounts, row)
		}
	}
	c.JSON(http.StatusOK, out)
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

	// Reservations already on the queue, so a booking can claim the one it
	// resolves and anything the bank stopped reporting can be closed out.
	reservations, err := h.openReservations(link.ID)
	if err != nil {
		return res, err
	}
	seen := map[string]bool{}

	var newest time.Time
	for _, t := range txs {
		// Anything that is neither booked nor a reservation (rejected,
		// cancelled) is still not stageable — it never moved money.
		st := strings.ToUpper(strings.TrimSpace(t.Status))
		if st != "" && st != openbanking.StatusBooked && st != openbanking.StatusPending {
			res.AutoSkipped++
			continue
		}
		row := adaptPSD2(t, link)
		seen[row.ExternalID] = true
		// The classifier knows a fixed merchant list; the user's ledger knows
		// the rest. Enriching here (not at commit) means the review queue
		// shows the final proposal, and the user corrects what they can see.
		// Category and labels only — the dedup key is left untouched.
		h.enrich(&row)
		// Only a booked row may advance the window anchor. A reservation can
		// carry an empty or optimistic booking date, and letting one push the
		// anchor forward would skip the rows behind it on the next sync.
		//
		// Card rows arrive with no booking_date at all over PSD2, so the
		// resolved transaction date stands in — otherwise the anchor never
		// moves and every sync re-reads the full 90 days forever.
		if !row.Pending {
			if booked := firstNonZeroTime(row.BookingDate, row.Date); booked.After(newest) {
				newest = booked
			}
		}

		// A reservation is never dedup'd against the ledger. It cannot be
		// committed, so a consuming content match would spend a ledger row
		// that the booked version has to claim a few days later.
		if row.Pending {
			if row.Verdict != domain.VerdictNeedsReview {
				row.Verdict = domain.VerdictPending
				row.VerdictNote = "reserved by the bank, not booked yet — the amount can still change"
			}
			res.Pending++
		} else if match, ok := byExternal[row.ExternalID]; ok {
			// Layer 1 — provider id. Cheap and certain.
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
			// A booking claims the reservation it resolves, inheriting any
			// correction made while it was still pending — which is what
			// makes reviewing a reservation early worth the effort.
			claimed := reservations.claim(&row)
			if err := h.repo.SaveStaged(&row); err != nil {
				return res, fmt.Errorf("staging %s: %w", row.ExternalID, err)
			}
			if claimed != nil {
				if err := h.closeReservation(claimed, &row.ID,
					fmt.Sprintf("booked by the bank on %s", row.BookingDate.Format("2006-01-02"))); err != nil {
					return res, err
				}
				res.Superseded++
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
				if prev.Edited {
					// Hands off everything but the verdict — the editable
					// fields carry the user's own corrections, made before
					// they walked away.
					prev.Verdict = row.Verdict
					prev.VerdictNote = row.VerdictNote
					prev.MatchedTxID = row.MatchedTxID
				} else {
					// Nobody has touched it, so re-apply the classifier in
					// full. Without this a fix only ever reaches rows fetched
					// after the fix shipped, and a queue full of misread rows
					// could only be cleared by dismissing them — which is
					// permanent, so they would never come back corrected.
					reclassify(prev, &row)
				}
			}
			if err := h.repo.SaveStaged(prev); err != nil {
				return res, fmt.Errorf("updating staged %s: %w", row.ExternalID, err)
			}
			res.Unchanged++
		}
	}

	// A reservation the bank has stopped reporting, inside a window we just
	// re-read, was released rather than booked — a hotel hold coming off, a
	// pre-auth reversed. Leaving it on the queue forever is how a review list
	// fills with things that never happened.
	for _, r := range reservations.remaining() {
		if seen[r.ExternalID] || r.Date.Before(from) {
			continue
		}
		if err := h.closeReservation(r, nil, "the bank released this reservation without booking it"); err != nil {
			return res, err
		}
		res.Released++
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
// reclassify replaces an untouched proposal with a freshly adapted one,
// keeping the identity and history of the staged row it replaces.
func reclassify(prev *domain.BankStagedTx, fresh *domain.BankStagedTx) {
	id, externalID, firstSeen, lastSeen := prev.ID, prev.ExternalID, prev.FirstSeenAt, prev.LastSeenAt
	state, importedTxID := prev.State, prev.ImportedTxID
	*prev = *fresh
	prev.ID, prev.ExternalID, prev.FirstSeenAt, prev.LastSeenAt = id, externalID, firstSeen, lastSeen
	prev.State, prev.ImportedTxID = state, importedTxID
}

func firstNonZeroTime(ts ...time.Time) time.Time {
	for _, t := range ts {
		if !t.IsZero() {
			return t
		}
	}
	return time.Time{}
}

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
	// Which staged rows look like something already entered by hand. Computed
	// here rather than in the browser: it needs the whole ledger, and the
	// rule has to be the same one the merge endpoint enforces.
	candidates, err := h.mergeCandidates(rows)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]stagedResponse, 0, len(rows))
	for i := range rows {
		r := toStagedResponse(&rows[i])
		if m := candidates[rows[i].ID]; m != nil {
			// Never arrives ticked. Content dedup missed it — the
			// descriptions differ — so without this the batch "Add" would
			// create exactly the duplicate the candidate is warning about.
			r.Preticked = false
			r.MergeCandidate = &mergeCandidate{
				ID:      m.ID,
				Date:    m.Date.Format("2006-01-02"),
				Comment: m.Comment,
				Amount:  m.Amount,
				Labels:  m.Labels,
			}
		}
		out = append(out, r)
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
	// Committable tells the UI whether this row can become a transaction.
	// Computed server-side for the same reason Preticked is: one rule, one
	// place, so the button and the endpoint cannot disagree.
	Committable bool `json:"committable"`
	// AmountMoney mirrors how transactions are serialised everywhere else,
	// so the UI formats one shape rather than two.
	AmountMoney domain.Money `json:"amount_money"`
	// Preticked tells the UI which rows arrive ticked. Computed server-side
	// so the rule lives in one place.
	Preticked bool `json:"preticked"`
	// MergeCandidate names a transaction already in the ledger that this row
	// is plainly the bank's version of — same amount, account and day, but a
	// description a person wrote rather than the one the bank sends.
	MergeCandidate *mergeCandidate `json:"merge_candidate,omitempty"`
}

// mergeCandidate is just enough of the existing transaction to recognise it.
type mergeCandidate struct {
	ID      uint    `json:"id"`
	Date    string  `json:"date"`
	Comment string  `json:"comment"`
	Amount  float64 `json:"amount"`
	Labels  string  `json:"labels"`
}

func toStagedResponse(t *domain.BankStagedTx) stagedResponse {
	return stagedResponse{
		BankStagedTx: *t,
		Committable:  t.Committable(),
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
				// Bound to this transaction's handle, so the snapshot lands
				// with the rows that caused it or not at all.
				balances: service.NewBalanceService(
					repository.NewBalanceRepository(tx),
					repository.NewTransactionRepository(tx),
				),
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
	res := commitResult{ImportedTxIDs: []uint{}}
	// The whole commit moves the balance sheet once. Per-row snapshots would
	// plant a step in the net-worth trend for every row added, and a 30-row
	// import is one moment of bringing the account up to date, not thirty.
	// The rows stay dated individually so the service can drop the ones the
	// latest snapshot already covers.
	var deltas []service.AccountDelta
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
		// A reservation is not a transaction yet. Its amount is not final and
		// the bank will re-issue it under a new reference when it books, so
		// committing one buys a few days of accuracy and pays for it with a
		// wrong amount and a duplicate. The corrections made here are kept
		// and carried onto the booked row.
		if !row.Committable() {
			res.Skipped++
			res.Notes = append(res.Notes, fmt.Sprintf(
				"%s is still only reserved by the bank — it can be added once it books", shortLabel(row)))
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
		if row.DebitAccount != "" {
			deltas = append(deltas, service.AccountDelta{Date: row.Date, Account: row.DebitAccount, Amount: -row.Amount})
		}
		if row.CreditAccount != "" {
			deltas = append(deltas, service.AccountDelta{Date: row.Date, Account: row.CreditAccount, Amount: row.Amount})
		}

		row.State = domain.StagedStateImported
		row.ImportedTxID = &tx.ID
		if err := h.repo.SaveStaged(row); err != nil {
			return res, err
		}
		res.Imported++
		res.ImportedTxIDs = append(res.ImportedTxIDs, tx.ID)
	}

	res.BalanceNote = h.snapshotCommit(deltas, res.Imported)
	return res, nil
}

// snapshotCommit moves the balance sheet for everything just committed, and
// returns the one honest line about what happened to it.
//
// A snapshot failure never fails the commit: the transactions are already
// written and correct, and refusing the whole import because the balance
// could not be restated would be a lie about what went in.
func (h *BankHandler) snapshotCommit(deltas []service.AccountDelta, imported int) string {
	switch {
	case imported == 0:
		return ""
	case h.balances == nil:
		return "Account balances were not changed — add a balance snapshot if you want the totals to move."
	case len(deltas) == 0:
		return "Account balances were not changed — none of these rows name an account."
	}
	note, err := h.balances.SnapshotFromDeltas(deltas)
	if err != nil {
		log.Printf("bank commit balance snapshot failed: %v", err)
		return "Account balances were not changed: " + err.Error()
	}
	if note != "" {
		return note
	}
	return "Account balances moved in a new snapshot."
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
		Date *string `json:"date"`
		Type *string `json:"type"`
		// Amount is editable because some rows arrive unusable: a
		// foreign-currency row is staged with a note saying "enter the euro
		// amount by hand", and until now there was no hand to enter it with.
		// RawAmount keeps the bank's own figure as the audit trail.
		Amount        *float64 `json:"amount"`
		Category      *string  `json:"category"`
		Comment       *string  `json:"comment"`
		Labels        *string  `json:"labels"`
		DebitAccount  *string  `json:"debit_account"`
		CreditAccount *string  `json:"credit_account"`
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
	if in.Amount != nil {
		if *in.Amount <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "amount must be greater than zero"})
			return
		}
		row.Amount = *in.Amount
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
	// From here on a re-sync refreshes only this row's verdict: an improved
	// classifier must not overwrite what a person decided.
	row.Edited = true
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
