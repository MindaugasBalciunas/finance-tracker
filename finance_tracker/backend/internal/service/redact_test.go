package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The raw PSD2 fields are the one place account identifiers survive, and they
// are the one thing in a bank row that is worth nothing to a model.
func TestRedactIdentifiers(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			"counterparty IBAN — psd2Payee falls back to one when the bank sends no name",
			"LT160000000000001234",
			"[account]",
		},
		{
			"IBAN printed in four-character groups",
			"Transfer to LT16 0000 0000 0000 1234 thanks",
			"Transfer to [account] thanks",
		},
		{
			"masked card number in a purchase narrative",
			"PIRKINYS 516793******2950 2026.01.04 23.40 EUR LIDL",
			"PIRKINYS [card] 2026.01.04 23.40 EUR LIDL",
		},
		{
			"a bare account-length digit run",
			"Order 4532015112830366 paid",
			"Order [number] paid",
		},
		{
			"the merchant name is the signal and stays",
			"BARBORA UAB",
			"BARBORA UAB",
		},
		{
			"amounts, dates and short references are left alone",
			"Mokestis 23.40 EUR 2026-01-04 ref 12345",
			"Mokestis 23.40 EUR 2026-01-04 ref 12345",
		},
		{"empty stays empty", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, redactIdentifiers(tc.in))
		})
	}
}
