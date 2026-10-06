package insights

import (
	"sort"
	"strings"
	"time"

	"ft/internal/ledger"
	"ft/internal/merchant"
	"ft/internal/money"
)

// NameAmount is one row of a "top N" list.
type NameAmount struct {
	Name   string      `json:"name"`
	Amount money.Cents `json:"amount"`
	Count  int         `json:"count"`
}

// TagStat is what one tag stands for in money.
type TagStat struct {
	Tag      string        `json:"tag"`
	Trip     bool          `json:"trip"`
	Count    int           `json:"count"`
	Spent    money.Cents   `json:"spent"`  // expenses
	Income   money.Cents   `json:"income"` // income and refunds carrying the tag
	Net      money.Cents   `json:"net"`    // spent − income
	First    string        `json:"first"`
	Last     string        `json:"last"`
	Months   []money.Cents `json:"months"` // net spend, last 12 months, oldest first
	Year     money.Cents   `json:"year"`   // net spend, last 12 months
	PerMonth money.Cents   `json:"per_month"`
	Cats     []NameAmount  `json:"categories"`
	Merch    []NameAmount  `json:"merchants"`
	Rules    int           `json:"rules"` // rules that add it
}

type TagReport struct {
	Tags        []TagStat   `json:"tags"`
	TaggedShare float64     `json:"tagged_share"` // of the last 12 months' spending
	YearSpent   money.Cents `json:"year_spent"`
	Months      []string    `json:"months"`
}

// lastMonths lists the 12 months ending with now's, oldest first.
func lastMonths(now time.Time) []string {
	out := make([]string, 12)
	for i := 0; i < 12; i++ {
		out[i] = time.Date(now.Year(), now.Month()-time.Month(11-i), 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
	}
	return out
}

func topN(m map[string]*NameAmount, n int) []NameAmount {
	out := make([]NameAmount, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Amount > out[j].Amount || out[i].Amount == out[j].Amount && out[i].Name < out[j].Name })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// TagStats sums every tag: totals, its 12-month shape, and where the money went.
func TagStats(txs []ledger.Tx, rules []ledger.Rule, now time.Time) TagReport {
	months := lastMonths(now)
	idx := map[string]int{}
	for i, m := range months {
		idx[m] = i
	}
	type acc struct {
		s     TagStat
		cats  map[string]*NameAmount
		merch map[string]*NameAmount
	}
	by := map[string]*acc{}
	var yearSpent, yearTagged money.Cents
	for i := range txs {
		t := &txs[i]
		mi, inYear := idx[t.Date[:7]]
		var net money.Cents
		switch t.Kind {
		case "expense":
			net = t.Amount
		case "income":
			net = -t.Amount
		}
		if inYear && t.Kind == "expense" {
			yearSpent += t.Amount
			if len(t.Tags) > 0 {
				yearTagged += t.Amount
			}
		}
		for _, tag := range t.Tags {
			a := by[tag]
			if a == nil {
				a = &acc{s: TagStat{Tag: tag, Trip: ledger.TripTag(tag), First: t.Date, Months: make([]money.Cents, 12)}, cats: map[string]*NameAmount{}, merch: map[string]*NameAmount{}}
				by[tag] = a
			}
			s := &a.s
			s.Count++
			if t.Date < s.First {
				s.First = t.Date
			}
			if t.Date > s.Last {
				s.Last = t.Date
			}
			switch t.Kind {
			case "expense":
				s.Spent += t.Amount
			case "income":
				s.Income += t.Amount
			}
			if inYear {
				s.Months[mi] += net
				s.Year += net
			}
			if t.Kind == "expense" {
				add := func(m map[string]*NameAmount, k string) {
					if k == "" {
						return
					}
					if m[k] == nil {
						m[k] = &NameAmount{Name: k}
					}
					m[k].Amount += t.Amount
					m[k].Count++
				}
				add(a.cats, t.Category)
				add(a.merch, t.Merchant)
			}
		}
	}
	ruleCount := map[string]int{}
	for _, r := range rules {
		for _, tag := range r.AddTags {
			ruleCount[tag]++
		}
	}
	rep := TagReport{Tags: []TagStat{}, YearSpent: yearSpent, Months: months}
	if yearSpent > 0 {
		rep.TaggedShare = float64(yearTagged) / float64(yearSpent)
	}
	for _, a := range by {
		s := a.s
		s.Net = s.Spent - s.Income
		s.Cats, s.Merch = topN(a.cats, 3), topN(a.merch, 5)
		s.Rules = ruleCount[s.Tag]
		// Average over the months the tag was in use (at least one).
		if f, err := time.Parse("2006-01-02", s.First); err == nil {
			l, _ := time.Parse("2006-01-02", s.Last)
			n := (l.Year()-f.Year())*12 + int(l.Month()-f.Month()) + 1
			s.PerMonth = s.Net / money.Cents(n)
		}
		rep.Tags = append(rep.Tags, s)
	}
	sort.Slice(rep.Tags, func(i, j int) bool {
		a, b := rep.Tags[i], rep.Tags[j]
		return a.Net > b.Net || a.Net == b.Net && a.Tag < b.Tag
	})
	return rep
}

// RuleStat is how a rule behaves against the whole ledger as it stands.
type RuleStat struct {
	ID         int64  `json:"id"`
	Matches    int    `json:"matches"`
	Recent     int    `json:"recent"` // matches in the last 90 days
	Last       string `json:"last"`
	Decides    int    `json:"decides"`    // rows where it is the first rule to set the category
	Overridden int    `json:"overridden"` // …of those, filed elsewhere by hand since
	Shadowed   bool   `json:"shadowed"`   // matches, but an earlier rule always sets the category first
	Duplicate  int64  `json:"duplicate"`  // an earlier rule with the same conditions and actions: redundant
}

type RuleReport struct {
	Rules    []RuleStat `json:"rules"`
	Coverage float64    `json:"coverage"` // share of the last 90 days' rows some enabled rule matches
	Unused   int        `json:"unused"`
	Shadowed int        `json:"shadowed"`
	Dupes    int        `json:"duplicates"`
	Overrode int        `json:"overrode"` // rules overridden in at least a third of their rows
	Top      []int64    `json:"top"`      // busiest rules over the last 90 days
}

func within(cat, parent string) bool { return cat == parent || strings.HasPrefix(cat, parent+".") }

// RuleStats replays every rule over the ledger in priority order.
func RuleStats(txs []ledger.Tx, rules []ledger.Rule, now time.Time) RuleReport {
	since := now.AddDate(0, 0, -90).Format("2006-01-02")
	stats := make([]RuleStat, len(rules))
	ms := make([]ledger.Matcher, len(rules))
	for i, r := range rules {
		stats[i].ID = r.ID
		ms[i] = ledger.NewMatcher(r)
	}
	var recentRows, covered int
	for i := range txs {
		t := &txs[i]
		f := ledger.FoldTx(t)
		recent := t.Date >= since && t.Kind != "transfer"
		if recent {
			recentRows++
		}
		catTaken, hit := false, false
		for j, r := range rules {
			if !ms[j].Match(t, f) {
				continue
			}
			s := &stats[j]
			s.Matches++
			if t.Date > s.Last {
				s.Last = t.Date
			}
			if t.Date >= since {
				s.Recent++
			}
			if !r.Enabled {
				continue
			}
			hit = true
			if r.SetCategory != "" && !catTaken {
				catTaken = true
				s.Decides++
				if t.Category != "" && !within(t.Category, r.SetCategory) {
					s.Overridden++
				}
			}
		}
		if recent && hit {
			covered++
		}
	}
	rep := RuleReport{Rules: stats}
	if recentRows > 0 {
		rep.Coverage = float64(covered) / float64(recentRows)
	}
	seen := map[string]int64{}
	for i, r := range rules {
		s := &stats[i]
		key := strings.Join([]string{merchant.Fold(r.Pattern), r.WhenCategory, r.WhenKind, r.SetCategory, merchant.Fold(r.SetMerchant), strings.Join(ledger.NormalizeTags(r.AddTags), ",")}, "|")
		if id, ok := seen[key]; ok {
			s.Duplicate = id
			rep.Dupes++
		} else {
			seen[key] = r.ID
		}
		s.Shadowed = r.Enabled && s.Matches > 0 && r.SetCategory != "" && s.Decides == 0 && r.SetMerchant == "" && len(r.AddTags) == 0
		switch {
		case s.Matches == 0:
			rep.Unused++
		case s.Shadowed:
			rep.Shadowed++
		}
		if s.Decides >= 3 && s.Overridden*3 >= s.Decides {
			rep.Overrode++
		}
	}
	order := make([]int, len(stats))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return stats[order[a]].Recent > stats[order[b]].Recent })
	for _, i := range order {
		if len(rep.Top) == 5 || stats[i].Recent == 0 {
			break
		}
		rep.Top = append(rep.Top, stats[i].ID)
	}
	return rep
}

// CategoryUse is how much a category is used.
type CategoryUse struct {
	Category string      `json:"category"`
	Count    int         `json:"count"`
	Year     money.Cents `json:"year"` // last 12 months, own rows only
	Last     string      `json:"last"`
	Rules    int         `json:"rules"`   // rules that set it
	Budgeted bool        `json:"budgeted"` // some budget line covers it
}

// CategoryUsage counts each category's own rows (not its children's).
func CategoryUsage(txs []ledger.Tx, rules []ledger.Rule, budgeted map[string]bool, now time.Time) map[string]CategoryUse {
	from := now.AddDate(-1, 0, 0).Format("2006-01-02")
	out := map[string]CategoryUse{}
	for i := range txs {
		t := &txs[i]
		u := out[t.Category]
		u.Category = t.Category
		u.Count++
		if t.Date > u.Last {
			u.Last = t.Date
		}
		if t.Date >= from && t.Kind != "transfer" {
			u.Year += t.Amount
		}
		out[t.Category] = u
	}
	for _, r := range rules {
		if r.SetCategory != "" {
			u := out[r.SetCategory]
			u.Category = r.SetCategory
			u.Rules++
			out[r.SetCategory] = u
		}
	}
	for c := range budgeted {
		u := out[c]
		u.Category = c
		u.Budgeted = true
		out[c] = u
	}
	return out
}

// AccountUse is how much an account is used in the ledger.
type AccountUse struct {
	Count int    `json:"count"`
	Last  string `json:"last"`
}

// AccountUsage counts rows per account, either side of a transfer included.
func AccountUsage(txs []ledger.Tx) map[string]AccountUse {
	out := map[string]AccountUse{}
	bump := func(id, date string) {
		if id == "" {
			return
		}
		u := out[id]
		u.Count++
		if date > u.Last {
			u.Last = date
		}
		out[id] = u
	}
	for i := range txs {
		bump(txs[i].AccountID, txs[i].Date)
		bump(txs[i].ToAccountID, txs[i].Date)
	}
	return out
}
