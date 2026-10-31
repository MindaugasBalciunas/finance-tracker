package cfo

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"ft/internal/goals"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
)

// The payday runbook turns "pay yourself first" into the transfers to make
// once the salary is in: each saving line with its amount, the account it
// usually leaves from and the one it goes to, then the wish list's share of
// last month's leftovers and any top-up a bill needs this week. Each step
// ticks itself off when the bank shows the transfer, so the list only ever
// asks for what is still missing. Read-only: it never moves money.

type RunStep struct {
	Key      string      `json:"key"`
	Kind     string      `json:"kind"` // saving | wishlist | move
	Title    string      `json:"title"`
	Detail   string      `json:"detail,omitempty"`
	From     string      `json:"from,omitempty"`
	FromName string      `json:"from_name,omitempty"`
	To       string      `json:"to,omitempty"`
	ToName   string      `json:"to_name,omitempty"`
	Amount   money.Cents `json:"amount"`
	Done     money.Cents `json:"done"`
	Status   string      `json:"status"` // done | partial | todo
	By       string      `json:"by,omitempty"`
	Link     string      `json:"link,omitempty"`
}

type Runbook struct {
	Month    string      `json:"month"`    // the calendar month the transfers belong to
	Payday   string      `json:"payday"`   // when this month's salary landed ("" = not yet)
	Expected string      `json:"expected"` // when the next one is due, by the salary rules
	Salary   money.Cents `json:"salary"`
	Steps    []RunStep   `json:"steps"`
	Total    money.Cents `json:"total"`
	DoneSum  money.Cents `json:"done_total"`
	Open     int         `json:"open"` // steps not done yet
}

// BuildRunbook is this calendar month's checklist.
func BuildRunbook(d *sql.DB, now time.Time) (*Runbook, error) { return buildRunbook(d, now, true, nil) }

// PaydaySteps is the checklist without this week's bill top-ups — what the
// Budget lines and Home's payday tile need, without building Home's cash plan.
func PaydaySteps(d *sql.DB, now time.Time) (*Runbook, error) { return buildRunbook(d, now, false, nil) }

// withMoves=false leaves out the cash top-ups, for Home, which shows those
// itself (and builds the cash plan the runbook would otherwise ask it for).
// cur is this month's plan report when the caller already has it.
func buildRunbook(d *sql.DB, now time.Time, withMoves bool, cur *plan.Report) (*Runbook, error) {
	today := now.Format("2006-01-02")
	settings := plan.LoadSettings(d)
	rules := settings.Salary()
	txs, err := ledger.All(d, ledger.Filter{From: now.AddDate(0, -7, 0).Format("2006-01-02"), To: today})
	if err != nil {
		return nil, err
	}
	accts, err := ledger.ListAccounts(d)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, a := range accts {
		names[a.ID] = a.Name
	}
	rb := &Runbook{Month: now.Format("2006-01"), Expected: nextPayday(rules, now), Steps: []RunStep{}}

	// Payday is this month's first salary payment; a salary paid in parts
	// (1 Oct and 9 Oct) adds up. Steps count over the whole calendar month,
	// as the plan's saving lines do: an ETF top-up on 2 Oct is done even
	// though a second salary part came on the 9th.
	month := now.Format("2006-01")
	for _, t := range txs {
		if t.Kind == "income" && ledger.Top(t.Category) == "salary" && !t.Pending && t.Date[:7] == month {
			if rb.Payday == "" || t.Date < rb.Payday {
				rb.Payday = t.Date
			}
			rb.Salary += t.Amount
		}
	}
	// The bill top-ups below come from Home's cash plan; building Home also
	// yields this month's report.
	var o *Overview
	if withMoves {
		if ov, err := buildOverview(d, now, 0, &cur); err == nil {
			o = ov
		}
	}
	r := cur
	if r == nil {
		if r, err = BudgetReport(d, rb.Month, now); err != nil {
			return nil, err
		}
	}

	for _, l := range r.Lines {
		if l.Kind != "saving" || l.Archived {
			continue
		}
		amt := l.Budgeted
		if amt <= 0 {
			continue
		}
		// Done is the plan line's own figure for the month: money sold back out
		// of a broker or pension counts against it, a rebalance between two of
		// them not at all — a €500 ETF sale must not tick off the ETF step.
		st := RunStep{Key: fmt.Sprintf("saving:%d", l.ID), Kind: "saving", Title: l.Name, Amount: amt, Done: max(0, l.Spent), Link: "/plan"}
		// Where this line's money usually goes, and from which account: only
		// rows moving money in (a sale back to Swedbank would flip the route).
		from, to := map[string]int{}, map[string]int{}
		for i := range txs {
			t := &txs[i]
			if !l.Matches(t) || (t.Category != "transfer.internal" && t.WealthDirection() <= 0) {
				continue
			}
			from[t.AccountID]++
			to[t.ToAccountID]++
		}
		st.From, st.To = mostUsed(from), mostUsed(to)
		if st.From == "" {
			st.From = settings.SalaryAccount
		}
		st.FromName, st.ToName = names[st.From], names[st.To]
		rb.add(st)
	}

	// The wish list: last month's leftovers, by priority.
	if gp, err := goals.Build(d, "", now); err == nil && gp != nil {
		var proposed money.Cents
		for _, a := range gp.Proposal {
			proposed += a.Amount
		}
		if want := proposed + gp.Funding.Funded; want > 0 {
			st := RunStep{Key: "wishlist:" + gp.Funding.Month, Kind: "wishlist", Title: "Fund the wish list", Amount: want, Done: gp.Funding.Funded, Link: "/plan/wishlist",
				Detail: fmt.Sprintf("%s left over from %s, split by priority.", eur(gp.Funding.LeftOver), monthName(gp.Funding.Month))}
			rb.add(st)
		}
	}

	// Top-ups a bill needs within the week: the cash plan already knows them.
	if o != nil && o.Cash != nil {
		week := now.AddDate(0, 0, 7).Format("2006-01-02")
		for i, m := range o.Cash.Moves {
			if m.By > week {
				continue
			}
			rb.add(RunStep{Key: fmt.Sprintf("move:%d:%s", i, m.By), Kind: "move", Title: "Top up " + m.ToName, Detail: m.For,
				From: m.From, FromName: m.FromName, To: m.To, ToName: m.ToName, Amount: m.Amount, By: m.By, Link: "/"})
		}
	}
	sort.SliceStable(rb.Steps, func(i, j int) bool { return rank(rb.Steps[i].Status) < rank(rb.Steps[j].Status) })
	return rb, nil
}

func (rb *Runbook) add(st RunStep) {
	switch {
	case st.Done >= st.Amount:
		st.Status = "done"
	case st.Done > 0:
		st.Status = "partial"
	default:
		st.Status = "todo"
	}
	rb.Total += st.Amount
	rb.DoneSum += min(st.Done, st.Amount)
	if st.Status != "done" {
		rb.Open++
	}
	rb.Steps = append(rb.Steps, st)
}

// Open steps first, finished ones sink to the bottom.
func rank(status string) int {
	switch status {
	case "partial":
		return 0
	case "todo":
		return 1
	}
	return 2
}

func mostUsed(n map[string]int) string {
	best, id := 0, ""
	for k, c := range n {
		if k != "" && (c > best || (c == best && k < id)) {
			best, id = c, k
		}
	}
	return id
}
