package cfo

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"time"

	"ft/internal/goals"
	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

// Where should the next euro go? Each place money can go is scored by what a
// euro there earns after tax, in the owner's own numbers: the buffer first
// while it is short (safety before return), the 3rd pillar while this year's
// 20% refund has room, extra mortgage principal at the loan's own rate, and
// investing at the expected return minus a margin for not being certain.
// A sum is then split down that list, each place taking what it has room
// for. Read-only: a suggestion, never a move.

type EuroOption struct {
	Key    string       `json:"key"` // buffer | pillar3 | loan:<id> | invest | goal:<id>
	Title  string       `json:"title"`
	Why    string       `json:"why"`
	Return float64      `json:"return"` // % a year, after tax, as scored
	Sure   bool         `json:"sure"`   // a known rate, not an expectation
	Room   *money.Cents `json:"room,omitempty"`
	Amount money.Cents  `json:"amount"` // this option's share of the sum
	To     string       `json:"to,omitempty"`
	ToName string       `json:"to_name,omitempty"`
	Link   string       `json:"link,omitempty"`
}

type NextEuro struct {
	Sum     money.Cents  `json:"sum"`
	Options []EuroOption `json:"options"` // best first
	Notes   []string     `json:"notes"`
}

// investMargin: points taken off the expected return for it not being
// certain — a guaranteed 3.9% on the mortgage beats an expected 5% that
// could be −20% in a bad year.
const investMargin = 2.0

// inflation turns the plan's real expected return into a nominal one that
// can stand next to a loan rate.
const inflation = 2.5

func NextEuroFor(d *sql.DB, sum money.Cents, now time.Time) (*NextEuro, error) {
	if sum <= 0 {
		sum = money.FromFloat(500)
	}
	settings := plan.LoadSettings(d)
	book, err := wealth.LoadBook(d)
	if err != nil {
		return nil, err
	}
	cats, err := ledger.CategoryMap(d)
	if err != nil {
		return nil, err
	}
	txs, err := ledger.All(d, ledger.Filter{From: now.AddDate(-1, -1, 0).Format("2006-01-02"), To: now.Format("2006-01-02")})
	if err != nil {
		return nil, err
	}
	out := &NextEuro{Sum: sum, Options: []EuroOption{}, Notes: []string{}}
	var opts []EuroOption
	room := func(c money.Cents) *money.Cents { return &c }

	// 1. The buffer: below target, cash comes first whatever it earns.
	em := insights.ComputeFI(txs, cats, book, settings, now).Emergency
	if gap := em.Target - em.Cash; gap > 0 {
		opts = append(opts, EuroOption{Key: "buffer", Title: "Emergency buffer", Return: math.Inf(1), Sure: true, Room: room(gap), Link: "/wealth/fi",
			Why: fmt.Sprintf("Cash covers %.1f of the %.0f months you aim for — %s short. Safety comes before return.", em.Months, em.TargetMonths, eur(gap))})
	}

	// 2. The 3rd pillar while this year's refund has room.
	ts := LoadTaxSettings(d)
	year := now.Year()
	var paid money.Cents
	pension := ""
	for _, t := range txs {
		if t.Category == "transfer.pension" && t.WealthDirection() > 0 && t.Date[:4] == fmt.Sprint(year) {
			paid += t.Amount
			pension = t.ToAccountID
		}
	}
	gross := settings.GrossFor(now.Format("2006-01")) * 12
	cap := money.FromFloat(pillar3Max)
	if gross > 0 {
		cap = min(cap, money.FromFloat(gross*0.25))
	}
	switch {
	case ts.Pillar3Before2025 != nil && *ts.Pillar3Before2025 && cap-paid > 0:
		opts = append(opts, EuroOption{Key: "pillar3", Title: "3rd-pillar pension", Return: pillar3Refund*100 + settings.ExpectedReturn + inflation - investMargin, Sure: false, Room: room(cap - paid),
			To: pension, ToName: accountName(book, pension), Link: "/wealth/tax",
			Why: fmt.Sprintf("20%% comes back with the declaration on the next %s (%s of %s paid in %d) — then it grows like any investment, locked until retirement.", eur(cap-paid), eur(paid), eur(cap), year)})
	case ts.Pillar3Before2025 != nil && *ts.Pillar3Before2025:
		out.Notes = append(out.Notes, fmt.Sprintf("This year's 3rd-pillar refund room is used: %s paid in %d, and the refund counts up to %s. It opens again in January.", eur(paid), year, eur(cap)))
	case ts.Pillar3Before2025 == nil && paid > 0:
		out.Notes = append(out.Notes, "Tell the app whether your 3rd-pillar contract predates 2025 (Wealth → Taxes): if it does, its 20% refund would likely come first here.")
	}

	// 3. Loans: every euro of principal earns the loan's rate, guaranteed.
	for _, l := range wealth.Loans(book, txs, now) {
		if l.Balance <= 0 || l.Rate <= 0 {
			continue
		}
		why := fmt.Sprintf("Pays down %s owed at %.2f%% — a sure %.2f%% a year, and %s of interest left on the schedule.", eur(l.Balance), l.Rate, l.Rate, eur(l.TotalInterest))
		if l.DaysToReset > 0 && l.DaysToReset < 120 {
			why += fmt.Sprintf(" The rate resets in %d days.", l.DaysToReset)
		}
		opts = append(opts, EuroOption{Key: "loan:" + l.AccountID, Title: "Extra principal on " + l.Name, Return: l.Rate, Sure: true, Room: room(l.Balance),
			To: l.AccountID, Link: "/wealth/loans", Why: why}) // the title names the loan already
	}

	// 4. Investing, at the plan's expected return made nominal, less the margin.
	inv := settings.ExpectedReturn + inflation
	broker := ""
	for _, t := range txs {
		if t.Category == "transfer.invest" && t.WealthDirection() > 0 {
			broker = t.ToAccountID
		}
	}
	opts = append(opts, EuroOption{Key: "invest", Title: "Invest", Return: inv - investMargin, To: broker, ToName: accountName(book, broker), Link: "/wealth/investments",
		Why: fmt.Sprintf("Expected about %.1f%% a year (your %.1f%% real plus %.1f%% inflation), scored %.1f points lower because it isn't certain.", inv, settings.ExpectedReturn, inflation, investMargin)})

	sort.SliceStable(opts, func(i, j int) bool { return opts[i].Return > opts[j].Return })

	// The wish list doesn't earn, but a goal about to miss its date can't
	// wait for returns: it goes right after the buffer.
	if gp, err := goals.Build(d, "", now); err == nil {
		for _, g := range gp.Goals {
			if g.Status == "active" && g.Outlook != nil && g.Outlook.Tone != "good" && g.TargetDate != "" && g.Remaining > 0 {
				when := g.TargetDate
				if t, err := time.Parse("2006-01-02", g.TargetDate); err == nil {
					when = t.Format("January 2006")
				}
				o := EuroOption{Key: fmt.Sprintf("goal:%d", g.ID), Title: g.Name, Room: room(g.Remaining), Link: "/plan/wishlist",
					Why: fmt.Sprintf("Due %s and running late — %s still to save. Money now keeps the date.", when, eur(g.Remaining))}
				at := 0
				if len(opts) > 0 && opts[0].Key == "buffer" {
					at = 1
				}
				opts = append(opts[:at], append([]EuroOption{o}, opts[at:]...)...)
				break
			}
		}
	}

	left := sum
	for i := range opts {
		take := left
		if opts[i].Room != nil {
			take = min(left, *opts[i].Room)
		}
		opts[i].Amount = take
		left -= take
		if math.IsInf(opts[i].Return, 1) {
			opts[i].Return = 0
		}
	}
	out.Options = opts
	return out, nil
}

func accountName(b *wealth.Book, id string) string {
	if a, ok := b.Accounts[id]; ok {
		return a.Name
	}
	return ""
}
