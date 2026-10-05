package merchant

import "testing"

func TestResolve(t *testing.T) {
	cases := map[string]string{
		"MAXIMA LT, UAB":                    "Maxima",
		"UAB Neste Lietuva 08222 Viln":      "Neste",
		"Neste Oil Luksio":                  "Neste",
		"NORFOS VAISTINE 06200 VILNIU":      "Norfos vaistinė",
		"NORFA - X":                         "Norfa",
		"IKI Pilaite":                       "IKI",
		"Paysera LT, UAB (Tabelo.lt)":       "Tabelo.lt",
		"PAYSERA LT UAB (pardavėjas: Pigu)": "Pigu.lt",
		"PAYPAL *REOLINK 0000000000 4":      "Reolink",
		"aliexpress L-1528 Luxembourg":      "AliExpress",
		"GOOGLE *YouTubePremium SW1W":       "YouTube Premium",
		"EVELINA PLYTNIKAITĖ (Lizingas)":    "Evelina",
		"JUSTAS MEŠKAUSKIS":                 "Justas Meškauskis",
		"UAB \"TECHNICA SERVICE\"":          "Technica Service",
		"House payment":                     "",
		"Child benefit":                     "",
		"Card refund (Swedbank)":            "",
		"Card refund: Neste Oil Nemun":      "Neste",
	}
	for in, want := range cases {
		if got := Resolve(in); got != want {
			t.Errorf("Resolve(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanStripsTerminalNoise(t *testing.T) {
	for in, want := range map[string]string{
		"Probroexpress.lt\\vito Gerulaicio G. 1\\+3": "Probroexpress.lt",
		"BARAS \"ARTISTAI\" 01130 VILNIUS":           "Baras Artistai",
		"UAB EUROVAISTINE V0250 06269":               "Eurovaistine",
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}

// "iki" is also the Lithuanian word for "until" — only a leading IKI is the chain.
func TestIKIIsNotUntil(t *testing.T) {
	if Known("mokėjimas iki 2026-10-01") != "" {
		t.Error("'iki' inside a sentence must not resolve to the IKI chain")
	}
	if Known("IKI EXPRESS") != "IKI" {
		t.Error("leading IKI is the chain")
	}
}

func TestFoldAndLooksLikePayee(t *testing.T) {
	if Fold("Šilumos ąžuolas") != "SILUMOS AZUOLAS" {
		t.Error(Fold("Šilumos ąžuolas"))
	}
	if !LooksLikePayee("UAB Kesko") || !LooksLikePayee("MOKI VEZI") || LooksLikePayee("Lunch with team") {
		t.Error("payee heuristics")
	}
}
