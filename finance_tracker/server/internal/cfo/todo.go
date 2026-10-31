package cfo

import (
	"database/sql"
	"fmt"
	"slices"
	"time"

	"ft/internal/decisions"
	"ft/internal/insights"
	"ft/internal/money"
	"ft/internal/plan"
)

// moreActions are the "Needs you" items from the planning tools: payday
// transfers still to make, and later, renewals and decisions to look at.
// cur is this month's plan report when the caller has it (nil runs it).
func moreActions(d *sql.DB, now time.Time, cur *plan.Report) []Action {
	var out []Action
	// Payday transfers: for ten days after the salary, while any are open.
	if rb, err := buildRunbook(d, now, false, cur); err == nil && rb.Payday != "" && rb.Open > 0 {
		if pd, err := time.Parse("2006-01-02", rb.Payday); err == nil && now.Sub(pd) <= 10*24*time.Hour {
			var left money.Cents
			saving, moved := 0, 0
			for _, s := range rb.Steps {
				if s.Kind != "saving" {
					continue
				}
				saving++
				if s.Status == "done" {
					moved++
				} else {
					left += s.Amount - s.Done
				}
			}
			if left > 0 {
				lvl := "info"
				if now.Sub(pd) > 3*24*time.Hour {
					lvl = "warn"
				}
				out = append(out, Action{Level: lvl, Kind: "payday", Text: fmt.Sprintf("Pay yourself first: %s still to move", eur(left)),
					Detail: fmt.Sprintf("%d of %d saving lines moved · see Plan → Budget", moved, saving), Link: "/plan"})
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
					Detail: "the cooling-off is over — buy it or let it go", Link: "/plan/wishlist"})
			case x.Status == "bought" && x.Worth == "" && x.ReviewOn != "" && x.ReviewOn <= today:
				out = append(out, Action{Level: "info", Kind: "worth", Text: fmt.Sprintf("Was %s worth %s?", x.Title, eur(x.Amount)),
					Detail: "one tap — it teaches the journal what you value", Link: "/plan/wishlist"})
			}
		}
	}
	// Both bill views below read the same history; read it once (each gets
	// its own copy, so neither sees the other's sorting).
	bills, berr := insights.BillHistory(d, now)
	// Subscriptions marked to cancel: the day has come, or it charged again.
	if subs, err := insights.AuditSubscriptionsIn(d, slices.Clone(bills), now); berr == nil && err == nil {
		soon := now.AddDate(0, 0, 3).Format("2006-01-02")
		for _, it := range subs.Items {
			v := it.Verdict
			if v == nil || v.Verdict != "cancel" {
				continue
			}
			switch {
			case it.ChargedAfterCancel != "":
				out = append(out, Action{Level: "bad", Kind: "subscription", Text: fmt.Sprintf("%s charged again on %s — you meant to cancel it", it.Merchant, shortDay(it.ChargedAfterCancel)),
					Detail: fmt.Sprintf("%s a year", eur(it.Yearly)), Link: "/plan/recurring"})
			case v.CancelBy <= soon:
				lvl := "info"
				if v.CancelBy <= now.Format("2006-01-02") {
					lvl = "warn"
				}
				out = append(out, Action{Level: lvl, Kind: "subscription", Text: fmt.Sprintf("Cancel %s by %s", it.Merchant, shortDay(v.CancelBy)),
					Detail: fmt.Sprintf("saves %s a year", eur(it.Yearly)), Link: "/plan/recurring", Due: v.CancelBy})
			}
		}
	}
	// Bills about to renew that cost more last time.
	if p, err := insights.PriceCreepIn(d, slices.Clone(bills), now); berr == nil && err == nil {
		for _, it := range p.Items {
			if it.RenewsSoon && it.Change >= 5 {
				out = append(out, Action{Level: "warn", Kind: "renewal", Text: fmt.Sprintf("%s renews %s — %.0f%% dearer last time", it.Merchant, shortDay(it.Next), it.Change),
					Detail: "get a quote before it renews", Link: "/insights/drift", Due: it.Next})
			}
		}
	}
	return out
}
