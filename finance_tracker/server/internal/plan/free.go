package plan

import (
	"ft/internal/ledger"
	"ft/internal/money"
)

// The free budget is this month's spending lines — everything the plan lets
// you choose, with fixed obligations (loan, alimony) and saving left out —
// split into needs and wants by the categories' own essential flag, so
// "how much of my budget is non-essential" follows the categories the owner
// marks in Settings rather than a second list kept here.

// FreePart is one slice of the free budget for the month.
type FreePart struct {
	Budgeted money.Cents `json:"budgeted"`
	Spent    money.Cents `json:"spent"`
	Left     money.Cents `json:"left"` // budgeted − spent; negative = over
	Lines    []string    `json:"lines"`
}

// FreeBudget is the month's free budget, with its needs and wants.
type FreeBudget struct {
	FreePart
	Needs FreePart `json:"needs"`
	Wants FreePart `json:"wants"`
	// InFunds is the part of Left still held by fund lines (Travel, Gifts…):
	// money set aside for lumpy costs, which Left to spend took out on the
	// 1st. Left − InFunds is what the day-to-day lines have left, so the
	// card and Left to spend can be read side by side (a €1,434 budget left
	// was €791 in funds and €642 day-to-day, beside €810 left to spend).
	InFunds money.Cents `json:"in_funds"`
	// FundSpent is what the funds paid this month out of what they held —
	// in Spent here, but not in Left to spend's "spent so far" (a payment
	// beyond what a fund held is, so it is left out here too).
	FundSpent money.Cents `json:"fund_spent"`
}

func (p *FreePart) add(name string, budgeted, spent money.Cents) {
	p.Budgeted += budgeted
	p.Spent += spent
	p.Left = p.Budgeted - p.Spent
	p.Lines = append(p.Lines, name)
}

// Essential reports whether a line is a need: it names categories and every
// one of them is marked essential. A line mixing in a non-essential category
// ("Leisure & shopping"), or one that only follows a tag, is a want — the
// money it holds can be cut.
func Essential(b Budget, cats map[string]ledger.Category) bool {
	if len(b.Categories) == 0 {
		return false
	}
	for _, c := range b.Categories {
		if !cats[c].Essential {
			return false
		}
	}
	return true
}

// Free sums the report's spending lines for its month. A monthly line uses
// the figures the Plan page shows for it. A fund counts what it holds this
// month — carry-over plus this month's share — and what it paid; an overdrawn
// fund holds just this month's share, because last month's overdraft already
// came out of last month's free money and must not shrink this one (a −€500
// Travel fund would otherwise put −€200 into November's budget). A yearly
// line counts its monthly share against this month's spend, so a
// €1,200-a-year line adds €100, not the whole year, to one month's budget.
func Free(r *Report, cats map[string]ledger.Category) FreeBudget {
	f := FreeBudget{FreePart: FreePart{Lines: []string{}}, Needs: FreePart{Lines: []string{}}, Wants: FreePart{Lines: []string{}}}
	for _, l := range r.Lines {
		if l.Kind != "spending" {
			continue
		}
		budgeted, spent := l.Budgeted, l.Spent
		switch fs := l.FundState; {
		case fs != nil:
			budgeted, spent = max(fs.Opening, 0)+fs.Contribution, fs.Spent
		case l.Period == "yearly":
			budgeted, spent = l.MonthlyShare, l.MonthSpent
		}
		f.FreePart.add(l.Name, budgeted, spent)
		if l.FundState != nil {
			// A fund that paid beyond what it held keeps nothing; the
			// overdraft is free spending, as in Left to spend (FundOverdrawn).
			f.InFunds += max(budgeted-spent, 0)
			f.FundSpent += min(spent, max(budgeted, 0))
		}
		if Essential(l.Budget, cats) {
			f.Needs.add(l.Name, budgeted, spent)
		} else {
			f.Wants.add(l.Name, budgeted, spent)
		}
	}
	return f
}

// FreeMoney is the month read from the income: what is free once fixed
// obligations and saving are planned, what was spent of it — split into
// essential and optional by each payment's own category, so a restaurant
// under the Food line counts as optional and groceries as essential — and
// what is left, of which some is kept in funds for lumpy costs and the rest
// is left to spend (the same figure as Left to spend on Home and Plan).
//
//	Free = Income − Fixed − Saving
//	Spent = Essential + Optional + Unreviewed − Refunds
//	Free − Spent = InFunds + LeftToSpend
//
// InFunds is this month's fund set-asides less what funds paid out of them.
// It goes negative when a fund pays from money saved in earlier months (a
// €1,200 flight from a Travel fund that held €1,100 before this month): that
// money adds to this month's, it doesn't come out of it.
type FreeMoney struct {
	Income      money.Cents `json:"income"`
	Fixed       money.Cents `json:"fixed"`
	Saving      money.Cents `json:"saving"`
	Free        money.Cents `json:"free"`
	Spent       money.Cents `json:"spent"`
	Essential   money.Cents `json:"essential"`
	Optional    money.Cents `json:"optional"`
	Refunds     money.Cents `json:"refunds"`       // money back, taken off spent
	Unreviewed  money.Cents `json:"unreviewed"`    // still in the bank inbox, category not settled
	InFunds     money.Cents `json:"in_funds"`      // see above; negative = paid from earlier months' fund money
	LeftToSpend money.Cents `json:"left_to_spend"` // = Report.SafeToSpend
}

// FreeMoneyOf reads a report from the income's side.
func FreeMoneyOf(r *Report, cats map[string]ledger.Category) FreeMoney {
	f := FreeMoney{Income: r.IncomeBase, Fixed: r.FixedPlanned, Saving: r.SavingPlanned,
		Spent: r.DiscretionarySpent, Refunds: r.Refunds, Unreviewed: r.Unreviewed, LeftToSpend: r.SafeToSpend}
	f.Free = f.Income - f.Fixed - f.Saving
	for c, v := range r.SpentByCategory {
		if cats[c].Essential {
			f.Essential += v
		} else {
			f.Optional += v
		}
	}
	f.InFunds = f.Free - f.Spent - f.LeftToSpend
	return f
}

// FixedMatches are what the fixed lines claim (loan, alimony): rows they
// match are obligations, never part of the month's free spending, so the
// Ledger's essential/optional view leaves them out as the Free money card
// does — categories and tag together, as Budget.Matches reads them.
func FixedMatches(budgets []Budget) []ledger.Match {
	out := []ledger.Match{}
	for _, b := range budgets {
		if b.Kind == "fixed" && !b.Archived && (len(b.Categories) > 0 || b.Tag != "") {
			out = append(out, ledger.Match{Categories: b.Categories, Tag: b.Tag})
		}
	}
	return out
}
