package cfo

import (
	"sort"
	"strconv"

	"ft/internal/insights"
	"ft/internal/merchant"
	"ft/internal/money"
)

// Judging optional spending: the regular places and the biggest categories,
// each with the owner's verdict (worth it / cut / not sure), a suggestion
// from what the data shows, and — for what the owner said to cut — how it
// has gone since.

type JudgeItem struct {
	Kind    string      `json:"kind"` // merchant | category
	Key     string      `json:"key"`
	Name    string      `json:"name"`
	Year    money.Cents `json:"year"`  // last twelve complete months
	Usual   money.Cents `json:"usual"` // a month, average of the last six complete
	Month   money.Cents `json:"month"` // this month so far
	Verdict string      `json:"verdict,omitempty"`
	Suggest string      `json:"suggest,omitempty"` // worth | cut, when the data leans one way
	Why     string      `json:"why,omitempty"`
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

// A regular place: three purchases or €100 in the twelve months.
const regularMin = money.Cents(100 * 100)

func judge(rows []optRow, r *OptionalReport, verdicts map[string]insights.Verdict, merName map[string]string, complete []string, thisM string) Judge {
	j := Judge{Items: []JudgeItem{}, Cutting: []FollowUp{}}
	inYear, recent6 := map[string]bool{}, map[string]bool{}
	for i, m := range complete {
		inYear[m] = true
		if i >= len(complete)-6 {
			recent6[m] = true
		}
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

	small, creep := map[string]bool{}, map[string]bool{}
	for _, s := range r.Leaks.SmallOften {
		small[merchant.Key(s.Merchant)] = true
	}
	for _, c := range r.Leaks.Creeping {
		creep[merchant.Key(c.Merchant)] = true
	}
	item := func(kind, key, name string) JudgeItem {
		it := JudgeItem{Kind: kind, Key: key, Name: name}
		in := match(kind, key)
		var over, oneOff, fund, trip money.Cents
		months := map[string]bool{}
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
			if recent6[o.month] {
				it.Usual += o.Amount
			}
			if o.over {
				over += o.Amount
			}
			if o.Amount >= OneOffMin && !o.fund && !hasTrip(o.Tags) {
				oneOff += o.Amount
			}
			if o.fund {
				fund += o.Amount
			}
			if hasTrip(o.Tags) {
				trip += o.Amount
			}
		}
		it.Usual /= 6
		share := func(v money.Cents) bool { return it.Year > 0 && v*10 >= it.Year*3 } // ≥ 30%
		most := func(v money.Cents) bool { return it.Year > 0 && v*2 >= it.Year }     // ≥ 50%
		switch {
		case kind == "merchant" && small[key]:
			it.Suggest, it.Why = "cut", "small and often"
		case kind == "merchant" && creep[key]:
			it.Suggest, it.Why = "cut", "creeping up"
		case share(over):
			it.Suggest, it.Why = "cut", pctOf(over, it.Year)+" of it after the month's money ran out"
		case most(oneOff):
			it.Suggest, it.Why = "cut", "mostly big one-offs"
		case most(fund):
			it.Suggest, it.Why = "worth", "planned — paid from a fund"
		case most(trip):
			it.Suggest, it.Why = "worth", "part of trips"
		case len(months) >= 8:
			it.Suggest, it.Why = "worth", "a steady part of your months"
		}
		if v, ok := verdicts[kind+":"+key]; ok {
			it.Verdict = v.Verdict
		}
		return it
	}

	// Regular places, then the biggest categories, then anything judged
	// that fell out of both lists.
	type agg struct {
		year money.Cents
		buys map[int64]bool
	}
	places := map[string]*agg{}
	for _, o := range rows {
		k := merchant.Key(o.Merchant)
		if !o.optional || k == "" || !inYear[o.month] {
			continue
		}
		if places[k] == nil {
			places[k] = &agg{buys: map[int64]bool{}}
		}
		places[k].year += o.Amount
		places[k].buys[o.purchase()] = true
	}
	var keys []string
	for k, a := range places {
		if len(a.buys) >= 3 || a.year >= regularMin {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, k int) bool {
		if places[keys[i]].year != places[keys[k]].year {
			return places[keys[i]].year > places[keys[k]].year
		}
		return keys[i] < keys[k]
	})
	seen := map[string]bool{}
	for _, k := range keys[:min(12, len(keys))] {
		j.Items = append(j.Items, item("merchant", k, merName[k]))
		seen["merchant:"+k] = true
	}
	for _, c := range r.Categories {
		j.Items = append(j.Items, item("category", c.Key, c.Key))
		seen["category:"+c.Key] = true
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

func pctOf(part, whole money.Cents) string {
	if whole <= 0 {
		return "0%"
	}
	return strconv.FormatInt(int64(part*100/whole), 10) + "%"
}
