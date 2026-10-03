package service

import (
	"math"
	"sort"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

// The budget engine: the one place that turns budget lines + transactions
// into "how am I doing". The Budget page, the home-page pulse card, the AI
// report and the MCP/chat budget tools all read its output, so a fund or a
// yearly line means the same thing everywhere.
//
// Line semantics for Budgeted / Spent / Remaining (what every consumer that
// only reads those three sees):
//   - monthly:        this month's limit, this month's spend, limit − spend
//   - yearly:         the annual amount, spend since 1 January, what's left
//   - fund (either):  available before this month's spending (carry-over +
//                     this month's share), this month's spend, available now

// MonthCell is one month of a line's history.
type MonthCell struct {
	Month  string  `json:"month"`
	Budget float64 `json:"budget"` // the month's share of the line
	Spent  float64 `json:"spent"`
	// FundEnd is the fund's balance after the month (fund lines only).
	FundEnd *float64 `json:"fund_end,omitempty"`
}

// YearProgress tracks a yearly line through its calendar year.
type YearProgress struct {
	Year      int     `json:"year"`
	Budget    float64 `json:"budget"`
	Spent     float64 `json:"spent"`
	Pace      float64 `json:"pace"`      // budget × share of the year elapsed
	Projected float64 `json:"projected"` // spend extrapolated to 31 December
	Elapsed   float64 `json:"elapsed"`   // 0..1
}

// FundState is a fund line's balance for the selected month.
type FundState struct {
	StartMonth   string  `json:"start_month"`
	Opening      float64 `json:"opening"`      // carried over from previous months
	Contribution float64 `json:"contribution"` // this month's share
	Spent        float64 `json:"spent"`        // drawn this month
	Available    float64 `json:"available"`    // opening + contribution − spent
	Contributed  float64 `json:"contributed"`  // since StartMonth, incl. this month
	Drawn        float64 `json:"drawn"`        // since StartMonth, incl. this month
}

// BudgetSuggestion proposes an amount from the line's last 12 complete
// months, sized to its period.
type BudgetSuggestion struct {
	Amount float64 `json:"amount"`
	Median float64 `json:"median"` // monthly
	P75    float64 `json:"p75"`    // monthly
	Mean   float64 `json:"mean"`   // monthly
	Max    float64 `json:"max"`    // monthly
	Months int     `json:"months"`
	// Lumpy: spend comes in bursts, so a monthly cap misfires — a fund fits.
	Lumpy bool   `json:"lumpy"`
	Basis string `json:"basis"`
}

// BudgetLineStatus is one budget line's progress.
type BudgetLineStatus struct {
	ID           uint    `json:"id"`
	Name         string  `json:"name"`
	Kind         string  `json:"kind"` // fixed | investment | spending
	Label        string  `json:"label,omitempty"`
	Category     string  `json:"category,omitempty"`
	Period       string  `json:"period"`
	Fund         bool    `json:"fund"`
	Amount       float64 `json:"amount"`        // effective this month, in its period
	MonthlyShare float64 `json:"monthly_share"` // what it claims of one month
	Budgeted     float64 `json:"budgeted"`
	Spent        float64 `json:"spent"`
	Remaining    float64 `json:"remaining"`
	MonthSpent   float64 `json:"month_spent"`

	Year       *YearProgress     `json:"year,omitempty"`
	FundState  *FundState        `json:"fund_state,omitempty"`
	History    []MonthCell       `json:"history"`
	Suggestion *BudgetSuggestion `json:"suggestion,omitempty"`
}

// UnbudgetedCategory is discretionary spend no spending line covers.
type UnbudgetedCategory struct {
	Category   string            `json:"category"`
	Spent      float64           `json:"spent"`      // selected month
	YearSpent  float64           `json:"year_spent"` // since 1 January
	History    []MonthCell       `json:"history"`
	Suggestion *BudgetSuggestion `json:"suggestion,omitempty"`
}

type BudgetStatusReport struct {
	Month            string             `json:"month"`
	Months           []string           `json:"months"` // the 12 history months, oldest first
	IncomeBase       float64            `json:"income_base,omitempty"`
	IncomeBaseSource string             `json:"income_base_source,omitempty"`
	Lines            []BudgetLineStatus `json:"lines"`
	// Monthly shares: a yearly line counts a twelfth, a fund its accrual.
	FixedPlanned      float64 `json:"fixed_planned"`
	InvestmentPlanned float64 `json:"investment_planned"`
	SpendingPlanned   float64 `json:"spending_planned"`
	// DiscretionarySpent is every expense not claimed by a fixed line.
	DiscretionarySpent float64 `json:"discretionary_spent"`
	// FundContributions is what fund lines set aside this month; FundSpent
	// the part of DiscretionarySpent they paid for out of savings.
	FundContributions float64 `json:"fund_contributions"`
	FundSpent         float64 `json:"fund_spent"`
	// SafeToSpend = income − fixed − investment targets − fund contributions
	// − discretionary spent outside funds. A trip paid from its fund doesn't
	// sink the month: that money was set aside in the months before.
	SafeToSpend *float64             `json:"safe_to_spend,omitempty"`
	Unbudgeted  []UnbudgetedCategory `json:"unbudgeted"`
}

func ymOf(t time.Time) string { return t.Format("2006-01") }

func parseYM(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01", s)
	return t, err == nil
}

func addMonths(ym string, n int) string {
	t, _ := parseYM(ym)
	return ymOf(t.AddDate(0, n, 0))
}

// amountFor resolves a line's amount in a month from its history steps
// (sorted by FromMonth); no steps means the line's own amount everywhere.
func amountFor(b domain.Budget, steps []domain.BudgetAmount, ym string) float64 {
	amount, found := b.Amount, false
	for _, s := range steps {
		if s.FromMonth <= ym {
			amount, found = s.Amount, true
		}
	}
	if !found && len(steps) > 0 {
		// Month before the first dated step and no open-ended step:
		// the earliest known amount is the best answer.
		amount = steps[0].Amount
	}
	return amount
}

func fundStart(b domain.Budget) string {
	if _, ok := parseYM(b.StartMonth); ok {
		return b.StartMonth
	}
	if !b.CreatedAt.IsZero() {
		return ymOf(b.CreatedAt)
	}
	return ""
}

func budgetTypes(kind string) []domain.TransactionType {
	switch kind {
	case "investment":
		return []domain.TransactionType{domain.TransactionTypeInvestment}
	case "fixed":
		// Pension/leasing land as investments but are still fixed costs.
		return []domain.TransactionType{domain.TransactionTypeExpense, domain.TransactionTypeInvestment}
	}
	return []domain.TransactionType{domain.TransactionTypeExpense}
}

func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	pos := q * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	return sorted[lo] + (sorted[hi]-sorted[lo])*(pos-float64(lo))
}

func ceilTo(v, step float64) float64 { return math.Ceil(v/step) * step }

// suggestFrom sizes an amount from a monthly spend series. A steady line
// gets its 75th percentile (tight but reachable); a lumpy one its mean, as a
// fund contribution — the mean is exactly what a fund needs to break even.
func suggestFrom(series []float64, period string, fund bool) *BudgetSuggestion {
	if len(series) < 3 {
		return nil
	}
	sorted := append([]float64(nil), series...)
	sort.Float64s(sorted)
	var sum float64
	zero := 0
	for _, v := range series {
		sum += v
		if v < 0.5 {
			zero++
		}
	}
	if sum < 1 {
		return nil
	}
	s := &BudgetSuggestion{
		Median: quantile(sorted, 0.5), P75: quantile(sorted, 0.75),
		Mean: sum / float64(len(series)), Max: sorted[len(sorted)-1], Months: len(series),
	}
	// Lumpy = a few big months drag the average well above the typical one
	// (or many months are empty). Small lines aren't worth a fund.
	s.Lumpy = (zero >= len(series)/3 || s.Mean > 1.5*math.Max(s.Median, 1)) && s.Mean >= 50
	switch {
	case period == domain.PeriodYearly:
		s.Amount = ceilTo(sum*12/float64(len(series)), 50)
		s.Basis = "last 12 months' total"
	case fund || s.Lumpy:
		s.Amount = ceilTo(s.Mean, 10)
		s.Basis = "monthly average — spend comes in bursts, so a fund carries it"
	default:
		s.Amount = ceilTo(s.P75, 10)
		s.Basis = "75th percentile month — 3 months in 4 fit"
	}
	if s.Amount < 10 {
		return nil // pocket change: not worth a budget line
	}
	return s
}

// ComputeBudgetPlan evaluates every line for `month` (YYYY-MM). `now`
// decides which months are complete and how far through the year we are.
func ComputeBudgetPlan(budgets []domain.Budget, amounts []domain.BudgetAmount, txs []domain.Transaction,
	month string, now time.Time, incomeBase float64, incomeSource string) *BudgetStatusReport {

	steps := map[uint][]domain.BudgetAmount{}
	for _, a := range amounts {
		steps[a.BudgetID] = append(steps[a.BudgetID], a)
	}
	for id := range steps {
		s := steps[id]
		sort.SliceStable(s, func(i, j int) bool { return s[i].FromMonth < s[j].FromMonth })
	}

	var plan []domain.Budget // trip lines live in the Trips view, not the plan
	var fixed []domain.Budget
	for _, b := range budgets {
		if b.Kind == "trip" {
			continue
		}
		if b.Period == "" {
			b.Period = domain.PeriodMonthly
		}
		plan = append(plan, b)
		if b.Kind == "fixed" {
			fixed = append(fixed, b)
		}
	}

	report := &BudgetStatusReport{Month: month, Lines: []BudgetLineStatus{}, Unbudgeted: []UnbudgetedCategory{}}
	for i := 11; i >= 0; i-- {
		report.Months = append(report.Months, addMonths(month, -i))
	}
	selT, _ := parseYM(month)
	year := selT.Year()
	nowYM := ymOf(now)

	// One pass: each line's spend per month, the discretionary pool per
	// month, and per-category unbudgeted spend per month.
	lineSpent := make([]map[string]float64, len(plan))
	for i := range lineSpent {
		lineSpent[i] = map[string]float64{}
	}
	discretionary := map[string]float64{}
	fundSpentBy := map[string]float64{}
	unbudgeted := map[string]map[string]float64{}
	firstYM := ""
	for _, tx := range txs {
		ym := ymOf(tx.Date)
		if ym > month {
			continue
		}
		if firstYM == "" || ym < firstYM {
			firstYM = ym
		}
		isFixed := false
		for _, b := range fixed {
			if budgetMatchesTx(b, tx) {
				isFixed = true
				break
			}
		}
		discretionaryTx := tx.Type == domain.TransactionTypeExpense && !isFixed
		if discretionaryTx {
			discretionary[ym] += tx.Amount
		}
		covered, fundTx := false, false
		for i, b := range plan {
			if !containsType(budgetTypes(b.Kind), tx.Type) || !budgetMatchesTx(b, tx) {
				continue
			}
			// Spending limits measure choices: fixed obligations don't
			// count against them (alimony matches Kids but has its own line).
			if b.Kind == "spending" {
				if !discretionaryTx {
					continue
				}
				covered = true
				if b.Fund {
					fundTx = true
				}
			}
			lineSpent[i][ym] += tx.Amount
		}
		if fundTx {
			fundSpentBy[ym] += tx.Amount
		}
		if discretionaryTx && !covered {
			c := string(tx.Category)
			if unbudgeted[c] == nil {
				unbudgeted[c] = map[string]float64{}
			}
			unbudgeted[c][ym] += tx.Amount
		}
	}

	// Complete months for suggestions: the 12 before the selected month (or
	// before the current one, whichever is earlier), never before history.
	lastComplete := addMonths(month, -1)
	if month >= nowYM {
		lastComplete = addMonths(nowYM, -1)
	}
	var suggestMonths []string
	for i := 11; i >= 0; i-- {
		m := addMonths(lastComplete, -i)
		if firstYM != "" && m >= firstYM {
			suggestMonths = append(suggestMonths, m)
		}
	}

	elapsed := float64(selT.Month()) / 12
	if month == nowYM {
		days := float64(time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day())
		elapsed = (float64(now.Month()-1) + float64(now.Day())/days) / 12
	}

	for i, b := range plan {
		bs := steps[b.ID]
		amount := amountFor(b, bs, month)
		share := b.MonthlyShare(amount)
		spentIn := lineSpent[i]
		line := BudgetLineStatus{
			ID: b.ID, Name: b.Name, Kind: b.Kind, Label: b.Label, Category: b.Category,
			Period: b.Period, Fund: b.Fund && b.Kind == "spending",
			Amount: amount, MonthlyShare: share, MonthSpent: spentIn[month],
		}

		var fundBal float64
		var fundEnd map[string]float64
		if line.Fund {
			start := fundStart(b)
			if start == "" {
				start = month
			}
			fs := &FundState{StartMonth: start}
			fundEnd = map[string]float64{}
			// The balance needs every month since the start, not just the
			// 12 shown.
			if start <= month {
				for m := start; m <= month; m = addMonths(m, 1) {
					c := b.MonthlyShare(amountFor(b, bs, m))
					sp := spentIn[m]
					if m == month {
						fs.Opening = fundBal
						fs.Contribution = c
						fs.Spent = sp
					}
					fundBal = roundCents(fundBal + c - sp)
					fs.Contributed = roundCents(fs.Contributed + c)
					fs.Drawn = roundCents(fs.Drawn + sp)
					fundEnd[m] = fundBal
				}
			}
			fs.Available = fundBal
			line.FundState = fs
			line.Budgeted = roundCents(fs.Opening + fs.Contribution)
			line.Spent = fs.Spent
			line.Remaining = fs.Available
			report.FundContributions += fs.Contribution
		} else if b.Period == domain.PeriodYearly {
			yp := &YearProgress{Year: year, Budget: amount, Elapsed: elapsed}
			for m := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC); ymOf(m) <= month; m = m.AddDate(0, 1, 0) {
				yp.Spent += spentIn[ymOf(m)]
			}
			yp.Pace = amount * elapsed
			if elapsed > 0 {
				yp.Projected = yp.Spent / elapsed
			}
			line.Year = yp
			line.Budgeted = amount
			line.Spent = yp.Spent
			line.Remaining = amount - yp.Spent
		} else {
			line.Budgeted = amount
			line.Spent = spentIn[month]
			line.Remaining = amount - line.Spent
		}

		for _, m := range report.Months {
			cell := MonthCell{Month: m, Budget: b.MonthlyShare(amountFor(b, bs, m)), Spent: spentIn[m]}
			if v, ok := fundEnd[m]; ok {
				v := v
				cell.FundEnd = &v
			}
			line.History = append(line.History, cell)
		}

		if b.Kind == "spending" {
			var series []float64
			for _, m := range suggestMonths {
				series = append(series, spentIn[m])
			}
			if s := suggestFrom(series, b.Period, line.Fund); s != nil {
				// Only worth showing when it moves the line by > 15%.
				if amount <= 0 || math.Abs(s.Amount-amount)/amount > 0.15 || (s.Lumpy && !line.Fund && b.Period == domain.PeriodMonthly) {
					line.Suggestion = s
				}
			}
		}

		switch b.Kind {
		case "fixed":
			report.FixedPlanned += share
		case "investment":
			report.InvestmentPlanned += share
		case "spending":
			report.SpendingPlanned += share
		}
		report.Lines = append(report.Lines, line)
	}

	report.DiscretionarySpent = discretionary[month]
	report.FundSpent = fundSpentBy[month]

	cats := make([]string, 0, len(unbudgeted))
	for c := range unbudgeted {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	for _, c := range cats {
		byM := unbudgeted[c]
		u := UnbudgetedCategory{Category: c, Spent: byM[month]}
		for m := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC); ymOf(m) <= month; m = m.AddDate(0, 1, 0) {
			u.YearSpent += byM[ymOf(m)]
		}
		var recent float64
		for _, m := range report.Months {
			u.History = append(u.History, MonthCell{Month: m, Spent: byM[m]})
			recent += byM[m]
		}
		if u.Spent < 0.5 && recent < 0.5 {
			continue // nothing this year-window: not worth a row
		}
		var series []float64
		for _, m := range suggestMonths {
			series = append(series, byM[m])
		}
		u.Suggestion = suggestFrom(series, domain.PeriodMonthly, false)
		report.Unbudgeted = append(report.Unbudgeted, u)
	}
	sort.SliceStable(report.Unbudgeted, func(i, j int) bool {
		a, b := report.Unbudgeted[i], report.Unbudgeted[j]
		if a.Spent != b.Spent {
			return a.Spent > b.Spent
		}
		return a.YearSpent > b.YearSpent
	})

	if incomeBase > 0 {
		report.IncomeBase = incomeBase
		report.IncomeBaseSource = incomeSource
		safe := incomeBase - report.FixedPlanned - report.InvestmentPlanned - report.FundContributions -
			(report.DiscretionarySpent - report.FundSpent)
		report.SafeToSpend = &safe
	}
	return report
}
