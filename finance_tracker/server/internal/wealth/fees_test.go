package wealth_test

import (
	"testing"

	. "ft/internal/testutil"
	"ft/internal/wealth"
)

// €20,000 in VWCE at a typical 0.22%, €5,000 in a single stock (no ongoing
// charge) and a €10,000 bank fund at 1.5% (the owner's figure): the bank fund
// dominates the cost, and moving it to a 0.2% index fund saves thousands
// over 20 years; a pension without a figure yet is counted as missing.
func TestFeeAnalyserFindsTheDearFund(t *testing.T) {
	d := DB(t)
	if err := wealth.SaveFeeSetting(d, "robur", wealth.FeeSetting{TER: 1.5}); err != nil {
		t.Fatal(err)
	}
	if err := wealth.SaveFeeSetting(d, "robur", wealth.FeeSetting{TER: 9}); err == nil {
		t.Error("a 9% fee accepted")
	}
	f := wealth.AnalyseFees(wealth.LoadFeeSettings(d),
		[]wealth.FeeLine{{ID: "VWCE", Name: "Vanguard All-World", Value: E(20000)}, {ID: "NVDA", Name: "Nvidia", Value: E(5000)}},
		[]wealth.FeeLine{{ID: "robur", Name: "Swedbank Robur", Value: E(10000)}, {ID: "artea", Name: "Artea", Value: E(8000)}}, 7.5)
	by := map[string]wealth.FeeLine{}
	for _, l := range f.Lines {
		by[l.ID] = l
	}
	if v := by["VWCE"]; v.Source != "typical" || v.TER != 0.22 || v.Yearly != E(44) {
		t.Errorf("VWCE %+v", v)
	}
	if n := by["NVDA"]; n.Kind != "stock" || n.Yearly != 0 {
		t.Errorf("stock %+v", n)
	}
	r := by["robur"]
	if r.Source != "yours" || r.Yearly != E(150) || r.Saving20 < E(2000) || f.Lines[0].ID != "robur" {
		t.Errorf("bank fund %+v (first %s)", r, f.Lines[0].ID)
	}
	if f.Missing != 1 || by["artea"].Source != "missing" {
		t.Errorf("missing %d %+v", f.Missing, by["artea"])
	}
}

// A core fund the typical table doesn't know is asked for, not taken for a
// share with no charge.
func TestFeeAnalyserAsksForAnUnknownCoreFund(t *testing.T) {
	f := wealth.AnalyseFees(nil, []wealth.FeeLine{{ID: "VALL", Name: "VALL", Value: E(2000)}, {ID: "OKLO", Name: "Oklo", Value: E(2000)}}, nil, 7.5, "VWCE", "VALL")
	by := map[string]wealth.FeeLine{}
	for _, l := range f.Lines {
		by[l.ID] = l
	}
	if by["VALL"].Kind != "fund" || by["VALL"].Source != "missing" || by["OKLO"].Kind != "stock" || f.Missing != 1 {
		t.Errorf("%+v", f.Lines)
	}
	// Given a fee (saved in any case), an unknown ticker is a fund: its
	// saving against a low-cost fund counts.
	f = wealth.AnalyseFees(map[string]wealth.FeeSetting{"oklo": {TER: 0.9}}, []wealth.FeeLine{{ID: "OKLO", Name: "Oklo", Value: E(2000)}}, nil, 7.5)
	if l := f.Lines[0]; l.Kind != "fund" || l.Source != "yours" || l.Yearly != E(18) || l.Saving20 <= 0 {
		t.Errorf("a fund with the owner's fee %+v", l)
	}
}
