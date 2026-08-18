package service

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/marketdata"
)

// This file adds the daily-status sections of the AI data report: this
// month's spending so far, live budget status (mirroring the frontend
// computeMonthPlan), and stock positions priced against Yahoo Finance.

// currentMonthSection summarizes spending/income/investing so far this
// calendar month and projects the month-end spend from the run rate.
func (s *insightService) currentMonthSection(allTxs []domain.Transaction, now time.Time) string {
	y, m := now.Year(), int(now.Month())
	var income, expenses, invested float64
	for _, tx := range allTxs {
		if tx.Date.Year() != y || int(tx.Date.Month()) != m {
			continue
		}
		switch tx.Type {
		case domain.TransactionTypeIncome:
			income += tx.Amount
		case domain.TransactionTypeExpense:
			expenses += tx.Amount
		case domain.TransactionTypeInvestment:
			invested += tx.Amount
		}
	}
	dayOfMonth := now.Day()
	daysInMonth := time.Date(y, now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	var perDay, projected float64
	if dayOfMonth > 0 {
		perDay = expenses / float64(dayOfMonth)
		projected = perDay * float64(daysInMonth)
	}
	return fmt.Sprintf(`=== THIS MONTH SO FAR (%s, day %d of %d) ===
Income received:  €%.0f
Spent:            €%.0f
Invested:         €%.0f
Net so far:       €%.0f
Avg daily spend:  €%.1f/day
Projected month-end spend: €%.0f (at the current daily rate)`,
		now.Format("2006-01"), dayOfMonth, daysInMonth,
		income, expenses, invested, income-expenses-invested, perDay, projected)
}

// budgetStatusSection mirrors the frontend computeMonthPlan: per-budget
// spent-vs-target for the current month, plus the safe-to-spend figure.
func (s *insightService) budgetStatusSection(allTxs []domain.Transaction, summary *domain.TransactionSummary, now time.Time) string {
	if s.budgetRepo == nil {
		return ""
	}
	budgets, err := s.budgetRepo.ListBudgets()
	if err != nil || len(budgets) == 0 {
		return ""
	}
	settings, _ := s.budgetRepo.GetSettings()

	y, m := now.Year(), int(now.Month())
	var monthTxs []domain.Transaction
	for _, tx := range allTxs {
		if tx.Date.Year() == y && int(tx.Date.Month()) == m {
			monthTxs = append(monthTxs, tx)
		}
	}

	var fixed, investment, spending []domain.Budget
	for _, b := range budgets {
		switch b.Kind {
		case "fixed":
			fixed = append(fixed, b)
		case "investment":
			investment = append(investment, b)
		case "spending":
			spending = append(spending, b)
		}
	}

	// Discretionary = this month's expenses not already claimed by a fixed
	// obligation (loan/alimony/pension etc. are commitments, not choices).
	isFixedTx := func(tx domain.Transaction) bool {
		for _, b := range fixed {
			if budgetMatchesTx(b, tx) {
				return true
			}
		}
		return false
	}
	var discretionary []domain.Transaction
	var discretionarySpent float64
	for _, tx := range monthTxs {
		if tx.Type == domain.TransactionTypeExpense && !isFixedTx(tx) {
			discretionary = append(discretionary, tx)
			discretionarySpent += tx.Amount
		}
	}

	spent := func(b domain.Budget) float64 {
		var wantTypes []domain.TransactionType
		switch b.Kind {
		case "investment":
			wantTypes = []domain.TransactionType{domain.TransactionTypeInvestment}
		case "fixed":
			// Pension/leasing land as investments but are still fixed costs.
			wantTypes = []domain.TransactionType{domain.TransactionTypeExpense, domain.TransactionTypeInvestment}
		default:
			wantTypes = []domain.TransactionType{domain.TransactionTypeExpense}
		}
		pool := monthTxs
		if b.Kind == "spending" {
			pool = discretionary
		}
		var total float64
		for _, tx := range pool {
			if containsType(wantTypes, tx.Type) && budgetMatchesTx(b, tx) {
				total += tx.Amount
			}
		}
		return total
	}

	var lines []string
	var fixedPlanned, investmentPlanned float64
	if len(fixed) > 0 {
		lines = append(lines, "Fixed obligations:")
		for _, b := range fixed {
			fixedPlanned += b.Amount
			paid := spent(b)
			state := "pending"
			if paid >= b.Amount*0.95 {
				state = "paid"
			}
			lines = append(lines, fmt.Sprintf("  - %s: €%.0f / €%.0f (%s)", b.Name, paid, b.Amount, state))
		}
	}
	if len(investment) > 0 {
		lines = append(lines, "Investment targets:")
		for _, b := range investment {
			investmentPlanned += b.Amount
			got := spent(b)
			state := "in progress"
			if got >= b.Amount*0.95 {
				state = "reached"
			}
			lines = append(lines, fmt.Sprintf("  - %s: €%.0f / €%.0f (%s)", b.Name, got, b.Amount, state))
		}
	}
	if len(spending) > 0 {
		lines = append(lines, "Spending limits:")
		for _, b := range spending {
			used := spent(b)
			pct := 0.0
			if b.Amount > 0 {
				pct = used / b.Amount * 100
			}
			lines = append(lines, fmt.Sprintf("  - %s: €%.0f / €%.0f (%.0f%%)", b.Name, used, b.Amount, pct))
		}
	}

	incomeBase, baseLabel := s.incomeBase(settings, summary, now)
	header := "=== BUDGET STATUS (this month) ==="
	if incomeBase > 0 {
		safe := incomeBase - fixedPlanned - investmentPlanned - discretionarySpent
		lines = append([]string{fmt.Sprintf("Income base: €%.0f (%s)", incomeBase, baseLabel)}, lines...)
		lines = append(lines, fmt.Sprintf("Safe to spend the rest of the month: €%.0f", safe))
	}
	return header + "\n" + strings.Join(lines, "\n")
}

// incomeBase derives the monthly income base the Budget page uses: manual or
// gross-salary override when set, otherwise the median of complete months.
func (s *insightService) incomeBase(settings *domain.BudgetSettings, summary *domain.TransactionSummary, now time.Time) (float64, string) {
	if settings != nil {
		switch settings.IncomeMode {
		case "manual":
			if settings.ManualIncome > 0 {
				return settings.ManualIncome, "manual"
			}
		case "gross":
			if settings.GrossSalary > 0 {
				return ltNetSalary(settings.GrossSalary, settings.MonthlyDeductions), "net of LT taxes"
			}
		}
	}
	return medianMonthlyIncome(summary.ByMonth, now), "median of complete months"
}

// stockPositionsSection lists holdings with cost basis and, when Yahoo
// answers, live price / market value / unrealized gain. Best-effort: a slow
// or failed quote degrades to cost-basis-only for that ticker.
func (s *insightService) stockPositionsSection(latest *domain.Balance) string {
	if s.stockSvc == nil {
		return ""
	}
	portfolio, err := s.stockSvc.GetPortfolio()
	if err != nil || portfolio == nil || len(portfolio.Holdings) == 0 {
		return ""
	}
	// Only open positions (a fully-sold ticker has ~0 shares).
	var open []domain.StockHolding
	for _, h := range portfolio.Holdings {
		if h.Shares > 0.0001 {
			open = append(open, h)
		}
	}
	sort.Slice(open, func(i, j int) bool { return open[i].TotalCost.Value > open[j].TotalCost.Value })

	quotes := s.fetchQuotes(open)

	var lines []string
	anyLive := false
	for _, h := range open {
		cur := string(h.AvgCost.Currency)
		base := fmt.Sprintf("  - %s: %.4g sh @ %.2f %s avg = %.0f %s cost",
			h.Ticker, h.Shares, h.AvgCost.Value, cur, h.TotalCost.Value, cur)
		if q := quotes[h.Ticker]; q != nil && q.Price > 0 {
			anyLive = true
			price, qcur := normalizeQuote(q.Price, q.Currency)
			if strings.EqualFold(qcur, cur) {
				// Same currency as the cost basis — market value and
				// unrealized gain are meaningful.
				mktValue := h.Shares * price
				gainPct := 0.0
				if h.TotalCost.Value > 0 {
					gainPct = (mktValue - h.TotalCost.Value) / h.TotalCost.Value * 100
				}
				base += fmt.Sprintf(" · now %.2f %s → %.0f %s (%+.1f%%)", price, cur, mktValue, cur, gainPct)
			} else {
				// Yahoo prices this listing in a different currency than the
				// trade was recorded in. Show the live price honestly but do
				// NOT invent a cross-currency gain % (no FX rate available).
				base += fmt.Sprintf(" · now %.2f %s (live; cost recorded in %s, no FX conversion applied)", price, qcur, cur)
			}
		}
		lines = append(lines, base)
	}

	header := "=== STOCK POSITIONS ==="
	if anyLive {
		header += " (live prices via Yahoo Finance)"
	} else {
		lines = append(lines, "(live prices unavailable right now — showing cost basis only)")
	}
	// Cost basis and realized gains are only meaningful per-currency — the
	// portfolio aggregate sums EUR+USD, so report subtotals instead.
	if costByCur := sumByCurrency(open); len(costByCur) > 0 {
		lines = append(lines, "Total cost basis: "+moneyByCurrency(costByCur))
	}
	if gainByCur := realizedByCurrency(portfolio.Holdings); len(gainByCur) > 0 {
		lines = append(lines, "Realized gains to date: "+moneyByCurrency(gainByCur))
	}
	if latest != nil {
		lines = append(lines, fmt.Sprintf("EUR portfolio snapshots (manually tracked): Swed ETF €%.0f · Revolut stocks €%.0f · IBKR €%.0f",
			latest.SwedETF, latest.RevStocks, latest.IBKRStocks))
	}
	return header + "\n" + strings.Join(lines, "\n")
}

// fetchQuotes prices every holding concurrently, bounded by an overall
// deadline so a slow Yahoo never stalls analysis or chat. Missing quotes are
// simply absent from the map.
func (s *insightService) fetchQuotes(holdings []domain.StockHolding) map[string]*marketdata.Quote {
	out := map[string]*marketdata.Quote{}
	if s.quote == nil || len(holdings) == 0 {
		return out
	}
	type res struct {
		ticker string
		q      *marketdata.Quote
	}
	ch := make(chan res, len(holdings))
	var wg sync.WaitGroup
	for _, h := range holdings {
		wg.Add(1)
		go func(ticker string) {
			defer wg.Done()
			q, err := s.quote(ticker)
			if err != nil {
				ch <- res{ticker, nil}
				return
			}
			ch <- res{ticker, q}
		}(h.Ticker)
	}
	go func() { wg.Wait(); close(ch) }()

	deadline := time.After(12 * time.Second)
	for i := 0; i < len(holdings); i++ {
		select {
		case r := <-ch:
			if r.q != nil {
				out[r.ticker] = r.q
			}
		case <-deadline:
			return out // whatever arrived in time
		}
	}
	return out
}

// normalizeQuote converts Yahoo's pence quotes ("GBp") to pounds so they can
// be compared against a GBP-recorded cost basis; other currencies pass through.
func normalizeQuote(price float64, currency string) (float64, string) {
	if currency == "GBp" {
		return price / 100, "GBP"
	}
	return price, currency
}

// sumByCurrency totals each holding's cost basis under its own currency.
func sumByCurrency(holdings []domain.StockHolding) map[string]float64 {
	out := map[string]float64{}
	for _, h := range holdings {
		out[string(h.TotalCost.Currency)] += h.TotalCost.Value
	}
	return out
}

// realizedByCurrency totals realized gains per currency, dropping negligible
// amounts (a never-sold portfolio has none).
func realizedByCurrency(holdings []domain.StockHolding) map[string]float64 {
	out := map[string]float64{}
	for _, h := range holdings {
		if v := h.RealizedGain.Value; v > 0.5 || v < -0.5 {
			out[string(h.RealizedGain.Currency)] += v
		}
	}
	return out
}

// moneyByCurrency renders a currency→amount map as "1000 EUR, 500 USD",
// ordered by currency code for stable output.
func moneyByCurrency(m map[string]float64) string {
	curs := make([]string, 0, len(m))
	for c := range m {
		curs = append(curs, c)
	}
	sort.Strings(curs)
	parts := make([]string, 0, len(curs))
	for _, c := range curs {
		parts = append(parts, fmt.Sprintf("%.0f %s", m[c], c))
	}
	return strings.Join(parts, ", ")
}

// budgetMatchesTx mirrors the frontend budgetMatches: rule-style AND — every
// matcher that is set must hold; a budget with neither matches nothing.
func budgetMatchesTx(b domain.Budget, tx domain.Transaction) bool {
	labels := splitBudgetLabels(b.Label)
	if len(labels) == 0 && b.Category == "" {
		return false
	}
	labelOk := len(labels) == 0
	for _, l := range labels {
		if tx.HasLabel(l) {
			labelOk = true
			break
		}
	}
	categoryOk := b.Category == "" || b.Category == string(tx.Category)
	return labelOk && categoryOk
}

func splitBudgetLabels(label string) []string {
	var out []string
	for _, l := range strings.Split(label, ",") {
		if t := strings.TrimSpace(l); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func containsType(types []domain.TransactionType, t domain.TransactionType) bool {
	for _, x := range types {
		if x == t {
			return true
		}
	}
	return false
}

// medianMonthlyIncome is the median income over complete months (excluding
// the current, in-progress month), matching the frontend's income base.
func medianMonthlyIncome(byMonth []domain.MonthlySummary, now time.Time) float64 {
	y, m := now.Year(), int(now.Month())
	var incomes []float64
	for _, ms := range byMonth {
		if ms.Year == y && ms.Month == m {
			continue // current month is incomplete
		}
		if ms.Income > 0 {
			incomes = append(incomes, ms.Income)
		}
	}
	if len(incomes) == 0 {
		return 0
	}
	sort.Float64s(incomes)
	mid := len(incomes) / 2
	if len(incomes)%2 == 1 {
		return incomes[mid]
	}
	return (incomes[mid-1] + incomes[mid]) / 2
}

// ltNetSalary mirrors frontend utils/ltSalary.ts (LT 2026 employee taxes):
// Sodra 19.5%, GPM 20% on (gross − NPD), NPD tapering from 747 above MMA.
func ltNetSalary(gross, deductions float64) float64 {
	const sodraRate = 0.195
	const gpmRate = 0.20
	const mma = 1038.0
	const npdMax = 747.0
	const npdSlope = 0.49
	sodra := gross * sodraRate
	npd := npdMax
	if gross > mma {
		npd = npdMax - npdSlope*(gross-mma)
		if npd < 0 {
			npd = 0
		}
	}
	gpm := (gross - npd) * gpmRate
	if gpm < 0 {
		gpm = 0
	}
	return gross - sodra - gpm - deductions
}

// categoryDetailSection is the substance behind the overview's per-category
// review: for each top expense category in the period — total, count, the
// biggest labels inside it, and the previous equal-length window's total for
// trend. Undated reports use the last 90 days so a trend window exists.
func (s *insightService) categoryDetailSection(allTxs []domain.Transaction, dateFrom, dateTo *time.Time) string {
	now := time.Now()
	from, to := dateFrom, dateTo
	if from == nil && to == nil {
		f := now.AddDate(0, 0, -90)
		from, to = &f, &now
	}
	if from == nil {
		f := now.AddDate(-20, 0, 0)
		from = &f
	}
	if to == nil {
		to = &now
	}
	span := to.Sub(*from)
	prevFrom := from.Add(-span - 24*time.Hour)
	prevTo := from.Add(-24 * time.Hour)

	inWindow := func(t time.Time, f, tt time.Time) bool {
		return !t.Before(f) && !t.After(tt.Add(24*time.Hour-time.Nanosecond))
	}
	type catAgg struct {
		total  float64
		count  int
		labels map[string]float64
	}
	cats := map[string]*catAgg{}
	prev := map[string]float64{}
	for _, tx := range allTxs {
		if tx.Type != domain.TransactionTypeExpense || tx.Category == "Transfers" {
			continue
		}
		cat := string(tx.Category)
		if inWindow(tx.Date, *from, *to) {
			if cats[cat] == nil {
				cats[cat] = &catAgg{labels: map[string]float64{}}
			}
			cats[cat].total += tx.Amount
			cats[cat].count++
			for _, l := range splitLabels(tx.Labels) {
				cats[cat].labels[l] += tx.Amount
			}
		} else if inWindow(tx.Date, prevFrom, prevTo) {
			prev[cat] += tx.Amount
		}
	}
	if len(cats) == 0 {
		return ""
	}
	type row struct {
		name string
		agg  *catAgg
	}
	var rows []row
	for name, agg := range cats {
		rows = append(rows, row{name, agg})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].agg.total > rows[j].agg.total })
	if len(rows) > 8 {
		rows = rows[:8]
	}
	var lines []string
	for _, r := range rows {
		line := fmt.Sprintf("  - %s: €%.0f (%d tx)", r.name, r.agg.total, r.agg.count)
		if p, ok := prev[r.name]; ok && p > 0 {
			line += fmt.Sprintf(" | previous period €%.0f (%+.0f%%)", p, (r.agg.total-p)/p*100)
		} else {
			line += " | no spend in the previous period"
		}
		type ls struct {
			label string
			total float64
		}
		var labels []ls
		for l, t := range r.agg.labels {
			labels = append(labels, ls{l, t})
		}
		sort.Slice(labels, func(i, j int) bool { return labels[i].total > labels[j].total })
		if len(labels) > 3 {
			labels = labels[:3]
		}
		if len(labels) > 0 {
			var parts []string
			for _, l := range labels {
				parts = append(parts, fmt.Sprintf("%s €%.0f", l.label, l.total))
			}
			line += " | top labels: " + strings.Join(parts, ", ")
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
