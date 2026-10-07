// Package bank pulls transactions from banks over PSD2 (Enable Banking),
// proposes how to file them, and holds them in an inbox until the owner
// accepts them into the ledger.
package bank

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"ft/internal/bank/openbanking"
	"ft/internal/merchant"
	"ft/internal/money"
)

// Proposal is one bank row read into ledger terms.
type Proposal struct {
	ExternalID  string
	Raw         string
	RawPayee    string
	RawDetails  string
	RawCurrency string
	BookingDate string
	Pending     bool

	Date        string
	Kind        string
	Amount      money.Cents
	AccountID   string
	ToAccountID string
	Category    string
	Merchant    string
	Note        string
	Guessed     bool // category is a placeholder for the rules/history to replace

	Verdict     string // new | internal | needs_review | pending
	VerdictNote string
	Direction   string // in | out
	CounterIBAN string
}

// Context is what the classifier knows about the owner.
type Context struct {
	OwnerNames []string          // payee names that mean "me" (own-account transfers)
	OwnIBANs   map[string]string // IBAN → ledger account id of own linked accounts
}

// ExternalID is the upsert and dedup key. The format is unchanged from v1 so
// rows imported before the rewrite are still recognised.
func ExternalID(tx openbanking.Transaction, linkID int64) string {
	ref := strings.TrimSpace(tx.EntryReference)
	if ref == "" {
		ref = strings.TrimSpace(tx.TransactionID)
	}
	if ref == "" {
		sum := sha256.Sum256([]byte(strings.Join([]string{
			tx.BookingDate, tx.ValueDate, tx.TransactionAmount.Amount,
			tx.TransactionAmount.Currency, tx.CreditDebitIndicator,
			tx.Creditor.Name, tx.Debtor.Name,
			strings.Join(tx.RemittanceInformation, " "),
		}, "|")))
		ref = "h" + hex.EncodeToString(sum[:12])
	}
	return fmt.Sprintf("eb:%d:%s", linkID, ref)
}

// Adapt reads one provider row for the ledger account the link feeds.
func Adapt(tx openbanking.Transaction, linkID int64, accountID string, ctx Context) Proposal {
	details := narrative(tx)
	in := strings.EqualFold(strings.TrimSpace(tx.CreditDebitIndicator), openbanking.IndicatorCredit)
	payee, iban := counterparty(tx, details, in)
	amount, amtErr := strconv.ParseFloat(strings.TrimSpace(tx.TransactionAmount.Amount), 64)
	if amount < 0 {
		amount = -amount
	}
	currency := strings.ToUpper(strings.TrimSpace(tx.TransactionAmount.Currency))
	raw, _ := json.Marshal(tx)
	p := Proposal{
		ExternalID: ExternalID(tx, linkID), Raw: string(raw), RawPayee: payee, RawDetails: details, RawCurrency: currency,
		BookingDate: isoDate(tx.BookingDate), Pending: strings.EqualFold(strings.TrimSpace(tx.Status), openbanking.StatusPending),
		Amount: money.FromFloat(amount), CounterIBAN: iban, Verdict: "new",
	}
	p.Date = purchaseDate(tx, details)
	p.Direction = "out"
	if in {
		p.Direction = "in"
	}
	dir := strings.ToUpper(strings.TrimSpace(tx.CreditDebitIndicator))
	switch {
	case dir != openbanking.IndicatorCredit && dir != openbanking.IndicatorDebit:
		p.Verdict, p.VerdictNote = "needs_review", fmt.Sprintf("unrecognised direction %q — set it by hand", tx.CreditDebitIndicator)
	case amtErr != nil || amount <= 0:
		p.Verdict, p.VerdictNote = "needs_review", fmt.Sprintf("could not read the amount %q", tx.TransactionAmount.Amount)
		p.Amount = 1
	case currency != "" && currency != "EUR":
		p.Verdict, p.VerdictNote = "needs_review", currency+" transaction — enter the euro amount by hand"
	}
	classify(&p, payee, details, accountID, ctx)
	if p.Pending && p.Verdict != "needs_review" {
		p.Verdict, p.VerdictNote = "pending", "reserved by the bank, not booked yet — the amount can still change"
	}
	return p
}

// narrative is the bank's free text: remittance information, else the bank
// transaction code description, never both concatenated.
func narrative(tx openbanking.Transaction) string {
	var parts []string
	for _, s := range tx.RemittanceInformation {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	if s := strings.TrimSpace(tx.BankTransactionCode.Description); s != "" {
		return s
	}
	return strings.TrimSpace(tx.Note)
}

// cardNarrativeRe recovers the merchant from a card purchase, which arrives
// over PSD2 with no counterparty at all:
//
//	PIRKINYS 516793******2669 02.10.26 17:34 65.20 EUR (134851) MAXIMA/X-787 MAXIMA Vilnius 000LT
var cardNarrativeRe = regexp.MustCompile(`(?i)^\s*(?:PIRKINYS|GR[ĄA]ŽINIMAS|GRAZINIMAS)\b.*\)\s*(\S.*)$`)

func cardMerchant(details string) string {
	m := cardNarrativeRe.FindStringSubmatch(strings.TrimSpace(details))
	if m == nil {
		return ""
	}
	tail := strings.TrimSpace(m[1])
	if i := strings.Index(tail, "/"); i > 0 {
		tail = strings.TrimSpace(tail[:i])
	}
	letters := 0
	for _, r := range tail {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	if letters < 2 {
		return ""
	}
	return tail
}

func counterparty(tx openbanking.Transaction, details string, in bool) (string, string) {
	name, iban := tx.Creditor.Name, tx.CreditorAccount.IBAN
	if in {
		name, iban = tx.Debtor.Name, tx.DebtorAccount.IBAN
	}
	iban = strings.ReplaceAll(strings.TrimSpace(iban), " ", "")
	if n := strings.TrimSpace(name); n != "" {
		return n, iban
	}
	if m := cardMerchant(details); m != "" {
		return m, iban
	}
	return "", iban
}

var (
	statementDateRe = regexp.MustCompile(`(?:PIRKINYS|GRĄŽINIMAS)\s+\d{6}\*+\d{4}\s+(\d{4})\.(\d{2})\.(\d{2})`)
	psd2CardDateRe  = regexp.MustCompile(`(?i)(?:PIRKINYS|GR[ĄA]ŽINIMAS|GRAZINIMAS)\s+[\d*]+\s+(\d{2})\.(\d{2})\.(\d{2})\b`)
	cashOpDateRe    = regexp.MustCompile(`(?:GRYNIEJI|INESIMAS|ĮNEŠIMAS)\s+\d[\d*]+\s+(\d{2})\.(\d{2})\.(\d{2})`)
	fxCurrencyRe    = regexp.MustCompile(`\b(DKK|NOK|SEK|GBP|TRY|PLN|CZK|HUF|CHF|USD)\b`)
)

func isoDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		if _, err := time.Parse("2006-01-02", s[:10]); err == nil {
			return s[:10]
		}
	}
	return ""
}

// purchaseDate prefers the date the card was used (embedded in the
// narrative) over the booking date — card rows often carry no booking date
// at all over PSD2.
func purchaseDate(tx openbanking.Transaction, details string) string {
	if m := statementDateRe.FindStringSubmatch(details); m != nil {
		return m[1] + "-" + m[2] + "-" + m[3]
	}
	for _, re := range []*regexp.Regexp{psd2CardDateRe, cashOpDateRe} {
		if m := re.FindStringSubmatch(details); m != nil {
			if d, err := time.Parse("06-01-02", m[3]+"-"+m[2]+"-"+m[1]); err == nil {
				return d.Format("2006-01-02")
			}
		}
	}
	for _, s := range []string{tx.TransactionDate, tx.BookingDate, tx.ValueDate} {
		if d := isoDate(s); d != "" {
			return d
		}
	}
	return time.Now().Format("2006-01-02")
}

func has(s string, subs ...string) bool {
	f := merchant.Fold(s)
	for _, x := range subs {
		if strings.Contains(f, merchant.Fold(x)) {
			return true
		}
	}
	return false
}

// Narrative noise that is not a human-written purpose.
var noisePrefixes = []string{"PIRKINYS", "GRYNIEJI", "GRĄŽIN", "GRAZIN", "INESIMAS", "ĮNEŠIMAS", "TMP", "#", "E-SĄSKAITA", "E.SĄSKAITOS",
	"E-SASKAITA", "SĄSKAITA MB", "SASKAITA MB", "PSD2", "NIPS", "LB PAPILDYTA", "MOKESTIS UŽ", "MOKESTIS UZ", "EAP-FN", "ORDER ID", "ORDER NO",
	"UZSAKYMO", "UŽSAKYMO", "PAYMENT FOR ORDER", "WWW.", "MOKĖJIMAS MOBILI", "MOKEJIMAS MOBILI", "P.P.MOK"}

// purpose returns the narrative when it reads like something a person wrote
// ("tvoros statyba", "Leonardas būrelis"), else "".
func purpose(payee, details string) string {
	d := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(details), "'"))
	if d == "" || strings.EqualFold(d, payee) {
		return ""
	}
	up := strings.ToUpper(d)
	for _, p := range noisePrefixes {
		if strings.HasPrefix(up, p) {
			return ""
		}
	}
	low := strings.ToLower(d)
	if low == "transfer" || low == "pervedimas" || len([]rune(d)) > 60 {
		return ""
	}
	letters, digits := 0, 0
	for _, r := range d {
		switch {
		case unicode.IsLetter(r):
			letters++
		case unicode.IsDigit(r):
			digits++
		}
	}
	if letters < 4 || letters <= digits {
		return ""
	}
	return d
}

func (p *Proposal) set(kind, category, merchantName, note string) {
	p.Kind, p.Category, p.Merchant, p.Note = kind, category, merchantName, note
}

// classify proposes kind/category/accounts. Everything not recognised here
// is left Guessed for the rules engine and the ledger's history.
func classify(p *Proposal, payee, details, accountID string, ctx Context) {
	up := merchant.Fold(payee + " " + details)
	low := strings.ToLower(details)
	note := payee
	if pp := purpose(payee, details); pp != "" {
		note = strings.TrimSpace(payee + " (" + pp + ")")
	}
	if note == "" {
		note = details
	}
	m := merchant.Resolve(payee)
	if m == "" && payee != "" {
		m = merchant.Clean(payee)
	}
	in := p.Direction == "in"
	if in {
		p.AccountID = accountID
	} else {
		p.AccountID = accountID
	}
	own := false
	for _, n := range ctx.OwnerNames {
		if n != "" && strings.Contains(merchant.Fold(payee), merchant.Fold(n)) {
			own = true
		}
	}
	ownAcct := ctx.OwnIBANs[p.CounterIBAN]
	if ownAcct != "" {
		own = true
	}

	if p.Verdict == "needs_review" {
		p.Guessed = true
		if in {
			p.set("income", "refunds", m, note)
		} else {
			p.set("expense", "other", m, note)
		}
		return
	}

	switch {
	case own && (has(low, "credit repayment", "credit card repayment", "kredito padengim")):
		p.set("transfer", "transfer.internal", "", "Credit card repayment")
		return
	case own && has(low, "revolut"):
		// falls through to the Revolut rule below
	case own:
		p.set("transfer", "transfer.internal", "", firstNonEmpty(purpose(payee, details), "Transfer between own accounts"))
		p.Verdict, p.VerdictNote = "internal", "a movement between your own accounts"
		if in {
			p.AccountID, p.ToAccountID = ownAcct, accountID
		} else {
			p.ToAccountID = ownAcct
		}
		return
	}

	if in {
		switch {
		case has(up, "TRANSFER BETWEEN"):
			p.set("transfer", "transfer.internal", "", "Transfer between own accounts")
			p.AccountID, p.ToAccountID = "", accountID
			p.Verdict, p.VerdictNote = "internal", "a movement between your own accounts"
		case payee == "" && has(up, "INESIMAS", "ĮNEŠIMAS"):
			p.set("transfer", "transfer.internal", "", "Cash deposit (ATM)")
			p.AccountID, p.ToAccountID = "cash", accountID
		case has(details, "GRĄŽIN", "GRAZIN"):
			p.set("income", "refunds", m, strings.TrimSpace("Card refund: "+payee))
		case has(up, "VAIK") && has(up, "IŠMOKA", "ISMOKA"), has(up, "SODRA", "VSDF", "SODROS"):
			p.set("income", "benefits", firstNonEmpty(m, "Sodra"), note)
		case has(low, "atlyginim", "darbo užmok", "darbo uzmok", "salary", "wages", "payroll"):
			p.set("income", "salary", m, note)
		case has(low, "dividend"):
			p.set("income", "investment_income", m, note)
		case p.Amount >= 25000:
			// Too big to guess as a refund (a refund lowers spending): money
			// back from savings or a sale is more likely. Review decides.
			p.set("income", "other_income", m, note)
			p.Guessed = true
		default:
			p.set("income", "refunds", m, note)
			p.Guessed = true
		}
		return
	}

	if payee == "" {
		switch {
		case has(low, "gryniej"):
			p.set("transfer", "transfer.internal", "", "Cash withdrawal (ATM)")
			p.ToAccountID = "cash"
		case has(low, "robur", "mini investicij"):
			p.set("transfer", "transfer.invest", "Swedbank Robur", "Swedbank Robur fund purchase")
			p.ToAccountID = "swed_etf"
		case has(low, "paslaugų plano", "paslaugu plano"):
			p.set("expense", "finance.bank_fees", "Swedbank", "Swedbank plan fee")
		case has(low, "palūkan", "palukan"):
			p.set("expense", "housing.mortgage_interest", "", firstNonEmpty(details, "Loan interest"))
		case has(low, "paskolos grąžin", "paskolos grazin", "kredito grąžin", "loan return", "paskolos dalies"):
			p.set("transfer", "transfer.debt", "", firstNonEmpty(details, "Loan return"))
			p.ToAccountID = "mortgage"
		default:
			p.set("expense", "finance.bank_fees", "", firstNonEmpty(details, "Bank fee"))
		}
		return
	}

	switch {
	case has(up, "REVOLUT"):
		p.set("transfer", "transfer.internal", "Revolut", "Revolut top up")
		p.ToAccountID = "revolut"
	case has(up, "INTERACTIVE BROKERS", "IBKR"):
		p.set("transfer", "transfer.invest", "IBKR", note)
		p.ToAccountID = "ibkr"
	case has(up, "ARTEA", "INVL"):
		p.set("transfer", "transfer.pension", "Artea", note)
		p.ToAccountID = "artea"
	case has(low, "aliment", "alimon"):
		p.set("expense", "kids.alimony", m, note)
	case has(low, "palūkan", "palukan") && has(up, "SEB", "PASKOL", "LOAN"):
		p.set("expense", "housing.mortgage_interest", m, note)
	case has(low, "paskolos grąžin", "paskolos grazin", "loan return"):
		p.set("transfer", "transfer.debt", m, note)
		p.ToAccountID = "mortgage"
	case fxCurrencyRe.MatchString(details) && !has(up, "ALIEXPRESS", "AMAZON", "GOOGLE", "APPLE", "PATREON", "ANTHROPIC", "CLAUDE", "OPENAI", "MICROSOFT", "MSFT"):
		p.set("expense", "travel.trip", m, note)
	default:
		p.set("expense", "other", m, note)
		p.Guessed = true
	}
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
