package insights

import (
	"fmt"
	"time"

	"ft/internal/ledger"
	"ft/internal/money"
)

// PacePoint is cumulative spending by day of month.
type PacePoint struct {
	Day       int          `json:"day"`
	Current   *money.Cents `json:"current,omitempty"` // this month (only up to today)
	LastMonth money.Cents  `json:"last_month"`
	Typical   money.Cents  `json:"typical"` // average of the previous six months
}

// Pace compares this month's cumulative spending with last month and the
// six-month norm, day by day — "am I spending faster than usual?".
func Pace(txs []ledger.Tx, now time.Time) []PacePoint {
	return pace(txs, now, spentAmount)
}

// spentAmount is a row's effect on spending: expenses add, refunds take off.
func spentAmount(t *ledger.Tx) money.Cents {
	switch {
	case IsRefund(t):
		return -t.Amount
	case t.Kind == "expense":
		return t.Amount
	}
	return 0
}

// OutPacePoint is cumulative spending plus net invested by day of month —
// everything that left the cash accounts on purpose. The *_invested fields
// are the invested part of each line, so the chart can split them.
type OutPacePoint struct {
	PacePoint
	CurrentInvested   *money.Cents `json:"current_invested,omitempty"`
	LastMonthInvested money.Cents  `json:"last_month_invested"`
	TypicalInvested   money.Cents  `json:"typical_invested"`
}

// PaceOut is Pace with investing on top of spending: transfers into
// investments, pension, loan principal and assets, net of payouts back to
// cash (a move between two wealth accounts is neutral; transfer.internal is
// not investing). "Have I spent and put away more than usual by today?"
func PaceOut(txs []ledger.Tx, now time.Time) []OutPacePoint {
	spent, inv := pace(txs, now, spentAmount), pace(txs, now, investedAmount)
	out := make([]OutPacePoint, len(spent))
	for i, s := range spent {
		v := inv[i]
		p := OutPacePoint{PacePoint: PacePoint{Day: s.Day, LastMonth: s.LastMonth + v.LastMonth, Typical: s.Typical + v.Typical},
			LastMonthInvested: v.LastMonth, TypicalInvested: v.Typical}
		if s.Current != nil {
			c, ci := *s.Current+*v.Current, *v.Current
			p.Current, p.CurrentInvested = &c, &ci
		}
		out[i] = p
	}
	return out
}

// pace is the day-by-day cumulative of amt for this month (up to today),
// last month and the average of the previous six months.
func pace(txs []ledger.Tx, now time.Time, amt func(*ledger.Tx) money.Cents) []PacePoint {
	month := now.Format("2006-01")
	byMonthDay := map[string]map[int]money.Cents{}
	for i := range txs {
		t := &txs[i]
		a := amt(t)
		if a == 0 {
			continue
		}
		m := t.Date[:7]
		if byMonthDay[m] == nil {
			byMonthDay[m] = map[int]money.Cents{}
		}
		var d int
		fmt.Sscanf(t.Date[8:], "%d", &d)
		byMonthDay[m][d] += a
	}
	cum := func(m string, day int) money.Cents {
		var s money.Cents
		for d := 1; d <= day; d++ {
			s += byMonthDay[m][d]
		}
		return s
	}
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, -1, 0).Format("2006-01")
	var prev []string
	for i := 1; i <= 6; i++ {
		prev = append(prev, first.AddDate(0, -i, 0).Format("2006-01"))
	}
	out := make([]PacePoint, 0, 31)
	for d := 1; d <= 31; d++ {
		p := PacePoint{Day: d, LastMonth: cum(last, d)}
		var s money.Cents
		for _, m := range prev {
			s += cum(m, d)
		}
		p.Typical = s / money.Cents(len(prev))
		if d <= now.Day() {
			c := cum(month, d)
			p.Current = &c
		}
		out = append(out, p)
	}
	return out
}
