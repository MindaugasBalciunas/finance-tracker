package bank

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"ft/internal/db"
	"ft/internal/entry"
	"ft/internal/ledger"
	"ft/internal/money"
)

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
		// Through entry, like every other write: a booked transfer to or from
		// one of your accounts the bank doesn't sync (a savings account
		// entered by hand) moves that account's balance, so it doesn't keep
		// showing money already moved out. Synced accounts are skipped:
		// their bank states the truth.
		if _, err := entry.Create(tx, &t); err != nil {
			return nil, err
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
