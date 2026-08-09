package handler

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"sort"
	"strconv"
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

Everything is EUR, dates are YYYY-MM-DD. One CSV per year keeps files small
enough for AI chat uploads — concatenate a column's files for full history.

## Files

- transactions_<year>.csv — every money event of that year
- balances_<year>.csv — point-in-time snapshots of all account balances
- stocks_<year>.csv — individual brokerage trades
- monthly_summary.csv — precomputed income / expenses / invested per month
- category_year_totals.csv — precomputed totals per category, type and year
- assets.csv — physical assets (real estate, vehicles…) with loans
- budgets.csv — the owner's monthly plan (fixed obligations, targets, limits)

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
// @Summary      Export an AI-analysis dataset as a ZIP of per-year CSVs
// @Description  Small per-year CSV files (transactions, balances, stocks) plus precomputed summaries and a README describing the schema — sized for AI chat uploads.
// @Tags         export
// @Produce      application/zip
// @Success      200  {string}  string  "ZIP archive"
// @Router       /export/ai.zip [get]
func (h *ExportHandler) ExportAIZip(c *gin.Context) {
	transactions, err := h.txSvc.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	balances, err := h.balSvc.List(domain.BalanceFilter{}, 0)
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
	summary, err := h.txSvc.GetSummary(domain.TransactionFilter{})
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

	if f, err := zw.Create("README.md"); err == nil {
		_, _ = f.Write([]byte(aiReadme))
	}

	// Transactions, one file per year.
	txByYear := map[int][][]string{}
	for _, tx := range transactions {
		y := tx.Date.Year()
		txByYear[y] = append(txByYear[y], []string{
			tx.Date.Format("2006-01-02"), string(tx.Type), string(tx.Category),
			fmt.Sprintf("%.2f", tx.Amount), tx.Comment, tx.Labels,
			tx.DebitAccount, tx.CreditAccount,
		})
	}
	txHeader := []string{"date", "type", "category", "amount_eur", "comment", "labels", "debit_account", "credit_account"}
	for _, y := range sortedYears(txByYear) {
		if err := writeCSV(fmt.Sprintf("transactions_%d.csv", y), txHeader, txByYear[y]); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
	}

	// Balances, one file per year.
	balByYear := map[int][][]string{}
	for _, b := range balances {
		y := b.Date.Year()
		balByYear[y] = append(balByYear[y], []string{
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
	for _, y := range sortedYears(balByYear) {
		if err := writeCSV(fmt.Sprintf("balances_%d.csv", y), balHeader, balByYear[y]); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
	}

	// Stock trades, one file per year.
	stockByYear := map[int][][]string{}
	for _, s := range stocks {
		y := s.Date.Year()
		stockByYear[y] = append(stockByYear[y], []string{
			s.Date.Format("2006-01-02"), string(s.Action), s.Ticker,
			fmt.Sprintf("%.6f", s.Shares), fmt.Sprintf("%.4f", s.PricePerShare),
			s.Currency, string(s.Source), s.Notes,
		})
	}
	stockHeader := []string{"date", "action", "ticker", "shares", "price_per_share", "currency", "source", "notes"}
	for _, y := range sortedYears(stockByYear) {
		if err := writeCSV(fmt.Sprintf("stocks_%d.csv", y), stockHeader, stockByYear[y]); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
	}

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

	// Assets and the budget plan for household context.
	var assetRows [][]string
	for _, a := range toAssetExportRows(assets) {
		assetRows = append(assetRows, []string{
			a.Name, a.Type, a.PurchaseDate, fmt.Sprintf("%.2f", a.PurchasePrice),
			fmt.Sprintf("%.2f", a.CurrentValue), a.ValuationDate,
			fmt.Sprintf("%.2f", a.LoanRemaining), a.LoanRate, a.Notes,
		})
	}
	if err := writeCSV("assets.csv",
		[]string{"name", "type", "purchase_date", "purchase_price_eur", "current_value_eur", "valuation_date", "loan_remaining_eur", "loan_rate", "notes"}, assetRows); err != nil {
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

	if err := zw.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	name := fmt.Sprintf("ai_finances_%s.zip", time.Now().Format("2006-01-02"))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

func sortedYears[T any](m map[int]T) []int {
	years := make([]int, 0, len(m))
	for y := range m {
		years = append(years, y)
	}
	sort.Ints(years)
	return years
}
