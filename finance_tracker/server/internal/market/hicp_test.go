package market

import "testing"

func TestParseHICPReadsTheLatestMonth(t *testing.T) {
	raw := []byte(`{"value":{"0":6.1},"dimension":{"time":{"category":{"index":{"2026-09":0}}}}}`)
	v := parseHICP(raw)
	if v == nil || v.Rate != 6.1 || v.Month != "2026-09" {
		t.Fatalf("%+v", v)
	}
	if parseHICP([]byte(`not json`)) != nil {
		t.Error("garbage parsed")
	}
}
