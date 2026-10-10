// finance-tracker-mcp exposes the Finance Tracker API as MCP tools for
// Claude Desktop, Claude Code or any MCP client.
//
// It never touches the database: every tool is an HTTP call to the app's
// API with a bearer token, and the server enforces the token's scope.
//
//   - read-only token (ftk_…): every read tool.
//   - read-write token (ftkw_…): also create/update transactions, rules and
//     tags, improve bank-inbox proposals, and manage the plan (budget lines,
//     plan settings, trips, recurring items) and the assistant's memory.
//     Accepting bank rows into the ledger, app settings, exports and backups
//     stay out of reach.
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

type budgetArgs struct {
	ID         int64     `json:"id,omitempty" jsonschema:"budget line id from get_budget_status; omit to create"`
	Name       *string   `json:"name,omitempty"`
	Kind       *string   `json:"kind,omitempty" jsonschema:"fixed | spending | saving"`
	Categories *[]string `json:"categories,omitempty" jsonschema:"category ids the line covers"`
	Tag        *string   `json:"tag,omitempty" jsonschema:"tag the line covers, e.g. kids or trip:rome"`
	Amount     *float64  `json:"amount,omitempty" jsonschema:"EUR per period"`
	Period     *string   `json:"period,omitempty" jsonschema:"monthly | yearly"`
	Fund       *bool     `json:"fund,omitempty" jsonschema:"spending lines: carry unspent money over (sinking fund)"`
	FromMonth  string    `json:"from_month,omitempty" jsonschema:"YYYY-MM the new amount starts (default: this month)"`
	AllMonths  bool      `json:"all_months,omitempty" jsonschema:"rewrite the amount for every month"`
	StartMonth *string   `json:"start_month,omitempty" jsonschema:"YYYY-MM the line starts"`
	Archived   *bool     `json:"archived,omitempty" jsonschema:"retire the line, keeping its history"`
}

type salaryRule struct {
	From      string `json:"from" jsonschema:"YYYY-MM-DD the rule applies from; empty = always"`
	PaidByDay int    `json:"paid_by_day" jsonschema:"salary booked on or before this day counts for the month before; 0 = none"`
}

type planSettingsArgs struct {
	IncomeMode        *string            `json:"income_mode,omitempty" jsonschema:"median | manual | gross"`
	ManualIncome      *float64           `json:"manual_income,omitempty" jsonschema:"EUR net per month"`
	GrossSalary       *float64           `json:"gross_salary,omitempty" jsonschema:"EUR gross per month"`
	MonthlyDeductions *float64           `json:"monthly_deductions,omitempty"`
	SalaryRules       []salaryRule       `json:"salary_rules,omitempty" jsonschema:"replaces the whole list — send every rule"`
	SalaryAccount     *string            `json:"salary_account,omitempty" jsonschema:"account id salary is paid into"`
	Buffers           map[string]float64 `json:"buffers,omitempty" jsonschema:"account id → EUR floor; merges per account, 0 removes one"`
	EmergencyMonths   *float64           `json:"emergency_months,omitempty"`
	TargetAge         *int               `json:"target_age,omitempty"`
	BirthYear         *int               `json:"birth_year,omitempty"`
	FIMonthlySpend    *float64           `json:"fi_monthly_spend,omitempty"`
	WithdrawalRate    *float64           `json:"withdrawal_rate,omitempty"`
	ExpectedReturn    *float64           `json:"expected_return,omitempty"`
}

type tripArgs struct {
	Name string  `json:"name" jsonschema:"trip name, e.g. rome-2026"`
	IDs  []int64 `json:"ids" jsonschema:"transaction ids"`
}

type recurringArgs struct {
	ID          int64   `json:"id,omitempty" jsonschema:"item id to update or delete; omit to add (an existing name is updated)"`
	Name        string  `json:"name,omitempty" jsonschema:"e.g. IBKR top-up"`
	Kind        string  `json:"kind,omitempty" jsonschema:"transfer | bill | income"`
	Amount      float64 `json:"amount,omitempty" jsonschema:"EUR, positive"`
	Day         int     `json:"day,omitempty" jsonschema:"usual day of month 1–31"`
	FromAccount string  `json:"from_account,omitempty"`
	ToAccount   string  `json:"to_account,omitempty"`
	Category    string  `json:"category,omitempty"`
	Cadence     string  `json:"cadence,omitempty" jsonschema:"monthly | quarterly | yearly"`
	Note        string  `json:"note,omitempty"`
	Hidden      bool    `json:"hidden,omitempty" jsonschema:"true = not really recurring, stop counting it"`
}

type goalArgs struct {
	ID          int64   `json:"id,omitempty" jsonschema:"goal id; omit to add"`
	Name        string  `json:"name,omitempty"`
	Target      float64 `json:"target,omitempty" jsonschema:"price in EUR"`
	TargetDate  string  `json:"target_date,omitempty" jsonschema:"YYYY-MM-DD, optional"`
	Note        string  `json:"note,omitempty"`
	URL         string  `json:"url,omitempty"`
	Status      string  `json:"status,omitempty" jsonschema:"active | bought | dropped"`
	Tag         string  `json:"tag,omitempty" jsonschema:"trip:<name> makes it a planned trip"`
	PriorityIDs []int64 `json:"priority_ids,omitempty" jsonschema:"goal ids first to last, to reorder"`
}

type fundArgs struct {
	Month       string `json:"month" jsonschema:"YYYY-MM, a finished month"`
	Allocations []struct {
		GoalID int64   `json:"goal_id"`
		Amount float64 `json:"amount" jsonschema:"EUR"`
	} `json:"allocations,omitempty" jsonschema:"omit to use the suggested split by priority"`
}

type noteArgs struct {
	Note string `json:"note" jsonschema:"one short sentence"`
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
	s := mcp.NewServer(&mcp.Implementation{Name: "finance-tracker", Version: "2.27.3"}, nil)

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
		{"get_recurring", "Recurring money movements: bills and subscriptions, standing orders between accounts and expected income — amount, accounts, usual day, next date.", "/insights/recurring"},
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

	// ── the plan (read-write token) ──────────────────────────────────
	mcp.AddTool(s, &mcp.Tool{Name: "get_plan_settings", Description: "The plan's settings: income basis, salary timing rules, salary account, account buffers, emergency months, FI targets, plus net salary from gross."},
		func(ctx context.Context, r *mcp.CallToolRequest, _ none) (*mcp.CallToolResult, any, error) {
			return text(call("GET", "/plan/settings", nil, nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "save_budget", Description: "WRITES — read-write token. Creates a budget line (no id) or changes one (only the fields given). An amount change starts this month — past months keep theirs — unless from_month or all_months. Propose changes first and apply the approved ones."},
		func(ctx context.Context, r *mcp.CallToolRequest, a budgetArgs) (*mcp.CallToolResult, any, error) {
			return text(call("PATCH", "/budgets", nil, a))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "delete_budget", Description: "WRITES — read-write token. Deletes a budget line and its history; prefer save_budget with archived=true. Confirm first."},
		func(ctx context.Context, r *mcp.CallToolRequest, a budgetArgs) (*mcp.CallToolResult, any, error) {
			return text(call("DELETE", fmt.Sprintf("/budgets/%d", a.ID), nil, nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "update_plan_settings", Description: "WRITES — read-write token. Changes plan settings; only the fields given change. Confirm first."},
		func(ctx context.Context, r *mcp.CallToolRequest, a planSettingsArgs) (*mcp.CallToolResult, any, error) {
			return text(call("PATCH", "/plan/settings", nil, a))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "tag_trip", Description: "WRITES — read-write token. Puts transactions under one trip tag (trip:<name>), replacing other trip tags on them. Confirm first."},
		func(ctx context.Context, r *mcp.CallToolRequest, a tripArgs) (*mcp.CallToolResult, any, error) {
			return text(call("POST", "/trips/tag", nil, a))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "save_recurring", Description: "WRITES — read-write token. Adds or updates a recurring item the cash plan counts: a standing order (kind transfer, from → to account), a bill (from account) or expected income (to account); hidden=true stops counting a detected one. Confirm first."},
		func(ctx context.Context, r *mcp.CallToolRequest, a recurringArgs) (*mcp.CallToolResult, any, error) {
			body := map[string]any{"merchant": a.Name, "kind": a.Kind, "amount": a.Amount, "day": a.Day, "from_account": a.FromAccount,
				"to_account": a.ToAccount, "category": a.Category, "cadence": a.Cadence, "note": a.Note, "hidden": a.Hidden}
			if a.ID > 0 {
				return text(call("PUT", fmt.Sprintf("/recurring/%d", a.ID), nil, body))
			}
			return text(call("POST", "/recurring", nil, body))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "delete_recurring", Description: "WRITES — read-write token. Removes the owner's recurring item (by id); a detected one comes back unless hidden instead. Confirm first."},
		func(ctx context.Context, r *mcp.CallToolRequest, a recurringArgs) (*mcp.CallToolResult, any, error) {
			return text(call("DELETE", fmt.Sprintf("/recurring/%d", a.ID), nil, nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_goals", Description: "The wish list: goals in priority order (price, saved, remaining, ETA, per-month need for a target date) and the funding month — income, paid yourself first, spending, left over, available — with the suggested split."},
		func(ctx context.Context, r *mcp.CallToolRequest, a monthArgs) (*mcp.CallToolResult, any, error) {
			return text(call("GET", "/goals", vals("month", a.Month), nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "save_goal", Description: "WRITES — read-write token. Adds a wish-list goal (no id) or changes one (send all its fields); priority_ids reorders the list. Confirm first."},
		func(ctx context.Context, r *mcp.CallToolRequest, a goalArgs) (*mcp.CallToolResult, any, error) {
			if len(a.PriorityIDs) > 0 {
				if _, err := call("POST", "/goals/order", nil, map[string]any{"ids": a.PriorityIDs}); err != nil {
					return nil, nil, err
				}
				if a.ID == 0 && a.Name == "" {
					return text(call("GET", "/goals", nil, nil))
				}
			}
			body := map[string]any{"name": a.Name, "target": a.Target, "target_date": a.TargetDate, "note": a.Note, "url": a.URL, "status": a.Status, "tag": a.Tag}
			if a.ID > 0 {
				return text(call("PUT", fmt.Sprintf("/goals/%d", a.ID), nil, body))
			}
			return text(call("POST", "/goals", nil, body))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "fund_goals", Description: "WRITES — read-write token. Puts a finished month's left over into goals (the suggested split unless allocations are given). Confirm first."},
		func(ctx context.Context, r *mcp.CallToolRequest, a fundArgs) (*mcp.CallToolResult, any, error) {
			allocs := []map[string]any{}
			if len(a.Allocations) == 0 {
				raw, err := call("GET", "/goals", vals("month", a.Month), nil)
				if err != nil {
					return nil, nil, err
				}
				var p struct {
					Proposal []map[string]any `json:"proposal"`
				}
				json.Unmarshal(raw, &p)
				allocs = p.Proposal
			}
			for _, x := range a.Allocations {
				allocs = append(allocs, map[string]any{"goal_id": x.GoalID, "amount": x.Amount})
			}
			return text(call("POST", "/goals/fund", nil, map[string]any{"month": a.Month, "allocations": allocs}))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "remember", Description: "WRITES — read-write token. Saves a lasting fact (decision, plan, life or income change, preference) to the memory every assistant reads. Use on your own when the owner tells you one."},
		func(ctx context.Context, r *mcp.CallToolRequest, a noteArgs) (*mcp.CallToolResult, any, error) {
			return text(call("POST", "/ai/remember", nil, a))
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
