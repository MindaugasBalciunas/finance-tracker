package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
)

type ImportHandler struct {
	txRepo    repository.TransactionRepository
	balRepo   repository.BalanceRepository
	stockRepo repository.StockRepository
}

func NewImportHandler(
	txRepo repository.TransactionRepository,
	balRepo repository.BalanceRepository,
	stockRepo repository.StockRepository,
) *ImportHandler {
	return &ImportHandler{txRepo: txRepo, balRepo: balRepo, stockRepo: stockRepo}
}

func (h *ImportHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/import/json", h.ImportJSON)
}

type importResult struct {
	Imported      importCounts `json:"imported"`
	Skipped       importCounts `json:"skipped"`
	ImportedTxIDs []uint       `json:"imported_tx_ids,omitempty"`
}

type importCounts struct {
	Transactions int `json:"transactions"`
	Balances     int `json:"balances"`
	StockTrades  int `json:"stock_trades"`
}

// ImportJSON godoc
// @Summary      Import financial data from a finances.json export file
// @Tags         import
// @Accept       multipart/form-data
// @Produce      application/json
// @Param        file formData file true "finances.json export file"
// @Success      200  {object}  importResult
// @Router       /import/json [post]
func (h *ImportHandler) ImportJSON(c *gin.Context) {
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "missing 'file' field in form"})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to read file"})
		return
	}

	var payload financeExport
	if err := json.Unmarshal(data, &payload); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid JSON: " + err.Error()})
		return
	}

	result := importResult{}

	// --- Transactions ---
	// Build content-fingerprint set from existing transactions to catch
	// duplicates even when IDs differ (e.g. importing into a different DB).
	existingTxs, _ := h.txRepo.ListAll()
	contentSeen := make(map[string]bool, len(existingTxs))
	for _, t := range existingTxs {
		contentSeen[fmt.Sprintf("%s|%s|%.2f|%s", t.Date.Format("2006-01-02"), t.Type, t.Amount, t.Category)] = true
	}

	for _, row := range payload.Transactions {
		date, err := time.Parse("2006-01-02", row.Date)
		if err != nil {
			result.Skipped.Transactions++
			continue
		}
		// Skip by original ID
		if row.ID > 0 {
			if _, err := h.txRepo.GetByID(row.ID); err == nil {
				result.Skipped.Transactions++
				continue
			}
		}
		// Skip by content fingerprint (catches duplicates when IDs don't match)
		key := fmt.Sprintf("%s|%s|%.2f|%s|%s", date.Format("2006-01-02"), row.Type, row.Amount, row.Category, row.Comment)
		if contentSeen[key] {
			result.Skipped.Transactions++
			continue
		}
		tx := &domain.Transaction{
			Date:     date,
			Type:     domain.TransactionType(row.Type),
			Amount:   row.Amount,
			Category: domain.Category(row.Category),
			Comment:  row.Comment,
		}
		if row.ID > 0 {
			tx.ID = row.ID // preserve original ID so re-imports are idempotent
		}
		if err := h.txRepo.Create(tx); err != nil {
			result.Skipped.Transactions++
			continue
		}
		contentSeen[key] = true
		result.ImportedTxIDs = append(result.ImportedTxIDs, tx.ID)
		result.Imported.Transactions++
	}

	// --- Balances — skip if date already exists (unique index) ---
	for _, row := range payload.Balances {
		date, err := time.Parse("2006-01-02", row.Date)
		if err != nil {
			result.Skipped.Balances++
			continue
		}
		dayEnd := date.Add(24 * time.Hour)
		existing, _ := h.balRepo.List(domain.BalanceFilter{DateFrom: &date, DateTo: &dayEnd})
		if len(existing) > 0 {
			result.Skipped.Balances++
			continue
		}
		b := &domain.Balance{
			Date:      date,
			Total:     row.Total,
			Seb:       row.Seb,
			Swed:      row.Swed,
			SwedETF:   row.SwedETF,
			SebPen:    row.SebPen,
			Luminor:   row.Luminor,
			Art:       row.Art,
			Cash:      row.Cash,
			RevM:      row.RevM,
			RevR:      row.RevR,
			RBTC:      row.RBTC,
			MBTC:      row.MBTC,
			BtcPrice:  row.BtcPrice,
			RevStocks: row.RevStocks,
		}
		if err := h.balRepo.Create(b); err != nil {
			result.Skipped.Balances++
			continue
		}
		result.Imported.Balances++
	}

	// --- Stock trades — deduplicate by date+ticker+action ---
	existingStocks, _ := h.stockRepo.ListAll()
	seen := make(map[string]bool, len(existingStocks))
	for _, s := range existingStocks {
		seen[s.Date.Format("2006-01-02")+"|"+s.Ticker+"|"+string(s.Action)] = true
	}

	for _, row := range payload.StockTrades {
		date, err := time.Parse("2006-01-02", row.Date)
		if err != nil {
			result.Skipped.StockTrades++
			continue
		}
		key := date.Format("2006-01-02") + "|" + row.Ticker + "|" + row.Action
		if seen[key] {
			result.Skipped.StockTrades++
			continue
		}
		trade := &domain.StockTrade{
			Date:          date,
			Action:        domain.StockAction(row.Action),
			Ticker:        row.Ticker,
			Shares:        row.Shares,
			PricePerShare: row.PricePerShare,
			Currency:      row.Currency,
			Notes:         row.Notes,
		}
		if err := h.stockRepo.Create(trade); err != nil {
			result.Skipped.StockTrades++
			continue
		}
		seen[key] = true
		result.Imported.StockTrades++
	}

	c.JSON(http.StatusOK, result)
}
