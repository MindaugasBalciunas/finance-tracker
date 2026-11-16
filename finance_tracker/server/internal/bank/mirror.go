package bank

import (
	"database/sql"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"ft/internal/db"
	"ft/internal/entry"
	"ft/internal/ledger"
	"ft/internal/money"
)

// ── one transfer, two banks ─────────────────────────────────────────
//
// Money moved between two of your own synced accounts is reported twice:
// the sending bank lists it going out, the receiving bank lists it coming
// in. Each row has its own bank id, so both used to become a transfer —
// €1,400 sent from Swedbank to SEB showed as two €1,400 transfers and the
// month's transfer total doubled. Here the second report is recognised as
// the other side of the first and folded into the same transaction.

const (
	mirrorNote      = "the other bank's side of a transfer already in your ledger"
	mirrorDayWindow = 3 // a transfer sent on Friday evening can arrive on Monday
	// apartNote marks a row the owner said is not the transfer it was folded
	// into (Unlink): it is never folded again.
	apartNote = "kept apart from the transfer it looked like"
)

// leg is one bank's report of a transfer: an inbox row or a ledger
// transaction that came from one. feed is the ledger account of the bank
// account that reported it, which tells the sending side from the receiving.
type leg struct {
	id       int64
	ext      string
	date     string
	from, to string
	feed     string
	amount   money.Cents
}

// feeds maps each linked bank account to the ledger account it feeds. Only
// accounts with a live session count — the same test the balance sheet uses
// for "the bank states this balance", so removing a merged duplicate never
// moves a balance the bank owns.
func feeds(q querier) (map[int64]string, error) {
	rows, err := q.Query(`SELECT id, account_id FROM bank_accounts WHERE account_id != '' AND uid != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var acct string
		if err := rows.Scan(&id, &acct); err != nil {
			return nil, err
		}
		out[id] = acct
	}
	return out, rows.Err()
}

// feedOf reads the bank account out of a bank id ("eb:<bank account>:<ref>").
func feedOf(ext string, f map[int64]string) string {
	parts := strings.SplitN(ext, ":", 3)
	if len(parts) != 3 || parts[0] != "eb" {
		return ""
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return ""
	}
	return f[id]
}

func dayGap(a, b string) float64 {
	ad, err1 := time.Parse("2006-01-02", a)
	bd, err2 := time.Parse("2006-01-02", b)
	if err1 != nil || err2 != nil {
		return math.MaxFloat64
	}
	return math.Abs(ad.Sub(bd).Hours() / 24)
}

// mirrors: the same amount within a few days, one leg reported by the
// sending account and the other by the receiving account, at least one of
// them naming the other's account, and neither naming a different one. Two
// payments of the same amount out of the same account are two payments,
// never a mirror; and "€500 left Swedbank for somewhere" plus "€500 reached
// SEB from somewhere" is not proof enough — those stay for the owner.
func mirrors(a, b leg) bool {
	if a.amount != b.amount || a.feed == "" || b.feed == "" || a.feed == b.feed || dayGap(a.date, b.date) > mirrorDayWindow {
		return false
	}
	if a.to == a.feed {
		a, b = b, a
	}
	return a.from == a.feed && b.to == b.feed && (a.to == "" || a.to == b.feed) && (b.from == "" || b.from == a.feed) &&
		(a.to == b.feed || b.from == a.feed)
}

// ledgerLegs lists bank-reported transfers that have not absorbed another
// bank's side yet — a transfer has two sides, so it merges once. That mark
// is the absorbed inbox row itself, and the inbox is not part of a plain
// backup: after a restore onto an instance without it, the rows are staged
// afresh and matched again by the same rule.
func ledgerLegs(q querier, f map[int64]string, where string, args ...any) ([]leg, error) {
	rows, err := q.Query(`SELECT t.id, t.external_id, t.date, COALESCE(t.account_id,''), COALESCE(t.to_account_id,''), t.amount
		FROM transactions t
		WHERE t.kind='transfer' AND t.pending=0 AND t.split_of IS NULL AND t.external_id LIKE 'eb:%' AND `+where+`
		  AND NOT EXISTS (SELECT 1 FROM transactions p WHERE p.split_of=t.id)
		  AND NOT EXISTS (SELECT 1 FROM bank_inbox b WHERE b.imported_tx_id=t.id AND b.external_id<>t.external_id AND b.state='imported')
		  AND NOT EXISTS (SELECT 1 FROM bank_inbox b WHERE b.external_id=t.external_id AND b.verdict_note='`+apartNote+`')
		ORDER BY t.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []leg
	for rows.Next() {
		var l leg
		var cents int64
		if err := rows.Scan(&l.id, &l.ext, &l.date, &l.from, &l.to, &cents); err != nil {
			return nil, err
		}
		l.amount, l.feed = money.Cents(cents), feedOf(l.ext, f)
		out = append(out, l)
	}
	return out, rows.Err()
}

func rowLeg(r *InboxRow, f map[int64]string) leg {
	feed := f[r.BankAccountID]
	if feed == "" {
		feed = feedOf(r.ExternalID, f)
	}
	return leg{id: r.ID, ext: r.ExternalID, date: r.Date, from: r.AccountID, to: r.ToAccountID, feed: feed, amount: r.Amount}
}

// findMirror returns the ledger transfer a bank row is the other side of
// (0 = none): the closest in date, then the oldest.
func findMirror(q querier, r *InboxRow, f map[int64]string) (int64, error) {
	if r.Kind != "transfer" || r.Pending || r.Amount <= 0 || r.VerdictNote == apartNote {
		return 0, nil
	}
	me := rowLeg(r, f)
	if me.feed == "" {
		return 0, nil
	}
	cands, err := ledgerLegs(q, f, `t.amount=?`, int64(r.Amount))
	if err != nil {
		return 0, err
	}
	var best int64
	bestGap := math.MaxFloat64
	for _, c := range cands {
		if g := dayGap(c.date, me.date); mirrors(me, c) && g < bestGap {
			best, bestGap = c.id, g
		}
	}
	return best, nil
}

// absorb folds a bank row into the transfer it mirrors: the transfer gains
// whichever account its own bank couldn't name, and the row is filed as
// imported under it, so deleting the transfer returns both rows to the inbox.
func absorb(tx *sql.Tx, r *InboxRow, txID int64) error {
	if err := completeTransfer(tx, txID, r.AccountID, r.ToAccountID, "", nil); err != nil {
		return err
	}
	r.State, r.ImportedTxID, r.MatchedTxID, r.Verdict, r.VerdictNote = "imported", txID, txID, "duplicate", mirrorNote
	return saveInbox(tx, r)
}

// completeTransfer fills the gaps of a transfer from its other side: a
// missing account, a missing note, tags it lacks. What the owner already
// wrote always stays.
func completeTransfer(tx *sql.Tx, id int64, from, to, note string, tags []string) error {
	cur, err := ledger.Get(tx, id)
	if err != nil {
		return err
	}
	next := cur
	if next.AccountID == "" && from != next.ToAccountID {
		next.AccountID = from
	}
	if next.ToAccountID == "" && to != next.AccountID {
		next.ToAccountID = to
	}
	if next.Note == "" {
		next.Note = note
	}
	next.Tags = ledger.NormalizeTags(append(append([]string{}, cur.Tags...), tags...))
	if next.AccountID == cur.AccountID && next.ToAccountID == cur.ToAccountID && next.Note == cur.Note && len(next.Tags) == len(cur.Tags) {
		return nil
	}
	return entry.Update(tx, cur, &next)
}

// linkMirrors files every open bank row that is the other side of a
// transfer already in the ledger. Returns how many it filed.
func (s *Service) linkMirrors() int {
	f, err := feeds(s.DB)
	if err != nil || len(f) < 2 {
		return 0
	}
	rows, err := listInbox(s.DB, "open")
	if err != nil {
		return 0
	}
	n := 0
	for i := range rows {
		r := &rows[i]
		if r.Kind != "transfer" || r.Pending {
			continue
		}
		var into int64
		err := entry.Do(s.DB, func(tx *sql.Tx) error {
			id, err := findMirror(tx, r, f)
			if err != nil || id == 0 {
				return err
			}
			into = id
			return absorb(tx, r, id)
		})
		// One row that can't be folded (its transfer no longer validates)
		// must not hold up the rest.
		if err != nil {
			log.Printf("bank: could not merge row %s into its transfer: %v", r.ExternalID, err)
			continue
		}
		if into > 0 {
			log.Printf("bank: row %s (%s, %s) is the other side of transaction %d — merged", r.ExternalID, r.Date, r.Amount, into)
			n++
		}
	}
	return n
}

// mergeMirrored repairs transfers that are already in the ledger twice, once
// from each bank: the older transaction stays (completed from the other),
// the newer one is removed through entry so balances follow, and its bank
// row is filed under the one that stayed. Returns how many pairs it merged.
func (s *Service) mergeMirrored() int {
	f, err := feeds(s.DB)
	if err != nil || len(f) < 2 {
		return 0
	}
	since := time.Now().AddDate(0, 0, -150).Format("2006-01-02")
	legs, err := ledgerLegs(s.DB, f, `t.date>=?`, since)
	if err != nil {
		return 0
	}
	gone := map[int64]bool{}
	n := 0
	for i := range legs {
		a := legs[i]
		if gone[a.id] {
			continue
		}
		var twin *leg
		bestGap := math.MaxFloat64
		for j := i + 1; j < len(legs); j++ {
			b := &legs[j]
			if g := dayGap(a.date, b.date); !gone[b.id] && mirrors(a, *b) && g < bestGap {
				twin, bestGap = b, g
			}
		}
		if twin == nil {
			continue
		}
		keep, drop := a, *twin
		err := entry.Do(s.DB, func(tx *sql.Tx) error {
			kept, err := ledger.Get(tx, keep.id)
			if err != nil {
				return err
			}
			dup, err := ledger.Get(tx, drop.id)
			if err != nil {
				return err
			}
			// The side tied to a recurring item is the one the plan knows.
			if kept.RecurringID == 0 && dup.RecurringID != 0 {
				keep, drop, dup = drop, keep, kept
			}
			if err := completeTransfer(tx, keep.id, dup.AccountID, dup.ToAccountID, dup.Note, dup.Tags); err != nil {
				return err
			}
			if err := entry.Delete(tx, drop.id); err != nil {
				return err
			}
			_, err = tx.Exec(`UPDATE bank_inbox SET state='imported', imported_tx_id=?, matched_tx_id=?, verdict='duplicate', verdict_note=?, last_seen_at=?
				WHERE external_id=?`, keep.id, keep.id, mirrorNote, db.Now(), drop.ext)
			return err
		})
		gone[a.id], gone[twin.id] = true, true
		if err != nil {
			log.Printf("bank: could not merge transfers %d and %d: %v", a.id, twin.id, err)
			continue
		}
		// The ledger lost a row: leave a trace of exactly which one.
		log.Printf("bank: transfer %d (%s, %s %s) was the other bank's side of %d (%s) — merged into it and removed",
			drop.id, drop.ext, drop.date, drop.amount, keep.id, keep.ext)
		n++
	}
	return n
}
