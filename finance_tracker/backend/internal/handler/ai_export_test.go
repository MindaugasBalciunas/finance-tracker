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
	"github.com/stretchr/testify/require"
)

func TestExportAIZip_YearSplitFilesAndReadme(t *testing.T) {
	gin.SetMode(gin.TestMode)
	day := func(s string) time.Time {
		d, err := time.Parse("2006-01-02", s)
		require.NoError(t, err)
		return d
	}
	txs := []domain.Transaction{
		{ID: 1, Date: day("2024-03-01"), Type: "expense", Amount: 50, Category: "Food", Comment: "Maxima", Labels: "groceries,maxima", DebitAccount: "swed"},
		{ID: 2, Date: day("2025-01-15"), Type: "income", Amount: 3000, Category: "Salary", Comment: "PVcase", CreditAccount: "swed"},
		{ID: 3, Date: day("2025-06-01"), Type: "investment", Amount: 200, Category: "Transfers", Comment: "Revolut top up", Labels: "revolut", DebitAccount: "swed", CreditAccount: "rev_m"},
	}
	bals := []domain.Balance{
		{ID: 1, Date: day("2024-03-31"), Total: 1000, Swed: 1000},
		{ID: 2, Date: day("2025-06-30"), Total: 2000, Swed: 1500, Cash: 500},
	}
	stocks := []domain.StockTrade{
		{ID: 1, Date: day("2025-02-01"), Action: "buy", Ticker: "VWCE", Shares: 2, PricePerShare: 110, Currency: "EUR", Source: "IBKR"},
	}

	txSvc := &mockTransactionService{}
	balSvc := &mockBalanceService{}
	stockSvc := &mockStockService{}
	assetSvc := &mockAssetService{}
	logRepo := &mockExportLog{}
	txSvc.On("ListAll").Return(txs, nil)
	txSvc.On("GetSummary", domain.TransactionFilter{}).Return(&domain.TransactionSummary{
		ByMonth: []domain.MonthlySummary{{Year: 2025, Month: 1, Income: 3000, Expenses: 0, Investments: 0}},
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
	for _, want := range []string{
		"README.md",
		"transactions_2024.csv", "transactions_2025.csv",
		"balances_2024.csv", "balances_2025.csv",
		"stocks_2025.csv",
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
	tx2024 := readCSV("transactions_2024.csv")
	require.Len(t, tx2024, 2, "header + one 2024 row")
	assert.Equal(t, []string{"2024-03-01", "expense", "Food", "50.00", "Maxima", "groceries,maxima", "swed", ""}, tx2024[1])
	tx2025 := readCSV("transactions_2025.csv")
	require.Len(t, tx2025, 3, "header + two 2025 rows")

	months := readCSV("monthly_summary.csv")
	require.Len(t, months, 2)
	assert.Equal(t, []string{"2025", "01", "3000.00", "0.00", "0.00", "3000.00"}, months[1])

	cats := readCSV("category_year_totals.csv")
	joined := ""
	for _, row := range cats {
		joined += strings.Join(row, "|") + "\n"
	}
	assert.Contains(t, joined, "2024|expense|Food|50.00")
	assert.Contains(t, joined, "2025|investment|Transfers|200.00")

	readme, err := names["README.md"].Open()
	require.NoError(t, err)
	body, _ := io.ReadAll(readme)
	readme.Close()
	assert.Contains(t, string(body), "Transfers")
	assert.Contains(t, string(body), "payroll")
	assert.Contains(t, string(body), "Account glossary")
}
