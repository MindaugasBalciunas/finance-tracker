package cfo_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ft/internal/cfo"
	"ft/internal/ledger"
	"ft/internal/notify"
	. "ft/internal/testutil"
)

// Alerts reach Home Assistant once each, in waking hours: a charge that
// looks doubled, and the month still to close.
func TestAlertsSentOnce(t *testing.T) {
	d := DB(t)
	var mu sync.Mutex
	var got []notify.Message
	ha := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m notify.Message
		json.Unmarshal(b, &m)
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
	}))
	defer ha.Close()
	if err := notify.Save(d, notify.Settings{WebhookURL: ha.URL + "/api/webhook/finance-alerts", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	d.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('app_origin','https://budget.example')`)
	Tx(t, d, ledger.Tx{Date: "2026-09-10", Amount: E(3000), Category: "salary", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: "2026-10-03", Amount: E(42.5), Category: "food.groceries", AccountID: "swed", Merchant: "Maxima"})
	Tx(t, d, ledger.Tx{Date: "2026-10-04", Amount: E(42.5), Category: "food.groceries", AccountID: "swed", Merchant: "Maxima"})

	night := time.Date(2026, 10, 5, 23, 0, 0, 0, time.Local)
	if n := cfo.SendAlerts(d, night); n != 0 {
		t.Fatalf("%d sent at night", n)
	}
	noon := time.Date(2026, 10, 5, 12, 0, 0, 0, time.Local)
	if n := cfo.SendAlerts(d, noon); n != 2 {
		t.Fatalf("sent %d: %+v", n, got)
	}
	var dup, close bool
	for _, m := range got {
		dup = dup || strings.Contains(m.Title, "Charged twice by Maxima")
		close = close || (m.Title == "Close September" && strings.HasPrefix(m.URL, "https://budget.example/#/insights/review"))
	}
	if !dup || !close {
		t.Errorf("alerts %+v", got)
	}
	if n := cfo.SendAlerts(d, noon.Add(30*time.Minute)); n != 0 {
		t.Errorf("sent again: %d", n)
	}
}
