package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"ft/internal/mcptools"
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
	"rename_tag": true, "update_inbox_row": true, "add_recurring": true, "save_budget": true, "delete_budget": true, "update_plan_settings": true, "tag_trip": true, "save_goal": true, "fund_goals": true, "set_trip_dates": true,
	"log_decision": true}

func toolDefs() []tool {
	date := "YYYY-MM-DD"
	return []tool{
		{Type: webSearchType, Name: "web_search", MaxUses: 5},
		{Name: "get_overview", Description: "Today's CFO snapshot: net worth and its parts, this month's cash flow vs the trailing average, plan pulse (left to spend — field safe_to_spend, budget lines over limit), emergency-fund months, FI progress, bank inbox size.", InputSchema: obj(map[string]any{})},
		{Name: "search_transactions", Description: "Find transactions. Returns up to 100 rows (limit, max 500) plus totals over the whole match. Categories are ids like 'food' or 'food.groceries' (a parent includes children).",
			InputSchema: obj(map[string]any{"from": str(date), "to": str(date), "kind": str("income | expense | transfer"), "categories": arr("category ids"),
				"accounts": arr("account ids"), "tags": arr("tags (any match)"), "merchant": str("exact merchant"), "query": str("free text over merchant/note/tags"),
				"min": num("min amount EUR"), "max": num("max amount EUR"), "sort": str("date | amount"), "limit": num("max rows, default 100, at most 500")})},
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
		{Name: "get_payday_runbook", Description: "Pay yourself first, as a checklist for the latest salary: each saving line's amount with the account it usually leaves from and goes to, the wish list's share of last month's leftovers, and bill top-ups due this week. Each step says done / partial / todo from the transfers the bank shows since payday. Read-only — it moves nothing.", InputSchema: obj(map[string]any{})},
		{Name: "get_next_euro", Description: "Where the next sum (default €500) is best put, ranked in the owner's numbers: the emergency buffer while short, the 3rd pillar while this year's 20% refund has room (pre-2025 contracts), a wish-list goal about to miss its date, then extra loan principal at the loan's rate vs investing at the expected return minus 2 points for uncertainty; the sum split down that list. Use it instead of reasoning out returns yourself.", InputSchema: obj(map[string]any{"amount": num("euros to place, default 500")})},
		{Name: "get_lifestyle_creep", Description: "Lifestyle creep: the last 12 complete months against the same 12 months a year earlier (years=3 for three) — income, spending and essential growth in %, the gap in points, the categories that grew and fell most, and regular bills that started since.", InputSchema: obj(map[string]any{"years": num("1 (default) or 3")})},
		{Name: "get_price_creep", Description: "Price creep on regular bills: each bill's latest charge against the one about a year earlier (% and € a year), renewals in the next 45 days (yearly ones found over two years), the personal inflation rate (those bills' price change weighted by annual cost) and Lithuania's official HICP rate when available.", InputSchema: obj(map[string]any{})},
		{Name: "get_decisions", Description: "The decision journal: purchases being considered (with cooling-off date), being saved for on the wish list (status saving, goal_id), bought or skipped, and how bought ones were rated (worth it yes/meh/no); plus the record — money kept by skipping, money regretted, worth-it rate by category.", InputSchema: obj(map[string]any{})},
		{Name: "get_decision_impact", Description: "What a purchase of a given amount would do, as plain sentences: share of this month's left to spend, days of usual spending, buffer months after paying from cash, how far the wish list slips, how much later financial independence comes.", InputSchema: obj(map[string]any{"amount": num("price in euros")}, "amount")},
		{Name: "log_decision", Description: "Writes a purchase being considered into the decision journal (title, amount in euros, optional category id and note); it starts 'thinking' with a cooling-off date by size. It never decides, rates or deletes. Confirm first.", InputSchema: obj(map[string]any{"title": str("what is being considered"), "amount": num("price in euros"), "category": str("category id, e.g. shopping.electronics"), "note": str("why")}, "title", "amount")},
		{Name: "get_ownership_costs", Description: "True cost of owning each property and vehicle over the last 12 months: running costs (spending tagged with it, by category), loan interest, value lost or gained, and what the owner's equity would earn invested (an estimate); a year and a month; per km for vehicles and a verdict against the owner's alternative (taxi/rental per km, or rent) when set.", InputSchema: obj(map[string]any{})},
		{Name: "get_fi_plan", Description: "Financial independence as levels, in today's euros: when the safe payout (withdrawal rate) of investable money covers fixed obligations, then essentials, then full spending, then spending +25% — capital needed, progress and date each; coast FI (when contributions could stop and FI still arrives by the target age); yearly projected payout a month; what +€100/250/500 a month does; savings rate; money locked in pensions.", InputSchema: obj(map[string]any{})},
		{Name: "get_milestones", Description: "Dated milestones at today's pace: emergency fund full, the next two round figures of liquid wealth, each loan paid off on its schedule, financial independence. Projections from the app's own figures — say so.", InputSchema: obj(map[string]any{})},
		{Name: "get_fees", Description: "What investments cost a year: each fund's ongoing charge plus platform fee (owner's figure, or a typical one for well-known ETFs — say which), € a year, 20-year drag at the expected return, and the saving against a 0.2% index fund; funds without a figure are counted as missing.", InputSchema: obj(map[string]any{})},
		{Name: "get_subscriptions", Description: "The subscription audit: regular charges you could cancel (obligations left out) with yearly cost, new ones (first charged within 90 days), ones that began as a free trial, and the owner's keep / cancel-by verdicts — including any charged again after being marked to cancel.", InputSchema: obj(map[string]any{})},
		{Name: "get_cash_calendar", Description: "Cash (checking, savings, cash) day by day for the next days (default 60): each bill, standing order, transfer and income on its date, the usual everyday spending taken off daily, the lowest day and the first day under the emergency floor or below zero.", InputSchema: obj(map[string]any{"days": num("7–120, default 60")})},
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

// ToolNames lists Ask CFO's own tools (not the provider's web search).
func ToolNames() []string {
	var out []string
	for _, d := range toolDefs() {
		if d.Type == "" {
			out = append(out, d.Name)
		}
	}
	return out
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

// Ask CFO's older names for the MCP tools it shares.
var mcpName = map[string]string{"add_rule": "save_rule", "add_recurring": "save_recurring", "get_inbox": "get_bank_inbox"}

// RunTool runs one tool the way the chat does. Each tool is the MCP tool of
// the same name (internal/mcptools) over the app's own routes, so Ask CFO,
// Claude's connector and the web share one implementation of every figure
// and rule — Ask CFO used to compute its own, and its cash flow drifted
// (a salary paid on the 1st–3rd after the window went missing).
func (a *Assistant) RunTool(ctx context.Context, name string, raw json.RawMessage) (string, error) {
	args := toolArgs{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
	}
	switch name {
	case "add_recurring":
		// The cash plan can only place an item it knows the day and the
		// accounts of; the web's form asks for these itself.
		kind := args.s("kind")
		if args.f("amount") <= 0 || args.f("day") < 1 {
			return "", errors.New("amount and a day of month (1–31) are required")
		}
		if (kind == "transfer" && (args.s("from_account") == "" || args.s("to_account") == "")) || (kind == "bill" && args.s("from_account") == "") || (kind == "income" && args.s("to_account") == "") {
			return "", errors.New("a transfer needs from_account and to_account, a bill from_account, income to_account")
		}
	case "get_portfolio":
		withTrades := args.b("include_trades")
		delete(args, "include_trades")
		p, err := a.callTool(ctx, name, args)
		if err != nil || !withTrades {
			return trimmed(p, err)
		}
		t, err := a.callTool(ctx, "get_trades", toolArgs{})
		if err != nil {
			return "", err
		}
		return trimmed(fmt.Sprintf(`{"portfolio":%s,"trades":%s}`, p, t), nil)
	}
	if n, ok := mcpName[name]; ok {
		name = n
	}
	out, err := a.callTool(ctx, name, args)
	if err != nil {
		return "", err
	}
	return trimmed(trim(name, args, out), nil)
}

// callTool runs one MCP tool in process and returns its text.
func (a *Assistant) callTool(ctx context.Context, name string, args toolArgs) (string, error) {
	cs, props, err := a.toolSession(ctx)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any(lenient(args, props[name]))})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, c := range res.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			b.WriteString(t.Text)
		}
	}
	if res.IsError {
		return "", errors.New(b.String())
	}
	return b.String(), nil
}

// lenient fits the model's arguments to the tool's schema, as the old
// hand-written tools did: the MCP schemas refuse nulls, extra fields and
// 20.5 where a whole number belongs, and each refusal cost a chat round.
// props maps a property to its JSON type.
func lenient(args toolArgs, props map[string]string) toolArgs {
	out := toolArgs{}
	for k, v := range args {
		typ, known := props[k]
		if v == nil || !known {
			continue
		}
		if f, ok := v.(float64); ok && typ == "integer" {
			v = math.Round(f)
		}
		out[k] = v
	}
	return out
}

// toolSession connects to the MCP tools over a.Call and reads their
// argument types. A failed connect is tried again on the next call.
func (a *Assistant) toolSession(ctx context.Context) (*mcp.ClientSession, map[string]map[string]string, error) {
	a.toolsMu.Lock()
	defer a.toolsMu.Unlock()
	if a.tools != nil {
		return a.tools, a.toolProps, nil
	}
	if a.Call == nil {
		return nil, nil, errors.New("tools are not available")
	}
	st, ct := mcp.NewInMemoryTransports()
	if _, err := mcptools.New(a.Call).Connect(context.Background(), st, nil); err != nil {
		return nil, nil, err
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "ask-cfo"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		return nil, nil, err
	}
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		cs.Close()
		return nil, nil, err
	}
	props := map[string]map[string]string{}
	for _, t := range list.Tools {
		props[t.Name] = propTypes(t.InputSchema)
	}
	a.tools, a.toolProps = cs, props
	return cs, props, nil
}

// propTypes reads a tool's input schema into property → JSON type (the
// non-null one when the schema allows null too).
func propTypes(schema any) map[string]string {
	out := map[string]string{}
	s, _ := schema.(map[string]any)
	ps, _ := s["properties"].(map[string]any)
	for k, p := range ps {
		m, _ := p.(map[string]any)
		switch t := m["type"].(type) {
		case string:
			out[k] = t
		case []any:
			for _, x := range t {
				if x != "null" {
					out[k], _ = x.(string)
				}
			}
		default:
			out[k] = ""
		}
	}
	return out
}

// trim drops what Ask CFO doesn't need from a result, to keep each chat
// round small: the Home sparkline and recent rows, budget lines' history,
// all but the 150 most used merchants. Tags come most used first.
func trim(name string, args toolArgs, out string) string {
	var drop func(v any) any
	switch {
	case name == "get_overview":
		drop = func(v any) any {
			if m, ok := v.(map[string]any); ok {
				delete(m, "spark")
				delete(m, "recent")
			}
			return v
		}
	case name == "get_budget_status":
		drop = func(v any) any {
			if m, ok := v.(map[string]any); ok {
				for _, k := range []string{"lines", "unbudgeted"} {
					rows, _ := m[k].([]any)
					for _, r := range rows {
						if row, ok := r.(map[string]any); ok {
							delete(row, "history")
						}
					}
				}
			}
			return v
		}
	case name == "get_reference" && args.s("what") == "merchants":
		drop = func(v any) any {
			if rows, ok := v.([]any); ok && len(rows) > 150 {
				return rows[:150]
			}
			return v
		}
	case name == "get_reference" && args.s("what") == "tags":
		drop = func(v any) any {
			if rows, ok := v.([]any); ok {
				count := func(i int) float64 {
					m, _ := rows[i].(map[string]any)
					c, _ := m["count"].(float64)
					return c
				}
				sort.SliceStable(rows, func(i, j int) bool { return count(i) > count(j) })
			}
			return v
		}
	default:
		return out
	}
	var v any
	if json.Unmarshal([]byte(out), &v) != nil {
		return out
	}
	b, err := json.Marshal(drop(v))
	if err != nil {
		return out
	}
	return string(b)
}

// trimmed caps a result so one tool can't fill the model's context.
func trimmed(out string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	if len(out) > 60000 {
		return out[:60000] + "… [truncated — narrow the query]", nil
	}
	return out, nil
}
