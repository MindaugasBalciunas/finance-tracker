package ledger_test

import (
	"testing"
	"time"

	"ft/internal/ledger"
	. "ft/internal/testutil"
)

// With a trip's real dates set, an expense inside them gets its tag by
// itself only when it is surely the trip's: a known merchant you don't pay
// at home (three times in a year outside trips), not kids' costs or a work
// lunch, not a bill — and not when it is already on a trip or the owner
// removed the tag. Everything else in the dates is offered, not added.
func TestTripDatesTagging(t *testing.T) {
	d := DB(t)
	for _, day := range []string{"2026-05-01", "2026-05-08", "2026-05-15", "2026-05-22"} {
		Tx(t, d, ledger.Tx{Date: day, Amount: E(10), Category: "food.groceries", AccountID: "swed", Merchant: "Lidl", Tags: []string{"kids"}})
	}
	Tx(t, d, ledger.Tx{Date: "2026-05-29", Amount: E(10), Category: "food.groceries", AccountID: "swed", Merchant: "Lidl"})
	if err := ledger.SaveTripDates(d, ledger.TripDates{Tag: "trip:rome", Start: "2026-06-23", End: "2026-06-27"}); err != nil {
		t.Fatal(err)
	}
	if ledger.SaveTripDates(d, ledger.TripDates{Tag: "rome", Start: "2026-06-23", End: "2026-06-27"}) == nil ||
		ledger.SaveTripDates(d, ledger.TripDates{Tag: "trip:x", Start: "2026-06-27", End: "2026-06-23"}) == nil {
		t.Error("bad trips accepted")
	}
	eng, err := ledger.LoadEngine(d)
	if err != nil {
		t.Fatal(err)
	}
	apply := func(tx ledger.Tx, suppressed ...string) []string {
		tx.Kind = "expense"
		eng.Apply(&tx, false, suppressed)
		return tx.Tags
	}
	for _, c := range []struct {
		name string
		tx   ledger.Tx
		sup  []string
		want string
	}{
		{"dinner at a place you don't go at home", ledger.Tx{Date: "2026-06-24", Category: "food.restaurants", Merchant: "Trattoria"}, nil, "trip:rome"},
		{"your home grocer", ledger.Tx{Date: "2026-06-24", Category: "food.groceries", Merchant: "Lidl"}, nil, ""},
		{"no merchant: can't tell", ledger.Tx{Date: "2026-06-24", Category: "food.restaurants"}, nil, ""},
		{"a phone bill", ledger.Tx{Date: "2026-06-24", Category: "utilities.telecom", Merchant: "Telia"}, nil, ""},
		{"kids' costs", ledger.Tx{Date: "2026-06-24", Category: "kids.general", Merchant: "Toy"}, nil, ""},
		{"removed by hand", ledger.Tx{Date: "2026-06-24", Category: "food.restaurants", Merchant: "Trattoria"}, []string{"trip:rome"}, ""},
		{"after the trip", ledger.Tx{Date: "2026-06-28", Category: "food.restaurants", Merchant: "Trattoria"}, nil, ""},
	} {
		got := apply(c.tx, c.sup...)
		trip := ""
		for _, tg := range got {
			if len(tg) > 5 && tg[:5] == "trip:" {
				trip = tg
			}
		}
		if trip != c.want {
			t.Errorf("%s: tags %v, want %q", c.name, got, c.want)
		}
	}

	// The form: a flight bought ahead points at the trip; inside the dates,
	// a home merchant is offered but not sure; Lidl's usual tag is kids.
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	s, err := ledger.SuggestTags(d, "Lidl", "2026-03-01", "travel.flights", "expense", now)
	if err != nil || s.Trip == nil || s.Trip.Tag != "trip:rome" || s.Trip.Why != "booking" {
		t.Errorf("a flight bought ahead: %v %+v", err, s)
	}
	if len(s.Usual) != 1 || s.Usual[0].Tag != "kids" || s.Usual[0].N != 4 || s.Usual[0].Of != 5 {
		t.Errorf("usual for Lidl: %+v", s.Usual)
	}
	if s, _ := ledger.SuggestTags(d, "Lidl", "2026-06-24", "food.groceries", "expense", now); s.Trip == nil || s.Trip.Sure || !s.Trip.Home {
		t.Errorf("Lidl in the trip's dates: %+v", s.Trip)
	}

	// What the trip missed: sure ones, maybes, bookings before.
	Tx(t, d, ledger.Tx{Date: "2026-06-25", Amount: E(30), Category: "food.restaurants", AccountID: "swed", Merchant: "Trattoria"})
	Tx(t, d, ledger.Tx{Date: "2026-06-25", Amount: E(12), Category: "food.groceries", AccountID: "swed", Merchant: "Lidl"})
	Tx(t, d, ledger.Tx{Date: "2026-02-10", Amount: E(150), Category: "travel.flights", AccountID: "swed", Merchant: "Ryanair"})
	Tx(t, d, ledger.Tx{Date: "2026-06-25", Amount: E(14), Category: "utilities.telecom", AccountID: "swed", Merchant: "Telia"})
	likely, maybe, book, err := ledger.TripUntagged(d, "trip:rome")
	if err != nil || len(likely) != 1 || likely[0].Merchant != "Trattoria" || len(maybe) != 1 || maybe[0].Merchant != "Lidl" || len(book) != 1 || book[0].Merchant != "Ryanair" {
		t.Errorf("untagged: %v / %+v / %+v / %+v", err, likely, maybe, book)
	}
}
