package api

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ft/internal/backup"
	"ft/internal/cfo"
	"ft/internal/db"
	"ft/internal/goals"
	"ft/internal/importv1"
	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

func (s *Server) dataRoutes() {
	s.handle("GET /api/export/backup.json", func(w http.ResponseWriter, r *http.Request) (any, error) {
		secrets := r.URL.Query().Get("secrets") == "1"
		f, err := backup.Export(s.DB, secrets)
		if err != nil {
			return nil, err
		}
		name := "finance-backup-" + time.Now().Format("2006-01-02")
		if secrets {
			name += "-with-secrets"
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.json"`)
		json.NewEncoder(w).Encode(f)
		// ?purpose=export (syncs, tooling) reads without counting as the
		// owner's off-device backup.
		if r.URL.Query().Get("purpose") != "export" {
			s.DB.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('last_backup_download',?)`, `"`+db.Now()+`"`)
		}
		return nil, nil
	})
	s.handle("POST /api/import/backup", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if r.URL.Query().Get("confirm") != "replace" {
			return nil, bad("restoring replaces all data — confirm with ?confirm=replace")
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 200<<20))
		if err != nil {
			return nil, err
		}
		if err := db.Backup(s.DB, s.DBPath, "pre-restore"); err != nil && s.DBPath != "" {
			return nil, fmt.Errorf("could not snapshot before restoring: %w", err)
		}
		counts, err := backup.Restore(s.DB, raw)
		return map[string]any{"restored": counts}, err
	})
	s.handle("POST /api/import/v1", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if r.URL.Query().Get("confirm") != "replace" {
			return nil, bad("importing a v1 backup replaces all data — confirm with ?confirm=replace")
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 200<<20))
		if err != nil {
			return nil, err
		}
		src, err := importv1.ReadJSON(raw)
		if err != nil {
			return nil, err
		}
		if s.DBPath != "" {
			if err := db.Backup(s.DB, s.DBPath, "pre-restore"); err != nil {
				return nil, err
			}
		}
		if err := clearData(s); err != nil {
			return nil, err
		}
		return importv1.Convert(s.DB, src)
	})
	s.handle("GET /api/export/transactions.csv", func(w http.ResponseWriter, r *http.Request) (any, error) {
		f := filterFrom(r)
		txs, err := ledger.All(s.DB, f)
		if err != nil {
			return nil, err
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="transactions.csv"`)
		writeTxCSV(w, txs)
		return nil, nil
	})
	s.handle("GET /api/export/ai.zip", func(w http.ResponseWriter, r *http.Request) (any, error) {
		buf, err := s.aiZip()
		if err != nil {
			return nil, err
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="finance-for-ai-`+time.Now().Format("2006-01-02")+`.zip"`)
		w.Write(buf)
		return nil, nil
	})
	s.handle("GET /api/backups", func(w http.ResponseWriter, r *http.Request) (any, error) {
		entries, _ := os.ReadDir(db.BackupDir(s.DBPath))
		type b struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
			At   string `json:"at"`
		}
		out := []b{}
		for _, e := range entries {
			if info, err := e.Info(); err == nil && strings.HasSuffix(e.Name(), ".db") {
				out = append(out, b{e.Name(), info.Size(), info.ModTime().UTC().Format(time.RFC3339)})
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].At > out[j].At })
		var last string
		s.DB.QueryRow(`SELECT value FROM settings WHERE key='last_backup_download'`).Scan(&last)
		return map[string]any{"snapshots": out, "last_download": strings.Trim(last, `"`)}, nil
	})
	s.handle("POST /api/backups", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]bool{"ok": true}, db.Backup(s.DB, s.DBPath, "manual")
	})
	s.handle("GET /api/backups/{name}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		name := filepath.Base(r.PathValue("name"))
		if !strings.HasSuffix(name, ".db") {
			return nil, bad("invalid name")
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		http.ServeFile(w, r, filepath.Join(db.BackupDir(s.DBPath), name))
		return nil, nil
	})
}

// clearData empties the user data tables (keeps secrets/auth/bank links).
func clearData(s *Server) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range []string{"budget_amounts", "budgets", "rules", "trades", "balances", "transactions", "categories", "accounts", "ai_messages"} {
		if _, err := tx.Exec(`DELETE FROM ` + t); err != nil {
			return err
		}
	}
	tx.Exec(`UPDATE bank_inbox SET matched_tx_id=NULL, imported_tx_id=NULL`)
	tx.Exec(`DELETE FROM settings WHERE key IN ('plan','ai_context','migrated_from_v1')`)
	return tx.Commit()
}

func writeTxCSV(w io.Writer, txs []ledger.Tx) {
	cw := csv.NewWriter(w)
	cw.Write([]string{"id", "date", "kind", "amount_eur", "signed_eur", "category", "merchant", "note", "tags", "account", "to_account", "source"})
	for _, t := range txs {
		cw.Write([]string{fmt.Sprint(t.ID), t.Date, t.Kind, t.Amount.String(), t.Signed().String(), t.Category, t.Merchant, t.Note,
			strings.Join(t.Tags, ";"), t.AccountID, t.ToAccountID, t.Source})
	}
	cw.Flush()
}

// aiZip is a self-describing dataset to hand to Claude.ai or another tool.
func (s *Server) aiZip() ([]byte, error) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	add := func(name string, write func(io.Writer) error) error {
		f, err := z.Create(name)
		if err != nil {
			return err
		}
		return write(f)
	}
	txs, err := ledger.All(s.DB, ledger.Filter{})
	if err != nil {
		return nil, err
	}
	cats, _ := ledger.CategoryMap(s.DB)
	book, err := wealth.LoadBook(s.DB)
	if err != nil {
		return nil, err
	}
	add("transactions.csv", func(w io.Writer) error { writeTxCSV(w, txs); return nil })
	add("cashflow_monthly.csv", func(w io.Writer) error {
		cw := csv.NewWriter(w)
		cw.Write([]string{"month", "income", "spending", "essential", "discretionary", "saved", "savings_rate", "invested", "mortgage_principal"})
		for _, f := range insights.CashFlow(txs, cats, "month", plan.LoadSettings(s.DB).Salary()) {
			cw.Write([]string{f.Period, f.Income.String(), f.Spending.String(), f.Essential.String(), f.Discretionary.String(), f.Saved.String(),
				fmt.Sprintf("%.3f", f.SavingsRate), f.Invested.String(), f.Principal.String()})
		}
		cw.Flush()
		return nil
	})
	add("networth_monthly.csv", func(w io.Writer) error {
		cw := csv.NewWriter(w)
		groups := []string{"cash", "investments", "pension", "crypto", "real_assets", "debt"}
		cw.Write(append([]string{"month_end", "net_worth", "liquid"}, groups...))
		for _, h := range book.History("", "", "month") {
			row := []string{h.Date, h.NetWorth.String(), h.Liquid.String()}
			for _, g := range groups {
				row = append(row, h.ByGroup[g].String())
			}
			cw.Write(row)
		}
		cw.Flush()
		return nil
	})
	add("balances.csv", func(w io.Writer) error {
		cw := csv.NewWriter(w)
		cw.Write([]string{"account", "date", "value_eur", "quantity", "source"})
		ids := make([]string, 0, len(book.Accounts))
		for id := range book.Accounts {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			for _, p := range book.Series(id) {
				q := ""
				if p.Quantity != nil {
					q = fmt.Sprint(*p.Quantity)
				}
				cw.Write([]string{id, p.Date, p.Value.String(), q, p.Source})
			}
		}
		cw.Flush()
		return nil
	})
	add("reference.json", func(w io.Writer) error {
		accts, _ := ledger.ListAccounts(s.DB)
		cl, _ := ledger.ListCategories(s.DB)
		budgets, _ := plan.List(s.DB, false)
		trades, _ := wealth.ListTrades(s.DB)
		return json.NewEncoder(w).Encode(map[string]any{"accounts": accts, "categories": cl, "budgets": budgets, "trades": trades,
			"plan_settings": plan.LoadSettings(s.DB)})
	})
	add("loans.csv", func(w io.Writer) error {
		cw := csv.NewWriter(w)
		cw.Write([]string{"account", "name", "lender", "owed_eur", "as_of", "rate_pct", "base_rate_name", "base_rate_pct", "margin_pct", "rate_reset_date",
			"monthly_payment_eur", "months_left", "payoff", "interest_left_eur", "secured_on", "asset_value_eur", "ltv"})
		for _, l := range wealth.Loans(book, txs, time.Now()) {
			d := l.Details
			cw.Write([]string{l.AccountID, l.Name, d.Lender, l.Balance.String(), l.BalanceDate, fmt.Sprintf("%.2f", l.Rate), d.BaseRateName,
				fmt.Sprintf("%.3f", d.BaseRate), fmt.Sprintf("%.2f", d.Margin), d.RateResetDate, fmt.Sprintf("%.2f", (l.NextInterest + l.NextPrincipal).Float()),
				fmt.Sprint(l.MonthsLeft), l.PayoffDate, l.TotalInterest.String(), d.AssetID, l.AssetValue.String(), fmt.Sprintf("%.3f", l.LTV)})
		}
		cw.Flush()
		return nil
	})
	// Nine files: chat assistants such as Gemini take at most ten per
	// prompt and don't open zips, so the archive is uploaded unpacked.
	jsonFile := func(name string, v any) {
		add(name, func(w io.Writer) error {
			e := json.NewEncoder(w)
			e.SetIndent("", " ")
			return e.Encode(v)
		})
	}
	now := time.Now()
	today := map[string]any{}
	if o, err := cfo.BuildOverview(s.DB, now, s.Bank.OpenCount()); err == nil {
		today["overview"] = o
	} else {
		return nil, err
	}
	if rep, err := cfo.BudgetReport(s.DB, now.Format("2006-01"), now); err == nil {
		today["budget_this_month"] = rep
	}
	if rec, _, err := insights.RecurringAll(s.DB, txs, now); err == nil {
		today["recurring"] = rec
	}
	jsonFile("today.json", today)
	gt := map[string]any{}
	if wish, err := goals.Build(s.DB, "", now); err == nil {
		moves := map[int64][]goals.Move{}
		for _, g := range wish.Goals {
			moves[g.ID], _ = goals.Moves(s.DB, g.ID)
		}
		gt["wishlist"], gt["goal_moves"] = wish, moves
	}
	budgets, _ := plan.List(s.DB, false)
	trips, sugg := plan.Trips(txs, budgets)
	gt["trips"], gt["untagged_trip_suggestions"], gt["planned_trips"] = trips, sugg, plannedTrips(s)
	jsonFile("goals_and_trips.json", gt)
	add("PROMPT.md", func(w io.Writer) error {
		var b strings.Builder
		b.WriteString(analysisPrompt)
		b.WriteString("\n# The files\n\nExported " + now.Format("2006-01-02") + ". All amounts EUR.\n\n")
		b.WriteString(strings.ReplaceAll(exportGuide, "{first}", book.FirstDate()))
		if c := strings.TrimSpace(s.AI.Context()); c != "" {
			b.WriteString("\n# My brief — who I am and how to advise me\n\n" + c + "\n")
		}
		if n := strings.TrimSpace(s.AI.Notes()); n != "" {
			b.WriteString("\n# Decisions I've made since (dated, newest last)\n\n" + n + "\n")
		}
		_, err := io.WriteString(w, b.String())
		return err
	})
	if err := z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// exportGuide describes each file of the AI export.
const exportGuide = `- transactions.csv — every transaction since {first}. kind: income | expense | transfer. amount is positive; signed_eur is
  +income / −expense / 0 transfer. category is a two-level id (food.groceries). Transfers with category transfer.invest / pension /
  debt (mortgage principal) / asset build wealth; transfer.internal just moves cash between own accounts. tags = people (kids,
  evelina, kristina), properties (house, apartment), trips (trip:…).
- cashflow_monthly.csv — income, spending (expenses − refunds), essential vs discretionary, saved, savings rate, invested.
- networth_monthly.csv — month-end net worth by group, including house, car and the mortgage (negative). Mortgage balances before
  2026 are reconstructed from payments (source=computed in balances.csv).
- balances.csv — every recorded account value.
- loans.csv — each loan: balance, rate (base + margin), reset date, payment, payoff, interest left, LTV.
- reference.json — accounts, category tree, budgets, investment trades, plan settings.
- today.json — overview: net worth, liquid (cash, brokers, crypto, II/III pillar pensions), this month vs typical, cash plan,
  emergency fund; budget_this_month: each budget line spent vs budgeted, funds, yearly lines; recurring: bills, subscriptions
  and standing orders the app knows (amount, cadence, next due, last paid).
- goals_and_trips.json — wishlist: goals to save for in priority order (target, saved, remaining, ETA; target 0 = no price yet),
  last finished month's funding (income − spending − invested = left over for goals), the suggested split and history;
  goal_moves: money put in or taken out per goal. My strategy: pay myself first (investing, pensions, principal), then
  obligations and spending; what is left at month end funds goals top-down. trips: past trips (trip:* tags) with totals and
  per-day cost; untagged_trip_suggestions: travel spending that looks like a trip; planned_trips: wish-list goals with a trip
  tag (spent = tagged spending already booked).
`

// analysisPrompt tells an outside AI assistant how to use the dataset.
// Shared via Settings → "Export for AI".
const analysisPrompt = `# Start here

You are my personal CFO. The files attached are my complete finances (EUR, Lithuania). Below this list are the file
formats, my brief and the decisions I've made since. Answer from the data — quote numbers, months and merchants, don't guess.

1. **Where I stand** — net worth and liquid assets now, change over 12 months and why (today.json, networth_monthly.csv).
2. **Cash flow** — savings rate for the last 12 months vs the year before; months that broke the pattern and the cause
   (cashflow_monthly.csv, transactions.csv). A salary booked on the 1st–3rd belongs to the previous month.
3. **Spending** — the 5 categories and 10 merchants that grew most; recurring charges I could cut.
4. **Debt** — mortgage: rate, reset date, prepay vs invest at my rate, and what happens if EURIBOR moves ±1% (loans.csv).
5. **Safety** — emergency fund in months of essential spending; anything stale or risky.
6. **Goals and trips** — is the wish list (goals_and_trips.json) realistic at my left-over rate? Which goal or planned
   trip to fund first, and what would bring the dates closer.
7. **Next 3 actions** — concrete, with euro amounts and dates.

Rules: mortgage principal (transfer.debt) is saving, not spending; transfer.internal is neutral; refunds reduce spending.
Keep it short — tables over prose.
`

// Review is a year in review.
type Review struct {
	Year          string                  `json:"year"`
	Flow          insights.Flow           `json:"flow"`
	Previous      insights.Flow           `json:"previous"`
	Months        []insights.Flow         `json:"months"`
	Categories    []insights.CategoryStat `json:"categories"`
	Merchants     []insights.NamedCount   `json:"merchants"`
	Largest       []ledger.Tx             `json:"largest"`
	NetWorthStart money.Cents             `json:"net_worth_start"`
	NetWorthEnd   money.Cents             `json:"net_worth_end"`
	Years         []insights.Flow         `json:"years"` // every year, for the long view
}

func (s *Server) yearReview(year string) (*Review, error) {
	txs, err := ledger.All(s.DB, ledger.Filter{})
	if err != nil {
		return nil, err
	}
	cats, _ := ledger.CategoryMap(s.DB)
	from, to := year+"-01-01", year+"-12-31"
	if t := today(); to > t {
		to = t
	}
	rv := &Review{Year: year}
	salary := plan.LoadSettings(s.DB).Salary()
	var yearTx []ledger.Tx
	for i := range txs {
		if d := plan.FlowDate(&txs[i], salary); d >= from && d <= to {
			yearTx = append(yearTx, txs[i])
		}
	}
	rv.Years = insights.CashFlow(txs, cats, "year", salary)
	for _, f := range rv.Years {
		switch f.Period {
		case year:
			rv.Flow = f
		case fmt.Sprint(atoi(year) - 1):
			rv.Previous = f
		}
	}
	rv.Months = insights.CashFlow(yearTx, cats, "month", salary)
	rv.Categories = insights.Breakdown(txs, from, to)
	rv.Merchants = insights.MerchantTotals(txs, from, to, 15)
	rv.Largest = insights.TopExpenses(txs, from, to, 10)
	book, err := wealth.LoadBook(s.DB)
	if err != nil {
		return nil, err
	}
	rv.NetWorthStart = book.SnapshotAt(fmt.Sprintf("%d-12-31", atoi(year)-1), false).NetWorth
	rv.NetWorthEnd = book.SnapshotAt(to, false).NetWorth
	return rv, nil
}

func atoi(s string) int {
	n := 0
	fmt.Sscan(s, &n)
	return n
}
