// finance-tracker-mcp is an MCP server exposing the Finance Tracker API as
// typed tools for MCP clients (Claude Desktop, Claude Code, or any gateway
// that attaches MCP servers).
//
// Two token scopes, selected by whichever token you put in FT_API_TOKEN:
//   - A read-only token (ftk_…) — the default and safe choice — can drive only
//     the read (GET) tools. The backend rejects it on any write, so even a
//     compromised or confused client cannot mutate data or read backups,
//     settings or the AI key.
//   - A read-write token (ftkw_…) additionally unlocks the write tools
//     (add_rule, update_rule, delete_rule, rename_label, retag_transactions),
//     which MUTATE your labels and auto-labeling rules. Mint it only when you
//     want the model to edit them, in Security → read-write API token.
//
// Regardless of scope:
//   - No database access: every tool is an HTTP call against the app's REST
//     API. The backend enforces the token's scope + a route allowlist
//     server-side, so a client can never exceed what the token permits.
//   - The token comes from the FT_API_TOKEN environment variable and is never
//     logged or echoed into tool output.
//   - Every list tool caps its result size, so a broad query cannot dump the
//     whole ledger into a model context.
//
// Configuration (environment):
//
//	FT_API_URL   base URL of the API, e.g. http://homeassistant.local:8098/api/v1
//	FT_API_TOKEN the token minted in Security → API token: read-only (ftk_…) by
//	             default, or read-write (ftkw_…) to enable the write tools
//
// Claude Desktop / Claude Code config example:
//
//	{"mcpServers": {"finance-tracker": {
//	  "command": "/path/to/finance-tracker-mcp",
//	  "env": {"FT_API_URL": "http://homeassistant.local:8098/api/v1",
//	          "FT_API_TOKEN": "ftk_..."}}}}
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	apiURL   string
	apiToken string
	client   = &http.Client{Timeout: 30 * time.Second}
)

// apiGET fetches an allowlisted API route and returns the raw JSON body.
func apiGET(path string, query url.Values) ([]byte, error) {
	u := apiURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("finance tracker API unreachable at %s: %w", apiURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4 MB hard cap
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return body, nil
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("API token rejected — mint a new one in the app under Security → API token")
	case http.StatusForbidden:
		return nil, fmt.Errorf("route not permitted for the read-only token")
	default:
		return nil, fmt.Errorf("API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
}

// apiWrite calls an allowlisted write route with an optional JSON body and
// returns the raw JSON response. Only a read-write token (ftkw_) reaches these
// routes; a read-only token is rejected server-side with 403, surfaced here as
// a clear "mint a read-write token" message.
func apiWrite(method, path string, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, apiURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("finance tracker API unreachable at %s: %w", apiURL, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4 MB hard cap
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return respBody, nil
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("API token rejected — mint a new one in the app under Security → API token")
	case http.StatusForbidden:
		return nil, fmt.Errorf("this token can't perform writes — mint a read-write token (ftkw_) in the app under Security → read-write API token")
	default:
		return nil, fmt.Errorf("API returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}
}

func textResult(body []byte) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}
}

// clampLimit bounds list sizes: default 100 rows, never more than 500.
func clampLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}

// ── Tool inputs ──────────────────────────────────────────────────────

type emptyArgs struct{}

type searchTransactionsArgs struct {
	Query     string   `json:"query,omitempty" jsonschema:"case-insensitive substring of the transaction comment (merchant, description)"`
	Label     string   `json:"label,omitempty" jsonschema:"label filter; comma list matches ANY of the labels (labels are lowercase tags like 'groceries' or 'loan')"`
	AllOf     bool     `json:"all_of,omitempty" jsonschema:"when true a comma list of labels must ALL be present on a row"`
	Category  string   `json:"category,omitempty" jsonschema:"exact category name, e.g. Food, Housing, Transport, Salary"`
	Type      string   `json:"type,omitempty" jsonschema:"expense | income | investment"`
	DateFrom  string   `json:"date_from,omitempty" jsonschema:"YYYY-MM-DD inclusive"`
	DateTo    string   `json:"date_to,omitempty" jsonschema:"YYYY-MM-DD inclusive"`
	AmountMin *float64 `json:"amount_min,omitempty" jsonschema:"only rows with amount >= this (EUR)"`
	AmountMax *float64 `json:"amount_max,omitempty" jsonschema:"only rows with amount <= this (EUR)"`
	Sort      string   `json:"sort,omitempty" jsonschema:"date (default) | amount | comment"`
	Dir       string   `json:"dir,omitempty" jsonschema:"asc | desc"`
	Limit     int      `json:"limit,omitempty" jsonschema:"max rows to return, default 100, cap 500"`
}

type summaryArgs struct {
	DateFrom  string   `json:"date_from,omitempty" jsonschema:"YYYY-MM-DD inclusive"`
	DateTo    string   `json:"date_to,omitempty" jsonschema:"YYYY-MM-DD inclusive"`
	Category  string   `json:"category,omitempty" jsonschema:"scope to one category"`
	Label     string   `json:"label,omitempty" jsonschema:"scope to one label (comma list = any of them)"`
	AmountMin *float64 `json:"amount_min,omitempty" jsonschema:"only rows with amount >= this (EUR)"`
	AmountMax *float64 `json:"amount_max,omitempty" jsonschema:"only rows with amount <= this (EUR)"`
}

type budgetStatusArgs struct {
	Month string `json:"month,omitempty" jsonschema:"YYYY-MM (default: current month)"`
}

type tradesArgs struct {
	Ticker string `json:"ticker,omitempty" jsonschema:"filter to one ticker, e.g. VWCE"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max trades to return, default 100, cap 500"`
}

type labelStatsArgs struct {
	Limit int `json:"limit,omitempty" jsonschema:"max labels to return, default 100, cap 500"`
}

type balancesArgs struct {
	DateFrom string `json:"date_from,omitempty" jsonschema:"YYYY-MM-DD inclusive"`
	DateTo   string `json:"date_to,omitempty" jsonschema:"YYYY-MM-DD inclusive"`
}

type quoteArgs struct {
	Ticker string `json:"ticker" jsonschema:"stock ticker symbol, e.g. VWCE or AAPL"`
}

// ── Write tool inputs (read-write token only) ────────────────────────

type deleteRuleArgs struct {
	RuleID int `json:"rule_id" jsonschema:"numeric id of the auto-labeling rule to delete (from get_label_rules)"`
}

type addRuleArgs struct {
	Label        string `json:"label" jsonschema:"label the new rule assigns (lowercase tag, e.g. groceries)"`
	CommentMatch string `json:"comment_match" jsonschema:"substring matched against the transaction comment; a leading ^ anchors to the start"`
	Category     string `json:"category,omitempty" jsonschema:"optional category the rule also requires to match, e.g. Food (empty = any category)"`
}

type updateRuleArgs struct {
	RuleID       int    `json:"rule_id" jsonschema:"numeric id of the rule to edit (from get_label_rules)"`
	CommentMatch string `json:"comment_match,omitempty" jsonschema:"new comment match pattern; a leading ^ anchors to the start"`
	Category     string `json:"category,omitempty" jsonschema:"new category the rule requires to match (empty = any category)"`
}

type retagItem struct {
	ID     int      `json:"id" jsonschema:"transaction id to edit (from search_transactions)"`
	Add    []string `json:"add,omitempty" jsonschema:"labels to add to this transaction"`
	Remove []string `json:"remove,omitempty" jsonschema:"labels to remove from this transaction"`
}

type retagArgs struct {
	Items []retagItem `json:"items" jsonschema:"per-transaction label edits; each names a transaction id plus labels to add and/or remove"`
}

type renameLabelArgs struct {
	From string `json:"from" jsonschema:"existing label to rename"`
	To   string `json:"to" jsonschema:"new label name; merges into it if it already exists"`
}

type createTransactionArgs struct {
	Type     string  `json:"type" jsonschema:"expense | income | investment"`
	Date     string  `json:"date" jsonschema:"YYYY-MM-DD"`
	Amount   float64 `json:"amount" jsonschema:"positive amount in EUR"`
	Comment  string  `json:"comment" jsonschema:"merchant / short description"`
	Category string  `json:"category" jsonschema:"one of the app's categories (see get_label_stats / the overview for valid names)"`
	Labels   string  `json:"labels" jsonschema:"optional comma-separated lowercase labels"`
	// Setting an account adjusts its balance in a new snapshot.
	DebitAccount  string `json:"debit_account" jsonschema:"optional: account the money left — seb|swed|swed_etf|seb_pen|luminor|art|rev_m|rev_r|rev_stocks|ibkr_stocks|cash"`
	CreditAccount string `json:"credit_account" jsonschema:"optional: account the money arrived at — same codes"`
}

// ── Tool handlers ────────────────────────────────────────────────────

func handleOverview(ctx context.Context, req *mcp.CallToolRequest, _ emptyArgs) (*mcp.CallToolResult, any, error) {
	body, err := apiGET("/ai/report", nil)
	if err != nil {
		return nil, nil, err
	}
	var out struct {
		Report string `json:"report"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, nil, err
	}
	return textResult([]byte(out.Report)), nil, nil
}

func handleSearchTransactions(ctx context.Context, req *mcp.CallToolRequest, a searchTransactionsArgs) (*mcp.CallToolResult, any, error) {
	q := url.Values{}
	set := func(k, v string) {
		if v != "" {
			q.Set(k, v)
		}
	}
	set("search", a.Query)
	set("label", a.Label)
	if a.AllOf {
		q.Set("label_mode", "all")
	}
	set("category", a.Category)
	set("type", a.Type)
	set("date_from", a.DateFrom)
	set("date_to", a.DateTo)
	if a.AmountMin != nil {
		q.Set("amount_min", fmt.Sprint(*a.AmountMin))
	}
	if a.AmountMax != nil {
		q.Set("amount_max", fmt.Sprint(*a.AmountMax))
	}
	set("sort", a.Sort)
	set("dir", a.Dir)
	q.Set("page", "1")
	q.Set("page_size", fmt.Sprint(clampLimit(a.Limit)))
	body, err := apiGET("/transactions", q)
	if err != nil {
		return nil, nil, err
	}
	return textResult(body), nil, nil
}

func handleSummary(ctx context.Context, req *mcp.CallToolRequest, a summaryArgs) (*mcp.CallToolResult, any, error) {
	q := url.Values{}
	if a.DateFrom != "" {
		q.Set("date_from", a.DateFrom)
	}
	if a.DateTo != "" {
		q.Set("date_to", a.DateTo)
	}
	if a.Category != "" {
		q.Set("category", a.Category)
	}
	if a.Label != "" {
		q.Set("label", a.Label)
	}
	if a.AmountMin != nil {
		q.Set("amount_min", fmt.Sprint(*a.AmountMin))
	}
	if a.AmountMax != nil {
		q.Set("amount_max", fmt.Sprint(*a.AmountMax))
	}
	body, err := apiGET("/transactions/summary", q)
	if err != nil {
		return nil, nil, err
	}
	return textResult(body), nil, nil
}

func handleLabelStats(ctx context.Context, req *mcp.CallToolRequest, a labelStatsArgs) (*mcp.CallToolResult, any, error) {
	body, err := apiGET("/labels/stats", nil)
	if err != nil {
		return nil, nil, err
	}
	// Trim client-side: the endpoint returns every label.
	var stats []map[string]any
	if err := json.Unmarshal(body, &stats); err == nil {
		if n := clampLimit(a.Limit); len(stats) > n {
			stats = stats[:n]
		}
		if trimmed, err := json.Marshal(stats); err == nil {
			body = trimmed
		}
	}
	return textResult(body), nil, nil
}

func handleBudgets(ctx context.Context, req *mcp.CallToolRequest, _ emptyArgs) (*mcp.CallToolResult, any, error) {
	body, err := apiGET("/budgets", nil)
	if err != nil {
		return nil, nil, err
	}
	return textResult(body), nil, nil
}

func handleUserContext(ctx context.Context, req *mcp.CallToolRequest, _ emptyArgs) (*mcp.CallToolResult, any, error) {
	body, err := apiGET("/ai/context", nil)
	if err != nil {
		return nil, nil, err
	}
	var out struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(out.Content) == "" {
		return textResult([]byte("(no CFO context written yet — the user can add one in the app under AI → Overview)")), nil, nil
	}
	return textResult([]byte(out.Content)), nil, nil
}

func handleBudgetStatus(ctx context.Context, req *mcp.CallToolRequest, a budgetStatusArgs) (*mcp.CallToolResult, any, error) {
	q := url.Values{}
	if a.Month != "" {
		q.Set("month", a.Month)
	}
	body, err := apiGET("/budgets/status", q)
	if err != nil {
		return nil, nil, err
	}
	return textResult(body), nil, nil
}

func handleLabelRules(ctx context.Context, req *mcp.CallToolRequest, _ emptyArgs) (*mcp.CallToolResult, any, error) {
	body, err := apiGET("/labels/rules", nil)
	if err != nil {
		return nil, nil, err
	}
	return textResult(body), nil, nil
}

func handleStockTrades(ctx context.Context, req *mcp.CallToolRequest, a tradesArgs) (*mcp.CallToolResult, any, error) {
	body, err := apiGET("/stocks", nil)
	if err != nil {
		return nil, nil, err
	}
	// The endpoint returns the whole ledger (date ASC) — filter and cap here.
	var trades []map[string]any
	if err := json.Unmarshal(body, &trades); err == nil {
		ticker := strings.ToUpper(strings.TrimSpace(a.Ticker))
		var kept []map[string]any
		for _, tr := range trades {
			if t, _ := tr["ticker"].(string); ticker == "" || strings.EqualFold(t, ticker) {
				kept = append(kept, tr)
			}
		}
		for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
			kept[i], kept[j] = kept[j], kept[i] // newest first
		}
		if n := clampLimit(a.Limit); len(kept) > n {
			kept = kept[:n]
		}
		if trimmed, err := json.Marshal(map[string]any{"total": len(trades), "returned": len(kept), "trades": kept}); err == nil {
			body = trimmed
		}
	}
	return textResult(body), nil, nil
}

func handleBalances(ctx context.Context, req *mcp.CallToolRequest, a balancesArgs) (*mcp.CallToolResult, any, error) {
	q := url.Values{}
	if a.DateFrom != "" {
		q.Set("date_from", a.DateFrom)
	}
	if a.DateTo != "" {
		q.Set("date_to", a.DateTo)
	}
	body, err := apiGET("/balances", q)
	if err != nil {
		return nil, nil, err
	}
	return textResult(body), nil, nil
}

func handlePortfolio(ctx context.Context, req *mcp.CallToolRequest, _ emptyArgs) (*mcp.CallToolResult, any, error) {
	body, err := apiGET("/stocks/portfolio", nil)
	if err != nil {
		return nil, nil, err
	}
	return textResult(body), nil, nil
}

func handleQuote(ctx context.Context, req *mcp.CallToolRequest, a quoteArgs) (*mcp.CallToolResult, any, error) {
	t := strings.ToUpper(strings.TrimSpace(a.Ticker))
	if t == "" || len(t) > 12 {
		return nil, nil, fmt.Errorf("ticker must be a short symbol like VWCE")
	}
	body, err := apiGET("/stocks/price/"+url.PathEscape(t), nil)
	if err != nil {
		return nil, nil, err
	}
	return textResult(body), nil, nil
}

func handleAssets(ctx context.Context, req *mcp.CallToolRequest, _ emptyArgs) (*mcp.CallToolResult, any, error) {
	assets, err := apiGET("/assets", nil)
	if err != nil {
		return nil, nil, err
	}
	summary, err := apiGET("/assets/summary", nil)
	if err != nil {
		return nil, nil, err
	}
	combined, _ := json.Marshal(map[string]json.RawMessage{
		"assets": assets, "summary": summary,
	})
	return textResult(combined), nil, nil
}

// ── Write handlers (read-write token only) ───────────────────────────

func handleDeleteRule(ctx context.Context, req *mcp.CallToolRequest, a deleteRuleArgs) (*mcp.CallToolResult, any, error) {
	if a.RuleID <= 0 {
		return nil, nil, fmt.Errorf("rule_id must be a positive rule id (see get_label_rules)")
	}
	body, err := apiWrite(http.MethodDelete, fmt.Sprintf("/labels/rules/%d", a.RuleID), nil)
	if err != nil {
		return nil, nil, err
	}
	return textResult(body), nil, nil
}

func handleAddRule(ctx context.Context, req *mcp.CallToolRequest, a addRuleArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(a.Label) == "" || strings.TrimSpace(a.CommentMatch) == "" {
		return nil, nil, fmt.Errorf("label and comment_match are both required to add a rule")
	}
	body, err := apiWrite(http.MethodPost, "/ai/rule-review/apply", map[string]any{
		"items": []map[string]any{{
			"action":        "add",
			"label":         a.Label,
			"comment_match": a.CommentMatch,
			"category":      a.Category,
		}},
	})
	if err != nil {
		return nil, nil, err
	}
	return textResult(body), nil, nil
}

func handleUpdateRule(ctx context.Context, req *mcp.CallToolRequest, a updateRuleArgs) (*mcp.CallToolResult, any, error) {
	if a.RuleID <= 0 {
		return nil, nil, fmt.Errorf("rule_id must be a positive rule id (see get_label_rules)")
	}
	body, err := apiWrite(http.MethodPost, "/ai/rule-review/apply", map[string]any{
		"items": []map[string]any{{
			"action":        "update",
			"rule_id":       a.RuleID,
			"comment_match": a.CommentMatch,
			"category":      a.Category,
		}},
	})
	if err != nil {
		return nil, nil, err
	}
	return textResult(body), nil, nil
}

func handleRetagTransactions(ctx context.Context, req *mcp.CallToolRequest, a retagArgs) (*mcp.CallToolResult, any, error) {
	if len(a.Items) == 0 {
		return nil, nil, fmt.Errorf("items must name at least one transaction to retag")
	}
	body, err := apiWrite(http.MethodPost, "/ai/label-reindex/apply", a)
	if err != nil {
		return nil, nil, err
	}
	return textResult(body), nil, nil
}

func handleRenameLabel(ctx context.Context, req *mcp.CallToolRequest, a renameLabelArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(a.From) == "" || strings.TrimSpace(a.To) == "" {
		return nil, nil, fmt.Errorf("both from and to are required to rename a label")
	}
	body, err := apiWrite(http.MethodPost, "/labels/rename", map[string]any{"from": a.From, "to": a.To})
	if err != nil {
		return nil, nil, err
	}
	return textResult(body), nil, nil
}

func handleCreateTransaction(ctx context.Context, req *mcp.CallToolRequest, a createTransactionArgs) (*mcp.CallToolResult, any, error) {
	if a.Amount <= 0 {
		return nil, nil, fmt.Errorf("amount must be a positive EUR value")
	}
	if strings.TrimSpace(a.Type) == "" || strings.TrimSpace(a.Date) == "" || strings.TrimSpace(a.Category) == "" {
		return nil, nil, fmt.Errorf("type, date (YYYY-MM-DD) and category are required")
	}
	body, err := apiWrite(http.MethodPost, "/transactions", map[string]any{
		"type": a.Type, "date": a.Date, "amount": a.Amount,
		"comment": a.Comment, "category": a.Category, "labels": a.Labels,
		"debit_account": a.DebitAccount, "credit_account": a.CreditAccount,
	})
	if err != nil {
		return nil, nil, err
	}
	return textResult(body), nil, nil
}

// dataNotes is prepended guidance so models interpret the data correctly.
const dataNotes = ` Data semantics: amounts are EUR; 'labels' is a comma-separated multiset of lowercase tags; category 'Transfers' rows are moves between the user's own accounts (NOT income or spending); fixed-obligation labels (loan, alimony, leasing, evelina) mark pre-committed money, not discretionary choices.`

func main() {
	apiURL = strings.TrimRight(os.Getenv("FT_API_URL"), "/")
	apiToken = os.Getenv("FT_API_TOKEN")
	if apiURL == "" || apiToken == "" {
		log.Fatal("FT_API_URL and FT_API_TOKEN must be set (mint the token in the app: Security → API token)")
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "finance-tracker", Version: "1.0.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_user_context",
		Description: "The user's own CFO-context briefing: who they are, income structure, investment framework, standing rules, open decisions and preferred communication style. Call it FIRST and follow it — it defines how to advise this user.",
	}, handleUserContext)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_overview",
		Description: "Full financial overview report: current balances/net worth, all-time and recent transaction summaries, top categories, fixed obligations vs discretionary labels, this month's spending vs budgets with safe-to-spend, and stock positions with live market prices. Call this FIRST for any broad question." + dataNotes,
	}, handleOverview)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_transactions",
		Description: "Search individual transactions with filters (comment substring, labels, category, type, date range, amount range, sort). Returns paginated rows (capped at 500)." + dataNotes,
	}, handleSearchTransactions)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_summary",
		Description: "Aggregated totals for any filter combination: total income, expenses, investments, plus per-category, per-month AND per-label breakdowns. Accepts the same filters as search_transactions (dates, category, label, amount range). Prefer this over search_transactions for sums, trends and 'where does money go' questions.",
	}, handleSummary)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_label_stats",
		Description: "Every label's footprint: transaction count, EUR volume, rule/budget usage, first/last use. Labels are behavioral tags (merchants, contexts, obligations).",
	}, handleLabelStats)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_budgets",
		Description: "The monthly budget plan as configured: fixed obligations, investment targets and spending limits, each with its matcher (label and/or category) and monthly amount. For actual progress against the plan use get_budget_status.",
	}, handleBudgets)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_budget_status",
		Description: "Month-to-date progress against every budget line: budgeted vs spent vs remaining, fixed/investment totals, discretionary spending and safe-to-spend. THE tool for 'am I on budget' and 'how much can I still spend' questions.",
	}, handleBudgetStatus)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_label_rules",
		Description: "The auto-labeling rules: each rule applies its label to transactions whose comment contains the pattern (^ anchors to the start) and category matches. Explains WHY rows carry a label.",
	}, handleLabelRules)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_balances",
		Description: "Account balance snapshots over time (net worth history per account: banks, ETF, pensions, cash, BTC units + btc_price). Optional date range.",
	}, handleBalances)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_stock_portfolio",
		Description: "Stock holdings from the trade ledger: shares, average cost, total cost basis and realized gains per ticker (cost basis only — use get_stock_quote for live prices).",
	}, handlePortfolio)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_stock_trades",
		Description: "Individual stock trades (buys/sells), newest first: date, action, ticker, shares, price, source. Optional ticker filter.",
	}, handleStockTrades)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_stock_quote",
		Description: "Live market price for one ticker (Yahoo Finance, ~5-minute cache; European exchange suffixes resolved automatically).",
	}, handleQuote)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_assets",
		Description: "Physical assets (real estate, vehicles, solar) with valuations, loan balances, interest structure (margin + EURIBOR) and net equity, plus portfolio totals.",
	}, handleAssets)

	// ── Write tools ──────────────────────────────────────────────────
	// These MUTATE data and require a read-write API token (ftkw_). With the
	// default read-only token (ftk_) the backend answers 403 and the tool
	// returns a clear "mint a read-write token" message.

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_rule",
		Description: "MUTATES DATA — requires a read-write API token (ftkw_). Creates a new auto-labeling rule: applies `label` to transactions whose comment contains `comment_match` (a leading ^ anchors to the start) and, when `category` is given, whose category matches it.",
	}, handleAddRule)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_rule",
		Description: "MUTATES DATA — requires a read-write API token (ftkw_). Edits an existing auto-labeling rule (id from get_label_rules): sets its comment match pattern and/or category.",
	}, handleUpdateRule)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_rule",
		Description: "MUTATES DATA — requires a read-write API token (ftkw_). Permanently deletes one auto-labeling rule by its id (from get_label_rules).",
	}, handleDeleteRule)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rename_label",
		Description: "MUTATES DATA — requires a read-write API token (ftkw_). Renames label `from` to `to` everywhere it is used (transactions, rules, budgets); merges into `to` if that label already exists.",
	}, handleRenameLabel)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "retag_transactions",
		Description: "MUTATES DATA — requires a read-write API token (ftkw_). Adds and/or removes labels on specific transactions by id (ids from search_transactions), one entry per transaction.",
	}, handleRetagTransactions)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_transaction",
		Description: "MUTATES DATA — requires a read-write API token (ftkw_). Creates a REAL new transaction (e.g. from a receipt the user describes). Amount is positive EUR; type is expense|income|investment; category must be a valid app category. Confirm the details with the user before calling.",
	}, handleCreateTransaction)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
