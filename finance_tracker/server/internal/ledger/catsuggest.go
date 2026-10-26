package ledger

import (
	"sort"
	"strings"
)

// CatSuggestions are the categories a form offers as one-tap chips: where
// this merchant's payments were filed before (most used first), then the
// categories used most lately for this kind, so picking one rarely needs
// the full list.
type CatSuggestions struct {
	Merchant []CatShare `json:"merchant"`
	Recent   []string   `json:"recent"`
}

type CatShare struct {
	ID string `json:"id"`
	N  int    `json:"n"`
}

func SuggestCategories(q querier, merchantName, kind string) (*CatSuggestions, error) {
	if kind == "" {
		kind = "expense"
	}
	out := &CatSuggestions{Merchant: []CatShare{}, Recent: []string{}}
	seen := map[string]bool{}
	if m := strings.TrimSpace(merchantName); m != "" {
		rows, err := q.Query(`SELECT category, COUNT(*) FROM (SELECT category FROM transactions WHERE lower(merchant)=lower(?) AND kind=? AND category<>'' ORDER BY date DESC LIMIT 30) GROUP BY category`, m, kind)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var c CatShare
			rows.Scan(&c.ID, &c.N)
			out.Merchant = append(out.Merchant, c)
		}
		rows.Close()
		sort.Slice(out.Merchant, func(i, j int) bool {
			if out.Merchant[i].N != out.Merchant[j].N {
				return out.Merchant[i].N > out.Merchant[j].N
			}
			return out.Merchant[i].ID < out.Merchant[j].ID
		})
		if len(out.Merchant) > 3 {
			out.Merchant = out.Merchant[:3]
		}
		for _, c := range out.Merchant {
			seen[c.ID] = true
		}
	}
	// The last 200 of this kind, by how often each category came up.
	rows, err := q.Query(`SELECT category, COUNT(*) n FROM (SELECT category FROM transactions WHERE kind=? AND category<>'' AND split_of IS NULL ORDER BY date DESC, id DESC LIMIT 200) GROUP BY category ORDER BY n DESC, category LIMIT 12`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int
		rows.Scan(&id, &n)
		if !seen[id] && len(out.Recent) < 8 {
			out.Recent = append(out.Recent, id)
		}
	}
	return out, rows.Err()
}
