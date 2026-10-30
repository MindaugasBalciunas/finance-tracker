package insights_test

import (
	"testing"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	. "ft/internal/testutil"
)

// A streaming service that began as a €0.99 trial is flagged as one; the
// alimony isn't a subscription at all; marking Spotify "cancel" counts it
// in what cancelling saves, and a charge after that is caught.
func TestSubscriptionAuditTrialsVerdictsAndObligations(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	Tx(t, d, ledger.Tx{Date: "2026-08-03", Amount: E(0.99), Category: "subscriptions.media", AccountID: "swed", Merchant: "Max"})
	for _, day := range []string{"2026-09-03", "2026-10-03"} {
		Tx(t, d, ledger.Tx{Date: day, Amount: E(9.99), Category: "subscriptions.media", AccountID: "swed", Merchant: "Max"})
	}
	for m := 1; m <= 12; m++ {
		day := now.AddDate(0, -m, 0).Format("2006-01") + "-07"
		Tx(t, d, ledger.Tx{Date: day, Amount: E(10.99), Category: "subscriptions.media", AccountID: "swed", Merchant: "Spotify"})
		Tx(t, d, ledger.Tx{Date: day, Amount: E(1000), Category: "kids.alimony", AccountID: "swed", Merchant: "Aliments"})
		Tx(t, d, ledger.Tx{Date: day, Amount: E(13.99), Category: "subscriptions.media", AccountID: "swed", Merchant: "YouTube"})
	}
	Tx(t, d, ledger.Tx{Date: "2026-10-07", Amount: E(10.99), Category: "subscriptions.media", AccountID: "swed", Merchant: "Spotify"})
	// Decided on 1 Oct to cancel by the 31st: the 7 Oct charge is the notice
	// period, not a missed cancel.
	if err := insights.SetSubVerdict(d, insights.SubKey("YouTube", "subscriptions.media"), insights.SubVerdict{Verdict: "cancel", CancelBy: "2026-10-31"}, time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := insights.SetSubVerdict(d, insights.SubKey("Spotify", "subscriptions.media"), insights.SubVerdict{Verdict: "cancel", CancelBy: "2026-09-15"}, now.AddDate(0, -2, 0)); err != nil {
		t.Fatal(err)
	}
	s, err := insights.AuditSubscriptions(d, now)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]insights.Subscription{}
	for _, it := range s.Items {
		by[it.Merchant] = it
	}
	if _, ok := by["Aliments"]; ok {
		t.Error("alimony listed as a subscription")
	}
	if m := by["Max"]; m.TrialFrom != "2026-08-03" || !m.New {
		t.Errorf("trial not flagged: %+v", m)
	}
	sp := by["Spotify"]
	if yt := by["YouTube"]; yt.ChargedAfterCancel != "" {
		t.Errorf("a charge before the cancel-by date flagged: %+v", yt)
	}
	// Spotify: decided 20 Aug to cancel by 15 Sep; the 7 Sep charge is fine,
	// the 7 Oct one isn't.
	if sp.Verdict == nil || sp.Verdict.Verdict != "cancel" || sp.ChargedAfterCancel != "2026-10-07" || s.ToCancel != sp.Yearly+by["YouTube"].Yearly {
		t.Errorf("cancel verdict %+v (to cancel %v)", sp, s.ToCancel)
	}
	if err := insights.SetSubVerdict(d, "x|y", insights.SubVerdict{Verdict: "cancel"}, now); err == nil {
		t.Error("cancel without a date accepted")
	}
}
