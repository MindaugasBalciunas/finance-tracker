package insights

import (
	"sort"

	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/wealth"
)

// ReconGap is a stretch between two recorded balances where the balance
// moved differently from the transactions dated in between.
type ReconGap struct {
	From        string      `json:"from"`
	To          string      `json:"to"`
	Balance     money.Cents `json:"balance_change"`
	Ledger      money.Cents `json:"ledger_flow"`
	Unexplained money.Cents `json:"unexplained"`
	// Timing: undone within a few days — a payment booked on another day
	// than the one it is dated, not missing money.
	Timing bool `json:"timing"`
}

// ReconAccount checks one cash-like account over a window.
type ReconAccount struct {
	AccountID string      `json:"account_id"`
	Name      string      `json:"name"`
	Kind      string      `json:"kind"`
	Source    string      `json:"latest_source"` // bank, manual, import…
	From      string      `json:"from"`
	To        string      `json:"to"`
	Opening   money.Cents `json:"opening"`
	Closing   money.Cents `json:"closing"`
	Ledger    money.Cents `json:"ledger_flow"`
	// Unexplained is what the transactions don't account for over the whole
	// window: closing − opening − ledger flow. Zero means it reconciles.
	Unexplained money.Cents `json:"unexplained"`
	Intervals   int         `json:"intervals"`
	Matched     int         `json:"matched"`
	Gaps        []ReconGap  `json:"gaps"`
	// LateBooking: one payment dated just before the window opened (or
	// before its last balance) that the bank booked on another day, and that
	// accounts for the whole unexplained amount.
	LateBooking *ReconLate `json:"late_booking,omitempty"`
	// Reconciles: nothing unexplained, or only that late booking.
	Reconciles bool `json:"reconciles"`
}

// ReconLate is a transaction booked by the bank later than it is dated.
type ReconLate struct {
	TxID     int64       `json:"tx_id"`
	Date     string      `json:"date"`
	Amount   money.Cents `json:"effect"`
	Merchant string      `json:"merchant,omitempty"`
	Note     string      `json:"note,omitempty"`
	// Edge: "opening" — dated before the window, booked inside it;
	// "closing" — dated inside, not booked by the last balance yet.
	Edge string `json:"edge"`
}

// lagDays is how late a bank may book a card payment after its date.
const lagDays = 5

// reconKinds are the accounts whose balance moves only by transactions;
// brokers, pensions, crypto and property also move with prices.
var reconKinds = map[string]bool{"checking": true, "savings": true, "cash": true, "loan": true}

const reconTolerance = money.Cents(100) // €1

// Reconcile compares each cash-like account's recorded balances since
// from with the transactions between them. The window opens on the last
// balance on or before from, so the first stretch is checked too.
func Reconcile(book *wealth.Book, txs []ledger.Tx, from string) []ReconAccount {
	flows := map[string][]ledger.Tx{}
	for _, t := range txs {
		flows[t.AccountID] = append(flows[t.AccountID], t)
		if t.Kind == "transfer" && t.ToAccountID != "" {
			flows[t.ToAccountID] = append(flows[t.ToAccountID], t)
		}
	}
	out := []ReconAccount{}
	for id, a := range book.Accounts {
		if a.Archived || !reconKinds[a.Kind] {
			continue
		}
		all := book.Series(id)
		start := -1
		for i, p := range all {
			if p.Date <= from {
				start = i
			} else if start < 0 {
				start = i
				break
			} else {
				break
			}
		}
		if start < 0 || len(all)-start < 2 {
			continue
		}
		pts := all[start:]
		flowIn := func(after, upTo string) money.Cents {
			var s money.Cents
			for _, t := range flows[id] {
				if t.Date > after && t.Date <= upTo {
					s += signedFor(t, id)
				}
			}
			return s
		}
		r := ReconAccount{AccountID: id, Name: a.Name, Kind: a.Kind, Source: pts[len(pts)-1].Source,
			From: pts[0].Date, To: pts[len(pts)-1].Date, Opening: pts[0].Value, Closing: pts[len(pts)-1].Value, Gaps: []ReconGap{}}
		drift := make([]money.Cents, len(pts)) // running unexplained at each point
		for i := 1; i < len(pts); i++ {
			f := flowIn(pts[i-1].Date, pts[i].Date)
			r.Ledger += f
			diff := pts[i].Value - pts[i-1].Value - f
			drift[i] = drift[i-1] + diff
			r.Intervals++
			if abs(diff) < reconTolerance {
				r.Matched++
			}
		}
		// A stretch whose gaps net back to zero within a few days is booking
		// timing: every gap in it is marked, the opening and the closing one.
		timing := make([]bool, len(pts))
		for i := 1; i < len(pts); i++ {
			if abs(drift[i]-drift[i-1]) < reconTolerance {
				continue
			}
			limit := addDays(pts[i].Date, 4)
			for j := i + 1; j < len(pts) && pts[j].Date <= limit; j++ {
				if abs(drift[j]-drift[i-1]) < reconTolerance {
					for k := i; k <= j; k++ {
						timing[k] = true
					}
					break
				}
			}
		}
		for i := 1; i < len(pts); i++ {
			diff := drift[i] - drift[i-1]
			if abs(diff) < reconTolerance {
				continue
			}
			g := ReconGap{From: pts[i-1].Date, To: pts[i].Date, Balance: pts[i].Value - pts[i-1].Value, Unexplained: diff, Timing: timing[i]}
			g.Ledger = g.Balance - diff
			r.Gaps = append(r.Gaps, g)
		}
		r.Unexplained = drift[len(drift)-1]
		if abs(r.Unexplained) >= reconTolerance {
			r.LateBooking = lateBooking(flows[id], id, r.From, r.To, r.Unexplained)
		}
		if lb := r.LateBooking; lb != nil {
			// The gaps near that edge are the booking showing up: timing.
			for i := range r.Gaps {
				g := &r.Gaps[i]
				if (lb.Edge == "opening" && g.To <= addDays(r.From, lagDays+2)) || (lb.Edge == "closing" && g.From >= addDays(r.To, -lagDays-2)) {
					g.Timing = true
				}
			}
		}
		r.Reconciles = abs(r.Unexplained) < reconTolerance || r.LateBooking != nil
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Reconciles != out[j].Reconciles {
			return !out[i].Reconciles
		}
		if ai, aj := abs(out[i].Unexplained), abs(out[j].Unexplained); ai != aj {
			return ai > aj
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// lateBooking finds the one transaction near an edge of the window that
// accounts for the whole unexplained amount: dated in the days before the
// window opened and booked inside it (it shows as unexplained), or dated in
// the last days and not booked by the last balance (its reverse).
func lateBooking(txs []ledger.Tx, account, open, close string, unexplained money.Cents) *ReconLate {
	for _, t := range txs {
		eff := signedFor(t, account)
		switch {
		case t.Date > addDays(open, -lagDays) && t.Date <= open && abs(eff-unexplained) < reconTolerance:
			return &ReconLate{TxID: t.ID, Date: t.Date, Amount: eff, Merchant: t.Merchant, Note: t.Note, Edge: "opening"}
		case t.Date > addDays(close, -lagDays) && t.Date <= close && abs(eff+unexplained) < reconTolerance:
			return &ReconLate{TxID: t.ID, Date: t.Date, Amount: eff, Merchant: t.Merchant, Note: t.Note, Edge: "closing"}
		}
	}
	return nil
}

// signedFor is a transaction's effect on one account's balance.
func signedFor(t ledger.Tx, account string) money.Cents {
	switch {
	case t.Kind == "income" && t.AccountID == account:
		return t.Amount
	case t.Kind == "expense" && t.AccountID == account:
		return -t.Amount
	case t.Kind == "transfer" && t.ToAccountID == account && t.AccountID == account:
		return 0
	case t.Kind == "transfer" && t.ToAccountID == account:
		return t.Amount
	case t.Kind == "transfer" && t.AccountID == account:
		return -t.Amount
	}
	return 0
}

func abs(c money.Cents) money.Cents {
	if c < 0 {
		return -c
	}
	return c
}
