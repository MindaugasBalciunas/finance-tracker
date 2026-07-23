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
	txRepo     repository.TransactionRepository
	balRepo    repository.BalanceRepository
	stockRepo  repository.StockRepository
	assetRepo  repository.AssetRepository
	budgetRepo repository.BudgetRepository
}

func NewImportHandler(
	txRepo repository.TransactionRepository,
	balRepo repository.BalanceRepository,
	stockRepo repository.StockRepository,
	assetRepo repository.AssetRepository,
) *ImportHandler {
	return &ImportHandler{txRepo: txRepo, balRepo: balRepo, stockRepo: stockRepo, assetRepo: assetRepo}
}

// WithBudgets enables budget/label-rule import and deterministic re-labeling
// of imported historical records.
func (h *ImportHandler) WithBudgets(repo repository.BudgetRepository) *ImportHandler {
	h.budgetRepo = repo
	return h
}

func (h *ImportHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/import/json", h.ImportJSON)
}

type importResult struct {
	Imported      importCounts `json:"imported"`
	Skipped       importCounts `json:"skipped"`
	ImportedTxIDs []uint       `json:"imported_tx_ids,omitempty"`
	Relabeled     int          `json:"relabeled"`
}

type importCounts struct {
	Transactions int `json:"transactions"`
	Balances     int `json:"balances"`
	StockTrades  int `json:"stock_trades"`
	Assets       int `json:"assets"`
	Budgets      int `json:"budgets"`
	LabelRules   int `json:"label_rules"`
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
	// Primary dedup: by original ID (handles re-importing the same export file).
	// Fingerprint dedup: only for rows without an ID, to catch content-identical
	// duplicates from manual/ID-less imports. Never applied to rows with IDs —
	// legitimate transactions can share the same date/amount/category/comment
	// (e.g. three rounds at the same bar on the same night).
	existingTxs, _ := h.txRepo.ListAll()
	contentSeen := make(map[string]bool, len(existingTxs))
	for _, t := range existingTxs {
		contentSeen[fmt.Sprintf("%s|%s|%.2f|%s|%s", t.Date.Format("2006-01-02"), t.Type, t.Amount, t.Category, t.Comment)] = true
	}

	for _, row := range payload.Transactions {
		date, err := time.Parse("2006-01-02", row.Date)
		if err != nil {
			result.Skipped.Transactions++
			continue
		}
		if row.ID > 0 {
			// ID-based dedup: skip if this exact record is already present.
			if _, err := h.txRepo.GetByID(row.ID); err == nil {
				result.Skipped.Transactions++
				continue
			}
		} else {
			// No ID: fall back to content fingerprint to avoid true duplicates.
			key := fmt.Sprintf("%s|%s|%.2f|%s|%s", date.Format("2006-01-02"), row.Type, row.Amount, row.Category, row.Comment)
			if contentSeen[key] {
				result.Skipped.Transactions++
				continue
			}
			contentSeen[key] = true
		}
		cat := domain.Category(row.Category)
		tx := &domain.Transaction{
			Date:          date,
			Type:          domain.TransactionType(row.Type),
			Amount:        row.Amount,
			Category:      cat,
			Comment:       row.Comment,
			Labels:        domain.NormalizeLabels(row.Labels),
			DebitAccount:  row.DebitAccount,
			CreditAccount: row.CreditAccount,
			SourceAccount: row.SourceAccount,
		}
		if row.ID > 0 {
			tx.ID = row.ID // preserve original ID so re-imports are idempotent
		}
		if err := h.txRepo.Create(tx); err != nil {
			result.Skipped.Transactions++
			continue
		}
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
			Date:       date,
			Total:      row.Total,
			Seb:        row.Seb,
			Swed:       row.Swed,
			SwedETF:    row.SwedETF,
			SebPen:     row.SebPen,
			Luminor:    row.Luminor,
			Art:        row.Art,
			Cash:       row.Cash,
			RevM:       row.RevM,
			RevR:       row.RevR,
			RBTC:       row.RBTC,
			MBTC:       row.MBTC,
			BtcPrice:   row.BtcPrice,
			RevStocks:  row.RevStocks,
			IBKRStocks: row.IBKRStocks,
		}
		if err := h.balRepo.Create(b); err != nil {
			result.Skipped.Balances++
			continue
		}
		result.Imported.Balances++
	}

	// --- Stock trades — deduplicate by date+ticker+action+shares ---
	// Shares are included because the same ticker can be bought/sold multiple
	// times on the same day in separate transactions at different quantities.
	existingStocks, _ := h.stockRepo.ListAll()
	seen := make(map[string]bool, len(existingStocks))
	for _, s := range existingStocks {
		seen[fmt.Sprintf("%s|%s|%s|%.4f", s.Date.Format("2006-01-02"), s.Ticker, string(s.Action), s.Shares)] = true
	}

	for _, row := range payload.StockTrades {
		date, err := time.Parse("2006-01-02", row.Date)
		if err != nil {
			result.Skipped.StockTrades++
			continue
		}
		key := fmt.Sprintf("%s|%s|%s|%.4f", date.Format("2006-01-02"), row.Ticker, row.Action, row.Shares)
		if seen[key] {
			result.Skipped.StockTrades++
			continue
		}
		source := domain.StockSource(row.Source)
		if source != domain.StockSourceIBKR {
			source = domain.StockSourceRevolut
		}
		trade := &domain.StockTrade{
			Date:          date,
			Action:        domain.StockAction(row.Action),
			Ticker:        row.Ticker,
			Shares:        row.Shares,
			PricePerShare: row.PricePerShare,
			Currency:      row.Currency,
			Source:        source,
			Notes:         row.Notes,
		}
		if err := h.stockRepo.Create(trade); err != nil {
			result.Skipped.StockTrades++
			continue
		}
		seen[key] = true
		result.Imported.StockTrades++
	}

	// --- Assets — deduplicate by name+type+purchase_date ---
	// Assets have no export ID; name plus type plus purchase date uniquely
	// identifies a physical asset for re-import purposes.
	existingAssets, _ := h.assetRepo.ListAll()
	assetSeen := make(map[string]bool, len(existingAssets))
	assetKey := func(name, typ, purchaseDate string) string {
		return fmt.Sprintf("%s|%s|%s", name, typ, purchaseDate)
	}
	for _, a := range existingAssets {
		assetSeen[assetKey(a.Name, string(a.Type), formatOptionalDate(a.PurchaseDate))] = true
	}

	parseOptDate := func(s string) (*time.Time, bool) {
		if s == "" {
			return nil, true
		}
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			return nil, false
		}
		return &d, true
	}

	for _, row := range payload.Assets {
		key := assetKey(row.Name, row.Type, row.PurchaseDate)
		if row.Name == "" || assetSeen[key] {
			result.Skipped.Assets++
			continue
		}
		purchaseDate, ok1 := parseOptDate(row.PurchaseDate)
		valuationDate, ok2 := parseOptDate(row.ValuationDate)
		loanRemainingDate, ok3 := parseOptDate(row.LoanRemainingDate)
		loanPaidOffDate, ok4 := parseOptDate(row.LoanPaidOffDate)
		if !ok1 || !ok2 || !ok3 || !ok4 {
			result.Skipped.Assets++
			continue
		}
		currentValue := row.CurrentValue
		if currentValue == 0 {
			currentValue = row.PurchasePrice
		}
		asset := &domain.Asset{
			Name:              row.Name,
			Type:              domain.AssetType(row.Type),
			PurchaseDate:      purchaseDate,
			PurchasePrice:     row.PurchasePrice,
			CurrentValue:      currentValue,
			ValuationDate:     valuationDate,
			Notes:             row.Notes,
			LoanRemaining:     row.LoanRemaining,
			LoanRemainingDate: loanRemainingDate,
			LoanRate:          row.LoanRate,
			LoanAccount:       row.LoanAccount,
			LoanPaidOffDate:   loanPaidOffDate,
		}
		if err := h.assetRepo.Create(asset); err != nil {
			result.Skipped.Assets++
			continue
		}
		assetSeen[key] = true
		result.Imported.Assets++
	}

	// --- Budgets & label rules (when present in the export) ---
	if h.budgetRepo != nil {
		existingBudgets, _ := h.budgetRepo.ListBudgets()
		budgetSeen := make(map[string]bool, len(existingBudgets))
		for _, b := range existingBudgets {
			budgetSeen[b.Name+"|"+b.Kind] = true
		}
		for _, row := range payload.Budgets {
			if budgetSeen[row.Name+"|"+row.Kind] || !domain.IsValidBudgetKind(row.Kind) {
				result.Skipped.Budgets++
				continue
			}
			b := domain.Budget{Name: row.Name, Kind: row.Kind, Label: row.Label, Category: row.Category, Amount: row.Amount}
			if err := h.budgetRepo.SaveBudget(&b); err != nil {
				result.Skipped.Budgets++
				continue
			}
			budgetSeen[b.Name+"|"+b.Kind] = true
			result.Imported.Budgets++
		}

		existingRules, _ := h.budgetRepo.ListRules()
		ruleSeen := make(map[string]bool, len(existingRules))
		for _, r := range existingRules {
			ruleSeen[r.Label+"|"+r.Category+"|"+r.CommentMatch] = true
		}
		for _, row := range payload.LabelRules {
			key := row.Label + "|" + row.Category + "|" + row.CommentMatch
			if row.Label == "" || ruleSeen[key] {
				result.Skipped.LabelRules++
				continue
			}
			rule := domain.LabelRule{Label: row.Label, Category: row.Category, CommentMatch: row.CommentMatch}
			if err := h.budgetRepo.SaveRule(&rule); err != nil {
				result.Skipped.LabelRules++
				continue
			}
			ruleSeen[key] = true
			result.Imported.LabelRules++
		}

		if payload.BudgetSettings != nil && domain.IsValidIncomeMode(payload.BudgetSettings.IncomeMode) {
			if cur, err := h.budgetRepo.GetSettings(); err == nil {
				cur.IncomeMode = payload.BudgetSettings.IncomeMode
				cur.ManualIncome = payload.BudgetSettings.ManualIncome
				cur.GrossSalary = payload.BudgetSettings.GrossSalary
				cur.MonthlyDeductions = payload.BudgetSettings.MonthlyDeductions
				_ = h.budgetRepo.SaveSettings(cur)
			}
		}

		// Deterministic migration: re-apply every rule across the whole table so
		// imported historical records (and pre-label rows) get their labels.
		allRules, _ := h.budgetRepo.ListRules()
		for _, rule := range allRules {
			if n, err := h.budgetRepo.ApplyLabel(rule); err == nil {
				result.Relabeled += n
			}
		}
	}

	c.JSON(http.StatusOK, result)
}
