package ledger_test

import (
	"reflect"
	"testing"

	"ft/internal/ledger"
	. "ft/internal/testutil"
)

// The form's category chips: the merchant's own categories, most used first
// (matched ignoring case), then the most used lately without repeating them.
func TestSuggestCategories(t *testing.T) {
	d := DB(t)
	for i, c := range []string{"food.groceries", "food.groceries", "kids.general", "food.groceries"} {
		Tx(t, d, ledger.Tx{Kind: "expense", Date: "2026-05-0" + string(rune('1'+i)), Amount: E(10), Category: c, AccountID: "swed", Merchant: "Lidl"})
	}
	Tx(t, d, ledger.Tx{Kind: "expense", Date: "2026-05-09", Amount: E(10), Category: "transport.fuel", AccountID: "swed", Merchant: "Neste"})
	s, err := ledger.SuggestCategories(d, "LIDL", "expense")
	if err != nil {
		t.Fatal(err)
	}
	if want := []ledger.CatShare{{"food.groceries", 3}, {"kids.general", 1}}; !reflect.DeepEqual(s.Merchant, want) {
		t.Errorf("merchant = %v, want %v", s.Merchant, want)
	}
	if want := []string{"transport.fuel"}; !reflect.DeepEqual(s.Recent, want) {
		t.Errorf("recent = %v, want %v", s.Recent, want)
	}
}
