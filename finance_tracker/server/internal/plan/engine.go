package plan

import (
	"math"
	"sort"
	"time"

	"ft/internal/ledger"
	"ft/internal/money"
)

// The budget engine turns budget lines + transactions into "how am I doing".
// Every consumer — the Plan page, the Home pulse, the AI tools — reads this
// one report, so a fund or a yearly line means the same thing everywhere.
//
//   - monthly line: this month's limit, this month's spend, limit − spend
//   - yearly line:  the annual amount, spend since 1 January, what's left
//   - fund:         available before this month's spending (carry-over +
//                   this month's share), this month's spend, available now
//
// Spending lines measure choices: rows claimed by a fixed line (alimony,
// mortgage) never count against a spending envelope.

type MonthCell struct {
	Month   string       `json:"month"`
	Budget  money.Cents  `json:"budget"`
	Spent   money.Cents  `json:"spent"`
	FundEnd *money.Cents `json:"fund_end,omitempty"`
}

type YearProgress struct {
	Year      int         `json:"year"`
	Budget    money.Cents `json:"budget"`
	Spent     money.Cents `json:"spent"`
	Pace      money.Cents `json:"pace"`
	Projected money.Cents `json:"projected"`
	Elapsed   float64     `json:"elapsed"`
}

type FundState struct {
	StartMonth   string      `json:"start_month"`
	Opening      money.Cents `json:"opening"`
	Contribution money.Cents `json:"contribution"`
	Spent        money.Cents `json:"spent"`
	Available    money.Cents `json:"available"`
	Contributed  money.Cents `json:"contributed"`
	Drawn        money.Cents `json:"drawn"`
}

type Suggestion struct {
	Amount money.Cents `json:"amount"`
	Median money.Cents `json:"median"`
	P75    money.Cents `json:"p75"`
	Mean   money.Cents `json:"mean"`
	Max    money.Cents `json:"max"`
	Months int         `json:"months"`
	Lumpy  bool        `json:"lumpy"`
	Basis  string      `json:"basis"`
}

type Line struct {
	Budget
	MonthlyShare money.Cents   `json:"monthly_share"`
	Budgeted     money.Cents   `json:"budgeted"`
	Spent        money.Cents   `json:"spent"`
	Remaining    money.Cents   `json:"remaining"`
	MonthSpent   money.Cents   `json:"month_spent"`
	Year         *YearProgress `json:"year,omitempty"`
	FundState    *FundState    `json:"fund_state,omitempty"`
	History      []MonthCell   `json:"history"`
	Suggestion   *Suggestion   `json:"suggestion,omitempty"`
}

type Unbudgeted struct {
	Category   string      `json:"category"`
	Spent      money.Cents `json:"spent"`
	YearSpent  money.Cents `json:"year_spent"`
	History    []MonthCell `json:"history"`
	Suggestion *Suggestion `json:"suggestion,omitempty"`
}

type Report struct {
	Month              string      `json:"month"`
	Months             []string    `json:"months"`
	IncomeBase         money.Cents `json:"income_base"`
	IncomeBaseSource   string      `json:"income_base_source"`
	IncomeActual       money.Cents `json:"income_actual"`
	Lines              []Line      `json:"lines"`
	FixedPlanned       money.Cents `json:"fixed_planned"`
	SavingPlanned      money.Cents `json:"saving_planned"`
	SpendingPlanned    money.Cents `json:"spending_planned"`
	FixedSpent         money.Cents `json:"fixed_spent"`
	SavedActual        money.Cents `json:"saved_actual"`
	DiscretionarySpent money.Cents `json:"discretionary_spent"`
	FundContributions  money.Cents `json:"fund_contributions"`
	FundSpent          money.Cents `json:"fund_spent"`
	SafeToSpend        money.Cents `json:"safe_to_spend"`
	// Free spending per month (discretionary minus what funds covered) —
	// the same money SafeToSpend draws on. For daily pace and "typical".
	FreeSpentByMonth map[string]money.Cents `json:"-"`
	// The same free spending by day of month ("2026-10" → day → amount), for
	// the month timeline and the typical-month curve.
	FreeSpentByDay map[string]map[int]money.Cents `json:"-"`
	// The same without one-offs (single payments of OneOff or more), for what
	// a typical month looks like: a big family dinner shouldn't set the pace.
	EverydayByDay map[string]map[int]money.Cents `json:"-"`
	Unbudgeted    []Unbudgeted                   `json:"unbudgeted"`
}

// OneOff is the size from which a single payment counts as a one-off rather
// than everyday spending (the cash planner uses the same line).
const OneOff = money.Cents(250 * 100)

func ym(date string) string { return date[:7] }

func dayOf(date string) int {
	d := 0
	for _, c := range date[8:10] {
		d = d*10 + int(c-'0')
	}
	return d
}

func addMonths(m string, n int) string {
	t, _ := time.Parse("2006-01", m)
	return t.AddDate(0, n, 0).Format("2006-01")
}

func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	pos := q * float64(len(sorted)-1)
	lo, hi := int(math.Floor(pos)), int(math.Ceil(pos))
	return sorted[lo] + (sorted[hi]-sorted[lo])*(pos-float64(lo))
}

func ceilTo(v, step float64) float64 { return math.Ceil(v/step) * step }

// suggestFrom sizes an amount from a monthly spend series (euros). A steady
// line gets its 75th percentile; a lumpy one its mean, as a fund.
func suggestFrom(series []float64, period string, fund bool) *Suggestion {
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
	med, p75, mean := quantile(sorted, 0.5), quantile(sorted, 0.75), sum/float64(len(series))
	s := &Suggestion{Median: money.FromFloat(med), P75: money.FromFloat(p75), Mean: money.FromFloat(mean),
		Max: money.FromFloat(sorted[len(sorted)-1]), Months: len(series)}
	s.Lumpy = (zero >= len(series)/3 || mean > 1.5*math.Max(med, 1)) && mean >= 50
	var amt float64
	switch {
	case period == "yearly":
		amt = ceilTo(sum*12/float64(len(series)), 50)
		s.Basis = "last 12 months' total"
	case fund || s.Lumpy:
		amt = ceilTo(mean, 10)
		s.Basis = "monthly average — spend comes in bursts, so a fund carries it"
	default:
		amt = ceilTo(p75, 10)
		s.Basis = "75th-percentile month — 3 months in 4 fit"
	}
	if amt < 10 {
		return nil
	}
	s.Amount = money.FromFloat(amt)
	return s
}

// Compute evaluates every line for month (YYYY-MM).
func Compute(budgets []Budget, txs []ledger.Tx, month string, now time.Time, settings Settings) *Report {
	var lines []Budget
	var fixed []Budget
	for _, b := range budgets {
		if b.Archived {
			continue
		}
		lines = append(lines, b)
		if b.Kind == "fixed" {
			fixed = append(fixed, b)
		}
	}
	r := &Report{Month: month, Lines: []Line{}, Unbudgeted: []Unbudgeted{}}
	for i := 11; i >= 0; i-- {
		r.Months = append(r.Months, addMonths(month, -i))
	}
	sel, _ := time.Parse("2006-01", month)
	year := sel.Year()
	nowYM := now.Format("2006-01")

	spent := make([]map[string]money.Cents, len(lines))
	for i := range spent {
		spent[i] = map[string]money.Cents{}
	}
	discretionary := map[string]money.Cents{}
	freeByDay := map[string]map[int]money.Cents{}
	everydayByDay := map[string]map[int]money.Cents{}
	fundSpent := map[string]money.Cents{}
	fixedSpent := map[string]money.Cents{}
	saved := map[string]money.Cents{}
	income := map[string]money.Cents{}
	unbudgeted := map[string]map[string]money.Cents{}
	firstYM := ""
	salary := settings.Salary()
	for i := range txs {
		t := &txs[i]
		m := ym(t.Date)
		if t.Kind == "income" { // salary counts in the month it pays for
			m = ym(FlowDate(t, salary))
		}
		if m > month {
			continue
		}
		if firstYM == "" || m < firstYM {
			firstYM = m
		}
		if t.Kind == "income" {
			income[m] += t.Amount
			continue
		}
		if t.Kind == "transfer" && (ledger.Top(t.Category) == "transfer") && t.Category != "transfer.internal" {
			saved[m] += t.Amount
		}
		isFixed := false
		for _, b := range fixed {
			if b.Matches(t) {
				isFixed = true
				break
			}
		}
		if isFixed {
			fixedSpent[m] += t.Amount
		}
		disc := t.Kind == "expense" && !isFixed
		if disc {
			discretionary[m] += t.Amount
		}
		covered, fundTx := false, false
		for j, b := range lines {
			if !b.Matches(t) {
				continue
			}
			if b.Kind == "spending" {
				if !disc {
					continue
				}
				covered = true
				if b.Fund {
					fundTx = true
				}
			}
			spent[j][m] += t.Amount
		}
		if fundTx {
			fundSpent[m] += t.Amount
		}
		if disc && !fundTx {
			if freeByDay[m] == nil {
				freeByDay[m] = map[int]money.Cents{}
			}
			freeByDay[m][dayOf(t.Date)] += t.Amount
			if t.Amount < OneOff {
				if everydayByDay[m] == nil {
					everydayByDay[m] = map[int]money.Cents{}
				}
				everydayByDay[m][dayOf(t.Date)] += t.Amount
			}
		}
		if disc && !covered {
			c := ledger.Top(t.Category)
			if unbudgeted[c] == nil {
				unbudgeted[c] = map[string]money.Cents{}
			}
			unbudgeted[c][m] += t.Amount
		}
	}

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
	elapsed := float64(sel.Month()) / 12
	if month == nowYM {
		days := float64(time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day())
		elapsed = (float64(now.Month()-1) + float64(now.Day())/days) / 12
	}

	for i, b := range lines {
		amount := b.AmountFor(month)
		share := b.MonthlyShare(amount)
		sp := spent[i]
		l := Line{Budget: b, MonthlyShare: share, MonthSpent: sp[month]}
		l.Amount = amount
		var fundEnd map[string]money.Cents
		switch {
		case b.Fund:
			start := b.StartMonth
			if start == "" {
				start = month
			}
			fs := &FundState{StartMonth: start}
			fundEnd = map[string]money.Cents{}
			var bal money.Cents
			for m := start; m <= month; m = addMonths(m, 1) {
				c := b.MonthlyShare(b.AmountFor(m))
				if m == month {
					fs.Opening, fs.Contribution, fs.Spent = bal, c, sp[m]
				}
				bal += c - sp[m]
				fs.Contributed += c
				fs.Drawn += sp[m]
				fundEnd[m] = bal
			}
			fs.Available = bal
			l.FundState = fs
			l.Budgeted = fs.Opening + fs.Contribution
			l.Spent = fs.Spent
			l.Remaining = fs.Available
			r.FundContributions += fs.Contribution
		case b.Period == "yearly":
			yp := &YearProgress{Year: year, Budget: amount, Elapsed: elapsed}
			for m := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC); m.Format("2006-01") <= month; m = m.AddDate(0, 1, 0) {
				yp.Spent += sp[m.Format("2006-01")]
			}
			yp.Pace = money.FromFloat(amount.Float() * elapsed)
			if elapsed > 0 {
				yp.Projected = money.FromFloat(yp.Spent.Float() / elapsed)
			}
			l.Year = yp
			l.Budgeted, l.Spent, l.Remaining = amount, yp.Spent, amount-yp.Spent
		default:
			l.Budgeted, l.Spent, l.Remaining = amount, sp[month], amount-sp[month]
		}
		for _, m := range r.Months {
			cell := MonthCell{Month: m, Budget: b.MonthlyShare(b.AmountFor(m)), Spent: sp[m]}
			if v, ok := fundEnd[m]; ok {
				v := v
				cell.FundEnd = &v
			}
			l.History = append(l.History, cell)
		}
		if b.Kind == "spending" {
			var series []float64
			for _, m := range suggestMonths {
				series = append(series, sp[m].Float())
			}
			if s := suggestFrom(series, b.Period, b.Fund); s != nil {
				if amount <= 0 || math.Abs(s.Amount.Float()-amount.Float())/amount.Float() > 0.15 || (s.Lumpy && !b.Fund && b.Period == "monthly") {
					l.Suggestion = s
				}
			}
		}
		switch b.Kind {
		case "fixed":
			r.FixedPlanned += share
		case "saving":
			r.SavingPlanned += share
		case "spending":
			r.SpendingPlanned += share
		}
		r.Lines = append(r.Lines, l)
	}
	r.DiscretionarySpent = discretionary[month]
	r.FundSpent = fundSpent[month]
	r.FreeSpentByDay = freeByDay
	r.EverydayByDay = everydayByDay
	r.FreeSpentByMonth = map[string]money.Cents{}
	for m, v := range discretionary {
		r.FreeSpentByMonth[m] = v - fundSpent[m]
	}
	r.FixedSpent = fixedSpent[month]
	r.SavedActual = saved[month]
	r.IncomeActual = income[month]

	cats := make([]string, 0, len(unbudgeted))
	for c := range unbudgeted {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	for _, c := range cats {
		byM := unbudgeted[c]
		u := Unbudgeted{Category: c, Spent: byM[month]}
		for m := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC); m.Format("2006-01") <= month; m = m.AddDate(0, 1, 0) {
			u.YearSpent += byM[m.Format("2006-01")]
		}
		var recent money.Cents
		for _, m := range r.Months {
			u.History = append(u.History, MonthCell{Month: m, Spent: byM[m]})
			recent += byM[m]
		}
		if u.Spent == 0 && recent == 0 {
			continue
		}
		var series []float64
		for _, m := range suggestMonths {
			series = append(series, byM[m].Float())
		}
		u.Suggestion = suggestFrom(series, "monthly", false)
		r.Unbudgeted = append(r.Unbudgeted, u)
	}
	sort.SliceStable(r.Unbudgeted, func(i, j int) bool {
		if r.Unbudgeted[i].Spent != r.Unbudgeted[j].Spent {
			return r.Unbudgeted[i].Spent > r.Unbudgeted[j].Spent
		}
		return r.Unbudgeted[i].YearSpent > r.Unbudgeted[j].YearSpent
	})

	r.IncomeBase, r.IncomeBaseSource = IncomeBase(settings, income, month, nowYM)
	r.SafeToSpend = r.IncomeBase - r.FixedPlanned - r.SavingPlanned - r.FundContributions - (r.DiscretionarySpent - r.FundSpent)
	return r
}

// IncomeBase is the monthly income the plan measures against.
func IncomeBase(s Settings, income map[string]money.Cents, month, nowYM string) (money.Cents, string) {
	switch s.IncomeMode {
	case "manual":
		if s.ManualIncome > 0 {
			return money.FromFloat(s.ManualIncome), "manual"
		}
	case "gross":
		if s.GrossSalary > 0 {
			return money.FromFloat(LTNetSalary(s.GrossSalary, s.MonthlyDeductions)), "gross salary, net of LT taxes"
		}
	}
	last := addMonths(month, -1)
	if month > nowYM {
		last = addMonths(nowYM, -1)
	}
	var v []float64
	for i := 0; i < 12; i++ {
		if x := income[addMonths(last, -i)]; x > 0 {
			v = append(v, x.Float())
		}
	}
	return money.FromFloat(median(v)), "median of the last 12 complete months"
}
