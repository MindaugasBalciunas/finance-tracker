package cfo_test

import (
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/ledger"
	. "ft/internal/testutil"
)

// Closing a month: the data steps tick themselves (inbox, categories), the
// decisions are ticked by the owner, and a closed month leaves Home.
func TestMonthClose(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	Tx(t, d, ledger.Tx{Date: "2026-09-10", Amount: E(3000), Category: "salary", AccountID: "swed"})
	vague := Tx(t, d, ledger.Tx{Date: "2026-09-12", Amount: E(40), Category: "food", AccountID: "swed"})
	d.Exec(`INSERT INTO bank_inbox(external_id,date,kind,amount,category,state,first_seen_at,last_seen_at) VALUES('r1','2026-09-28','expense',1500,'food.groceries','open','x','x')`)

	c, err := cfo.Close(d, "2026-09", now)
	if err != nil {
		t.Fatal(err)
	}
	step := func(c *cfo.MonthClose, k string) cfo.CloseStep {
		for _, s := range c.Steps {
			if s.Key == k {
				return s
			}
		}
		t.Fatalf("no step %s in %+v", k, c.Steps)
		return cfo.CloseStep{}
	}
	if len(c.Steps) != 6 || step(c, "inbox").Done || step(c, "categories").Done || step(c, "review").Done || !step(c, "review").Manual {
		t.Fatalf("open month %+v", c)
	}
	if o, _ := cfo.BuildOverview(d, now, 0); o.Close == nil || o.Close.Month != "2026-09" {
		t.Errorf("Home doesn't offer the close: %+v", o.Close)
	}
	if err := cfo.TickClose(d, "2026-09", "inbox", true); err == nil {
		t.Error("a data step can't be ticked by hand")
	}

	d.Exec(`UPDATE bank_inbox SET state='imported'`)
	vague.Category = "food.groceries"
	ledger.Update(d, &vague)
	if err := cfo.TickClose(d, "2026-09", "review", true); err != nil {
		t.Fatal(err)
	}
	c, _ = cfo.Close(d, "2026-09", now)
	if !step(c, "inbox").Done || !step(c, "categories").Done || !step(c, "review").Done || step(c, "plan").Done {
		t.Errorf("after the work %+v", c.Steps)
	}
	if err := cfo.FinishClose(d, "2026-09"); err != nil {
		t.Fatal(err)
	}
	if c, _ = cfo.Close(d, "2026-09", now); !c.Closed {
		t.Error("not closed")
	}
	if o, _ := cfo.BuildOverview(d, now, 0); o.Close != nil {
		t.Error("a closed month still on Home")
	}
}
