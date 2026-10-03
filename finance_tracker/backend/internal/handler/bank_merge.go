package handler

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
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
// and a person types the day they remember; one day either way covers that,
// a week would start pairing unrelated groceries.
const mergeDayWindow = 1

// amountMatchEpsilon is cent-level equality for money that arrived as a
// decimal string on one side and a float on the other.
const amountMatchEpsilon = 0.005

// findMergeCandidate looks for a ledger row that is plainly the same payment
// as this staged row but was written by hand.
//
// Deliberately strict on everything except the description: same direction,
// same amount to the cent, same account, within a day. The description is the
// one field two people — the user and the bank — reliably disagree about.
//
// Rows that already carry a provider id are skipped: those came from a sync
// and the existing layers own them.
func findMergeCandidate(row *domain.BankStagedTx, existing []domain.Transaction, claimed map[uint]bool) *domain.Transaction {
	if row == nil || row.Amount <= 0 {
		return nil
	}
	var best *domain.Transaction
	bestGap := time.Duration(math.MaxInt64)
	for i := range existing {
		t := &existing[i]
		if t.ExternalID != "" || claimed[t.ID] {
			continue
		}
		if t.Type != row.Type || math.Abs(t.Amount-row.Amount) > amountMatchEpsilon {
			continue
		}
		if !sameAccounts(t, row) {
			continue
		}
		gap := t.Date.Sub(row.Date)
		if gap < 0 {
			gap = -gap
		}
		if gap > mergeDayWindow*24*time.Hour {
			continue
		}
		if gap < bestGap {
			best, bestGap = t, gap
		}
	}
	return best
}

// sameAccounts requires the money to have moved between the same places. A
// staged row always names at least one account (the link's own), so a ledger
// row with none is not a match — it was never tied to this account.
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

	tx.ExternalID = row.ExternalID
	if err := h.txRepo.Update(tx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	row.State = domain.StagedStateImported
	row.ImportedTxID = &tx.ID
	row.Verdict = domain.VerdictDuplicateExact
	row.MatchedTxID = &tx.ID
	row.VerdictNote = "merged into " + describeMatch(tx)
	if err := h.repo.SaveStaged(row); err != nil {
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
