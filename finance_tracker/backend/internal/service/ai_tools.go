package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"sort"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/marketdata"
	"github.com/mindaugas/finance-tracker/internal/repository"
)

// AI chat tools: the same read-only surface the MCP server exposes, executed
// in-process for the web chat's tool-calling loop. The model requests a call,
// the backend runs it against its own services (never raw SQL, never a
// mutation) and feeds the result back. Descriptions mirror cmd/mcp.

// aiToolDataNotes teaches the model the data's semantics once per tool.
const aiToolDataNotes = " Amounts are EUR; 'labels' is a comma-separated multiset of lowercase tags; category 'Transfers' rows are moves between the user's own accounts (not income or spending); fixed-obligation labels (loan, alimony, leasing, evelina) are pre-committed money, not discretionary choices."

// maxToolResultBytes caps any single tool result fed back to the model.
const maxToolResultBytes = 30_000

// maxToolRounds is a pure infinite-spin guard, NOT a practical limit: the
// chat's 240s context budget (checked before every round) is what actually
// bounds a runaway loop and its cost. Deep integrations legitimately chain
// many quick queries — a 12-round cap kept cutting them off mid-work, so the
// ceiling sits far above anything the time budget allows in practice.
const maxToolRounds = 60

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

// accountCodes mirrors the balance columns (applyAccountDelta and the form's
// ACCOUNT_LABELS): the only accounts a transaction may debit/credit. Setting
// one makes SnapshotFromTransaction adjust that account's balance.
var accountCodes = map[string]bool{
	"seb": true, "swed": true, "swed_etf": true, "seb_pen": true,
	"luminor": true, "art": true, "rev_m": true, "rev_r": true,
	"rev_stocks": true, "ibkr_stocks": true, "cash": true,
}

const accountCodeList = "seb, swed, swed_etf, seb_pen, luminor, art, rev_m, rev_r, rev_stocks, ibkr_stocks, cash"

// normalizeAccountCode lowercases/trims an account code; ok=false when it
// isn't a real account ("" is valid and means none).
func normalizeAccountCode(a string) (string, bool) {
	a = strings.ToLower(strings.TrimSpace(a))
	if a == "" {
		return "", true
	}
	return a, accountCodes[a]
}

// webSearchTool declares Anthropic's provider-executed web search, which the
// nexos gateway passes through (verified live). The search runs entirely
// server-side — results come back already woven into the reply.
func webSearchTool() gatewayTool {
	var t gatewayTool
	t.ServerType = "web_search_20250305"
	t.Function.Name = "web_search"
	t.MaxUses = 3
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
			"The budget plan as configured: fixed obligations, investment targets, spending lines and trip budgets with their matchers, amount, period (monthly or yearly) and fund flag. For actual progress against the plan use get_budget_status.",
			obj(map[string]any{})),
		mkTool("get_budget_status",
			"Progress against every budget line for a month. Monthly lines: limit/spent/left this month. Yearly lines (period=yearly): annual amount, spent since 1 January, pace and projection (see `year`). Fund lines (fund=true): money set aside monthly that carries over — budgeted = carried in + this month's share, remaining = available now (see `fund_state`); a big month drawn from a fund is planned, not overspending. Also: 12-month history per line, suggested amounts from the last 12 months, unbudgeted categories, and safe-to-spend. THE tool for 'am I on budget' and 'how much can I still spend' questions.",
			obj(map[string]any{"month": str("YYYY-MM (default: current month)")})),
		mkTool("get_trips",
			"Trips: every trip:… label costed as a whole (dates, days, total net of refunds, per day, breakdown by category and label, optional trip budget and what is left), plus untagged Vacation spending grouped by date as suggested trips. Use for 'what did that holiday cost' questions.",
			obj(map[string]any{})),
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
		// ── Internet & social signal ──────────────────────────────────────
		// Provider-executed live web search (runs server-side through the
		// gateway): current rates, market news, product prices, anything not
		// in the database. Capped per answer to bound cost.
		webSearchTool(),
		mkTool("get_market_buzz",
			"Live news headlines and social-network chatter (Stocktwits, Bluesky) for one stock/ETF ticker, plus the market Fear & Greed reading. Use for sentiment around the user's positions.",
			obj(map[string]any{"ticker": str("ticker symbol, e.g. VWCE, OKLO")}, "ticker")),

		mkTool("record_user_decision",
			"PERSISTENTLY record a decision the user just stated in this conversation — e.g. answering an [OPEN] question from their CFO context, or changing a standing rule. Appends a dated entry to the Decisions log in their context document, which every future AI call reads. Use ONLY when the user explicitly states a decision; NEVER to record your own suggestions or tentative leanings. Confirm to the user what was recorded.",
			obj(map[string]any{
				"decision": str("the decision in one sentence, as the user stated it, e.g. 'Buffer floor revised to €15,000 (5 months × €3,000 critical burn)'"),
			}, "decision")),

		// ── Write tools (label/rule maintenance) ──────────────────────────
		// These MUTATE data. Only ever call them after the user has explicitly
		// approved the specific change in this conversation. Every call is
		// logged to the AI activity feed.
		mkTool("delete_rule",
			"Delete one auto-labeling rule by id (get_label_rules lists ids). Removes the rule only — transactions keep the labels already applied. Use to kill a wrong or duplicate rule. ONLY after the user approves this specific deletion.",
			obj(map[string]any{"rule_id": num("the rule id to delete")}, "rule_id")),
		mkTool("add_rule",
			"Create an auto-labeling rule: it applies 'label' to every transaction whose comment contains 'comment_match' (a leading ^ anchors to the start) and, when set, whose category matches. Immediately labels matching history. ONLY after the user approves.",
			obj(map[string]any{
				"label":         str("the lowercase label to apply"),
				"comment_match": str("substring to match in the comment (≥2 chars; ^ anchors to start)"),
				"category":      str("optional: restrict to this category (empty = any category)"),
			}, "label")),
		mkTool("update_rule",
			"Change an existing rule's comment pattern and/or category (the label is fixed — to change a label, delete and add). Re-applies the rule to history. ONLY after the user approves.",
			obj(map[string]any{
				"rule_id":       num("the rule id to update"),
				"comment_match": str("the new substring pattern (^ anchors to start)"),
				"category":      str("optional new category scope (empty = any)"),
			}, "rule_id")),
		mkTool("retag_transactions",
			"Add and/or remove labels on specific transactions by id (from search_transactions). Fixed-obligation labels (loan, alimony, leasing, evelina) can never be removed. ONLY after the user approves the change.",
			obj(map[string]any{
				"retag": map[string]any{
					"type":        "array",
					"description": "the per-transaction changes",
					"items": obj(map[string]any{
						"id":     num("transaction id"),
						"add":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "labels to add"},
						"remove": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "labels to remove"},
					}, "id"),
				},
			}, "retag")),
		mkTool("get_staged_transactions",
			"Bank rows pulled from Swedbank/SEB over PSD2 that are WAITING FOR THE USER'S APPROVAL — not in the ledger yet, and they only get there when the user presses Add. Each carries the classifier's proposal, the raw bank narrative behind it, and a verdict: new, duplicate_exact, duplicate_content (looks like an existing row), internal (own transfer), needs_review."+aiToolDataNotes,
			obj(map[string]any{
				"state":   str("staged (awaiting review, the default) | imported | dismissed"),
				"verdict": str("new | duplicate_exact | duplicate_content | internal | needs_review"),
				"limit":   num("max rows, default 100, cap 500"),
			})),
		mkTool("get_bank_connections",
			"The user's PSD2 bank connections: status, days until each consent expires, and the accounts on them with their app mapping. Nothing watches consent expiry in the background, so a connection dies quietly unless someone looks.",
			obj(map[string]any{})),
		mkTool("update_staged_transaction",
			"Corrects the proposed category/description/labels/accounts on ONE bank row still awaiting review. Does NOT import it — the row stays in the queue and the user still approves it by hand. Use it to fix rows the classifier misread. The raw bank fields are never touched. ONLY after the user approves the change.",
			obj(map[string]any{
				"id":             num("the staged row's id, from get_staged_transactions"),
				"date":           str("YYYY-MM-DD"),
				"type":           str("expense | income | investment"),
				"category":       str("exact category name"),
				"comment":        str("the description that will appear in the ledger"),
				"labels":         str("comma-separated lowercase tags"),
				"debit_account":  str("account key the money left, e.g. swed, seb, cash"),
				"credit_account": str("account key the money arrived in; omit for an ordinary expense"),
			})),
		mkTool("rename_label",
			"Rename or merge a label everywhere at once — across transactions, rules and budgets. Renaming to an existing label MERGES the two. ONLY after the user approves.",
			obj(map[string]any{
				"from": str("the current label"),
				"to":   str("the new label (existing label = merge)"),
			}, "from", "to")),
		mkTool("create_transaction",
			"Create a REAL new transaction — e.g. from a receipt/screenshot the user attached, or a spending they described. Amounts are EUR and positive. ONLY after the user confirms the details you extracted; show them the fields first. One call per transaction. Setting an account automatically adjusts that account's balance in a new snapshot (skipped for backdated entries) — ask which account the money moved through when it isn't obvious.",
			obj(map[string]any{
				"type":     str("expense | income | investment"),
				"date":     str("YYYY-MM-DD"),
				"amount":   flt("positive amount in EUR"),
				"comment":  str("merchant / description"),
				"category": str("one of the app's categories"),
				"labels":   str("optional comma-separated lowercase labels"),
				"debit_account": str("optional: account the money LEFT (expense paying account; investment/transfer source). " +
					"One of: seb, swed, swed_etf, seb_pen, luminor, art, rev_m, rev_r, rev_stocks, ibkr_stocks, cash"),
				"credit_account": str("optional: account the money ARRIVED at (income destination; investment/transfer target). Same codes"),
			}, "type", "date", "amount", "category")),
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
		// write-tool args
		RuleID       uint   `json:"rule_id"`
		CommentMatch string `json:"comment_match"`
		From         string `json:"from"`
		To           string `json:"to"`
		Retag        []struct {
			ID     uint     `json:"id"`
			Add    []string `json:"add"`
			Remove []string `json:"remove"`
		} `json:"retag"`
		// banking review-queue args
		ID      uint   `json:"id"`
		State   string `json:"state"`
		Verdict string `json:"verdict"`
		// create_transaction args (Date/Type/Category/Comment reuse the fields
		// above where names collide; Amount here is a value, not a filter).
		Date          string  `json:"date"`
		Comment       string  `json:"comment"`
		Amount        float64 `json:"amount"`
		Labels        string  `json:"labels"`
		DebitAccount  string  `json:"debit_account"`
		CreditAccount string  `json:"credit_account"`
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

	case "get_trips":
		trips, suggestions, err := s.Trips()
		if err != nil {
			return "", err
		}
		return marshalToolResult(map[string]any{"trips": trips, "suggestions": suggestions})

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

	case "get_market_buzz":
		ticker := strings.ToUpper(strings.TrimSpace(args.Ticker))
		if ticker == "" {
			return "", fmt.Errorf("ticker is required")
		}
		return marshalToolResult(map[string]any{
			"ticker":      ticker,
			"headlines":   marketdata.Headlines(ticker, 6),
			"social_buzz": marketdata.SocialBuzz(ticker, 6),
			"fear_greed":  marketdata.FearGreed(),
			"note":        "headlines are news-feed titles; social_buzz is public Stocktwits/Bluesky chatter — unverified crowd sentiment, treat as signal, not fact",
		})

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

	case "delete_rule":
		if s.budgetRepo == nil {
			return "", fmt.Errorf("labels unavailable")
		}
		if args.RuleID == 0 {
			return "", fmt.Errorf("rule_id is required")
		}
		res, err := s.ApplyRuleSuggestions([]RuleApplyItem{{Action: "delete", RuleID: args.RuleID}})
		if err != nil {
			return "", err
		}
		if res.Deleted == 0 {
			return "", fmt.Errorf("no rule with id %d", args.RuleID)
		}
		return marshalToolResult(map[string]any{"deleted": res.Deleted})

	case "add_rule":
		if s.budgetRepo == nil {
			return "", fmt.Errorf("labels unavailable")
		}
		res, err := s.ApplyRuleSuggestions([]RuleApplyItem{{
			Action: "add", Label: args.Label, CommentMatch: args.CommentMatch, Category: args.Category,
		}})
		if err != nil {
			return "", err
		}
		if res.Added == 0 {
			return "", fmt.Errorf("rule rejected — check the label, pattern (≥2 chars) and category")
		}
		return marshalToolResult(map[string]any{"added": res.Added, "relabeled": res.Relabeled})

	case "update_rule":
		if s.budgetRepo == nil {
			return "", fmt.Errorf("labels unavailable")
		}
		if args.RuleID == 0 {
			return "", fmt.Errorf("rule_id is required")
		}
		res, err := s.ApplyRuleSuggestions([]RuleApplyItem{{
			Action: "update", RuleID: args.RuleID, CommentMatch: args.CommentMatch, Category: args.Category,
		}})
		if err != nil {
			return "", err
		}
		if res.Updated == 0 {
			return "", fmt.Errorf("rule %d not updated — check the id and pattern", args.RuleID)
		}
		return marshalToolResult(map[string]any{"updated": res.Updated, "relabeled": res.Relabeled})

	case "retag_transactions":
		if len(args.Retag) == 0 {
			return "", fmt.Errorf("retag must list at least one transaction")
		}
		items := make([]LabelApplyItem, len(args.Retag))
		for i, r := range args.Retag {
			items[i] = LabelApplyItem{ID: r.ID, Add: r.Add, Remove: r.Remove}
		}
		applied, err := s.ApplyLabelSuggestions(items)
		if err != nil {
			return "", err
		}
		return marshalToolResult(map[string]any{"retagged": applied})

	case "rename_label":
		if s.budgetRepo == nil {
			return "", fmt.Errorf("labels unavailable")
		}
		from := strings.ToLower(strings.TrimSpace(args.From))
		to := strings.ToLower(strings.TrimSpace(args.To))
		if from == "" || to == "" {
			return "", fmt.Errorf("both from and to are required")
		}
		res, err := s.budgetRepo.RenameLabel(from, to)
		if err != nil {
			return "", err
		}
		s.logAIActivity("tagging", "applied", fmt.Sprintf("renamed label %q → %q (%d transactions)", from, to, res.Transactions))
		return marshalToolResult(map[string]any{"renamed": res.Transactions, "from": from, "to": to})

	case "create_transaction":
		typ := domain.TransactionType(strings.ToLower(strings.TrimSpace(args.Type)))
		if typ != domain.TransactionTypeExpense && typ != domain.TransactionTypeIncome && typ != domain.TransactionTypeInvestment {
			return "", fmt.Errorf("type must be expense, income or investment")
		}
		if args.Amount <= 0 {
			return "", fmt.Errorf("amount must be a positive EUR value")
		}
		cat, ok := normalizeScanCategory(args.Category)
		if !ok {
			return "", fmt.Errorf("unknown category %q — use one of the app's categories", args.Category)
		}
		if _, derr := time.Parse("2006-01-02", strings.TrimSpace(args.Date)); derr != nil {
			return "", fmt.Errorf("date must be YYYY-MM-DD")
		}
		debit, ok := normalizeAccountCode(args.DebitAccount)
		if !ok {
			return "", fmt.Errorf("unknown debit_account %q — valid: %s", args.DebitAccount, accountCodeList)
		}
		credit, ok := normalizeAccountCode(args.CreditAccount)
		if !ok {
			return "", fmt.Errorf("unknown credit_account %q — valid: %s", args.CreditAccount, accountCodeList)
		}
		tx, err := s.txSvc.Create(CreateTransactionInput{
			Date:          strings.TrimSpace(args.Date),
			Type:          typ,
			Amount:        args.Amount,
			Comment:       strings.TrimSpace(args.Comment),
			Category:      domain.Category(cat),
			Labels:        domain.NormalizeLabels(args.Labels),
			DebitAccount:  debit,
			CreditAccount: credit,
		})
		if err != nil {
			return "", fmt.Errorf("creating transaction: %w", err)
		}
		s.logAIActivity("transaction", "created", fmt.Sprintf("created %s €%.2f %q (%s) id=%d debit=%s credit=%s", typ, args.Amount, tx.Comment, cat, tx.ID, debit, credit))
		// Report what actually happened to the balance, not what probably
		// happened: tx.BalanceNote is set by the create path whenever the
		// snapshot was skipped, and is empty when the delta was applied.
		// Guessing here is how the model ends up telling the user their
		// balance moved when it didn't.
		note := tx.BalanceNote
		if note == "" {
			note = "a new balance snapshot was created with the account delta applied"
		}
		return marshalToolResult(map[string]any{
			"created_id": tx.ID, "type": string(typ), "amount": args.Amount,
			"category": cat, "comment": tx.Comment,
			"debit_account": debit, "credit_account": credit, "note": note,
		})

	case "get_staged_transactions", "get_bank_connections", "update_staged_transaction":
		if s.bankRepo == nil {
			return "", fmt.Errorf("no bank is connected to this install")
		}
		switch name {
		case "get_bank_connections":
			return s.chatBankConnections()
		case "get_staged_transactions":
			state := strings.TrimSpace(args.State)
			if state == "" {
				state = domain.StagedStateStaged
			}
			return s.chatStagedRows(state, strings.TrimSpace(args.Verdict), limit)
		default:
			return s.chatUpdateStaged(args.ID, map[string]string{
				"date": args.Date, "type": args.Type, "category": args.Category,
				"comment": args.Comment, "labels": args.Labels,
				"debit_account": args.DebitAccount, "credit_account": args.CreditAccount,
			})
		}

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

// ── banking review queue ────────────────────────────────────────────
//
// These read and amend the staging table directly rather than going out
// through the HTTP API, the same way every other chat tool reaches its
// service. The one thing they deliberately cannot do is commit: putting a row
// in the ledger stays a human action, because approving each row by hand is
// the entire reason the staging queue exists.

// chatStagedRows renders the review queue compactly. The raw bank narrative is
// included — it is what a model needs to tell a misread row from a correct
// one, and it is the only place a card merchant's name survives. Account and
// card numbers inside it are not: see redactIdentifiers.
func (s *insightService) chatStagedRows(state, verdict string, limit int) (string, error) {
	rows, total, err := s.bankRepo.ListStaged(repository.StagedFilter{
		State: state, Verdict: verdict, Page: 1, PageSize: limit,
	})
	if err != nil {
		return "", err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{
			"id": r.ID, "date": r.Date.Format("2006-01-02"),
			"type": r.Type, "amount": r.Amount, "category": r.Category,
			"comment": r.Comment, "labels": r.Labels,
			"debit_account": r.DebitAccount, "credit_account": r.CreditAccount,
			"verdict": r.Verdict, "verdict_note": r.VerdictNote,
			"raw_payee": redactIdentifiers(r.RawPayee), "raw_details": redactIdentifiers(r.RawDetails),
			"raw_direction": r.RawDK, "currency": r.RawCurrency,
		})
	}
	return jsonResult(map[string]any{"total": total, "returned": len(out), "transactions": out})
}

func (s *insightService) chatBankConnections() (string, error) {
	conns, err := s.bankRepo.ListConnections()
	if err != nil {
		return "", err
	}
	out := make([]map[string]any, 0, len(conns))
	for _, c := range conns {
		links, err := s.bankRepo.ListLinks(c.ID)
		if err != nil {
			return "", err
		}
		accs := make([]map[string]any, 0, len(links))
		for _, l := range links {
			accs = append(accs, map[string]any{
				"display_name": l.DisplayName,
				"account_key":  l.AccountKey,
				"last_synced":  formatDayPtr(l.LastSyncedAt),
			})
		}
		out = append(out, map[string]any{
			"bank": c.ASPSPName, "country": c.ASPSPCountry, "status": c.Status,
			"days_until_expiry": int(time.Until(c.ValidUntil).Hours() / 24),
			"accounts":          accs,
		})
	}
	return jsonResult(map[string]any{"connections": out})
}

// chatUpdateStaged patches one row's ledger-bound fields. It mirrors the HTTP
// handler's rules: a row that already left the queue is not editable, the
// verdict is never re-derived — it answers "have I seen this bank row before",
// which retyping a comment does not change — and the user's label rules
// re-run afterwards, so an AI correction earns the same automatic labels a
// correction made in the UI would.
func (s *insightService) chatUpdateStaged(id uint, patch map[string]string) (string, error) {
	if id == 0 {
		return "", fmt.Errorf("id is required — take it from get_staged_transactions")
	}
	rows, err := s.bankRepo.GetStagedByIDs([]uint{id})
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("no staged row with id %d", id)
	}
	row := &rows[0]
	if row.State != domain.StagedStateStaged {
		return "", fmt.Errorf("row %d has already been %s — it is no longer in the review queue", id, row.State)
	}

	before := s.matchedStagedRules(row)
	changed := make([]string, 0, len(patch))
	for field, v := range patch {
		v = strings.TrimSpace(v)
		// An account is the one field where empty is a real value, but the
		// model omits what it does not mean to touch, so a blank is "leave it".
		if v == "" {
			continue
		}
		switch field {
		case "date":
			d, err := time.Parse("2006-01-02", v)
			if err != nil {
				return "", fmt.Errorf("date must be YYYY-MM-DD, got %q", v)
			}
			row.Date = d
		case "type":
			switch domain.TransactionType(v) {
			case domain.TransactionTypeExpense, domain.TransactionTypeIncome, domain.TransactionTypeInvestment:
				row.Type = domain.TransactionType(v)
			default:
				return "", fmt.Errorf("type must be expense, income or investment, got %q", v)
			}
		case "category":
			row.Category = domain.Category(v)
		case "comment":
			row.Comment = v
		case "labels":
			row.Labels = v
		case "debit_account", "credit_account":
			if !domain.IsValidAccountKey(v) {
				return "", fmt.Errorf("unknown account %q", v)
			}
			if field == "debit_account" {
				row.DebitAccount = v
			} else {
				row.CreditAccount = v
			}
		default:
			continue
		}
		changed = append(changed, field)
	}
	if len(changed) == 0 {
		return "", fmt.Errorf("nothing to change — pass at least one field")
	}
	if !containsLabel(changed, "labels") {
		s.reapplyStagedRules(row, before)
	}
	// Same contract as the HTTP edit path: a corrected row is no longer
	// re-classified by later syncs.
	row.Edited = true
	if err := s.bankRepo.SaveStaged(row); err != nil {
		return "", err
	}
	sort.Strings(changed)
	return jsonResult(map[string]any{
		"id": row.ID, "updated": changed,
		"still_awaiting_approval": true,
		"note":                    "Changed the proposal only. The row is still in the review queue — the user adds it to the ledger by hand.",
	})
}

// matchedStagedRules / reapplyStagedRules mirror the HTTP edit path: only a
// rule that starts matching because of this edit may add its label, so a
// label the user deliberately removed is not put back by the next correction.
func (s *insightService) matchedStagedRules(row *domain.BankStagedTx) map[string]bool {
	out := map[string]bool{}
	if s.budgetRepo == nil {
		return out
	}
	rules, err := s.budgetRepo.ListRules()
	if err != nil {
		return out
	}
	probe := &domain.Transaction{Category: row.Category, Comment: row.Comment, Labels: row.Labels}
	for _, rule := range rules {
		if rule.Matches(probe) {
			out[rule.Label+"|"+rule.Category+"|"+rule.CommentMatch] = true
		}
	}
	return out
}

func (s *insightService) reapplyStagedRules(row *domain.BankStagedTx, before map[string]bool) {
	if s.budgetRepo == nil {
		return
	}
	rules, err := s.budgetRepo.ListRules()
	if err != nil {
		return
	}
	probe := &domain.Transaction{Category: row.Category, Comment: row.Comment, Labels: row.Labels}
	for _, rule := range rules {
		if rule.Matches(probe) && !before[rule.Label+"|"+rule.Category+"|"+rule.CommentMatch] {
			probe.AddLabel(rule.Label)
		}
	}
	row.Labels = probe.Labels
}

func formatDayPtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format("2006-01-02")
}

func jsonResult(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
