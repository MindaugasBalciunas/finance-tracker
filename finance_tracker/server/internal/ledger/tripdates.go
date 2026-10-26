package ledger

import (
	"errors"
	"strings"
	"time"

	"ft/internal/db"
)

// TripDates is a trip's real period. Expenses dated inside it are the
// trip's (bills and obligations aside), so new and bank-synced rows get the
// trip:… tag without anyone remembering to add it.
type TripDates struct {
	Tag   string `json:"tag"`   // trip:italy-spring-2026
	Start string `json:"start"` // first day, YYYY-MM-DD
	End   string `json:"end"`   // last day
}

// NotTrip: what never belongs to a trip however it is dated — bills,
// obligations, money moved between own accounts.
func NotTrip(t *Tx) bool {
	if t.Kind != "expense" {
		return true
	}
	switch Top(t.Category) {
	case "utilities", "subscriptions", "housing", "finance":
		return true
	}
	return t.Category == "kids.alimony" || t.Category == "other.family"
}

// HomeMerchants are merchants paid at least three times in the last year
// outside any trip — the local grocer, the usual fuel station. Spending
// there during a trip's dates is probably at home, so it is suggested for
// the trip, never added by itself.
func HomeMerchants(q querier, trips []TripDates, now time.Time) map[string]bool {
	rows, err := q.Query(`SELECT lower(merchant), date, tags FROM transactions WHERE kind='expense' AND merchant != '' AND date >= ?`, now.AddDate(-1, 0, 0).Format("2006-01-02"))
	if err != nil {
		return map[string]bool{}
	}
	defer rows.Close()
	n := map[string]int{}
	for rows.Next() {
		var m, d, tags string
		rows.Scan(&m, &d, &tags)
		if TripOn(trips, d) == "" && !hasTripTag(SplitTags(tags)) {
			n[m]++
		}
	}
	home := map[string]bool{}
	for m, c := range n {
		if c >= 3 {
			home[m] = true
		}
	}
	return home
}

// SureTrip says whether an expense inside a trip's dates is the trip's for
// sure (added by itself) or only maybe (suggested): a known merchant you
// don't pay at home, not kids' costs or a work lunch.
func SureTrip(t *Tx, home map[string]bool) bool {
	if NotTrip(t) || strings.TrimSpace(t.Merchant) == "" || home[strings.ToLower(strings.TrimSpace(t.Merchant))] {
		return false
	}
	return Top(t.Category) != "kids" && t.Category != "food.work_lunch"
}

func ListTripDates(q querier) ([]TripDates, error) {
	rows, err := q.Query(`SELECT tag, start_date, end_date FROM trip_dates ORDER BY start_date`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TripDates{}
	for rows.Next() {
		var t TripDates
		if err := rows.Scan(&t.Tag, &t.Start, &t.End); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SaveTripDates sets (or replaces) a trip's period.
func SaveTripDates(e execer, t TripDates) error {
	t.Tag = strings.TrimSpace(strings.ToLower(t.Tag))
	if !TripTag(t.Tag) || len(t.Tag) <= len("trip:") {
		return errors.New("a trip tag starts with trip:")
	}
	a, err1 := time.Parse("2006-01-02", t.Start)
	b, err2 := time.Parse("2006-01-02", t.End)
	if err1 != nil || err2 != nil {
		return errors.New("dates are YYYY-MM-DD")
	}
	if b.Before(a) {
		return errors.New("the trip ends before it starts")
	}
	if b.Sub(a) > 120*24*time.Hour {
		return errors.New("a trip is at most 120 days")
	}
	_, err := e.Exec(`INSERT INTO trip_dates(tag,start_date,end_date,created_at,updated_at) VALUES(?,?,?,?,?)
		ON CONFLICT(tag) DO UPDATE SET start_date=excluded.start_date, end_date=excluded.end_date, updated_at=excluded.updated_at`,
		t.Tag, t.Start, t.End, db.Now(), db.Now())
	return err
}

func DeleteTripDates(e execer, tag string) error {
	_, err := e.Exec(`DELETE FROM trip_dates WHERE tag=?`, tag)
	return err
}

// TripOn is the trip whose dates hold date, or "".
func TripOn(trips []TripDates, date string) string {
	for _, t := range trips {
		if date >= t.Start && date <= t.End {
			return t.Tag
		}
	}
	return ""
}

func hasTripTag(tags []string) bool {
	for _, t := range tags {
		if TripTag(t) {
			return true
		}
	}
	return false
}

// TagSuggestions are one-tap tags for the transaction form: the trip whose
// dates hold the date (or, for a booking, the trip it is probably for), the
// tags this merchant usually gets, and the tags used most lately.
type TagSuggestions struct {
	Trip     *TripHint  `json:"trip,omitempty"`
	Usual    []TagShare `json:"usual"`
	Frequent []string   `json:"frequent"`
}

type TripHint struct {
	TripDates
	Why  string `json:"why"`  // dates | booking
	Sure bool   `json:"sure"` // add it by itself (a merchant you don't pay at home)
	Home bool   `json:"home"` // the merchant is one you pay at home too
}

type TagShare struct {
	Tag string `json:"tag"`
	N   int    `json:"n"`  // of the merchant's last payments that had it
	Of  int    `json:"of"` // how many were looked at
}

// SuggestTags: usual tags need three earlier payments to the merchant and
// a tag on at least 60% of the last ten — in backtests that was right 98%
// of the time.
func SuggestTags(q querier, merchantName, date, category, kind string, now time.Time) (*TagSuggestions, error) {
	out := &TagSuggestions{Usual: []TagShare{}, Frequent: []string{}}
	trips, err := ListTripDates(q)
	if err != nil {
		return nil, err
	}
	t := Tx{Kind: kind, Category: category, Date: date}
	if t.Kind == "" {
		t.Kind = "expense"
	}
	if !NotTrip(&t) && date != "" {
		if tag := TripOn(trips, date); tag != "" {
			home := HomeMerchants(q, trips, now)
			probe := Tx{Kind: t.Kind, Category: category, Merchant: merchantName}
			for _, tr := range trips {
				if tr.Tag == tag {
					out.Trip = &TripHint{TripDates: tr, Why: "dates", Sure: SureTrip(&probe, home), Home: home[strings.ToLower(strings.TrimSpace(merchantName))]}
				}
			}
		} else if Top(category) == "travel" {
			// A flight or a hotel booked ahead: the nearest trip starting within nine months.
			d, err := time.Parse("2006-01-02", date)
			if err == nil {
				best := ""
				for _, tr := range trips {
					s, _ := time.Parse("2006-01-02", tr.Start)
					if s.After(d) && s.Sub(d) <= 270*24*time.Hour && (best == "" || tr.Start < best) {
						best = tr.Start
						out.Trip = &TripHint{TripDates: tr, Why: "booking"}
					}
				}
			}
		}
	}
	if m := strings.TrimSpace(merchantName); m != "" {
		rows, err := q.Query(`SELECT tags FROM transactions WHERE lower(merchant)=lower(?) ORDER BY date DESC LIMIT 10`, m)
		if err != nil {
			return nil, err
		}
		var n int
		count := map[string]int{}
		for rows.Next() {
			var tags string
			rows.Scan(&tags)
			n++
			for _, tg := range SplitTags(tags) {
				if !TripTag(tg) {
					count[tg]++
				}
			}
		}
		rows.Close()
		if n >= 3 {
			for tg, c := range count {
				if float64(c) >= 0.6*float64(n) {
					out.Usual = append(out.Usual, TagShare{tg, c, n})
				}
			}
			sortShares(out.Usual)
		}
	}
	rows, err := q.Query(`SELECT tags FROM transactions WHERE tags != '' AND date >= ?`, now.AddDate(0, 0, -120).Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	freq := map[string]int{}
	for rows.Next() {
		var tags string
		rows.Scan(&tags)
		for _, tg := range SplitTags(tags) {
			if !TripTag(tg) {
				freq[tg]++
			}
		}
	}
	rows.Close()
	usual := map[string]bool{}
	for _, u := range out.Usual {
		usual[u.Tag] = true
	}
	var fs []TagShare
	for tg, c := range freq {
		if !usual[tg] {
			fs = append(fs, TagShare{tg, c, 0})
		}
	}
	sortShares(fs)
	for _, f := range fs[:min(8, len(fs))] {
		out.Frequent = append(out.Frequent, f.Tag)
	}
	return out, nil
}

func sortShares(s []TagShare) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && (s[j].N > s[j-1].N || s[j].N == s[j-1].N && s[j].Tag < s[j-1].Tag); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// TripUntagged lists what a trip probably missed: expenses inside its dates
// without a trip tag (bills aside), and travel bookings in the nine months
// before it.
func TripUntagged(q querier, tag string) (likely, maybe, bookings []Tx, err error) {
	var tr TripDates
	if err := q.QueryRow(`SELECT tag, start_date, end_date FROM trip_dates WHERE tag=?`, tag).Scan(&tr.Tag, &tr.Start, &tr.End); err != nil {
		return nil, nil, nil, errors.New("this trip has no dates yet")
	}
	trips, _ := ListTripDates(q)
	home := HomeMerchants(q, trips, time.Now())
	s, _ := time.Parse("2006-01-02", tr.Start)
	all, err := All(q, Filter{From: s.AddDate(0, 0, -270).Format("2006-01-02"), To: tr.End, Kind: "expense"})
	if err != nil {
		return nil, nil, nil, err
	}
	likely, maybe, bookings = []Tx{}, []Tx{}, []Tx{}
	for _, t := range all {
		if hasTripTag(t.Tags) || NotTrip(&t) {
			continue
		}
		switch {
		case t.Date >= tr.Start && SureTrip(&t, home):
			likely = append(likely, t)
		case t.Date >= tr.Start:
			maybe = append(maybe, t)
		case Top(t.Category) == "travel":
			bookings = append(bookings, t)
		}
	}
	return likely, maybe, bookings, nil
}
