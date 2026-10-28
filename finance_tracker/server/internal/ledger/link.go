package ledger

import (
	"net/url"
	"time"
)

// Link is the app route that lists the transactions behind a figure: pairs
// of Ledger filter keys and values (category, tag, merchant, account, kind,
// from, to, period); empty values are left out. Without from or period the
// Ledger would show only this month, so such a link spans all of history.
func Link(kv ...string) string {
	q := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			q.Add(kv[i], kv[i+1])
		}
	}
	if q.Get("from") == "" && q.Get("to") == "" && q.Get("period") == "" {
		q.Set("period", "all")
	}
	return "/ledger?" + q.Encode()
}

// MonthLink: the same for one calendar month (YYYY-MM).
func MonthLink(month string, kv ...string) string {
	to := month + "-31"
	if t, err := time.Parse("2006-01", month); err == nil {
		to = t.AddDate(0, 1, -1).Format("2006-01-02")
	}
	return Link(append([]string{"from", month + "-01", "to", to}, kv...)...)
}
