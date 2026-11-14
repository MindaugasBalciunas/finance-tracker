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
		if Essential(l.Budget, cats) {
			f.Needs.add(l.Name, budgeted, spent)
		} else {
			f.Wants.add(l.Name, budgeted, spent)
		}
	}
	return f
}
