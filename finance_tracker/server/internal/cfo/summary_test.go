package cfo_test

import (
	"strings"
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/ledger"
	"ft/internal/plan"
	. "ft/internal/testutil"
)

// Insights' summary: a typical month adds up to its income, and the
// findings name what to act on — a plan missed every month, one-offs that
// keep coming, spending no line covers, lines far from reality — worst
// first, with obligations kept out of "bills".
func TestBuildSummary(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	months := []string{"2025-10", "2025-11", "2025-12", "2026-01", "2026-02", "2026-03", "2026-04", "2026-05", "2026-06", "2026-07", "2026-08", "2026-09"}
	for i, m := range months {
		Tx(t, d, ledger.Tx{Date: m + "-10", Amount: E(3000), Category: "salary", AccountID: "swed"})
		Tx(t, d, ledger.Tx{Date: m + "-15", Amount: E(500), Category: "kids.alimony", AccountID: "swed", Merchant: "Aliments"})
		for _, day := range []string{"-02", "-06", "-11", "-16", "-21", "-26"} { // everyday shopping, not a one-off
			Tx(t, d, ledger.Tx{Date: m + day, Amount: E(200), Category: "food.groceries", AccountID: "swed", Merchant: "Maxima"})
		}
		Tx(t, d, ledger.Tx{Date: m + "-07", Amount: E(150), Category: "health", AccountID: "swed"})
		if i%2 == 0 {
			Tx(t, d, ledger.Tx{Date: m + "-20", Amount: E(600), Category: "travel", AccountID: "swed", Merchant: "Ryanair"})
		}
	}
	for _, b := range []plan.Budget{
		{Name: "Alimony", Kind: "fixed", Categories: []string{"kids.alimony"}, Amount: E(500)},
		{Name: "ETF", Kind: "saving", Categories: []string{"transfer.invest"}, Amount: E(1000)},
		{Name: "Food", Kind: "spending", Categories: []string{"food"}, Amount: E(600)},
	} {
		if err := plan.Save(d, &b, ""); err != nil {
			t.Fatal(err)
		}
	}
	s, err := cfo.BuildSummary(d, now)
	if err != nil {
		t.Fatal(err)
	}
	if s.From != "2025-10" || s.To != "2026-09" || s.Months != 12 {
		t.Fatalf("window %s–%s %d", s.From, s.To, s.Months)
	}
	f := s.Flow
	if f.Income != E(3000) || f.Fixed != E(500) || f.OneOffs != E(300) || f.Everyday != E(1350) || f.Saved != E(850) { // 1200 food + 150 health; 600 every other month
		t.Errorf("typical month %+v", f)
	}
	if f.Fixed+f.Everyday+f.OneOffs+f.Saved != f.Income {
		t.Errorf("parts don't add up: %+v", f)
	}
	by := map[string]cfo.Finding{}
	for _, x := range s.Findings {
		by[x.Key] = x
	}
	// 3000 − 500 fixed − 1000 saving leaves 1500; day-to-day runs 1350 + 150, plus 600 every other month.
	if p := by["plan_fit"]; p.Tone != "warn" || len(p.Items) != 6 || p.Items[0].Tone != "bad" || p.Items[1].Tone != "good" {
		t.Errorf("plan fit %+v", p)
	}
	if o := by["one_offs"]; o.Tone != "warn" || o.Amount != E(300) || len(o.Items) != 6 || o.Items[0].Label != "Ryanair" {
		t.Errorf("one-offs %+v", o)
	}
	if u := by["unbudgeted"]; u.Amount != E(450) || u.Items[0].Label != "Travel" || u.Items[0].Amount != E(300) { // travel 300/mo + health 150/mo
		t.Errorf("unbudgeted %+v", u)
	}
	if l := by["lines_off"]; len(l.Items) != 1 || l.Items[0].Label != "Food" || l.Items[0].Amount != E(600) || l.Items[0].Tone != "bad" {
		t.Errorf("lines off %+v", l)
	}
	if sv := by["saving"]; sv.Tone != "warn" || sv.Amount != E(850) {
		t.Errorf("saving %+v", sv)
	}
	if b, ok := by["bills"]; ok {
		for _, it := range b.Items {
			if strings.Contains(it.Label, "Aliments") {
				t.Errorf("an obligation listed as a bill: %+v", b)
			}
		}
	}
	for i := 1; i < len(s.Findings); i++ {
		rank := map[string]int{"bad": 0, "warn": 1, "info": 2, "good": 3}
		if rank[s.Findings[i-1].Tone] > rank[s.Findings[i].Tone] {
			t.Errorf("not worst first: %s before %s", s.Findings[i-1].Key, s.Findings[i].Key)
		}
	}
}
