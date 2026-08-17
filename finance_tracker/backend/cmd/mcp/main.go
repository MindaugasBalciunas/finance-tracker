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
	Query    string `json:"query,omitempty" jsonschema:"case-insensitive substring of the transaction comment (merchant, description)"`
	Label    string `json:"label,omitempty" jsonschema:"label filter; comma list matches ANY of the labels (labels are lowercase tags like 'groceries' or 'loan')"`
	AllOf    bool   `json:"all_of,omitempty" jsonschema:"when true a comma list of labels must ALL be present on a row"`
	Category string `json:"category,omitempty" jsonschema:"exact category name, e.g. Food, Housing, Transport, Salary"`
	Type     string `json:"type,omitempty" jsonschema:"expense | income | investment"`
	DateFrom string `json:"date_from,omitempty" jsonschema:"YYYY-MM-DD inclusive"`
	DateTo   string `json:"date_to,omitempty" jsonschema:"YYYY-MM-DD inclusive"`
	Sort     string `json:"sort,omitempty" jsonschema:"date (default) | amount | comment"`
	Dir      string `json:"dir,omitempty" jsonschema:"asc | desc"`
	Limit    int    `json:"limit,omitempty" jsonschema:"max rows to return, default 100, cap 500"`
}

type summaryArgs struct {
	DateFrom string `json:"date_from,omitempty" jsonschema:"YYYY-MM-DD inclusive"`
	DateTo   string `json:"date_to,omitempty" jsonschema:"YYYY-MM-DD inclusive"`
	Category string `json:"category,omitempty" jsonschema:"scope to one category"`
	Label    string `json:"label,omitempty" jsonschema:"scope to one label"`
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
		Description: "Search individual transactions with filters (comment substring, labels, category, type, date range, sort). Returns paginated rows (capped at 500)." + dataNotes,
	}, handleSearchTransactions)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_summary",
		Description: "Aggregated totals for a period and/or category/label: total income, expenses, investments, plus per-category and per-month breakdowns. Prefer this over search_transactions for sums and trends.",
	}, handleSummary)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_label_stats",
		Description: "Every label's footprint: transaction count, EUR volume, rule/budget usage, first/last use. Labels are behavioral tags (merchants, contexts, obligations).",
	}, handleLabelStats)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_budgets",
		Description: "The monthly budget plan: fixed obligations, investment targets and spending limits, each with its matcher (label and/or category) and monthly amount.",
	}, handleBudgets)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_balances",
		Description: "Account balance snapshots over time (net worth history per account: banks, ETF, pensions, cash, BTC units + btc_price). Optional date range.",
	}, handleBalances)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_stock_portfolio",
		Description: "Stock holdings from the trade ledger: shares, average cost, total cost basis and realized gains per ticker (cost basis only — use get_stock_quote for live prices).",
	}, handlePortfolio)

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
