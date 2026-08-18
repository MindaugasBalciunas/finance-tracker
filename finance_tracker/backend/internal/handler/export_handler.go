package handler

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/mindaugas/finance-tracker/internal/service"
)

// sortChronologically orders export rows oldest-first (date, then ID) so the
// JSON reads as a timeline regardless of the repositories' listing order.
func sortChronologically(transactions []domain.Transaction, balances []domain.Balance) {
	sort.SliceStable(transactions, func(i, j int) bool {
		if !transactions[i].Date.Equal(transactions[j].Date) {
			return transactions[i].Date.Before(transactions[j].Date)
		}
		return transactions[i].ID < transactions[j].ID
	})
	sort.SliceStable(balances, func(i, j int) bool {
		if !balances[i].Date.Equal(balances[j].Date) {
			return balances[i].Date.Before(balances[j].Date)
		}
		return balances[i].ID < balances[j].ID
	})
}

type ExportHandler struct {
	txSvc         service.TransactionService
	balSvc        service.BalanceService
	stockSvc      service.StockService
	assetSvc      service.AssetService
	exportLogRepo repository.ExportLogRepository
	budgetRepo    repository.BudgetRepository
	insightRepo   repository.InsightRepository // optional: AI settings in backups
}

// budgetRows exports the budget plan and label rules (nil-safe).
func (h *ExportHandler) budgetRows() ([]budgetExportRow, []labelRuleExportRow) {
	if h.budgetRepo == nil {
		return nil, nil
	}
	var bRows []budgetExportRow
	if budgets, err := h.budgetRepo.ListBudgets(); err == nil {
		for _, b := range budgets {
			bRows = append(bRows, budgetExportRow{Name: b.Name, Kind: b.Kind, Label: b.Label, Category: b.Category, Amount: b.Amount})
		}
	}
	var rRows []labelRuleExportRow
	if rules, err := h.budgetRepo.ListRules(); err == nil {
		for _, r := range rules {
			rRows = append(rRows, labelRuleExportRow{Label: r.Label, Category: r.Category, CommentMatch: r.CommentMatch})
		}
	}
	return bRows, rRows
}

func (h *ExportHandler) settingsRow() *budgetSettingsRow {
	if h.budgetRepo == nil {
		return nil
	}
	s, err := h.budgetRepo.GetSettings()
	if err != nil || s.IncomeMode == "median" && s.ManualIncome == 0 && s.GrossSalary == 0 {
		return nil
	}
	return &budgetSettingsRow{IncomeMode: s.IncomeMode, ManualIncome: s.ManualIncome, GrossSalary: s.GrossSalary, MonthlyDeductions: s.MonthlyDeductions}
}

func NewExportHandler(txSvc service.TransactionService, balSvc service.BalanceService, stockSvc service.StockService, assetSvc service.AssetService, exportLogRepo repository.ExportLogRepository) *ExportHandler {
	return &ExportHandler{txSvc: txSvc, balSvc: balSvc, stockSvc: stockSvc, assetSvc: assetSvc, exportLogRepo: exportLogRepo}
}

// WithBudgets includes budgets and label rules in JSON exports.
func (h *ExportHandler) WithBudgets(repo repository.BudgetRepository) *ExportHandler {
	h.budgetRepo = repo
	return h
}

// WithAI includes the AI gateway settings in JSON backups.
func (h *ExportHandler) WithAI(repo repository.InsightRepository) *ExportHandler {
	h.insightRepo = repo
	return h
}

// aiContextContent returns the CFO-context document ("" when unset).
func (h *ExportHandler) aiContextContent() string {
	if h.insightRepo == nil {
		return ""
	}
	c, err := h.insightRepo.GetAIContext()
	if err != nil {
		return ""
	}
	return c.Content
}

func (h *ExportHandler) aiSettingsRow() *aiSettingsExportRow {
	if h.insightRepo == nil {
		return nil
	}
	s, err := h.insightRepo.GetAISettings()
	if err != nil || (s.APIKey == "" && s.Model == "") {
		return nil
	}
	return &aiSettingsExportRow{GatewayURL: s.GatewayURL, Model: s.Model, APIKey: s.APIKey}
}

func (h *ExportHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/export")
	g.GET("/transactions.csv", h.ExportTransactions)
	g.GET("/balances.csv", h.ExportBalances)
	g.GET("/ai.zip", h.ExportAIZip)
	g.GET("/finances.json", h.ExportAllJSON)
	g.GET("/finances-partial.json", h.ExportPartialJSON)
	g.GET("/status", h.ExportStatus)
}

// ExportTransactions godoc
// @Summary      Export transactions as CSV
// @Tags         export
// @Produce      text/csv
// @Success      200  {string}  string  "CSV file"
// @Router       /export/transactions.csv [get]
func (h *ExportHandler) ExportTransactions(c *gin.Context) {
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

	ts := time.Now().Format("2006-01-02")
	name := fmt.Sprintf("export_transactions_%s%s.csv", ts, rangeSuffix(from, to))
	if from == nil && to == nil {
		name = fmt.Sprintf("export_transactions_all_%s.csv", ts)
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", name))

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"id", "date", "type", "amount", "category", "comment", "labels"})
	for _, tx := range transactions {
		_ = w.Write([]string{
			strconv.FormatUint(uint64(tx.ID), 10),
			tx.Date.Format("2006-01-02"),
			string(tx.Type),
			fmt.Sprintf("%.2f", tx.Amount),
			string(tx.Category),
			csvSafe(tx.Comment),
			csvSafe(tx.Labels),
		})
	}
	w.Flush()
}

// csvSafe neutralizes spreadsheet formula injection: a free-text field
// starting with = + - @ (or tab/CR) executes as a formula when the CSV opens
// in Excel/Sheets — and comments arrive from imported bank statements, so
// they are attacker-reachable. A leading apostrophe marks the cell as
// literal text. Only applied to free-text fields, never numeric ones.
func csvSafe(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

// ExportBalances godoc
// @Summary      Export balance snapshots as CSV
// @Tags         export
// @Produce      text/csv
// @Success      200  {string}  string  "CSV file"
// @Router       /export/balances.csv [get]
func (h *ExportHandler) ExportBalances(c *gin.Context) {
	from, to, err := parseExportRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	balances, err := h.balSvc.List(domain.BalanceFilter{DateFrom: from, DateTo: to}, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	ts := time.Now().Format("2006-01-02")
	name := fmt.Sprintf("export_balances_%s%s.csv", ts, rangeSuffix(from, to))
	if from == nil && to == nil {
		name = fmt.Sprintf("export_balances_all_%s.csv", ts)
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", name))

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{
		"id", "date", "total", "seb", "swed", "swed_etf", "seb_pen",
		"luminor", "art", "cash", "rev_m", "rev_r", "rbtc", "mbtc", "btc_price", "rev_stocks", "ibkr_stocks",
	})
	for _, b := range balances {
		_ = w.Write([]string{
			strconv.FormatUint(uint64(b.ID), 10),
			b.Date.Format("2006-01-02"),
			fmt.Sprintf("%.2f", b.Total),
			fmt.Sprintf("%.2f", b.Seb),
			fmt.Sprintf("%.2f", b.Swed),
			fmt.Sprintf("%.2f", b.SwedETF),
			fmt.Sprintf("%.2f", b.SebPen),
			fmt.Sprintf("%.2f", b.Luminor),
			fmt.Sprintf("%.2f", b.Art),
			fmt.Sprintf("%.2f", b.Cash),
			fmt.Sprintf("%.2f", b.RevM),
			fmt.Sprintf("%.2f", b.RevR),
			fmt.Sprintf("%.8f", b.RBTC),
			fmt.Sprintf("%.8f", b.MBTC),
			fmt.Sprintf("%.2f", b.BtcPrice),
			fmt.Sprintf("%.2f", b.RevStocks),
			fmt.Sprintf("%.2f", b.IBKRStocks),
		})
	}
	w.Flush()
}

// exportSchemaVersion tracks the finances.json format. Importers accept any
// older version (fields only ever get added, never renamed or removed):
//
//	v1 — transactions/balances/stock_trades/assets (+budgets/label_rules/budget_settings)
//	v2 — schema_version field, balance rows carry id + time (multi-snapshot days)
//	v3 — ai_settings (gateway URL, model and API key travel with the backup,
//	     so a wipe-and-restore doesn't lose the AI configuration)
//	v4 — ai_context (the user's CFO briefing travels with the backup)
const exportSchemaVersion = 4

type financeExport struct {
	SchemaVersion   int                  `json:"schema_version,omitempty"`
	ExportDate      string               `json:"export_date"`
	SuggestedPrompt string               `json:"suggested_prompt"`
	Transactions    []txExportRow        `json:"transactions"`
	Balances        []balExportRow       `json:"balances"`
	StockTrades     []stockExportRow     `json:"stock_trades"`
	Assets          []assetExportRow     `json:"assets"`
	Budgets         []budgetExportRow    `json:"budgets,omitempty"`
	LabelRules      []labelRuleExportRow `json:"label_rules,omitempty"`
	BudgetSettings  *budgetSettingsRow   `json:"budget_settings,omitempty"`
	AISettings      *aiSettingsExportRow `json:"ai_settings,omitempty"`
	// AIContext is the user's CFO-briefing document (v4 backups).
	AIContext string `json:"ai_context,omitempty"`
}

// aiSettingsExportRow carries the gateway configuration INCLUDING the API
// key — the backup is the user's own file and restoring it must bring the
// AI features back without re-entering the key.
type aiSettingsExportRow struct {
	GatewayURL string `json:"gateway_url,omitempty"`
	Model      string `json:"model,omitempty"`
	APIKey     string `json:"api_key,omitempty"`
}

type budgetSettingsRow struct {
	IncomeMode        string  `json:"income_mode"`
	ManualIncome      float64 `json:"manual_income,omitempty"`
	GrossSalary       float64 `json:"gross_salary,omitempty"`
	MonthlyDeductions float64 `json:"monthly_deductions,omitempty"`
}

type budgetExportRow struct {
	Name     string  `json:"name"`
	Kind     string  `json:"kind"`
	Label    string  `json:"label,omitempty"`
	Category string  `json:"category,omitempty"`
	Amount   float64 `json:"amount"`
}

type labelRuleExportRow struct {
	Label        string `json:"label"`
	Category     string `json:"category,omitempty"`
	CommentMatch string `json:"comment_match,omitempty"`
}

type txExportRow struct {
	ID            uint    `json:"id"`
	Date          string  `json:"date"`
	Type          string  `json:"type"`
	Amount        float64 `json:"amount_eur"`
	Category      string  `json:"category"`
	Comment       string  `json:"comment,omitempty"`
	Labels        string  `json:"labels,omitempty"`
	DebitAccount  string  `json:"debit_account,omitempty"`
	CreditAccount string  `json:"credit_account,omitempty"`
	SourceAccount string  `json:"source_account,omitempty"`
}

type balExportRow struct {
	ID         uint    `json:"id,omitempty"`
	Date       string  `json:"date"`
	Time       string  `json:"time,omitempty"`
	Total      float64 `json:"total_eur"`
	Seb        float64 `json:"seb,omitempty"`
	Swed       float64 `json:"swed,omitempty"`
	SwedETF    float64 `json:"swed_etf,omitempty"`
	SebPen     float64 `json:"seb_pension,omitempty"`
	Luminor    float64 `json:"luminor,omitempty"`
	Art        float64 `json:"art,omitempty"`
	Cash       float64 `json:"cash,omitempty"`
	RevM       float64 `json:"revolut_m,omitempty"`
	RevR       float64 `json:"revolut_r,omitempty"`
	RBTC       float64 `json:"btc_r,omitempty"`
	MBTC       float64 `json:"btc_m,omitempty"`
	BtcPrice   float64 `json:"btc_price_eur,omitempty"`
	RevStocks  float64 `json:"revolut_stocks,omitempty"`
	IBKRStocks float64 `json:"ibkr_stocks,omitempty"`
}

type stockExportRow struct {
	Date          string  `json:"date"`
	Action        string  `json:"action"`
	Ticker        string  `json:"ticker"`
	Shares        float64 `json:"shares"`
	PricePerShare float64 `json:"price_per_share"`
	Currency      string  `json:"currency"`
	Source        string  `json:"source,omitempty"`
	Notes         string  `json:"notes,omitempty"`
}

type assetExportRow struct {
	Name              string  `json:"name"`
	Type              string  `json:"type"`
	PurchaseDate      string  `json:"purchase_date,omitempty"`
	PurchasePrice     float64 `json:"purchase_price_eur"`
	CurrentValue      float64 `json:"current_value_eur"`
	ValuationDate     string  `json:"valuation_date,omitempty"`
	Notes             string  `json:"notes,omitempty"`
	LoanRemaining     float64 `json:"loan_remaining_eur,omitempty"`
	LoanRemainingDate string  `json:"loan_remaining_date,omitempty"`
	LoanRate          string  `json:"loan_rate,omitempty"`
	LoanAccount       string  `json:"loan_account,omitempty"`
	LoanPaidOffDate   string  `json:"loan_paid_off_date,omitempty"`

	LoanMargin         float64 `json:"loan_margin,omitempty"`
	LoanLabel          string  `json:"loan_label,omitempty"`
	LoanBaseRate       float64 `json:"loan_base_rate,omitempty"`
	LoanRateResetDate  string  `json:"loan_rate_reset_date,omitempty"`
	LoanMonthlyPayment float64 `json:"loan_monthly_payment_eur,omitempty"`
}

// formatOptionalDate renders a nullable date as YYYY-MM-DD or "".
func formatOptionalDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

func toTxExportRows(transactions []domain.Transaction) []txExportRow {
	rows := make([]txExportRow, len(transactions))
	for i, tx := range transactions {
		rows[i] = txExportRow{
			ID:            tx.ID,
			Date:          tx.Date.Format("2006-01-02"),
			Type:          string(tx.Type),
			Amount:        tx.Amount,
			Category:      string(tx.Category),
			Comment:       tx.Comment,
			Labels:        tx.Labels,
			DebitAccount:  tx.DebitAccount,
			CreditAccount: tx.CreditAccount,
			SourceAccount: tx.SourceAccount,
		}
	}
	return rows
}

func toBalExportRows(balances []domain.Balance) []balExportRow {
	rows := make([]balExportRow, len(balances))
	for i, b := range balances {
		rows[i] = balExportRow{
			ID:         b.ID,
			Date:       b.Date.Format("2006-01-02"),
			Total:      b.Total,
			Seb:        b.Seb,
			Swed:       b.Swed,
			SwedETF:    b.SwedETF,
			SebPen:     b.SebPen,
			Luminor:    b.Luminor,
			Art:        b.Art,
			Cash:       b.Cash,
			RevM:       b.RevM,
			RevR:       b.RevR,
			RBTC:       b.RBTC,
			MBTC:       b.MBTC,
			BtcPrice:   b.BtcPrice,
			RevStocks:  b.RevStocks,
			IBKRStocks: b.IBKRStocks,
		}
		// Time-of-day distinguishes multiple snapshots on the same date; only
		// written when non-midnight so v1-era daily snapshots stay unchanged.
		if hhmmss := b.Date.Format("15:04:05"); hhmmss != "00:00:00" {
			rows[i].Time = hhmmss
		}
	}
	return rows
}

func toStockExportRows(stocks []domain.StockTrade) []stockExportRow {
	rows := make([]stockExportRow, len(stocks))
	for i, s := range stocks {
		rows[i] = stockExportRow{
			Date:          s.Date.Format("2006-01-02"),
			Action:        string(s.Action),
			Ticker:        s.Ticker,
			Shares:        s.Shares,
			PricePerShare: s.PricePerShare,
			Currency:      s.Currency,
			Source:        string(s.Source),
			Notes:         s.Notes,
		}
	}
	return rows
}

// parseExportRange reads optional ?from=YYYY-MM-DD&to=YYYY-MM-DD query params.
// Both bounds are inclusive; 'to' covers the entire named day (the returned
// bound is that day's final nanosecond, so it composes with the repositories'
// `date <= ?` filters without dragging in the next day's midnight snapshots).
func parseExportRange(c *gin.Context) (from, to *time.Time, err error) {
	if s := c.Query("from"); s != "" {
		d, perr := time.Parse("2006-01-02", s)
		if perr != nil {
			return nil, nil, fmt.Errorf("invalid 'from' date %q, expected YYYY-MM-DD", s)
		}
		from = &d
	}
	if s := c.Query("to"); s != "" {
		d, perr := time.Parse("2006-01-02", s)
		if perr != nil {
			return nil, nil, fmt.Errorf("invalid 'to' date %q, expected YYYY-MM-DD", s)
		}
		d = d.Add(24*time.Hour - time.Nanosecond)
		to = &d
	}
	return from, to, nil
}

// rangeSuffix renders the selected period for filenames: "_2026-06-01_to_2026-06-30",
// "_from_2026-06-01" or "_until_2026-06-30".
func rangeSuffix(from, to *time.Time) string {
	switch {
	case from != nil && to != nil:
		return "_" + from.Format("2006-01-02") + "_to_" + to.Format("2006-01-02")
	case from != nil:
		return "_from_" + from.Format("2006-01-02")
	case to != nil:
		return "_until_" + to.Format("2006-01-02")
	}
	return ""
}

func inRange(date time.Time, from, to *time.Time) bool {
	if from != nil && date.Before(*from) {
		return false
	}
	if to != nil && date.After(*to) {
		return false
	}
	return true
}

func filterTxRange(txs []domain.Transaction, from, to *time.Time) []domain.Transaction {
	if from == nil && to == nil {
		return txs
	}
	out := txs[:0:0]
	for _, t := range txs {
		if inRange(t.Date, from, to) {
			out = append(out, t)
		}
	}
	return out
}

func filterStockRange(trades []domain.StockTrade, from, to *time.Time) []domain.StockTrade {
	if from == nil && to == nil {
		return trades
	}
	out := trades[:0:0]
	for _, s := range trades {
		if inRange(s.Date, from, to) {
			out = append(out, s)
		}
	}
	return out
}

func toAssetExportRows(assets []domain.Asset) []assetExportRow {
	rows := make([]assetExportRow, len(assets))
	for i, a := range assets {
		rows[i] = assetExportRow{
			Name:              a.Name,
			Type:              string(a.Type),
			PurchaseDate:      formatOptionalDate(a.PurchaseDate),
			PurchasePrice:     a.PurchasePrice,
			CurrentValue:      a.CurrentValue,
			ValuationDate:     formatOptionalDate(a.ValuationDate),
			Notes:             a.Notes,
			LoanRemaining:     a.LoanRemaining,
			LoanRemainingDate: formatOptionalDate(a.LoanRemainingDate),
			LoanRate:          a.LoanRate,
			LoanAccount:       a.LoanAccount,
			LoanPaidOffDate:   formatOptionalDate(a.LoanPaidOffDate),

			LoanMargin:         a.LoanMargin,
			LoanLabel:          a.LoanLabel,
			LoanBaseRate:       a.LoanBaseRate,
			LoanRateResetDate:  formatOptionalDate(a.LoanRateResetDate),
			LoanMonthlyPayment: a.LoanMonthlyPayment,
		}
	}
	return rows
}

const exportPrompt = `You are a personal finance advisor. I'm sharing my complete financial data exported from my finance tracker app. Please analyze it and help me understand:
1. My overall financial health and net worth trend
2. My spending patterns and top expense categories
3. How my savings rate looks over time
4. My investment portfolio (stocks + crypto) performance
5. My physical assets (real estate, vehicles, solar) — value, loans and net equity
6. Any concerns or areas I should improve
7. Specific actionable recommendations for my situation

All monetary amounts are in EUR unless otherwise noted. Stock prices may be in USD.`

// ExportAllJSON godoc
// @Summary      Export all financial data as JSON for AI analysis
// @Tags         export
// @Produce      application/json
// @Success      200  {object}  financeExport
// @Router       /export/finances.json [get]
func (h *ExportHandler) ExportAllJSON(c *gin.Context) {
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

	transactions = filterTxRange(transactions, from, to)
	stocks = filterStockRange(stocks, from, to)
	sortChronologically(transactions, balances)

	ts := time.Now().Format("2006-01-02")
	// Filename prefixes tell the files apart in a downloads folder:
	// backup_… restores everything, export_… is a date-scoped extract.
	// ?purpose=export marks an all-time download that is NOT meant as a
	// backup (e.g. handing data to an AI) — it keeps the export_ name and
	// leaves the "last full export" marker untouched.
	filename := fmt.Sprintf("backup_finances_%s.json", ts)
	switch {
	case from != nil || to != nil:
		// A date-limited export is not a restorable full backup: name it
		// differently and leave the "last full export" marker untouched.
		filename = fmt.Sprintf("export_finances_%s%s.json", ts, rangeSuffix(from, to))
	case c.Query("purpose") == "export":
		filename = fmt.Sprintf("export_finances_all_%s.json", ts)
	default:
		// The marker anchors the partial-export chain: if it silently fails,
		// the next partial export starts from a stale point and quietly
		// misses data. Fail loudly instead — the export is repeatable.
		if err := h.exportLogRepo.Save("full"); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "could not record the export marker: " + err.Error()})
			return
		}
	}
	budgetRows, ruleRows := h.budgetRows()
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.JSON(http.StatusOK, financeExport{
		SchemaVersion:   exportSchemaVersion,
		ExportDate:      ts,
		SuggestedPrompt: exportPrompt,
		Transactions:    toTxExportRows(transactions),
		Balances:        toBalExportRows(balances),
		StockTrades:     toStockExportRows(stocks),
		Assets:          toAssetExportRows(assets),
		Budgets:         budgetRows,
		LabelRules:      ruleRows,
		BudgetSettings:  h.settingsRow(),
		AISettings:      h.aiSettingsRow(),
		AIContext:       h.aiContextContent(),
	})
}

// ExportPartialJSON godoc
// @Summary      Export financial data since last full export as JSON
// @Tags         export
// @Produce      application/json
// @Success      200  {object}  financeExport
// @Router       /export/finances-partial.json [get]
func (h *ExportHandler) ExportPartialJSON(c *gin.Context) {
	since, err := h.exportLogRepo.GetLastTime("full")
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	if since == nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "no full export found — run a full export first"})
		return
	}

	transactions, err := h.txSvc.ListSince(*since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	balances, err := h.balSvc.List(domain.BalanceFilter{DateFrom: since}, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	stocks, err := h.stockSvc.ListSince(*since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	assets, err := h.assetSvc.ListSince(*since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	sortChronologically(transactions, balances)

	ts := time.Now().Format("2006-01-02")
	fromStr := since.Format("2006-01-02")
	if err := h.exportLogRepo.Save("partial"); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "could not record the export marker: " + err.Error()})
		return
	}
	budgetRows, ruleRows := h.budgetRows()
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"partial_finances_%s_since_%s.json\"", ts, fromStr))
	c.JSON(http.StatusOK, financeExport{
		SchemaVersion:   exportSchemaVersion,
		ExportDate:      ts,
		SuggestedPrompt: fmt.Sprintf("This is a partial export of new financial data added since %s. Please update your analysis with these new records.", fromStr),
		Transactions:    toTxExportRows(transactions),
		Balances:        toBalExportRows(balances),
		StockTrades:     toStockExportRows(stocks),
		Assets:          toAssetExportRows(assets),
		Budgets:         budgetRows,
		LabelRules:      ruleRows,
		BudgetSettings:  h.settingsRow(),
		AISettings:      h.aiSettingsRow(),
		AIContext:       h.aiContextContent(),
	})
}

type exportStatus struct {
	LastFullExport    *string `json:"last_full_export"`
	LastPartialExport *string `json:"last_partial_export"`
}

// ExportStatus godoc
// @Summary      Get last export timestamps
// @Tags         export
// @Produce      application/json
// @Success      200  {object}  exportStatus
// @Router       /export/status [get]
func (h *ExportHandler) ExportStatus(c *gin.Context) {
	full, err := h.exportLogRepo.GetLastTime("full")
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	partial, err := h.exportLogRepo.GetLastTime("partial")
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	status := exportStatus{}
	if full != nil {
		s := full.Format("2006-01-02")
		status.LastFullExport = &s
	}
	if partial != nil {
		s := partial.Format("2006-01-02")
		status.LastPartialExport = &s
	}
	c.JSON(http.StatusOK, status)
}
