// finance-tracker-mcp is a read-only MCP server exposing the Finance Tracker
// API as typed tools for MCP clients (Claude Desktop, Claude Code, or any
// gateway that attaches MCP servers).
//
// Safeguards, by construction:
//   - No database access: every tool is an HTTP GET against the app's REST
//     API, authenticated with the read-only API token (Security → API token).
//     The backend enforces GET-only + a route allowlist server-side, so even
//     a compromised or confused client cannot mutate data or read backups,
//     settings or the AI key.
//   - The token comes from the FT_API_TOKEN environment variable and is never
//     logged or echoed into tool output.
//   - Every list tool caps its result size, so a broad query cannot dump the
//     whole ledger into a model context.
//
// Configuration (environment):
//
//	FT_API_URL   base URL of the API, e.g. http://homeassistant.local:8098/api/v1
//	FT_API_TOKEN the read-only token minted in Security → API token (ftk_…)
//
// Claude Desktop / Claude Code config example:
//
//	{"mcpServers": {"finance-tracker": {
//	  "command": "/path/to/finance-tracker-mcp",
//	  "env": {"FT_API_URL": "http://homeassistant.local:8098/api/v1",
//	          "FT_API_TOKEN": "ftk_..."}}}}
package main

import (
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

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
