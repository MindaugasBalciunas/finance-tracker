package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/openbanking"
)

// PSD2 → ledger adapter.
//
// This lives in package handler, not in package openbanking, because
// classifySwedbank and its helpers are unexported here: an adapter in
// openbanking would need openbanking to import handler, which imports
// openbanking back. Keeping the API client free of domain knowledge is also
// the cleaner split.
//
// The guiding rule throughout: produce *byte-identical* inputs to what the
// CSV importer would have produced for the same bank row. Dedup against the
// existing ledger is comparative — history carries whatever the CSV path
// derived, so any difference here (a date shifted by a day, a bank code
// appended to the narrative) turns a duplicate into a new row.

// adaptPSD2 converts one provider row into a staged transaction. The returned
// row always has a verdict assigned from its own content (internal /
// needs_review / new); dedup against the ledger happens separately, in the
// caller, which is where the baseline lives.
func adaptPSD2(tx openbanking.Transaction, link *domain.BankAccountLink) domain.BankStagedTx {
	payee := psd2Payee(tx)
	details := psd2Details(tx)
	dk, dkOK := psd2Indicator(tx)
	amount, amtErr := strconv.ParseFloat(strings.TrimSpace(tx.TransactionAmount.Amount), 64)
	// The API signs nothing: direction lives entirely in
	// credit_debit_indicator, matching the CSV's D/K column.
	if amount < 0 {
		amount = -amount
	}
	currency := strings.ToUpper(strings.TrimSpace(tx.TransactionAmount.Currency))

	booking := psd2Date(tx.BookingDate)
	date := psd2PreferredDate(tx, details, booking)

	raw, _ := json.Marshal(tx)
	staged := domain.BankStagedTx{
		LinkID:      link.ID,
		ExternalID:  psd2ExternalID(tx, link.ID),
		Raw:         string(raw),
		RawPayee:    payee,
		RawDetails:  details,
		RawAmount:   amount,
		RawDK:       dk,
		RawCurrency: currency,
		BookingDate: booking,
		Date:        date,
		Amount:      amount,
	}

	// A row we cannot read is staged, never dropped. Silently discarding a
	// transaction because its currency was unexpected is exactly the failure
	// mode this feature exists to avoid.
	switch {
	case !dkOK:
		staged.Verdict = domain.VerdictNeedsReview
		staged.VerdictNote = fmt.Sprintf("unrecognised direction %q — set it by hand", tx.CreditDebitIndicator)
	case amtErr != nil || amount <= 0:
		staged.Verdict = domain.VerdictNeedsReview
		staged.VerdictNote = fmt.Sprintf("could not read the amount %q", tx.TransactionAmount.Amount)
	case currency != "" && currency != "EUR":
		// The CSV path converts LTL at the fixed changeover rate. A live
		// foreign-currency row carries its own exchange_rate and guessing it
		// would be inventing a number.
		staged.Verdict = domain.VerdictNeedsReview
		staged.VerdictNote = fmt.Sprintf("%s transaction — enter the euro amount by hand", currency)
	}
	if staged.Verdict == domain.VerdictNeedsReview {
		// Still give the user a usable starting point. The classifier never
		// ran, so the account is set here rather than remapped: the one fact
		// about this row that is not in doubt is which account it arrived on.
		staged.Type = domain.TransactionTypeExpense
		staged.Category = domain.CategoryFinance
		staged.Comment = strings.TrimSpace(firstNonEmpty(payee, details, "Bank transaction"))
		staged.DebitAccount = link.AccountKey
		if dk == "K" {
			staged.Type = domain.TransactionTypeIncome
			staged.DebitAccount = ""
			staged.CreditAccount = link.AccountKey
		}
		// Finance is a placeholder here, not a reading of the row — the
		// enricher may still find a better category in the user's history.
		staged.CategoryGuessed = true
		return staged
	}

	row, skip := classifySwedbank(date, payee, details, amount, dk)
	if skip {
		// The classifier's "internal" verdict: own-account movement that the
		// CSV path drops. Staged (collapsed, unticked) rather than dropped,
		// so a misfire is visible instead of silent.
		staged.Verdict = domain.VerdictInternal
		staged.VerdictNote = "looks like a movement between your own accounts"
		staged.Type = domain.TransactionTypeInvestment
		staged.Category = domain.CategoryTransfers
		staged.Comment = strings.TrimSpace(firstNonEmpty(payee, details, "Internal transfer"))
		applyAccountRemap(&staged, link.AccountKey)
		return staged
	}

	staged.Verdict = domain.VerdictNew
	staged.Date = row.Date
	staged.Type = row.Type
	staged.Category = row.Category
	staged.Comment = row.Comment
	staged.Labels = row.Labels
	staged.Amount = row.Amount
	staged.DebitAccount = row.Debit
	staged.CreditAccount = row.Credit
	staged.CategoryGuessed = row.Guessed
	applyAccountRemap(&staged, link.AccountKey)
	return staged
}

// applyAccountRemap rewrites the classifier's "swed" placeholder to the
// account this link actually feeds.
//
// classifySwedbank hardcodes "swed" in seventeen places, always meaning "the
// account this statement belongs to". For an SEB link that is a different
// key. Exact equality only — swed_etf is a genuinely different account and
// must survive untouched. Counterparty keys (cash, rev_m) are correct
// regardless of which bank the row came from and are left alone.
func applyAccountRemap(t *domain.BankStagedTx, accountKey string) {
	if accountKey == "" || accountKey == "swed" {
		return
	}
	if t.DebitAccount == "swed" {
		t.DebitAccount = accountKey
	}
	if t.CreditAccount == "swed" {
		t.CreditAccount = accountKey
	}
}

// psd2Payee picks the counterparty. For a debit that is the creditor; for a
// credit, the debtor.
//
// The empty payee is load-bearing: classifySwedbank routes payee == "" into
// its fee / ATM / Robur branch, so inventing a name here — "Swedbank", say,
// or the bank transaction code — would silently reroute those rows.
func psd2Payee(tx openbanking.Transaction) string {
	var name, iban string
	if strings.ToUpper(tx.CreditDebitIndicator) == openbanking.IndicatorCredit {
		name, iban = tx.Debtor.Name, tx.DebtorAccount.IBAN
	} else {
		name, iban = tx.Creditor.Name, tx.CreditorAccount.IBAN
	}
	if n := strings.TrimSpace(name); n != "" {
		return n
	}
	// The counterparty IBAN is a weaker identifier but still an identifier —
	// and the CSV's payee column carries one for plain transfers too.
	return strings.TrimSpace(iban)
}

// psd2Details builds the narrative the classifier matches on.
//
// remittance_information is the direct analogue of the CSV's Paaiškinimai
// column. When it is empty the bank transaction code description is the only
// description there is — but the two are never *concatenated*: appending a
// bank code to a real narrative changes the string the classifier sees and
// breaks dedup against every CSV-era row.
func psd2Details(tx openbanking.Transaction) string {
	parts := make([]string, 0, len(tx.RemittanceInformation))
	for _, p := range tx.RemittanceInformation {
		if s := strings.TrimSpace(p); s != "" {
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

// psd2Indicator maps CRDT/DBIT onto the CSV's K/D. An unrecognised value is
// reported, not guessed: the sign of every row depends on it.
func psd2Indicator(tx openbanking.Transaction) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(tx.CreditDebitIndicator)) {
	case openbanking.IndicatorCredit:
		return "K", true
	case openbanking.IndicatorDebit:
		return "D", true
	}
	return "", false
}

// psd2PreferredDate resolves the transaction date the way the CSV importer
// does, and in the same order.
//
// It is tempting to just take transaction_date — the provider's own card
// purchase date — and skip the regexes. That would be wrong: historical rows
// carry the date the *regex* produced during CSV import. If a PSD2 row uses
// transaction_date and the two differ by a day, content dedup misses and the
// duplicate ships. Running the same regex on the same narrative yields a
// byte-identical date; the provider fields are the fallback for narratives
// that carry no embedded date.
func psd2PreferredDate(tx openbanking.Transaction, details string, booking time.Time) time.Time {
	if m := purchaseDateRe.FindStringSubmatch(details); m != nil {
		if d, err := time.Parse("2006-01-02", m[1]+"-"+m[2]+"-"+m[3]); err == nil {
			return d
		}
	}
	if m := cashOpDateRe.FindStringSubmatch(details); m != nil {
		// ATM rows carry DD.MM.YY.
		if d, err := time.Parse("06-01-02", m[3]+"-"+m[2]+"-"+m[1]); err == nil {
			return d
		}
	}
	if d := psd2Date(tx.TransactionDate); !d.IsZero() {
		return d
	}
	if !booking.IsZero() {
		return booking
	}
	return psd2Date(tx.ValueDate)
}

// psd2Date parses an ISO date. Dates are stored at UTC midnight throughout
// the ledger, which is what the CSV path produces too.
func psd2Date(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if d, err := time.Parse("2006-01-02", s); err == nil {
		return d
	}
	// Some banks send a full timestamp where the schema says date.
	if d, err := time.Parse(time.RFC3339, s); err == nil {
		return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	}
	return time.Time{}
}

// psd2ExternalID is the upsert key and the layer-1 dedup key.
//
// entry_reference is the bank's own row id and is preferred. Whether it is
// genuinely stable per bank is verified empirically; the commit path
// pre-checks by external_id and treats a hit as already-imported, so the
// unique index is a loud backstop rather than the primary mechanism. The
// content hash is the last resort for a bank that sends neither id.
func psd2ExternalID(tx openbanking.Transaction, linkID uint) string {
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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
