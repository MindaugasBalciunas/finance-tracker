package ledger

import (
	"strings"

	"ft/internal/money"
)

// Owed is what each person still owes the owner: expenses tagged
// owed:<who> (paid for them) minus income with the same tag (paid back).
func Owed(q querier) (map[string]money.Cents, error) {
	txs, err := All(q, Filter{Query: "owed:"})
	if err != nil {
		return nil, err
	}
	by := map[string]money.Cents{}
	for _, t := range txs {
		for _, tag := range t.Tags {
			if p, ok := strings.CutPrefix(tag, "owed:"); ok {
				if t.Kind == "income" {
					by[p] -= t.Amount
				} else {
					by[p] += t.Amount
				}
			}
		}
	}
	return by, nil
}
