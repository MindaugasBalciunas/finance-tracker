package bank

import (
	"strings"
	"testing"

	"ft/internal/bank/openbanking"
)

func row(ref, dk, amount, payee, details string) openbanking.Transaction {
	t := openbanking.Transaction{EntryReference: ref, CreditDebitIndicator: dk, Status: openbanking.StatusBooked, BookingDate: "2026-09-12",
		TransactionAmount: openbanking.Amount{Currency: "EUR", Amount: amount}, RemittanceInformation: []string{details}}
	if dk == openbanking.IndicatorCredit {
		t.Debtor.Name = payee
	} else {
		t.Creditor.Name = payee
	}
	return t
}

var ctx = Context{OwnerNames: []string{"MINDAUGAS BALCIUNAS"}, OwnIBANs: map[string]string{"LT000000000000000002": "seb"}}

// Captured from the first live sync (2026-10-03): a card purchase arrives
// with NO counterparty; the merchant lives in the narrative, and the date is
// DD.MM.YY.
func TestCardPurchaseFromNarrative(t *testing.T) {
	tx := row("r1", "DBIT", "65.20", "", "PIRKINYS 516793******2669 02.10.26 17:34 65.20 EUR (134851) MAXIMA/X-787 MAXIMA Vilnius 000LT")
	tx.BookingDate = ""
	p := Adapt(tx, 3, "swed", ctx)
	if p.RawPayee != "MAXIMA" || p.Merchant != "Maxima" || p.Date != "2026-10-02" {
		t.Fatalf("%+v", p)
	}
	if p.Kind != "expense" || p.AccountID != "swed" || p.Verdict != "new" || !p.Guessed {
		t.Fatalf("an unrecognised purchase is a guess for the rules to fill: %+v", p)
	}
	if p.ExternalID != "eb:3:r1" {
		t.Error("external id format must stay v1-compatible", p.ExternalID)
	}
}

func TestStatementDateFormatPreferred(t *testing.T) {
	tx := row("r", "DBIT", "15.15", "Wolt 00180 Helsinki", "PIRKINYS 516793******2950 2022.01.04 15.15 EUR (522658) Wolt 00180 Helsinki")
	tx.TransactionDate = "2022-01-05" // provider disagrees with the narrative
	if p := Adapt(tx, 1, "swed", ctx); p.Date != "2022-01-04" {
		t.Error(p.Date)
	}
	plain := row("r", "DBIT", "5", "Shop", "plain text")
	plain.BookingDate, plain.TransactionDate = "2026-09-12", "2026-09-11"
	if p := Adapt(plain, 1, "swed", ctx); p.Date != "2026-09-11" {
		t.Error("transaction_date beats booking_date", p.Date)
	}
}

func TestClassification(t *testing.T) {
	cases := []struct {
		name               string
		tx                 openbanking.Transaction
		kind, cat, to      string
		verdict, merchant  string
	}{
		{"ATM withdrawal", row("a", "DBIT", "600", "", "GRYNIEJI 516793******2950 26.08.22 09:22 600.00 EUR (465020) H836/HB LUKSIO"), "transfer", "transfer.internal", "cash", "new", ""},
		{"bank fee", row("b", "DBIT", "1.5", "", "Mokestis už kortelę"), "expense", "finance.bank_fees", "", "new", ""},
		{"plan fee", row("c", "DBIT", "3", "", "Paslaugų plano mokestis"), "expense", "finance.bank_fees", "", "new", "Swedbank"},
		{"robur", row("d", "DBIT", "50", "", "Mini investicijos SWEDBANK ROBUR"), "transfer", "transfer.invest", "swed_etf", "new", "Swedbank Robur"},
		{"own account", row("e", "DBIT", "1400", "MINDAUGAS BALCIUNAS", "tarp savo sąskaitų"), "transfer", "transfer.internal", "", "internal", ""},
		{"credit card repayment", row("f", "DBIT", "300", "Mindaugas Balciunas", "Credit card repayment"), "transfer", "transfer.internal", "", "new", ""},
		{"revolut top up", row("g", "DBIT", "30", "Revolut", "To Revolut"), "transfer", "transfer.internal", "revolut", "new", "Revolut"},
		{"ibkr", row("h", "DBIT", "1000", "INTERACTIVE BROKERS IRELAND LIMITED", "top up"), "transfer", "transfer.invest", "ibkr", "new", "IBKR"},
		{"alimony", row("i", "DBIT", "1000", "EVELINA BALČIŪNIENĖ", "Aliments 2026.10"), "expense", "kids.alimony", "", "new", "Evelina"},
		{"mortgage principal", row("j", "DBIT", "466.21", "", "Paskolos grąžinimas"), "transfer", "transfer.debt", "mortgage", "new", ""},
		{"mortgage interest", row("k", "DBIT", "894.45", "", "Palūkanos už paskolą"), "expense", "housing.mortgage_interest", "", "new", ""},
		{"foreign card spend", row("l", "DBIT", "58.55", "MADKLUBBEN VEST", "PIRKINYS 516793******2950 2022.12.13 425.00 DKK VALIUTOS KURSAS 7.43 (562023) MADKLUBBEN VEST 1620 Koebenhavn"), "expense", "travel.trip", "", "new", "Madklubben Vest"},
		{"card refund", row("m", "CRDT", "5", "", "GRĄŽINIMAS 516793******2950 2026.09.01 5.00 EUR (1) X"), "income", "refunds", "", "new", ""},
		{"salary", row("n", "CRDT", "3010", "SPECTRA TECH UAB", "Darbo užmokestis 2026.09"), "income", "salary", "", "new", "Spectra Tech"},
		{"child benefit", row("o", "CRDT", "70", "VSDF", "Išmoka vaikui"), "income", "benefits", "", "new", "Sodra"},
		{"unknown credit", row("p", "CRDT", "20", "Jonas Jonaitis", "skola"), "income", "refunds", "", "new", "Jonas Jonaitis"},
	}
	for _, c := range cases {
		p := Adapt(c.tx, 1, "swed", ctx)
		if p.Kind != c.kind || p.Category != c.cat || p.ToAccountID != c.to || p.Verdict != c.verdict || p.Merchant != c.merchant {
			t.Errorf("%s: got kind=%s cat=%s to=%s verdict=%s merchant=%q", c.name, p.Kind, p.Category, p.ToAccountID, p.Verdict, p.Merchant)
		}
	}
	// A transfer to an own linked account names it.
	own := row("q", "DBIT", "100", "Someone", "pervedimas")
	own.CreditorAccount.IBAN = "LT00 0000 0000 0000 0002"
	if p := Adapt(own, 1, "swed", ctx); p.ToAccountID != "seb" || p.Verdict != "internal" {
		t.Errorf("own IBAN: %+v", p)
	}
}

func TestRowsThatNeedReview(t *testing.T) {
	fx := row("x", "DBIT", "425.00", "Shop", "card")
	fx.TransactionAmount.Currency = "DKK"
	if p := Adapt(fx, 1, "swed", ctx); p.Verdict != "needs_review" || !strings.Contains(p.VerdictNote, "DKK") {
		t.Error("non-EUR", p.Verdict)
	}
	odd := row("y", "SIDEWAYS", "1", "Shop", "card")
	if p := Adapt(odd, 1, "swed", ctx); p.Verdict != "needs_review" {
		t.Error("unknown direction", p.Verdict)
	}
	bad := row("z", "DBIT", "abc", "Shop", "card")
	if p := Adapt(bad, 1, "swed", ctx); p.Verdict != "needs_review" || p.Amount <= 0 {
		t.Error("unreadable amount", p.Verdict)
	}
	pend := row("w", "DBIT", "10", "Shop", "card")
	pend.Status = openbanking.StatusPending
	if p := Adapt(pend, 1, "swed", ctx); p.Verdict != "pending" || !p.Pending {
		t.Error("reservation", p.Verdict)
	}
}

func TestExternalIDFallsBackToHash(t *testing.T) {
	a := row("", "DBIT", "1", "x", "y")
	b := a
	if ExternalID(a, 1) != ExternalID(b, 1) || !strings.HasPrefix(ExternalID(a, 1), "eb:1:h") {
		t.Error("stable content hash")
	}
	a.TransactionID = "tid"
	if ExternalID(a, 1) != "eb:1:tid" {
		t.Error("transaction_id before hash")
	}
}

func TestNarrativeNeverConcatenated(t *testing.T) {
	tx := openbanking.Transaction{RemittanceInformation: []string{" Wolt ", ""}, BankTransactionCode: openbanking.BankTransactionCode{Description: "Card"}}
	if narrative(tx) != "Wolt" {
		t.Error(narrative(tx))
	}
	tx.RemittanceInformation = nil
	if narrative(tx) != "Card" {
		t.Error("bank code is the fallback only")
	}
}

func TestPurposeKeepsHumanText(t *testing.T) {
	if purpose("UAB X", "tvoros statyba Platiniskiu 21a") == "" {
		t.Error("human purpose dropped")
	}
	for _, noise := range []string{"PIRKINYS 1234", "E-SĄSKAITA 2026", "Transfer", "123456789"} {
		if purpose("UAB X", noise) != "" {
			t.Errorf("%q is noise", noise)
		}
	}
}

func TestCallbackURLParsing(t *testing.T) {
	code, state, err := ParseCallbackURL("https://x.duckdns.org:8443/?code=abc&state=xyz#/")
	if err != nil || code != "abc" || state != "xyz" {
		t.Fatal(code, state, err)
	}
	code, state, _ = ParseCallbackURL("https://x/#/settings/banks?code=c2&state=s2")
	if code != "c2" || state != "s2" {
		t.Fatal("query after the fragment")
	}
	if _, _, err := ParseCallbackURL("https://x/?error=access_denied&error_description=User+cancelled"); err == nil || !strings.Contains(err.Error(), "User cancelled") {
		t.Fatal("bank refusal surfaced", err)
	}
}
