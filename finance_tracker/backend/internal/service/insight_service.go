package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/marketdata"
	"github.com/mindaugas/finance-tracker/internal/repository"
)

type InsightService interface {
	GetLatest() (*domain.AIInsight, error)
	// Generate builds and stores a per-section overview; dateFrom/dateTo
	// (nil = all time) scope the period-sensitive sections.
	Generate(dateFrom, dateTo *time.Time) (*domain.AIInsight, error)
	List(limit int) ([]domain.AIInsight, error)
	// Chat answers one turn of the AI chat: server-stored history plus the
	// new user message, grounded in a system message carrying the same data
	// report the analysis uses. Both turns are persisted so the conversation
	// follows the user across devices.
	Chat(ctx context.Context, message string) (string, error)
	ChatHistory() ([]domain.AIChatMessage, error)
	ClearChat() error
	AISettings() (*domain.AISettings, error)
	// SaveAISettings updates the gateway config. An empty apiKey keeps the
	// stored key unless clearKey is set.
	SaveAISettings(gatewayURL, model, apiKey string, clearKey bool) (*domain.AISettings, error)
	// TestGateway makes a minimal round-trip through the configured gateway.
	TestGateway() error
	// DataReport returns the full financial context report (all-time scope)
	// — the same text the analysis and chat are grounded in. Contains no
	// secrets; consumed by the MCP server's get_overview tool.
	DataReport() (string, error)
	// ViewSummary returns a short AI review of one app view for the period;
	// cached server-side for 15 minutes per view+period.
	ViewSummary(view string, from, to *time.Time, refresh bool) (string, error)
	// AssistTransaction suggests labels + a cleaner description for one
	// transaction, learned from the user's own history.
	AssistTransaction(input TransactionAssistInput) (*TransactionAssist, error)
	// ReindexSuggest proposes labeling changes: mode "unlabeled" tags rows
	// with no labels; mode "review" audits labeled rows and proposes remaps.
	// ApplyLabelSuggestions writes the user-approved changes (fixed-obligation
	// labels can never be removed).
	ReindexSuggest(mode string, limit, offset int) (*ReindexResult, error)
	ApplyLabelSuggestions(items []LabelApplyItem) (int, error)
	// RuleReview audits the auto-labeling rule set (dead/redundant/too-broad
	// rules, recurring uncovered patterns) and proposes adds/updates/deletes.
	// ApplyRuleSuggestions writes the user-approved rule changes.
	RuleReview() (*RuleReviewResult, error)
	ApplyRuleSuggestions(items []RuleApplyItem) (*RuleApplyResult, error)
	// BudgetStatus computes per-budget month-to-date progress (current month
	// when year/month are zero) — the JSON twin of the report's text section.
	BudgetStatus(year, month int) (*BudgetStatusReport, error)
	// AIContext returns the user's CFO-context document; SaveAIContext
	// replaces it (capped at 32KB).
	AIContext() (*domain.AIContext, error)
	SaveAIContext(content string) (*domain.AIContext, error)
}

type insightService struct {
	repo       repository.InsightRepository
	txSvc      TransactionService
	balSvc     BalanceService
	budgetRepo repository.BudgetRepository // may be nil; budget section is skipped when so
	stockSvc   StockService                // may be nil; stock section is skipped when so
	assetSvc   AssetService                // may be nil; the assets chat tool reports unavailable
	// quote fetches a live market quote; injectable so tests avoid the network.
	quote func(ticker string) (*marketdata.Quote, error)
}

func NewInsightService(repo repository.InsightRepository, txSvc TransactionService, balSvc BalanceService,
	budgetRepo repository.BudgetRepository, stockSvc StockService, assetSvc AssetService) InsightService {
	return &insightService{
		repo: repo, txSvc: txSvc, balSvc: balSvc,
		budgetRepo: budgetRepo, stockSvc: stockSvc, assetSvc: assetSvc,
		quote: marketdata.Fetch,
	}
}

func (s *insightService) GetLatest() (*domain.AIInsight, error) {
	return s.repo.GetLatest()
}

func (s *insightService) List(limit int) ([]domain.AIInsight, error) {
	return s.repo.List(limit)
}

func (s *insightService) Generate(dateFrom, dateTo *time.Time) (*domain.AIInsight, error) {
	prompt, err := s.buildPrompt(dateFrom, dateTo)
	if err != nil {
		return nil, fmt.Errorf("building prompt: %w", err)
	}

	// Configured gateway (nexos.ai) first; the ANTHROPIC_API_KEY env var
	// remains as a legacy fallback for pre-gateway deployments.
	settings, serr := s.repo.GetAISettings()
	if serr != nil {
		return nil, fmt.Errorf("reading AI settings: %w", serr)
	}
	var content string
	switch {
	case settings.Configured():
		msgs := []domain.ChatMessage{}
		if ctx := s.userContextBlock(); ctx != "" {
			msgs = append(msgs, domain.ChatMessage{Role: "system", Content: ctx})
		}
		msgs = append(msgs, domain.ChatMessage{Role: "user", Content: prompt})
		content, err = callGateway(context.Background(), settings, msgs, 4096)
		if err != nil {
			return nil, fmt.Errorf("calling AI gateway: %w", err)
		}
	case os.Getenv("ANTHROPIC_API_KEY") != "":
		content, err = callClaude(os.Getenv("ANTHROPIC_API_KEY"), prompt)
		if err != nil {
			return nil, fmt.Errorf("calling Claude API: %w", err)
		}
	default:
		return nil, errors.New("AI gateway not configured — add your nexos.ai API key and model in AI settings")
	}

	insight := &domain.AIInsight{Content: content}
	if err := s.repo.Create(insight); err != nil {
		return nil, fmt.Errorf("saving insight: %w", err)
	}
	s.logAIActivity("analysis", periodLabel(dateFrom, dateTo), content)
	return insight, nil
}

// maxChatTurns bounds how much history is replayed to the gateway — the
// system data report already dominates the context. Replayed history is
// further trimmed to the memory window (7 days) and each message to
// maxReplayChars: old or oversized turns cost tokens on EVERY question, and
// the recent-activity digests carry the gist instead. Stored history is
// never trimmed — only what goes over the wire.
const maxChatTurns = 24
const maxReplayChars = 1600

func (s *insightService) Chat(ctx context.Context, message string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	settings, err := s.repo.GetAISettings()
	if err != nil {
		return "", err
	}
	if !settings.Configured() {
		return "", errors.New("AI gateway not configured — add your nexos.ai API key and model in AI settings")
	}
	system, err := s.chatSystemMessage()
	if err != nil {
		return "", fmt.Errorf("building context: %w", err)
	}
	history, err := s.repo.ListChat(maxChatTurns)
	if err != nil {
		return "", err
	}
	cutoff := time.Now().Add(-aiMemoryWindow)
	messages := make([]gatewayMessage, 0, len(history)+2)
	messages = append(messages, gatewayMessage{Role: "system", Content: system})
	for _, m := range history {
		if m.CreatedAt.Before(cutoff) {
			continue // older turns live on as activity digests, not transcripts
		}
		content := m.Content
		if len(content) > maxReplayChars {
			content = content[:maxReplayChars] + "… [earlier answer trimmed]"
		}
		messages = append(messages, gatewayMessage{Role: m.Role, Content: content})
	}
	messages = append(messages, gatewayMessage{Role: "user", Content: message})

	// Agentic loop: the model may call read-only tools (the same surface the
	// MCP server exposes) to query the database live before answering. Tool
	// turns stay ephemeral — only the user question and the final answer are
	// persisted to history.
	tools := chatTools()
	var reply string
	for round := 0; ; round++ {
		if round >= maxToolRounds {
			return "", fmt.Errorf("the model exceeded %d tool rounds without answering — try a narrower question", maxToolRounds)
		}
		// Stop before starting another round once the overall budget is spent,
		// with a message the user can act on — better than the request hanging
		// until nginx returns a bare 504.
		if err := ctx.Err(); err != nil {
			return "", errors.New("this question took too long to answer — try a narrower one, or ask for fewer things at once")
		}
		msg, err := callGatewayFull(ctx, settings, messages, 8192, tools)
		if err != nil {
			if ctx.Err() != nil {
				return "", errors.New("this question took too long to answer — try a narrower one, or ask for fewer things at once")
			}
			return "", fmt.Errorf("calling AI gateway: %w", err)
		}
		if len(msg.ToolCalls) == 0 {
			if strings.TrimSpace(msg.Content) == "" {
				return "", errors.New("gateway returned an empty reply — try again or raise the model's token limit")
			}
			reply = msg.Content
			break
		}
		// Echo the assistant turn (with its tool_calls), then answer each
		// call. Tool failures are reported back as text so the model can
		// correct its arguments instead of the whole chat failing.
		messages = append(messages, msg)
		for _, call := range msg.ToolCalls {
			result, terr := s.runChatTool(call.Function.Name, call.Function.Arguments)
			if terr != nil {
				result = "tool error: " + terr.Error()
			}
			messages = append(messages, gatewayMessage{
				Role: "tool", ToolCallID: call.ID, Content: result,
			})
		}
	}

	// Persist both turns only after a successful reply — a failed call
	// leaves history unchanged so a retry doesn't duplicate the question.
	if err := s.repo.AppendChat(
		&domain.AIChatMessage{Role: "user", Content: message},
		&domain.AIChatMessage{Role: "assistant", Content: reply},
	); err != nil {
		return "", fmt.Errorf("saving chat: %w", err)
	}
	s.logAIActivity("chat", "", "Q: "+clipText(message, 160)+" — A: "+clipText(reply, 240))
	return reply, nil
}

func (s *insightService) ChatHistory() ([]domain.AIChatMessage, error) {
	return s.repo.ListChat(200)
}

// maxAIContextBytes bounds the CFO-context document — it rides along on
// every AI call (cached, but still), so keep it a briefing, not an archive.
const maxAIContextBytes = 32_000

func (s *insightService) AIContext() (*domain.AIContext, error) {
	return s.repo.GetAIContext()
}

func (s *insightService) SaveAIContext(content string) (*domain.AIContext, error) {
	content = strings.TrimSpace(content)
	if len(content) > maxAIContextBytes {
		return nil, fmt.Errorf("context is %d bytes — keep it under %d (it travels with every AI call)", len(content), maxAIContextBytes)
	}
	return s.repo.SaveAIContext(content)
}

// userContextBlock renders the CFO context as a prompt section ("" when the
// user hasn't written one). Injected as a system block, so with the prompt
// cache it costs almost nothing after the first call.
func (s *insightService) userContextBlock() string {
	c, err := s.repo.GetAIContext()
	if err != nil || strings.TrimSpace(c.Content) == "" {
		return ""
	}
	return "=== USER CFO CONTEXT (written by the user: who they are, their framework, standing rules and communication style — follow it) ===\n" + c.Content
}

func (s *insightService) ClearChat() error {
	return s.repo.ClearChat()
}

func (s *insightService) DataReport() (string, error) {
	return s.buildDataReport(nil, nil)
}

func (s *insightService) AISettings() (*domain.AISettings, error) {
	return s.repo.GetAISettings()
}

func (s *insightService) SaveAISettings(gatewayURL, model, apiKey string, clearKey bool) (*domain.AISettings, error) {
	settings, err := s.repo.GetAISettings()
	if err != nil {
		return nil, err
	}
	settings.GatewayURL = strings.TrimRight(strings.TrimSpace(gatewayURL), "/")
	if settings.GatewayURL == "" {
		settings.GatewayURL = domain.DefaultGatewayURL
	}
	settings.Model = strings.TrimSpace(model)
	switch {
	case clearKey:
		settings.APIKey = ""
	case strings.TrimSpace(apiKey) != "":
		settings.APIKey = strings.TrimSpace(apiKey)
	}
	if err := s.repo.SaveAISettings(settings); err != nil {
		return nil, err
	}
	return settings, nil
}

func (s *insightService) TestGateway() error {
	settings, err := s.repo.GetAISettings()
	if err != nil {
		return err
	}
	if !settings.Configured() {
		return errors.New("AI gateway not configured — set an API key and model first")
	}
	reply, err := callGateway(context.Background(), settings, []domain.ChatMessage{
		{Role: "user", Content: "Reply with the single word: ok"},
	}, 256)
	if err != nil {
		return err
	}
	if strings.TrimSpace(reply) == "" {
		return errors.New("gateway returned an empty reply")
	}
	return nil
}

// buildDataReport assembles the shared financial-context sections used by
// both the one-shot analysis prompt and the chat system message.
// buildDataReport assembles the shared financial-context report. When
// dateFrom/dateTo are set, the headline transaction summary, top categories
// and cash-flow reflect that period; point-in-time sections (balance
// snapshot, this month, budget, stock positions) are always current, and the
// budget income base still uses the full history.
func (s *insightService) buildDataReport(dateFrom, dateTo *time.Time) (string, error) {
	// All-time summary — feeds the budget income base and the label
	// aggregates, which need full history regardless of the selected period.
	allTime, err := s.txSvc.GetSummary(domain.TransactionFilter{})
	if err != nil {
		return "", err
	}

	// Period summary drives the headline numbers; falls back to all-time.
	summary := allTime
	if dateFrom != nil || dateTo != nil {
		summary, err = s.txSvc.GetSummary(domain.TransactionFilter{DateFrom: dateFrom, DateTo: dateTo})
		if err != nil {
			return "", err
		}
	}

	// Latest balance snapshot
	latest, err := s.balSvc.GetLatest(0)
	if err != nil {
		return "", err
	}

	// Build category breakdown (top 5 expenses in the selected period)
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
	labelSections := buildLabelSections(allTxs, allTime.TotalIncome, time.Now())

	// Monthly cash flow across the selected period (capped to the most
	// recent 12 months so an all-time selection stays readable).
	var monthlyLines []string
	months := summary.ByMonth
	if len(months) > 12 {
		months = months[len(months)-12:]
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
	investments := latest.SwedETF + latest.RevStocks + latest.IBKRStocks
	pensions := latest.SebPen + latest.Art

	// Convert BTC to EUR using snapshot price or 0 for legacy entries
	cryptoEur := 0.0
	if latest.BtcPrice > 0 {
		cryptoEur = (latest.RBTC + latest.MBTC) * latest.BtcPrice
	} else {
		// Legacy format where RBTC/MBTC were stored as EUR
		cryptoEur = latest.RBTC + latest.MBTC
	}

	period := periodLabel(dateFrom, dateTo)
	report := fmt.Sprintf(`=== REPORTING PERIOD ===
%s
(The balance snapshot, this-month figures, budget status and stock positions below are always current; the transaction summary, top categories and cash flow reflect the period above.)

=== CURRENT BALANCE SNAPSHOT (as of %s) ===
Net Worth:    €%.0f
Free Cash:    €%.0f  (SEB + Swedbank + Luminor + Cash + Revolut)
Investments:  €%.0f  (Swed ETF + Revolut Stocks + IBKR)
Pensions:     €%.0f  (Swed 2nd Pillar + Artea 3rd Pillar)
Crypto (BTC): €%.0f  (Revolut R + M BTC)

=== TRANSACTION SUMMARY (%s) ===
Total Income:     €%.0f
Total Expenses:   €%.0f
Total Invested:   €%.0f
Net Saved:        €%.0f
Savings Rate:     %.1f%%

=== TOP EXPENSE CATEGORIES (%s) ===
%s

=== CATEGORY DETAIL (per category: total, trend vs the previous equal-length period, top labels) ===
%s

=== FIXED MONTHLY OBLIGATIONS (by label: loan, alimony, leasing, counterparty transfers) ===
%s
These are pre-committed, not spending decisions — when judging spending habits, also consider the discretionary picture with these excluded.

=== TOP SPENDING LABELS, DISCRETIONARY (last 12 months) ===
%s

=== INCOME SOURCES (by label, all time) ===
%s

=== MONTHLY CASH FLOW (%s) ===
%s`,
		period,
		latest.Date.Format("2006-01-02"),
		latest.Total, freeCash, investments, pensions, cryptoEur,
		period, summary.TotalIncome, summary.TotalExpenses, summary.TotalInvestments,
		summary.TotalIncome-summary.TotalExpenses, savingsRate,
		period, strings.Join(topExpenses, "\n"),
		s.categoryDetailSection(allTxs, dateFrom, dateTo),
		labelSections.fixedObligations,
		labelSections.topSpendingLabels,
		labelSections.incomeSources,
		period, strings.Join(monthlyLines, "\n"),
	)

	// Daily-status sections: this month so far, live budget status, and live
	// stock positions. Each is best-effort and self-contained — a nil
	// dependency or a market-data hiccup adds nothing rather than failing.
	now := time.Now()
	if sec := s.currentMonthSection(allTxs, now); sec != "" {
		report += "\n\n" + sec
	}
	if sec := s.budgetStatusSection(allTxs, allTime, now); sec != "" {
		report += "\n\n" + sec
	}
	if sec := s.stockPositionsSection(latest); sec != "" {
		report += "\n\n" + sec
	}

	return report, nil
}

// periodLabel renders a human date range for the report header.
func periodLabel(from, to *time.Time) string {
	const f = "2006-01-02"
	switch {
	case from != nil && to != nil:
		return from.Format(f) + " to " + to.Format(f)
	case from != nil:
		return "since " + from.Format(f)
	case to != nil:
		return "through " + to.Format(f)
	default:
		return "all time"
	}
}

// buildPrompt wraps the data report in the per-section advisor instruction —
// the prompt behind "Generate analysis". The output is split into fixed
// sections the UI renders as separate cards, scoped to the selected period.
func (s *insightService) buildPrompt(dateFrom, dateTo *time.Time) (string, error) {
	report, err := s.buildDataReport(dateFrom, dateTo)
	if err != nil {
		return "", err
	}
	return `You are a personal finance advisor reviewing real financial data for a private individual in Lithuania.

` + report + `

Write a personal finance overview as EXACTLY these six sections, each introduced by its heading on its own line, in this order and with these exact headings:

## Transactions
2-4 sentences on spending in the reporting period: total spent, the biggest categories and labels, and any notable shift. Separate fixed obligations from discretionary choices.

## Categories
A detailed category review from the CATEGORY DETAIL data: one "- " bullet per category (up to 8), each naming the category with its EUR total, the trend vs the previous period (call out anything ±30%% or more), what drives it (top labels), and a verdict: fine / watch / act. Bold nothing; keep each bullet to one or two lines.

## Balances
2-3 sentences on net worth and how it is composed (free cash vs investments vs pensions vs crypto), and what stands out.

## Stocks
2-3 sentences on the stock positions and their live market performance (winners/losers, unrealised gains). If there are no positions or no live prices, say so briefly.

## Budget
2-3 sentences on how this month is tracking against the budgets: what is over or at risk, and the safe-to-spend figure.

## Reports
The savings rate and trend, then 2-3 specific actionable opportunities as "- " bullet points, and end with one positive highlight.

Rules: begin the whole response with a single line "Period: <the reporting period>". Use ONLY the six "## " headings above as markup — no other markdown, no bold. Address the person directly as "you". Keep the whole thing under 600 words. If a section genuinely has no data, still emit its heading with one short sentence saying so.`, nil
}

// chatSystemMessage grounds the AI chat in the same data report the
// analysis uses, but leaves the conversation open-ended. Chat always sees
// the full history (no period filter).
func (s *insightService) chatSystemMessage() (string, error) {
	report, err := s.buildDataReport(nil, nil)
	if err != nil {
		return "", err
	}
	system := `You are a personal finance assistant for a private individual in Lithuania. You have their real financial data below — ground every answer in it and quote concrete numbers. You also have read-only tools to query the live database (search_transactions, get_summary, get_balances, …): USE THEM whenever the report below doesn't already contain the exact figures a question needs, instead of estimating. Currency is EUR. Be concise and direct; address the person as "you". Answers render as GitHub-flavored markdown in the app — use bullets, **bold** for key figures, and compact tables when comparing numbers; avoid top-level headings.

CHARTS: when a trend, breakdown or comparison is clearer as a picture — or whenever the user asks to "show", "chart", "graph", "plot" or "visualize" — emit a fenced code block tagged ` + "`chart`" + ` containing ONE JSON object, in addition to a short sentence of text. The app renders it as an interactive chart. Schema:
` + "```chart" + `
{"type":"bar","title":"Spending by category","x":"label","unit":"€","series":[{"name":"Spent","key":"value"}],"data":[{"label":"Food","value":420.5},{"label":"Housing","value":1200}]}
` + "```" + `
Rules: type is one of line|bar|area|pie. "x" names the category/label field in each data row; each series "key" names a numeric field in each data row (pie uses exactly one series). Use real figures from the data or tools — never invent numbers. Keep it to at most ~24 data points and 4 series. Charts are optional: prefer a table for a handful of exact numbers, a chart for trends over time or many categories. Emit at most 2 charts per answer.

` + report
	if ctx := s.userContextBlock(); ctx != "" {
		system += "\n\n" + ctx
	}
	if memo := s.recentAIContext(8, "chat"); memo != "" {
		system += "\n\n" + memo
	}
	return system, nil
}

// fixedObligationLabels marks money that isn't a spending decision (loan,
// alimony, leasing payments and counterparty transfers). Mirrors
// FIXED_LABELS in the frontend (utils/labels.ts).
var fixedObligationLabels = domain.FixedObligationLabels

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

// Internal gateway abstraction (OpenAI-style shapes, kept as the app-wide
// interface; the wire layer below translates to the Anthropic Messages API).
// gatewayMessage is one transcript turn, including the tool-calling
// fields: an assistant turn may carry tool_calls instead of content, and a
// "tool" turn answers one call by id.
type gatewayMessage struct {
	Role       string            `json:"role"`
	Content    string            `json:"content"`
	ToolCalls  []gatewayToolCall `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
}

type gatewayToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // JSON-encoded args
	} `json:"function"`
}

type gatewayTool struct {
	Type     string `json:"type"` // always "function"
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

// ── Anthropic Messages API wire types ─────────────────────────────────
// The gateway is called through nexos.ai's Anthropic-native /messages
// passthrough (docs.nexos.ai/gateway-api/messages): unlike the OpenAI
// translation layer, it preserves cache_control, so the big system report
// and tool definitions are prompt-cached between rounds and turns. The
// internal gatewayMessage/gatewayTool shapes above stay as the app-wide
// abstraction; translation happens only here.

type anthropicCacheControl struct {
	Type string `json:"type"`          // "ephemeral"
	TTL  string `json:"ttl,omitempty"` // "5m" (default) | "1h"
}

// anthropicBlock is one content block — text, tool_use or tool_result
// (fields overlap; unused ones stay empty).
type anthropicBlock struct {
	Type string `json:"type"`
	// text blocks
	Text string `json:"text,omitempty"`
	// tool_use blocks
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result blocks
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`

	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicMessage struct {
	Role    string           `json:"role"` // "user" | "assistant"
	Content []anthropicBlock `json:"content"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    []anthropicBlock   `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
}

type anthropicResponse struct {
	Content    []anthropicBlock `json:"content"`
	StopReason string           `json:"stop_reason"`
	Usage      struct {
		InputTokens        int     `json:"input_tokens"`
		OutputTokens       int     `json:"output_tokens"`
		CacheCreationInput int     `json:"cache_creation_input_tokens"`
		CacheReadInput     int     `json:"cache_read_input_tokens"`
		NexosCreditsCost   float64 `json:"nexos_credits_cost"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func toGatewayMessages(messages []domain.ChatMessage) []gatewayMessage {
	out := make([]gatewayMessage, len(messages))
	for i, m := range messages {
		out[i] = gatewayMessage{Role: m.Role, Content: m.Content}
	}
	return out
}

// callGateway posts one exchange to the configured gateway's Anthropic-native
// Messages endpoint and returns the assistant's reply text — the plain,
// tool-free path used by Generate and TestGateway.
func callGateway(ctx context.Context, settings *domain.AISettings, messages []domain.ChatMessage, maxTokens int) (string, error) {
	msg, err := callGatewayFull(ctx, settings, toGatewayMessages(messages), maxTokens, nil)
	if err != nil {
		return "", err
	}
	// Reasoning models can exhaust max_tokens on hidden reasoning and return
	// empty content — treat that as a failure rather than persisting a blank
	// insight or wedging the chat with an unsendable empty turn.
	if strings.TrimSpace(msg.Content) == "" {
		return "", errors.New("gateway returned an empty reply — try again or raise the model's token limit")
	}
	return msg.Content, nil
}

// callGatewayFull is the tool-aware gateway call over the Anthropic-native
// Messages API: it returns the full assistant message, which may carry
// tool_calls instead of content. The last system block gets a cache_control
// breakpoint, so tools + system (the expensive, stable prefix) are read from
// the prompt cache on tool rounds and follow-up turns.
func callGatewayFull(ctx context.Context, settings *domain.AISettings, messages []gatewayMessage, maxTokens int, tools []gatewayTool) (gatewayMessage, error) {
	var zero gatewayMessage
	if ctx == nil {
		ctx = context.Background()
	}

	// Translate the internal transcript to Anthropic wire form: system turns
	// become top-level system blocks; tool results become tool_result blocks
	// inside a user message (consecutive results share one message, as the
	// API requires them directly after the tool_use turn).
	var system []anthropicBlock
	var wire []anthropicMessage
	appendBlocks := func(role string, blocks ...anthropicBlock) {
		wire = append(wire, anthropicMessage{Role: role, Content: blocks})
	}
	for _, m := range messages {
		switch {
		case m.Role == "system":
			system = append(system, anthropicBlock{Type: "text", Text: m.Content})
		case m.ToolCallID != "":
			block := anthropicBlock{Type: "tool_result", ToolUseID: m.ToolCallID, Content: m.Content}
			if n := len(wire); n > 0 && wire[n-1].Role == "user" && len(wire[n-1].Content) > 0 && wire[n-1].Content[0].Type == "tool_result" {
				wire[n-1].Content = append(wire[n-1].Content, block)
			} else {
				appendBlocks("user", block)
			}
		case m.Role == "assistant" && len(m.ToolCalls) > 0:
			var blocks []anthropicBlock
			if strings.TrimSpace(m.Content) != "" {
				blocks = append(blocks, anthropicBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				input := strings.TrimSpace(tc.Function.Arguments)
				if input == "" {
					input = "{}"
				}
				blocks = append(blocks, anthropicBlock{
					Type: "tool_use", ID: tc.ID, Name: tc.Function.Name, Input: json.RawMessage(input),
				})
			}
			appendBlocks("assistant", blocks...)
		default:
			appendBlocks(m.Role, anthropicBlock{Type: "text", Text: m.Content})
		}
	}
	if len(system) > 0 {
		// One breakpoint caches the whole prefix (tools are serialized before
		// system): 5-minute ephemeral fits the chat/tool-round cadence.
		system[len(system)-1].CacheControl = &anthropicCacheControl{Type: "ephemeral"}
	}
	wireTools := make([]anthropicTool, len(tools))
	for i, t := range tools {
		wireTools[i] = anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		}
	}

	body, err := json.Marshal(anthropicRequest{
		Model:     settings.Model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  wire,
		Tools:     wireTools,
	})
	if err != nil {
		return zero, err
	}

	url := strings.TrimRight(settings.GatewayURL, "/") + "/messages"
	client := &http.Client{Timeout: 120 * time.Second}

	// Rate limits and gateway-side transient failures (429/502/503/529) get
	// a short retry with backoff, honoring Retry-After — one hiccup should
	// not fail a whole analysis, chat turn or labeling scan. Other statuses
	// (including 500) are treated as final: through an LLM gateway they are
	// almost always a real config/request problem, and retrying them would
	// just triple the latency of a genuine failure.
	const maxGatewayAttempts = 3
	var resp *http.Response
	for attempt := 1; ; attempt++ {
		// Bind each request to the caller's context: when the chat's overall
		// budget expires (or the client disconnects) the in-flight gateway
		// call is cancelled instead of running out its own 120s timeout.
		req, rerr := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if rerr != nil {
			return zero, rerr
		}
		req.Header.Set("Authorization", "Bearer "+settings.APIKey)
		req.Header.Set("Content-Type", "application/json")

		var derr error
		resp, derr = client.Do(req)
		if derr != nil {
			return zero, derr
		}
		transient := resp.StatusCode == 429 || resp.StatusCode == 502 ||
			resp.StatusCode == 503 || resp.StatusCode == 529
		if !transient || attempt == maxGatewayAttempts {
			break
		}
		delay := time.Duration(attempt) * 2 * time.Second
		if ra, aerr := strconv.Atoi(resp.Header.Get("Retry-After")); aerr == nil && ra > 0 && ra <= 60 {
			delay = time.Duration(ra) * time.Second
		}
		resp.Body.Close()
		log.Printf("gateway: %s (attempt %d/%d), retrying in %s", resp.Status, attempt, maxGatewayAttempts, delay)
		// A plain sleep would ignore a cancelled context — wait, but wake early.
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(delay):
		}
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, err
	}

	var result anthropicResponse
	jsonErr := json.Unmarshal(respBytes, &result)
	// Surface a structured gateway error message when present (these are the
	// gateway's own words — safe to relay), otherwise a status-only message.
	// The raw upstream body is never reflected: with a user-controlled URL
	// that would be a read primitive against internal endpoints.
	if jsonErr == nil && result.Error != nil && result.Error.Message != "" {
		return zero, fmt.Errorf("gateway error: %s", result.Error.Message)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return zero, fmt.Errorf("gateway returned %s", resp.Status)
	}
	if jsonErr != nil {
		return zero, fmt.Errorf("gateway returned %s but the response was not valid JSON", resp.Status)
	}

	// Cost visibility in the server log: cache reads are the whole point of
	// the native passthrough, so make hits (and misses) observable.
	u := result.Usage
	if u.InputTokens+u.OutputTokens > 0 {
		log.Printf("gateway: in=%d out=%d cache_read=%d cache_write=%d credits=%.5f",
			u.InputTokens, u.OutputTokens, u.CacheReadInput, u.CacheCreationInput, u.NexosCreditsCost)
	}

	var msg gatewayMessage
	msg.Role = "assistant"
	for _, b := range result.Content {
		switch b.Type {
		case "text":
			msg.Content += b.Text
		case "tool_use":
			var tc gatewayToolCall
			tc.ID = b.ID
			tc.Type = "function"
			tc.Function.Name = b.Name
			tc.Function.Arguments = string(b.Input)
			if strings.TrimSpace(tc.Function.Arguments) == "" {
				tc.Function.Arguments = "{}"
			}
			msg.ToolCalls = append(msg.ToolCalls, tc)
		}
	}
	if strings.TrimSpace(msg.Content) == "" && len(msg.ToolCalls) == 0 {
		return zero, errors.New("gateway returned an empty reply — try again or raise the model's token limit")
	}
	// max_tokens means the model hit the output cap mid-answer. Keep the
	// partial text but say so — a silently cut answer reads as complete and
	// wrong, which is worse than a visibly truncated one.
	if result.StopReason == "max_tokens" && strings.TrimSpace(msg.Content) != "" {
		msg.Content += "\n\n⚠ [answer truncated — the model hit its output token limit]"
	}
	return msg, nil
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
