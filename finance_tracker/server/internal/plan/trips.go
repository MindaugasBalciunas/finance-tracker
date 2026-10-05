package plan

import (
	"sort"
	"strings"
	"time"

	"ft/internal/ledger"
	"ft/internal/money"
)

// Trips group one holiday's transactions under a trip:… tag, so a trip is
// costed as a whole (total, per day, lodging vs flights vs food) while the
// category keeps saying what each row was.

type NamedAmount struct {
	Name   string      `json:"name"`
	Amount money.Cents `json:"amount"`
}

type Trip struct {
	Tag        string        `json:"tag"`  // trip:zakopane
	Name       string        `json:"name"` // zakopane
	From       string        `json:"from"`
	To         string        `json:"to"`
	Days       int           `json:"days"`
	Count      int           `json:"count"`
	Total      money.Cents   `json:"total"`                 // expenses − refunds
	BookedFrom string        `json:"booked_from,omitempty"` // earliest row when paid ahead of the trip
	PerDay     money.Cents   `json:"per_day"`
	ByCategory []NamedAmount `json:"by_category"`
	Budget     *money.Cents  `json:"budget,omitempty"`
}

type TripSuggestion struct {
	From  string      `json:"from"`
	To    string      `json:"to"`
	Days  int         `json:"days"`
	Count int         `json:"count"`
	Total money.Cents `json:"total"`
	TxIDs []int64     `json:"tx_ids"`
	Top   []string    `json:"top"`
}

const tripGapDays = 4

func sorted(m map[string]money.Cents) []NamedAmount {
	out := make([]NamedAmount, 0, len(m))
	for k, v := range m {
		if v != 0 {
			out = append(out, NamedAmount{k, v})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Amount > out[j].Amount })
	return out
}

func days(from, to string) int {
	a, _ := time.Parse("2006-01-02", from)
	b, _ := time.Parse("2006-01-02", to)
	return int(b.Sub(a).Hours()/24) + 1
}

// Trips summarises every trip tag, newest first, plus runs of untagged
// travel spending that look like a trip.
func Trips(txs []ledger.Tx, budgets []Budget) ([]Trip, []TripSuggestion) {
	trips := map[string]*Trip{}
	cats := map[string]map[string]money.Cents{}
	for _, t := range txs {
		for _, tag := range t.Tags {
			if !ledger.TripTag(tag) {
				continue
			}
			tr := trips[tag]
			if tr == nil {
				tr = &Trip{Tag: tag, Name: strings.TrimPrefix(tag, "trip:"), From: t.Date, To: t.Date}
				trips[tag] = tr
				cats[tag] = map[string]money.Cents{}
			}
			if t.Date < tr.From {
				tr.From = t.Date
			}
			if t.Date > tr.To {
				tr.To = t.Date
			}
			tr.Count++
			switch t.Kind {
			case "expense":
				tr.Total += t.Amount
				cats[tag][t.Category] += t.Amount
			case "income":
				tr.Total -= t.Amount
				cats[tag]["refunds"] -= t.Amount
			}
		}
	}
	for _, b := range budgets {
		if tr := trips[b.Tag]; tr != nil && b.Tag != "" {
			a := b.Amount
			tr.Budget = &a
		}
	}
	// The trip itself is the run of dates holding most of its spending;
	// flights and hotels booked months ahead stay in the total but don't
	// stretch the trip to a year.
	dates := map[string]map[string]money.Cents{}
	for _, t := range txs {
		for _, tag := range t.Tags {
			if ledger.TripTag(tag) && t.Kind == "expense" {
				if dates[tag] == nil {
					dates[tag] = map[string]money.Cents{}
				}
				dates[tag][t.Date] += t.Amount
			}
		}
	}
	var out []Trip
	for tag, tr := range trips {
		if from, to := mainRun(dates[tag]); from != "" {
			if from > tr.From {
				tr.BookedFrom, tr.From = tr.From, from
			}
			if days(to, tr.To) > tripGapDays*2+1 {
				tr.To = to // a refund weeks later is not part of the trip
			}
		}
		tr.Days = days(tr.From, tr.To)
		if tr.Days > 0 {
			tr.PerDay = tr.Total / money.Cents(tr.Days)
		}
		tr.ByCategory = sorted(cats[tag])
		out = append(out, *tr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].To > out[j].To })

	// Suggestions: untagged travel expenses clustered by date.
	var loose []ledger.Tx
	for _, t := range txs {
		if t.Kind != "expense" || ledger.Top(t.Category) != "travel" {
			continue
		}
		tagged := false
		for _, tag := range t.Tags {
			if ledger.TripTag(tag) {
				tagged = true
			}
		}
		if !tagged && t.Date >= time.Now().AddDate(-2, 0, 0).Format("2006-01-02") {
			loose = append(loose, t)
		}
	}
	sort.Slice(loose, func(i, j int) bool { return loose[i].Date < loose[j].Date })
	var sugg []TripSuggestion
	var cur *TripSuggestion
	top := map[string]money.Cents{}
	flush := func() {
		if cur != nil && cur.Count >= 2 {
			cur.Days = days(cur.From, cur.To)
			for _, n := range sorted(top) {
				if len(cur.Top) < 3 {
					cur.Top = append(cur.Top, n.Name)
				}
			}
			sugg = append(sugg, *cur)
		}
		cur = nil
		top = map[string]money.Cents{}
	}
	for _, t := range loose {
		if cur != nil && days(cur.To, t.Date) > tripGapDays+1 {
			flush()
		}
		if cur == nil {
			cur = &TripSuggestion{From: t.Date}
		}
		cur.To = t.Date
		cur.Count++
		cur.Total += t.Amount
		cur.TxIDs = append(cur.TxIDs, t.ID)
		top[firstNonEmpty(t.Merchant, t.Note)] += t.Amount
	}
	flush()
	sort.Slice(sugg, func(i, j int) bool { return sugg[i].To > sugg[j].To })
	return out, sugg
}

func mainRun(byDate map[string]money.Cents) (string, string) {
	if len(byDate) == 0 {
		return "", ""
	}
	ds := make([]string, 0, len(byDate))
	for d := range byDate {
		ds = append(ds, d)
	}
	sort.Strings(ds)
	bestFrom, bestTo, from := ds[0], ds[0], ds[0]
	var best, cur money.Cents
	for i, d := range ds {
		if i > 0 && days(ds[i-1], d) > tripGapDays*2+1 {
			cur, from = 0, d
		}
		cur += byDate[d]
		if cur > best {
			best, bestFrom, bestTo = cur, from, d
		}
	}
	return bestFrom, bestTo
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
