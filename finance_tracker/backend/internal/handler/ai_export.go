package handler

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
)

// The AI dataset export: one ZIP of small per-year CSV files plus a README
// that explains the schema and the modeling quirks an analyst (human or AI)
// must know to read the numbers correctly. CSV per year keeps every file
// well inside a chat upload / context window; the README makes the dataset
// self-describing so it works with any AI, not one particular product.

const aiReadme = `# Personal finance dataset

Everything is EUR, dates are YYYY-MM-DD. The archive is deliberately compact
(≤10 files — the Gemini web app rejects larger ZIPs), and long histories live
in precomputed summaries so no single file is big enough to get silently
truncated by a chat platform.

## Using this archive in a web AI chat (Gemini, ChatGPT, Claude)

Upload the ZIP directly, then paste:

> Read README.md and cfo-context.md first, then act as my CFO-style
> financial advisor using the CSV data. Check the DATA MANIFEST at the
> bottom of the README and confirm you can access every file completely —
> if anything is truncated, say so before analyzing.

## Files

- transactions_recent.csv — every money event of the last 24 months (or of
  the whole selected period, for period-scoped exports)
- transactions_archive.csv — older history (full exports only). Prefer the
  precomputed summaries for long-horizon questions; open the archive only
  for specific old transactions.
- balances.csv — point-in-time snapshots of all account balances
- stocks.csv — the complete brokerage trade ledger. ALWAYS full history,
  even in period-scoped exports: cost basis needs every trade.
- monthly_summary.csv — precomputed income / expenses / invested per month
- category_year_totals.csv — precomputed totals per category, type and year
- assets.csv — physical assets (real estate, vehicles…) with loans
- budgets.csv — the owner's monthly plan (fixed obligations, targets, limits)
- cfo-context.md — the owner's own briefing (framework, standing rules,
  communication style) — READ IT FIRST and advise accordingly (present only
  when the owner has written one)

## Transactions

Columns: date, type, category, amount_eur, comment, labels, debit_account, credit_account.

- type is expense | income | investment.
- category "Transfers" (investment-typed) is money moved between the owner's
  OWN accounts (ATM cash, Revolut top-ups, pension payouts) — NOT spending,
  NOT investing. Exclude it from spending/investment analysis; the
  precomputed summaries already do.
- labels is a comma list of tags that differentiate within/across categories:
  who (kids, evelina), what-for (education, alimony, divorce, loan, fees),
  vendor (maxima, ikea), source (payroll, danske, vipps mobilepay).
- Rows labeled "payroll" are pension contributions deducted from gross salary
  (employer's and own part): real compensation and real investing that never
  crossed a bank account — each has a matching income + investment pair, so
  income and invested stay consistent.
- debit_account / credit_account use the account keys from the glossary
  below; empty means no tracked account was touched.

## Balances

One row per snapshot: total plus per-account columns (glossary below).
r_btc / m_btc hold BTC units; btc_price is the EUR/BTC rate used, and the
totals include BTC at that rate. History notes:

- Values before 2024-01-31 are partially reconstructed from official
  records: seb_pen follows the Sodra state contribution record (to the cent)
  and compounds after contributions were suspended on 2019-06-10; art
  follows the INVL fund's unit ledger; cash grows linearly from a €750
  buffer (Sept 2020) to the first tracked amount. Treat pre-2024 splits as
  well-founded approximations; totals from 2024-01-31 on are hand-tracked.

## Account glossary

| key | account |
|-----|---------|
| swed | Swedbank — main everyday checking account |
| seb | SEB checking account |
| cash | physical cash |
| rev_m | Revolut (owner M) |
| rev_r | Revolut (partner R) |
| swed_etf | Swedbank ETF portfolio |
| rev_stocks | Revolut brokerage portfolio |
| ibkr_stocks | Interactive Brokers portfolio |
| seb_pen | SEB 2nd-pillar pension (contributions suspended 2019-06) |
| art | Artea (INVL) 3rd-pillar pension |
| luminor | Luminor account (dormant) |
| r_btc / m_btc | Bitcoin holdings, in BTC units |

## Suggested analysis

Net-worth trend and composition from balances; savings rate from
monthly_summary (income − expenses vs income); spending structure from
category_year_totals and labels (e.g. groceries vs restaurant within Food);
portfolio activity from stocks; asset equity from assets (value − loan).
`

// ExportAIZip godoc
// @Summary      Export an AI-analysis dataset as a compact ZIP (≤10 files)
// @Description  Recent + archive transaction CSVs, balances, the full trade ledger, precomputed summaries, the CFO context and a README with a data manifest — sized and shaped for web AI chat uploads (Gemini caps ZIPs at 10 files). Optional date_from/date_to scope the data.
// @Tags         export
// @Produce      application/zip
// @Success      200  {string}  string  "ZIP archive"
// @Router       /export/ai.zip [get]
func (h *ExportHandler) ExportAIZip(c *gin.Context) {
	// Optional ?date_from/?date_to scope transactions, balances and the
	// summaries — a this-year or 12-month archive is lighter for chat
	// uploads. Stocks stay full-history regardless (cost basis).
	from, to, err := parseExportRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	transactions, err := h.txSvc.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	transactions = filterTxRange(transactions, from, to)
	balances, err := h.balSvc.List(domain.BalanceFilter{DateFrom: from, DateTo: to}, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	stocks, err := h.stockSvc.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	assets, err := h.assetSvc.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	summary, err := h.txSvc.GetSummary(domain.TransactionFilter{DateFrom: from, DateTo: to})
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	sortChronologically(transactions, balances)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeCSV := func(name string, header []string, rows [][]string) error {
		f, err := zw.Create(name)
		if err != nil {
			return err
		}
		w := csv.NewWriter(f)
		if err := w.Write(header); err != nil {
			return err
		}
		if err := w.WriteAll(rows); err != nil {
			return err
		}
		w.Flush()
		return w.Error()
	}

	// The user's CFO briefing rides along, so an AI session started from
	// this ZIP has the framework, not just the numbers.
	if ctx := h.aiContextContent(); strings.TrimSpace(ctx) != "" {
		if f, err := zw.Create("cfo-context.md"); err == nil {
			_, _ = f.Write([]byte(ctx))
		}
	}

	// Transactions in two tiers: the analytically hot last 24 months, and a
	// cold archive. Web AI chats cap ZIPs at ~10 files (Gemini) and quietly
	// truncate long files — the recent tier stays small enough to be read in
	// full, and the precomputed summaries carry the long horizon.
	manifest := &zipManifest{}
	if from != nil || to != nil {
		human := strings.ReplaceAll(strings.TrimPrefix(rangeSuffix(from, to), "_"), "_", " ")
		manifest.note = fmt.Sprintf("This archive is period-scoped (%s) — export without a period for full history (stocks.csv is always complete).", human)
	}
	cutoff := time.Now().AddDate(-2, 0, 0)
	txRow := func(tx domain.Transaction) []string {
		return []string{
			tx.Date.Format("2006-01-02"), string(tx.Type), string(tx.Category),
			fmt.Sprintf("%.2f", tx.Amount), csvSafe(tx.Comment), csvSafe(tx.Labels),
			tx.DebitAccount, tx.CreditAccount,
		}
	}
	var txRecent, txArchive [][]string
	for _, tx := range transactions {
		if tx.Date.Before(cutoff) {
			txArchive = append(txArchive, txRow(tx))
		} else {
			txRecent = append(txRecent, txRow(tx))
		}
	}
	txHeader := []string{"date", "type", "category", "amount_eur", "comment", "labels", "debit_account", "credit_account"}
	if err := writeCSV("transactions_recent.csv", txHeader, txRecent); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	manifest.add("transactions_recent.csv", txRecent)
	if len(txArchive) > 0 {
		if err := writeCSV("transactions_archive.csv", txHeader, txArchive); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
		manifest.add("transactions_archive.csv", txArchive)
	}

	// Balances, one file (snapshots are few hundred rows at most).
	var balRows [][]string
	for _, b := range balances {
		balRows = append(balRows, []string{
			b.Date.Format("2006-01-02"),
			fmt.Sprintf("%.2f", b.Total), fmt.Sprintf("%.2f", b.Swed), fmt.Sprintf("%.2f", b.Seb),
			fmt.Sprintf("%.2f", b.Cash), fmt.Sprintf("%.2f", b.RevM), fmt.Sprintf("%.2f", b.RevR),
			fmt.Sprintf("%.2f", b.SwedETF), fmt.Sprintf("%.2f", b.RevStocks), fmt.Sprintf("%.2f", b.IBKRStocks),
			fmt.Sprintf("%.2f", b.SebPen), fmt.Sprintf("%.2f", b.Art), fmt.Sprintf("%.2f", b.Luminor),
			fmt.Sprintf("%.8f", b.RBTC), fmt.Sprintf("%.8f", b.MBTC), fmt.Sprintf("%.2f", b.BtcPrice),
		})
	}
	balHeader := []string{
		"date", "total", "swed", "seb", "cash", "rev_m", "rev_r",
		"swed_etf", "rev_stocks", "ibkr_stocks", "seb_pen", "art", "luminor",
		"r_btc", "m_btc", "btc_price",
	}
	if err := writeCSV("balances.csv", balHeader, balRows); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	manifest.add("balances.csv", balRows)

	// Stock trades, one file.
	var stockRows [][]string
	for _, s := range stocks {
		stockRows = append(stockRows, []string{
			s.Date.Format("2006-01-02"), string(s.Action), s.Ticker,
			fmt.Sprintf("%.6f", s.Shares), fmt.Sprintf("%.4f", s.PricePerShare),
			s.Currency, string(s.Source), s.Notes,
		})
	}
	stockHeader := []string{"date", "action", "ticker", "shares", "price_per_share", "currency", "source", "notes"}
	if err := writeCSV("stocks.csv", stockHeader, stockRows); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	manifest.add("stocks.csv", stockRows)

	// Precomputed monthly summary (Transfers already excluded server-side).
	var monthRows [][]string
	for _, m := range summary.ByMonth {
		monthRows = append(monthRows, []string{
			strconv.Itoa(m.Year), fmt.Sprintf("%02d", m.Month),
			fmt.Sprintf("%.2f", m.Income), fmt.Sprintf("%.2f", m.Expenses),
			fmt.Sprintf("%.2f", m.Investments),
			fmt.Sprintf("%.2f", m.Income-m.Expenses-m.Investments),
		})
	}
	if err := writeCSV("monthly_summary.csv",
		[]string{"year", "month", "income_eur", "expenses_eur", "invested_eur", "net_saved_eur"}, monthRows); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	manifest.add("monthly_summary.csv", monthRows)

	// Per-year category totals (Transfers rows carried but flagged by name;
	// the README tells readers to exclude them from spending analysis).
	type catKey struct {
		year     int
		typ      string
		category string
	}
	catTotals := map[catKey]float64{}
	for _, tx := range transactions {
		k := catKey{tx.Date.Year(), string(tx.Type), string(tx.Category)}
		catTotals[k] += tx.Amount
	}
	catKeys := make([]catKey, 0, len(catTotals))
	for k := range catTotals {
		catKeys = append(catKeys, k)
	}
	sort.Slice(catKeys, func(i, j int) bool {
		a, b := catKeys[i], catKeys[j]
		if a.year != b.year {
			return a.year < b.year
		}
		if a.typ != b.typ {
			return a.typ < b.typ
		}
		return a.category < b.category
	})
	var catRows [][]string
	for _, k := range catKeys {
		catRows = append(catRows, []string{
			strconv.Itoa(k.year), k.typ, k.category, fmt.Sprintf("%.2f", catTotals[k]),
		})
	}
	if err := writeCSV("category_year_totals.csv",
		[]string{"year", "type", "category", "total_eur"}, catRows); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	manifest.add("category_year_totals.csv", catRows)

	// Assets and the budget plan for household context.
	var assetRows [][]string
	for _, a := range toAssetExportRows(assets) {
		assetRows = append(assetRows, []string{
			csvSafe(a.Name), a.Type, a.PurchaseDate, fmt.Sprintf("%.2f", a.PurchasePrice),
			fmt.Sprintf("%.2f", a.CurrentValue), a.ValuationDate,
			fmt.Sprintf("%.2f", a.LoanRemaining), a.LoanRate,
			fmt.Sprintf("%.2f", a.LoanMargin), fmt.Sprintf("%.2f", a.LoanBaseRate),
			a.LoanRateResetDate, fmt.Sprintf("%.2f", a.LoanMonthlyPayment), csvSafe(a.Notes),
		})
	}
	if err := writeCSV("assets.csv",
		[]string{"name", "type", "purchase_date", "purchase_price_eur", "current_value_eur", "valuation_date", "loan_remaining_eur", "loan_rate", "loan_margin_pct", "loan_base_rate_pct", "loan_rate_reset_date", "loan_monthly_payment_eur", "notes"}, assetRows); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	budgetRows, _ := h.budgetRows()
	var planRows [][]string
	for _, b := range budgetRows {
		planRows = append(planRows, []string{b.Name, b.Kind, b.Label, b.Category, fmt.Sprintf("%.2f", b.Amount)})
	}
	if err := writeCSV("budgets.csv",
		[]string{"name", "kind", "label", "category", "monthly_amount_eur"}, planRows); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	manifest.add("assets.csv", assetRows)
	manifest.add("budgets.csv", planRows)

	// README last, so it can carry the live data manifest.
	if f, err := zw.Create("README.md"); err == nil {
		_, _ = f.Write([]byte(aiReadme + manifest.render()))
	}

	if err := zw.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	name := fmt.Sprintf("ai_finances_%s%s.zip", time.Now().Format("2006-01-02"), rangeSuffix(from, to))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

// zipManifest records row counts and date coverage per CSV so a model can
// verify it ingested each file completely — web AI chats quietly truncate
// long files, and an unnoticed truncation is worse than an error.
type zipManifest struct {
	note  string
	lines []string
}

func (m *zipManifest) add(name string, rows [][]string) {
	line := fmt.Sprintf("- %s: %d data rows", name, len(rows))
	// First column is the date on the row-level files (already chronological).
	if len(rows) > 0 && len(rows[0]) > 0 && len(rows[0][0]) == 10 && rows[0][0][4] == '-' {
		line += fmt.Sprintf(", %s → %s", rows[0][0], rows[len(rows)-1][0])
	}
	m.lines = append(m.lines, line)
}

func (m *zipManifest) render() string {
	head := ""
	if m.note != "" {
		head = m.note + "\n\n"
	}
	return `

## DATA MANIFEST (integrity check — verify before analyzing)

` + head + strings.Join(m.lines, "\n") + `

If a file's rows you can actually access don't match the counts above, your
platform truncated it — SAY SO before analyzing, and fall back to the
precomputed summaries (monthly_summary, category_year_totals) plus
transactions_recent.csv rather than silently working from partial data.
`
}
