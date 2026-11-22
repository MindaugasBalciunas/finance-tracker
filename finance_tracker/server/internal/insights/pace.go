package insights

import (
	"fmt"
	"sort"
	"strings"
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

// IncomeUse is how this month's income was used so far: spent, invested
// (by the wealth account it went into or came back out of) and what is
// left. Income, spending and invested are the month's cash-flow figures,
// so they match Cash flow; pay still due for the month (ExpectedSalary)
// counts toward what's left, as Cash flow's lighter bars do.
type IncomeUse struct {
	Month          string       `json:"month"`
	Income         money.Cents  `json:"income"`
	ExpectedSalary money.Cents  `json:"expected_salary"`
	Spent          money.Cents  `json:"spent"`
	Invested       money.Cents  `json:"invested"`
	PayrollPension money.Cents  `json:"payroll_pension"` // pension paid by payroll: never part of income, shown apart
	Left           money.Cents  `json:"left"`
	Into           []InvestedTo `json:"into"`
}

// InvestedTo is the month's net invested into one wealth account; negative
// when more came back out of it than went in.
type InvestedTo struct {
	AccountID string      `json:"account_id"` // "" for a payout whose wealth account the bank row doesn't name
	Name      string      `json:"name"`
	Kind      string      `json:"kind"`
	Amount    money.Cents `json:"amount"`
}

// MonthIncomeUse splits f (this month's cash flow, with ExpectedSalary
// filled) by account. names maps account id → account for display.
func MonthIncomeUse(f Flow, txs []ledger.Tx, names map[string]ledger.Account) IncomeUse {
	out := IncomeUse{Month: f.Period, Income: f.Income, ExpectedSalary: f.ExpectedSalary, Spent: f.Spending, Invested: f.Invested,
		PayrollPension: f.PayrollPension, Into: []InvestedTo{}}
	out.Left = f.Income + f.ExpectedSalary - f.Spending - f.Invested
	by := map[string]*InvestedTo{}
	for i := range txs {
		t := &txs[i]
		if !strings.HasPrefix(t.Date, f.Period) {
			continue
		}
		amt := investedAmount(t)
		if amt == 0 {
			continue
		}
		// Money in is credited to where it went; a payout to where it came from.
		// A bank row may name no wealth account at all: money in and money
		// out then stay apart, so neither hides inside the other.
		id, kind := t.ToAccountID, t.ToKind
		if amt < 0 {
			id, kind = t.AccountID, t.FromKind
		}
		key := id
		if id == "" {
			key = fmt.Sprintf("|%t", amt > 0)
		}
		x := by[key]
		if x == nil {
			x = &InvestedTo{AccountID: id, Kind: kind, Name: names[id].Name}
			switch {
			case id == "" && amt > 0:
				x.Name = "Not linked to an account"
			case id == "":
				x.Name = "Paid back out"
			case x.Name == "":
				x.Name = "Removed account" // rows kept after the account was deleted
			}
			by[key] = x
		}
		x.Amount += amt
	}
	for _, x := range by {
		if x.Amount != 0 {
			out.Into = append(out.Into, *x)
		}
	}
	sort.Slice(out.Into, func(i, j int) bool {
		if out.Into[i].Amount != out.Into[j].Amount {
			return out.Into[i].Amount > out.Into[j].Amount
		}
		return out.Into[i].Name < out.Into[j].Name
	})
	return out
}
