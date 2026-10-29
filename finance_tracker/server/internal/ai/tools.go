package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ft/internal/cfo"
	"sort"
	"strings"
	"time"

	"ft/internal/bank"
	"ft/internal/goals"
	"ft/internal/ibkr"
	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/market"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

func obj(props map[string]any, required ...string) map[string]any {
	o := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		o["required"] = required
	}
	return o
}
func str(d string) map[string]any { return map[string]any{"type": "string", "description": d} }
func num(d string) map[string]any { return map[string]any{"type": "number", "description": d} }
func arr(d string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": d}
}

// writeTools change data; the system prompt requires explicit confirmation.
var writeTools = map[string]bool{"remember": true, "create_transaction": true, "update_transaction": true, "add_rule": true, "delete_rule": true,
	"rename_tag": true, "update_inbox_row": true, "add_recurring": true, "save_budget": true, "delete_budget": true, "update_plan_settings": true, "tag_trip": true, "save_goal": true, "fund_goals": true, "set_trip_dates": true}

func toolDefs() []tool {
	date := "YYYY-MM-DD"
	return []tool{
		{Type: webSearchType, Name: "web_search", MaxUses: 5},
		{Name: "get_overview", Description: "Today's CFO snapshot: net worth and its parts, this month's cash flow vs the trailing average, plan pulse (left to spend — field safe_to_spend, budget lines over limit), emergency-fund months, FI progress, bank inbox size.", InputSchema: obj(map[string]any{})},
		{Name: "search_transactions", Description: "Find transactions. Returns at most 200 rows plus totals over the whole match. Categories are ids like 'food' or 'food.groceries' (a parent includes children).",
			InputSchema: obj(map[string]any{"from": str(date), "to": str(date), "kind": str("income | expense | transfer"), "categories": arr("category ids"),
				"accounts": arr("account ids"), "tags": arr("tags (any match)"), "merchant": str("exact merchant"), "query": str("free text over merchant/note/tags"),
				"min": num("min amount EUR"), "max": num("max amount EUR"), "sort": str("date | amount"), "limit": num("max rows, default 50")})},
		{Name: "cash_flow", Description: "Income, spending (essential/discretionary), saved, savings rate, invested (incl. mortgage principal) by month or year. Default: last 12 months by month.",
			InputSchema: obj(map[string]any{"from": str(date), "to": str(date), "granularity": str("month | year")})},
		{Name: "spending_breakdown", Description: "Spending by category with subcategories, share, monthly average and change vs the previous equal window, plus top merchants.",
			InputSchema: obj(map[string]any{"from": str(date), "to": str(date)})},
		{Name: "get_net_worth", Description: "Net worth on a date (by account and group) or its history (month ends). Includes house, car and mortgage.",
			InputSchema: obj(map[string]any{"date": str("snapshot date (default today)"), "history_from": str("if set, return month-end history from this date")})},
		{Name: "get_account_history", Description: "Balance points of one account.", InputSchema: obj(map[string]any{"account_id": str("account id"), "from": str(date)}, "account_id")},
		{Name: "get_budget_status", Description: "The monthly plan: every budget line (with its id for save_budget) (fixed / spending / saving, funds and yearly lines), spent vs budgeted, suggestions, unbudgeted spending, left to spend (field safe_to_spend).",
			InputSchema: obj(map[string]any{"month": str("YYYY-MM, default current")})},
		{Name: "get_recurring", Description: "Recurring money movements the cash plan uses: bills and subscriptions, standing orders between accounts (transfers) and expected income — amount, monthly cost, accounts, usual day, next date, whether this month's one is done. source: detected, edited or manual (added by the owner).", InputSchema: obj(map[string]any{})},
		{Name: "get_forecast", Description: "Cash (checking, savings, cash) month by month for the next 12 months: the plan's income, obligations and saving, usual everyday spending, the one-off allowance, dated yearly bills and wish-list goals in their month; the first month below the emergency target or out of cash. Pass scenario fields to compare a what-if with the baseline (it does the arithmetic — use it instead of calculating): gross (new gross salary, LT taxes applied), saving (saving lines per month), everyday_pct (−20 = 20% less everyday spending), from (YYYY-MM the change starts), purchases [{label, month, amount}]. Bonuses and extra investing are not in the baseline.",
			InputSchema: obj(map[string]any{"gross": num("new gross salary per month"), "saving": num("saving lines per month, EUR"), "everyday_pct": num("change to everyday spending, %"),
				"from": str("YYYY-MM the change starts"), "purchases": map[string]any{"type": "array", "description": "extra one-off purchases", "items": obj(map[string]any{"label": str("what"), "month": str("YYYY-MM"), "amount": num("EUR")}, "month", "amount")},
				"income": num("or a net income per month, EUR"), "skip_goals": map[string]any{"type": "array", "description": "wish-list goal ids to leave out", "items": map[string]any{"type": "number"}}})},
		{Name: "get_portfolio_quality", Description: "Money-weighted annual return of the investment portfolio, the core funds and the hand-picked stocks; the picks' money in the core fund on the same days (are the picks worth it?); single stocks above the concentration limit; where the next monthly contribution goes toward the owner's target mix (buying only — selling makes gains taxable).", InputSchema: obj(map[string]any{})},
		{Name: "get_fun_facts", Description: "Fun statistics from the whole history: employers ranked by what they paid, the first pay cheque, lifetime income, most visited shops and where the most money went, little habits added up (coffees, fast food, fuel…), the biggest purchase and day, the big spending weekday, the priciest trips, the biggest spending year. Obligations are left out. Optional from/to limit it to a period.", InputSchema: obj(map[string]any{"from": str("YYYY-MM-DD, optional"), "to": str("YYYY-MM-DD, optional")})},
		{Name: "get_tax", Description: "Lithuanian income declaration (GPM311, due 1 May) estimate for a year: salary above the 25% bracket left to pay, interest and investment gains above their €500 exemptions, the 3rd-pillar refund (only contracts signed before 2025), dividends note; due and refund totals. Estimates from the ledger — say so; VMI's pre-filled declaration is what counts.",
			InputSchema: obj(map[string]any{"year": num("tax year, default this year")})},
		{Name: "get_fi", Description: "Financial-independence projection: target, investable assets, progress, years to FI, required monthly saving for the target age, emergency fund.", InputSchema: obj(map[string]any{})},
		{Name: "get_loans", Description: "Loans (mortgage): balance, rate, next payment split, payoff date, equity and LTV, principal/interest paid in the last 12 months.", InputSchema: obj(map[string]any{})},
		{Name: "get_portfolio", Description: "Investment positions with live prices, cost basis, gains (EUR), plus trades if asked.", InputSchema: obj(map[string]any{"include_trades": map[string]any{"type": "boolean"}})},
		{Name: "get_quote", Description: "Live quote, 52-week range and analyst targets for a ticker.", InputSchema: obj(map[string]any{"ticker": str("ticker symbol")}, "ticker")},
		{Name: "get_market_buzz", Description: "News headlines, public social chatter and Fear & Greed for a ticker. Social chatter is unverified.", InputSchema: obj(map[string]any{"ticker": str("ticker symbol")}, "ticker")},
		{Name: "get_trips", Description: "Trips (trip:* tags) with totals, per day and category split; their real dates where set (spending inside them gets the trip tag by itself); untagged travel spending that looks like a trip; planned trips being saved for on the wish list. done = the trip is over (in history: by hand, 30 days after its dates or 90 days after its last row).", InputSchema: obj(map[string]any{})},
		{Name: "get_reconciliation", Description: "Checks each cash, savings and loan account: its recorded balances (bank-synced or entered) vs the transactions dated between them, since a date (default 30 days ago). Per account: reconciles (true = matches, or only a late_booking — a payment the bank booked a few days off its date), the unexplained total, and gaps — timing ones (undone within days: booking date ≠ transaction date) vs lasting ones (a missing, duplicated or mis-assigned transaction). Use it to answer whether the ledger matches the banks.",
			InputSchema: obj(map[string]any{"from": str("YYYY-MM-DD, default 30 days ago")})},
		{Name: "get_reference", Description: "Reference data: categories, accounts, tags, rules or merchants.", InputSchema: obj(map[string]any{"what": str("categories | accounts | tags | rules | merchants")}, "what")},
		{Name: "ibkr_live", Description: "Live, read-only data from the owner's Interactive Brokers account (when connected in Settings → Banks): 'summary' (net liquidation, cash, buying power), 'positions' (quantity, price, value, P&L, cost basis), balances' (cash by currency), 'orders' (live orders and their status), 'instructions' (saved order instructions — not live until the owner submits them in IBKR), or 'trades' for a period (TODAY, DAYS_7, DAYS_30, DAYS_90, MONTH_TO_DATE, YEAR_TO_DATE, LAST_QUARTER…). Prefer it over get_portfolio for what IBKR holds right now; it cannot trade.",
			InputSchema: obj(map[string]any{"what": str("summary | positions | balances | orders | instructions | trades"), "period": str("for trades; default DAYS_30")}, "what")},
		{Name: "get_month_review", Description: "A month's review as the app shows it: income, spending and saving vs the previous month and the six-month average, a verdict, highlights, the net worth change, categories vs their norm, top expenses and income, daily spending, a 12-month trend, the budget brief, money owed to the owner and data checks. Default: the current month.", InputSchema: obj(map[string]any{"month": str("YYYY-MM, default current")})},
		{Name: "get_checks", Description: "Data-health to-dos: stale balances or valuations, bank consent running out, inbox rows waiting, vague categories, an overdue backup, loan rate resets — each with a level (warn | info) and where in the app to fix it.", InputSchema: obj(map[string]any{})},
		{Name: "get_owed", Description: "Money people owe the owner: per person, expenses tagged owed:<who> minus their repayments. Positive = still owed.", InputSchema: obj(map[string]any{})},
		{Name: "get_net_worth_movement", Description: "Which accounts drove net worth between two dates: each account's balance at the start and end and the change, largest first. Default: the last 12 months.", InputSchema: obj(map[string]any{"from": str(date), "to": str(date)})},
		{Name: "get_trip_untagged", Description: "For a trip with real dates: spending inside the dates that lacks its tag — likely (paid away from home) and maybe (merchants also paid at home) — and travel bookings made before it. Tag the right ones with tag_trip after the owner confirms.", InputSchema: obj(map[string]any{"tag": str("trip:<name>")}, "tag")},
		{Name: "set_trip_dates", Description: "Set a trip's real first and last day (tag trip:<name>), so new expenses inside them get the tag by themselves; clear=true removes the dates. ONLY after approval.",
			InputSchema: obj(map[string]any{"tag": str("trip:<name>"), "start": str("first day YYYY-MM-DD"), "end": str("last day YYYY-MM-DD"), "clear": map[string]any{"type": "boolean"}}, "tag")},
		{Name: "get_inbox", Description: "Bank rows waiting for review (proposed category, merchant, likely duplicates).", InputSchema: obj(map[string]any{})},
		{Name: "create_transaction", Description: "Add a transaction. ONLY after the user confirmed the details.",
			InputSchema: obj(map[string]any{"date": str(date), "amount": num("EUR, positive"), "category": str("category id"), "merchant": str("merchant"), "note": str("description"),
				"account_id": str("account paid from / into"), "to_account_id": str("transfers: destination account"), "tags": arr("tags")}, "date", "amount", "category")},
		{Name: "update_transaction", Description: "Change category, merchant, note or tags of a transaction. ONLY after the user approved the change.",
			InputSchema: obj(map[string]any{"id": num("transaction id"), "category": str("category id"), "merchant": str("merchant"), "note": str("note"), "tags": arr("replace tags with this list")}, "id")},
		{Name: "add_rule", Description: "Add a categorisation rule (applies to future rows; set apply_to_history to also fix existing ones). ONLY after approval.",
			InputSchema: obj(map[string]any{"pattern": str("text in merchant/note, '^' anchors to the start"), "when_category": str("only rows in this category"),
				"set_category": str("category id"), "set_merchant": str("merchant"), "add_tags": arr("tags"), "apply_to_history": map[string]any{"type": "boolean"}})},
		{Name: "remember", Description: "Save a lasting fact for future chats: a decision, plan, life or income change, correction or preference the database can't show (e.g. 'decided to keep VWCE as core, no new satellites until 2027'). Use on your own whenever the owner tells you one — no approval needed.",
			InputSchema: obj(map[string]any{"note": str("one short sentence")}, "note")},
		{Name: "add_recurring", Description: "Add (or update, by name) a recurring item so the cash plan counts it: a standing order between accounts (kind transfer, from_account → to_account), a bill (from_account) or expected income (to_account). Monthly on the given day unless cadence says otherwise. ONLY after approval.",
			InputSchema: obj(map[string]any{"name": str("e.g. 'IBKR top-up' — an existing item with this name is updated"), "kind": str("transfer | bill | income"), "amount": num("EUR, positive"),
				"day": num("usual day of month, 1–31"), "from_account": str("account id"), "to_account": str("account id"), "category": str("category id, optional"),
				"cadence": str("monthly | quarterly | yearly, default monthly"), "note": str("optional")}, "name", "kind", "amount", "day")},
		{Name: "save_budget", Description: "Create a budget line (no id) or change one (id from get_budget_status; only the fields given change). kind: fixed (obligations), spending (envelopes; fund=true makes it a sinking fund that carries over), saving (transfers like transfer.invest). An amount change starts this month — past months keep theirs — unless from_month (YYYY-MM) or all_months is set. archived=true retires a line and keeps its history. ONLY after approval.",
			InputSchema: obj(map[string]any{"id": num("budget line id; omit to create"), "name": str("line name"), "kind": str("fixed | spending | saving"),
				"categories": arr("category ids the line covers (a parent includes its children)"), "tag": str("tag the line covers, e.g. kids or trip:rome"),
				"amount": num("EUR per period"), "period": str("monthly | yearly"), "fund": map[string]any{"type": "boolean", "description": "spending lines: carry unspent money over"},
				"from_month": str("YYYY-MM the new amount starts"), "all_months": map[string]any{"type": "boolean", "description": "rewrite the amount for every month"},
				"start_month": str("YYYY-MM the line starts; a fund sets money aside from this month, before it the line is a plain monthly envelope"), "archived": map[string]any{"type": "boolean"}})},
		{Name: "delete_budget", Description: "Delete a budget line and its history by id. Prefer save_budget with archived=true. ONLY after approval.", InputSchema: obj(map[string]any{"id": num("budget line id")}, "id")},
		{Name: "get_plan_settings", Description: "The plan's settings: income basis (income_mode median|manual|gross, manual_income, gross_salary, gross_changes — a new gross from a month on, monthly_deductions), salary_rules (when salary paid early in a month counts for the month before), salary_account, buffers per account, emergency_months, FI targets (target_age, birth_year, fi_monthly_spend, withdrawal_rate, expected_return), plus net salary from gross.", InputSchema: obj(map[string]any{})},
		{Name: "update_plan_settings", Description: "Change plan settings; only the fields given change, buffers merge per account (0 removes one). salary_rules replaces the whole list — send every rule, e.g. [{\"from\":\"\",\"paid_by_day\":3},{\"from\":\"2026-11-01\",\"paid_by_day\":10}] for a new job paying by the 10th for the month before. A raise or new salary from a later month goes in gross_changes (replaces the list — send every change), not gross_salary, so the months before keep the old salary; a finished month uses the salary that came in once its pay window has closed. ONLY after approval.",
			InputSchema: obj(map[string]any{"income_mode": str("median | manual | gross"), "manual_income": num("EUR net per month"), "gross_salary": num("EUR gross per month — the salary before any gross_changes"),
				"gross_changes":      map[string]any{"type": "array", "items": obj(map[string]any{"from": str("YYYY-MM the new salary starts"), "gross": num("EUR gross per month")})},
				"monthly_deductions": num("EUR"), "salary_rules": map[string]any{"type": "array", "items": obj(map[string]any{"from": str("YYYY-MM-DD or empty"), "paid_by_day": num("0–28")})},
				"salary_account": str("account id"), "buffers": map[string]any{"type": "object", "description": "account id → EUR floor", "additionalProperties": map[string]any{"type": "number"}},
				"emergency_months": num("months of spending"), "target_age": num("FI age"), "birth_year": num("year"), "fi_monthly_spend": num("EUR"), "withdrawal_rate": num("%"), "expected_return": num("% real")})},
		{Name: "tag_trip", Description: "Put transactions under one trip (tag trip:<name>), replacing other trip tags on them — use get_trips suggestions for the ids. ONLY after approval.",
			InputSchema: obj(map[string]any{"name": str("trip name, e.g. rome-2026"), "ids": map[string]any{"type": "array", "items": map[string]any{"type": "number"}}}, "name", "ids")},
		{Name: "get_goals", Description: "The wish list: goals (things saved up for) in priority order with price, saved, remaining, ETA at the average left over and per-month need for a target date; plus the funding month — income, paid-yourself-first (invested), spending, left over, already funded, available — and the suggested split by priority, and the last six months' left over. Each goal's held is the money set aside for it in a savings account; pots are those accounts with balance, set aside and free.",
			InputSchema: obj(map[string]any{"month": str("YYYY-MM to fund; default the last finished month")})},
		{Name: "save_goal", Description: "Add a wish-list goal (no id; it goes last in priority) or change one: name, price (target), target_date, note, link, status (active | bought | dropped). A planned trip is a goal with tag trip:<name> — its tagged spending counts as spent against it. priority_ids reorders the whole list. ONLY after approval.",
			InputSchema: obj(map[string]any{"id": num("goal id; omit to add"), "name": str("e.g. Bambu Lab X2D"), "target": num("price in EUR"), "target_date": str("YYYY-MM-DD, optional"),
				"note": str("optional"), "url": str("https link, optional"), "status": str("active | bought | dropped"), "tag": str("trip:<name> for a planned trip"),
				"priority_ids": map[string]any{"type": "array", "items": map[string]any{"type": "number"}, "description": "goal ids first to last, to reorder"}})},
		{Name: "fund_goals", Description: "Put a finished month's left over into goals. Without allocations, uses the suggested split (priority order). ONLY after approval.",
			InputSchema: obj(map[string]any{"month": str("YYYY-MM, a finished month"), "allocations": map[string]any{"type": "array", "items": obj(map[string]any{"goal_id": num("goal id"), "amount": num("EUR")})}}, "month")},
		{Name: "delete_rule", Description: "Delete a rule by id. ONLY after approval.", InputSchema: obj(map[string]any{"id": num("rule id")}, "id")},
		{Name: "rename_tag", Description: "Rename a tag everywhere (empty 'to' removes it). ONLY after approval.", InputSchema: obj(map[string]any{"from": str("tag"), "to": str("new tag")}, "from")},
		{Name: "update_inbox_row", Description: "Improve a bank inbox proposal (category, merchant, note, tags). Cannot accept it into the ledger — the user does that.",
			InputSchema: obj(map[string]any{"id": num("inbox row id"), "category": str("category id"), "merchant": str("merchant"), "note": str("note"), "tags": arr("tags")}, "id")},
	}
}

type toolArgs map[string]any

func (a toolArgs) s(k string) string {
	if v, ok := a[k].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
func (a toolArgs) f(k string) float64 {
	if v, ok := a[k].(float64); ok {
		return v
	}
	return 0
}
func (a toolArgs) b(k string) bool {
	v, _ := a[k].(bool)
	return v
}
func (a toolArgs) list(k string) ([]string, bool) {
	raw, ok := a[k].([]any)
	if !ok {
		return nil, false
	}
	var out []string
	for _, x := range raw {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out, true
}

func jsonOut(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	if len(b) > 60000 {
		return string(b[:60000]) + "… [truncated — narrow the query]", nil
	}
	return string(b), nil
}

func (a *Assistant) runTool(name string, raw json.RawMessage) (string, error) {
	var args toolArgs
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
	}
	now := time.Now()
	today := now.Format("2006-01-02")
	switch name {
	case "get_overview":
		return jsonOut(a.Overview(now))
	case "get_portfolio_quality":
		trades, err := wealth.ListTrades(a.DB)
		if err != nil {
			return "", err
		}
		p := wealth.BuildPortfolio(trades, true)
		st := wealth.LoadInvestSettings(a.DB, p)
		return jsonOut(map[string]any{"quality": wealth.BuildQuality(trades, p, st, func(tk string) ([]market.HistoryPoint, error) { return market.History(tk, "max") }, wealth.LiveEUR(), now), "settings": st})
	case "get_fun_facts":
		txs, err := ledger.All(a.DB, ledger.Filter{})
		if err != nil {
			return "", err
		}
		cats, _ := ledger.CategoryMap(a.DB)
		return jsonOut(insights.FunFacts(txs, cats, now, args.s("from"), args.s("to")))
	case "get_tax":
		var in struct {
			Year int `json:"year"`
		}
		json.Unmarshal(raw, &in)
		if in.Year == 0 {
			in.Year = now.Year()
		}
		return jsonOut2(cfo.Tax(a.DB, in.Year, now))
	case "get_forecast":
		var sc cfo.Scenario
		json.Unmarshal(raw, &sc)
		if sc.Gross == 0 && sc.Saving == nil && sc.Income == nil && sc.EverydayPct == 0 && len(sc.Purchases) == 0 && len(sc.SkipGoals) == 0 {
			return jsonOut2(cfo.CompareForecast(a.DB, now, 12, nil))
		}
		return jsonOut2(cfo.CompareForecast(a.DB, now, 12, &sc))
	case "search_transactions":
		f := ledger.Filter{From: args.s("from"), To: args.s("to"), Kind: args.s("kind"), Merchant: args.s("merchant"), Query: args.s("query"),
			Min: money.FromFloat(args.f("min")), Max: money.FromFloat(args.f("max")), Sort: args.s("sort"), Limit: int(args.f("limit"))}
		f.Categories, _ = args.list("categories")
		f.Accounts, _ = args.list("accounts")
		f.Tags, _ = args.list("tags")
		if f.Limit <= 0 {
			f.Limit = 50
		}
		if f.Limit > 200 {
			f.Limit = 200
		}
		return jsonOut2(ledger.List(a.DB, f))
	case "cash_flow":
		from, to := args.s("from"), args.s("to")
		if from == "" {
			from = now.AddDate(-1, 0, 0).Format("2006-01") + "-01"
		}
		if to == "" {
			to = today
		}
		txs, err := ledger.All(a.DB, ledger.Filter{From: from, To: to})
		if err != nil {
			return "", err
		}
		cats, _ := ledger.CategoryMap(a.DB)
		g := args.s("granularity")
		if g != "year" {
			g = "month"
		}
		return jsonOut(insights.CashFlow(txs, cats, g, plan.LoadSettings(a.DB).Salary()))
	case "spending_breakdown":
		from, to := args.s("from"), args.s("to")
		if from == "" || to == "" {
			from, to = insights.Window("month", now)
		}
		txs, err := ledger.All(a.DB, ledger.Filter{})
		if err != nil {
			return "", err
		}
		return jsonOut(map[string]any{"categories": insights.Breakdown(txs, from, to), "top_merchants": insights.MerchantTotals(txs, from, to, 15)})
	case "get_net_worth":
		book, err := wealth.LoadBook(a.DB)
		if err != nil {
			return "", err
		}
		if hf := args.s("history_from"); hf != "" {
			return jsonOut(book.History(hf, "", "month"))
		}
		d := args.s("date")
		if d == "" {
			d = today
		}
		return jsonOut(book.SnapshotAt(d, true))
	case "get_account_history":
		book, err := wealth.LoadBook(a.DB)
		if err != nil {
			return "", err
		}
		var pts []wealth.Point
		for _, p := range book.Series(args.s("account_id")) {
			if p.Date >= args.s("from") {
				pts = append(pts, p)
			}
		}
		if len(pts) > 400 {
			pts = pts[len(pts)-400:]
		}
		return jsonOut(pts)
	case "get_budget_status":
		m := args.s("month")
		if m == "" {
			m = now.Format("2006-01")
		}
		r, err := a.BudgetReport(m, now)
		if err != nil {
			return "", err
		}
		for i := range r.Lines {
			r.Lines[i].History = nil
		}
		for i := range r.Unbudgeted {
			r.Unbudgeted[i].History = nil
		}
		return jsonOut(r)
	case "get_recurring":
		txs, err := ledger.All(a.DB, ledger.Filter{From: now.AddDate(-2, 0, 0).Format("2006-01-02")})
		if err != nil {
			return "", err
		}
		list, _, err := insights.RecurringAll(a.DB, txs, now)
		if err != nil {
			return "", err
		}
		return jsonOut(list)
	case "get_fi":
		txs, err := ledger.All(a.DB, ledger.Filter{From: now.AddDate(-2, 0, 0).Format("2006-01-02")})
		if err != nil {
			return "", err
		}
		cats, _ := ledger.CategoryMap(a.DB)
		book, err := wealth.LoadBook(a.DB)
		if err != nil {
			return "", err
		}
		return jsonOut(insights.ComputeFI(txs, cats, book, plan.LoadSettings(a.DB), now))
	case "get_loans":
		book, err := wealth.LoadBook(a.DB)
		if err != nil {
			return "", err
		}
		txs, _ := ledger.All(a.DB, ledger.Filter{From: now.AddDate(-1, 0, 0).Format("2006-01-02")})
		return jsonOut(wealth.Loans(book, txs, now))
	case "get_portfolio":
		trades, err := wealth.ListTrades(a.DB)
		if err != nil {
			return "", err
		}
		p := wealth.BuildPortfolio(trades, true)
		if args.b("include_trades") {
			return jsonOut(map[string]any{"portfolio": p, "trades": trades})
		}
		return jsonOut(p)
	case "get_quote":
		tk := strings.ToUpper(args.s("ticker"))
		if !wealth.ValidTicker(tk) {
			return "", errors.New("invalid ticker")
		}
		q, err := market.Fetch(tk)
		if err != nil {
			return "", err
		}
		an, _ := market.Analyst(tk)
		return jsonOut(map[string]any{"quote": q, "analyst": an})
	case "get_market_buzz":
		tk := strings.ToUpper(args.s("ticker"))
		if !wealth.ValidTicker(tk) {
			return "", errors.New("invalid ticker")
		}
		return jsonOut(map[string]any{"fear_greed": market.FearGreed(), "headlines": market.Headlines(tk, 8), "social": market.SocialBuzz(tk, 8)})
	case "get_reconciliation":
		from := args.s("from")
		if _, err := time.Parse("2006-01-02", from); err != nil {
			from = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
		}
		book, err := wealth.LoadBook(a.DB)
		if err != nil {
			return "", err
		}
		txs, err := ledger.All(a.DB, ledger.Filter{From: time.Now().AddDate(-1, 0, 0).Format("2006-01-02")})
		if err != nil {
			return "", err
		}
		return jsonOut(insights.Reconcile(book, txs, from))
	case "get_month_review":
		return jsonOut2(cfo.BuildMonthReview(a.DB, args.s("month"), now))
	case "get_checks":
		return jsonOut(cfo.Checks(a.DB, now))
	case "get_owed":
		return jsonOut2(ledger.Owed(a.DB))
	case "get_net_worth_movement":
		book, err := wealth.LoadBook(a.DB)
		if err != nil {
			return "", err
		}
		from, to := args.s("from"), args.s("to")
		if to == "" {
			to = now.Format("2006-01-02")
		}
		if from == "" {
			from = now.AddDate(-1, 0, 0).Format("2006-01-02")
		}
		mv := book.Movement(from, to)
		if mv == nil {
			mv = []wealth.Move{}
		}
		return jsonOut(mv)
	case "get_trip_untagged":
		likely, maybe, book, err := ledger.TripUntagged(a.DB, args.s("tag"))
		if err != nil {
			return "", err
		}
		return jsonOut(map[string]any{"in_dates": likely, "maybe": maybe, "bookings": book})
	case "set_trip_dates":
		if b, _ := args["clear"].(bool); b {
			if err := ledger.DeleteTripDates(a.DB, args.s("tag")); err != nil {
				return "", err
			}
		} else if err := ledger.SaveTripDates(a.DB, ledger.TripDates{Tag: args.s("tag"), Start: args.s("start"), End: args.s("end")}); err != nil {
			return "", err
		}
		return jsonOut2(ledger.ListTripDates(a.DB))
	case "get_trips":
		txs, err := ledger.All(a.DB, ledger.Filter{})
		if err != nil {
			return "", err
		}
		budgets, _ := plan.List(a.DB, false)
		trips, sugg := plan.Trips(txs, budgets)
		var planned []goals.Goal // trips being saved for on the wish list
		saving := map[string]bool{}
		if all, err := goals.List(a.DB); err == nil {
			for _, g := range all {
				if g.Tag != "" && g.Status == "active" {
					planned = append(planned, g)
					saving[g.Tag] = true
				}
			}
		}
		dates, _ := ledger.ListTripDates(a.DB)
		states, _ := ledger.TripStates(a.DB)
		plan.MarkDone(trips, dates, states, saving, time.Now())
		return jsonOut(map[string]any{"trips": trips, "suggestions": sugg, "planned": planned, "dates": dates})
	case "get_reference":
		switch args.s("what") {
		case "categories":
			return jsonOut2(ledger.ListCategories(a.DB))
		case "accounts":
			return jsonOut2(ledger.ListAccounts(a.DB))
		case "tags":
			tags, err := ledger.ListTags(a.DB)
			sort.Slice(tags, func(i, j int) bool { return tags[i].Count > tags[j].Count })
			return jsonOut2(tags, err)
		case "rules":
			return jsonOut2(ledger.ListRules(a.DB))
		case "merchants":
			m, err := ledger.ListMerchants(a.DB, now.AddDate(-2, 0, 0).Format("2006-01-02"), today)
			if len(m) > 150 {
				m = m[:150]
			}
			return jsonOut2(m, err)
		}
		return "", errors.New("what must be categories, accounts, tags, rules or merchants")
	case "ibkr_live":
		if a.IBKR == nil {
			return "", ibkr.ErrNotConnected
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		out, err := a.IBKR.LiveView(ctx, args.s("what"), args.s("period"))
		if err != nil {
			return "", err
		}
		return string(out), nil
	case "get_inbox":
		if a.Bank == nil {
			return "[]", nil
		}
		return jsonOut2(a.Bank.Inbox("open"))
	case "create_transaction":
		t := ledger.Tx{Date: args.s("date"), Amount: money.FromFloat(args.f("amount")), Category: args.s("category"), Merchant: args.s("merchant"),
			Note: args.s("note"), AccountID: args.s("account_id"), ToAccountID: args.s("to_account_id"), Source: "manual"}
		t.Tags, _ = args.list("tags")
		if err := ledger.Validate(a.DB, &t); err != nil {
			return "", err
		}
		if err := ledger.Insert(a.DB, &t); err != nil {
			return "", err
		}
		var moved []string
		for acct, delta := range wealth.TxDeltas(t.Kind, t.AccountID, t.ToAccountID, t.Amount) {
			if ok, _ := wealth.ApplyDelta(a.DB, acct, t.Date, delta); ok {
				moved = append(moved, acct)
			}
		}
		return jsonOut(map[string]any{"created": t, "balances_moved": moved})
	case "update_transaction":
		t, err := ledger.Get(a.DB, int64(args.f("id")))
		if err != nil {
			return "", errors.New("transaction not found")
		}
		if v := args.s("category"); v != "" {
			t.Category = v
			t.Kind = ""
		}
		if _, ok := args["merchant"]; ok {
			t.Merchant = args.s("merchant")
		}
		if _, ok := args["note"]; ok {
			t.Note = args.s("note")
		}
		if tags, ok := args.list("tags"); ok {
			t.Tags = tags
		}
		if err := ledger.Validate(a.DB, &t); err != nil {
			return "", err
		}
		if err := ledger.Update(a.DB, &t); err != nil {
			return "", err
		}
		return jsonOut(map[string]any{"updated": t})
	case "add_rule":
		r := ledger.Rule{Pattern: args.s("pattern"), WhenCategory: args.s("when_category"), SetCategory: args.s("set_category"),
			SetMerchant: args.s("set_merchant"), Enabled: true}
		r.AddTags, _ = args.list("add_tags")
		if r.Pattern == "" && r.WhenCategory == "" {
			return "", errors.New("a rule needs a pattern or a when_category")
		}
		if err := ledger.SaveRule(a.DB, &r); err != nil {
			return "", err
		}
		res := map[string]any{"rule": r}
		if args.b("apply_to_history") {
			n, err := ledger.ApplyRuleToHistory(a.DB, r)
			if err != nil {
				return "", err
			}
			res["history_rows_changed"] = n
		}
		return jsonOut(res)
	case "remember":
		note := args.s("note")
		if note == "" {
			return "", errors.New("nothing to remember")
		}
		if err := a.Remember(note); err != nil {
			return "", err
		}
		return `{"remembered":true}`, nil
	case "add_recurring":
		it := insights.RecurringItem{Merchant: args.s("name"), Kind: args.s("kind"), Amount: money.FromFloat(args.f("amount")), Day: int(args.f("day")),
			FromAccount: args.s("from_account"), ToAccount: args.s("to_account"), Category: args.s("category"), Cadence: args.s("cadence"), Note: args.s("note")}
		if it.Amount <= 0 || it.Day < 1 {
			return "", errors.New("amount and a day of month (1–31) are required")
		}
		if (it.Kind == "transfer" && (it.FromAccount == "" || it.ToAccount == "")) || (it.Kind == "bill" && it.FromAccount == "") || (it.Kind == "income" && it.ToAccount == "") {
			return "", errors.New("a transfer needs from_account and to_account, a bill from_account, income to_account")
		}
		if err := insights.SaveRecurringItem(a.DB, &it); err != nil {
			return "", err
		}
		return jsonOut(map[string]any{"saved": it})
	case "save_budget":
		var p plan.BudgetPatch
		if err := json.Unmarshal(raw, &p); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		b, err := plan.Patch(a.DB, p, now)
		return jsonOut2(map[string]any{"saved": b}, err)
	case "delete_budget":
		if _, err := plan.Get(a.DB, int64(args.f("id"))); err != nil {
			return "", err
		}
		return `{"deleted":true}`, plan.Delete(a.DB, int64(args.f("id")))
	case "get_plan_settings":
		st := plan.LoadSettings(a.DB)
		return jsonOut(map[string]any{"settings": st, "net_from_gross": plan.LTNetSalary(st.GrossFor(time.Now().Format("2006-01")), st.MonthlyDeductions)})
	case "update_plan_settings":
		st, err := plan.PatchSettings(a.DB, raw)
		return jsonOut2(map[string]any{"saved": st}, err)
	case "tag_trip":
		var in struct {
			Name string  `json:"name"`
			IDs  []int64 `json:"ids"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		tag, n, err := plan.TagTrip(a.DB, in.Name, in.IDs)
		return jsonOut2(map[string]any{"tag": tag, "transactions_tagged": n}, err)
	case "get_goals":
		return jsonOut2(goals.Build(a.DB, args.s("month"), now))
	case "save_goal":
		if ids, ok := args["priority_ids"].([]any); ok && len(ids) > 0 {
			var order []int64
			for _, x := range ids {
				if f, ok := x.(float64); ok {
					order = append(order, int64(f))
				}
			}
			if err := goals.Reorder(a.DB, order); err != nil {
				return "", err
			}
			if args.f("id") == 0 && args.s("name") == "" {
				return jsonOut2(goals.List(a.DB))
			}
		}
		g := goals.Goal{}
		if id := int64(args.f("id")); id != 0 {
			cur, err := goals.Get(a.DB, id)
			if err != nil {
				return "", err
			}
			g = cur
		}
		for k, set := range map[string]*string{"name": &g.Name, "target_date": &g.TargetDate, "note": &g.Note, "url": &g.URL, "status": &g.Status, "tag": &g.Tag} {
			if _, ok := args[k]; ok {
				*set = args.s(k)
			}
		}
		if _, ok := args["target"]; ok {
			g.Target = money.FromFloat(args.f("target"))
		}
		if err := goals.Save(a.DB, &g); err != nil {
			return "", err
		}
		return jsonOut2(goals.Get(a.DB, g.ID))
	case "fund_goals":
		var in struct {
			Month       string `json:"month"`
			Allocations []struct {
				GoalID int64   `json:"goal_id"`
				Amount float64 `json:"amount"`
			} `json:"allocations"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		var allocs []goals.Allocation
		if len(in.Allocations) == 0 {
			p, err := goals.Build(a.DB, in.Month, now)
			if err != nil {
				return "", err
			}
			allocs = p.Proposal
		}
		for _, x := range in.Allocations {
			allocs = append(allocs, goals.Allocation{GoalID: x.GoalID, Amount: money.FromFloat(x.Amount)})
		}
		p, err := goals.Fund(a.DB, in.Month, allocs, now)
		if err != nil {
			return "", err
		}
		return jsonOut(map[string]any{"funded": allocs, "goals": p.Goals, "month": p.Funding})
	case "delete_rule":
		return `{"deleted":true}`, ledger.DeleteRule(a.DB, int64(args.f("id")))
	case "rename_tag":
		n, err := ledger.RenameTag(a.DB, args.s("from"), strings.ToLower(args.s("to")))
		return jsonOut2(map[string]int{"transactions_changed": n}, err)
	case "update_inbox_row":
		if a.Bank == nil {
			return "", errors.New("banking unavailable")
		}
		e := bank.Edit{}
		if v := args.s("category"); v != "" {
			e.Category = &v
		}
		if _, ok := args["merchant"]; ok {
			v := args.s("merchant")
			e.Merchant = &v
		}
		if _, ok := args["note"]; ok {
			v := args.s("note")
			e.Note = &v
		}
		if tags, ok := args.list("tags"); ok {
			e.Tags = &tags
		}
		return jsonOut2(a.Bank.Update(int64(args.f("id")), e))
	}
	return "", fmt.Errorf("unknown tool %q", name)
}

func jsonOut2[T any](v T, err error) (string, error) {
	if err != nil {
		return "", err
	}
	return jsonOut(v)
}
