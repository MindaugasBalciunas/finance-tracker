package handler

import (
	"fmt"
	"strings"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

// dedupIndex matches incoming rows against the existing ledger by content.
//
// It is a MULTISET, not a set: each existing row absorbs exactly one
// incoming row, so a re-import is a no-op while genuine repeats (two
// identical rounds at the same bar on the same day) still come through.
// Category is deliberately excluded from the key — the same bank row may
// have been categorised differently by an earlier import.
//
// Shared by the CSV importer and the PSD2 staging path. Both need exactly
// this behaviour, and a third copy of it is how the two would drift.
type dedupIndex struct {
	remaining map[string]int
	// rows keeps one pointer per occupied key so a caller can name the
	// transaction a row matched ("looks like #1234 · 2026-09-12 · Lidl").
	rows map[string][]*domain.Transaction
}

// newDedupIndex builds the baseline from every existing transaction. The
// caller must treat a failed baseline read as fatal: proceeding with an empty
// index re-imports an entire statement.
func newDedupIndex(existing []domain.Transaction) *dedupIndex {
	d := &dedupIndex{
		remaining: make(map[string]int, len(existing)),
		rows:      make(map[string][]*domain.Transaction, len(existing)),
	}
	for i := range existing {
		t := &existing[i]
		k := dedupKey(t.Date, t.Type, t.Amount, t.Comment)
		d.remaining[k]++
		d.rows[k] = append(d.rows[k], t)
	}
	return d
}

func dedupKey(date time.Time, typ domain.TransactionType, amount float64, comment string) string {
	return fmt.Sprintf("%s|%s|%.2f|%s", date.Format("2006-01-02"), typ, amount, strings.ToLower(strings.TrimSpace(comment)))
}

// take consumes one match for the given row, returning the transaction it
// matched (nil if the match was added by add rather than read from the DB).
// A second call with the same content only matches if the ledger really does
// hold two such rows.
func (d *dedupIndex) take(date time.Time, typ domain.TransactionType, amount float64, comment string) (*domain.Transaction, bool) {
	k := dedupKey(date, typ, amount, comment)
	if d.remaining[k] <= 0 {
		return nil, false
	}
	d.remaining[k]--
	var matched *domain.Transaction
	if rows := d.rows[k]; len(rows) > 0 {
		matched = rows[0]
		d.rows[k] = rows[1:]
	}
	return matched, true
}

// has reports a match without consuming it — for a read-only verdict that
// will be re-checked at commit time.
func (d *dedupIndex) has(date time.Time, typ domain.TransactionType, amount float64, comment string) (*domain.Transaction, bool) {
	k := dedupKey(date, typ, amount, comment)
	if d.remaining[k] <= 0 {
		return nil, false
	}
	if rows := d.rows[k]; len(rows) > 0 {
		return rows[0], true
	}
	return nil, true
}

// add records a row written during this batch, so a later row with identical
// content in the same batch is recognised as a duplicate of it.
func (d *dedupIndex) add(date time.Time, typ domain.TransactionType, amount float64, comment string) {
	d.remaining[dedupKey(date, typ, amount, comment)]++
}
