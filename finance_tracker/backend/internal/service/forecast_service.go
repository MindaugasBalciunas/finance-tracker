package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

// AI investment forecast: one gateway call turns the current portfolio and
// savings history into blended return scenarios, a contribution target, a
// target allocation and a short narrative. The whole document is persisted —
// the dashboard chart is computed client-side from the saved assumptions, so
// looking at the forecast never spends tokens; only an explicit regenerate
// does.

type ForecastScenario struct {
	Name         string  `json:"name"`
	AnnualReturn float64 `json:"annual_return"`
	Rationale    string  `json:"rationale"`
}

type ForecastAllocation struct {
	Bucket     string  `json:"bucket"`
	CurrentPct float64 `json:"current_pct"`
	TargetPct  float64 `json:"target_pct"`
	Action     string  `json:"action"`
}

// ForecastBucket projects one account type on its own: its yearly growth
// rate and the new money flowing into it per month — this is what lets the
// UI show which buckets stay flat and which compound.
type ForecastBucket struct {
	Bucket       string  `json:"bucket"` // free_cash | investments | pensions | crypto
	AnnualReturn float64 `json:"annual_return"`
	MonthlyFlow  float64 `json:"monthly_flow"`
	Note         string  `json:"note"`
}

type forecastAI struct {
	Narrative           string               `json:"narrative"`
	MonthlyContribution float64              `json:"monthly_contribution"`
	Scenarios           []ForecastScenario   `json:"scenarios"`
	BucketProjections   []ForecastBucket     `json:"bucket_projections"`
	TargetAllocation    []ForecastAllocation `json:"target_allocation"`
	Actions             []string             `json:"actions"`
}

type forecastCurrent struct {
	TotalEur          float64 `json:"total_eur"`
	FreeCash          float64 `json:"free_cash"`
	Investments       float64 `json:"investments"`
	Pensions          float64 `json:"pensions"`
	Crypto            float64 `json:"crypto"`
	MonthlyIncomeAvg  float64 `json:"monthly_income_avg"`
	MonthlyExpenseAvg float64 `json:"monthly_expense_avg"`
	MonthlyInvestAvg  float64 `json:"monthly_invest_avg"`
}

type forecastDoc struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Current     forecastCurrent `json:"current"`
	AI          forecastAI      `json:"ai"`
}

// ForecastResult carries the saved document (raw JSON — the handler passes it
// through untouched) plus the usage that generating it cost.
type ForecastResult struct {
	Exists       bool
	Doc          json.RawMessage
	Model        string
	CostUSD      float64
	InputTokens  int
	OutputTokens int
	CreatedAt    time.Time
}

func (s *insightService) Forecast(ctx context.Context, refresh bool) (ForecastResult, error) {
	var zero ForecastResult
	if !refresh {
		saved, err := s.repo.LatestForecast()
		if err != nil {
			return zero, err
		}
		if saved == nil {
			return ForecastResult{Exists: false}, nil
		}
		return ForecastResult{
			Exists: true, Doc: json.RawMessage(saved.Content), Model: saved.Model,
			CostUSD: saved.CostUSD, InputTokens: saved.InputTokens,
			OutputTokens: saved.OutputTokens, CreatedAt: saved.CreatedAt,
		}, nil
	}

	settings, err := s.repo.GetAISettings()
	if err != nil {
		return zero, err
	}
	if err := requireAI(settings); err != nil {
		return zero, err
	}

	current, facts, err := s.forecastFacts()
	if err != nil {
		return zero, err
	}

	prompt := fmt.Sprintf(`You are the user's personal CFO. From the data below, produce an investment forecast for their WHOLE net worth over 1, 5 and 10 years.

Return ONLY a JSON object (no markdown fence, no prose around it) with exactly this shape:
{
  "narrative": "3-5 sentences: where they stand today and the clear path to the 10-year outcome (markdown allowed)",
  "monthly_contribution": 1200,
  "scenarios": [
    {"name": "conservative", "annual_return": 0.03, "rationale": "one sentence"},
    {"name": "expected", "annual_return": 0.06, "rationale": "one sentence"},
    {"name": "optimistic", "annual_return": 0.09, "rationale": "one sentence"}
  ],
  "bucket_projections": [
    {"bucket": "free_cash", "annual_return": 0.0, "monthly_flow": 0, "note": "one clause"},
    {"bucket": "investments", "annual_return": 0.07, "monthly_flow": 900, "note": "one clause"},
    {"bucket": "pensions", "annual_return": 0.05, "monthly_flow": 250, "note": "one clause"},
    {"bucket": "crypto", "annual_return": 0.0, "monthly_flow": 0, "note": "one clause"}
  ],
  "target_allocation": [
    {"bucket": "Free cash", "current_pct": 30, "target_pct": 15, "action": "one imperative sentence"}
  ],
  "actions": ["3-5 concrete next steps"]
}

Rules:
- Be compact: one short sentence per rationale and action, narrative at most 5 sentences — the whole JSON must stay well under 2000 tokens.
- annual_return is the blended yearly growth rate of the WHOLE net worth given its CURRENT allocation drifting toward the target (cash earns ~0%%, broad index funds their long-run rates). Each must be between -0.10 and 0.20, strictly increasing across the three scenarios.
- monthly_contribution: realistic NEW money invested per month, grounded in their savings history below (income minus expenses); never more than their average monthly savings.
- bucket_projections: one entry per bucket the user actually holds, using EXACTLY these ids: free_cash, investments, pensions, crypto. annual_return is that bucket's own growth rate (cash ~0). monthly_flow is new money into that bucket per month (0 for buckets that just sit; pension flows include employer/II-pillar contributions; may be negative when deliberately drawing down). This view shows the user what stays flat and what compounds.
- target_allocation: how the portfolio SHOULD look. Cover at least Free cash, Investments, Pensions%s; target_pct values sum to roughly 100.
- Keep every figure in EUR and grounded in the data — no invented positions.

=== DATA ===
%s`, map[bool]string{true: ", Crypto", false: ""}[current.Crypto > 0], facts)

	msgs := []domain.ChatMessage{}
	if ctxBlock := s.userContextBlock(); ctxBlock != "" {
		msgs = append(msgs, domain.ChatMessage{Role: "system", Content: ctxBlock})
	}
	msgs = append(msgs, domain.ChatMessage{Role: "user", Content: prompt})
	msg, err := callGatewayFull(ctx, settings, toGatewayMessages(msgs), 8192, nil)
	if err != nil {
		return zero, err
	}

	raw := extractJSON(msg.Content, '{', '}')
	if raw == "" {
		return zero, errors.New("gateway reply contained no JSON forecast — try again")
	}
	var ai forecastAI
	if err := json.Unmarshal([]byte(raw), &ai); err != nil {
		// A reply cut off by the output-token cap parses as garbage — name
		// the real cause so "try again" is obviously the fix.
		if strings.Contains(msg.Content, "answer truncated") {
			return zero, errors.New("the model hit its output limit mid-forecast — try again")
		}
		return zero, fmt.Errorf("gateway returned malformed forecast JSON: %w", err)
	}
	if err := validateForecastAI(&ai, current); err != nil {
		return zero, err
	}

	doc := forecastDoc{GeneratedAt: time.Now(), Current: current, AI: ai}
	content, err := json.Marshal(doc)
	if err != nil {
		return zero, err
	}
	rec := &domain.AIForecast{
		Content: string(content), Model: settings.Model, CostUSD: msg.Usage.CostUSD,
		InputTokens: msg.Usage.InputTokens, OutputTokens: msg.Usage.OutputTokens,
	}
	if err := s.repo.SaveForecast(rec); err != nil {
		return zero, err
	}
	s.logAIActivity("forecast", "dashboard", firstLine(ai.Narrative))
	return ForecastResult{
		Exists: true, Doc: json.RawMessage(content), Model: rec.Model,
		CostUSD: rec.CostUSD, InputTokens: rec.InputTokens,
		OutputTokens: rec.OutputTokens, CreatedAt: rec.CreatedAt,
	}, nil
}

// forecastFacts assembles the numbers the model reasons from: the latest
// balance snapshot bucketed the same way the dashboard groups accounts, the
// last 12 complete months of cash flow, and the open stock positions.
func (s *insightService) forecastFacts() (forecastCurrent, string, error) {
	var cur forecastCurrent
	latest, err := s.balSvc.GetLatest(0)
	if err != nil || latest == nil {
		return cur, "", errors.New("no balance snapshot yet — add balances before forecasting")
	}
	cur.TotalEur = latest.Total
	cur.FreeCash = latest.Seb + latest.Swed + latest.Luminor + latest.Cash + latest.RevM + latest.RevR
	cur.Investments = latest.SwedETF + latest.RevStocks + latest.IBKRStocks
	cur.Pensions = latest.SebPen + latest.Art
	cur.Crypto = latest.RBTC + latest.MBTC
	if latest.BtcPrice > 0 {
		cur.Crypto = (latest.RBTC + latest.MBTC) * latest.BtcPrice
	}

	summary, err := s.txSvc.GetSummary(domain.TransactionFilter{})
	if err != nil {
		return cur, "", err
	}
	now := time.Now()
	months := summary.ByMonth
	complete := months[:0:0]
	for _, m := range months {
		if m.Year == now.Year() && m.Month == int(now.Month()) {
			continue
		}
		complete = append(complete, m)
	}
	if len(complete) > 12 {
		complete = complete[len(complete)-12:]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Net worth today: €%.0f — free cash €%.0f, investments €%.0f, pensions €%.0f, crypto €%.0f (snapshot %s)\n",
		cur.TotalEur, cur.FreeCash, cur.Investments, cur.Pensions, cur.Crypto, latest.Date.Format("2006-01-02"))
	if len(complete) > 0 {
		var inc, exp, inv float64
		b.WriteString("\nMonthly cash flow (last complete months):\n")
		for _, m := range complete {
			fmt.Fprintf(&b, "  - %04d-%02d: income €%.0f, expenses €%.0f, invested €%.0f\n", m.Year, m.Month, m.Income, m.Expenses, m.Investments)
			inc += m.Income
			exp += m.Expenses
			inv += m.Investments
		}
		n := float64(len(complete))
		cur.MonthlyIncomeAvg = inc / n
		cur.MonthlyExpenseAvg = exp / n
		cur.MonthlyInvestAvg = inv / n
		fmt.Fprintf(&b, "Averages: income €%.0f/mo, expenses €%.0f/mo, invested €%.0f/mo → savings capacity €%.0f/mo\n",
			cur.MonthlyIncomeAvg, cur.MonthlyExpenseAvg, cur.MonthlyInvestAvg, cur.MonthlyIncomeAvg-cur.MonthlyExpenseAvg)
	}
	if sec := s.stockPositionsSection(latest); sec != "" {
		b.WriteString("\n" + sec)
	}
	return cur, b.String(), nil
}

// validateForecastAI keeps a hallucinated or malformed reply out of the saved
// document: sane scenario count and rates, contribution within the user's
// actual means, percentages in range.
func validateForecastAI(ai *forecastAI, cur forecastCurrent) error {
	if strings.TrimSpace(ai.Narrative) == "" {
		return errors.New("forecast narrative missing")
	}
	if len(ai.Scenarios) < 2 || len(ai.Scenarios) > 4 {
		return fmt.Errorf("expected 3 scenarios, got %d", len(ai.Scenarios))
	}
	for _, sc := range ai.Scenarios {
		if strings.TrimSpace(sc.Name) == "" {
			return errors.New("scenario missing a name")
		}
		if sc.AnnualReturn < -0.5 || sc.AnnualReturn > 0.5 {
			return fmt.Errorf("scenario %q return %.2f out of range", sc.Name, sc.AnnualReturn)
		}
	}
	sort.Slice(ai.Scenarios, func(i, j int) bool { return ai.Scenarios[i].AnnualReturn < ai.Scenarios[j].AnnualReturn })
	// Contribution stays within the user's demonstrated savings capacity
	// (floored at zero — negative capacity means nothing to invest).
	capacity := cur.MonthlyIncomeAvg - cur.MonthlyExpenseAvg
	if capacity < 0 {
		capacity = 0
	}
	if ai.MonthlyContribution < 0 {
		ai.MonthlyContribution = 0
	}
	if ai.MonthlyContribution > capacity {
		ai.MonthlyContribution = capacity
	}
	if ai.MonthlyContribution == 0 && cur.MonthlyInvestAvg > 0 {
		ai.MonthlyContribution = cur.MonthlyInvestAvg
	}
	for i := range ai.TargetAllocation {
		a := &ai.TargetAllocation[i]
		a.CurrentPct = clampPct(a.CurrentPct)
		a.TargetPct = clampPct(a.TargetPct)
	}
	// Bucket projections are optional (older prompts, terse models); keep
	// only the known bucket ids with sane rates and flows.
	validBuckets := map[string]bool{"free_cash": true, "investments": true, "pensions": true, "crypto": true}
	kept := ai.BucketProjections[:0:0]
	for _, b := range ai.BucketProjections {
		if !validBuckets[b.Bucket] {
			continue
		}
		if b.AnnualReturn < -0.5 {
			b.AnnualReturn = -0.5
		}
		if b.AnnualReturn > 0.5 {
			b.AnnualReturn = 0.5
		}
		if b.MonthlyFlow < -20000 {
			b.MonthlyFlow = -20000
		}
		if b.MonthlyFlow > 20000 {
			b.MonthlyFlow = 20000
		}
		kept = append(kept, b)
	}
	ai.BucketProjections = kept
	return nil
}

func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// firstLine trims a narrative down to a one-line activity digest.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}
