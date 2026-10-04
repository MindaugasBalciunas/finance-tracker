package handler

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
)

// Matching a bank row to a transaction the user already entered by hand.
//
// Content dedup keys on date+type+amount+comment, which only ever catches a
// row this app wrote itself. A purchase typed in as "Maxima food" and the
// same purchase arriving from the bank as "MAXIMA" differ in the one field
// the key cannot do without, so the queue offered it as new and the ledger
// got it twice.
//
// The honest answer is not a looser dedup — dropping the comment from the key
// would start silently swallowing genuine repeats. It is to notice the
// likeness, say so, and let the user merge: the bank's reference is stamped
// onto the row they already wrote, so every later sync recognises it on the
// provider id and nothing is entered twice.

// mergeDayWindow is how far apart a hand-entered date and the bank's date may
// sit and still be the same purchase. A card row is dated when it was swiped
// and people type the day they remember — the first live sync showed two-day
// gaps (Apollo, Lidl) that a one-day window turned into duplicates. Three
// days still keeps a week's groceries apart.
const mergeDayWindow = 3

// amountMatchEpsilon is cent-level equality for money that arrived as a
// decimal string on one side and a float on the other.
const amountMatchEpsilon = 0.005

// findMergeCandidate looks for a ledger row that is probably the same payment
// as this staged row but was written by hand.
//
// A candidate is only a suggestion: it unticks the row and offers "Link to
// it", and the row can still be added. Missing one is what costs — a ticked
// duplicate — so the rule is generous where the first live sync showed real
// duplicates slipping through, and the ranking keeps the likeliest first:
//
//   - same amount to the cent, within mergeDayWindow days;
//   - same direction: money out (expense or investment — the bank calls an
//     IBKR or Artea transfer an expense, the ledger an investment) or in;
//   - any account, but a row on the same account beats one on another
//     (an IBKR top-up typed under Cash is still that top-up).
//
// Rows that already carry a provider id are skipped: those came from a sync
// and the existing layers own them.
func findMergeCandidate(row *domain.BankStagedTx, existing []domain.Transaction, claimed map[uint]bool) *domain.Transaction {
	if row == nil || row.Amount <= 0 {
		return nil
	}
	var best *domain.Transaction
	bestScore := math.MaxFloat64
	for i := range existing {
		t := &existing[i]
		if t.ExternalID != "" || claimed[t.ID] {
			continue
		}
		if math.Abs(t.Amount-row.Amount) > amountMatchEpsilon || !sameDirection(t.Type, row.Type) {
			continue
		}
		gap := t.Date.Sub(row.Date)
		if gap < 0 {
			gap = -gap
		}
		if gap > mergeDayWindow*24*time.Hour {
			continue
		}
		score := gap.Hours() / 24
		if !sameAccounts(t, row) {
			score += 10
		}
		if t.Type != row.Type {
			score += 0.5
		}
		if score < bestScore {
			best, bestScore = t, score
		}
	}
	return best
}

// sameDirection: money out (expense, investment) or money in (income).
func sameDirection(a, b domain.TransactionType) bool {
	out := func(t domain.TransactionType) bool {
		return t == domain.TransactionTypeExpense || t == domain.TransactionTypeInvestment
	}
	return a == b || (out(a) && out(b))
}

// sameAccounts reports whether the money moved between the same places — the
// ranking's strongest signal. A staged row always names the link's account.
func sameAccounts(t *domain.Transaction, row *domain.BankStagedTx) bool {
	debit, credit := t.DebitAccount, t.CreditAccount
	if debit == "" && credit == "" && t.SourceAccount != "" {
		if t.Type == domain.TransactionTypeExpense {
			debit = t.SourceAccount
		} else {
			credit = t.SourceAccount
		}
	}
	return debit == row.DebitAccount && credit == row.CreditAccount
}

// MergeStaged attaches a bank row to a transaction already in the ledger.
//
// The ledger row wins on every field: the user wrote it, and their "Maxima
// food" is a better description than the bank's "MAXIMA". All the merge moves
// is the provider id, which is what makes every future sync recognise the row
// on layer 1 instead of offering it again.
func (h *BankHandler) MergeStaged(c *gin.Context) {
	id, err := uintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var in struct {
		TransactionID uint `json:"transaction_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rows, err := h.repo.GetStagedByIDs([]uint{id})
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
		c.JSON(http.StatusConflict, gin.H{"error": "only a row still awaiting review can be merged"})
		return
	}
	// A reservation is not a payment yet, and its amount can still move. Let
	// it book first, then merge the booked row.
	if row.Pending {
		c.JSON(http.StatusConflict, gin.H{"error": "this is still only reserved by the bank — merge it once it books"})
		return
	}

	tx, err := h.txRepo.GetByID(in.TransactionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "transaction not found"})
		return
	}
	// Refuse to re-point a row that already belongs to another bank row —
	// that would orphan the first one and let it be offered again.
	if tx.ExternalID != "" && tx.ExternalID != row.ExternalID {
		c.JSON(http.StatusConflict, gin.H{
			"error": "that transaction is already linked to a different bank row",
		})
		return
	}

	if err := h.mergeInto(row, tx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"merged_into": tx.ID,
		"note": fmt.Sprintf(
			"Linked to %s. Your own description and labels were kept; later syncs will recognise it.",
			describeMatch(tx)),
	})
}

// UnmergeStaged is the way back: it drops the provider id from the ledger row
// and returns the bank row to the queue.
//
// Needed because a merge is a judgement call made from four fields. Getting
// it wrong must cost a tap, not a hand-edit of the database.
func (h *BankHandler) UnmergeStaged(c *gin.Context) {
	id, err := uintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rows, err := h.repo.GetStagedByIDs([]uint{id})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(rows) == 0 || rows[0].ImportedTxID == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "that row is not linked to a transaction"})
		return
	}
	row := &rows[0]

	tx, err := h.txRepo.GetByID(*row.ImportedTxID)
	if err == nil && tx.ExternalID == row.ExternalID {
		tx.ExternalID = ""
		if err := h.txRepo.Update(tx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	row.State = domain.StagedStateStaged
	row.ImportedTxID = nil
	row.Verdict = domain.VerdictNew
	row.VerdictNote = ""
	row.MatchedTxID = nil
	if err := h.repo.SaveStaged(row); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"restored": true})
}

// listMergeCandidates re-reads the ledger for the review list, so the UI can
// name the row a staged entry would merge into.
func (h *BankHandler) mergeCandidates(rows []domain.BankStagedTx) (map[uint]*domain.Transaction, error) {
	out := map[uint]*domain.Transaction{}
	if len(rows) == 0 {
		return out, nil
	}
	existing, err := h.txRepo.ListAll()
	if err != nil {
		return nil, err
	}
	claimed := map[uint]bool{}
	for i := range rows {
		r := &rows[i]
		if r.State != domain.StagedStateStaged || r.Pending || r.MatchedTxID != nil {
			continue
		}
		if m := findMergeCandidate(r, existing, claimed); m != nil {
			claimed[m.ID] = true
			out[r.ID] = m
		}
	}
	return out, nil
}

// mergeInto stamps the bank row's id onto the ledger transaction and closes
// the staged row as imported. The ledger row keeps every field the user wrote.
func (h *BankHandler) mergeInto(row *domain.BankStagedTx, tx *domain.Transaction) error {
	tx.ExternalID = row.ExternalID
	if err := h.txRepo.Update(tx); err != nil {
		return err
	}
	row.State = domain.StagedStateImported
	row.ImportedTxID = &tx.ID
	row.Verdict = domain.VerdictDuplicateExact
	row.MatchedTxID = &tx.ID
	row.VerdictNote = "merged into " + describeMatch(tx)
	return h.repo.SaveStaged(row)
}

// autoLinkDayWindow and autoLinkRivalWindow bound the automatic link: the
// match itself within a day, and nothing else that could be it within three.
const (
	autoLinkDayWindow   = 1
	autoLinkRivalWindow = 3
)

// autoLinkSafe links staged rows to hand-entered transactions when there is
// no doubt they are the same payment, so the review queue only holds what
// needs a decision. "No doubt" is stricter than a suggestion:
//
//   - same type, same amount to the cent, same account(s), dates ≤ 1 day apart;
//   - the ledger has no other unlinked row of that amount and direction
//     within three days, and no other waiting bank row could claim it.
//
// Anything looser stays a suggestion for the user ("Link to it"). Returns how
// many rows were linked; a failure stops the pass but never the sync.
func (h *BankHandler) autoLinkSafe() (int, error) {
	rows, _, err := h.repo.ListStaged(repository.StagedFilter{State: domain.StagedStateStaged})
	if err != nil {
		return 0, err
	}
	existing, err := h.txRepo.ListAll()
	if err != nil {
		return 0, err
	}
	var open []*domain.BankStagedTx
	for i := range rows {
		r := &rows[i]
		if !r.Pending && r.MatchedTxID == nil && r.Amount > 0 {
			open = append(open, r)
		}
	}
	near := func(a, b time.Time, days int) bool {
		d := a.Sub(b)
		if d < 0 {
			d = -d
		}
		return d <= time.Duration(days)*24*time.Hour
	}
	sameMoney := func(a, b float64) bool { return math.Abs(a-b) <= amountMatchEpsilon }

	linked := 0
	used := map[uint]bool{}
	for _, r := range open {
		var match *domain.Transaction
		rivals := 0
		for i := range existing {
			t := &existing[i]
			if t.ExternalID != "" || used[t.ID] || !sameMoney(t.Amount, r.Amount) || !sameDirection(t.Type, r.Type) {
				continue
			}
			if !near(t.Date, r.Date, autoLinkRivalWindow) {
				continue
			}
			rivals++
			if t.Type == r.Type && sameAccounts(t, r) && near(t.Date, r.Date, autoLinkDayWindow) {
				match = t
			}
		}
		if match == nil || rivals != 1 {
			continue
		}
		competing := 0
		for _, o := range open {
			if o.ID != r.ID && o.State == domain.StagedStateStaged && sameMoney(o.Amount, r.Amount) &&
				sameDirection(o.Type, r.Type) && near(o.Date, match.Date, autoLinkRivalWindow) {
				competing++
			}
		}
		if competing > 0 {
			continue
		}
		if err := h.mergeInto(r, match); err != nil {
			return linked, err
		}
		used[match.ID] = true
		linked++
	}
	return linked, nil
}
