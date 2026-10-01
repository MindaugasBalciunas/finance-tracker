package handler

import (
	"encoding/csv"
	"strings"
	"testing"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/openbanking"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// psd2Fixture is the EUR subset of swedFixture — the same rows, minus the
// LTL-era ones and the statement summary rows.
//
// The LTL rows are excluded on purpose: the CSV path converts them at the
// fixed changeover rate, while a live PSD2 row carries its own exchange rate
// and is deliberately flagged needs_review instead of guessed. Comparing them
// would be comparing two intentionally different behaviours.
const psd2Fixture = `"Sąskaitos Nr.","","Data","Gavėjas","Paaiškinimai","Suma","Valiuta","D/K","Įrašo Nr.","Kodas","Įmokos kodas","Dok. Nr.",
"LT16","20","2022-01-04","","Paslaugų plano ""Patogu"" programos ""Auksinė paslauga"" dalyviams mokestis 2021.12","0.70","EUR","D","1001","M","","",
"LT16","20","2022-01-06","Wolt 00180 Helsinki","PIRKINYS 516793******2950 2022.01.04 15.15 EUR (522658) Wolt 00180 Helsinki","15.15","EUR","D","1002","K","","",
"LT16","20","2022-01-09","UAB IGNITIS","E.Sąskaitos Nr. LDTESB-01464043 apmokėjimas","8.12","EUR","D","1003","MK","","SOP","",
"LT16","20","2022-01-13","VILNIAUS MIESTO SAVIVALDYBĖS ADMINISTRACIJA","Išmoka vaikui","70.00","EUR","K","1004","MK","","1841","",
"LT16","20","2022-01-14","MOBILEPAY A/S LITHUANIA BRANCH","TMP -įeinantis-  Pervedimas pagal darbo sutarti su MobilePay A/S 2022/01 men.","830.00","EUR","K","1005","MK","","",
"LT16","20","2022-01-29","EVELINA PLYTNIKAITĖ","Lizingas","300.00","EUR","D","1006","MK","","",
"LT16","20","2022-01-29","EVELINA BALČIŪNIENĖ","Vaiku darželis","210.00","EUR","D","1007","MK","","",
"LT16","20","2022-09-19","EVELINA BALČIŪNIENĖ","Namo paskola","1132.00","EUR","D","1008","MK","","",
"LT16","20","2022-05-30","'50146 LIDL SNIPISKES","PIRKINYS 516793******2950 2022.05.25 83.06 EUR (294126) 50146 LIDL SNIPISKES   08204 VILNIUS      ","83.06","EUR","D","1009","K","","",
"LT16","20","2022-08-26","","GRYNIEJI 516793******2950 26.08.22 09:22 600.00 EUR (465020) H836/HB LUKSIO G.23>VILNIUS LT","600.00","EUR","D","1010","K","","",
"LT16","20","2022-12-15","MADKLUBBEN VEST","PIRKINYS 516793******2950 2022.12.13 425.00 DKK VALIUTOS KURSAS 7.436570, VALIUTOS KEITIMO MOK 1.40 EUR (562023) MADKLUBBEN VEST        1620 Koebenhavn V ","58.55","EUR","D","1011","K","","",
"LT16","20","2023-11-27","Mindaugas BALCIUNAS","Transfer between my accounts","1000.00","EUR","K","1012","MK","","",
"LT16","20","2022-08-04","ARŪNAS KUGINYS","NT sandoris (Vienbutį gyvenamąjį namą)","26625.00","EUR","D","1013","MK","","12","",
"LT16","20","2023-02-07","","GRĄŽINIMAS 516793******2950 2023.02.06 8.96 EUR () WWW.NARYSTE.SVAROSBROLIAI","8.96","EUR","K","1014","K","","",
"LT16","20","2022-08-23","MOKI VEZI 06229 VILNIUS","PIRKINYS 516793******2950 2022.08.20 259.99 EUR (990886) MOKI VEZI 06229 VILNIUS","259.99","EUR","D","1015","K","","",
"LT16","20","2018-01-31","DANSKE BANK A/S LIETUVOS FILIALAS","Danske Bank A/S. Saskaitos papildymas 2018/01 men.","830.00","EUR","K","1020","MK","","",
"LT16","20","2024-05-02","MINDAUGAS BALČIŪNAS","Pervedimas į Taupyklės sąskaitą po 5.9 EUR mokėjimo","5.90","EUR","D","1021","MK","","",
"LT16","20","2020-04-30","MINDAUGAS BALČIŪNAS","Kredito padengimas","325.45","EUR","D","1022","MK","","",
"LT16","20","2020-09-23","UAB TOKVILA","Pradine imoka uz automobili.  VIN JTMW53FV00D502936","3336.00","EUR","D","1023","MK","","",
`

// psd2RowsFromCSV re-expresses each statement line as the Enable Banking
// transaction the same payment would arrive as. This is the bridge the whole
// feature rests on, so it is built once and used by every test below.
func psd2RowsFromCSV(t *testing.T, fixture string) []openbanking.Transaction {
	t.Helper()
	r := csv.NewReader(strings.NewReader(fixture))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	recs, err := r.ReadAll()
	require.NoError(t, err)

	var out []openbanking.Transaction
	for _, rec := range recs[1:] {
		if len(rec) < 9 || strings.TrimSpace(rec[1]) != "20" {
			continue
		}
		payee, details, dk := rec[3], rec[4], strings.TrimSpace(rec[7])
		tx := openbanking.Transaction{
			EntryReference:    strings.TrimSpace(rec[8]),
			Status:            openbanking.StatusBooked,
			BookingDate:       rec[2],
			ValueDate:         rec[2],
			TransactionAmount: openbanking.Amount{Amount: rec[5], Currency: rec[6]},
			BankTransactionCode: openbanking.BankTransactionCode{
				// Banks do send this. It must never be appended to a real
				// narrative, which is what the fidelity assertion proves.
				Description: "Card payment",
			},
			RemittanceInformation: []string{details},
		}
		if dk == "K" {
			tx.CreditDebitIndicator = openbanking.IndicatorCredit
			tx.Debtor.Name = payee
		} else {
			tx.CreditDebitIndicator = openbanking.IndicatorDebit
			tx.Creditor.Name = payee
		}
		out = append(out, tx)
	}
	return out
}

// TestPSD2AdapterMatchesCSVImport is the test that decides whether this
// feature works at all.
//
// Dedup against history is comparative: every existing row carries whatever
// the CSV importer derived from the same bank payment. If the PSD2 adapter
// produces a date one day off, or a comment with a bank code appended, the
// row is not recognised as a duplicate and ships as a new transaction. So the
// assertion is not "looks reasonable" — it is field-for-field equality with
// what parseSwedbankCSV produces from the same statement.
func TestPSD2AdapterMatchesCSVImport(t *testing.T) {
	want, _, internal, err := parseSwedbankCSV(strings.NewReader(psd2Fixture))
	require.NoError(t, err)
	require.NotEmpty(t, want)

	link := &domain.BankAccountLink{ID: 7, AccountKey: "swed"}
	var got []domain.BankStagedTx
	var skipped int
	for _, tx := range psd2RowsFromCSV(t, psd2Fixture) {
		row := adaptPSD2(tx, link)
		if row.Verdict == domain.VerdictInternal {
			skipped++
			continue
		}
		got = append(got, row)
	}

	assert.Equal(t, internal, skipped, "the same rows are treated as own-account noise on both paths")
	require.Equal(t, len(want), len(got), "one staged row per CSV-importable row")

	for i := range want {
		w, g := want[i], got[i]
		assert.Equal(t, domain.VerdictNew, g.Verdict, "row %d (%s)", i, w.Comment)
		assert.Equal(t, w.Date.Format("2006-01-02"), g.Date.Format("2006-01-02"), "date, row %d (%s)", i, w.Comment)
		assert.Equal(t, w.Type, g.Type, "type, row %d (%s)", i, w.Comment)
		assert.Equal(t, w.Category, g.Category, "category, row %d (%s)", i, w.Comment)
		assert.Equal(t, w.Comment, g.Comment, "comment, row %d", i)
		assert.Equal(t, w.Labels, g.Labels, "labels, row %d (%s)", i, w.Comment)
		assert.InDelta(t, w.Amount, g.Amount, 0.001, "amount, row %d (%s)", i, w.Comment)
		assert.Equal(t, w.Debit, g.DebitAccount, "debit, row %d (%s)", i, w.Comment)
		assert.Equal(t, w.Credit, g.CreditAccount, "credit, row %d (%s)", i, w.Comment)
	}
}

// TestPSD2DedupsAgainstCSVHistory closes the loop on the test above: the
// staged rows are keyed exactly like the ledger rows a CSV import produced,
// so every one of them is caught by content dedup rather than shipping as new.
// This is the ~150-duplicate class the staging step exists for.
func TestPSD2DedupsAgainstCSVHistory(t *testing.T) {
	csvRows, _, _, err := parseSwedbankCSV(strings.NewReader(psd2Fixture))
	require.NoError(t, err)

	existing := make([]domain.Transaction, 0, len(csvRows))
	for i, r := range csvRows {
		existing = append(existing, domain.Transaction{
			ID: uint(i + 1), Date: r.Date, Type: r.Type,
			Amount: r.Amount, Comment: r.Comment, Category: r.Category,
		})
	}
	dedup := newDedupIndex(existing)

	link := &domain.BankAccountLink{ID: 7, AccountKey: "swed"}
	for _, tx := range psd2RowsFromCSV(t, psd2Fixture) {
		row := adaptPSD2(tx, link)
		if row.Verdict != domain.VerdictNew {
			continue
		}
		match, dup := dedup.take(row.Date, row.Type, row.Amount, row.Comment)
		assert.True(t, dup, "%s (%s) should match history", row.Comment, row.Date.Format("2006-01-02"))
		assert.NotNil(t, match)
	}
}

// TestPSD2GenuineRepeatsSurvive is the other half of the dedup contract. The
// index is a multiset, so two identical payments on the same day — two rounds
// at the same bar — must not collapse into one.
func TestPSD2GenuineRepeatsSurvive(t *testing.T) {
	rows := psd2RowsFromCSV(t, psd2Fixture)
	require.NotEmpty(t, rows)
	// Two bank rows, same content, different entry references.
	a := rows[1]
	b := rows[1]
	b.EntryReference = "9001"

	link := &domain.BankAccountLink{ID: 7, AccountKey: "swed"}
	ra, rb := adaptPSD2(a, link), adaptPSD2(b, link)
	require.NotEqual(t, ra.ExternalID, rb.ExternalID, "distinct provider ids")

	// The ledger holds one of them already.
	dedup := newDedupIndex([]domain.Transaction{{
		ID: 1, Date: ra.Date, Type: ra.Type, Amount: ra.Amount, Comment: ra.Comment,
	}})
	_, dupA := dedup.take(ra.Date, ra.Type, ra.Amount, ra.Comment)
	_, dupB := dedup.take(rb.Date, rb.Type, rb.Amount, rb.Comment)
	assert.True(t, dupA, "first one matches the historical row")
	assert.False(t, dupB, "the second is a genuine repeat and must come through")
}

func TestPSD2AccountRemap(t *testing.T) {
	// A cash withdrawal: the classifier files it as swed → cash.
	rows := psd2RowsFromCSV(t, psd2Fixture)
	var atm openbanking.Transaction
	for _, r := range rows {
		if strings.HasPrefix(r.RemittanceInformation[0], "GRYNIEJI") {
			atm = r
		}
	}
	require.NotEmpty(t, atm.EntryReference)

	swed := adaptPSD2(atm, &domain.BankAccountLink{ID: 1, AccountKey: "swed"})
	assert.Equal(t, "swed", swed.DebitAccount)
	assert.Equal(t, "cash", swed.CreditAccount)

	// The same payment arriving on an SEB link. "swed" in the classifier
	// means "the account this statement belongs to", so it is rewritten —
	// but the counterparty key is correct regardless of which bank it came
	// from and must survive.
	seb := adaptPSD2(atm, &domain.BankAccountLink{ID: 2, AccountKey: "seb"})
	assert.Equal(t, "seb", seb.DebitAccount)
	assert.Equal(t, "cash", seb.CreditAccount)
}

// TestPSD2RemapLeavesSwedETFAlone — swed_etf is a genuinely different
// account. A prefix or contains match here would silently misfile every
// investment row.
func TestPSD2RemapLeavesSwedETFAlone(t *testing.T) {
	row := domain.BankStagedTx{DebitAccount: "swed_etf", CreditAccount: "swed"}
	applyAccountRemap(&row, "seb")
	assert.Equal(t, "swed_etf", row.DebitAccount)
	assert.Equal(t, "seb", row.CreditAccount)
}

func TestPSD2NonEURNeedsReview(t *testing.T) {
	tx := openbanking.Transaction{
		EntryReference:       "5001",
		Status:               openbanking.StatusBooked,
		BookingDate:          "2026-09-12",
		CreditDebitIndicator: openbanking.IndicatorDebit,
		TransactionAmount:    openbanking.Amount{Amount: "425.00", Currency: "DKK"},
		Creditor:             openbanking.PartyName{Name: "MADKLUBBEN VEST"},
	}
	row := adaptPSD2(tx, &domain.BankAccountLink{ID: 1, AccountKey: "swed"})
	assert.Equal(t, domain.VerdictNeedsReview, row.Verdict)
	assert.Contains(t, row.VerdictNote, "DKK")
	// Staged, not dropped — and with a usable starting point, because a row
	// nobody can read is still a row that happened.
	assert.Equal(t, "MADKLUBBEN VEST", row.Comment)
	assert.Equal(t, domain.TransactionTypeExpense, row.Type)
	assert.Equal(t, "swed", row.DebitAccount)
}

func TestPSD2UnknownIndicatorNeedsReview(t *testing.T) {
	tx := openbanking.Transaction{
		EntryReference:       "5002",
		Status:               openbanking.StatusBooked,
		BookingDate:          "2026-09-12",
		CreditDebitIndicator: "WHAT",
		TransactionAmount:    openbanking.Amount{Amount: "10.00", Currency: "EUR"},
	}
	row := adaptPSD2(tx, &domain.BankAccountLink{ID: 1, AccountKey: "swed"})
	// The sign of every row depends on this field, so an unreadable value is
	// reported rather than guessed.
	assert.Equal(t, domain.VerdictNeedsReview, row.Verdict)
}

// TestPSD2DetailsNeverConcatenated — the bank transaction code description is
// a fallback for an empty narrative, never an addition to a real one.
// Appending it would change the string the classifier matches on and break
// dedup against every CSV-era row.
func TestPSD2DetailsNeverConcatenated(t *testing.T) {
	withNarrative := openbanking.Transaction{
		RemittanceInformation: []string{"Lizingas"},
		BankTransactionCode:   openbanking.BankTransactionCode{Description: "Credit transfer"},
	}
	assert.Equal(t, "Lizingas", psd2Details(withNarrative))

	empty := openbanking.Transaction{
		BankTransactionCode: openbanking.BankTransactionCode{Description: "Credit transfer"},
	}
	assert.Equal(t, "Credit transfer", psd2Details(empty))
}

// TestPSD2EmptyPayeeIsLoadBearing — classifySwedbank routes an empty payee
// into its fee / ATM / refund branch. Inventing a name here (the bank's own,
// say, or the transaction code) would silently reroute all of those.
func TestPSD2EmptyPayeeIsLoadBearing(t *testing.T) {
	tx := openbanking.Transaction{
		CreditDebitIndicator: openbanking.IndicatorDebit,
		BankTransactionCode:  openbanking.BankTransactionCode{Description: "Service fee"},
	}
	assert.Equal(t, "", psd2Payee(tx))
}

func TestPSD2ExternalIDFallsBackToHash(t *testing.T) {
	link := &domain.BankAccountLink{ID: 3}
	base := openbanking.Transaction{
		BookingDate:          "2026-09-12",
		CreditDebitIndicator: openbanking.IndicatorDebit,
		TransactionAmount:    openbanking.Amount{Amount: "23.40", Currency: "EUR"},
		Creditor:             openbanking.PartyName{Name: "Lidl"},
	}
	// entry_reference wins.
	withRef := base
	withRef.EntryReference = "abc"
	assert.Equal(t, "eb:3:abc", psd2ExternalID(withRef, link.ID))

	// Then transaction_id.
	withTxID := base
	withTxID.TransactionID = "t-9"
	assert.Equal(t, "eb:3:t-9", psd2ExternalID(withTxID, link.ID))

	// With neither, a content hash — stable for the same row, different for
	// a different one. A bank sending no row id must not collapse every one
	// of its transactions onto a single external id.
	h1 := psd2ExternalID(base, link.ID)
	h2 := psd2ExternalID(base, link.ID)
	other := base
	other.TransactionAmount.Amount = "23.41"
	assert.Equal(t, h1, h2)
	assert.NotEqual(t, h1, psd2ExternalID(other, link.ID))
	assert.True(t, strings.HasPrefix(h1, "eb:3:h"))
}

// TestPSD2PrefersEmbeddedPurchaseDate — the provider's own transaction_date
// is NOT preferred over the regex. History carries the date the regex
// produced during CSV import; a PSD2 row using transaction_date instead would
// differ by a day on exactly the rows where the two disagree, and content
// dedup would miss.
func TestPSD2PrefersEmbeddedPurchaseDate(t *testing.T) {
	tx := openbanking.Transaction{
		EntryReference:       "6001",
		Status:               openbanking.StatusBooked,
		BookingDate:          "2022-01-06",
		TransactionDate:      "2022-01-05", // provider disagrees with the narrative
		CreditDebitIndicator: openbanking.IndicatorDebit,
		TransactionAmount:    openbanking.Amount{Amount: "15.15", Currency: "EUR"},
		Creditor:             openbanking.PartyName{Name: "Wolt 00180 Helsinki"},
		RemittanceInformation: []string{
			"PIRKINYS 516793******2950 2022.01.04 15.15 EUR (522658) Wolt 00180 Helsinki",
		},
	}
	row := adaptPSD2(tx, &domain.BankAccountLink{ID: 1, AccountKey: "swed"})
	assert.Equal(t, "2022-01-04", row.Date.Format("2006-01-02"))

	// With no date in the narrative, the provider fields are the fallback.
	plain := tx
	plain.RemittanceInformation = []string{"Wolt"}
	assert.Equal(t, "2022-01-05", adaptPSD2(plain, &domain.BankAccountLink{ID: 1, AccountKey: "swed"}).Date.Format("2006-01-02"))
}
