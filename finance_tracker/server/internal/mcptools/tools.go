// Package mcptools is the Finance Tracker's MCP tool set — tools, shared
// instructions and ready-made prompts — used by the stdio server
// (cmd/mcp) and by the app's remote MCP endpoint for Claude's custom
// connectors. Every tool is a call to the app's HTTP API, so what a client
// can reach is exactly its token's scope.
package mcptools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Caller performs one API request (path without the /api prefix) and
// returns the response body, or an error the client can read.
type Caller func(method, path string, q url.Values, body any) ([]byte, error)

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

type periodArgs struct {
	From string `json:"from,omitempty" jsonschema:"YYYY-MM-DD"`
	To   string `json:"to,omitempty" jsonschema:"YYYY-MM-DD"`
}

type historyArgs struct {
	AccountID string `json:"account_id" jsonschema:"account id"`
	From      string `json:"from,omitempty" jsonschema:"YYYY-MM-DD, optional"`
}

type forecastArgs struct {
	Gross       float64  `json:"gross,omitempty" jsonschema:"a new gross salary per month, EUR (LT taxes applied)"`
	Income      *float64 `json:"income,omitempty" jsonschema:"or a net income per month, EUR"`
	Saving      *float64 `json:"saving,omitempty" jsonschema:"saving lines per month, EUR"`
	EverydayPct float64  `json:"everyday_pct,omitempty" jsonschema:"-20 = everyday spending 20% lower"`
	From        string   `json:"from,omitempty" jsonschema:"YYYY-MM the change starts"`
	Purchases   []struct {
		Label  string  `json:"label"`
		Month  string  `json:"month" jsonschema:"YYYY-MM"`
		Amount float64 `json:"amount" jsonschema:"EUR"`
	} `json:"purchases,omitempty" jsonschema:"extra one-off buys"`
	SkipGoals []int64 `json:"skip_goals,omitempty" jsonschema:"wish-list goal ids to leave out"`
}

type yearArgs struct {
	Year int `json:"year,omitempty" jsonschema:"default this year"`
}

type tripTagArgs struct {
	Tag string `json:"tag" jsonschema:"trip:<name>"`
}

type tripDatesArgs struct {
	Tag   string `json:"tag" jsonschema:"trip:<name>"`
	Start string `json:"start,omitempty" jsonschema:"first day YYYY-MM-DD"`
	End   string `json:"end,omitempty" jsonschema:"last day YYYY-MM-DD"`
	Clear bool   `json:"clear,omitempty" jsonschema:"remove the dates instead"`
}

type ibkrArgs struct {
	What   string `json:"what" jsonschema:"summary | positions | balances | orders | instructions | trades"`
	Period string `json:"period,omitempty" jsonschema:"trades only: TODAY, DAYS_7, DAYS_30 (default), DAYS_90, MONTH_TO_DATE, YEAR_TO_DATE, LAST_QUARTER"`
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

func getTx(call Caller, id int64) (map[string]any, error) {
	b, err := call("GET", fmt.Sprintf("/transactions/%d", id), nil, nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Transaction map[string]any `json:"transaction"`
	}
	return out.Transaction, json.Unmarshal(b, &out)
}

// WriteTools change data (their descriptions start with WRITES). A
// read-only connection isn't offered them at all.
var WriteTools = []string{"create_transaction", "update_transaction", "save_rule", "delete_rule", "rename_tag", "update_inbox_row", "save_budget", "delete_budget",
	"update_plan_settings", "tag_trip", "save_recurring", "delete_recurring", "save_goal", "fund_goals", "remember", "set_trip_dates"}

// New is every tool, the shared instructions and the ready-made analyses
// (prompts) over call. The stdio server passes an HTTP client of the API;
// the app's own remote endpoint (/mcp) calls its routes in process with
// the connected app's token, under the same token scope.
func New(call Caller) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "finance-tracker", Version: "2.30.0"}, &mcp.ServerOptions{Instructions: instructions})

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
	mcp.AddTool(s, &mcp.Tool{Name: "get_budget_status", Description: "The monthly plan: every budget line (fixed/spending/saving, funds, yearly), spent vs budget, suggestions, unbudgeted spending, left to spend (field safe_to_spend)."},
		func(ctx context.Context, r *mcp.CallToolRequest, a monthArgs) (*mcp.CallToolResult, any, error) {
			return text(call("GET", "/plan", vals("month", a.Month), nil))
		})
	for _, t := range []struct{ name, desc, path string }{
		{"get_recurring", "Recurring money movements the cash plan uses: bills and subscriptions, standing orders between accounts (transfers) and expected income — amount, monthly cost, accounts, usual day, next date, whether this month's one is done. source: detected, edited or manual (added by the owner).", "/insights/recurring"},
		{"get_fi", "Financial-independence projection: target, investable assets, progress, years to FI, required monthly saving for the target age, emergency fund.", "/insights/fi"},
		{"get_portfolio_quality", "Money-weighted annual return of the investment portfolio, the core funds and the hand-picked stocks; the picks' money in the core fund on the same days (are the picks worth it?); single stocks above the concentration limit; where the next monthly contribution goes toward the owner's target mix (buying only — selling makes gains taxable).", "/portfolio/quality"},
		{"get_loans", "Loans (mortgage): balance, rate, next payment split, payoff date, equity and LTV, rate reset, principal/interest paid in the last 12 months.", "/loans"},
		{"get_trips", "Trips (trip:* tags) with totals, per day and category split; their real dates where set (spending inside them gets the trip tag by itself); untagged travel spending that looks like a trip; planned trips being saved for on the wish list.", "/trips"},
		{"get_bank_inbox", "Bank rows waiting for the owner's review (not in the ledger yet), with proposals, likely duplicates and the ids update_inbox_row takes. Accepting them stays with the owner, in the app.", "/bank/inbox"},
		{"get_bank_connections", "Bank connections, consent expiry and mapped accounts.", "/bank/connections"},
		{"get_owed", "Money people owe the owner: per person, expenses tagged owed:<who> minus their repayments. Positive = still owed.", "/owed"},
		{"get_checks", "Data-health to-dos: stale balances or valuations, bank consent running out, inbox rows waiting, vague categories, an overdue backup, loan rate resets — each with a level (warn | info) and where in the app to fix it.", "/checks"},
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
			cur, err := getTx(call, a.ID)
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
	mcp.AddTool(s, &mcp.Tool{Name: "get_reconciliation", Description: "Checks each cash, savings and loan account: recorded balances (bank-synced or entered) vs the transactions dated between them, since a date (default 30 days ago). Per account: reconciles (true = matches, or only a payment the bank booked a few days off its date — late_booking), the unexplained total and gaps — timing ones (undone within days) vs lasting ones (a missing, duplicated or mis-assigned transaction)."},
		func(ctx context.Context, r *mcp.CallToolRequest, a reconArgs) (*mcp.CallToolResult, any, error) {
			return text(call("GET", "/insights/reconcile", vals("from", a.From), nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "ibkr_live", Description: "Live, read-only data from the owner's Interactive Brokers account (when connected in the app, Settings → Banks): 'summary' (net liquidation, cash, buying power), 'positions' (quantity, price, value, P&L, cost basis), 'balances' (cash by currency), 'orders' (live orders and their status), 'instructions' (saved order instructions — not live until the owner submits them in IBKR), or 'trades' for a period. Prefer it over get_portfolio for what IBKR holds right now. It cannot trade. IBKR ends its sign-in every 7 days; if it has lapsed, tell the owner to connect again in the app."},
		func(ctx context.Context, r *mcp.CallToolRequest, a ibkrArgs) (*mcp.CallToolResult, any, error) {
			return text(call("GET", "/ibkr/live", vals("what", a.What, "period", a.Period), nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_account_history", Description: "Balance points of one account (recorded or bank-synced), from a date — the latest 400 at most."},
		func(ctx context.Context, r *mcp.CallToolRequest, a historyArgs) (*mcp.CallToolResult, any, error) {
			return text(call("GET", "/balances", vals("account", a.AccountID, "from", a.From, "limit", "400"), nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_forecast", Description: "Cash (checking, savings, cash) month by month for the next 12 months: the plan's income, obligations and saving, usual everyday spending, the one-off allowance, dated yearly bills and wish-list goals in their month; the first month below the emergency target or out of cash. Pass scenario fields to compare a what-if with the baseline (it does the arithmetic — use it instead of calculating). Bonuses and extra investing are not in the baseline."},
		func(ctx context.Context, r *mcp.CallToolRequest, a forecastArgs) (*mcp.CallToolResult, any, error) {
			if a.Gross == 0 && a.Income == nil && a.Saving == nil && a.EverydayPct == 0 && len(a.Purchases) == 0 && len(a.SkipGoals) == 0 {
				return text(call("GET", "/forecast", nil, nil))
			}
			b, _ := json.Marshal(a)
			return text(call("GET", "/forecast", vals("scenario", string(b)), nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_tax", Description: "Lithuanian income declaration (GPM311, due 1 May) estimate for a year: salary above the 25% bracket left to pay, interest and investment gains above their €500 exemptions, the 3rd-pillar refund (only contracts signed before 2025), dividends note; due and refund totals. Estimates from the ledger — say so; VMI's pre-filled declaration is what counts."},
		func(ctx context.Context, r *mcp.CallToolRequest, a yearArgs) (*mcp.CallToolResult, any, error) {
			q := url.Values{}
			if a.Year > 0 {
				q.Set("year", fmt.Sprint(a.Year))
			}
			return text(call("GET", "/tax", q, nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_fun_facts", Description: "Fun statistics from the whole history: employers ranked by what they paid, the first pay cheque, lifetime income, most visited shops and where the most money went, little habits added up (coffees, fast food, fuel…), the biggest purchase and day, the big spending weekday, the priciest trips, the biggest spending year. Obligations are left out. Optional from/to limit it to a period."},
		func(ctx context.Context, r *mcp.CallToolRequest, a periodArgs) (*mcp.CallToolResult, any, error) {
			return text(call("GET", "/insights/fun", vals("from", a.From, "to", a.To), nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_market_buzz", Description: "News headlines, public social chatter and Fear & Greed for a ticker. Social chatter is unverified — say so."},
		func(ctx context.Context, r *mcp.CallToolRequest, a tickerArgs) (*mcp.CallToolResult, any, error) {
			return text(call("GET", "/market/buzz/"+url.PathEscape(strings.ToUpper(a.Ticker)), nil, nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_month_review", Description: "A month's review as the app shows it: income, spending and saving vs the previous month and the six-month average, a verdict, highlights, the net worth change, categories vs their norm, top expenses and income, daily spending, a 12-month trend, the budget brief, money owed to the owner and data checks. Default: the current month."},
		func(ctx context.Context, r *mcp.CallToolRequest, a monthArgs) (*mcp.CallToolResult, any, error) {
			return text(call("GET", "/insights/month", vals("month", a.Month), nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_net_worth_movement", Description: "Which accounts drove net worth between two dates: each account's balance at the start and end and the change, largest first. Default: the last 12 months."},
		func(ctx context.Context, r *mcp.CallToolRequest, a periodArgs) (*mcp.CallToolResult, any, error) {
			return text(call("GET", "/networth/movement", vals("from", a.From, "to", a.To), nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "get_trip_untagged", Description: "For a trip with real dates: spending inside the dates that lacks its tag — likely (paid away from home) and maybe (merchants also paid at home) — and travel bookings made before it. Tag the right ones with tag_trip after the owner confirms."},
		func(ctx context.Context, r *mcp.CallToolRequest, a tripTagArgs) (*mcp.CallToolResult, any, error) {
			return text(call("GET", "/trips/untagged", vals("tag", a.Tag), nil))
		})
	mcp.AddTool(s, &mcp.Tool{Name: "set_trip_dates", Description: "WRITES — read-write token. Sets a trip's real first and last day (tag trip:<name>), so new expenses inside them get the tag by themselves; clear=true removes the dates. Confirm first."},
		func(ctx context.Context, r *mcp.CallToolRequest, a tripDatesArgs) (*mcp.CallToolResult, any, error) {
			if a.Clear {
				return text(call("DELETE", "/trips/dates", vals("tag", a.Tag), nil))
			}
			return text(call("PUT", "/trips/dates", nil, map[string]any{"tag": a.Tag, "start": a.Start, "end": a.End}))
		})
	addPrompts(s)
	return s
}

func orNull(b []byte) string {
	if len(bytes.TrimSpace(b)) == 0 {
		return "null"
	}
	return string(b)
}
