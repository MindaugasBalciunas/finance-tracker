package insights

import (
	"fmt"
	"math"
	"sort"
	"strconv"
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
	Link   string     `json:"link,omitempty"` // the transactions (or page) behind the headline
}

type FactItem struct {
	Label string  `json:"label"`
	Value string  `json:"value"`
	Sub   string  `json:"sub,omitempty"`
	N     float64 `json:"n,omitempty"`    // the number behind Value, for bars
	From  string  `json:"from,omitempty"` // a span (employers: first and last year)
	To    string  `json:"to,omitempty"`
	Top   bool    `json:"top,omitempty"`   // the highlighted one
	Years []int   `json:"years,omitempty"` // employers: each year they actually paid (gaps stay gaps)
	Link  string  `json:"link,omitempty"`  // the transactions behind this row
}

// obligation: money that isn't a choice (alimony, the mortgage, family
// support) — left out of the fun.
func obligation(cat string) bool { return ledger.Obligation(cat) }

// bill: things that arrive by themselves — not "visits".
func bill(cat string) bool {
	switch ledger.Top(cat) {
	case "utilities", "subscriptions", "housing", "finance":
		return true
	}
	return false
}

// FunFacts digs through the ledger for the stories in it — the whole
// history, or the days between from and to (inclusive; "" = open). In a
// window the lifetime-only facts (the first pay cheque, this year's
// favourite) are left out and the story counts days instead of years.
func FunFacts(txs []ledger.Tx, cats map[string]ledger.Category, now time.Time, from, to string) []Fact {
	windowed := from != "" || to != ""
	all := txs // the whole history: "new places" and "the period before" need it
	if windowed {
		in := make([]ledger.Tx, 0, len(txs))
		for _, t := range txs {
			if (from == "" || t.Date >= from) && (to == "" || t.Date <= to) {
				in = append(in, t)
			}
		}
		txs = in
	}
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
	// The period the facts cover, as Ledger filters.
	win := func(kv ...string) string { return ledger.Link(append([]string{"from", from, "to", to}, kv...)...) }
	dayLink := func(d string, kv ...string) string {
		return ledger.Link(append([]string{"from", d, "to", d}, kv...)...)
	}
	// Choices in a span, kept inside the window: obligations left out, as the figures do.
	choices := func(a, b string) string {
		if from > a {
			a = from
		}
		if to != "" && to < b {
			b = to
		}
		return ledger.Link("from", a, "to", b, "kind", "expense", "choices", "1")
	}

	// Who paid the most.
	type payer struct {
		name        string
		total       money.Cents
		first, last string
		n           int
		years       map[int]bool
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
			p = &payer{name: t.Merchant, first: t.Date, last: t.Date, years: map[int]bool{}}
			payers[k] = p
		}
		if y, err := strconv.Atoi(t.Date[:4]); err == nil {
			p.years[y] = true
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
		if year(ps[0].first) == year(ps[0].last) {
			f.Detail = fmt.Sprintf("%s in %s, in %d payments.", eur(ps[0].total), year(ps[0].first), ps[0].n)
		}
		f.Link = win("kind", "income", "category", "salary", "merchant", ps[0].name)
		for _, p := range ps[:min(6, len(ps))] {
			span := year(p.first)
			if year(p.last) != span {
				span += "–" + year(p.last)
			}
			var ys []int
			for y := range p.years {
				ys = append(ys, y)
			}
			sort.Ints(ys)
			f.Items = append(f.Items, FactItem{Label: p.name, Value: eur(p.total), Sub: span, N: p.total.Float(), From: year(p.first), To: year(p.last), Years: ys,
				Link: win("kind", "income", "category", "salary", "merchant", p.name)})
		}
		out = append(out, f)
	}
	if firstPay != nil && !windowed {
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
			Detail: fmt.Sprintf("From %s on %s.", firstPay.Merchant, day(firstPay.Date)), Link: dayLink(firstPay.Date, "kind", "income")}
		if recent > 0 && firstPay.Amount > 0 {
			f.Detail += fmt.Sprintf(" A month's salary is now %s — %.0f× that.", eur(recent), recent.Float()/firstPay.Amount.Float())
		}
		out = append(out, f)
	}
	if income > 0 && firstDate != "" {
		f := Fact{Key: "lifetime", Icon: "trend", Title: "Earned since records began", Value: eur(income), Link: win("kind", "income"),
			Detail: fmt.Sprintf("Since %s, across %s transactions.", day(firstDate), thousandsInt(len(txs)))}
		if windowed {
			end := now.Format("2006-01-02")
			if to != "" && to < end {
				end = to
			}
			start := firstDate
			if from != "" {
				start = from
			}
			a, _ := time.Parse("2006-01-02", start)
			b, _ := time.Parse("2006-01-02", end)
			days := int(b.Sub(a).Hours()/24) + 1
			f.Title = "Earned in this period"
			f.Detail = fmt.Sprintf("%s – %s, across %s transactions.", day(start), day(end), thousandsInt(len(txs)))
			f.Items = []FactItem{
				{Label: "days", Value: thousandsInt(days), N: float64(days), From: start, To: end},
				{Label: "transactions", Value: thousandsInt(len(txs)), N: float64(len(txs))},
				{Label: "earned", Value: eur(income), N: income.Float()},
			}
		} else if t, err := time.Parse("2006-01-02", firstDate); err == nil {
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
	// (Over the whole history, as the Ledger's "choices" filter has it.)
	people := map[string]bool{}
	for i := range all {
		if all[i].Kind == "expense" && all[i].Merchant != "" && ledger.PaysAPerson(all[i].Category) {
			people[merchant.Key(all[i].Merchant)] = true
		}
	}
	for i := range txs {
		t := &txs[i]
		k := merchant.Fold(t.Merchant)
		if t.Kind != "expense" || obligation(t.Category) || (t.Merchant != "" && people[merchant.Key(t.Merchant)]) {
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
		f := Fact{Key: "visits", Icon: "bag", Title: "Most visited", Value: list[0].name, Detail: fmt.Sprintf("%d times, %s in all.", list[0].n, eur(list[0].total)),
			Link: win("kind", "expense", "merchant", list[0].name)}
		for _, s := range list[:min(5, len(list))] {
			f.Items = append(f.Items, FactItem{Label: s.name, Value: fmt.Sprintf("%d×", s.n), Sub: eur(s.total), N: float64(s.n), Link: win("kind", "expense", "merchant", s.name)})
		}
		out = append(out, f)
		sort.Slice(list, func(i, j int) bool { return list[i].total > list[j].total })
		f = Fact{Key: "spent_most", Icon: "coins", Title: "Where the most money went", Value: list[0].name, Detail: fmt.Sprintf("%s over %d payments.", eur(list[0].total), list[0].n),
			Link: win("kind", "expense", "merchant", list[0].name)}
		for _, s := range list[:min(5, len(list))] {
			f.Items = append(f.Items, FactItem{Label: s.name, Value: eur(s.total), Sub: fmt.Sprintf("%d×", s.n), N: s.total.Float(), Link: win("kind", "expense", "merchant", s.name)})
		}
		out = append(out, f)
		// This year's favourite: most visits in the last 12 months.
		sort.Slice(list, func(i, j int) bool { return list[i].recent > list[j].recent })
		if list[0].recent >= 5 && !windowed {
			out = append(out, Fact{Key: "this_year", Icon: "heart", Title: "This year's favourite", Value: list[0].name,
				Detail: fmt.Sprintf("%d visits in the last 12 months — about one every %d days.", list[0].recent, max(1, 365/list[0].recent)),
				Link:   ledger.Link("from", since, "kind", "expense", "merchant", list[0].name)})
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
		habits = append(habits, FactItem{Label: h.label, Value: thousandsInt(n), Sub: sub, N: float64(n), Link: win("kind", "expense", "category", h.cat)})
	}
	if len(habits) > 0 {
		out = append(out, Fact{Key: "habits", Icon: "utensils", Title: "Little habits, added up", Value: habits[0].Value + " " + strings.ToLower(habits[0].Label), Items: habits, Link: habits[0].Link})
	}

	// Records.
	if biggest != nil {
		name := biggest.Merchant
		if name == "" {
			name = biggest.Note
		}
		out = append(out, Fact{Key: "biggest", Icon: "alert", Title: "Biggest single purchase", Value: eur(biggest.Amount),
			Detail: fmt.Sprintf("%s · %s · %s.", name, catLabel(cats, biggest.Category), day(biggest.Date)), Link: dayLink(biggest.Date, "kind", "expense", "category", biggest.Category)})
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
			out = append(out, Fact{Key: "big_day", Icon: "calendar", Title: "Most expensive day", Value: day(best), Detail: eur(bestV) + " out the door in one day.", Link: dayLink(best, "kind", "expense", "choices", "1")})
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
		f := Fact{Key: "trips", Icon: "plane", Title: "Trips", Value: fmt.Sprintf("%d tagged", len(ts)), Detail: "The priciest ones:", Link: "/plan/trips"}
		for _, k := range ts[:min(4, len(ts))] {
			// The whole trip: bookings made before the window belong to it too.
			f.Items = append(f.Items, FactItem{Label: tripName(k), Value: eur(trips[k]), Sub: year(tripDays[k][1]), N: trips[k].Float(), Link: ledger.Link("tag", k)}) // when it happened, not when booked
		}
		out = append(out, f)
	}

	out = append(out, periodFacts(all, txs, now, from, to, windowed, eur, day)...)

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
		f := Fact{Key: "years", Icon: "chart", Title: "The biggest spending year", Value: hi, Link: choices(hi+"-01-01", hi+"-12-31"),
			Detail: fmt.Sprintf("%s, against %s in %s — choices only, obligations left out.", eur(yearly[hi]), eur(yearly[ys[0]]), ys[0])}
		for _, y := range ys {
			f.Items = append(f.Items, FactItem{Label: y, Value: eur(yearly[y]), N: yearly[y].Float(), Top: y == hi, Link: choices(y+"-01-01", y+"-12-31")})
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

// choice: a payment that was a choice (not an obligation, not a bill).
func choice(t *ledger.Tx) bool {
	return t.Kind == "expense" && !obligation(t.Category)
}

// periodFacts are the facts that read well for any period: the rhythm of
// spending days, the big and quiet months, new places, the signature price,
// the smallest purchase, the biggest money back, what was invested, and how
// the period compares with the one before it.
func periodFacts(all, txs []ledger.Tx, now time.Time, from, to string, windowed bool, eur func(money.Cents) string, day func(string) string) []Fact {
	var out []Fact
	if len(txs) == 0 {
		return out
	}
	start, end := from, to
	if start == "" {
		start = txs[0].Date
		for _, t := range txs {
			if t.Date < start {
				start = t.Date
			}
		}
	}
	if end == "" || end > now.Format("2006-01-02") {
		end = now.Format("2006-01-02")
	}
	// For all time, start where the records get dense (the first month with
	// 15+ payments) — early years with a handful of rows say nothing about habits.
	if !windowed {
		perMonth := map[string]int{}
		for i := range txs {
			if choice(&txs[i]) {
				perMonth[txs[i].Date[:7]]++
			}
		}
		var ms []string
		for m, n := range perMonth {
			if n >= 15 {
				ms = append(ms, m)
			}
		}
		sort.Strings(ms)
		if len(ms) > 0 && ms[0]+"-01" > start {
			start = ms[0] + "-01"
			kept := txs[:0:0]
			for _, t := range txs {
				if t.Date >= start {
					kept = append(kept, t)
				}
			}
			txs = kept
		}
	}
	a, _ := time.Parse("2006-01-02", start)
	b, _ := time.Parse("2006-01-02", end)
	span := int(b.Sub(a).Hours()/24) + 1
	if span < 1 {
		return out
	}
	// People paid obligations are never shops, refunds or new places.
	people := map[string]bool{}
	for i := range all {
		if all[i].Kind == "expense" && all[i].Merchant != "" && ledger.PaysAPerson(all[i].Category) {
			people[placeKey(all[i].Merchant)] = true
		}
	}

	// Rhythm: days with a choice payment, and the longest break.
	days := map[string]bool{}
	var spent money.Cents
	months := map[string]money.Cents{}
	amounts := map[money.Cents]int{}
	amountAt := map[money.Cents]map[string]bool{}
	var tiny *ledger.Tx
	for i := range txs {
		t := &txs[i]
		if !choice(t) || (t.Merchant != "" && people[placeKey(t.Merchant)]) {
			continue
		}
		days[t.Date] = true
		spent += t.Amount
		months[t.Date[:7]] += t.Amount
		if t.Amount >= money.FromFloat(1) && t.Amount < money.FromFloat(50) {
			amounts[t.Amount]++
			if amountAt[t.Amount] == nil {
				amountAt[t.Amount] = map[string]bool{}
			}
			if t.Merchant != "" {
				amountAt[t.Amount][t.Merchant] = true
			}
		}
		if tiny == nil || t.Amount < tiny.Amount {
			tiny = t
		}
	}
	if len(days) > 0 {
		ds := make([]string, 0, len(days))
		for d := range days {
			ds = append(ds, d)
		}
		sort.Strings(ds)
		gap, gFrom, gTo := 0, "", ""
		for i := 1; i < len(ds); i++ {
			x, _ := time.Parse("2006-01-02", ds[i-1])
			y, _ := time.Parse("2006-01-02", ds[i])
			if g := int(y.Sub(x).Hours()/24) - 1; g > gap {
				gap, gFrom, gTo = g, ds[i-1], ds[i]
			}
		}
		f := Fact{Key: "rhythm", Icon: "calendar", Title: "Days you spent money", Value: fmt.Sprintf("%s of %s", thousandsInt(len(days)), thousandsInt(span)),
			Detail: fmt.Sprintf("%.0f%% of days, about %s a day on choices.", 100*float64(len(days))/float64(span), eur(spent/money.Cents(span)))}
		if gap >= 2 {
			f.Detail += fmt.Sprintf(" Longest break: %d days (%s – %s).", gap, day(gFrom), day(gTo))
		}
		out = append(out, f)
	}

	// The big and the quiet month (complete months only, at least three).
	cur := now.Format("2006-01")
	var ms []string
	for m := range months {
		if m < cur {
			ms = append(ms, m)
		}
	}
	sort.Strings(ms)
	if len(ms) >= 3 && len(ms) <= 36 {
		hi, lo := ms[0], ms[0]
		for _, m := range ms {
			if months[m] > months[hi] {
				hi = m
			}
			if months[m] < months[lo] {
				lo = m
			}
		}
		mName := func(m string) string { t, _ := time.Parse("2006-01", m); return t.Format("January 2006") }
		inWin := func(m string) string { // the month's choices, inside the period
			a, b := m+"-01", m+"-31"
			if t, err := time.Parse("2006-01", m); err == nil {
				b = t.AddDate(0, 1, -1).Format("2006-01-02")
			}
			if a < start {
				a = start
			}
			if b > end {
				b = end
			}
			return ledger.Link("from", a, "to", b, "kind", "expense", "choices", "1")
		}
		f := Fact{Key: "months", Icon: "chart", Title: "Big month, quiet month", Value: mName(hi), Link: inWin(hi),
			Detail: fmt.Sprintf("%s on choices, against %s in %s.", eur(months[hi]), eur(months[lo]), mName(lo))}
		for _, m := range ms {
			t, _ := time.Parse("2006-01", m)
			f.Items = append(f.Items, FactItem{Label: t.Format("Jan"), Sub: m, Value: eur(months[m]), N: months[m].Float(), Top: m == hi, Link: inWin(m)})
		}
		out = append(out, f)
	}

	// New places: merchants first paid in this period.
	if windowed {
		first := map[string]string{}
		name := map[string]string{}
		count := map[string]int{}
		for i := range all {
			t := &all[i]
			if t.Kind != "expense" || t.Merchant == "" {
				continue
			}
			k := placeKey(t.Merchant)
			if d, ok := first[k]; !ok || t.Date < d {
				first[k] = t.Date // the first payment of any kind
			}
			if t.Date >= start && t.Date <= end && choice(t) && !bill(t.Category) && !people[k] {
				count[k]++
				name[k] = t.Merchant
			}
		}
		var fresh []string
		for k, d := range first {
			if d >= start && d <= end && count[k] > 0 {
				fresh = append(fresh, k)
			}
		}
		if len(fresh) > 0 {
			sort.Slice(fresh, func(i, j int) bool {
				return count[fresh[i]] > count[fresh[j]] || count[fresh[i]] == count[fresh[j]] && first[fresh[i]] < first[fresh[j]]
			})
			f := Fact{Key: "new_places", Icon: "spark", Title: "New places", Value: fmt.Sprintf("%d you'd never paid before", len(fresh))}
			for _, k := range fresh[:min(5, len(fresh))] {
				f.Items = append(f.Items, FactItem{Label: name[k], Value: fmt.Sprintf("%d×", count[k]), Sub: "first on " + day(first[k]), N: float64(count[k]),
					Link: ledger.Link("from", start, "to", end, "kind", "expense", "merchant", name[k])})
			}
			out = append(out, f)
		}
	}

	// The signature price: the amount paid most often.
	if len(amounts) > 0 {
		var best money.Cents
		for a, n := range amounts {
			if n > amounts[best] || n == amounts[best] && a < best {
				best = a
			}
		}
		if amounts[best] >= 3 {
			var where []string
			for m := range amountAt[best] {
				where = append(where, m)
			}
			sort.Strings(where)
			if len(where) > 3 {
				where = where[:3]
			}
			f := Fact{Key: "signature", Icon: "coins", Title: "Your signature price", Value: "€" + best.String(),
				Detail: fmt.Sprintf("Paid exactly that %d times", amounts[best])}
			if len(where) > 0 {
				f.Detail += " — " + strings.Join(where, ", ")
			}
			f.Detail += "."
			out = append(out, f)
		}
	}
	if tiny != nil && tiny.Amount < money.FromFloat(1) {
		out = append(out, Fact{Key: "tiny", Icon: "coins", Title: "Smallest payment", Value: "€" + tiny.Amount.String(),
			Detail: fmt.Sprintf("%s, %s.", firstNonEmpty(tiny.Merchant, tiny.Note, "a payment"), day(tiny.Date)), Link: ledger.Link("from", tiny.Date, "to", tiny.Date, "kind", "expense")})
	}

	// The biggest money back, and what went into wealth.
	var refund *ledger.Tx
	var invested money.Cents
	for i := range txs {
		t := &txs[i]
		if t.Kind == "income" && ledger.Top(t.Category) == "refunds" && !people[placeKey(t.Merchant)] && (refund == nil || t.Amount > refund.Amount) {
			refund = t
		}
		if t.Category == "transfer.invest" || t.Category == "transfer.pension" || t.Category == "transfer.debt" {
			invested += money.Cents(t.WealthDirection()) * t.Amount
		}
	}
	if refund != nil && refund.Amount >= money.FromFloat(20) {
		out = append(out, Fact{Key: "refund", Icon: "piggy", Title: "Biggest money back", Value: eur(refund.Amount),
			Detail: fmt.Sprintf("%s, %s.", firstNonEmpty(refund.Merchant, refund.Note, "a refund"), day(refund.Date)), Link: ledger.Link("from", refund.Date, "to", refund.Date, "category", "refunds")})
	}
	if invested > 0 {
		out = append(out, Fact{Key: "invested", Icon: "trend", Title: "Paid yourself first", Value: eur(invested),
			Detail: fmt.Sprintf("Into investments, pension and loan principal — about %s a day.", eur(invested/money.Cents(span))),
			Link:   ledger.Link("from", start, "to", end, "category", "transfer.invest", "category", "transfer.pension", "category", "transfer.debt")})
	}

	// The period against the same length before it.
	if windowed && from != "" {
		pTo := a.AddDate(0, 0, -1)
		pFrom := pTo.AddDate(0, 0, -(span - 1))
		var before money.Cents
		for i := range all {
			t := &all[i]
			if choice(t) && t.Date >= pFrom.Format("2006-01-02") && t.Date <= pTo.Format("2006-01-02") {
				before += t.Amount
			}
		}
		if before > 0 {
			ch := (spent.Float() - before.Float()) / before.Float()
			word := "more"
			if ch < 0 {
				word = "less"
			}
			out = append(out, Fact{Key: "vs_before", Icon: "trend", Title: "Against the period before", Value: fmt.Sprintf("%.0f%% %s", math.Abs(ch)*100, word),
				Detail: fmt.Sprintf("%s on choices, against %s in the %d days before (%s – %s).", eur(spent), eur(before), span, day(pFrom.Format("2006-01-02")), day(pTo.Format("2006-01-02")))})
		}
	}
	return out
}

// placeKey folds a merchant name for matching, spaces ignored ("360 Arena" = "360arena").
func placeKey(m string) string { return merchant.Key(m) }
