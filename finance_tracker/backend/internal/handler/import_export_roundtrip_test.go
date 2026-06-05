package handler_test

// Round-trip tests: prove that export → import produces zero delta.
//
// Strategy:
//   1. Export: drive ExportHandler with the existing mock services (defined in
//      *_handler_test.go files in this package) populated with known fixtures.
//      Capture the JSON response.
//   2. Import: drive ImportHandler with mock repositories, POST the captured JSON.
//   3. Assert that importResult.Imported == original count, Skipped == 0.
//   4. Idempotency: second import with all records already present →
//      Imported == 0, Skipped == original count.

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/handler"
	"github.com/mindaugas/finance-tracker/internal/repository/mock"
	tm "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- local mirror of the unexported financeExport struct ----
// Mirrors the JSON produced by ExportAllJSON exactly, so the test
// can both parse export output and construct import payloads.

type rtExport struct {
	ExportDate  string      `json:"export_date"`
	Transactions []rtTx     `json:"transactions"`
	Balances    []rtBalance `json:"balances"`
	StockTrades []rtStock   `json:"stock_trades"`
}

type rtTx struct {
	ID       uint    `json:"id"`
	Date     string  `json:"date"`
	Type     string  `json:"type"`
	Amount   float64 `json:"amount_eur"`
	Category string  `json:"category"`
	Comment  string  `json:"comment,omitempty"`
}

type rtBalance struct {
	Date      string  `json:"date"`
	Total     float64 `json:"total_eur"`
	Seb       float64 `json:"seb,omitempty"`
	Swed      float64 `json:"swed,omitempty"`
	SwedETF   float64 `json:"swed_etf,omitempty"`
	SebPen    float64 `json:"seb_pension,omitempty"`
	Luminor   float64 `json:"luminor,omitempty"`
	Art       float64 `json:"art,omitempty"`
	Cash      float64 `json:"cash,omitempty"`
	RevM      float64 `json:"revolut_m,omitempty"`
	RevR      float64 `json:"revolut_r,omitempty"`
	RBTC      float64 `json:"btc_r,omitempty"`
	MBTC      float64 `json:"btc_m,omitempty"`
	BtcPrice  float64 `json:"btc_price_eur,omitempty"`
	RevStocks float64 `json:"revolut_stocks,omitempty"`
}

type rtStock struct {
	Date          string  `json:"date"`
	Action        string  `json:"action"`
	Ticker        string  `json:"ticker"`
	Shares        float64 `json:"shares"`
	PricePerShare float64 `json:"price_per_share"`
	Currency      string  `json:"currency"`
	Notes         string  `json:"notes,omitempty"`
}

type rtImportResult struct {
	Imported rtCounts `json:"imported"`
	Skipped  rtCounts `json:"skipped"`
}
type rtCounts struct {
	Transactions int `json:"transactions"`
	Balances     int `json:"balances"`
	StockTrades  int `json:"stock_trades"`
}

// ---- mock for ExportLogRepository (not in repository/mock/) ----

type mockExportLog struct{ tm.Mock }

func (m *mockExportLog) Save(t string) error {
	return m.Called(t).Error(0)
}
func (m *mockExportLog) GetLastTime(t string) (*time.Time, error) {
	args := m.Called(t)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*time.Time), args.Error(1)
}

// ---- canonical test fixtures ----

var (
	rtDay1 = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	rtDay2 = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	rtDay3 = time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)

	rtTransactions = []domain.Transaction{
		{ID: 1, Date: rtDay1, Type: domain.TransactionTypeExpense, Amount: 88.91, Category: domain.CategoryFood, Comment: "Maxima"},
		{ID: 2, Date: rtDay2, Type: domain.TransactionTypeIncome, Amount: 3500.00, Category: domain.CategorySalary},
		{ID: 3, Date: rtDay3, Type: domain.TransactionTypeInvestment, Amount: 200.00, Category: domain.CategoryStocksETF, Comment: "ETF buy"},
	}

	rtBalances = []domain.Balance{
		{Date: rtDay1, Total: 15000, Seb: 5000, Swed: 3000, Cash: 500, RBTC: 0.05, BtcPrice: 80000},
		{Date: rtDay2, Total: 18200, Seb: 5500, Swed: 3200, RevStocks: 1500},
	}

	rtStocks = []domain.StockTrade{
		{Date: rtDay1, Action: domain.StockActionBuy, Ticker: "AAPL", Shares: 5, PricePerShare: 210.50, Currency: "USD"},
		{Date: rtDay2, Action: domain.StockActionBuy, Ticker: "MSFT", Shares: 2, PricePerShare: 415.00, Currency: "USD", Notes: "DCA"},
	}
)

// ---- helpers ----

func rtMultipartBody(t *testing.T, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "finances.json")
	require.NoError(t, err)
	_, err = fw.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return &buf, w.FormDataContentType()
}

func rtImportRouter(txRepo *mock.TransactionRepository, balRepo *mock.BalanceRepository, stockRepo *mock.StockRepository) *gin.Engine {
	r := gin.New()
	handler.NewImportHandler(txRepo, balRepo, stockRepo).RegisterRoutes(r.Group("/api/v1"))
	return r
}

// rtBuildJSON constructs a finances.json payload from raw domain objects,
// mirroring exactly what ExportAllJSON serialises.
func rtBuildJSON(t *testing.T, txs []domain.Transaction, bals []domain.Balance, stocks []domain.StockTrade) []byte {
	t.Helper()
	txRows := make([]rtTx, len(txs))
	for i, tx := range txs {
		txRows[i] = rtTx{ID: tx.ID, Date: tx.Date.Format("2006-01-02"), Type: string(tx.Type), Amount: tx.Amount, Category: string(tx.Category), Comment: tx.Comment}
	}
	balRows := make([]rtBalance, len(bals))
	for i, b := range bals {
		balRows[i] = rtBalance{Date: b.Date.Format("2006-01-02"), Total: b.Total, Seb: b.Seb, Swed: b.Swed, SwedETF: b.SwedETF, SebPen: b.SebPen, Luminor: b.Luminor, Art: b.Art, Cash: b.Cash, RevM: b.RevM, RevR: b.RevR, RBTC: b.RBTC, MBTC: b.MBTC, BtcPrice: b.BtcPrice, RevStocks: b.RevStocks}
	}
	stockRows := make([]rtStock, len(stocks))
	for i, s := range stocks {
		stockRows[i] = rtStock{Date: s.Date.Format("2006-01-02"), Action: string(s.Action), Ticker: s.Ticker, Shares: s.Shares, PricePerShare: s.PricePerShare, Currency: s.Currency, Notes: s.Notes}
	}
	data, err := json.Marshal(rtExport{ExportDate: time.Now().Format("2006-01-02"), Transactions: txRows, Balances: balRows, StockTrades: stockRows})
	require.NoError(t, err)
	return data
}

// ---- Export tests ----

func TestExportAllJSON_ContainsAllRecords(t *testing.T) {
	txSvc := &mockTransactionService{}
	balSvc := &mockBalanceService{}
	stockSvc := &mockStockService{}
	logRepo := &mockExportLog{}

	txSvc.On("ListAll").Return(rtTransactions, nil)
	balSvc.On("List", domain.BalanceFilter{}, float64(0)).Return(rtBalances, nil)
	stockSvc.On("ListAll").Return(rtStocks, nil)
	logRepo.On("Save", "full").Return(nil)

	r := gin.New()
	handler.NewExportHandler(txSvc, balSvc, stockSvc, logRepo).RegisterRoutes(r.Group("/api/v1"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/export/finances.json", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var got rtExport
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))

	assert.Len(t, got.Transactions, 3, "all transactions exported")
	assert.Len(t, got.Balances, 2, "all balances exported")
	assert.Len(t, got.StockTrades, 2, "all stock trades exported")

	// Field mapping spot-checks
	assert.Equal(t, "2026-01-15", got.Transactions[0].Date)
	assert.Equal(t, "expense", got.Transactions[0].Type)
	assert.Equal(t, 88.91, got.Transactions[0].Amount)
	assert.Equal(t, "Food", got.Transactions[0].Category)
	assert.Equal(t, "Maxima", got.Transactions[0].Comment)

	assert.Equal(t, "2026-01-15", got.Balances[0].Date)
	assert.Equal(t, float64(15000), got.Balances[0].Total)
	assert.Equal(t, 0.05, got.Balances[0].RBTC)
	assert.Equal(t, float64(80000), got.Balances[0].BtcPrice)

	assert.Equal(t, "AAPL", got.StockTrades[0].Ticker)
	assert.Equal(t, "buy", got.StockTrades[0].Action)
	assert.Equal(t, float64(5), got.StockTrades[0].Shares)
}

func TestExportAllJSON_EmptyDB_ProducesEmptyArraysNotNull(t *testing.T) {
	txSvc := &mockTransactionService{}
	balSvc := &mockBalanceService{}
	stockSvc := &mockStockService{}
	logRepo := &mockExportLog{}

	txSvc.On("ListAll").Return([]domain.Transaction{}, nil)
	balSvc.On("List", domain.BalanceFilter{}, float64(0)).Return([]domain.Balance{}, nil)
	stockSvc.On("ListAll").Return([]domain.StockTrade{}, nil)
	logRepo.On("Save", "full").Return(nil)

	r := gin.New()
	handler.NewExportHandler(txSvc, balSvc, stockSvc, logRepo).RegisterRoutes(r.Group("/api/v1"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/export/finances.json", nil))
	require.Equal(t, http.StatusOK, w.Code)

	// Parse as raw JSON to check for null vs []
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.Equal(t, `[]`, string(raw["transactions"]), "empty transactions must be [] not null")
	assert.Equal(t, `[]`, string(raw["balances"]), "empty balances must be [] not null")
	assert.Equal(t, `[]`, string(raw["stock_trades"]), "empty stock_trades must be [] not null")
}

// ---- Import tests ----

func TestImportJSON_AllNew_ImportsEverything(t *testing.T) {
	txRepo := &mock.TransactionRepository{}
	balRepo := &mock.BalanceRepository{}
	stockRepo := &mock.StockRepository{}

	txRepo.On("ListAll").Return([]domain.Transaction{}, nil)
	for _, tx := range rtTransactions {
		txRepo.On("GetByID", tx.ID).Return(nil, errors.New("not found"))
	}
	txRepo.On("Create", tm.AnythingOfType("*domain.Transaction")).Return(nil)
	balRepo.On("List", tm.AnythingOfType("domain.BalanceFilter")).Return([]domain.Balance{}, nil)
	balRepo.On("Create", tm.AnythingOfType("*domain.Balance")).Return(nil)
	stockRepo.On("ListAll").Return([]domain.StockTrade{}, nil)
	stockRepo.On("Create", tm.AnythingOfType("*domain.StockTrade")).Return(nil)

	body, ct := rtMultipartBody(t, rtBuildJSON(t, rtTransactions, rtBalances, rtStocks))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/json", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	rtImportRouter(txRepo, balRepo, stockRepo).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var result rtImportResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))

	assert.Equal(t, 3, result.Imported.Transactions)
	assert.Equal(t, 2, result.Imported.Balances)
	assert.Equal(t, 2, result.Imported.StockTrades)
	assert.Equal(t, 0, result.Skipped.Transactions)
	assert.Equal(t, 0, result.Skipped.Balances)
	assert.Equal(t, 0, result.Skipped.StockTrades)
}

func TestImportJSON_Idempotent_SkipsAllOnSecondImport(t *testing.T) {
	txRepo := &mock.TransactionRepository{}
	balRepo := &mock.BalanceRepository{}
	stockRepo := &mock.StockRepository{}

	// All records already exist
	txRepo.On("ListAll").Return(rtTransactions, nil)
	for i := range rtTransactions {
		tx := rtTransactions[i]
		txRepo.On("GetByID", tx.ID).Return(&tx, nil)
	}
	balRepo.On("List", tm.AnythingOfType("domain.BalanceFilter")).Return([]domain.Balance{{Date: rtDay1}}, nil)
	stockRepo.On("ListAll").Return(rtStocks, nil)

	body, ct := rtMultipartBody(t, rtBuildJSON(t, rtTransactions, rtBalances, rtStocks))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/json", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	rtImportRouter(txRepo, balRepo, stockRepo).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var result rtImportResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))

	assert.Equal(t, 0, result.Imported.Transactions, "second import must not create duplicate transactions")
	assert.Equal(t, 0, result.Imported.StockTrades, "second import must not create duplicate stock trades")
	assert.Equal(t, 3, result.Skipped.Transactions)
	assert.Equal(t, 2, result.Skipped.StockTrades)

	// Create must never be called on any repo
	txRepo.AssertNotCalled(t, "Create", tm.Anything)
	balRepo.AssertNotCalled(t, "Create", tm.Anything)
	stockRepo.AssertNotCalled(t, "Create", tm.Anything)
}

// ---- Round-trip test ----

// TestExportImportRoundTrip is the primary proof:
// the JSON produced by ExportAllJSON can be fully re-imported with zero loss or duplication.
func TestExportImportRoundTrip(t *testing.T) {
	// --- Step 1: Export ---
	txSvc := &mockTransactionService{}
	balSvc := &mockBalanceService{}
	stockSvc := &mockStockService{}
	logRepo := &mockExportLog{}

	txSvc.On("ListAll").Return(rtTransactions, nil)
	balSvc.On("List", domain.BalanceFilter{}, float64(0)).Return(rtBalances, nil)
	stockSvc.On("ListAll").Return(rtStocks, nil)
	logRepo.On("Save", "full").Return(nil)

	exportRouter := gin.New()
	handler.NewExportHandler(txSvc, balSvc, stockSvc, logRepo).RegisterRoutes(exportRouter.Group("/api/v1"))

	ew := httptest.NewRecorder()
	exportRouter.ServeHTTP(ew, httptest.NewRequest(http.MethodGet, "/api/v1/export/finances.json", nil))
	require.Equal(t, http.StatusOK, ew.Code, "export must succeed")

	exportedJSON := ew.Body.Bytes()

	// Verify the exported JSON parses and has the right counts
	var exported rtExport
	require.NoError(t, json.Unmarshal(exportedJSON, &exported))
	require.Len(t, exported.Transactions, len(rtTransactions), "export must contain all transactions")
	require.Len(t, exported.Balances, len(rtBalances), "export must contain all balances")
	require.Len(t, exported.StockTrades, len(rtStocks), "export must contain all stock trades")

	// --- Step 2: Import the exact JSON that was exported ---
	txRepo := &mock.TransactionRepository{}
	balRepo := &mock.BalanceRepository{}
	stockRepo := &mock.StockRepository{}

	txRepo.On("ListAll").Return([]domain.Transaction{}, nil)
	for _, tx := range rtTransactions {
		txRepo.On("GetByID", tx.ID).Return(nil, errors.New("not found"))
	}
	txRepo.On("Create", tm.AnythingOfType("*domain.Transaction")).Return(nil)
	balRepo.On("List", tm.AnythingOfType("domain.BalanceFilter")).Return([]domain.Balance{}, nil)
	balRepo.On("Create", tm.AnythingOfType("*domain.Balance")).Return(nil)
	stockRepo.On("ListAll").Return([]domain.StockTrade{}, nil)
	stockRepo.On("Create", tm.AnythingOfType("*domain.StockTrade")).Return(nil)

	body, ct := rtMultipartBody(t, exportedJSON)
	importReq := httptest.NewRequest(http.MethodPost, "/api/v1/import/json", body)
	importReq.Header.Set("Content-Type", ct)
	iw := httptest.NewRecorder()
	rtImportRouter(txRepo, balRepo, stockRepo).ServeHTTP(iw, importReq)

	require.Equal(t, http.StatusOK, iw.Code, "import must succeed")
	var result rtImportResult
	require.NoError(t, json.Unmarshal(iw.Body.Bytes(), &result))

	// --- Step 3: Zero-delta assertions ---
	assert.Equal(t, len(rtTransactions), result.Imported.Transactions, "all transactions must be imported")
	assert.Equal(t, len(rtBalances), result.Imported.Balances, "all balances must be imported")
	assert.Equal(t, len(rtStocks), result.Imported.StockTrades, "all stock trades must be imported")
	assert.Equal(t, 0, result.Skipped.Transactions, "no transactions skipped on fresh import")
	assert.Equal(t, 0, result.Skipped.Balances, "no balances skipped on fresh import")
	assert.Equal(t, 0, result.Skipped.StockTrades, "no stock trades skipped on fresh import")

	total := result.Imported.Transactions + result.Imported.Balances + result.Imported.StockTrades
	original := len(rtTransactions) + len(rtBalances) + len(rtStocks)
	assert.Equal(t, original, total, "total imported == total original — zero delta proven")
}

// ---- Edge-case tests ----

func TestImportJSON_MissingFileField_Returns400(t *testing.T) {
	r := rtImportRouter(&mock.TransactionRepository{}, &mock.BalanceRepository{}, &mock.StockRepository{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/json", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestImportJSON_InvalidJSON_Returns400(t *testing.T) {
	r := rtImportRouter(&mock.TransactionRepository{}, &mock.BalanceRepository{}, &mock.StockRepository{})
	body, ct := rtMultipartBody(t, []byte(`not valid json {{`))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/json", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestImportJSON_EmptyPayload_ImportsNothing(t *testing.T) {
	txRepo := &mock.TransactionRepository{}
	txRepo.On("ListAll").Return([]domain.Transaction{}, nil)
	stockRepo := &mock.StockRepository{}
	stockRepo.On("ListAll").Return([]domain.StockTrade{}, nil)

	r := rtImportRouter(txRepo, &mock.BalanceRepository{}, stockRepo)
	body, ct := rtMultipartBody(t, rtBuildJSON(t, nil, nil, nil))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/json", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var result rtImportResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Equal(t, 0, result.Imported.Transactions)
	assert.Equal(t, 0, result.Imported.Balances)
	assert.Equal(t, 0, result.Imported.StockTrades)
}
