package handler

import "testing"

// TestValidTicker guards the ticker whitelist. The regression that motivated
// it: a too-strict class rejected "EURUSD=X", which the frontend fetches as
// its USD/EUR rate — silently blanking every EUR-converted portfolio total.
func TestValidTicker(t *testing.T) {
	valid := []string{
		"AAPL", "VWCE", "BRK-B", "BTC-USD", "RDS.A",
		"EURUSD=X", // FX pair — the app's USD/EUR rate
		"GC=F",     // futures
		"^GSPC",    // index
	}
	for _, tk := range valid {
		if !validTicker.MatchString(tk) {
			t.Errorf("expected %q to be accepted", tk)
		}
	}

	invalid := []string{
		"", "AAAAAAAAAAAAAAAA", // empty / >15 chars
		"abc'; DROP", "a b", "AAPL/../x", "<script>", "eur usd",
	}
	for _, tk := range invalid {
		if validTicker.MatchString(tk) {
			t.Errorf("expected %q to be rejected", tk)
		}
	}
}
