package handler

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/service"
)

type ExportHandler struct {
	txSvc  service.TransactionService
	balSvc service.BalanceService
}

func NewExportHandler(txSvc service.TransactionService, balSvc service.BalanceService) *ExportHandler {
	return &ExportHandler{txSvc: txSvc, balSvc: balSvc}
}

func (h *ExportHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/export")
	g.GET("/transactions.csv", h.ExportTransactions)
	g.GET("/balances.csv", h.ExportBalances)
}

// ExportTransactions godoc
// @Summary      Export transactions as CSV
// @Tags         export
// @Produce      text/csv
// @Success      200  {string}  string  "CSV file"
// @Router       /export/transactions.csv [get]
func (h *ExportHandler) ExportTransactions(c *gin.Context) {
	result, err := h.txSvc.List(domain.TransactionFilter{Page: 1, PageSize: 1000000})
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=\"transactions.csv\"")

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"id", "date", "type", "amount", "category", "comment"})
	for _, tx := range result.Data {
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
	balances, err := h.balSvc.List(domain.BalanceFilter{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=\"balances.csv\"")

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{
		"id", "date", "total", "seb", "swed", "swed_etf", "swed_pen",
		"luminor", "art", "cash", "rev_m", "rev_r", "rbtc", "mbtc", "btc_price", "rev_stocks",
	})
	for _, b := range balances {
		_ = w.Write([]string{
			strconv.FormatUint(uint64(b.ID), 10),
			b.Date.Format("2006-01-02"),
			fmt.Sprintf("%.2f", b.Total),
			fmt.Sprintf("%.2f", b.Seb),
			fmt.Sprintf("%.2f", b.Swed),
			fmt.Sprintf("%.2f", b.SwedETF),
			fmt.Sprintf("%.2f", b.SwedPen),
			fmt.Sprintf("%.2f", b.Luminor),
			fmt.Sprintf("%.2f", b.Art),
			fmt.Sprintf("%.2f", b.Cash),
			fmt.Sprintf("%.2f", b.RevM),
			fmt.Sprintf("%.2f", b.RevR),
			fmt.Sprintf("%.8f", b.RBTC),
			fmt.Sprintf("%.8f", b.MBTC),
			fmt.Sprintf("%.2f", b.BtcPrice),
			fmt.Sprintf("%.2f", b.RevStocks),
		})
	}
	w.Flush()
}
