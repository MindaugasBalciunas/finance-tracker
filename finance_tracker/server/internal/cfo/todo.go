package cfo

import (
	"database/sql"
	"fmt"
	"time"

	"ft/internal/decisions"
	"ft/internal/insights"
	"ft/internal/money"
)

// moreActions are the "Needs you" items from the planning tools: payday
// transfers still to make, and later, renewals and decisions to look at.
func moreActions(d *sql.DB, now time.Time) []Action {
	var out []Action
	// Payday transfers: for ten days after the salary, while any are open.
	if rb, err := buildRunbook(d, now, false); err == nil && rb.Payday != "" && rb.Open > 0 {
		if pd, err := time.Parse("2006-01-02", rb.Payday); err == nil && now.Sub(pd) <= 10*24*time.Hour {
			var left money.Cents
			for _, s := range rb.Steps {
				if s.Kind == "saving" && s.Status != "done" {
					left += s.Amount - s.Done
				}
			}
			if left > 0 {
				lvl := "info"
				if now.Sub(pd) > 3*24*time.Hour {
					lvl = "warn"
				}
				out = append(out, Action{Level: lvl, Kind: "payday", Text: fmt.Sprintf("Pay yourself first: %s still to move", eur(left)),
					Detail: fmt.Sprintf("%d of %d payday steps done", len(rb.Steps)-rb.Open, len(rb.Steps)), Link: "/plan/payday"})
			}
		}
	}
	// The decision journal: cooling-off over, or time to say if it was worth it.
	if all, err := decisions.List(d); err == nil {
		today := now.Format("2006-01-02")
		for _, x := range all {
			switch {
			case x.Status == "thinking" && x.CoolUntil != "" && x.CoolUntil <= today:
				out = append(out, Action{Level: "info", Kind: "decide", Text: fmt.Sprintf("Still want %s (%s)?", x.Title, eur(x.Amount)),
					Detail: "the cooling-off is over — buy it or let it go", Link: "/plan/decisions"})
			case x.Status == "bought" && x.Worth == "" && x.ReviewOn != "" && x.ReviewOn <= today:
				out = append(out, Action{Level: "info", Kind: "worth", Text: fmt.Sprintf("Was %s worth %s?", x.Title, eur(x.Amount)),
					Detail: "one tap — it teaches the journal what you value", Link: "/plan/decisions"})
			}
		}
	}
	// Bills about to renew that cost more last time.
	if p, err := insights.PriceCreep(d, now); err == nil {
		for _, it := range p.Items {
			if it.RenewsSoon && it.Change >= 5 {
				out = append(out, Action{Level: "warn", Kind: "renewal", Text: fmt.Sprintf("%s renews %s — %.0f%% dearer last time", it.Merchant, shortDay(it.Next), it.Change),
					Detail: "get a quote before it renews", Link: "/insights/drift", Due: it.Next})
			}
		}
	}
	return out
}
