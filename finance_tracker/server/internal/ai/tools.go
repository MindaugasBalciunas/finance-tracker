package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"ft/internal/bank"
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
	"rename_tag": true, "update_inbox_row": true, "add_recurring": true, "save_budget": true, "delete_budget": true, "update_plan_settings": true, "tag_trip": true}

func toolDefs() []tool {
	date := "YYYY-MM-DD"
	return []tool{
		{Type: webSearchType, Name: "web_search", MaxUses: 5},
		{Name: "get_overview", Description: "Today's CFO snapshot: net worth and its parts, this month's cash flow vs the trailing average, plan pulse (safe to spend, budget lines over limit), emergency-fund months, FI progress, bank inbox size.", InputSchema: obj(map[string]any{})},
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
		{Name: "get_budget_status", Description: "The monthly plan: every budget line (with its id for save_budget) (fixed / spending / saving, funds and yearly lines), spent vs budgeted, suggestions, unbudgeted spending, safe-to-spend.",
			InputSchema: obj(map[string]any{"month": str("YYYY-MM, default current")})},
		{Name: "get_recurring", Description: "Recurring money movements the cash plan uses: bills and subscriptions, standing orders between accounts (transfers) and expected income — amount, monthly cost, accounts, usual day, next date, whether this month's one is done. source: detected, edited or manual (added by the owner).", InputSchema: obj(map[string]any{})},
		{Name: "get_fi", Description: "Financial-independence projection: target, investable assets, progress, years to FI, required monthly saving for the target age, emergency fund.", InputSchema: obj(map[string]any{})},
		{Name: "get_loans", Description: "Loans (mortgage): balance, rate, next payment split, payoff date, equity and LTV, principal/interest paid in the last 12 months.", InputSchema: obj(map[string]any{})},
		{Name: "get_portfolio", Description: "Investment positions with live prices, cost basis, gains (EUR), plus trades if asked.", InputSchema: obj(map[string]any{"include_trades": map[string]any{"type": "boolean"}})},
		{Name: "get_quote", Description: "Live quote, 52-week range and analyst targets for a ticker.", InputSchema: obj(map[string]any{"ticker": str("ticker symbol")}, "ticker")},
		{Name: "get_market_buzz", Description: "News headlines, public social chatter and Fear & Greed for a ticker. Social chatter is unverified.", InputSchema: obj(map[string]any{"ticker": str("ticker symbol")}, "ticker")},
		{Name: "get_trips", Description: "Trips (trip:* tags) with totals, per day and category split, plus untagged travel spending that looks like a trip.", InputSchema: obj(map[string]any{})},
		{Name: "get_reference", Description: "Reference data: categories, accounts, tags, rules or merchants.", InputSchema: obj(map[string]any{"what": str("categories | accounts | tags | rules | merchants")}, "what")},
		{Name: "ibkr_live", Description: "Live, read-only data from the owner's Interactive Brokers account (when connected in Settings → Banks): 'summary' (net liquidation, cash, buying power), 'positions' (quantity, price, value, P&L, cost basis), balances' (cash by currency), 'orders' (live orders and their status), 'instructions' (saved order instructions — not live until the owner submits them in IBKR), or 'trades' for a period (TODAY, DAYS_7, DAYS_30, DAYS_90, MONTH_TO_DATE, YEAR_TO_DATE, LAST_QUARTER…). Prefer it over get_portfolio for what IBKR holds right now; it cannot trade.",
			InputSchema: obj(map[string]any{"what": str("summary | positions | balances | orders | instructions | trades"), "period": str("for trades; default DAYS_30")}, "what")},
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
				"start_month": str("YYYY-MM the line starts"), "archived": map[string]any{"type": "boolean"}})},
		{Name: "delete_budget", Description: "Delete a budget line and its history by id. Prefer save_budget with archived=true. ONLY after approval.", InputSchema: obj(map[string]any{"id": num("budget line id")}, "id")},
		{Name: "get_plan_settings", Description: "The plan's settings: income basis (income_mode median|manual|gross, manual_income, gross_salary, monthly_deductions), salary_rules (when salary paid early in a month counts for the month before), salary_account, buffers per account, emergency_months, FI targets (target_age, birth_year, fi_monthly_spend, withdrawal_rate, expected_return), plus net salary from gross.", InputSchema: obj(map[string]any{})},
		{Name: "update_plan_settings", Description: "Change plan settings; only the fields given change, buffers merge per account (0 removes one). salary_rules replaces the whole list — send every rule, e.g. [{\"from\":\"\",\"paid_by_day\":3},{\"from\":\"2026-11-01\",\"paid_by_day\":10}] for a new job paying by the 10th for the month before. ONLY after approval.",
			InputSchema: obj(map[string]any{"income_mode": str("median | manual | gross"), "manual_income": num("EUR net per month"), "gross_salary": num("EUR gross per month"),
				"monthly_deductions": num("EUR"), "salary_rules": map[string]any{"type": "array", "items": obj(map[string]any{"from": str("YYYY-MM-DD or empty"), "paid_by_day": num("0–28")})},
				"salary_account": str("account id"), "buffers": map[string]any{"type": "object", "description": "account id → EUR floor", "additionalProperties": map[string]any{"type": "number"}},
				"emergency_months": num("months of spending"), "target_age": num("FI age"), "birth_year": num("year"), "fi_monthly_spend": num("EUR"), "withdrawal_rate": num("%"), "expected_return": num("% real")})},
		{Name: "tag_trip", Description: "Put transactions under one trip (tag trip:<name>), replacing other trip tags on them — use get_trips suggestions for the ids. ONLY after approval.",
			InputSchema: obj(map[string]any{"name": str("trip name, e.g. rome-2026"), "ids": map[string]any{"type": "array", "items": map[string]any{"type": "number"}}}, "name", "ids")},
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
	case "get_trips":
		txs, err := ledger.All(a.DB, ledger.Filter{})
		if err != nil {
			return "", err
		}
		budgets, _ := plan.List(a.DB, false)
		trips, sugg := plan.Trips(txs, budgets)
		return jsonOut(map[string]any{"trips": trips, "suggestions": sugg})
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
		if a.IBKR == nil || !a.IBKR.Connected() {
			return "", ibkr.ErrNotConnected
		}
		tool := map[string]string{"summary": "get_account_summary", "positions": "get_account_positions", "balances": "get_account_balances", "trades": "get_account_trades",
			"orders": "get_account_orders", "instructions": "get_order_instructions"}[args.s("what")]
		if tool == "" {
			return "", fmt.Errorf("what must be summary, positions, balances, trades, orders or instructions")
		}
		var in map[string]any
		if tool == "get_account_trades" {
			p := strings.ToUpper(args.s("period"))
			if p == "" {
				p = "DAYS_30"
			}
			in = map[string]any{"period": p}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		out, err := a.IBKR.Live(ctx, tool, in)
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
		return jsonOut(map[string]any{"settings": st, "net_from_gross": plan.LTNetSalary(st.GrossSalary, st.MonthlyDeductions)})
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
