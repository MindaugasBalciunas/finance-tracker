package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
)

type InsightService interface {
	GetLatest() (*domain.AIInsight, error)
	Generate() (*domain.AIInsight, error)
	List(limit int) ([]domain.AIInsight, error)
}

type insightService struct {
	repo   repository.InsightRepository
	txSvc  TransactionService
	balSvc BalanceService
}

func NewInsightService(repo repository.InsightRepository, txSvc TransactionService, balSvc BalanceService) InsightService {
	return &insightService{repo: repo, txSvc: txSvc, balSvc: balSvc}
}

func (s *insightService) GetLatest() (*domain.AIInsight, error) {
	return s.repo.GetLatest()
}

func (s *insightService) List(limit int) ([]domain.AIInsight, error) {
	return s.repo.List(limit)
}

func (s *insightService) Generate() (*domain.AIInsight, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, errors.New("ANTHROPIC_API_KEY not set")
	}

	prompt, err := s.buildPrompt()
	if err != nil {
		return nil, fmt.Errorf("building prompt: %w", err)
	}

	content, err := callClaude(apiKey, prompt)
	if err != nil {
		return nil, fmt.Errorf("calling Claude API: %w", err)
	}

	insight := &domain.AIInsight{Content: content}
	if err := s.repo.Create(insight); err != nil {
		return nil, fmt.Errorf("saving insight: %w", err)
	}
	return insight, nil
}

func (s *insightService) buildPrompt() (string, error) {
	// All-time summary
	summary, err := s.txSvc.GetSummary(domain.TransactionFilter{})
	if err != nil {
		return "", err
	}

	// Latest balance snapshot
	latest, err := s.balSvc.GetLatest(0)
	if err != nil {
		return "", err
	}

	// Last 6 months trend
	sixMonthsAgo := time.Now().AddDate(0, -6, 0)
	recentSummary, _ := s.txSvc.GetSummary(domain.TransactionFilter{DateFrom: &sixMonthsAgo})

	// Build category breakdown (top 5 expenses)
	var topExpenses []string
	count := 0
	for _, c := range summary.ByCategory {
		if c.Type == domain.TransactionTypeExpense && count < 5 {
			topExpenses = append(topExpenses, fmt.Sprintf("  - %s: €%.0f (%d transactions)", c.Category, c.Total, c.Count))
			count++
		}
	}

	// Label aggregates — labels cut across categories (fixed obligations,
	// merchants, employers), so without them the model can't separate
	// pre-committed money from spending decisions.
	allTxs, err := s.txSvc.ListAll()
	if err != nil {
		return "", err
	}
	labelSections := buildLabelSections(allTxs, summary.TotalIncome, time.Now())

	// Build monthly spending summary (last 6 months)
	var monthlyLines []string
	months := recentSummary.ByMonth
	if len(months) > 6 {
		months = months[len(months)-6:]
	}
	for _, m := range months {
		line := fmt.Sprintf("  - %s %d: expenses €%.0f, income €%.0f, invested €%.0f",
			time.Month(m.Month).String()[:3], m.Year, m.Expenses, m.Income, m.Investments)
		if top := labelSections.topMonthLabels[fmt.Sprintf("%04d-%02d", m.Year, m.Month)]; top != "" {
			line += " | top labels: " + top
		}
		monthlyLines = append(monthlyLines, line)
	}

	savingsRate := 0.0
	if summary.TotalIncome > 0 {
		savingsRate = ((summary.TotalIncome - summary.TotalExpenses) / summary.TotalIncome) * 100
	}

	freeCash := latest.Seb + latest.Swed + latest.Luminor + latest.Cash + latest.RevM + latest.RevR
	investments := latest.SwedETF + latest.RevStocks
	pensions := latest.SebPen + latest.Art

	// Convert BTC to EUR using snapshot price or 0 for legacy entries
	cryptoEur := 0.0
	if latest.BtcPrice > 0 {
		cryptoEur = (latest.RBTC + latest.MBTC) * latest.BtcPrice
	} else {
		// Legacy format where RBTC/MBTC were stored as EUR
		cryptoEur = latest.RBTC + latest.MBTC
	}

	prompt := fmt.Sprintf(`You are a personal finance advisor reviewing real financial data for a private individual in Lithuania.

=== CURRENT BALANCE SNAPSHOT (as of %s) ===
Net Worth:    €%.0f
Free Cash:    €%.0f  (SEB + Swedbank + Luminor + Cash + Revolut)
Investments:  €%.0f  (Swed ETF + Revolut Stocks)
Pensions:     €%.0f  (Swed 2nd Pillar + Artea 3rd Pillar)
Crypto (BTC): €%.0f  (Revolut R + M BTC)

=== ALL-TIME TRANSACTION SUMMARY ===
Total Income:     €%.0f
Total Expenses:   €%.0f
Total Invested:   €%.0f
Net Saved:        €%.0f
Savings Rate:     %.1f%%

=== TOP EXPENSE CATEGORIES (all time) ===
%s

=== FIXED MONTHLY OBLIGATIONS (by label: loan, alimony, leasing, counterparty transfers) ===
%s
These are pre-committed, not spending decisions — when judging spending habits, also consider the discretionary picture with these excluded.

=== TOP SPENDING LABELS, DISCRETIONARY (last 12 months) ===
%s

=== INCOME SOURCES (by label, all time) ===
%s

=== RECENT MONTHLY CASH FLOW (last 6 months) ===
%s

Please write a concise personal finance overview covering:
1. Overall financial health (2-3 sentences assessing net worth, savings rate, and balance composition)
2. Expense trends (2-3 sentences on spending patterns and any concerns from recent months — use the label data to name specific spending drivers, and separate fixed obligations from choices)
3. Opportunities (3 specific, actionable bullet points for improvement or optimisation)
4. One positive highlight to acknowledge good financial behaviour

Keep it direct, personal, and under 350 words. Write in plain paragraphs and bullet points — no section headers or markdown formatting. Address the person directly as "you".`,
		latest.Date.Format("2006-01-02"),
		latest.Total, freeCash, investments, pensions, cryptoEur,
		summary.TotalIncome, summary.TotalExpenses, summary.TotalInvestments,
		summary.TotalIncome-summary.TotalExpenses, savingsRate,
		strings.Join(topExpenses, "\n"),
		labelSections.fixedObligations,
		labelSections.topSpendingLabels,
		labelSections.incomeSources,
		strings.Join(monthlyLines, "\n"),
	)

	return prompt, nil
}

// fixedObligationLabels marks money that isn't a spending decision (loan,
// alimony, leasing payments and counterparty transfers). Mirrors
// FIXED_LABELS in the frontend (utils/labels.ts).
var fixedObligationLabels = []string{"loan", "alimony", "leasing", "evelina"}

type labelSections struct {
	fixedObligations  string
	topSpendingLabels string
	incomeSources     string
	// topMonthLabels maps "YYYY-MM" to a "label €X, label €Y" line for the
	// month's biggest discretionary labels.
	topMonthLabels map[string]string
}

type labelAgg struct {
	label string
	total float64
	count int
	first time.Time
}

func topLabelAggs(sums map[string]*labelAgg, n int) []*labelAgg {
	out := make([]*labelAgg, 0, len(sums))
	for _, a := range sums {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].total != out[j].total {
			return out[i].total > out[j].total
		}
		return out[i].label < out[j].label
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// buildLabelSections aggregates the label field into the prompt sections:
// fixed obligations (all time + monthly average), top discretionary spending
// labels (last 12 months), income by label (employers), and per-month top
// labels for the recent-cash-flow lines.
func buildLabelSections(txs []domain.Transaction, totalIncome float64, now time.Time) labelSections {
	isFixed := func(t *domain.Transaction) bool {
		for _, l := range fixedObligationLabels {
			if t.HasLabel(l) {
				return true
			}
		}
		return false
	}

	yearAgo := now.AddDate(-1, 0, 0)
	sixMonthsAgo := now.AddDate(0, -6, 0)

	fixedSums := map[string]*labelAgg{}
	spendSums := map[string]*labelAgg{}
	incomeSums := map[string]*labelAgg{}
	monthLabelSums := map[string]map[string]*labelAgg{}
	fixedTxTotal := 0.0

	for i := range txs {
		tx := &txs[i]
		labels := strings.Split(tx.Labels, ",")
		switch tx.Type {
		case domain.TransactionTypeIncome:
			for _, l := range labels {
				if l == "" {
					continue
				}
				if incomeSums[l] == nil {
					incomeSums[l] = &labelAgg{label: l}
				}
				incomeSums[l].total += tx.Amount
				incomeSums[l].count++
			}
		case domain.TransactionTypeExpense:
			if isFixed(tx) {
				// A payment can carry several fixed labels (the importer tags
				// loan transfers "loan,evelina") — attribute it ONCE, to its
				// first fixed label, so the obligation lines don't double
				// count the same money.
				fixedTxTotal += tx.Amount
				for _, fl := range fixedObligationLabels {
					if !tx.HasLabel(fl) {
						continue
					}
					if fixedSums[fl] == nil {
						fixedSums[fl] = &labelAgg{label: fl, first: tx.Date}
					}
					fixedSums[fl].total += tx.Amount
					fixedSums[fl].count++
					if tx.Date.Before(fixedSums[fl].first) {
						fixedSums[fl].first = tx.Date
					}
					break
				}
				continue
			}
			for _, l := range labels {
				if l == "" {
					continue
				}
				if tx.Date.After(yearAgo) {
					if spendSums[l] == nil {
						spendSums[l] = &labelAgg{label: l}
					}
					spendSums[l].total += tx.Amount
					spendSums[l].count++
				}
				if tx.Date.After(sixMonthsAgo) {
					mk := tx.Date.Format("2006-01")
					if monthLabelSums[mk] == nil {
						monthLabelSums[mk] = map[string]*labelAgg{}
					}
					if monthLabelSums[mk][l] == nil {
						monthLabelSums[mk][l] = &labelAgg{label: l}
					}
					monthLabelSums[mk][l].total += tx.Amount
					monthLabelSums[mk][l].count++
				}
			}
		}
	}

	none := "  - none"

	var fixedLines []string
	for _, a := range topLabelAggs(fixedSums, 6) {
		line := fmt.Sprintf("  - %s: €%.0f total (%d payments", a.label, a.total, a.count)
		// Monthly rate over the label's OWN lifetime — a leasing that started
		// last year must not be averaged over the loan's decade.
		if labelMonths := monthsBetween(a.first, now); labelMonths >= 2 {
			line += fmt.Sprintf(", ≈€%.0f/month since %s", a.total/float64(labelMonths), a.first.Format("2006-01"))
		}
		line += ")"
		if totalIncome > 0 {
			line += fmt.Sprintf(" — %.1f%% of all-time income", a.total/totalIncome*100)
		}
		fixedLines = append(fixedLines, line)
	}
	if len(fixedLines) == 0 {
		fixedLines = []string{none}
	} else if totalIncome > 0 {
		fixedLines = append(fixedLines, fmt.Sprintf("  - TOTAL fixed obligations: €%.0f — %.1f%% of all-time income", fixedTxTotal, fixedTxTotal/totalIncome*100))
	}

	var spendLines []string
	for _, a := range topLabelAggs(spendSums, 10) {
		spendLines = append(spendLines, fmt.Sprintf("  - %s: €%.0f (%d transactions, avg €%.0f)", a.label, a.total, a.count, a.total/float64(a.count)))
	}
	if len(spendLines) == 0 {
		spendLines = []string{none}
	}

	var incomeLines []string
	for _, a := range topLabelAggs(incomeSums, 8) {
		line := fmt.Sprintf("  - %s: €%.0f (%d payments)", a.label, a.total, a.count)
		if totalIncome > 0 {
			line += fmt.Sprintf(" — %.0f%% of all income", a.total/totalIncome*100)
		}
		incomeLines = append(incomeLines, line)
	}
	if len(incomeLines) == 0 {
		incomeLines = []string{none}
	}

	topMonthLabels := map[string]string{}
	for mk, sums := range monthLabelSums {
		var parts []string
		for _, a := range topLabelAggs(sums, 3) {
			parts = append(parts, fmt.Sprintf("%s €%.0f", a.label, a.total))
		}
		topMonthLabels[mk] = strings.Join(parts, ", ")
	}

	return labelSections{
		fixedObligations:  strings.Join(fixedLines, "\n"),
		topSpendingLabels: strings.Join(spendLines, "\n"),
		incomeSources:     strings.Join(incomeLines, "\n"),
		topMonthLabels:    topMonthLabels,
	}
}

// monthsBetween counts calendar months from a to b, at least 1.
func monthsBetween(a, b time.Time) int {
	m := (b.Year()-a.Year())*12 + int(b.Month()) - int(a.Month()) + 1
	if m < 1 {
		return 1
	}
	return m
}

// Claude API types
type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []claudeMessage `json:"messages"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

const claudeModel = "claude-haiku-4-5-20251001"

func callClaude(apiKey, prompt string) (string, error) {
	reqBody := claudeRequest{
		Model:     claudeModel,
		MaxTokens: 1024,
		Messages: []claudeMessage{
			{Role: "user", Content: prompt},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result claudeResponse
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("Claude API error: %s", result.Error.Message)
	}

	if len(result.Content) == 0 {
		return "", errors.New("empty response from Claude")
	}

	return result.Content[0].Text, nil
}
