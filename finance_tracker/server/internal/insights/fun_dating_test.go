package insights_test

import (
	"testing"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
)

func date(d, note, merch string, eur float64, tags ...string) ledger.Tx {
	t := tx(d, "expense", "dating", eur, "swed")
	t.Note, t.Merchant, t.Tags = note, merch, tags
	return t
}

// The dating category tells its own story: what dates cost and went on
// (read from notes, English or Lithuanian), the flowers, the biggest date,
// and money spent with the same person outside dating.
func TestDatingFacts(t *testing.T) {
	trip := tx("2026-08-01", "expense", "travel.trip", 900, "swed")
	trip.Tags = []string{"ana", "trip:rome"}
	txs := []ledger.Tx{
		date("2026-05-01", "Coffee date", "", 8, "ana"),
		date("2026-05-07", "Flowers", "", 30, "ana"),
		date("2026-05-07", "Dinner", "", 50, "ana"),
		date("2026-06-10", "Ring for the proposal", "", 300, "ana"),
		date("2026-06-15", "", "Gėlių parduotuvė", 20, "ana"),
		date("2026-07-11", "Drinks at the concert", "", 18),
		trip,
		tx("2026-08-02", "expense", "food", 40, "swed"),
	}
	facts := insights.FunFacts(txs, cats(t), time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC), "", "")
	by := map[string]insights.Fact{}
	for _, f := range facts {
		by[f.Key] = f
	}
	d := by["dating"]
	if d.Value != "€426" || d.Detail != "5 dates since 1 May 2026 — about €85 a date." {
		t.Fatalf("dating: %+v", d)
	}
	got := map[string]string{}
	for _, it := range d.Items {
		got[it.Label] = it.Value
	}
	if got["Gifts"] != "€300" || got["Food & drinks"] != "€58" || got["Flowers"] != "€50" || got["Outings"] != "€18" || !d.Items[0].Top || d.Items[0].Label != "Gifts" {
		t.Fatalf("where it went: %+v", d.Items)
	}
	if f := by["flowers"]; f.Value != "2 times" || f.Detail != "€50 on flowers — a bouquet about every 39 days." {
		t.Fatalf("flowers: %+v", f)
	}
	if f := by["date_big"]; f.Value != "€300" || f.Detail != "10 Jun 2026 · Ring for the proposal" {
		t.Fatalf("biggest: %+v", f)
	}
	if f := by["together"]; f.Value != "€900" || f.Detail != "More with #ana outside dating, mostly travel." {
		t.Fatalf("together: %+v", f)
	}
	if by["dating"].Link == "" || by["together"].Link == "" {
		t.Fatal("facts link to their transactions")
	}
	// No dating rows in the window: no dating facts.
	for _, f := range insights.FunFacts(txs, cats(t), time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC), "2026-08-01", "2026-10-31") {
		if f.Key == "dating" || f.Key == "flowers" || f.Key == "together" {
			t.Fatalf("%s shown without dating rows", f.Key)
		}
	}
}
