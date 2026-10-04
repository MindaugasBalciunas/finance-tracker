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
	"ft/internal/db"
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
		s.DB.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('last_backup_download',?)`, `"`+db.Now()+`"`)
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
		w.Header().Set("Content-Disposition", `attachment; filename="finance-ai-dataset-`+time.Now().Format("2006-01-02")+`.zip"`)
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
		for _, f := range insights.CashFlow(txs, cats, "month") {
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
	add("context.md", func(w io.Writer) error { _, err := io.WriteString(w, s.AI.Context()); return err })
	add("README.md", func(w io.Writer) error {
		_, err := io.WriteString(w, `# Personal finance dataset

Exported `+time.Now().Format("2006-01-02")+`. All amounts EUR.

- transactions.csv — every transaction since `+book.FirstDate()+`. kind: income | expense | transfer. amount is positive; signed_eur is
  +income / −expense / 0 transfer. category is a two-level id (food.groceries). Transfers with category transfer.invest / pension /
  debt (mortgage principal) / asset build wealth; transfer.internal just moves cash between own accounts. tags = people (kids,
  evelina, kristina), properties (house, apartment), trips (trip:…).
- cashflow_monthly.csv — income, spending (expenses − refunds), essential vs discretionary, saved, savings rate, invested.
- networth_monthly.csv — month-end net worth by group, including house, car and the mortgage (negative). Mortgage balances before
  2026 are reconstructed from payments (source=computed in balances.csv).
- balances.csv — every recorded account value.
- reference.json — accounts, category tree, budgets, investment trades, plan settings.
- context.md — the owner's own brief: who they are and how they want to be advised.
`)
		return err
	})
	if err := z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

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
	var yearTx []ledger.Tx
	for _, t := range txs {
		if t.Date >= from && t.Date <= to {
			yearTx = append(yearTx, t)
		}
	}
	rv.Years = insights.CashFlow(txs, cats, "year")
	for _, f := range rv.Years {
		switch f.Period {
		case year:
			rv.Flow = f
		case fmt.Sprint(atoi(year) - 1):
			rv.Previous = f
		}
	}
	rv.Months = insights.CashFlow(yearTx, cats, "month")
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
