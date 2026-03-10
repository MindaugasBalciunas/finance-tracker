package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
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

	// Build monthly spending summary (last 6 months)
	var monthlyLines []string
	months := recentSummary.ByMonth
	if len(months) > 6 {
		months = months[len(months)-6:]
	}
	for _, m := range months {
		monthlyLines = append(monthlyLines, fmt.Sprintf("  - %s %d: expenses €%.0f, income €%.0f, invested €%.0f",
			time.Month(m.Month).String()[:3], m.Year, m.Expenses, m.Income, m.Investments))
	}

	savingsRate := 0.0
	if summary.TotalIncome > 0 {
		savingsRate = ((summary.TotalIncome - summary.TotalExpenses) / summary.TotalIncome) * 100
	}

	freeCash := latest.Seb + latest.Swed + latest.Luminor + latest.Cash + latest.RevM + latest.RevR
	investments := latest.SwedETF + latest.RevStocks
	pensions := latest.SwedPen + latest.Art

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

=== RECENT MONTHLY CASH FLOW (last 6 months) ===
%s

Please write a concise personal finance overview covering:
1. Overall financial health (2-3 sentences assessing net worth, savings rate, and balance composition)
2. Expense trends (2-3 sentences on spending patterns and any concerns from recent months)
3. Opportunities (3 specific, actionable bullet points for improvement or optimisation)
4. One positive highlight to acknowledge good financial behaviour

Keep it direct, personal, and under 350 words. Write in plain paragraphs and bullet points — no section headers or markdown formatting. Address the person directly as "you".`,
		latest.Date.Format("2006-01-02"),
		latest.Total, freeCash, investments, pensions, cryptoEur,
		summary.TotalIncome, summary.TotalExpenses, summary.TotalInvestments,
		summary.TotalIncome-summary.TotalExpenses, savingsRate,
		strings.Join(topExpenses, "\n"),
		strings.Join(monthlyLines, "\n"),
	)

	return prompt, nil
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
