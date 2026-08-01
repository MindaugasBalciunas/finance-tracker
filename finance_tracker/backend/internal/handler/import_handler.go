package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	rg.POST("/import/swedbank", h.ImportSwedbankCSV)
	rg.POST("/import/invl", h.ImportINVLCSV)
}

type swedbankImportResult struct {
	Imported  int    `json:"imported"`
	Duplicate int    `json:"duplicate"`
	Internal  int    `json:"internal"`
	Relabeled int    `json:"relabeled"`
	Balances  int    `json:"balances"`
	Enriched  int    `json:"enriched"`
	Unmatched int    `json:"unmatched"`
	DateFrom  string `json:"date_from,omitempty"`
	DateTo    string `json:"date_to,omitempty"`
}

// foldLT lowercases and strips Lithuanian diacritics so "EVELINA
// BALCIUNIENE" (bank spelling) matches "EVELINA BALČIŪNIENĖ" (curated row).
func foldLT(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	r := strings.NewReplacer("ą", "a", "č", "c", "ę", "e", "ė", "e", "į", "i", "š", "s", "ų", "u", "ū", "u", "ž", "z")
	return r.Replace(s)
}

// ImportSwedbankCSV godoc
// @Summary      Import a Swedbank account statement CSV
// @Tags         import
// @Accept       multipart/form-data
// @Produce      application/json
// @Param        file formData file true "Swedbank statement .csv"
// @Success      200  {object}  swedbankImportResult
// @Router       /import/swedbank [post]
func (h *ImportHandler) ImportSwedbankCSV(c *gin.Context) {
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "missing 'file' field in form"})
		return
	}
	defer file.Close()

	rows, stmtBalances, internal, err := parseSwedbankCSV(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "failed to parse statement: " + err.Error()})
		return
	}

	// Dedup by date|type|amount|comment as a MULTISET: each existing row
	// absorbs one matching statement row, so re-imports are no-ops while
	// genuine repeats (two identical rounds at the same bar) still import.
	// Category is deliberately excluded from the key: the same bank row may
	// have been categorised differently by an earlier import.
	existing, _ := h.txRepo.ListAll()
	remaining := make(map[string]int, len(existing))
	key := func(date time.Time, typ domain.TransactionType, amount float64, comment string) string {
		return fmt.Sprintf("%s|%s|%.2f|%s", date.Format("2006-01-02"), typ, amount, strings.ToLower(strings.TrimSpace(comment)))
	}
	for _, t := range existing {
		remaining[key(t.Date, t.Type, t.Amount, t.Comment)]++
	}

	// Enrich mode: never create rows. A statement row whose classified
	// comment EXTENDS an existing row's comment (same date/type/amount,
	// diacritic-insensitive prefix — "EVELINA PLYTNIKAITĖ" → "EVELINA
	// PLYTNIKAITĖ (Būsto paskola)") upgrades that row's comment and merges
	// labels. Curated comments that aren't a prefix are left alone.
	enrich := c.Query("mode") == "enrich"
	candidates := map[string][]*domain.Transaction{}
	if enrich {
		for i := range existing {
			t := &existing[i]
			ck := fmt.Sprintf("%s|%s|%.2f", t.Date.Format("2006-01-02"), t.Type, t.Amount)
			candidates[ck] = append(candidates[ck], t)
		}
	}

	result := swedbankImportResult{Internal: internal}
	for _, row := range rows {
		k := key(row.Date, row.Type, row.Amount, row.Comment)
		if remaining[k] > 0 {
			remaining[k]--
			result.Duplicate++
			continue
		}
		if enrich {
			ck := fmt.Sprintf("%s|%s|%.2f", row.Date.Format("2006-01-02"), row.Type, row.Amount)
			matched := false
			for i, cand := range candidates[ck] {
				if cand == nil || len(cand.Comment) == 0 || len(row.Comment) <= len(cand.Comment) {
					continue
				}
				if strings.HasPrefix(foldLT(row.Comment), foldLT(cand.Comment)) {
					cand.Comment = row.Comment
					for _, l := range strings.Split(domain.NormalizeLabels(row.Labels), ",") {
						if l != "" {
							cand.AddLabel(l)
						}
					}
					if err := h.txRepo.Update(cand); err == nil {
						result.Enriched++
						candidates[ck][i] = nil // consumed
						matched = true
					}
					break
				}
			}
			if !matched {
				result.Unmatched++
			}
			continue
		}
		tx := &domain.Transaction{
			Date:          row.Date,
			Type:          row.Type,
			Amount:        row.Amount,
			Category:      row.Category,
			Comment:       row.Comment,
			Labels:        domain.NormalizeLabels(row.Labels),
			DebitAccount:  row.Debit,
			CreditAccount: row.Credit,
		}
		if err := h.txRepo.Create(tx); err != nil {
			result.Duplicate++
			continue
		}
		result.Imported++
		d := row.Date.Format("2006-01-02")
		if result.DateFrom == "" || d < result.DateFrom {
			result.DateFrom = d
		}
		if d > result.DateTo {
			result.DateTo = d
		}
	}

	// Deterministic labeling over the new rows (groceries, fuel, security…).
	if h.budgetRepo != nil {
		if rules, err := h.budgetRepo.ListRules(); err == nil {
			for _, rule := range rules {
				if n, err := h.budgetRepo.ApplyLabel(rule); err == nil {
					result.Relabeled += n
				}
			}
		}
	}

	// Restore the Swedbank balance history from the statement's own opening/
	// closing rows — but only BEFORE the fully-tracked era. Once real
	// multi-account snapshots exist, injecting a swed-only row would show a
	// misleading dip in total net worth.
	if len(stmtBalances) > 0 {
		existingBals, _ := h.balRepo.List(domain.BalanceFilter{})
		taken := make(map[string]bool, len(existingBals))
		// Statement-restored snapshots carry only swed — possibly enriched
		// with art (INVL import) and cash/seb_pen (startup backfills) — so
		// the boundary of the fully-tracked era is the first snapshot
		// holding accounts beyond the restorable/backfillable ones.
		earliestFull := time.Time{}
		for _, b := range existingBals {
			taken[b.Date.Format("2006-01-02")] = true
			if b.Total-b.Swed-b.Art-b.Cash-b.SebPen > 0.005 {
				if earliestFull.IsZero() || b.Date.Before(earliestFull) {
					earliestFull = b.Date
				}
			}
		}
		for _, sb := range stmtBalances {
			day := sb.Date.Format("2006-01-02")
			if taken[day] || (!earliestFull.IsZero() && !sb.Date.Before(earliestFull)) {
				continue
			}
			if err := h.balRepo.Create(&domain.Balance{Date: sb.Date, Swed: sb.Amount, Total: sb.Amount}); err == nil {
				taken[day] = true
				result.Balances++
			}
		}
	}

	c.JSON(http.StatusOK, result)
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
		// Old exports may carry retired categories (Kids - *, Divorce) — map
		// them before fingerprinting so they dedup against migrated rows.
		cat, legacyLabels := domain.CanonicalCategory(domain.TransactionType(row.Type), domain.Category(row.Category))
		labels := row.Labels
		for _, l := range legacyLabels {
			labels += "," + l
		}
		if row.ID > 0 {
			// ID-based dedup: skip if this exact record is already present.
			if _, err := h.txRepo.GetByID(row.ID); err == nil {
				result.Skipped.Transactions++
				continue
			}
		} else {
			// No ID: fall back to content fingerprint to avoid true duplicates.
			key := fmt.Sprintf("%s|%s|%.2f|%s|%s", date.Format("2006-01-02"), row.Type, row.Amount, cat, row.Comment)
			if contentSeen[key] {
				result.Skipped.Transactions++
				continue
			}
			contentSeen[key] = true
		}
		tx := &domain.Transaction{
			Date:          date,
			Type:          domain.TransactionType(row.Type),
			Amount:        row.Amount,
			Category:      cat,
			Comment:       row.Comment,
			Labels:        domain.NormalizeLabels(labels),
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

	// --- Balances ---
	// v2 exports carry id + time: dedup by ID (idempotent re-import) and keep
	// the time-of-day so multiple snapshots per day survive a restore.
	// v1 exports (no id/time) fall back to the old one-per-day rule.
	for _, row := range payload.Balances {
		date, err := time.Parse("2006-01-02", row.Date)
		if err != nil {
			result.Skipped.Balances++
			continue
		}
		if row.Time != "" {
			if t, terr := time.Parse("15:04:05", row.Time); terr == nil {
				date = date.Add(time.Duration(t.Hour())*time.Hour +
					time.Duration(t.Minute())*time.Minute +
					time.Duration(t.Second())*time.Second)
			}
		}
		if row.ID > 0 {
			if _, err := h.balRepo.GetByID(row.ID); err == nil {
				result.Skipped.Balances++
				continue
			}
		} else {
			dayStart := date.Truncate(24 * time.Hour)
			dayEnd := dayStart.Add(24*time.Hour - time.Nanosecond)
			existing, _ := h.balRepo.List(domain.BalanceFilter{DateFrom: &dayStart, DateTo: &dayEnd})
			if len(existing) > 0 {
				result.Skipped.Balances++
				continue
			}
		}
		b := &domain.Balance{
			ID:         row.ID,
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
			// Budgets from old exports may target retired categories.
			cat, _ := domain.CanonicalCategory("", domain.Category(row.Category))
			b := domain.Budget{Name: row.Name, Kind: row.Kind, Label: row.Label, Category: string(cat), Amount: row.Amount}
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
			// Category-scoped rules from old exports: Kids - * folds into Kids;
			// a Divorce-scoped rule would over-apply on Finance, so drop it.
			if row.Category == "Divorce" {
				result.Skipped.LabelRules++
				continue
			}
			canonRuleCat, _ := domain.CanonicalCategory("", domain.Category(row.Category))
			ruleCat := string(canonRuleCat)
			key := row.Label + "|" + ruleCat + "|" + row.CommentMatch
			if row.Label == "" || ruleSeen[key] {
				result.Skipped.LabelRules++
				continue
			}
			rule := domain.LabelRule{Label: row.Label, Category: ruleCat, CommentMatch: row.CommentMatch}
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
