package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"ft/internal/advice"
)

// instructions reach the client when it connects: the same data model,
// strategy and trust rules Ask CFO follows in the app.
var instructions = "You are working with the owner's personal finances (Lithuania, EUR) in their Finance Tracker app, through these tools. Call get_user_context first and follow the owner's brief and the decisions they asked to remember.\n\n" +
	advice.DataModel + "\n\n" + advice.Strategy + "\n\n" + advice.Trust + `

Changing data: tools marked WRITES need a read-write token. Propose first, use them only after the owner approves that specific change, then report exactly what changed. Accepting bank rows into the ledger stays with the owner, in the app. When the owner tells you something lasting the data can't show (a decision, a plan, an income change), save it with remember.

Answer style: direct, numbers first, compact tables over prose.

Ready-made analyses (prompts): month_close, budget_review, wishlist_review, reconcile_recent.`

type reconArgs struct {
	From string `json:"from,omitempty" jsonschema:"YYYY-MM-DD, default 30 days ago"`
}

// now is replaceable in tests.
var now = time.Now

func addPrompts(s *mcp.Server) {
	monthArg := func(def string) []*mcp.PromptArgument {
		return []*mcp.PromptArgument{{Name: "month", Title: "Month", Description: "YYYY-MM, default " + def}}
	}
	prompt := func(name, title, desc string, args []*mcp.PromptArgument, body func(map[string]string) (string, error)) {
		s.AddPrompt(&mcp.Prompt{Name: name, Title: title, Description: desc, Arguments: args},
			func(ctx context.Context, r *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				text, err := body(r.Params.Arguments)
				if err != nil {
					return nil, err
				}
				return &mcp.GetPromptResult{Description: desc, Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: text}}}}, nil
			})
	}

	prompt("month_close", "Month close", "Close a finished month: cash flow vs the usual, the budget, what drove it, and the wish-list funding.", monthArg("the last finished month"),
		func(a map[string]string) (string, error) {
			m, err := month(a["month"], firstOfMonth(now()).AddDate(0, -1, 0))
			if err != nil {
				return "", err
			}
			first, last := monthBounds(m)
			prev := m.AddDate(0, -6, 0).Format("2006-01-02")
			return fmt.Sprintf(`Close %[1]s for me.

1. get_user_context.
2. cash_flow from %[4]s to %[3]s: %[1]s's income, spending (essential / discretionary), saved, savings rate and invested, against the average of the 6 months before.
3. get_budget_status month=%[5]s: lines over or well under budget, and unbudgeted spending.
4. spending_breakdown from %[2]s to %[3]s: the categories and merchants that moved most against the month before.
5. get_goals month=%[5]s: the funding (income − spending − invested = left over) and the suggested split.

Answer with one verdict line, a table of the key numbers (this month vs the 6-month average), the 3 things that drove the month, and the proposed wish-list funding. Ask before calling fund_goals.`, m.Format("January 2006"), first, last, prev, m.Format("2006-01")), nil
		})

	prompt("budget_review", "Budget review", "Lines that are over or under, and concrete changes to the plan — applied only after approval.", monthArg("this month"),
		func(a map[string]string) (string, error) {
			m, err := month(a["month"], now())
			if err != nil {
				return "", err
			}
			ym := m.Format("2006-01")
			back := func(n int) string { return m.AddDate(0, -n, 0).Format("2006-01") }
			return fmt.Sprintf(`Review my budget for %s.

1. get_user_context, then get_budget_status month=%s, get_plan_settings and get_recurring.
2. get_budget_status for %s, %s and %s: which lines run over or well under month after month, and which spending has no line.
3. Check the plan covers every recurring bill and standing order.

Propose changes as a table (line, now → new, why), with the monthly effect in total. Apply the ones I approve with save_budget / update_plan_settings — amount changes start this month so past months stay as planned.`, m.Format("January 2006"), ym, back(1), back(2), back(3)), nil
		})

	prompt("wishlist_review", "Wish list and trips", "Are the goals and planned trips realistic at the left-over rate, and what would bring them closer?", nil,
		func(map[string]string) (string, error) {
			from := now().AddDate(-1, 0, 0).Format("2006-01-02")
			return fmt.Sprintf(`Review my wish list and planned trips.

1. get_user_context, get_goals, get_trips.
2. cash_flow from %s: the monthly left over after paying myself first and all spending, and how steady it is.
3. For each goal in priority order: price, saved, remaining and ETA at the average left over (your calculation where the app has none). Name goals still without a price, and planned trips with spending already booked.
4. get_budget_status: which specific lines would free money for the top goals, and by how much a month.

Answer with a table of goals (goal, remaining, ETA now, ETA with the changes) and at most 3 changes. Ask before save_goal or fund_goals.`, from), nil
		})

	prompt("reconcile_recent", "Reconcile recent", "Do the bank balances match the ledger? Lasting gaps explained, with fixes proposed.", []*mcp.PromptArgument{{Name: "from", Title: "Since", Description: "YYYY-MM-DD, default 30 days ago"}},
		func(a map[string]string) (string, error) {
			from := strings.TrimSpace(a["from"])
			if from == "" {
				from = now().AddDate(0, 0, -30).Format("2006-01-02")
			} else if _, err := time.Parse("2006-01-02", from); err != nil {
				return "", fmt.Errorf("from must be YYYY-MM-DD")
			}
			return fmt.Sprintf(`Check that my accounts reconcile since %s.

1. get_reconciliation from=%s.
2. Accounts with reconciles=true are fine — one line for all of them (a late_booking is a payment the bank booked on another day; name it, nothing to fix). Timing gaps (undone within days) are booking dates, not missing money: count them, don't chase them.
3. For each lasting gap: search_transactions on that account and the likely other side (another own account the same days, a transfer with no destination) and get_bank_inbox, then say what is most likely missing, duplicated or on the wrong account.

Answer with a table (account, dates, amount, likely cause, proposed fix). Only recent history matters; change nothing until I approve each fix.`, from, from), nil
		})
}

// month parses YYYY-MM, or falls back to def's month.
func month(s string, def time.Time) (time.Time, error) {
	if s = strings.TrimSpace(s); s == "" {
		return time.Date(def.Year(), def.Month(), 1, 0, 0, 0, 0, time.UTC), nil
	}
	m, err := time.Parse("2006-01", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("month must be YYYY-MM")
	}
	return m, nil
}

func firstOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func monthBounds(m time.Time) (string, string) {
	return m.Format("2006-01-02"), m.AddDate(0, 1, -1).Format("2006-01-02")
}
