package handler

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/mindaugas/finance-tracker/internal/service"
)

// BudgetStatusSource is what the review needs from the budget engine.
// service.InsightService satisfies it.
type BudgetStatusSource interface {
	BudgetStatus(year, month int) (*service.BudgetStatusReport, error)
}

// ReviewHandler serves the month-end review: one page answering "how did
// last month go, and is anything in the data stale or waiting on me?".
// Deterministic — no AI call — so it is free, instant and always available.
type ReviewHandler struct {
	txRepo  repository.TransactionRepository
	balRepo repository.BalanceRepository
	assets  repository.AssetRepository
	bank    repository.BankRepository // optional
	budgets BudgetStatusSource        // optional
	now     func() time.Time
}

func NewReviewHandler(txRepo repository.TransactionRepository, balRepo repository.BalanceRepository,
	assets repository.AssetRepository, bank repository.BankRepository, budgets BudgetStatusSource) *ReviewHandler {
	return &ReviewHandler{txRepo: txRepo, balRepo: balRepo, assets: assets, bank: bank, budgets: budgets, now: time.Now}
}

func (h *ReviewHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/review", h.Get)
}

type reviewTotals struct {
	Income      float64  `json:"income"`
	Spending    float64  `json:"spending"`
	Invested    float64  `json:"invested"`
	NetSaved    float64  `json:"net_saved"`
	SavingsRate *float64 `json:"savings_rate,omitempty"` // % of income
}

type reviewNetWorth struct {
	StartDate string  `json:"start_date"`
	Start     float64 `json:"start"`
	EndDate   string  `json:"end_date"`
	End       float64 `json:"end"`
	Change    float64 `json:"change"`
}

type reviewCategory struct {
	Category string  `json:"category"`
	Spent    float64 `json:"spent"`
	Average  float64 `json:"average"` // over the previous six months
	Delta    float64 `json:"delta"`
}

type reviewExpense struct {
	ID       uint    `json:"id"`
	Date     string  `json:"date"`
	Amount   float64 `json:"amount"`
	Category string  `json:"category"`
	Comment  string  `json:"comment"`
}

type reviewBudgetLine struct {
	Name     string  `json:"name"`
	Budgeted float64 `json:"budgeted"`
	Spent    float64 `json:"spent"`
}

type reviewBudget struct {
	Over        []reviewBudgetLine `json:"over"`
	WithinCount int                `json:"within_count"`
	SafeToSpend *float64           `json:"safe_to_spend,omitempty"`
}

type reviewCheck struct {
	// Level is "warn" (needs action) or "info".
	Level string `json:"level"`
	Text  string `json:"text"`
	// Link is an app route that fixes it, e.g. "/balances".
	Link string `json:"link,omitempty"`
}

type monthReview struct {
	Month    string `json:"month"` // YYYY-MM
	Complete bool   `json:"complete"`
	reviewTotals
	Previous    reviewTotals     `json:"previous"`
	SixMonthAvg reviewTotals     `json:"six_month_avg"`
	NetWorth    *reviewNetWorth  `json:"net_worth,omitempty"`
	Categories  []reviewCategory `json:"categories"`
	TopExpenses []reviewExpense  `json:"top_expenses"`
	Budget      *reviewBudget    `json:"budget,omitempty"`
	OwedToYou   float64          `json:"owed_to_you"`
	Checks      []reviewCheck    `json:"checks"`
}

func (h *ReviewHandler) Get(c *gin.Context) {
	now := h.now()
	// Default: the last complete month — a review of a month still running
	// is a forecast, not a review.
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	if v := c.Query("month"); v != "" {
		t, err := time.Parse("2006-01", v)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "month must be YYYY-MM"})
			return
		}
		month = t
	}

	txs, err := h.txRepo.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	bals, err := h.balRepo.List(domain.BalanceFilter{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	in := reviewInput{month: month, now: now, txs: txs, balances: bals}
	if assets, err := h.assets.ListAll(); err == nil {
		in.assets = assets
	}
	if h.budgets != nil {
		if st, err := h.budgets.BudgetStatus(month.Year(), int(month.Month())); err == nil {
			in.budget = st
		}
	}
	if h.bank != nil {
		if conns, err := h.bank.ListConnections(); err == nil {
			in.conns = conns
		}
		if pending, err := h.bank.CountPendingByLink(); err == nil {
			for _, n := range pending {
				in.bankWaiting += n
			}
		}
	}
	c.JSON(http.StatusOK, buildMonthReview(in))
}

type reviewInput struct {
	month       time.Time // first day, UTC
	now         time.Time
	txs         []domain.Transaction
	balances    []domain.Balance
	assets      []domain.Asset
	budget      *service.BudgetStatusReport
	conns       []domain.BankConnection
	bankWaiting int
}

func r2(v float64) float64 { return math.Round(v*100) / 100 }

func totalsOf(txs []domain.Transaction) reviewTotals {
	var t reviewTotals
	for i := range txs {
		x := &txs[i]
		// Transfers move money between own accounts (or are shares someone
		// owes) — neither income nor spending, same as every other report.
		if x.Category == domain.CategoryTransfers {
			continue
		}
		switch x.Type {
		case domain.TransactionTypeIncome:
			t.Income += x.Amount
		case domain.TransactionTypeExpense:
			t.Spending += x.Amount
		case domain.TransactionTypeInvestment:
			t.Invested += x.Amount
		}
	}
	return finishTotals(t)
}

func finishTotals(t reviewTotals) reviewTotals {
	t.Income, t.Spending, t.Invested = r2(t.Income), r2(t.Spending), r2(t.Invested)
	t.NetSaved = r2(t.Income - t.Spending)
	if t.Income > 0 {
		rate := math.Round(t.NetSaved/t.Income*1000) / 10
		t.SavingsRate = &rate
	}
	return t
}

func inMonth(t time.Time, m time.Time) bool {
	return t.Year() == m.Year() && t.Month() == m.Month()
}

func buildMonthReview(in reviewInput) monthReview {
	m := in.month
	end := m.AddDate(0, 1, 0)
	out := monthReview{
		Month:       m.Format("2006-01"),
		Complete:    !in.now.Before(end),
		Categories:  []reviewCategory{},
		TopExpenses: []reviewExpense{},
		Checks:      []reviewCheck{},
	}

	byMonth := map[string][]domain.Transaction{}
	for _, t := range in.txs {
		k := t.Date.Format("2006-01")
		byMonth[k] = append(byMonth[k], t)
	}
	cur := byMonth[out.Month]
	out.reviewTotals = totalsOf(cur)
	out.Previous = totalsOf(byMonth[m.AddDate(0, -1, 0).Format("2006-01")])

	// Six-month average over the months before, counting only months with
	// any data — a ledger that starts mid-window must not average in zeros.
	var sum reviewTotals
	n := 0
	catSum := map[string]float64{}
	for i := 1; i <= 6; i++ {
		rows := byMonth[m.AddDate(0, -i, 0).Format("2006-01")]
		if len(rows) == 0 {
			continue
		}
		n++
		t := totalsOf(rows)
		sum.Income += t.Income
		sum.Spending += t.Spending
		sum.Invested += t.Invested
		for _, x := range rows {
			if x.Type == domain.TransactionTypeExpense && x.Category != domain.CategoryTransfers {
				catSum[string(x.Category)] += x.Amount
			}
		}
	}
	if n > 0 {
		f := float64(n)
		out.SixMonthAvg = finishTotals(reviewTotals{Income: sum.Income / f, Spending: sum.Spending / f, Invested: sum.Invested / f})
	}

	// Categories that moved most against their own average.
	catCur := map[string]float64{}
	var expenses []domain.Transaction
	unlabeled := 0
	for _, x := range cur {
		if x.Type != domain.TransactionTypeExpense || x.Category == domain.CategoryTransfers {
			continue
		}
		catCur[string(x.Category)] += x.Amount
		expenses = append(expenses, x)
		if x.Labels == "" {
			unlabeled++
		}
	}
	seen := map[string]bool{}
	for k := range catCur {
		seen[k] = true
	}
	for k := range catSum {
		seen[k] = true
	}
	for k := range seen {
		avg := 0.0
		if n > 0 {
			avg = catSum[k] / float64(n)
		}
		d := catCur[k] - avg
		if math.Abs(d) < 1 {
			continue
		}
		out.Categories = append(out.Categories, reviewCategory{Category: k, Spent: r2(catCur[k]), Average: r2(avg), Delta: r2(d)})
	}
	sort.Slice(out.Categories, func(i, j int) bool {
		if math.Abs(out.Categories[i].Delta) != math.Abs(out.Categories[j].Delta) {
			return math.Abs(out.Categories[i].Delta) > math.Abs(out.Categories[j].Delta)
		}
		return out.Categories[i].Category < out.Categories[j].Category
	})
	if len(out.Categories) > 6 {
		out.Categories = out.Categories[:6]
	}

	sort.Slice(expenses, func(i, j int) bool { return expenses[i].Amount > expenses[j].Amount })
	for i := 0; i < len(expenses) && i < 5; i++ {
		x := expenses[i]
		out.TopExpenses = append(out.TopExpenses, reviewExpense{ID: x.ID, Date: x.Date.Format("2006-01-02"),
			Amount: r2(x.Amount), Category: string(x.Category), Comment: x.Comment})
	}

	// Net worth: the last snapshot before the month against the last one in
	// it. Balances arrive newest first.
	var startB, endB *domain.Balance
	for i := range in.balances {
		b := &in.balances[i]
		if b.Date.Before(m) {
			if startB == nil || b.Date.After(startB.Date) {
				startB = b
			}
		} else if b.Date.Before(end) {
			if endB == nil || b.Date.After(endB.Date) {
				endB = b
			}
		}
	}
	if startB != nil && endB != nil {
		out.NetWorth = &reviewNetWorth{
			StartDate: startB.Date.Format("2006-01-02"), Start: r2(startB.Total),
			EndDate: endB.Date.Format("2006-01-02"), End: r2(endB.Total), Change: r2(endB.Total - startB.Total),
		}
	}

	if in.budget != nil {
		rb := &reviewBudget{Over: []reviewBudgetLine{}, SafeToSpend: in.budget.SafeToSpend}
		for _, l := range in.budget.Lines {
			// Monthly, non-fund lines only: a yearly line or a fund is judged
			// over its own horizon, not by one month's spend.
			if l.Period != domain.PeriodMonthly || l.Fund || l.Kind == "investment" || l.Amount <= 0 {
				continue
			}
			if l.MonthSpent > l.Amount+0.5 {
				rb.Over = append(rb.Over, reviewBudgetLine{Name: l.Name, Budgeted: r2(l.Amount), Spent: r2(l.MonthSpent)})
			} else {
				rb.WithinCount++
			}
		}
		sort.Slice(rb.Over, func(i, j int) bool { return rb.Over[i].Spent-rb.Over[i].Budgeted > rb.Over[j].Spent-rb.Over[j].Budgeted })
		out.Budget = rb
	}

	// Money other people owe you, across all time.
	for _, x := range in.txs {
		if x.OwedPerson() == "" {
			continue
		}
		if x.Type == domain.TransactionTypeIncome {
			out.OwedToYou -= x.Amount
		} else {
			out.OwedToYou += x.Amount
		}
	}
	out.OwedToYou = r2(out.OwedToYou)

	out.Checks = reviewChecks(in, endB, unlabeled)
	return out
}

func reviewChecks(in reviewInput, monthEnd *domain.Balance, unlabeled int) []reviewCheck {
	checks := []reviewCheck{}
	add := func(level, link, format string, a ...any) {
		checks = append(checks, reviewCheck{Level: level, Text: fmt.Sprintf(format, a...), Link: link})
	}
	days := func(t time.Time) int { return int(in.now.Sub(t).Hours() / 24) }

	var latest *domain.Balance
	for i := range in.balances {
		if latest == nil || in.balances[i].Date.After(latest.Date) {
			latest = &in.balances[i]
		}
	}
	switch {
	case latest == nil:
		add("warn", "/balances", "No balance snapshot yet — add one so net worth can be tracked.")
	case days(latest.Date) > 14:
		add("warn", "/balances", "Latest balance snapshot is %d days old — add a fresh one.", days(latest.Date))
	}
	if monthEnd == nil && latest != nil {
		add("info", "/balances", "No balance snapshot inside this month, so its net-worth change can't be shown.")
	}
	for _, a := range in.assets {
		ref := a.ValuationDate
		if ref == nil {
			ref = a.PurchaseDate
		}
		if ref != nil && days(*ref) > 183 {
			add("info", "/assets", "%s was last valued %s — over 6 months ago.", a.Name, ref.Format("2006-01-02"))
		}
		if a.LoanRateResetDate != nil {
			if d := int(a.LoanRateResetDate.Sub(in.now).Hours() / 24); d >= 0 && d <= 45 {
				add("info", "/assets", "%s loan rate resets on %s.", a.Name, a.LoanRateResetDate.Format("2006-01-02"))
			}
		}
	}
	for _, c := range in.conns {
		if c.Status != domain.BankConnAuthorized {
			continue
		}
		if d := int(c.ValidUntil.Sub(in.now).Hours() / 24); d < 0 {
			add("warn", "/banking", "%s bank access has expired — reconnect it.", c.ASPSPName)
		} else if d <= 14 {
			add("warn", "/banking", "%s bank access expires in %d days — reconnect before it lapses.", c.ASPSPName, d)
		}
	}
	if in.bankWaiting > 0 {
		add("warn", "/transactions", "%d bank row%s waiting in the inbox to be reviewed.", in.bankWaiting, plural(in.bankWaiting))
	}
	if unlabeled > 0 {
		add("info", "/transactions", "%d expense%s this month without labels.", unlabeled, plural(unlabeled))
	}
	return checks
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
