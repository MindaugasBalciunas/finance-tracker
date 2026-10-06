package cfo

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"ft/internal/insights"
	"ft/internal/money"
)

// Action is one thing that needs the owner, for the "Needs you" list: money
// to move before a payment, a planned transfer that hasn't happened, rows to
// review, a month review waiting. Most urgent first.
type Action struct {
	Level  string `json:"level"` // bad | warn | info
	Kind   string `json:"kind"`  // move | short | planned | inbox | review | check | budgets
	Text   string `json:"text"`
	Detail string `json:"detail,omitempty"`
	Link   string `json:"link,omitempty"` // in-app route; "cash" = the cash card on Home
	Due    string `json:"due,omitempty"`
}

func eur(c money.Cents) string {
	v := c.Float()
	s := fmt.Sprintf("%.0f", v)
	if v != float64(int64(v)) {
		s = fmt.Sprintf("%.2f", v)
	}
	// thousands separator
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, frac, _ := strings.Cut(s, ".")
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + "," + whole[i:]
	}
	if frac != "" {
		whole += "." + frac
	}
	if neg {
		return "−€" + whole
	}
	return "€" + whole
}

func shortDay(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.Format("2 Jan")
}

func buildActions(o *Overview, budgetDiffs int, plannedLate []insights.Recurring, now time.Time) []Action {
	out := []Action{}
	soon := now.AddDate(0, 0, 3).Format("2006-01-02")
	if c := o.Cash; c != nil {
		// Only the next top-up; the whole schedule lives in the cash card.
		if len(c.Moves) > 0 {
			m := c.Moves[0]
			lvl := "info"
			if m.By <= soon {
				lvl = "warn"
			}
			detail := "by " + shortDay(m.By)
			if m.For != "" {
				detail += " · for " + strings.ToLower(m.For[:1]) + m.For[1:]
			}
			if n := len(c.Moves) - 1; n > 0 {
				detail += fmt.Sprintf(" · then %d more before payday", n)
			}
			out = append(out, Action{Level: lvl, Kind: "move", Text: fmt.Sprintf("Move %s from %s to %s", eur(m.Amount), m.FromName, m.ToName), Detail: detail, Link: "cash", Due: m.By})
		}
		for _, m := range c.ToSavings {
			out = append(out, Action{Level: "info", Kind: "idle", Text: fmt.Sprintf("%s in %s could earn in %s", eur(m.Amount), m.FromName, m.ToName),
				Detail: m.For, Link: "cash"})
		}
		if c.Short > 0 {
			detail := "payday " + shortDay(c.Payday) + " · lower a buffer, or spend less"
			lvl := "bad"
			if c.Cash >= c.Short {
				detail = fmt.Sprintf("deposit %s of your %s cash, or lower a buffer", eur(c.Short), eur(c.Cash))
				lvl = "warn"
			}
			out = append(out, Action{Level: lvl, Kind: "short", Text: fmt.Sprintf("Savings fall %s short before payday", eur(c.Short)), Detail: detail, Link: "cash"})
		}
	}
	for _, r := range plannedLate {
		out = append(out, Action{Level: "warn", Kind: "planned", Text: fmt.Sprintf("%s %s planned for %s hasn't happened", r.Merchant, eur(r.Amount), shortDay(r.Next)),
			Link: "/insights/recurring", Due: r.Next})
	}
	if o.InboxOpen > 0 {
		t := fmt.Sprintf("%d bank transactions wait for review", o.InboxOpen)
		if o.InboxOpen == 1 {
			t = "1 bank transaction waits for review"
		}
		out = append(out, Action{Level: "info", Kind: "inbox", Text: t, Link: "/ledger/inbox"})
	}
	if o.ReviewMonth != "" {
		if m, err := time.Parse("2006-01", o.ReviewMonth); err == nil {
			out = append(out, Action{Level: "info", Kind: "review", Text: "Your " + m.Format("January 2006") + " review is ready", Detail: "how the month went and what needs a look", Link: "/insights/review?month=" + o.ReviewMonth})
		}
	}
	for _, c := range o.Checks {
		if strings.HasPrefix(c.Link, "/ledger/inbox") {
			continue
		}
		out = append(out, Action{Level: c.Level, Kind: "check", Text: c.Text, Link: c.Link})
	}
	if budgetDiffs > 0 {
		t := fmt.Sprintf("%d budgets differ from your last 12 months", budgetDiffs)
		if budgetDiffs == 1 {
			t = "1 budget differs from your last 12 months"
		}
		out = append(out, Action{Level: "info", Kind: "budgets", Text: t, Link: "/plan"})
	}
	rank := map[string]int{"bad": 0, "warn": 1, "info": 2}
	sort.SliceStable(out, func(i, j int) bool {
		if rank[out[i].Level] != rank[out[j].Level] {
			return rank[out[i].Level] < rank[out[j].Level]
		}
		di, dj := out[i].Due, out[j].Due
		if (di == "") != (dj == "") {
			return di != ""
		}
		return di < dj
	})
	return out
}
