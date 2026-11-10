package cfo_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/testutil"
)

// A month with nothing in it reviews as empty lists, never null: the Month
// review page maps over them and a null broke it (a fresh install, or a month
// before the first transaction).
func TestMonthReviewEmptyMonthHasLists(t *testing.T) {
	d := testutil.DB(t)
	r, err := cfo.BuildMonthReview(d, "2026-10", time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(r)
	for _, k := range []string{"highlights", "categories", "top_expenses", "top_income", "daily", "trend", "checks"} {
		if strings.Contains(string(b), `"`+k+`":null`) {
			t.Errorf("%s is null: %s", k, b)
		}
	}
}
