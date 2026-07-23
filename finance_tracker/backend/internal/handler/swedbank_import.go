package handler

import (
	"encoding/csv"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

// Swedbank LT account statement CSV importer. Column layout:
//
//	0 account, 1 row kind (10 opening/20 tx/82 turnover/86 closing), 2 date,
//	3 payee, 4 details, 5 amount, 6 currency, 7 D/K, 8 record no, 9 code
//	(TT/M fees, K card, MK transfer, AS/K2/LS summary rows)
//
// Card purchases are re-dated to the actual purchase date embedded in the
// details ("PIRKINYS … 2022.01.04 …"), matching how earlier statements were
// imported — that keeps boundary rows deduplicable across statement files.

type swedTx struct {
	Date     time.Time
	Type     domain.TransactionType
	Category domain.Category
	Comment  string
	Labels   string
	Amount   float64
	Debit    string
	Credit   string
}

var purchaseDateRe = regexp.MustCompile(`(?:PIRKINYS|GRĄŽINIMAS)\s+\d{6}\*+\d{4}\s+(\d{4})\.(\d{2})\.(\d{2})`)
var cashOpDateRe = regexp.MustCompile(`(?:GRYNIEJI|INESIMAS|ĮNEŠIMAS)\s+\d[\d*]+\s+(\d{2})\.(\d{2})\.(\d{2})`)
var fxCurrencyRe = regexp.MustCompile(`\b(DKK|NOK|SEK|GBP|TRY|PLN|CZK|HUF|CHF)\b`)

// Official fixed conversion rate: Lithuania joined the euro on 2015-01-01.
const ltlPerEur = 3.4528

// swedBalance is an account-level balance stated by the bank itself in the
// statement's opening ("Likutis pradžiai") and closing ("Likutis pabaigai")
// rows — used to restore the Swedbank balance history.
type swedBalance struct {
	Date   time.Time
	Amount float64
}

// parseSwedbankCSV turns a statement into classified transactions, a
// month-end balance series reconstructed from the bank-stated opening
// balance plus every row's flow, and a count of skipped internal
// own-account movements.
func parseSwedbankCSV(r io.Reader) (txs []swedTx, balances []swedBalance, internal int, err error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	type flow struct {
		date  time.Time
		delta float64
	}
	var flows []flow
	var openDate, closeDate time.Time
	var openBal float64
	first := true
	for {
		rec, rerr := reader.Read()
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, nil, 0, rerr
		}
		if first { // header
			first = false
			continue
		}
		if len(rec) < 10 {
			continue
		}
		kind := strings.TrimSpace(rec[1])
		date, derr := time.Parse("2006-01-02", strings.TrimSpace(rec[2]))
		amount, aerr := strconv.ParseFloat(strings.TrimSpace(rec[5]), 64)
		currency := strings.TrimSpace(rec[6])
		if currency == "LTL" {
			amount = math.Round(amount/ltlPerEur*100) / 100
		}

		// Opening/closing rows: the bank's own balance statement. A date may
		// appear twice (separate EUR and LTL sub-balances) — sum them.
		if kind == "10" || kind == "86" {
			if derr == nil && aerr == nil {
				if kind == "10" {
					openBal += amount
					if openDate.IsZero() || date.Before(openDate) {
						openDate = date
					}
				} else if date.After(closeDate) {
					closeDate = date
				}
			}
			continue
		}
		code := strings.TrimSpace(rec[9])
		if kind != "20" || code == "AS" || code == "K2" || code == "LS" {
			continue // turnover and other technical rows
		}
		if derr != nil || aerr != nil || amount <= 0 {
			continue
		}
		payee := strings.TrimSpace(rec[3])
		details := strings.TrimSpace(rec[4])

		// Every row moves the account balance — including internal transfers
		// that are skipped as transactions. Flows use the POSTING date; the
		// account changed when the bank posted, not when the card was swiped.
		delta := amount
		if strings.TrimSpace(rec[7]) == "D" {
			delta = -amount
		}
		flows = append(flows, flow{date: date, delta: delta})

		// The LTL→EUR changeover pair (one debit in LTL, one credit in EUR,
		// both marked with the conversion rate) is a technical event, not a
		// transaction. Its converted flows cancel out above.
		if payee == "" && strings.Contains(details, "kursas") && strings.Contains(details, "LTL") {
			internal++
			continue
		}

		// Prefer the embedded purchase date over the posting date.
		if m := purchaseDateRe.FindStringSubmatch(details); m != nil {
			if d, e := time.Parse("2006-01-02", m[1]+"-"+m[2]+"-"+m[3]); e == nil {
				date = d
			}
		} else if m := cashOpDateRe.FindStringSubmatch(details); m != nil {
			// ATM rows carry DD.MM.YY
			if d, e := time.Parse("06-01-02", m[3]+"-"+m[2]+"-"+m[1]); e == nil {
				date = d
			}
		}

		tx, skip := classifySwedbank(date, payee, details, amount, strings.TrimSpace(rec[7]))
		if skip {
			internal++
			continue
		}
		txs = append(txs, tx)
	}
	// Month-end balance series: start from the bank-stated opening balance
	// and replay every flow. The last month must land on the bank-stated
	// closing balance, so the series is self-checking.
	if !openDate.IsZero() && !closeDate.IsZero() {
		sort.SliceStable(flows, func(i, j int) bool { return flows[i].date.Before(flows[j].date) })
		running := openBal
		i := 0
		for cur := time.Date(openDate.Year(), openDate.Month(), 1, 0, 0, 0, 0, time.UTC); cur.Before(closeDate); cur = cur.AddDate(0, 1, 0) {
			monthEnd := cur.AddDate(0, 1, -1)
			for i < len(flows) && !flows[i].date.After(monthEnd) {
				running += flows[i].delta
				i++
			}
			balances = append(balances, swedBalance{Date: monthEnd, Amount: math.Round(running*100) / 100})
		}
	}
	return txs, balances, internal, nil
}

// evelinaTransfer maps a transfer to the ex-wife by its stated purpose —
// leasing and house-loan instalments are not discretionary spending.
func evelinaTransfer(date time.Time, payee, details string, amount float64) swedTx {
	d := strings.ToLower(details)
	base := swedTx{Date: date, Amount: amount, Type: domain.TransactionTypeExpense, Debit: "swed"}
	switch {
	case strings.Contains(d, "lizing"):
		base.Type = domain.TransactionTypeInvestment
		base.Category = "Vehicle"
		base.Comment = payee + " (Lizingas)"
		base.Labels = "leasing,evelina"
	case strings.Contains(d, "paskol"):
		base.Category = "Finance"
		base.Comment = payee + " (Būsto paskola)"
		base.Labels = "loan,evelina"
	case strings.Contains(d, "vaik") || strings.Contains(d, "daržel") || strings.Contains(d, "kailiniams"):
		base.Category = "Kids - General"
		base.Comment = payee + " (" + details + ")"
		base.Labels = "evelina"
	case strings.Contains(d, "dovan"):
		base.Category = "Gifts"
		base.Comment = payee + " (" + details + ")"
		base.Labels = "evelina"
	case strings.Contains(d, "pramog"):
		base.Category = "Entertainment"
		base.Comment = payee + " (" + details + ")"
		base.Labels = "evelina"
	default:
		base.Category = "Finance"
		base.Comment = payee
		if details != "" {
			base.Comment = payee + " (" + details + ")"
		}
		base.Labels = "evelina"
	}
	return base
}

type merchantRule struct {
	patterns []string
	category domain.Category
}

// Ordered: first match wins. Patterns are matched against UPPER(payee+details).
var swedMerchantRules = []merchantRule{
	{[]string{"SKAITLIS", "VAIKYSTĖS STEBUKLAS"}, "Kids - Education"},
	{[]string{"BABY CITY", "BABYCIT", "TOY CITY", "KINDERLAND", "VAIKUTIS"}, "Kids - General"},
	{[]string{"GJENSIDIGE", "COMPENSA", "DRAUDIMO"}, "Finance"},
	{[]string{"IGNITIS", "TELIA", "ŠILUMOS TINKLAI", "VILNIAUS VANDENYS", "MANO BŪSTAS", "SAUGOS TARNYBA ARGUS", "FOXPAY", "BITĖ LIETUVA", "SAVIVALDYBES ADMINISTRACIJA", "SAVIVALDYBĖS ADMINISTRACIJA", "TEO LT", "RADIJO IR TELEVIZIJOS", "VILNIAUS ENERGIJA", "DUJŲ TIEKIMAS", "DUJU TIEKIMAS", "ENERGIJOS TIEKIMAS", "ENERGIJOS SKIRSTYMO", "ŽIRMŪNŲ BŪSTAS", "ZIRMUNU BUSTAS", "RINKLIAVA"}, "Utilities"},
	{[]string{"VAISTINE", "VAISTINĖ", "BENU ", "CAMELIA", "NORTHWAY", "POLIK", "KARDIOLITA", "SANIDENTAS", "AKUSERIJOS", "VEZIO INS", "AUREUS PORTUS", "HIPERFARMA", "ŠARĖJIENĖ", "TREATWELL", "SVEIKATINE", "MIGRACIJOS DEP", "SPA VILNIUS", "GRAND SPA", "GRANDSPA", "MASAZO", "MASAŽO", "DZENTELMENU", "BIOMED", "DORIS GROUP"}, "Health"},
	{[]string{"NESTE", "CIRCLE K", "VIADA", "TOKVILA", "TUVLITA", "TUV NORD", "CITYBEE", "EGAS.EU", "PLOVYKLA", "SVAROSBROLIAI", "DVIRACIU ARENA", "UNIPARK", "DAGRIS", "BOLT.EU", "VILOKTA", "AUTOLAB", "AUTOSERVIS", "CARISTA", "DEALS ON WHEELS", "SUSISIEKIMO PASLAUGOS", "STATOIL", "LUKOIL", "ORLEN"}, "Transport"},
	{[]string{"LIDL", "MAXIMA", "NORFA -", "RIMI", "IKI PILAITE", "IKI GEDIMINO", "IKI EXPRESS", "IKI SESKINE", "IKI KARALIAUCIAUS", "IKI EGLUTE", "PARDUOTUVE AIBE", "SKULAS", "PREKYBOS TASKAS", "NAUSEDZIU", "NATURALIA", "ZALIA STOTELE", "IKI PLYTINES", "IKI BAJORU", "IKI MINSKAS", "CENTO VYTURELIS"}, "Food"},
	{[]string{"WOLT", "MCDONALD", "PIZZA", "JAMMI", "KAVINE", "DRAKONAS", "DRAKONAI", "OLIVE KITCHEN", "PREZO", "KEPYKLELE", "CRUSTUM", "GREY ", "SEFO", "MESOS BROLIAI", "SUSHI", "KIBIN", "PASLEPTI RECEPTAI", "VILLA ALICANTE", "CASA DELLA PASTA", "TOGRI", "MANHATTAN LEDAI", "ZVERYNO", "BELMONTAS", "HUNGRY MOOSE", "OLEARYS", "APL RESTAURANTS", "SEGIRA", "GREITAS MAISTAS", "LIA PASTA", "KIBIM", "BLYNINE", "DARZOVES"}, "Food"},
	{[]string{"BUKOWSKI", "LOCAL PUB", "LA BIRRA", "WINE 12", "APOTEKA BARAS", `"VILNIAUS" BARAS`, "BILIARDO", "BOULINGAS", "FUKSAS"}, "Entertainment"},
	{[]string{"YOUTUBEPREMIUM", "CONTRIBEE", "GOOGLE STORAGE", "DELFI", "INTERNETO VIZIJA", "VERSLO ŽINIOS", "VERSLO ZINIOS", "MSFT"}, "Subscriptions"},
	{[]string{"GERA DOVANA", "STAIGMENU SALA", "STAIGMENŲ SALA", "GELIU", "GELES", "GĖLĖS", "GELESNAMO", "GINTARESGELES", "BRILLANTE", "BALČIŪNAITĖ"}, "Gifts"},
	{[]string{"DEICHMANN", "PEPCO", "NORTHLAND", "ŠILKO ŠVARA"}, "Clothing"},
	{[]string{"PIGU", "VARLE", "ELECTRONIC TRADE", "MK TRADE", "MOBKEISA", "LATERCASE", "SAMSUNG", "LEMONA", "TOPO CENTRAS", "TOPO GRUP", "KILOBAITAS", "BETA MEDIA", "BETA.LT", "MARKIT"}, "Entertainment"},
	{[]string{"XBOX", "UBISOFT", "GOOGLE PLAY", "GOOGLE PAYMENT", "PEGASAS", "RIAUBA", "SILAS PC", "VILMIRA", "PIER ", "VERSLO CENTRAS", "GOBOX", "SIRVINTU SPORTO", "ZIRGYNAS", "ALBAS"}, "Entertainment"},
	{[]string{"MOKI VEZI", "MOKI-VEZI", "KESKO", "SENUKAI", "DEPO ", "IKEA", "JYSK", "FLUGGER", "TECHNORENTA", "EX TOTO", "JRNR STATYBA", "SISTEMAX", "VANDENVALA", "DEXTERA", "TECHNICA SERVICE", "REKUPERATORI", "SEPTIKAS", "SIURBLIAI", "SIOS NORDIC", "REOLINK", "BAUSA", "SEISUK", "VIDAXL", "MANOROBOTAS", "KITCHENFORCE", "TROBOS", "BALDAI", "FURNITANAS", "OSMO", "VEDRANA", "BONIDECO", "BORVUS", "PROMOSTAR", "COMFOPAGALVES", "ULMAS", "GINORIS", "ELPAMA", "ARUODAS", "SKELBIU", "BIJOLA", "LPEXPRESS", "AERACIJA", "VILDIKA", "KRINONA", "DEZEMIS", "SAULĖTEKIO BŪSTAS", "SAULETEKIO BUSTAS"}, "Housing"},
	{[]string{"ALIEXPRESS", "ALIPAY"}, "Entertainment"},
	{[]string{"REGISTRŲ CENTRAS", "NOTA ", "NOTAR", "MOKESČIŲ INSPEKCIJA", "DIGINET", "GEDIMINO TECHNIKOS UNIVERSI"}, "Finance"},
	{[]string{"TRAVEL VALUE", "DUTY FREE", "AIRBNB", "VIESBUTIS", "BOOKING.COM"}, "Vacation"},
}

// Wolt bills from Helsinki year-round, so Helsinki is deliberately absent.
var vacationCityTokens = []string{"KOEBENHAVN", "KOBENHAVN", "OSLO", "GARDERMOEN", "LONDON", "WARSZAWA", "ANTALYA", "VEJLE", "BIRMINGHAM"}

// canonicalMerchant mirrors the startup comment-cleanup migration for retail
// chains, so statement re-imports dedup against already-canonicalised rows
// instead of re-adding the raw bank strings.
func canonicalMerchant(up, payee string) string {
	switch {
	case strings.Contains(up, "MOKI VEZI") || strings.Contains(up, "MOKI-VEZI"):
		return "Moki Veži"
	case strings.Contains(up, "KESKO"):
		return "Kesko Senukai"
	case strings.HasPrefix(strings.ToUpper(payee), "DEPO "):
		return "Depo"
	case strings.Contains(strings.ToUpper(payee), "UAB PIGU") || strings.Contains(up, `UAB "PIGU"`):
		return "Pigu.lt"
	case strings.EqualFold(payee, "Varle UAB"):
		return "Varlė.lt"
	}
	return payee
}

func classifySwedbank(date time.Time, payee, details string, amount float64, dk string) (swedTx, bool) {
	up := strings.ToUpper(payee + " " + details)
	lowDetails := strings.ToLower(details)

	// Transfers under the user's own name: loan repayments are real expenses,
	// everything between own accounts (incl. the Taupyklė micro-savings
	// sweeps) is internal. "To Revolut" falls through to the Revolut rule.
	upPayee := strings.ToUpper(payee)
	if (strings.Contains(upPayee, "MINDAUGAS") || strings.Contains(upPayee, "BALCIUNAS") || strings.Contains(upPayee, "BALČIŪNAS")) &&
		!strings.Contains(upPayee, "EVELINA") {
		switch {
		case strings.Contains(lowDetails, "credit repayment") || strings.Contains(lowDetails, "credit card repayment") || strings.Contains(lowDetails, "kredito padengim"):
			return swedTx{Date: date, Amount: amount, Type: domain.TransactionTypeExpense,
				Category: "Finance", Comment: "Credit repayment", Labels: "loan", Debit: "swed"}, false
		case strings.Contains(lowDetails, "revolut"):
			// handled by the Revolut top-up rule below
		default:
			return swedTx{}, true // own-account movement (tarp savo sąskaitų, taupyklė, …)
		}
	}

	// ---- credits ----
	if dk == "K" {
		base := swedTx{Date: date, Amount: amount, Type: domain.TransactionTypeIncome, Credit: "swed"}
		switch {
		case strings.Contains(up, "TRANSFER BETWEEN") || strings.Contains(lowDetails, "transfer between"):
			return swedTx{}, true // own-account movement, not income
		case payee == "" && (strings.Contains(up, "INESIMAS") || strings.Contains(up, "ĮNEŠIMAS")):
			// Cash deposited at an ATM — money moving from the cash pocket.
			return swedTx{Date: date, Amount: amount, Type: domain.TransactionTypeInvestment,
				Category: "Finance", Comment: "Cash deposit (ATM)", Debit: "cash", Credit: "swed"}, false
		case strings.HasPrefix(strings.ToUpper(details), "GRĄŽIN"): // GRĄŽINIMAS / GRĄŽINAMAS
			base.Category = "Reimbursement"
			base.Comment = strings.TrimSpace("Card refund: " + payee)
			if payee == "" {
				base.Comment = "Card refund (Swedbank)"
			}
		case strings.Contains(up, "MOBILEPAY") || strings.Contains(up, "VIPPS"):
			base.Category = "Salary"
			base.Comment = payee
		case strings.Contains(up, "DANSKE"):
			base.Category = "Salary"
			base.Comment = payee
			base.Labels = "danske"
		case strings.Contains(up, "BARCLAYS"):
			base.Category = "Salary"
			base.Comment = payee
			base.Labels = "barclays"
		case strings.Contains(up, "DOCLOGIX") || strings.Contains(up, "DOCLOGIC"):
			base.Category = "Salary"
			base.Comment = payee
			base.Labels = "doclogix"
		case strings.Contains(up, "APP CAMP"):
			base.Category = "Salary"
			base.Comment = payee
			base.Labels = "app camp"
		case strings.Contains(up, "UAB ALNA") || strings.Contains(up, "ALNA BUSINESS") || strings.Contains(up, "ALNA SOFTWARE"):
			base.Category = "Salary"
			base.Comment = payee
			base.Labels = "alna"
		case strings.Contains(up, "IŠMOKA") && strings.Contains(up, "VAIK"):
			base.Category = "Reimbursement"
			base.Comment = "Child benefit (išmoka vaikui)"
		case strings.Contains(up, "LEVENT SOPHIE"):
			base.Category = "Freelance"
			base.Comment = "Rent - Levent Sophie"
		case strings.Contains(up, "FAKROGHA") || strings.Contains(up, "PREYE"):
			base.Category = "Freelance"
			base.Comment = "Rent - Preye Benjamin Fakrogha"
		case strings.Contains(up, "NWABUDE"):
			base.Category = "Freelance"
			base.Comment = "Rent - Lynda Ifeoma Nwabude"
		case strings.Contains(up, "VSDF") || strings.Contains(up, "SODROS") || strings.Contains(up, "SODRA"):
			base.Category = "Reimbursement"
			base.Comment = "SoDra išmoka"
		case strings.Contains(up, "EVELINA") || strings.Contains(up, "BALČIŪNIEN"):
			base.Category = "Reimbursement"
			base.Comment = payee
			base.Labels = "evelina"
		default:
			base.Category = "Reimbursement"
			base.Comment = canonicalMerchant(up, payee)
			if base.Comment == "" {
				base.Comment = details
			}
		}
		return base, false
	}

	// ---- debits ----
	// Bank fees carry an empty payee.
	if payee == "" {
		switch {
		case strings.Contains(lowDetails, "gryniej"):
			return swedTx{Date: date, Amount: amount, Type: domain.TransactionTypeInvestment,
				Category: "Finance", Comment: "Cash withdrawal (ATM)", Debit: "swed", Credit: "cash"}, false
		case strings.Contains(lowDetails, "paslaugų plano") || strings.Contains(lowDetails, "paslaugu plano"):
			return swedTx{Date: date, Amount: amount, Type: domain.TransactionTypeExpense,
				Category: "Finance", Comment: "Swedbank plan fee", Labels: "fees", Debit: "swed"}, false
		default:
			return swedTx{Date: date, Amount: amount, Type: domain.TransactionTypeExpense,
				Category: "Finance", Comment: "Swedbank card fee", Labels: "fees", Debit: "swed"}, false
		}
	}

	if strings.Contains(up, "EVELINA") || strings.Contains(up, "PLYTNIKAIT") || strings.Contains(up, "BALČIŪNIEN") {
		return evelinaTransfer(date, payee, details, amount), false
	}
	// RAV4 down payments at Tokvila are car-purchase capital, not servicing.
	if strings.Contains(up, "TOKVILA") && (strings.Contains(lowDetails, "automobil") || strings.Contains(lowDetails, "pradin")) {
		return swedTx{Date: date, Amount: amount, Type: domain.TransactionTypeInvestment,
			Category: "Vehicle", Comment: "RAV4 pradinė įmoka (Tokvila)", Debit: "swed"}, false
	}
	if strings.Contains(up, "KUGINYS") || strings.Contains(up, "KUGINIEN") {
		return swedTx{Date: date, Amount: amount, Type: domain.TransactionTypeInvestment,
			Category: "Real Estate", Comment: "House purchase — Platiniškių 21A (NT sandoris, " + payee + ")",
			Debit: "swed"}, false
	}
	// Revolut card top-ups are money moved to another own account.
	if strings.Contains(up, "REVOLUT") {
		return swedTx{Date: date, Amount: amount, Type: domain.TransactionTypeInvestment,
			Category: "Finance", Comment: "Revolut top up", Debit: "swed", Credit: "rev_m"}, false
	}

	// Trips: foreign-currency card purchases (except online USD shops).
	if fxCurrencyRe.MatchString(details) {
		return swedTx{Date: date, Amount: amount, Type: domain.TransactionTypeExpense,
			Category: "Vacation", Comment: payee, Debit: "swed"}, false
	}
	for _, city := range vacationCityTokens {
		if strings.Contains(up, city) && !strings.Contains(up, "WOLT") {
			return swedTx{Date: date, Amount: amount, Type: domain.TransactionTypeExpense,
				Category: "Vacation", Comment: payee, Debit: "swed"}, false
		}
	}

	for _, rule := range swedMerchantRules {
		for _, p := range rule.patterns {
			if strings.Contains(up, p) {
				return swedTx{Date: date, Amount: amount, Type: domain.TransactionTypeExpense,
					Category: rule.category, Comment: canonicalMerchant(up, payee), Debit: "swed"}, false
			}
		}
	}

	// Person-to-person transfers with a gift-ish note.
	if strings.Contains(up, "BDAY") || strings.Contains(up, "BIRTHDAY") || strings.Contains(up, "GIMTADIEN") || strings.Contains(up, "DOVAN") {
		return swedTx{Date: date, Amount: amount, Type: domain.TransactionTypeExpense,
			Category: "Gifts", Comment: payee + " (" + details + ")", Debit: "swed"}, false
	}

	return swedTx{Date: date, Amount: amount, Type: domain.TransactionTypeExpense,
		Category: "Entertainment", Comment: payee, Debit: "swed"}, false
}
