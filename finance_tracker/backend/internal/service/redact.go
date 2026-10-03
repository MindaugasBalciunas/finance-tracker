package service

import "regexp"

// Redaction for text leaving this machine for the AI gateway.
//
// The raw PSD2 fields are the one place account identifiers survive: psd2Payee
// deliberately falls back to the counterparty's IBAN when the bank sends no
// name, and a card narrative carries a masked PAN ("516793******2950").
// Those strings are a third party's banking details, and they are worth
// nothing to a model deciding whether a purchase is groceries.
//
// The counterparty's NAME is deliberately kept — it is the whole signal for
// categorising, and sending it is what the user opted into by turning AI on.
// Only the numbers that identify an account go.

var (
	// IBAN: two letters, two check digits, then 10–30 alphanumerics, which
	// banks print either solid or in four-character groups.
	ibanRe = regexp.MustCompile(`\b[A-Z]{2}[0-9]{2}(?:[ ]?[A-Z0-9]{4}){2,8}\b`)
	// A masked card number as the LT banks write it.
	maskedPANRe = regexp.MustCompile(`\b[0-9]{4,6}\*{2,}[0-9]{2,4}\b`)
	// Any bare run long enough to be a card or account number. Deliberately
	// after the two above, so the more specific label wins.
	longDigitsRe = regexp.MustCompile(`\b[0-9]{12,19}\b`)
)

// redactIdentifiers strips account and card numbers from text bound for the
// AI gateway, leaving everything a model actually reasons over.
func redactIdentifiers(s string) string {
	if s == "" {
		return s
	}
	s = maskedPANRe.ReplaceAllString(s, "[card]")
	s = ibanRe.ReplaceAllString(s, "[account]")
	s = longDigitsRe.ReplaceAllString(s, "[number]")
	return s
}
