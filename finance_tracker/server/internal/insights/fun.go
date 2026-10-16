package insights

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"ft/internal/ledger"
	"ft/internal/merchant"
	"ft/internal/money"
)

// Fact is one fun statistic from the whole history: a headline figure, a
// line of context, and optionally a short ranking.
type Fact struct {
	Key    string     `json:"key"`
	Icon   string     `json:"icon"`
	Title  string     `json:"title"`
	Value  string     `json:"value"`
	Detail string     `json:"detail,omitempty"`
	Items  []FactItem `json:"items,omitempty"`
}

type FactItem struct {
	Label string  `json:"label"`
	Value string  `json:"value"`
	Sub   string  `json:"sub,omitempty"`
	N     float64 `json:"n,omitempty"`    // the number behind Value, for bars
	From  string  `json:"from,omitempty"` // a span (employers: first and last year)
	To    string  `json:"to,omitempty"`
	Top   bool    `json:"top,omitempty"` // the highlighted one
}

// obligation: money that isn't a choice (alimony, the mortgage, family
// support) — left out of the fun.
func obligation(cat string) bool {
	return cat == "kids.alimony" || cat == "other.family" || strings.HasPrefix(cat, "housing.mortgage") || ledger.Top(cat) == "finance"
}

// bill: things that arrive by themselves — not "visits".
func bill(cat string) bool {
	switch ledger.Top(cat) {
	case "utilities", "subscriptions", "housing", "finance":
		return true
	}
	return false
}

// FunFacts digs through the whole ledger for the stories in it.
func FunFacts(txs []ledger.Tx, cats map[string]ledger.Category, now time.Time) []Fact {
	var out []Fact
	eur := func(c money.Cents) string { return eurWhole(c) }
	day := func(d string) string {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			return d
		}
		return t.Format("2 Jan 2006")
	}
	year := func(d string) string { return d[:4] }

	// Who paid the most.
	type payer struct {
		name        string
		total       money.Cents
		first, last string
		n           int
	}
	payers := map[string]*payer{}
	var firstPay *ledger.Tx
	var income money.Cents
	firstDate := ""
	for i := range txs {
		t := &txs[i]
		if firstDate == "" || t.Date < firstDate {
			firstDate = t.Date
		}
		if t.Kind != "income" {
			continue
		}
		income += t.Amount
		if ledger.Top(t.Category) != "salary" || t.Merchant == "" {
			continue
		}
		if firstPay == nil || t.Date < firstPay.Date {
			firstPay = t
		}
		k := merchant.Fold(t.Merchant)
		p := payers[k]
		if p == nil {
			p = &payer{name: t.Merchant, first: t.Date, last: t.Date}
			payers[k] = p
		}
		p.total += t.Amount
		p.n++
		if t.Date < p.first {
			p.first = t.Date
		}
		if t.Date > p.last {
			p.last, p.name = t.Date, t.Merchant
		}
	}
	if len(payers) > 0 {
		ps := make([]*payer, 0, len(payers))
		for _, p := range payers {
			ps = append(ps, p)
		}
		sort.Slice(ps, func(i, j int) bool { return ps[i].total > ps[j].total })
		f := Fact{Key: "employers", Icon: "briefcase", Title: "Who paid you the most", Value: ps[0].name,
			Detail: fmt.Sprintf("%s over %s–%s, in %d payments.", eur(ps[0].total), year(ps[0].first), year(ps[0].last), ps[0].n)}
		for _, p := range ps[:min(6, len(ps))] {
			span := year(p.first)
			if year(p.last) != span {
				span += "–" + year(p.last)
			}
			f.Items = append(f.Items, FactItem{Label: p.name, Value: eur(p.total), Sub: span, N: p.total.Float(), From: year(p.first), To: year(p.last)})
		}
		out = append(out, f)
	}
	if firstPay != nil {
		var recent money.Cents // the latest full month of salary
		months := map[string]money.Cents{}
		for i := range txs {
			if txs[i].Kind == "income" && ledger.Top(txs[i].Category) == "salary" {
				months[txs[i].Date[:7]] += txs[i].Amount
			}
		}
		for i := 1; i <= 3 && recent == 0; i++ {
			recent = months[now.AddDate(0, -i, 0).Format("2006-01")]
		}
		f := Fact{Key: "first_pay", Icon: "piggy", Title: "Your first pay cheque", Value: eur(firstPay.Amount),
			Detail: fmt.Sprintf("From %s on %s.", firstPay.Merchant, day(firstPay.Date))}
		if recent > 0 && firstPay.Amount > 0 {
			f.Detail += fmt.Sprintf(" A month's salary is now %s — %.0f× that.", eur(recent), recent.Float()/firstPay.Amount.Float())
		}
		out = append(out, f)
	}
	if income > 0 && firstDate != "" {
		f := Fact{Key: "lifetime", Icon: "trend", Title: "Earned since records began", Value: eur(income),
			Detail: fmt.Sprintf("Since %s, across %s transactions.", day(firstDate), thousandsInt(len(txs)))}
		if t, err := time.Parse("2006-01-02", firstDate); err == nil {
			f.Items = []FactItem{
				{Label: "years", Value: fmt.Sprintf("%.0f", now.Sub(t).Hours()/24/365.25), N: math.Floor(now.Sub(t).Hours() / 24 / 365.25), From: year(firstDate)},
				{Label: "transactions", Value: thousandsInt(len(txs)), N: float64(len(txs))},
				{Label: "earned", Value: eur(income), N: income.Float()},
			}
		}
		out = append(out, f)
	}

	// Shops: most visited, most spent.
	type shop = funShop
	shops := map[string]*shop{}
	byCat := map[string]map[string]*shop{}
	weekday := map[time.Weekday]money.Cents{}
	days := map[string]money.Cents{}
	var biggest *ledger.Tx
	catCount := map[string]int{}
	catSum := map[string]money.Cents{}
	trips := map[string]money.Cents{}
	tripDays := map[string][2]string{}
	since := now.AddDate(-1, 0, 0).Format("2006-01-02")
	// People paid obligations (an ex-partner, family) aren't shops.
	people := map[string]bool{}
	for i := range txs {
		if c := txs[i].Category; txs[i].Kind == "expense" && txs[i].Merchant != "" && (c == "kids.alimony" || c == "other.family" || strings.HasPrefix(c, "housing.mortgage")) {
			people[merchant.Fold(txs[i].Merchant)] = true
		}
	}
	for i := range txs {
		t := &txs[i]
		k := merchant.Fold(t.Merchant)
		if t.Kind != "expense" || obligation(t.Category) || (t.Merchant != "" && people[k]) {
			continue
		}
		days[t.Date] += t.Amount
		catCount[t.Category]++
		catSum[t.Category] += t.Amount
		for _, tag := range t.Tags {
			if ledger.TripTag(tag) {
				trips[tag] += t.Amount
				r := tripDays[tag]
				if r[0] == "" || t.Date < r[0] {
					r[0] = t.Date
				}
				if t.Date > r[1] {
					r[1] = t.Date
				}
				tripDays[tag] = r
			}
		}
		if biggest == nil || t.Amount > biggest.Amount {
			biggest = t
		}
		if d, err := time.Parse("2006-01-02", t.Date); err == nil && t.Amount < money.FromFloat(250) {
			weekday[d.Weekday()] += t.Amount
		}
		if t.Merchant == "" || bill(t.Category) {
			continue
		}
		s := shops[k]
		if s == nil {
			s = &shop{name: t.Merchant, cat: t.Category}
			shops[k] = s
		}
		s.n++
		s.total += t.Amount
		if t.Date >= since {
			s.recent++
		}
		leaf := t.Category
		if byCat[leaf] == nil {
			byCat[leaf] = map[string]*shop{}
		}
		if byCat[leaf][k] == nil {
			byCat[leaf][k] = &shop{name: t.Merchant}
		}
		byCat[leaf][k].n++
		byCat[leaf][k].total += t.Amount
	}
	list := make([]*shop, 0, len(shops))
	for _, s := range shops {
		list = append(list, s)
	}
	if len(list) > 0 {
		sort.Slice(list, func(i, j int) bool {
			return list[i].n > list[j].n || list[i].n == list[j].n && list[i].total > list[j].total
		})
		f := Fact{Key: "visits", Icon: "bag", Title: "Most visited", Value: list[0].name, Detail: fmt.Sprintf("%d times, %s in all.", list[0].n, eur(list[0].total))}
		for _, s := range list[:min(5, len(list))] {
			f.Items = append(f.Items, FactItem{Label: s.name, Value: fmt.Sprintf("%d×", s.n), Sub: eur(s.total), N: float64(s.n)})
		}
		out = append(out, f)
		sort.Slice(list, func(i, j int) bool { return list[i].total > list[j].total })
		f = Fact{Key: "spent_most", Icon: "coins", Title: "Where the most money went", Value: list[0].name, Detail: fmt.Sprintf("%s over %d payments.", eur(list[0].total), list[0].n)}
		for _, s := range list[:min(5, len(list))] {
			f.Items = append(f.Items, FactItem{Label: s.name, Value: eur(s.total), Sub: fmt.Sprintf("%d×", s.n), N: s.total.Float()})
		}
		out = append(out, f)
		// This year's favourite: most visits in the last 12 months.
		sort.Slice(list, func(i, j int) bool { return list[i].recent > list[j].recent })
		if list[0].recent >= 5 {
			out = append(out, Fact{Key: "this_year", Icon: "heart", Title: "This year's favourite", Value: list[0].name,
				Detail: fmt.Sprintf("%d visits in the last 12 months — about one every %d days.", list[0].recent, max(1, 365/list[0].recent))})
		}
	}

	// Habits: a few everyday categories with counts and favourites.
	var habits []FactItem
	for _, h := range []struct{ cat, label string }{
		{"food.coffee", "Coffees"}, {"food.fast_food", "Fast-food stops"}, {"food.restaurants", "Restaurant meals"},
		{"food.delivery", "Deliveries"}, {"transport.fuel", "Fuel fill-ups"}, {"transport.parking", "Parking tickets"}, {"leisure.lottery", "Lottery tickets"},
	} {
		n := catCount[h.cat]
		if n < 3 {
			continue
		}
		sub := fmt.Sprintf("%s · about %s each", eur(catSum[h.cat]), eur(catSum[h.cat]/money.Cents(n)))
		if fav := topShop(byCat[h.cat]); fav != "" {
			sub += " · mostly " + fav
		}
		habits = append(habits, FactItem{Label: h.label, Value: thousandsInt(n), Sub: sub, N: float64(n)})
	}
	if len(habits) > 0 {
		out = append(out, Fact{Key: "habits", Icon: "utensils", Title: "Little habits, added up", Value: habits[0].Value + " " + strings.ToLower(habits[0].Label), Items: habits})
	}

	// Records.
	if biggest != nil {
		name := biggest.Merchant
		if name == "" {
			name = biggest.Note
		}
		out = append(out, Fact{Key: "biggest", Icon: "alert", Title: "Biggest single purchase", Value: eur(biggest.Amount),
			Detail: fmt.Sprintf("%s · %s · %s.", name, catLabel(cats, biggest.Category), day(biggest.Date))})
	}
	if len(days) > 0 {
		best, bestV := "", money.Cents(0)
		for d, v := range days {
			if v > bestV || v == bestV && d < best {
				best, bestV = d, v
			}
		}
		// The same day as the biggest purchase says nothing new.
		if biggest == nil || best != biggest.Date {
			out = append(out, Fact{Key: "big_day", Icon: "calendar", Title: "Most expensive day", Value: day(best), Detail: eur(bestV) + " out the door in one day."})
		}
	}
	if len(weekday) > 0 {
		var ds []time.Weekday
		for d := range weekday {
			ds = append(ds, d)
		}
		sort.Slice(ds, func(i, j int) bool { return weekday[ds[i]] > weekday[ds[j]] })
		var total money.Cents
		for _, v := range weekday {
			total += v
		}
		f := Fact{Key: "weekday", Icon: "calendar", Title: "Your big spending day", Value: ds[0].String(),
			Detail: fmt.Sprintf("%.0f%% of everyday spending lands on a %s; the quietest day is %s.", 100*weekday[ds[0]].Float()/total.Float(), ds[0], ds[len(ds)-1])}
		for _, d := range []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday} {
			f.Items = append(f.Items, FactItem{Label: d.String()[:3], Value: fmt.Sprintf("%.0f%%", 100*weekday[d].Float()/total.Float()), N: weekday[d].Float(), Top: d == ds[0]})
		}
		out = append(out, f)
	}
	if len(trips) > 0 {
		var ts []string
		for k := range trips {
			ts = append(ts, k)
		}
		sort.Slice(ts, func(i, j int) bool { return trips[ts[i]] > trips[ts[j]] })
		f := Fact{Key: "trips", Icon: "plane", Title: "Trips", Value: fmt.Sprintf("%d tagged", len(ts)), Detail: "The priciest ones:"}
		for _, k := range ts[:min(4, len(ts))] {
			f.Items = append(f.Items, FactItem{Label: tripName(k), Value: eur(trips[k]), Sub: year(tripDays[k][1]), N: trips[k].Float()}) // when it happened, not when booked
		}
		out = append(out, f)
	}

	// Spending by year: how life grew.
	yearly := map[string]money.Cents{}
	for d, v := range days {
		yearly[d[:4]] += v
	}
	var ys []string
	for y := range yearly {
		if yearly[y] > money.FromFloat(1000) && y < now.Format("2006") {
			ys = append(ys, y)
		}
	}
	sort.Strings(ys)
	if len(ys) >= 3 {
		hi := ys[0]
		for _, y := range ys {
			if yearly[y] > yearly[hi] {
				hi = y
			}
		}
		f := Fact{Key: "years", Icon: "chart", Title: "The biggest spending year", Value: hi,
			Detail: fmt.Sprintf("%s, against %s in %s — choices only, obligations left out.", eur(yearly[hi]), eur(yearly[ys[0]]), ys[0])}
		for _, y := range ys {
			f.Items = append(f.Items, FactItem{Label: y, Value: eur(yearly[y]), N: yearly[y].Float(), Top: y == hi})
		}
		out = append(out, f)
	}
	return out
}

type funShop struct {
	name   string
	n      int
	total  money.Cents
	cat    string
	recent int
}

// topShop is the most visited merchant among a category's (at least 3 visits).
func topShop(m map[string]*funShop) string {
	best, bn := "", 2
	for _, s := range m {
		if s.n > bn || s.n == bn && best != "" && s.name < best {
			best, bn = s.name, s.n
		}
	}
	return best
}

func tripName(tag string) string {
	s := strings.TrimPrefix(tag, "trip:")
	s = strings.ReplaceAll(s, "-", " ")
	if s == "" {
		return tag
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func catLabel(cats map[string]ledger.Category, id string) string {
	if c, ok := cats[id]; ok {
		if p, ok := cats[ledger.Top(id)]; ok && ledger.Top(id) != id {
			return p.Name + " · " + c.Name
		}
		return c.Name
	}
	return id
}

func eurWhole(c money.Cents) string {
	v := (c.Abs() + 50) / 100
	s := fmt.Sprintf("%d", v)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return "€" + s
}

func thousandsInt(n int) string {
	s := fmt.Sprintf("%d", n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
