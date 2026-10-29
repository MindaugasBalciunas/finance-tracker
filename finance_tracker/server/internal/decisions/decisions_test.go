package decisions_test

import (
	"testing"
	"time"

	"ft/internal/decisions"
	. "ft/internal/testutil"
)

// A €1,200 laptop sleeps for a week; buying it sets a review two months on;
// rating it "no" counts it as regretted, and skipping counts as money kept.
func TestJournalCoolsOffDecidesAndRates(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	x := decisions.Decision{Title: "Laptop", Amount: E(1200), Category: "shopping.electronics", Status: "bought", Worth: "yes"}
	if err := decisions.Create(d, &x, now); err != nil {
		t.Fatal(err)
	}
	if x.Status != "thinking" || x.Worth != "" || x.CoolUntil != "2026-10-22" {
		t.Fatalf("a new decision starts thinking, cooling for a week: %+v", x)
	}
	x.Status = "bought"
	if err := decisions.Update(d, &x, now.AddDate(0, 0, 8)); err != nil {
		t.Fatal(err)
	}
	if x.DecidedOn != "2026-10-23" || x.ReviewOn != "2026-12-22" {
		t.Errorf("bought %+v", x)
	}
	x.Worth = "no"
	if err := decisions.Update(d, &x, now.AddDate(0, 2, 10)); err != nil {
		t.Fatal(err)
	}
	y := decisions.Decision{Title: "Second TV", Amount: E(800), Category: "shopping.electronics"}
	decisions.Create(d, &y, now)
	y.Status = "skipped"
	decisions.Update(d, &y, now)

	all, _ := decisions.List(d)
	r := decisions.Summarise(all)
	if r.Bought != 1 || r.No != 1 || r.RegrettedSum != E(1200) || r.Skipped != 1 || r.SkippedSum != E(800) {
		t.Errorf("record %+v", r)
	}
	if len(r.ByCategory) != 1 || r.ByCategory[0].Rated != 1 || r.ByCategory[0].Worth != 0 {
		t.Errorf("by category %+v", r.ByCategory)
	}
}

func TestJournalRejectsNonsense(t *testing.T) {
	d := DB(t)
	now := time.Now()
	for _, x := range []decisions.Decision{{Title: "", Amount: E(10)}, {Title: "Free", Amount: 0}, {Title: "x", Amount: E(5), Note: string(make([]byte, 1001))}} {
		if err := decisions.Create(d, &x, now); err == nil {
			t.Errorf("accepted %+v", x)
		}
	}
	x := decisions.Decision{Title: "Bike", Amount: E(400)}
	decisions.Create(d, &x, now)
	x.Worth = "yes" // still thinking: can't be rated
	if err := decisions.Update(d, &x, now); err == nil {
		t.Error("rated before buying")
	}
}
