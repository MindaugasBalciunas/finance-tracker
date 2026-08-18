package handler_test

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// The AI ZIP is compact by design: ≤10 files (Gemini's ZIP cap), transactions
// tiered into a small recent file + a cold archive, single balances/stocks
// files, and a README that ends with a data manifest so a model can detect
// platform-side truncation.
func TestExportAIZip_CompactLayoutAndManifest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now()
	recentDay := now.AddDate(0, -3, 0)
	archiveDay := now.AddDate(0, -30, 0)
	txs := []domain.Transaction{
		{ID: 1, Date: archiveDay, Type: "expense", Amount: 50, Category: "Food", Comment: "Maxima", Labels: "groceries,maxima", DebitAccount: "swed"},
		{ID: 2, Date: recentDay, Type: "income", Amount: 3000, Category: "Salary", Comment: "PVcase", CreditAccount: "swed"},
		{ID: 3, Date: recentDay.AddDate(0, 1, 0), Type: "investment", Amount: 200, Category: "Transfers", Comment: "Revolut top up", Labels: "revolut", DebitAccount: "swed", CreditAccount: "rev_m"},
	}
	bals := []domain.Balance{
		{ID: 1, Date: archiveDay, Total: 1000, Swed: 1000},
		{ID: 2, Date: recentDay, Total: 2000, Swed: 1500, Cash: 500},
	}
	stocks := []domain.StockTrade{
		{ID: 1, Date: recentDay, Action: "buy", Ticker: "VWCE", Shares: 2, PricePerShare: 110, Currency: "EUR", Source: "IBKR"},
	}

	txSvc := &mockTransactionService{}
	balSvc := &mockBalanceService{}
	stockSvc := &mockStockService{}
	assetSvc := &mockAssetService{}
	logRepo := &mockExportLog{}
	txSvc.On("ListAll").Return(txs, nil)
	txSvc.On("GetSummary", domain.TransactionFilter{}).Return(&domain.TransactionSummary{
		ByMonth: []domain.MonthlySummary{{Year: recentDay.Year(), Month: int(recentDay.Month()), Income: 3000}},
	}, nil)
	balSvc.On("List", domain.BalanceFilter{}, float64(0)).Return(bals, nil)
	stockSvc.On("ListAll").Return(stocks, nil)
	assetSvc.On("ListAll").Return([]domain.Asset{}, nil)

	r := gin.New()
	handler.NewExportHandler(txSvc, balSvc, stockSvc, assetSvc, logRepo).RegisterRoutes(r.Group("/api/v1"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/ai.zip", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "ai_finances_")

	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	require.NoError(t, err)
	names := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		names[f.Name] = f
	}
	assert.LessOrEqual(t, len(zr.File), 10, "Gemini rejects ZIPs with more than 10 files")
	for _, want := range []string{
		"README.md",
		"transactions_recent.csv", "transactions_archive.csv",
		"balances.csv", "stocks.csv",
		"monthly_summary.csv", "category_year_totals.csv", "assets.csv", "budgets.csv",
	} {
		assert.Contains(t, names, want)
	}

	readCSV := func(name string) [][]string {
		f, err := names[name].Open()
		require.NoError(t, err)
		defer f.Close()
		rows, err := csv.NewReader(f).ReadAll()
		require.NoError(t, err)
		return rows
	}
	recent := readCSV("transactions_recent.csv")
	require.Len(t, recent, 3, "header + the two rows inside 24 months")
	assert.Equal(t, recentDay.Format("2006-01-02"), recent[1][0])
	archive := readCSV("transactions_archive.csv")
	require.Len(t, archive, 2, "header + the 30-month-old row")
	assert.Equal(t, []string{archiveDay.Format("2006-01-02"), "expense", "Food", "50.00", "Maxima", "groceries,maxima", "swed", ""}, archive[1])

	balRows := readCSV("balances.csv")
	require.Len(t, balRows, 3, "header + both snapshots in one file")
	stockRows := readCSV("stocks.csv")
	require.Len(t, stockRows, 2)

	cats := readCSV("category_year_totals.csv")
	joined := ""
	for _, row := range cats {
		joined += strings.Join(row, "|") + "\n"
	}
	assert.Contains(t, joined, "expense|Food|50.00")
	assert.Contains(t, joined, "investment|Transfers|200.00")

	readme, err := names["README.md"].Open()
	require.NoError(t, err)
	body, _ := io.ReadAll(readme)
	readme.Close()
	text := string(body)
	assert.Contains(t, text, "Transfers")
	assert.Contains(t, text, "payroll")
	assert.Contains(t, text, "Account glossary")
	assert.Contains(t, text, "DATA MANIFEST")
	assert.Contains(t, text, "transactions_recent.csv: 2 data rows")
	assert.Contains(t, text, "transactions_archive.csv: 1 data rows")
	assert.Contains(t, text, "web AI chat")
}

// A date range scopes transactions/balances/summaries; stocks stay complete
// and the manifest says the archive is period-scoped.
func TestExportAIZip_PeriodScoped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now()
	inRange := now.AddDate(0, -2, 0)
	outOfRange := now.AddDate(0, -20, 0)
	txs := []domain.Transaction{
		{ID: 1, Date: outOfRange, Type: "expense", Amount: 50, Category: "Food", Comment: "OLD ROW"},
		{ID: 2, Date: inRange, Type: "expense", Amount: 80, Category: "Food", Comment: "NEW ROW"},
	}
	stocks := []domain.StockTrade{
		{ID: 1, Date: outOfRange, Action: "buy", Ticker: "VWCE", Shares: 2, PricePerShare: 110, Currency: "EUR", Source: "IBKR"},
	}
	from := now.AddDate(0, -12, 0)

	txSvc := &mockTransactionService{}
	balSvc := &mockBalanceService{}
	stockSvc := &mockStockService{}
	assetSvc := &mockAssetService{}
	logRepo := &mockExportLog{}
	txSvc.On("ListAll").Return(txs, nil)
	txSvc.On("GetSummary", mock.MatchedBy(func(f domain.TransactionFilter) bool { return f.DateFrom != nil })).
		Return(&domain.TransactionSummary{}, nil)
	balSvc.On("List", mock.MatchedBy(func(f domain.BalanceFilter) bool { return f.DateFrom != nil }), float64(0)).
		Return([]domain.Balance{}, nil)
	stockSvc.On("ListAll").Return(stocks, nil)
	assetSvc.On("ListAll").Return([]domain.Asset{}, nil)

	r := gin.New()
	handler.NewExportHandler(txSvc, balSvc, stockSvc, assetSvc, logRepo).RegisterRoutes(r.Group("/api/v1"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/ai.zip?from="+from.Format("2006-01-02"), nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code, rec.Body.String())

	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	require.NoError(t, err)
	names := map[string]*zip.File{}
	for _, f := range zr.File {
		names[f.Name] = f
	}
	assert.NotContains(t, names, "transactions_archive.csv", "everything in range is recent")

	read := func(name string) string {
		f, err := names[name].Open()
		require.NoError(t, err)
		defer f.Close()
		b, _ := io.ReadAll(f)
		return string(b)
	}
	txt := read("transactions_recent.csv")
	assert.Contains(t, txt, "NEW ROW")
	assert.NotContains(t, txt, "OLD ROW", "out-of-range transaction excluded")
	assert.Contains(t, read("stocks.csv"), "VWCE", "trade ledger stays complete regardless of range")
	readme := read("README.md")
	assert.Contains(t, readme, "period-scoped")
}
