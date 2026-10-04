package importv1

import (
	"strings"

	"ft/internal/ledger"
	"ft/internal/merchant"
)

// Account ids: v1 key → v2 id.
var accountID = map[string]string{
	"seb": "seb", "swed": "swed", "cash": "cash", "rev_m": "revolut", "rev_r": "revolut_etf",
	"swed_etf": "swed_etf", "rev_stocks": "revolut_stocks", "ibkr_stocks": "ibkr", "seb_pen": "seb_pension",
	"art": "artea", "luminor": "luminor", "mbtc": "btc_m", "rbtc": "btc_r",
}

// MapAccount converts a v1 account key.
func MapAccount(k string) string {
	k = strings.TrimSpace(k)
	if k == "" {
		return ""
	}
	if v, ok := accountID[k]; ok {
		return v
	}
	return strings.TrimPrefix(k, "acc_")
}

type acctDef struct {
	name, institution, kind string
	liquid                  bool
	sort                    int
}

var builtinAccounts = map[string]acctDef{
	"swed":           {"Swedbank", "Swedbank", "checking", true, 10},
	"seb":            {"SEB", "SEB", "checking", true, 20},
	"revolut":        {"Revolut", "Revolut", "checking", true, 30},
	"cash":           {"Cash", "", "cash", true, 40},
	"luminor":        {"Luminor", "Luminor", "checking", true, 90},
	"ibkr":           {"IBKR", "IBKR", "brokerage", true, 100},
	"revolut_stocks": {"Revolut Stocks", "Revolut", "brokerage", true, 110},
	"revolut_etf":    {"Revolut ETF", "Revolut", "brokerage", true, 120},
	"swed_etf":       {"Swedbank funds", "Swedbank", "brokerage", true, 130},
	"btc_m":          {"BTC (M)", "", "crypto", true, 200},
	"btc_r":          {"BTC (R)", "", "crypto", true, 210},
	"seb_pension":    {"SEB pension (II pillar)", "SEB", "pension", false, 300},
	"artea":          {"Artea (III pillar)", "Artea", "pension", false, 310},
}

// Labels that become the merchant (vendors and employers).
var merchantLabels = map[string]string{
	"maxima": "Maxima", "lidl": "Lidl", "iki": "IKI", "rimi": "Rimi", "norfa": "Norfa", "barbora": "Barbora",
	"aliexpress": "AliExpress", "ebay": "eBay", "senukai": "Senukai", "depo": "Depo", "moki vezi": "Moki Veži",
	"ikea": "IKEA", "jysk": "JYSK", "telia": "Telia", "bite": "Bitė", "youtube": "YouTube Premium", "netflix": "Netflix",
	"claude": "Claude", "revolut": "Revolut", "ibkr": "IBKR", "artea": "Artea", "danske": "Danske Bank",
	"barclays": "Barclays", "doclogix": "DocLogix", "app camp": "App Camp", "pvcase": "PVcase", "nexos": "Nexos.ai",
	"vipps mobilepay": "Vipps MobilePay", "spectra tech": "Spectra Tech", "algis ramanauskas": "Algis Ramanauskas",
	"vmi": "VMI", "alna": "Alna", "google": "Google",
}

// Labels that survive as tags: people, properties, trips, context.
var keepTags = map[string]bool{
	"kids": true, "evelina": true, "kristina": true, "friend": true, "jekaterina": true, "parents": true,
	"family": true, "ruta": true, "apartment": true, "house": true, "work": true, "vacation": true, "divorce": true,
	"rav4": true,
}

// labelCategory maps a refining label onto a v2 category, in priority order.
var labelCategory = []struct {
	labels []string
	cat    string
}{
	{[]string{"alimony"}, "kids.alimony"},
	{[]string{"bank fee"}, "finance.bank_fees"},
	{[]string{"tax", "vmi"}, "finance.taxes"},
	{[]string{"fine"}, "finance.fines"},
	{[]string{"legal"}, "finance.legal"},
	{[]string{"groceries", "maxima", "lidl", "iki", "rimi", "norfa", "barbora"}, "food.groceries"},
	{[]string{"fast food"}, "food.fast_food"},
	{[]string{"delivery"}, "food.delivery"},
	{[]string{"coffee"}, "food.coffee"},
	{[]string{"lunch"}, "food.work_lunch"},
	{[]string{"restaurant", "dinner"}, "food.restaurants"},
	{[]string{"fuel"}, "transport.fuel"},
	{[]string{"parking"}, "transport.parking"},
	{[]string{"car wash", "car"}, "transport.car"},
	{[]string{"taxi", "bus"}, "transport.taxi"},
	{[]string{"electricity"}, "utilities.electricity"},
	{[]string{"gas", "heating"}, "utilities.heating"},
	{[]string{"internet", "phone", "telia", "bite"}, "utilities.telecom"},
	{[]string{"security"}, "utilities.security"},
	{[]string{"pharmacy"}, "health.pharmacy"},
	{[]string{"therapy"}, "health.therapy"},
	{[]string{"gym"}, "health.fitness"},
	{[]string{"beauty", "haircut", "spa"}, "health.care"},
	{[]string{"dental", "medical", "optics"}, "health.medical"},
	{[]string{"aliexpress", "ebay"}, "shopping.online"},
	{[]string{"electronics", "ps5", "tv"}, "shopping.electronics"},
	{[]string{"clothing", "shoes"}, "shopping.clothing"},
	{[]string{"bar", "beer"}, "leisure.going_out"},
	{[]string{"cinema", "concert"}, "leisure.events"},
	{[]string{"lottery"}, "leisure.lottery"},
	{[]string{"hotel"}, "travel.lodging"},
	{[]string{"flights"}, "travel.flights"},
	{[]string{"diy", "moki vezi", "senukai", "depo", "construction", "fix", "air conditioner", "service"}, "housing.maintenance"},
	{[]string{"furniture", "ikea", "jysk"}, "housing.furnishing"},
	{[]string{"rent", "dorm"}, "housing.rent"},
	{[]string{"youtube", "netflix"}, "subscriptions.media"},
	{[]string{"claude", "web"}, "subscriptions.software"},
	{[]string{"algis ramanauskas"}, "subscriptions.creators"},
	{[]string{"gift"}, "gifts"},
}

// Purpose categories keep their top level: a grocery run on a trip is trip
// money (and the Vacation fund pays for it), labels refine only inside it.
var purposeTop = map[string]string{"Vacation": "travel", "Kids": "kids", "Dating": "dating"}

// v1 category → default v2 category when nothing more specific is known.
var expenseDefault = map[string]string{
	"Food": "food", "Entertainment": "leisure", "Finance": "finance", "Housing": "housing", "Utilities": "utilities",
	"Transport": "transport", "Health": "health", "Subscriptions": "subscriptions", "Gifts": "gifts",
	"Clothing": "shopping.clothing", "Vacation": "travel.trip", "Kids": "kids.general", "Dating": "dating",
	"Gaming": "leisure.hobbies", "Divorce": "finance.divorce",
}

// Merchant → category, for rows with no refining label.
var merchantCategory = map[string]string{
	"Maxima": "food.groceries", "Lidl": "food.groceries", "IKI": "food.groceries", "Rimi": "food.groceries", "Norfa": "food.groceries",
	"Barbora": "food.groceries", "Wolt": "food.delivery", "McDonald's": "food.fast_food", "Hesburger": "food.fast_food",
	"Ignitis": "utilities.electricity", "ESO": "utilities.electricity", "Lietuvos energijos tiekimas": "utilities.electricity",
	"Vilniaus šilumos tinklai": "utilities.heating", "Vilniaus energija": "utilities.heating", "Lietuvos dujų tiekimas": "utilities.heating",
	"Vilniaus vandenys": "utilities.water", "Telia": "utilities.telecom", "Bitė": "utilities.telecom", "Interneto vizija": "utilities.telecom",
	"Mano būstas": "utilities.building", "Žirmūnų būstas": "utilities.building", "Argus": "utilities.security",
	"Neste": "transport.fuel", "Baltic Petroleum": "transport.fuel", "Circle K": "transport.fuel", "Viada": "transport.fuel", "Orlen": "transport.fuel",
	"mParking": "transport.parking", "Unipark": "transport.parking", "Tuvlita": "transport.car", "Tokvila": "transport.car",
	"Bolt": "transport.taxi", "CityBee": "transport.taxi",
	"Eurovaistinė": "health.pharmacy", "Gintarinė vaistinė": "health.pharmacy", "Norfos vaistinė": "health.pharmacy", "Camelia": "health.pharmacy",
	"Benu": "health.pharmacy", "Gym+": "health.fitness", "Northway": "health.medical",
	"AliExpress": "shopping.online", "eBay": "shopping.online", "Pigu.lt": "shopping.online", "Varlė.lt": "shopping.online",
	"Temu": "shopping.online", "Amazon": "shopping.online",
	"Senukai": "housing.maintenance", "Moki Veži": "housing.maintenance", "Depo": "housing.maintenance",
	"IKEA": "housing.furnishing", "JYSK": "housing.furnishing",
	"YouTube Premium": "subscriptions.media", "Netflix": "subscriptions.media", "Spotify": "subscriptions.media",
	"Contribee": "subscriptions.creators", "Patreon": "subscriptions.creators", "Microsoft": "subscriptions.software",
	"Google One": "subscriptions.software", "Claude": "subscriptions.software",
	"VMI": "finance.taxes", "Registrų centras": "finance.legal", "Gjensidige": "finance.insurance", "Compensa": "finance.insurance",
	"Lietuvos draudimas": "finance.insurance", "PZU": "finance.insurance", "Skaitlis": "kids.education",
	"Novaturas": "travel.trip", "Airbnb": "travel.lodging", "Booking.com": "travel.lodging", "Ryanair": "travel.flights", "Wizz Air": "travel.flights",
}

// Mapped is one v1 transaction in v2 terms.
type Mapped struct {
	Kind, Category, Merchant, Account, ToAccount string
	Tags                                         []string
	Dropped                                      []string // labels consumed or discarded
}

func splitLabels(s string) []string {
	var out []string
	for _, l := range strings.Split(s, ",") {
		if l = strings.ToLower(strings.TrimSpace(l)); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func hasAny(labels []string, want ...string) bool {
	for _, l := range labels {
		for _, w := range want {
			if l == w {
				return true
			}
		}
	}
	return false
}

func contains(s string, subs ...string) bool {
	f := merchant.Fold(s)
	for _, x := range subs {
		if strings.Contains(f, merchant.Fold(x)) {
			return true
		}
	}
	return false
}

// MapTx converts one v1 transaction.
func MapTx(t V1Tx) Mapped {
	labels := splitLabels(t.Labels)
	m := Mapped{}
	debit, credit := MapAccount(t.Debit), MapAccount(t.Credit)
	if debit == "" && credit == "" && t.Source != "" {
		if t.Type == "income" {
			credit = MapAccount(t.Source)
		} else {
			debit = MapAccount(t.Source)
		}
	}

	// Merchant: an explicit vendor label beats parsing the comment.
	for _, l := range labels {
		if v, ok := merchantLabels[l]; ok && m.Merchant == "" {
			m.Merchant = v
		}
	}
	if m.Merchant == "" {
		m.Merchant = merchant.Resolve(t.Comment)
	}

	switch t.Type {
	case "income":
		m.Kind, m.Account = "income", firstNonEmpty(credit, debit)
		m.Category = incomeCategory(t, labels)
	case "investment":
		m.Kind, m.Account, m.ToAccount = "transfer", debit, credit
		m.Category = transferCategory(t, &m)
	default:
		m.Kind, m.Account = "expense", firstNonEmpty(debit, credit)
		m.Category = expenseCategory(t, labels, m.Merchant)
		if m.Category == "transfer.debt" {
			m.Kind, m.ToAccount = "transfer", "mortgage"
		}
	}

	// Tags: people, properties, trips, context. Everything else was either
	// absorbed into the category / merchant or was noise.
	top := ledger.Top(m.Category)
	for _, l := range labels {
		switch {
		case strings.HasPrefix(l, "trip:") || strings.HasPrefix(l, "owed"):
			m.Tags = append(m.Tags, l)
		case keepTags[l]:
			if (l == "kids" && top == "kids") || (l == "divorce" && m.Category == "finance.divorce") || (l == "vacation" && top == "travel") {
				m.Dropped = append(m.Dropped, l)
				continue
			}
			m.Tags = append(m.Tags, l)
		default:
			m.Dropped = append(m.Dropped, l)
		}
	}
	if contains(t.Comment, "EVELINA", "PLYTNIKAIT") && !hasAny(m.Tags, "evelina") {
		m.Tags = append(m.Tags, "evelina")
	}
	if t.Category == "Dating" && !hasAny(m.Tags, "kristina") && t.Date >= "2026-04-20" {
		m.Tags = append(m.Tags, "kristina")
	}
	m.Tags = ledger.NormalizeTags(m.Tags)
	return m
}

func incomeCategory(t V1Tx, labels []string) string {
	switch t.Category {
	case "Salary":
		return "salary"
	case "Freelance":
		if contains(t.Comment, "binance") {
			return "other_income"
		}
		return "side_income"
	case "Reimbursement":
		switch {
		case hasAny(labels, "dividents", "dividends"):
			return "investment_income"
		case contains(t.Comment, "child benefit", "išmoka vaik", "ismoka vaik", "sodra", "vsdf"):
			return "benefits"
		}
		return "refunds"
	}
	return "other_income"
}

func transferCategory(t V1Tx, m *Mapped) string {
	switch t.Category {
	case "Pension":
		if m.ToAccount == "" {
			m.ToAccount = "artea"
		}
		return "transfer.pension"
	case "Stocks & ETF":
		if m.ToAccount == "" {
			switch {
			case contains(t.Comment, "ibkr", "interactive brokers"):
				m.ToAccount = "ibkr"
			case contains(t.Comment, "robur", "mini invest", "swed etf", "swedbank"):
				m.ToAccount = "swed_etf"
			case contains(t.Comment, "binance", "btc"):
				m.ToAccount = "btc_m"
			case contains(t.Comment, "revolut"):
				m.ToAccount = "revolut_stocks"
			}
		}
		return "transfer.invest"
	case "Vehicle":
		if contains(t.Comment, "pradin", "buyout", "down payment") {
			return "transfer.asset"
		}
		return "transfer.debt"
	case "Real Estate":
		return "transfer.asset"
	}
	if contains(t.Comment, "artea", "invl") {
		if m.ToAccount == "" {
			m.ToAccount = "artea"
		}
		return "transfer.pension"
	}
	return "transfer.internal"
}

func expenseCategory(t V1Tx, labels []string, merch string) string {
	c := t.Comment
	// Mortgage: principal is a transfer into the loan (equity), interest is
	// the cost of the house, and pre-2026 payments are one combined amount.
	if hasAny(labels, "loan", "loan return") {
		switch {
		case contains(c, "notar"):
			return "finance.legal"
		case contains(c, "return", "grąžin", "grazin") || hasAny(labels, "loan return"):
			return "transfer.debt"
		case contains(c, "interest", "palūkan", "palukan"):
			return "housing.mortgage_interest"
		}
		return "housing.mortgage"
	}
	if hasAny(labels, "alimony") || contains(c, "aliment", "alimon") {
		return "kids.alimony"
	}
	if hasAny(labels, "divorce") && t.Category == "Finance" {
		return "finance.divorce"
	}
	if contains(c, "untracked cash") {
		return "other.cash"
	}
	if t.Category == "Finance" {
		switch {
		case contains(c, "card fee", "plan fee", "banko mokestis", "bank fee"):
			return "finance.bank_fees"
		case contains(c, "notar", "registr"):
			return "finance.legal"
		case contains(c, "draud", "insurance"):
			return "finance.insurance"
		case contains(c, "gedimino techn"):
			return "other.education"
		case hasAny(labels, "evelina") || contains(c, "EVELINA", "PLYTNIKAIT"):
			return "other.family"
		case hasAny(labels, "parents"):
			return "other.family"
		}
	}

	purpose := purposeTop[t.Category]
	for _, lc := range labelCategory {
		if !hasAny(labels, lc.labels...) {
			continue
		}
		if purpose != "" && ledger.Top(lc.cat) != purpose {
			continue
		}
		return lc.cat
	}
	if t.Category == "Health" && contains(c, "vaistin") {
		return "health.pharmacy"
	}
	if t.Category == "Housing" && contains(c, "insurance", "draud") {
		return "housing.insurance"
	}
	if t.Category == "Utilities" && contains(c, "radijo ir televizijos", "miesto gijos", "rinkliava") {
		return "utilities.building"
	}
	if mc, ok := merchantCategory[merch]; ok {
		if purpose == "" || ledger.Top(mc) == purpose {
			// Don't let a merchant drag a row out of the user's own domain
			// (Senukai bought for the kids stays Kids).
			if d := expenseDefault[t.Category]; d == "" || ledger.Top(d) == ledger.Top(mc) || t.Category == "Entertainment" || t.Category == "Finance" {
				return mc
			}
		}
	}
	if d, ok := expenseDefault[t.Category]; ok {
		return d
	}
	return "other"
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// RuleFor converts a v1 label rule into a v2 rule, or ok=false when the rule
// only re-stated its category.
func RuleFor(r V1Rule) (ledger.Rule, bool) {
	label := strings.ToLower(strings.TrimSpace(r.Label))
	out := ledger.Rule{Pattern: strings.TrimSpace(r.Match), Enabled: true, Priority: 100}
	switch {
	case strings.HasPrefix(label, "trip:") || keepTags[label]:
		out.AddTags = []string{label}
	case merchantLabels[label] != "" && out.Pattern != "":
		out.SetMerchant = merchantLabels[label]
		for _, lc := range labelCategory {
			if hasAny([]string{label}, lc.labels...) {
				out.SetCategory = lc.cat
				break
			}
		}
	default:
		for _, lc := range labelCategory {
			if hasAny([]string{label}, lc.labels...) {
				out.SetCategory = lc.cat
				break
			}
		}
	}
	if out.SetCategory == "" && out.SetMerchant == "" && len(out.AddTags) == 0 {
		return out, false
	}
	if out.Pattern == "" {
		// A category-only v1 rule ("every Dating row gets kristina"):
		// keep the condition, translated.
		top := expenseDefault[r.Category]
		if top == "" {
			return out, false
		}
		out.WhenCategory = ledger.Top(top)
		if out.SetCategory != "" {
			return out, false // "every X is Y" makes no sense across the new tree
		}
	}
	if purpose := purposeTop[r.Category]; purpose != "" {
		if out.SetCategory != "" && ledger.Top(out.SetCategory) != purpose {
			out.SetCategory = ""
		}
		out.WhenCategory = purpose
	}
	if out.SetCategory == "" && out.SetMerchant == "" && len(out.AddTags) == 0 {
		return out, false
	}
	return out, true
}
