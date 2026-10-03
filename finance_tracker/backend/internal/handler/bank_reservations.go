package handler

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
)

// The reservation lifecycle.
//
// A card purchase reaches the bank twice: first as a reservation (the money
// is held, nothing is booked), then days later as a booked row — under a NEW
// entry_reference, often a new booking date, and sometimes a different
// amount, because a fuel pre-auth is a round number and a restaurant adds the
// tip after you have left.
//
// Neither dedup layer can connect the two: layer 1 keys on the provider
// reference, which changed, and layer 2 keys on date+amount+comment, any of
// which may have moved. So the pairing is done here, on what does hold
// steady — the account, the direction, the counterparty, and an amount that
// is close enough — and consumed once, so two identical reservations are
// resolved by two bookings rather than both collapsing onto one.

// reservationWindow is how long a reservation may wait for its booking.
// Card schemes settle within a few days; a hold older than this that is still
// being reported is stale rather than pending.
const reservationWindow = 10 * 24 * time.Hour

// reservationTolerance is how far a booked amount may drift from the amount
// reserved and still be the same purchase. A tip or a fuel pre-auth moves it;
// a different purchase at the same merchant usually moves it further.
const reservationTolerance = 0.20 // 20%

// openReservations holds the reservations still awaiting a booking, ready to
// be claimed. Claiming removes one, so each booking resolves exactly one.
type openReservations struct {
	rows []*domain.BankStagedTx
	// taken marks rows already claimed in this run.
	taken map[uint]bool
}

func (h *BankHandler) openReservations(linkID uint) (*openReservations, error) {
	rows, _, err := h.repo.ListStaged(repository.StagedFilter{
		State: domain.StagedStateStaged, LinkID: linkID, PageSize: 500,
	})
	if err != nil {
		return nil, fmt.Errorf("reading open reservations: %w", err)
	}
	out := &openReservations{taken: map[uint]bool{}}
	for i := range rows {
		if rows[i].Pending {
			out.rows = append(out.rows, &rows[i])
		}
	}
	return out, nil
}

// claim finds the reservation this booked row resolves, removes it from the
// pool and copies forward the corrections made while it was pending.
//
// Returns nil for a booking that resolves nothing, which is the normal case
// for anything that was never reserved — a transfer, a direct debit, or a
// card row whose reservation was never fetched.
func (o *openReservations) claim(booked *domain.BankStagedTx) *domain.BankStagedTx {
	if booked == nil || booked.Pending || o == nil {
		return nil
	}
	best := -1
	bestGap := math.MaxFloat64
	for i, r := range o.rows {
		if o.taken[r.ID] || !reservationMatches(r, booked) {
			continue
		}
		// Closest amount wins, so two holds at one merchant resolve to the
		// bookings that actually match them.
		gap := math.Abs(r.RawAmount - booked.RawAmount)
		if gap < bestGap {
			best, bestGap = i, gap
		}
	}
	if best < 0 {
		return nil
	}
	hold := o.rows[best]
	o.taken[hold.ID] = true
	carryReservationEdits(hold, booked)
	return hold
}

// remaining lists the reservations no booking claimed.
func (o *openReservations) remaining() []*domain.BankStagedTx {
	var out []*domain.BankStagedTx
	for _, r := range o.rows {
		if !o.taken[r.ID] {
			out = append(out, r)
		}
	}
	return out
}

// reservationMatches decides whether a booked row is the settlement of a
// reservation. Everything it tests is a field the bank does not rewrite
// between the two: the direction, the currency, the counterparty, roughly the
// amount, and a few days at most.
func reservationMatches(hold, booked *domain.BankStagedTx) bool {
	if hold.RawDK != booked.RawDK || !strings.EqualFold(hold.RawCurrency, booked.RawCurrency) {
		return false
	}
	if !sameCounterparty(hold.RawPayee, booked.RawPayee) {
		return false
	}
	// Booked on or after the hold, and not months later.
	gap := booked.Date.Sub(hold.Date)
	if gap < -24*time.Hour || gap > reservationWindow {
		return false
	}
	return amountsClose(hold.RawAmount, booked.RawAmount)
}

// amountsClose allows the drift a tip or a pre-auth introduces, and nothing
// like a second purchase.
func amountsClose(held, booked float64) bool {
	if held <= 0 || booked <= 0 {
		return false
	}
	// A small absolute floor so cheap rows are not matched by percentage
	// alone — 20% of €2 is 40 cents, which is most of a price difference.
	allowed := math.Max(held*reservationTolerance, 0.50)
	return math.Abs(held-booked) <= allowed
}

// sameCounterparty compares merchant names loosely enough to survive the
// reformatting banks do between a hold and its booking (padding, terminal
// ids, case). An empty payee on both sides — bank fees, ATM rows — is not
// treated as a match: it identifies nothing.
func sameCounterparty(a, b string) bool {
	na, nb := normalisePayee(a), normalisePayee(b)
	if na == "" || nb == "" {
		return false
	}
	return na == nb || strings.HasPrefix(na, nb) || strings.HasPrefix(nb, na)
}

func normalisePayee(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		switch {
		case r >= 'A' && r <= 'Z', r > 127:
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			// Digits are terminal and store numbers — they move between the
			// hold and the booking, so they are not part of the identity.
		}
	}
	return b.String()
}

// carryReservationEdits moves the user's corrections from the reservation
// onto the booked row that replaces it.
//
// Only the fields a person would have fixed, and deliberately NOT date,
// amount or comment: those three are the booked row's own truth (the final
// amount is the whole reason to wait for it) and the comment is part of the
// content-dedup key, which has to stay byte-identical to what the CSV
// importer would have produced.
func carryReservationEdits(hold, booked *domain.BankStagedTx) {
	if hold.Category != "" {
		booked.Category = hold.Category
	}
	if hold.Type != "" {
		booked.Type = hold.Type
	}
	if hold.Labels != "" {
		booked.Labels = domain.NormalizeLabels(hold.Labels)
	}
	if hold.DebitAccount != "" {
		booked.DebitAccount = hold.DebitAccount
	}
	if hold.CreditAccount != "" {
		booked.CreditAccount = hold.CreditAccount
	}
	if hold.EnrichNote != "" && booked.EnrichNote == "" {
		booked.EnrichNote = hold.EnrichNote
	}
}

// closeReservation retires a reservation: superseded by the booking that
// replaced it, or released by the bank. Kept rather than deleted — "where did
// that row go" deserves an answer, and the raw provider payload is the audit
// trail for a hold that never became anything.
func (h *BankHandler) closeReservation(hold *domain.BankStagedTx, supersededBy *uint, note string) error {
	hold.State = domain.StagedStateSuperseded
	hold.SupersededBy = supersededBy
	hold.VerdictNote = note
	hold.LastSeenAt = time.Now()
	if err := h.repo.SaveStaged(hold); err != nil {
		return fmt.Errorf("closing reservation %s: %w", hold.ExternalID, err)
	}
	return nil
}
