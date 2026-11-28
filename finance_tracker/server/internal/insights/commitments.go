package insights

import (
	"sort"

	"ft/internal/ledger"
	"ft/internal/merchant"
	"ft/internal/money"
)

// Fixed commitments: money that isn't a choice — alimony, family support, the
// mortgage, finance costs (ledger.Obligation) and anything else paid to a
// person who is paid one (an ex-partner, the mortgage bank). The "biggest
// expenses" and "top merchants" lists leave them out: €1,000 of alimony every
// month filled eight of Year review's ten places and hid the real one-offs.
// They are shown beside the lists as one line of totals instead.

// CommitmentTotal is one category's fixed commitments in a window.
type CommitmentTotal struct {
	Category string      `json:"category"`
	Total    money.Cents `json:"total"`
	Count    int         `json:"count"`
}

// CommitmentPeople are the merchant keys ever paid an obligation, taken from
// txs (pass the whole history, as ledger.ObligationPayees does in SQL).
func CommitmentPeople(txs []ledger.Tx) map[string]bool {
	people := map[string]bool{}
	for i := range txs {
		if t := &txs[i]; t.Kind == "expense" && t.Merchant != "" && ledger.PaysAPerson(t.Category) {
			people[merchant.Key(t.Merchant)] = true
		}
	}
	return people
}

// Committed: an expense that is a fixed commitment, not a choice.
func Committed(t *ledger.Tx, people map[string]bool) bool {
	return t.Kind == "expense" && (ledger.Obligation(t.Category) || (t.Merchant != "" && people[merchant.Key(t.Merchant)]))
}

// Choices are the expenses in txs that aren't fixed commitments.
func Choices(txs []ledger.Tx, people map[string]bool) []ledger.Tx {
	out := make([]ledger.Tx, 0, len(txs))
	for i := range txs {
		if !Committed(&txs[i], people) {
			out = append(out, txs[i])
		}
	}
	return out
}

// Commitments totals the fixed commitments in a window by category, largest
// first; empty, not null, when there are none.
func Commitments(txs []ledger.Tx, people map[string]bool, from, to string) []CommitmentTotal {
	by := map[string]*CommitmentTotal{}
	for i := range txs {
		t := &txs[i]
		if t.Date < from || t.Date > to || !Committed(t, people) {
			continue
		}
		c := by[t.Category]
		if c == nil {
			c = &CommitmentTotal{Category: t.Category}
			by[t.Category] = c
		}
		c.Total += t.Amount
		c.Count++
	}
	out := make([]CommitmentTotal, 0, len(by))
	for _, c := range by {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].Category < out[j].Category
	})
	return out
}
