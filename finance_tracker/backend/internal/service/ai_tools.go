package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

// AI chat tools: the same read-only surface the MCP server exposes, executed
// in-process for the web chat's tool-calling loop. The model requests a call,
// the backend runs it against its own services (never raw SQL, never a
// mutation) and feeds the result back. Descriptions mirror cmd/mcp.

// aiToolDataNotes teaches the model the data's semantics once per tool.
const aiToolDataNotes = " Amounts are EUR; 'labels' is a comma-separated multiset of lowercase tags; category 'Transfers' rows are moves between the user's own accounts (not income or spending); fixed-obligation labels (loan, alimony, leasing, evelina) are pre-committed money, not discretionary choices."

// maxToolResultBytes caps any single tool result fed back to the model.
const maxToolResultBytes = 30_000

// maxToolRounds bounds the agentic loop — a model that keeps asking for
// tools past this gets cut off rather than looping forever.
const maxToolRounds = 6

func obj(props map[string]any, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func str(desc string) map[string]any   { return map[string]any{"type": "string", "description": desc} }
func flt(desc string) map[string]any   { return map[string]any{"type": "number", "description": desc} }
func num(desc string) map[string]any   { return map[string]any{"type": "integer", "description": desc} }
func boolp(desc string) map[string]any { return map[string]any{"type": "boolean", "description": desc} }

func mkTool(name, description string, params map[string]any) gatewayTool {
	var t gatewayTool
	t.Type = "function"
	t.Function.Name = name
	t.Function.Description = description
	t.Function.Parameters = params
	return t
}

// chatTools declares the tool surface offered to the web chat model.
func chatTools() []gatewayTool {
	return []gatewayTool{
		mkTool("get_overview",
			"Full financial overview report: balances/net worth, transaction summaries, top categories, fixed vs discretionary labels, this month vs budgets with safe-to-spend, stock positions with live prices. Call this first for broad questions."+aiToolDataNotes,
			obj(map[string]any{})),
		mkTool("search_transactions",
			"Search individual transactions with filters. Returns up to 'limit' rows (default 100, max 500) plus the total match count."+aiToolDataNotes,
			obj(map[string]any{
				"query":      str("case-insensitive substring of the comment (merchant, description)"),
				"label":      str("label filter; comma list matches ANY of the labels"),
				"all_of":     boolp("when true, a comma list of labels must ALL be present"),
				"category":   str("exact category name, e.g. Food, Housing, Salary"),
				"type":       str("expense | income | investment"),
				"date_from":  str("YYYY-MM-DD inclusive"),
				"date_to":    str("YYYY-MM-DD inclusive"),
				"amount_min": flt("only rows with amount >= this (EUR)"),
				"amount_max": flt("only rows with amount <= this (EUR)"),
				"sort":       str("date (default) | amount | comment"),
				"dir":        str("asc | desc"),
				"limit":      num("max rows, default 100, cap 500"),
			})),
		mkTool("get_summary",
			"Aggregated totals for any filter combination: income, expenses, investments, plus per-category, per-month AND per-label breakdowns. Accepts the same filters as search_transactions (dates, category, label, amount range). Prefer this over search_transactions for sums, trends and 'where does money go' questions.",
			obj(map[string]any{
				"date_from":  str("YYYY-MM-DD inclusive"),
				"date_to":    str("YYYY-MM-DD inclusive"),
				"category":   str("scope to one category"),
				"label":      str("scope to one label (comma list = any of them)"),
				"amount_min": flt("only rows with amount >= this (EUR)"),
				"amount_max": flt("only rows with amount <= this (EUR)"),
			})),
		mkTool("get_label_stats",
			"Every label's footprint: transaction count, EUR volume, rules/budget usage, first/last use.",
			obj(map[string]any{"limit": num("max labels, default 100")})),
		mkTool("get_budgets",
			"The monthly budget plan as configured: fixed obligations, investment targets and spending limits with their matchers and amounts. For actual progress against the plan use get_budget_status.",
			obj(map[string]any{})),
		mkTool("get_budget_status",
			"Month-to-date progress against every budget line: budgeted vs spent vs remaining, fixed/investment totals, discretionary spending and safe-to-spend. THE tool for 'am I on budget' and 'how much can I still spend' questions.",
			obj(map[string]any{"month": str("YYYY-MM (default: current month)")})),
		mkTool("get_label_rules",
			"The auto-labeling rules: each rule applies its label to transactions whose comment contains the pattern (^ anchors to the start) and category matches. Explains WHY rows carry a label.",
			obj(map[string]any{})),
		mkTool("get_balances",
			"Account balance snapshots over time (per-account net worth history). Optional date range; capped at 200 snapshots.",
			obj(map[string]any{
				"date_from": str("YYYY-MM-DD inclusive"),
				"date_to":   str("YYYY-MM-DD inclusive"),
			})),
		mkTool("get_stock_portfolio",
			"Stock holdings from the trade ledger: shares, average cost, cost basis, realized gains per ticker (cost basis only; use get_stock_quote for live prices).",
			obj(map[string]any{})),
		mkTool("get_stock_trades",
			"Individual stock trades (buys/sells), newest first: date, action, ticker, shares, price, source. Optional ticker filter.",
			obj(map[string]any{
				"ticker": str("filter to one ticker, e.g. VWCE"),
				"limit":  num("max trades, default 100, cap 500"),
			})),
		mkTool("get_stock_quote",
			"Live market price for one ticker (Yahoo Finance, cached ~5 min).",
			obj(map[string]any{"ticker": str("ticker symbol, e.g. VWCE or AAPL")}, "ticker")),
		mkTool("get_assets",
			"Physical assets (real estate, vehicles, solar): valuations, loan balances, interest structure and net equity.",
			obj(map[string]any{})),
		mkTool("record_user_decision",
			"PERSISTENTLY record a decision the user just stated in this conversation — e.g. answering an [OPEN] question from their CFO context, or changing a standing rule. Appends a dated entry to the Decisions log in their context document, which every future AI call reads. Use ONLY when the user explicitly states a decision; NEVER to record your own suggestions or tentative leanings. Confirm to the user what was recorded.",
			obj(map[string]any{
				"decision": str("the decision in one sentence, as the user stated it, e.g. 'Buffer floor revised to €15,000 (5 months × €3,000 critical burn)'"),
			}, "decision")),
	}
}

// runChatTool executes one tool call. Every branch is read-only; errors come
// back as strings so the model can see what went wrong and adjust.
func (s *insightService) runChatTool(name string, rawArgs string) (string, error) {
	var args struct {
		Query     string   `json:"query"`
		Label     string   `json:"label"`
		AllOf     bool     `json:"all_of"`
		Category  string   `json:"category"`
		Type      string   `json:"type"`
		DateFrom  string   `json:"date_from"`
		DateTo    string   `json:"date_to"`
		AmountMin *float64 `json:"amount_min"`
		AmountMax *float64 `json:"amount_max"`
		Sort      string   `json:"sort"`
		Dir       string   `json:"dir"`
		Limit     int      `json:"limit"`
		Ticker    string   `json:"ticker"`
		Month     string   `json:"month"`
		Decision  string   `json:"decision"`
	}
	if rawArgs != "" {
		if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
			return "", fmt.Errorf("invalid tool arguments: %w", err)
		}
	}
	limit := args.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	parseDay := func(v string) *time.Time {
		if v == "" {
			return nil
		}
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			return nil
		}
		return &t
	}

	switch name {
	case "get_overview":
		return s.buildDataReport(nil, nil)

	case "search_transactions":
		filter := domain.TransactionFilter{
			DateFrom:  parseDay(args.DateFrom),
			DateTo:    parseDay(args.DateTo),
			Label:     strings.ToLower(strings.TrimSpace(args.Label)),
			Search:    strings.TrimSpace(args.Query),
			AmountMin: args.AmountMin,
			AmountMax: args.AmountMax,
			Sort:      args.Sort,
			Dir:       args.Dir,
			Page:      1,
			PageSize:  limit,
		}
		if args.AllOf {
			filter.LabelMode = "all"
		}
		if args.Type != "" {
			t := domain.TransactionType(args.Type)
			filter.Type = &t
		}
		if args.Category != "" {
			cat := domain.Category(args.Category)
			filter.Category = &cat
		}
		page, err := s.txSvc.List(filter)
		if err != nil {
			return "", err
		}
		type row struct {
			Date     string  `json:"date"`
			Type     string  `json:"type"`
			Amount   float64 `json:"amount_eur"`
			Category string  `json:"category"`
			Comment  string  `json:"comment"`
			Labels   string  `json:"labels,omitempty"`
		}
		rows := make([]row, len(page.Data))
		for i, tx := range page.Data {
			rows[i] = row{
				Date: tx.Date.Format("2006-01-02"), Type: string(tx.Type), Amount: tx.Amount,
				Category: string(tx.Category), Comment: tx.Comment, Labels: tx.Labels,
			}
		}
		return marshalToolResult(map[string]any{"total_matches": page.Total, "returned": len(rows), "rows": rows})

	case "get_summary":
		filter := domain.TransactionFilter{
			DateFrom:  parseDay(args.DateFrom),
			DateTo:    parseDay(args.DateTo),
			Label:     strings.ToLower(strings.TrimSpace(args.Label)),
			AmountMin: args.AmountMin,
			AmountMax: args.AmountMax,
		}
		if args.Category != "" {
			cat := domain.Category(args.Category)
			filter.Category = &cat
		}
		summary, err := s.txSvc.GetSummary(filter)
		if err != nil {
			return "", err
		}
		return marshalToolResult(summary)

	case "get_label_stats":
		if s.budgetRepo == nil {
			return "", fmt.Errorf("label stats unavailable")
		}
		stats, err := s.budgetRepo.LabelStats()
		if err != nil {
			return "", err
		}
		if len(stats) > limit {
			stats = stats[:limit]
		}
		return marshalToolResult(stats)

	case "get_budgets":
		if s.budgetRepo == nil {
			return "", fmt.Errorf("budgets unavailable")
		}
		budgets, err := s.budgetRepo.ListBudgets()
		if err != nil {
			return "", err
		}
		return marshalToolResult(budgets)

	case "get_budget_status":
		year, month := 0, 0
		if args.Month != "" {
			t, err := time.Parse("2006-01", args.Month)
			if err != nil {
				return "", fmt.Errorf("month must be YYYY-MM")
			}
			year, month = t.Year(), int(t.Month())
		}
		status, err := s.BudgetStatus(year, month)
		if err != nil {
			return "", err
		}
		return marshalToolResult(status)

	case "get_label_rules":
		if s.budgetRepo == nil {
			return "", fmt.Errorf("label rules unavailable")
		}
		rules, err := s.budgetRepo.ListRules()
		if err != nil {
			return "", err
		}
		return marshalToolResult(rules)

	case "get_stock_trades":
		if s.stockSvc == nil {
			return "", fmt.Errorf("stocks unavailable")
		}
		trades, err := s.stockSvc.ListAll()
		if err != nil {
			return "", err
		}
		ticker := strings.ToUpper(strings.TrimSpace(args.Ticker))
		var kept []domain.StockTrade
		for _, tr := range trades {
			if ticker == "" || strings.EqualFold(tr.Ticker, ticker) {
				kept = append(kept, tr)
			}
		}
		// Newest first, capped.
		for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
			kept[i], kept[j] = kept[j], kept[i]
		}
		if len(kept) > limit {
			kept = kept[:limit]
		}
		return marshalToolResult(map[string]any{"total": len(trades), "returned": len(kept), "trades": kept})

	case "get_balances":
		balances, err := s.balSvc.List(domain.BalanceFilter{DateFrom: parseDay(args.DateFrom), DateTo: parseDay(args.DateTo)}, 0)
		if err != nil {
			return "", err
		}
		if len(balances) > 200 {
			balances = balances[len(balances)-200:] // newest snapshots win
		}
		return marshalToolResult(balances)

	case "get_stock_portfolio":
		if s.stockSvc == nil {
			return "", fmt.Errorf("stocks unavailable")
		}
		portfolio, err := s.stockSvc.GetPortfolio()
		if err != nil {
			return "", err
		}
		return marshalToolResult(portfolio)

	case "get_stock_quote":
		t := strings.ToUpper(strings.TrimSpace(args.Ticker))
		if t == "" || len(t) > 12 {
			return "", fmt.Errorf("ticker must be a short symbol like VWCE")
		}
		if s.quote == nil {
			return "", fmt.Errorf("quotes unavailable")
		}
		q, err := s.quote(t)
		if err != nil {
			return "", err
		}
		return marshalToolResult(q)

	case "get_assets":
		if s.assetSvc == nil {
			return "", fmt.Errorf("assets unavailable")
		}
		assets, err := s.assetSvc.ListAll()
		if err != nil {
			return "", err
		}
		summary, err := s.assetSvc.GetSummary()
		if err != nil {
			return "", err
		}
		return marshalToolResult(map[string]any{"assets": assets, "summary": summary})

	case "record_user_decision":
		// The ONLY writing tool in the chat loop, and deliberately narrow:
		// it can append a dated line to the user's own context document,
		// nothing else. Not exposed via MCP (the API token is GET-only).
		decision := clipText(args.Decision, 300)
		if decision == "" {
			return "", fmt.Errorf("decision text is required")
		}
		ctx, err := s.repo.GetAIContext()
		if err != nil {
			return "", err
		}
		content := strings.TrimRight(ctx.Content, "\n")
		const logHeader = "## Decisions log (recorded from chat at the user's request)"
		if !strings.Contains(content, logHeader) {
			if content != "" {
				content += "\n\n"
			}
			content += logHeader
		}
		content += "\n- " + time.Now().Format("2006-01-02") + ": " + decision
		if _, err := s.SaveAIContext(content); err != nil {
			return "", err
		}
		s.logAIActivity("decision", "", decision)
		return marshalToolResult(map[string]any{"recorded": decision, "note": "appended to the user's context document — all future AI calls will see it"})

	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

// marshalToolResult renders a tool result as JSON, truncated to the cap so a
// broad query can't flood the model context.
func marshalToolResult(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	if len(b) > maxToolResultBytes {
		return string(b[:maxToolResultBytes]) + `… (truncated — narrow the query with filters or a smaller limit)`, nil
	}
	return string(b), nil
}
