package insights

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"ft/internal/ledger"
	"ft/internal/merchant"
	"ft/internal/money"
)

// isDating: the dating category, top level ("dating") or a leaf of another
// one ("leisure.dating").
func isDating(cat string) bool {
	return ledger.Top(cat) == "dating" || strings.HasSuffix(cat, ".dating")
}

// What a date payment went on, read from its note and merchant (English
// and Lithuanian, folded: "Gėlių" → "GELIU"). First match wins, so "Lidl,
// ice cream, flowers" counts as flowers and "Drinks at the concert" as an
// outing.
var dateKinds = []struct {
	name  string
	words []string
}{
	{"Flowers", []string{"FLOWER", "FLORIST", "GELE", "GELI", "ROZE", "ROZI", "BIJUN", "PUOKST", "TULP"}},
	{"Gifts", []string{"GIFT", "RING", "DOVAN", "PRESENT"}},
	{"Outings", []string{"CINEMA", "KINO", "CONCERT", "KONCERT", "OPERA", "THEATRE", "TEATR", "RIDING", "RESORT", "SPA", "BOTANI", "MUSEUM", "MUZIEJ", "DVIRAT", "BOWLING", "PARKING"}},
	{"Food & drinks", []string{"DINNER", "LUNCH", "BREAKFAST", "COFFE", "KAVA", "FOOD", "RESTAURANT", "PIZZ", "PICO", "SUSHI", "WINE", "VYN", "APEROL", "APPEROL", "DRINK", "ICE CREAM", "GRILL", "BAR", "CAKE", "TEA", "KIBIN", "BURRATA", "STRAWBERR", "BRASK", "WATERMELON", "ARBUZ", "CHOCOLATE", "SOKOLAD"}},
}

func dateKind(t *ledger.Tx) string {
	up := " " + merchant.Fold(t.Note+" "+t.Merchant) + " "
	for _, k := range dateKinds {
		for _, w := range k.words {
			if strings.Contains(up, w) {
				return k.name
			}
		}
	}
	return "Other"
}

// datingFacts tells the dating story of txs (already the window): what it
// cost and went on, the flowers, the biggest date, and money spent with the
// same person outside dating (trips, a hotel). Nothing without dating rows.
func datingFacts(txs []ledger.Tx, cats map[string]ledger.Category, from, to string) []Fact {
	var rows []*ledger.Tx
	catN := map[string]int{}
	for i := range txs {
		if t := &txs[i]; t.Kind == "expense" && isDating(t.Category) {
			rows = append(rows, t)
			catN[t.Category]++
		}
	}
	if len(rows) < 3 {
		return nil
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Date < rows[j].Date })
	cat := ""
	for c, n := range catN {
		if n > catN[cat] || n == catN[cat] && c < cat {
			cat = c
		}
	}
	link := func(kv ...string) string {
		return ledger.Link(append([]string{"from", from, "to", to, "category", cat}, kv...)...)
	}
	day := func(d string) string {
		t, _ := time.Parse("2006-01-02", d)
		return t.Format("2 Jan 2006")
	}

	var total money.Cents
	byDay := map[string]money.Cents{}
	byKind := map[string]money.Cents{}
	kindN := map[string]int{}
	var flowers []*ledger.Tx
	tagN := map[string]int{}
	for _, t := range rows {
		total += t.Amount
		byDay[t.Date] += t.Amount
		k := dateKind(t)
		byKind[k] += t.Amount
		kindN[k]++
		if k == "Flowers" {
			flowers = append(flowers, t)
		}
		for _, tag := range t.Tags {
			if !strings.HasPrefix(tag, "trip:") && !strings.Contains(tag, ":") {
				tagN[tag]++
			}
		}
	}
	var out []Fact

	// What dating costs, and where that money goes.
	f := Fact{Key: "dating", Icon: "heart", Title: "Dating", Value: eurWhole(total),
		Detail: fmt.Sprintf("%d dates since %s — about %s a date.", len(byDay), day(rows[0].Date), eurWhole(total/money.Cents(len(byDay)))), Link: link()}
	kinds := make([]string, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool { return byKind[kinds[i]] > byKind[kinds[j]] })
	for i, k := range kinds {
		f.Items = append(f.Items, FactItem{Label: k, Value: eurWhole(byKind[k]), Sub: fmt.Sprintf("%d×", kindN[k]), N: byKind[k].Float(), Top: i == 0})
	}
	out = append(out, f)

	// Flowers: how many bouquets, and how often.
	if len(flowers) >= 2 {
		var sum money.Cents
		for _, t := range flowers {
			sum += t.Amount
		}
		a, _ := time.Parse("2006-01-02", flowers[0].Date)
		b, _ := time.Parse("2006-01-02", flowers[len(flowers)-1].Date)
		every := int(b.Sub(a).Hours()/24) / (len(flowers) - 1)
		detail := fmt.Sprintf("%s on flowers", eurWhole(sum))
		if every > 0 {
			detail += fmt.Sprintf(" — a bouquet about every %d days.", every)
		}
		out = append(out, Fact{Key: "flowers", Icon: "gift", Title: "Flowers bought", Value: fmt.Sprintf("%d times", len(flowers)), Detail: detail, Link: link()})
	}

	// The biggest date: the day it cost the most, and what it was.
	best := ""
	for d, v := range byDay {
		if best == "" || v > byDay[best] || v == byDay[best] && d < best {
			best = d
		}
	}
	var top *ledger.Tx
	for _, t := range rows {
		if t.Date == best && (top == nil || t.Amount > top.Amount) {
			top = t
		}
	}
	what := top.Note
	if what == "" {
		what = top.Merchant
	}
	detail := day(best)
	if what != "" {
		detail += " · " + what
	}
	out = append(out, Fact{Key: "date_big", Icon: "spark", Title: "The biggest date", Value: eurWhole(byDay[best]), Detail: detail,
		Link: ledger.Link("from", best, "to", best, "category", cat)})

	// The person on most dates, and what was spent with them outside dating.
	who := ""
	for tag, n := range tagN {
		if n > tagN[who] || n == tagN[who] && tag < who {
			who = tag
		}
	}
	if who != "" && tagN[who]*2 >= len(rows) {
		var with money.Cents
		byCat := map[string]money.Cents{}
		for i := range txs {
			t := &txs[i]
			if t.Kind != "expense" || isDating(t.Category) || !hasTag(t.Tags, who) {
				continue
			}
			with += t.Amount
			byCat[ledger.Top(t.Category)] += t.Amount
		}
		if with > 0 {
			mostly := ""
			for c, v := range byCat {
				if mostly == "" || v > byCat[mostly] || v == byCat[mostly] && c < mostly {
					mostly = c
				}
			}
			name := mostly
			if c, ok := cats[mostly]; ok {
				name = c.Name
			}
			out = append(out, Fact{Key: "together", Icon: "users", Title: "Together, beyond dates", Value: eurWhole(with),
				Detail: fmt.Sprintf("More with #%s outside dating, mostly %s.", who, strings.ToLower(name)),
				Link:   ledger.Link("from", from, "to", to, "tag", who, "kind", "expense")})
		}
	}
	return out
}

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}
