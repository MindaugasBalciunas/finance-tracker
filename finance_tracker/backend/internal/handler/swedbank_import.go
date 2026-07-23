package handler

import (
	"encoding/csv"
	"io"
	"regexp"
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
var withdrawalDateRe = regexp.MustCompile(`GRYNIEJI\s+\d{6}\*+\d{4}\s+(\d{2})\.(\d{2})\.(\d{2})`)
var fxCurrencyRe = regexp.MustCompile(`\b(DKK|NOK|SEK|GBP|TRY|PLN|CZK|HUF|CHF)\b`)

// parseSwedbankCSV turns a statement into classified transactions plus a
// count of skipped internal own-account movements.
func parseSwedbankCSV(r io.Reader) (txs []swedTx, internal int, err error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	first := true
	for {
		rec, rerr := reader.Read()
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, 0, rerr
		}
		if first { // header
			first = false
			continue
		}
		if len(rec) < 10 {
			continue
		}
		kind, code := strings.TrimSpace(rec[1]), strings.TrimSpace(rec[9])
		if kind != "20" || code == "AS" || code == "K2" || code == "LS" {
			continue // opening/closing/turnover rows
		}
		date, derr := time.Parse("2006-01-02", strings.TrimSpace(rec[2]))
		if derr != nil {
			continue
		}
		amount, aerr := strconv.ParseFloat(strings.TrimSpace(rec[5]), 64)
		if aerr != nil || amount <= 0 {
			continue
		}
		payee := strings.TrimSpace(rec[3])
		details := strings.TrimSpace(rec[4])

		// Prefer the embedded purchase date over the posting date.
		if m := purchaseDateRe.FindStringSubmatch(details); m != nil {
			if d, e := time.Parse("2006-01-02", m[1]+"-"+m[2]+"-"+m[3]); e == nil {
				date = d
			}
		} else if m := withdrawalDateRe.FindStringSubmatch(details); m != nil {
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
	return txs, internal, nil
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
	{[]string{"BABY CITY", "BABYCIT", "TOY CITY"}, "Kids - General"},
	{[]string{"GJENSIDIGE", "COMPENSA", "DRAUDIMO"}, "Finance"},
	{[]string{"IGNITIS", "TELIA", "ŠILUMOS TINKLAI", "VILNIAUS VANDENYS", "MANO BŪSTAS", "SAUGOS TARNYBA ARGUS", "FOXPAY", "BITĖ LIETUVA", "SAVIVALDYBES ADMINISTRACIJA", "SAVIVALDYBĖS ADMINISTRACIJA"}, "Utilities"},
	{[]string{"VAISTINE", "VAISTINĖ", "BENU ", "CAMELIA", "NORTHWAY", "POLIK", "KARDIOLITA", "SANIDENTAS", "AKUSERIJOS", "VEZIO INS", "AUREUS PORTUS", "HIPERFARMA", "ŠARĖJIENĖ", "TREATWELL", "SVEIKATINE", "MIGRACIJOS DEP"}, "Health"},
	{[]string{"NESTE", "CIRCLE K", "VIADA", "TOKVILA", "TUVLITA", "TUV NORD", "CITYBEE", "EGAS.EU", "PLOVYKLA", "SVAROSBROLIAI", "DVIRACIU ARENA", "UNIPARK", "DAGRIS", "BOLT.EU"}, "Transport"},
	{[]string{"LIDL", "MAXIMA", "NORFA -", "RIMI", "IKI PILAITE", "IKI GEDIMINO", "IKI EXPRESS", "IKI SESKINE", "IKI KARALIAUCIAUS", "IKI EGLUTE", "PARDUOTUVE AIBE", "SKULAS", "PREKYBOS TASKAS", "NAUSEDZIU", "NATURALIA", "ZALIA STOTELE"}, "Food"},
	{[]string{"WOLT", "MCDONALD", "PIZZA", "JAMMI", "KAVINE", "DRAKONAS", "DRAKONAI", "OLIVE KITCHEN", "PREZO", "KEPYKLELE", "CRUSTUM", "GREY ", "SEFO", "MESOS BROLIAI", "SUSHI", "KIBIN", "PASLEPTI RECEPTAI", "VILLA ALICANTE", "CASA DELLA PASTA", "TOGRI", "MANHATTAN LEDAI", "ZVERYNO", "BELMONTAS", "HUNGRY MOOSE", "OLEARYS", "APL RESTAURANTS"}, "Food"},
	{[]string{"BUKOWSKI", "LOCAL PUB", "LA BIRRA", "WINE 12", "APOTEKA BARAS", `"VILNIAUS" BARAS`, "BILIARDO", "BOULINGAS", "FUKSAS"}, "Entertainment"},
	{[]string{"YOUTUBEPREMIUM", "CONTRIBEE", "GOOGLE STORAGE", "DELFI", "INTERNETO VIZIJA"}, "Subscriptions"},
	{[]string{"GERA DOVANA", "STAIGMENU SALA", "STAIGMENŲ SALA", "GELIU TURGUS", "GELES", "GĖLĖS", "GELESNAMO", "GINTARESGELES", "BRILLANTE", "BALČIŪNAITĖ"}, "Gifts"},
	{[]string{"DEICHMANN", "PEPCO", "NORTHLAND", "ŠILKO ŠVARA"}, "Clothing"},
	{[]string{"PIGU", "VARLE", "ELECTRONIC TRADE", "MK TRADE", "MOBKEISA", "LATERCASE", "SAMSUNG", "LEMONA"}, "Entertainment"},
	{[]string{"XBOX", "UBISOFT", "GOOGLE PLAY", "GOOGLE PAYMENT", "PEGASAS", "RIAUBA", "SILAS PC", "VILMIRA", "PIER ", "VERSLO CENTRAS", "GOBOX", "SIRVINTU SPORTO", "ZIRGYNAS", "ALBAS"}, "Entertainment"},
	{[]string{"MOKI VEZI", "MOKI-VEZI", "KESKO", "SENUKAI", "DEPO ", "IKEA", "JYSK", "FLUGGER", "TECHNORENTA", "EX TOTO", "JRNR STATYBA", "SISTEMAX", "VANDENVALA", "DEXTERA", "TECHNICA SERVICE", "REKUPERATORI", "SEPTIKAS", "SIURBLIAI", "SIOS NORDIC", "REOLINK", "BAUSA", "SEISUK", "VIDAXL", "MANOROBOTAS", "KITCHENFORCE", "TROBOS", "BALDAI", "FURNITANAS", "OSMO", "VEDRANA", "BONIDECO", "BORVUS", "PROMOSTAR", "COMFOPAGALVES", "ULMAS", "GINORIS", "ELPAMA", "ARUODAS", "SKELBIU", "BIJOLA", "LPEXPRESS"}, "Housing"},
	{[]string{"ALIEXPRESS", "ALIPAY"}, "Entertainment"},
	{[]string{"REGISTRŲ CENTRAS", "NOTA ", "NOTAR", "MOKESČIŲ INSPEKCIJA", "DIGINET"}, "Finance"},
	{[]string{"TRAVEL VALUE", "DUTY FREE"}, "Vacation"},
}

// Wolt bills from Helsinki year-round, so Helsinki is deliberately absent.
var vacationCityTokens = []string{"KOEBENHAVN", "KOBENHAVN", "OSLO", "GARDERMOEN", "LONDON", "WARSZAWA", "ANTALYA", "VEJLE", "BIRMINGHAM"}

func classifySwedbank(date time.Time, payee, details string, amount float64, dk string) (swedTx, bool) {
	up := strings.ToUpper(payee + " " + details)
	lowDetails := strings.ToLower(details)

	// ---- credits ----
	if dk == "K" {
		base := swedTx{Date: date, Amount: amount, Type: domain.TransactionTypeIncome, Credit: "swed"}
		switch {
		case strings.Contains(up, "TRANSFER BETWEEN") || strings.Contains(lowDetails, "transfer between"):
			return swedTx{}, true // own-account movement, not income
		case strings.HasPrefix(strings.ToUpper(details), "GRĄŽINIMAS"):
			base.Category = "Reimbursement"
			base.Comment = "Card refund: " + payee
		case strings.Contains(up, "MOBILEPAY") || strings.Contains(up, "VIPPS"):
			base.Category = "Salary"
			base.Comment = payee
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
			base.Comment = payee
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
	if strings.Contains(up, "MINDAUGAS BAL") && strings.Contains(lowDetails, "credit repayment") {
		return swedTx{Date: date, Amount: amount, Type: domain.TransactionTypeExpense,
			Category: "Finance", Comment: "Credit repayment", Labels: "loan", Debit: "swed"}, false
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
					Category: rule.category, Comment: payee, Debit: "swed"}, false
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
