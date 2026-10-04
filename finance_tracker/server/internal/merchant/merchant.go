// Package merchant turns raw bank payee strings into one canonical merchant
// name, so "UAB Neste Lietuva 08222 Viln", "Neste Oil Luksio" and "NESTE"
// all aggregate as "Neste".
package merchant

import (
	"regexp"
	"strings"
	"unicode"
)

// alias maps a folded, upper-cased token to a canonical merchant. Order
// matters: the first hit wins, so specific entries precede generic ones
// (Norfos vaistinė before Norfa).
var aliases = []struct{ match, name string }{
	{"NORFOS VAISTINE", "Norfos vaistinė"},
	{"EUROVAISTINE", "Eurovaistinė"},
	{"GINTARINE VAISTINE", "Gintarinė vaistinė"},
	{"CAMELIA", "Camelia"},
	{"BENU ", "Benu"},
	{"MAXIMA", "Maxima"},
	{"LIDL", "Lidl"},
	{"NORFA", "Norfa"},
	{"RIMI", "Rimi"},
	{"BARBORA", "Barbora"},
	{"ALIEXPRESS", "AliExpress"},
	{"ALIPAY", "AliExpress"},
	{"EBAY", "eBay"},
	{"AMAZON", "Amazon"},
	{"TEMU", "Temu"},
	{"PIGU", "Pigu.lt"},
	{"VARLE", "Varlė.lt"},
	{"KESKO", "Senukai"},
	{"SENUKAI", "Senukai"},
	{"MOKI VEZI", "Moki Veži"},
	{"MOKI-VEZI", "Moki Veži"},
	{"IKEA", "IKEA"},
	{"JYSK", "JYSK"},
	{"IGNITIS", "Ignitis"},
	{"ENERGIJOS SKIRSTYMO", "ESO"},
	{"ENERGIJOS TIEKIMAS", "Lietuvos energijos tiekimas"},
	{"DUJU TIEKIMAS", "Lietuvos dujų tiekimas"},
	{"VILNIAUS ENERGIJA", "Vilniaus energija"},
	{"SILUMOS TINKLAI", "Vilniaus šilumos tinklai"},
	{"VILNIAUS VANDENYS", "Vilniaus vandenys"},
	{"MANO BUSTAS", "Mano būstas"},
	{"ZIRMUNU BUSTAS", "Žirmūnų būstas"},
	{"SAUGOS TARNYBA ARGUS", "Argus"},
	{"TELIA", "Telia"},
	{"TEO LT", "Telia"},
	{"BITE LIETUVA", "Bitė"},
	{"INTERNETO VIZIJA", "Interneto vizija"},
	{"NESTE", "Neste"},
	{"BALTICPETROLEUM", "Baltic Petroleum"},
	{"BALTIC PETRO", "Baltic Petroleum"},
	{"CIRCLE K", "Circle K"},
	{"VIADA", "Viada"},
	{"ORLEN", "Orlen"},
	{"MPARKING", "mParking"},
	{"UNIPARK", "Unipark"},
	{"TUVLITA", "Tuvlita"},
	{"CITYBEE", "CityBee"},
	{"BOLT", "Bolt"},
	{"WOLT", "Wolt"},
	{"MCDONALD", "McDonald's"},
	{"HESBURGER", "Hesburger"},
	{"GYM PLIUS", "Gym+"},
	{"NORTHWAY", "Northway"},
	{"YOUTUBE", "YouTube Premium"},
	{"GOOGLE STORAGE", "Google One"},
	{"GOOGLE ONE", "Google One"},
	{"CONTRIBEE", "Contribee"},
	{"PATREON", "Patreon"},
	{"MSFT", "Microsoft"},
	{"MICROSOFT", "Microsoft"},
	{"CLAUDE", "Claude"},
	{"ANTHROPIC", "Claude"},
	{"NETFLIX", "Netflix"},
	{"SPOTIFY", "Spotify"},
	{"SKAITLIS", "Skaitlis"},
	{"AIRBNB", "Airbnb"},
	{"BOOKING.COM", "Booking.com"},
	{"RYANAIR", "Ryanair"},
	{"WIZZ", "Wizz Air"},
	{"WIZAIR", "Wizz Air"},
	{"MOKESCIU INSPEKCIJA", "VMI"},
	{"REGISTRU CENTRAS", "Registrų centras"},
	{"SODRA", "Sodra"},
	{"VSDF", "Sodra"},
	{"INTERACTIVE BROKERS", "IBKR"},
	{"IBKR", "IBKR"},
	{"REVOLUT", "Revolut"},
	{"ARTEA", "Artea"},
	{"INVL", "Artea"},
	{"GJENSIDIGE", "Gjensidige"},
	{"COMPENSA", "Compensa"},
	{"LIETUVOS DRAUDIMAS", "Lietuvos draudimas"},
	{"PZU", "PZU"},
	{"TOKVILA", "Tokvila"},
	{"NOVATURAS", "Novaturas"},
	{"EVELINA", "Evelina"},
	{"PLYTNIKAIT", "Evelina"},
	{"360ARENA", "360 Arena"},
	{"SKYPARK", "SkyPark"},
	{"SKY PARK", "SkyPark"},
}

// iki is also the Lithuanian word for "until", so the chain only matches as
// a leading word.
var ikiRe = regexp.MustCompile(`^IKI\b`)

// Fold strips Lithuanian diacritics and upper-cases, for matching.
func Fold(s string) string {
	return strings.ToUpper(folder.Replace(s))
}

var folder = strings.NewReplacer(
	"ą", "a", "č", "c", "ę", "e", "ė", "e", "į", "i", "š", "s", "ų", "u", "ū", "u", "ž", "z",
	"Ą", "A", "Č", "C", "Ę", "E", "Ė", "E", "Į", "I", "Š", "S", "Ų", "U", "Ū", "U", "Ž", "Z",
)

// Known returns the canonical merchant for a raw string when it matches the
// alias table, or "".
func Known(raw string) string {
	up := Fold(raw)
	for _, a := range aliases {
		if strings.Contains(up, a.match) {
			return a.name
		}
	}
	if ikiRe.MatchString(strings.TrimSpace(up)) {
		return "IKI"
	}
	return ""
}

var (
	legalForms = regexp.MustCompile(`(?i)\b(UAB|AB|MB|VSI|VŠĮ|VŠI|IĮ|II|SIA|OY|AS|LTD|LIMITED|GMBH|INC|LLC|S\.?A\.?|APS|BUDŽETINĖ ĮSTAIGA|BIUDŽETINĖ ĮSTAIGA|UŽDAROJI AKCINĖ BENDROVĖ|AKCINĖ BENDROVĖ|VIEŠOJI ĮSTAIGA|VALSTYBĖS ĮMONĖ|SAVIVALDYBĖS ĮMONĖ)\b\.?,?`)
	terminalTail = regexp.MustCompile(`(?i)(\s+(LT-)?\d{3,}.*$)|(\s+[A-Z]{1,3}-?\d+.*$)|(\*\S*)`)
	cityTail     = regexp.MustCompile(`(?i)\s+(VILNIUS|VILNIAUS|VILNI|VILN|KAUNAS|KLAIPEDA|LUXEMBOURG|LONDON|DUBLIN|RIGA|000LT|LT)\.?$`)
	spaces       = regexp.MustCompile(`\s+`)
	quotes       = strings.NewReplacer(`"`, "", `“`, "", `”`, "", `„`, "", `'`, "")
)

// Clean derives a readable merchant from a bank payee that is not in the
// alias table: strips legal forms, terminal ids, postal codes and trailing
// city names, and title-cases ALL-CAPS strings.
func Clean(raw string) string {
	s := raw
	if i := strings.Index(s, " ("); i > 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, '\\'); i > 0 {
		s = s[:i]
	}
	s = quotes.Replace(s)
	s = legalForms.ReplaceAllString(s, " ")
	s = terminalTail.ReplaceAllString(s, "")
	for i := 0; i < 2; i++ {
		s = cityTail.ReplaceAllString(strings.TrimSpace(s), "")
	}
	s = strings.Trim(spaces.ReplaceAllString(s, " "), " ,.-/")
	if isAllCaps(s) {
		s = titleCase(s)
	}
	if r := []rune(s); len(r) > 40 {
		s = strings.TrimSpace(string(r[:40]))
	}
	return s
}

// LooksLikePayee reports whether a description reads like a bank payee
// string rather than something a person typed ("House payment", "Child
// benefit"). Bank strings shout, carry legal forms or terminal numbers.
func LooksLikePayee(s string) bool {
	if legalForms.MatchString(s) {
		return true
	}
	head := s
	if i := strings.Index(head, " ("); i > 0 {
		head = head[:i]
	}
	if isAllCaps(head) && len([]rune(head)) >= 3 {
		return true
	}
	return regexp.MustCompile(`\d{4,}`).MatchString(head)
}

// intermediaries are payment processors that front the real seller; v1
// stored the seller in brackets after them ("Paysera LT, UAB (Tabelo.lt)").
var intermediaries = []string{"PAYSERA", "OPAY", "EVP INTERNATIONAL", "ELEKTRONINIU MOKEJIMU", "TRUSTLY", "MAKSEKESKUS", "PAYPAL"}

// Resolve returns the best merchant name for a description: the alias table
// first, then a cleaned payee when the text looks like one, else "".
func Resolve(desc string) string {
	up := Fold(desc)
	for _, im := range intermediaries {
		if strings.Contains(up, im) {
			if i, j := strings.Index(desc, "("), strings.LastIndex(desc, ")"); i > 0 && j > i+1 {
				if inner := strings.TrimSpace(desc[i+1 : j]); inner != "" {
					if k := Known(inner); k != "" {
						return k
					}
					return Clean(inner)
				}
			}
			if im == "PAYPAL" {
				if rest := strings.TrimSpace(desc[strings.Index(up, "PAYPAL")+6:]); strings.HasPrefix(rest, "*") {
					return Clean(strings.TrimPrefix(rest, "*"))
				}
			}
			return Clean(desc)
		}
	}
	if m := Known(desc); m != "" {
		return m
	}
	if strings.HasPrefix(desc, "Card refund") {
		rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(desc, "Card refund"), ":"))
		if rest == "" || strings.HasPrefix(rest, "(") {
			return ""
		}
		return Resolve(rest)
	}
	if LooksLikePayee(desc) {
		return Clean(desc)
	}
	return ""
}

func isAllCaps(s string) bool {
	letters, upper := 0, 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	return letters >= 2 && upper == letters
}

func titleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}
