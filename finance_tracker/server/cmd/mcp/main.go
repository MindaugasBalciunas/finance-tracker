// finance-tracker-mcp exposes the Finance Tracker API as MCP tools for
// Claude Desktop, Claude Code or any MCP client.
//
// It never touches the database: every tool is an HTTP call to the app's
// API with a bearer token, and the server enforces the token's scope.
//
//   - read-only token (ftk_…): every read tool.
//   - read-write token (ftkw_…): also create/update transactions, rules and
//     tags, and improve bank-inbox proposals. Accepting bank rows into the
//     ledger, settings, exports and backups stay out of reach.
//
// Configuration:
//
//	FT_API_URL   e.g. https://finance.example/api  (or http://homeassistant.local:8099/api)
//	FT_API_TOKEN the token minted in Settings → Security → API tokens
//	FT_BASIC_AUTH optional "user:password" when nginx basic auth fronts the API
package main

import (
	"bytes"
	"context"
	"encoding/base64"
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
	apiURL, apiToken, basicAuth string
	client                      = &http.Client{Timeout: 60 * time.Second}
)

func call(method, path string, q url.Values, body any) ([]byte, error) {
	u := apiURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, u, rd)
	if err != nil {
		return nil, err
	}
	// nginx basic auth and the app token both want Authorization; the app
	// also accepts the token in X-API-Token for exactly this case.
	if basicAuth != "" {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(basicAuth)))
		req.Header.Set("X-API-Token", apiToken)
	} else {
		req.Header.Set("Authorization", "Bearer "+apiToken)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("finance tracker unreachable at %s: %w", apiURL, err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, fmt.Errorf("token rejected — mint a new one in Settings → Security → API tokens")
	case resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("outside this token's scope (writes need a read-write ftkw_ token)")
	case resp.StatusCode >= 300:
		return nil, fmt.Errorf("API returned %s: %s", resp.Status, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func text(b []byte, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	if len(b) > 200_000 {
		b = append(b[:200_000], []byte("… [truncated — narrow the query]")...)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
}

func vals(kv ...string) url.Values {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			v.Set(kv[i], kv[i+1])
		}
	}
	return v
}

const model = " Amounts are EUR. Transactions are income | expense | transfer; categories are two-level ids (food.groceries); transfer.invest/pension/debt/asset build wealth, transfer.internal moves cash between own accounts; mortgage principal is transfer.debt, interest is housing.mortgage_interest. Tags are people (kids, evelina, kristina), properties and trips (trip:…)."

type none struct{}

type searchArgs struct {
	From       string   `json:"from,omitempty" jsonschema:"YYYY-MM-DD inclusive"`
	To         string   `json:"to,omitempty" jsonschema:"YYYY-MM-DD inclusive"`
	Kind       string   `json:"kind,omitempty" jsonschema:"income | expense | transfer"`
	Categories []string `json:"categories,omitempty" jsonschema:"category ids; a parent includes its children"`
	Accounts   []string `json:"accounts,omitempty" jsonschema:"account ids"`
	Tags       []string `json:"tags,omitempty" jsonschema:"any of these tags"`
	Merchant   string   `json:"merchant,omitempty" jsonschema:"exact merchant"`
	Query      string   `json:"query,omitempty" jsonschema:"free text over merchant, note and tags"`
	Min        float64  `json:"min,omitempty" jsonschema:"minimum amount EUR"`
	Max        float64  `json:"max,omitempty" jsonschema:"maximum amount EUR"`
	Sort       string   `json:"sort,omitempty" jsonschema:"date (default) | amount | +date"`
	Limit      int      `json:"limit,omitempty" jsonschema:"rows, default 100, max 500"`
}

type windowArgs struct {
	From   string `json:"from,omitempty" jsonschema:"YYYY-MM-DD"`
	To     string `json:"to,omitempty" jsonschema:"YYYY-MM-DD"`
	Preset string `json:"preset,omitempty" jsonschema:"month | last_month | 3m | ytd | 12m | last_year | all (used when from/to are empty)"`
}

type flowArgs struct {
	From        string `json:"from,omitempty" jsonschema:"YYYY-MM-DD"`
	To          string `json:"to,omitempty" jsonschema:"YYYY-MM-DD"`
	Granularity string `json:"granularity,omitempty" jsonschema:"month (default) | year"`
}

type nwArgs struct {
	Date        string `json:"date,omitempty" jsonschema:"snapshot date, default today"`
	HistoryFrom string `json:"history_from,omitempty" jsonschema:"when set, month-end history from this date instead of one snapshot"`
}

type monthArgs struct {
	Month string `json:"month,omitempty" jsonschema:"YYYY-MM, default current"`
}

type tickerArgs struct {
	Ticker string `json:"ticker" jsonschema:"ticker symbol, e.g. VWCE or AAPL"`
}

type refArgs struct {
	What string `json:"what" jsonschema:"categories | accounts | tags | rules | merchants"`
}

type txArgs struct {
	ID          int64    `json:"id,omitempty" jsonschema:"transaction id (update only)"`
	Date        string   `json:"date,omitempty" jsonschema:"YYYY-MM-DD"`
	Amount      float64  `json:"amount,omitempty" jsonschema:"EUR, positive"`
	Category    string   `json:"category,omitempty" jsonschema:"category id"`
	Merchant    string   `json:"merchant,omitempty"`
	Note        string   `json:"note,omitempty"`
	AccountID   string   `json:"account_id,omitempty" jsonschema:"bank/cash account paid from or into"`
	ToAccountID string   `json:"to_account_id,omitempty" jsonschema:"transfers only"`
	Tags        []string `json:"tags,omitempty"`
}

type ruleArgs struct {
	ID           int64    `json:"id,omitempty" jsonschema:"rule id (update/delete)"`
	Pattern      string   `json:"pattern,omitempty" jsonschema:"text in merchant or note; ^ anchors to the start"`
	WhenCategory string   `json:"when_category,omitempty"`
	SetCategory  string   `json:"set_category,omitempty"`
	SetMerchant  string   `json:"set_merchant,omitempty"`
	AddTags      []string `json:"add_tags,omitempty"`
	Disabled     bool     `json:"disabled,omitempty"`
}

type renameArgs struct {
	From string `json:"from" jsonschema:"tag to rename"`
	To   string `json:"to,omitempty" jsonschema:"new name; empty removes the tag everywhere"`
}

type inboxArgs struct {
	ID       int64    `json:"id" jsonschema:"inbox row id"`
	Category string   `json:"category,omitempty"`
	Merchant string   `json:"merchant,omitempty"`
	Note     string   `json:"note,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

func getTx(ctx context.Context, id int64) (map[string]any, error) {
	b, err := call("GET", fmt.Sprintf("/transactions/%d", id), nil, nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Transaction map[string]any `json:"transaction"`
	}
	return out.Transaction, json.Unmarshal(b, &out)
}

func main() {
	apiURL = strings.TrimRight(os.Getenv("FT_API_URL"), "/")
	apiToken = os.Getenv("FT_API_TOKEN")
	basicAuth = os.Getenv("FT_BASIC_AUTH")
	if apiURL == "" || apiToken == "" {
		log.Fatal("set FT_API_URL (…/api) and FT_API_TOKEN (Settings → Security → API tokens)")
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "finance-tracker", Version: "2.20.2"}, nil)

	mcp.AddTool(s, &mcp.Tool{Name: "get_user_context", Description: "The owner's own brief (who they are, framework, standing rules, how to advise them) plus decisions they asked to remember. Call FIRST and follow it."},
		func(ctx context.Context, r *mcp.CallToolRequest, _ none) (*mcp.CallToolResult, any, error) {
			c, err := call("GET", "/ai/context", nil, nil)
			if err != nil {
				return nil, nil, err
			}
			n, _ := call("GET", "/ai/notes", nil, nil)
			return text([]byte(fmt.Sprintf(`{"context":%s,"notes":%s}`, c, orNull(n))), nil)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_overview", Description: "Today's CFO snapshot: net worth by group, this month vs the trailing average, plan pulse, emergency fund, FI progress, anomalies, upcoming recurring charges, bank inbox size." + model},
		func(ctx context.Context, r *mcp.CallToolRequest, _ none) (*mcp.CallToolResult, any, error) {
			return text(call("GET", "/overview", nil, nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "search_transactions", Description: "Find transactions; returns rows plus income/expense/transfer totals over the whole match." + model},
		func(ctx context.Context, r *mcp.CallToolRequest, a searchArgs) (*mcp.CallToolResult, any, error) {
			if a.Limit <= 0 || a.Limit > 500 {
				a.Limit = 100
			}
			q := vals("from", a.From, "to", a.To, "kind", a.Kind, "merchant", a.Merchant, "q", a.Query, "sort", a.Sort, "limit", fmt.Sprint(a.Limit))
			if a.Min > 0 {
				q.Set("min", fmt.Sprint(a.Min))
			}
			if a.Max > 0 {
				q.Set("max", fmt.Sprint(a.Max))
			}
			for _, c := range a.Categories {
				q.Add("category", c)
			}
			for _, c := range a.Accounts {
				q.Add("account", c)
			}
			for _, c := range a.Tags {
				q.Add("tag", c)
			}
			return text(call("GET", "/transactions", q, nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "cash_flow", Description: "Income, spending (essential/discretionary), saved, savings rate, invested and mortgage principal per month or year." + model},
		func(ctx context.Context, r *mcp.CallToolRequest, a flowArgs) (*mcp.CallToolResult, any, error) {
			return text(call("GET", "/insights/cashflow", vals("from", a.From, "to", a.To, "granularity", a.Granularity), nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "spending_breakdown", Description: "Spending by category and subcategory in a window, vs the previous equal window; top merchants; largest expenses."},
		func(ctx context.Context, r *mcp.CallToolRequest, a windowArgs) (*mcp.CallToolResult, any, error) {
			return text(call("GET", "/insights/breakdown", vals("from", a.From, "to", a.To, "preset", a.Preset), nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_net_worth", Description: "Net worth on a date by account and group, or month-end history. Includes house, car and mortgage."},
		func(ctx context.Context, r *mcp.CallToolRequest, a nwArgs) (*mcp.CallToolResult, any, error) {
			if a.HistoryFrom != "" {
				return text(call("GET", "/networth/history", vals("from", a.HistoryFrom), nil))
			}
			return text(call("GET", "/networth", vals("date", a.Date), nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_budget_status", Description: "The monthly plan: every budget line (fixed/spending/saving, funds, yearly), spent vs budget, suggestions, unbudgeted spending, safe to spend."},
		func(ctx context.Context, r *mcp.CallToolRequest, a monthArgs) (*mcp.CallToolResult, any, error) {
			return text(call("GET", "/plan", vals("month", a.Month), nil))
		})
	for _, t := range []struct{ name, desc, path string }{
		{"get_recurring", "Detected subscriptions and recurring bills with monthly cost and next expected charge.", "/insights/recurring"},
		{"get_fi", "Financial-independence projection and emergency fund.", "/insights/fi"},
		{"get_loans", "Loans: balance, rate, next payment split, payoff, equity/LTV, rate reset.", "/loans"},
		{"get_trips", "Trips with totals and per-day cost, plus untagged travel that looks like a trip.", "/trips"},
		{"get_bank_inbox", "Bank rows waiting for the owner's review (not in the ledger yet), with proposals and likely duplicates.", "/bank/inbox"},
		{"get_bank_connections", "Bank connections, consent expiry and mapped accounts.", "/bank/connections"},
		{"get_owed", "Money people owe the owner (split parts marked owed, minus repayments).", "/owed"},
	} {
		path := t.path
		mcp.AddTool(s, &mcp.Tool{Name: t.name, Description: t.desc}, func(ctx context.Context, r *mcp.CallToolRequest, _ none) (*mcp.CallToolResult, any, error) {
			return text(call("GET", path, nil, nil))
		})
	}
	mcp.AddTool(s, &mcp.Tool{Name: "get_portfolio", Description: "Investment positions with live prices, cost basis and gains in EUR, plus closed positions."},
		func(ctx context.Context, r *mcp.CallToolRequest, _ none) (*mcp.CallToolResult, any, error) {
			return text(call("GET", "/portfolio", nil, nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_trades", Description: "Every investment trade."},
		func(ctx context.Context, r *mcp.CallToolRequest, _ none) (*mcp.CallToolResult, any, error) {
			return text(call("GET", "/trades", nil, nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_quote", Description: "Live quote with 52-week range and analyst targets."},
		func(ctx context.Context, r *mcp.CallToolRequest, a tickerArgs) (*mcp.CallToolResult, any, error) {
			q, err := call("GET", "/market/quote/"+url.PathEscape(strings.ToUpper(a.Ticker)), nil, nil)
			if err != nil {
				return nil, nil, err
			}
			an, _ := call("GET", "/market/analyst/"+url.PathEscape(strings.ToUpper(a.Ticker)), nil, nil)
			return text([]byte(fmt.Sprintf(`{"quote":%s,"analyst":%s}`, q, orNull(an))), nil)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_reference", Description: "Reference data: categories, accounts (with balances), tags, rules or merchants."},
		func(ctx context.Context, r *mcp.CallToolRequest, a refArgs) (*mcp.CallToolResult, any, error) {
			switch a.What {
			case "categories", "accounts", "tags", "rules", "merchants":
				return text(call("GET", "/"+a.What, nil, nil))
			}
			return nil, nil, fmt.Errorf("what must be categories, accounts, tags, rules or merchants")
		})

	// ── writes (read-write token) ─────────────────────────────────────
	mcp.AddTool(s, &mcp.Tool{Name: "create_transaction", Description: "WRITES — read-write token. Adds a transaction. Confirm the details with the owner first. Empty category lets the app's rules decide."},
		func(ctx context.Context, r *mcp.CallToolRequest, a txArgs) (*mcp.CallToolResult, any, error) {
			body := map[string]any{"date": a.Date, "amount": a.Amount, "category": a.Category, "merchant": a.Merchant, "note": a.Note,
				"account_id": a.AccountID, "to_account_id": a.ToAccountID, "tags": a.Tags, "auto_fill": a.Category == ""}
			return text(call("POST", "/transactions", nil, body))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "update_transaction", Description: "WRITES — read-write token. Changes fields of one transaction (only the fields given). Confirm with the owner first."},
		func(ctx context.Context, r *mcp.CallToolRequest, a txArgs) (*mcp.CallToolResult, any, error) {
			cur, err := getTx(ctx, a.ID)
			if err != nil {
				return nil, nil, err
			}
			for k, v := range map[string]any{"date": a.Date, "category": a.Category, "merchant": a.Merchant, "note": a.Note, "account_id": a.AccountID, "to_account_id": a.ToAccountID} {
				if s, _ := v.(string); s != "" {
					cur[k] = s
					if k == "category" {
						delete(cur, "kind")
					}
				}
			}
			if a.Amount > 0 {
				cur["amount"] = a.Amount
			}
			if a.Tags != nil {
				cur["tags"] = a.Tags
			}
			return text(call("PUT", fmt.Sprintf("/transactions/%d", a.ID), nil, cur))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "save_rule", Description: "WRITES — read-write token. Creates (no id) or replaces (id) a categorisation rule. Confirm first."},
		func(ctx context.Context, r *mcp.CallToolRequest, a ruleArgs) (*mcp.CallToolResult, any, error) {
			body := map[string]any{"pattern": a.Pattern, "when_category": a.WhenCategory, "set_category": a.SetCategory, "set_merchant": a.SetMerchant,
				"add_tags": a.AddTags, "enabled": !a.Disabled}
			if a.ID > 0 {
				return text(call("PUT", fmt.Sprintf("/rules/%d", a.ID), nil, body))
			}
			return text(call("POST", "/rules", nil, body))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "delete_rule", Description: "WRITES — read-write token. Deletes a rule by id. Confirm first."},
		func(ctx context.Context, r *mcp.CallToolRequest, a ruleArgs) (*mcp.CallToolResult, any, error) {
			return text(call("DELETE", fmt.Sprintf("/rules/%d", a.ID), nil, nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "rename_tag", Description: "WRITES — read-write token. Renames a tag everywhere (empty 'to' removes it). Confirm first."},
		func(ctx context.Context, r *mcp.CallToolRequest, a renameArgs) (*mcp.CallToolResult, any, error) {
			return text(call("POST", "/tags/rename", nil, map[string]string{"from": a.From, "to": a.To}))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "update_inbox_row", Description: "WRITES — read-write token. Improves one bank-inbox proposal. It does NOT add it to the ledger; the owner accepts rows in the app."},
		func(ctx context.Context, r *mcp.CallToolRequest, a inboxArgs) (*mcp.CallToolResult, any, error) {
			body := map[string]any{}
			if a.Category != "" {
				body["category"] = a.Category
			}
			if a.Merchant != "" {
				body["merchant"] = a.Merchant
			}
			if a.Note != "" {
				body["note"] = a.Note
			}
			if a.Tags != nil {
				body["tags"] = a.Tags
			}
			return text(call("PUT", fmt.Sprintf("/bank/inbox/%d", a.ID), nil, body))
		})

	if err := s.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

func orNull(b []byte) string {
	if len(bytes.TrimSpace(b)) == 0 {
		return "null"
	}
	return string(b)
}
