package cfo

import (
	"sort"

	"ft/internal/insights"
	"ft/internal/merchant"
	"ft/internal/money"
)

// Judging optional spending: the habits — places and categories paid in
// most months — each with the owner's verdict (keep = worth, cut back =
// cut), and for what the owner said to cut how it has gone since. One-off
// shops (a laptop at bigbox, DIY runs) and guessed "leak" labels were left
// out in 3.1.1: there is nothing to keep or cut about a single purchase,
// and buying late in a month doesn't make something a leak.

type JudgeItem struct {
	Kind    string      `json:"kind"` // merchant | category
	Key     string      `json:"key"`
	Name    string      `json:"name"`
	Year    money.Cents `json:"year"`   // last twelve complete months
	Usual   money.Cents `json:"usual"`  // a month: Year / 12, so it always agrees with Year
	Month   money.Cents `json:"month"`  // this month so far
	Count   int         `json:"count"`  // payments in the twelve months
	Months  int         `json:"months"` // of the twelve, the months with a payment
	Verdict string      `json:"verdict,omitempty"`
}

// FollowUp: a place or category the owner said to cut.
type FollowUp struct {
	Kind      string       `json:"kind"`
	Key       string       `json:"key"`
	Name      string       `json:"name"`
	Usual     money.Cents  `json:"usual"`                // a month, when judged
	ThisMonth money.Cents  `json:"this_month"`           // since the verdict, when it was given this month
	LastMonth *money.Cents `json:"last_month,omitempty"` // the last complete month, once it came after the verdict
	JudgedOn  string       `json:"judged_on"`
}

// Judge: the twelve complete months' optional spending by verdict — a
// payment takes its place's verdict, else its own category's (exactly that
// category: a verdict on €50 filed as plain "leisure" must not paint every
// leisure.* payment) — and the items to judge.
type Judge struct {
	Worth   money.Cents `json:"worth"`
	Cut     money.Cents `json:"cut"`
	Unsure  money.Cents `json:"unsure"`
	Open    money.Cents `json:"open"` // not judged
	Items   []JudgeItem `json:"items"`
	Cutting []FollowUp  `json:"cutting"`
}

// A habit: paid in at least six of the twelve complete months. Trips are
// left out of places (a tour operator's instalments aren't a habit); their
// categories count like any other.
const habitMonths = 6

func judge(rows []optRow, r *OptionalReport, verdicts map[string]insights.Verdict, merName map[string]string, complete []string, thisM string) Judge {
	j := Judge{Items: []JudgeItem{}, Cutting: []FollowUp{}}
	inYear := map[string]bool{}
	for _, m := range complete {
		inYear[m] = true
	}
	lastComplete := ""
	if len(complete) > 0 {
		lastComplete = complete[len(complete)-1]
	}
	match := func(kind, key string) func(o optRow) bool {
		if kind == "merchant" {
			return func(o optRow) bool { return merchant.Key(o.Merchant) == key }
		}
		return func(o optRow) bool { return o.Category == key }
	}
	verdictOf := func(o optRow) string {
		if v, ok := verdicts["merchant:"+merchant.Key(o.Merchant)]; ok && o.Merchant != "" {
			return v.Verdict
		}
		if v, ok := verdicts["category:"+o.Category]; ok {
			return v.Verdict
		}
		return ""
	}
	for _, o := range rows {
		if !o.optional || !inYear[o.month] {
			continue
		}
		switch verdictOf(o) {
		case "worth":
			j.Worth += o.Amount
		case "cut":
			j.Cut += o.Amount
		case "unsure":
			j.Unsure += o.Amount
		default:
			j.Open += o.Amount
		}
	}

	item := func(kind, key, name string) JudgeItem {
		it := JudgeItem{Kind: kind, Key: key, Name: name}
		in := match(kind, key)
		months := map[string]bool{}
		buys := map[int64]bool{}
		for _, o := range rows {
			if !o.optional || !in(o) {
				continue
			}
			if o.month == thisM {
				it.Month += o.Amount
				continue
			}
			if !inYear[o.month] {
				continue
			}
			it.Year += o.Amount
			months[o.month] = true
			buys[o.purchase()] = true
		}
		it.Usual, it.Count, it.Months = it.Year/12, len(buys), len(months)
		if v, ok := verdicts[kind+":"+key]; ok {
			it.Verdict = v.Verdict
		}
		return it
	}

	// Habits: places (trips left out) and categories paid in at least
	// habitMonths of the twelve, biggest first; then anything judged that
	// is no longer one, so a verdict stays visible.
	seen := map[string]bool{}
	var habits []JudgeItem
	places := map[string]bool{}
	for _, o := range rows {
		if k := merchant.Key(o.Merchant); o.optional && k != "" && inYear[o.month] && !hasTrip(o.Tags) {
			places[k] = true
		}
	}
	for k := range places {
		if it := item("merchant", k, merName[k]); it.Months >= habitMonths {
			habits = append(habits, it)
		}
	}
	for _, c := range r.Categories {
		if it := item("category", c.Key, c.Key); it.Months >= habitMonths {
			habits = append(habits, it)
		}
	}
	sort.Slice(habits, func(i, k int) bool {
		if habits[i].Year != habits[k].Year {
			return habits[i].Year > habits[k].Year
		}
		return habits[i].Kind+habits[i].Key < habits[k].Kind+habits[k].Key
	})
	for _, it := range habits {
		j.Items = append(j.Items, it)
		seen[it.Kind+":"+it.Key] = true
	}
	var judged []string
	for k := range verdicts {
		if !seen[k] {
			judged = append(judged, k)
		}
	}
	sort.Strings(judged)
	for _, k := range judged {
		v := verdicts[k]
		j.Items = append(j.Items, item(v.Kind, v.Key, v.Name))
	}

	// Follow-up on what the owner said to cut.
	for _, it := range j.Items {
		v, ok := verdicts[it.Kind+":"+it.Key]
		if !ok || v.Verdict != "cut" {
			continue
		}
		f := FollowUp{Kind: it.Kind, Key: it.Key, Name: it.Name, Usual: v.Usual, ThisMonth: it.Month, JudgedOn: v.JudgedOn}
		// Judged this month: count only what came after the decision.
		if len(v.JudgedOn) >= 7 && v.JudgedOn[:7] == thisM {
			f.ThisMonth = 0
			in := match(it.Kind, it.Key)
			for _, o := range rows {
				if o.optional && o.month == thisM && o.Date >= v.JudgedOn && in(o) {
					f.ThisMonth += o.Amount
				}
			}
		}
		// Last month counts once a whole month has passed since the verdict.
		if lastComplete != "" && len(v.JudgedOn) >= 7 && v.JudgedOn[:7] < lastComplete {
			var last money.Cents
			in := match(it.Kind, it.Key)
			for _, o := range rows {
				if o.optional && o.month == lastComplete && in(o) {
					last += o.Amount
				}
			}
			f.LastMonth = &last
		}
		j.Cutting = append(j.Cutting, f)
	}
	return j
}
