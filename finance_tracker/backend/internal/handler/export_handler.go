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
}

func NewExportHandler(txSvc service.TransactionService, balSvc service.BalanceService, stockSvc service.StockService, assetSvc service.AssetService, exportLogRepo repository.ExportLogRepository) *ExportHandler {
	return &ExportHandler{txSvc: txSvc, balSvc: balSvc, stockSvc: stockSvc, assetSvc: assetSvc, exportLogRepo: exportLogRepo}
}

func (h *ExportHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/export")
	g.GET("/transactions.csv", h.ExportTransactions)
	g.GET("/balances.csv", h.ExportBalances)
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
	transactions, err := h.txSvc.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	ts := time.Now().Format("2006-01-02")
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"transactions_%s.csv\"", ts))

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"id", "date", "type", "amount", "category", "comment"})
	for _, tx := range transactions {
		_ = w.Write([]string{
			strconv.FormatUint(uint64(tx.ID), 10),
			tx.Date.Format("2006-01-02"),
			string(tx.Type),
			fmt.Sprintf("%.2f", tx.Amount),
			string(tx.Category),
			tx.Comment,
		})
	}
	w.Flush()
}

// ExportBalances godoc
// @Summary      Export balance snapshots as CSV
// @Tags         export
// @Produce      text/csv
// @Success      200  {string}  string  "CSV file"
// @Router       /export/balances.csv [get]
func (h *ExportHandler) ExportBalances(c *gin.Context) {
	balances, err := h.balSvc.List(domain.BalanceFilter{}, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	ts := time.Now().Format("2006-01-02")
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"balances_%s.csv\"", ts))

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

type financeExport struct {
	ExportDate      string           `json:"export_date"`
	SuggestedPrompt string           `json:"suggested_prompt"`
	Transactions    []txExportRow    `json:"transactions"`
	Balances        []balExportRow   `json:"balances"`
	StockTrades     []stockExportRow `json:"stock_trades"`
	Assets          []assetExportRow `json:"assets"`
}

type txExportRow struct {
	ID            uint    `json:"id"`
	Date          string  `json:"date"`
	Type          string  `json:"type"`
	Amount        float64 `json:"amount_eur"`
	Category      string  `json:"category"`
	Comment       string  `json:"comment,omitempty"`
	DebitAccount  string  `json:"debit_account,omitempty"`
	CreditAccount string  `json:"credit_account,omitempty"`
	SourceAccount string  `json:"source_account,omitempty"`
}

type balExportRow struct {
	Date       string  `json:"date"`
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
}

// formatOptionalDate renders a nullable date as YYYY-MM-DD or "".
func formatOptionalDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
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

	sortChronologically(transactions, balances)

	txRows := make([]txExportRow, len(transactions))
	for i, tx := range transactions {
		txRows[i] = txExportRow{
			ID:            tx.ID,
			Date:          tx.Date.Format("2006-01-02"),
			Type:          string(tx.Type),
			Amount:        tx.Amount,
			Category:      string(tx.Category),
			Comment:       tx.Comment,
			DebitAccount:  tx.DebitAccount,
			CreditAccount: tx.CreditAccount,
			SourceAccount: tx.SourceAccount,
		}
	}

	balRows := make([]balExportRow, len(balances))
	for i, b := range balances {
		balRows[i] = balExportRow{
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
	}

	stockRows := make([]stockExportRow, len(stocks))
	for i, s := range stocks {
		stockRows[i] = stockExportRow{
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

	ts := time.Now().Format("2006-01-02")
	_ = h.exportLogRepo.Save("full")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"finances_%s.json\"", ts))
	c.JSON(http.StatusOK, financeExport{
		ExportDate:      ts,
		SuggestedPrompt: exportPrompt,
		Transactions:    txRows,
		Balances:        balRows,
		StockTrades:     stockRows,
		Assets:          toAssetExportRows(assets),
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

	txRows := make([]txExportRow, len(transactions))
	for i, tx := range transactions {
		txRows[i] = txExportRow{
			ID:            tx.ID,
			Date:          tx.Date.Format("2006-01-02"),
			Type:          string(tx.Type),
			Amount:        tx.Amount,
			Category:      string(tx.Category),
			Comment:       tx.Comment,
			DebitAccount:  tx.DebitAccount,
			CreditAccount: tx.CreditAccount,
			SourceAccount: tx.SourceAccount,
		}
	}

	balRows := make([]balExportRow, len(balances))
	for i, b := range balances {
		balRows[i] = balExportRow{
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
	}

	stockRows := make([]stockExportRow, len(stocks))
	for i, s := range stocks {
		stockRows[i] = stockExportRow{
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

	ts := time.Now().Format("2006-01-02")
	fromStr := since.Format("2006-01-02")
	_ = h.exportLogRepo.Save("partial")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"finances_partial_%s_from_%s.json\"", ts, fromStr))
	c.JSON(http.StatusOK, financeExport{
		ExportDate:      ts,
		SuggestedPrompt: fmt.Sprintf("This is a partial export of new financial data added since %s. Please update your analysis with these new records.", fromStr),
		Transactions:    txRows,
		Balances:        balRows,
		StockTrades:     stockRows,
		Assets:          toAssetExportRows(assets),
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
