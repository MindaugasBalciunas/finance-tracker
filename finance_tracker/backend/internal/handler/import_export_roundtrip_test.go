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
	"github.com/stretchr/testify/assert"
	tm "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ---- local mirror of the unexported financeExport struct ----
// Mirrors the JSON produced by ExportAllJSON exactly, so the test
// can both parse export output and construct import payloads.

type rtExport struct {
	ExportDate   string      `json:"export_date"`
	Transactions []rtTx      `json:"transactions"`
	Balances     []rtBalance `json:"balances"`
	StockTrades  []rtStock   `json:"stock_trades"`
	Assets       []rtAsset   `json:"assets"`
}

type rtTx struct {
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

type rtBalance struct {
	ID        uint    `json:"id,omitempty"`
	Date      string  `json:"date"`
	Time      string  `json:"time,omitempty"`
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

type rtAsset struct {
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

type rtImportResult struct {
	Imported rtCounts `json:"imported"`
	Skipped  rtCounts `json:"skipped"`
}
type rtCounts struct {
	Transactions int `json:"transactions"`
	Balances     int `json:"balances"`
	StockTrades  int `json:"stock_trades"`
	Assets       int `json:"assets"`
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

func rtDatePtr(y int, m time.Month, d int) *time.Time {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &t
}

var (
	rtDay1 = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	rtDay2 = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	rtDay3 = time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)

	rtTransactions = []domain.Transaction{
		{ID: 1, Date: rtDay1, Type: domain.TransactionTypeExpense, Amount: 88.91, Category: domain.CategoryFood, Comment: "Maxima", DebitAccount: "swed"},
		{ID: 2, Date: rtDay2, Type: domain.TransactionTypeIncome, Amount: 3500.00, Category: domain.CategorySalary, CreditAccount: "seb"},
		{ID: 3, Date: rtDay3, Type: domain.TransactionTypeInvestment, Amount: 200.00, Category: domain.CategoryStocksETF, Comment: "ETF buy", DebitAccount: "swed", CreditAccount: "swed_etf"},
	}

	rtBalances = []domain.Balance{
		{Date: rtDay1, Total: 15000, Seb: 5000, Swed: 3000, Cash: 500, RBTC: 0.05, BtcPrice: 80000},
		{Date: rtDay2, Total: 18200, Seb: 5500, Swed: 3200, RevStocks: 1500},
	}

	rtStocks = []domain.StockTrade{
		{Date: rtDay1, Action: domain.StockActionBuy, Ticker: "AAPL", Shares: 5, PricePerShare: 210.50, Currency: "USD"},
		{Date: rtDay2, Action: domain.StockActionBuy, Ticker: "MSFT", Shares: 2, PricePerShare: 415.00, Currency: "USD", Notes: "DCA"},
	}

	rtAssets = []domain.Asset{
		{
			Name: "Toyota RAV4 Style Hybrid", Type: domain.AssetTypeVehicle,
			PurchaseDate: rtDatePtr(2020, 11, 17), PurchasePrice: 34000, CurrentValue: 34000,
			LoanPaidOffDate: rtDatePtr(2025, 11, 17), Notes: "Bought new. Fully paid on 2025-11-17.",
		},
		{
			Name: "House Platiniškių 21A, Platiniškių k.", Type: domain.AssetTypeRealEstate,
			PurchaseDate: rtDatePtr(2022, 8, 17), PurchasePrice: 355000,
			CurrentValue: 410000, ValuationDate: rtDatePtr(2025, 12, 1),
			LoanRemaining: 263568.67, LoanRemainingDate: rtDatePtr(2026, 7, 20),
			LoanRate: "6M EURIBOR + 1.3%", LoanAccount: "seb",
			Notes: "Energy class B, built 2017.",
		},
		{
			Name: "Solar panels 10.35 kWp (SOLAX X3 Hybrid G4)", Type: domain.AssetTypeSolar,
			PurchasePrice: 4551.50, CurrentValue: 4551.50,
			Notes: "Gross €7,100.84 − €2,549.34 subsidy.",
		},
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

func rtImportRouter(txRepo *mock.TransactionRepository, balRepo *mock.BalanceRepository, stockRepo *mock.StockRepository, assetRepo *mock.AssetRepository) *gin.Engine {
	r := gin.New()
	handler.NewImportHandler(txRepo, balRepo, stockRepo, assetRepo).RegisterRoutes(r.Group("/api/v1"))
	return r
}

func rtOptDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

func rtAssetRows(assets []domain.Asset) []rtAsset {
	rows := make([]rtAsset, len(assets))
	for i, a := range assets {
		rows[i] = rtAsset{
			Name: a.Name, Type: string(a.Type),
			PurchaseDate: rtOptDate(a.PurchaseDate), PurchasePrice: a.PurchasePrice,
			CurrentValue: a.CurrentValue, ValuationDate: rtOptDate(a.ValuationDate),
			Notes:         a.Notes,
			LoanRemaining: a.LoanRemaining, LoanRemainingDate: rtOptDate(a.LoanRemainingDate),
			LoanRate: a.LoanRate, LoanAccount: a.LoanAccount, LoanPaidOffDate: rtOptDate(a.LoanPaidOffDate),
		}
	}
	return rows
}

// rtBuildJSON constructs a finances.json payload from raw domain objects,
// mirroring exactly what ExportAllJSON serialises.
func rtBuildJSON(t *testing.T, txs []domain.Transaction, bals []domain.Balance, stocks []domain.StockTrade, assets []domain.Asset) []byte {
	t.Helper()
	txRows := make([]rtTx, len(txs))
	for i, tx := range txs {
		txRows[i] = rtTx{ID: tx.ID, Date: tx.Date.Format("2006-01-02"), Type: string(tx.Type), Amount: tx.Amount, Category: string(tx.Category), Comment: tx.Comment, DebitAccount: tx.DebitAccount, CreditAccount: tx.CreditAccount, SourceAccount: tx.SourceAccount}
	}
	balRows := make([]rtBalance, len(bals))
	for i, b := range bals {
		balRows[i] = rtBalance{Date: b.Date.Format("2006-01-02"), Total: b.Total, Seb: b.Seb, Swed: b.Swed, SwedETF: b.SwedETF, SebPen: b.SebPen, Luminor: b.Luminor, Art: b.Art, Cash: b.Cash, RevM: b.RevM, RevR: b.RevR, RBTC: b.RBTC, MBTC: b.MBTC, BtcPrice: b.BtcPrice, RevStocks: b.RevStocks}
	}
	stockRows := make([]rtStock, len(stocks))
	for i, s := range stocks {
		stockRows[i] = rtStock{Date: s.Date.Format("2006-01-02"), Action: string(s.Action), Ticker: s.Ticker, Shares: s.Shares, PricePerShare: s.PricePerShare, Currency: s.Currency, Notes: s.Notes}
	}
	data, err := json.Marshal(rtExport{ExportDate: time.Now().Format("2006-01-02"), Transactions: txRows, Balances: balRows, StockTrades: stockRows, Assets: rtAssetRows(assets)})
	require.NoError(t, err)
	return data
}

// rtExportRouter wires an ExportHandler with all four mock services pre-loaded
// with the canonical fixtures.
func rtExportRouter(txs []domain.Transaction, bals []domain.Balance, stocks []domain.StockTrade, assets []domain.Asset) *gin.Engine {
	txSvc := &mockTransactionService{}
	balSvc := &mockBalanceService{}
	stockSvc := &mockStockService{}
	assetSvc := &mockAssetService{}
	logRepo := &mockExportLog{}

	txSvc.On("ListAll").Return(txs, nil)
	balSvc.On("List", domain.BalanceFilter{}, float64(0)).Return(bals, nil)
	stockSvc.On("ListAll").Return(stocks, nil)
	assetSvc.On("ListAll").Return(assets, nil)
	logRepo.On("Save", "full").Return(nil)

	r := gin.New()
	handler.NewExportHandler(txSvc, balSvc, stockSvc, assetSvc, logRepo).RegisterRoutes(r.Group("/api/v1"))
	return r
}

// ---- Export tests ----

func TestExportAllJSON_ContainsAllRecords(t *testing.T) {
	r := rtExportRouter(rtTransactions, rtBalances, rtStocks, rtAssets)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/export/finances.json", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var got rtExport
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))

	assert.Len(t, got.Transactions, 3, "all transactions exported")
	assert.Len(t, got.Balances, 2, "all balances exported")
	assert.Len(t, got.StockTrades, 2, "all stock trades exported")
	assert.Len(t, got.Assets, 3, "all assets exported")

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

	// Asset field mapping — car
	assert.Equal(t, "Toyota RAV4 Style Hybrid", got.Assets[0].Name)
	assert.Equal(t, "vehicle", got.Assets[0].Type)
	assert.Equal(t, "2020-11-17", got.Assets[0].PurchaseDate)
	assert.Equal(t, 34000.0, got.Assets[0].PurchasePrice)
	assert.Equal(t, "2025-11-17", got.Assets[0].LoanPaidOffDate)

	// Asset field mapping — house with mortgage
	house := got.Assets[1]
	assert.Equal(t, "real_estate", house.Type)
	assert.Equal(t, "2022-08-17", house.PurchaseDate)
	assert.Equal(t, 355000.0, house.PurchasePrice)
	assert.Equal(t, 410000.0, house.CurrentValue)
	assert.Equal(t, "2025-12-01", house.ValuationDate)
	assert.Equal(t, 263568.67, house.LoanRemaining)
	assert.Equal(t, "2026-07-20", house.LoanRemainingDate)
	assert.Equal(t, "6M EURIBOR + 1.3%", house.LoanRate)
	assert.Equal(t, "seb", house.LoanAccount)

	// Asset field mapping — solar without purchase date
	assert.Equal(t, "solar", got.Assets[2].Type)
	assert.Equal(t, "", got.Assets[2].PurchaseDate, "missing purchase date exported as empty")
	assert.Equal(t, 4551.50, got.Assets[2].PurchasePrice)
}

// TestExportAllJSON_ChronologicalOrder verifies the export sorts transactions
// and balances oldest-first even when the repositories return newest-first
// (the UI listing order).
func TestExportAllJSON_ChronologicalOrder(t *testing.T) {
	reversedTx := make([]domain.Transaction, len(rtTransactions))
	for i, tx := range rtTransactions {
		reversedTx[len(rtTransactions)-1-i] = tx
	}
	reversedBal := make([]domain.Balance, len(rtBalances))
	for i, b := range rtBalances {
		reversedBal[len(rtBalances)-1-i] = b
	}

	r := rtExportRouter(reversedTx, reversedBal, []domain.StockTrade{}, []domain.Asset{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/export/finances.json", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var got rtExport
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))

	for i := 1; i < len(got.Transactions); i++ {
		assert.LessOrEqual(t, got.Transactions[i-1].Date, got.Transactions[i].Date,
			"transactions must be in ascending date order")
	}
	for i := 1; i < len(got.Balances); i++ {
		assert.LessOrEqual(t, got.Balances[i-1].Date, got.Balances[i].Date,
			"balances must be in ascending date order")
	}
	assert.Equal(t, "2026-01-15", got.Transactions[0].Date, "oldest transaction first")
	assert.Equal(t, "2026-01-15", got.Balances[0].Date, "oldest balance first")
}

func TestExportAllJSON_EmptyDB_ProducesEmptyArraysNotNull(t *testing.T) {
	r := rtExportRouter([]domain.Transaction{}, []domain.Balance{}, []domain.StockTrade{}, []domain.Asset{})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/export/finances.json", nil))
	require.Equal(t, http.StatusOK, w.Code)

	// Parse as raw JSON to check for null vs []
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.Equal(t, `[]`, string(raw["transactions"]), "empty transactions must be [] not null")
	assert.Equal(t, `[]`, string(raw["balances"]), "empty balances must be [] not null")
	assert.Equal(t, `[]`, string(raw["stock_trades"]), "empty stock_trades must be [] not null")
	assert.Equal(t, `[]`, string(raw["assets"]), "empty assets must be [] not null")
}

// ---- Import tests ----

func TestImportJSON_AllNew_ImportsEverything(t *testing.T) {
	txRepo := &mock.TransactionRepository{}
	balRepo := &mock.BalanceRepository{}
	stockRepo := &mock.StockRepository{}
	assetRepo := &mock.AssetRepository{}

	txRepo.On("ListAll").Return([]domain.Transaction{}, nil)
	for _, tx := range rtTransactions {
		txRepo.On("GetByID", tx.ID).Return(nil, errors.New("not found"))
	}
	txRepo.On("Create", tm.AnythingOfType("*domain.Transaction")).Return(nil)
	balRepo.On("List", tm.AnythingOfType("domain.BalanceFilter")).Return([]domain.Balance{}, nil)
	balRepo.On("Create", tm.AnythingOfType("*domain.Balance")).Return(nil)
	stockRepo.On("ListAll").Return([]domain.StockTrade{}, nil)
	stockRepo.On("Create", tm.AnythingOfType("*domain.StockTrade")).Return(nil)
	assetRepo.On("ListAll").Return([]domain.Asset{}, nil)
	assetRepo.On("Create", tm.AnythingOfType("*domain.Asset")).Return(nil)

	body, ct := rtMultipartBody(t, rtBuildJSON(t, rtTransactions, rtBalances, rtStocks, rtAssets))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/json", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	rtImportRouter(txRepo, balRepo, stockRepo, assetRepo).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var result rtImportResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))

	assert.Equal(t, 3, result.Imported.Transactions)
	assert.Equal(t, 2, result.Imported.Balances)
	assert.Equal(t, 2, result.Imported.StockTrades)
	assert.Equal(t, 3, result.Imported.Assets)
	assert.Equal(t, 0, result.Skipped.Transactions)
	assert.Equal(t, 0, result.Skipped.Balances)
	assert.Equal(t, 0, result.Skipped.StockTrades)
	assert.Equal(t, 0, result.Skipped.Assets)
}

func TestImportJSON_Idempotent_SkipsAllOnSecondImport(t *testing.T) {
	txRepo := &mock.TransactionRepository{}
	balRepo := &mock.BalanceRepository{}
	stockRepo := &mock.StockRepository{}
	assetRepo := &mock.AssetRepository{}

	// All records already exist
	txRepo.On("ListAll").Return(rtTransactions, nil)
	for i := range rtTransactions {
		tx := rtTransactions[i]
		txRepo.On("GetByID", tx.ID).Return(&tx, nil)
	}
	balRepo.On("List", tm.AnythingOfType("domain.BalanceFilter")).Return([]domain.Balance{{Date: rtDay1}}, nil)
	stockRepo.On("ListAll").Return(rtStocks, nil)
	assetRepo.On("ListAll").Return(rtAssets, nil)

	body, ct := rtMultipartBody(t, rtBuildJSON(t, rtTransactions, rtBalances, rtStocks, rtAssets))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/json", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	rtImportRouter(txRepo, balRepo, stockRepo, assetRepo).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var result rtImportResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))

	assert.Equal(t, 0, result.Imported.Transactions, "second import must not create duplicate transactions")
	assert.Equal(t, 0, result.Imported.StockTrades, "second import must not create duplicate stock trades")
	assert.Equal(t, 0, result.Imported.Assets, "second import must not create duplicate assets")
	assert.Equal(t, 3, result.Skipped.Transactions)
	assert.Equal(t, 2, result.Skipped.StockTrades)
	assert.Equal(t, 3, result.Skipped.Assets)

	// Create must never be called on any repo
	txRepo.AssertNotCalled(t, "Create", tm.Anything)
	balRepo.AssertNotCalled(t, "Create", tm.Anything)
	stockRepo.AssertNotCalled(t, "Create", tm.Anything)
	assetRepo.AssertNotCalled(t, "Create", tm.Anything)
}

// ---- Round-trip test ----

// TestExportImportRoundTrip is the primary proof:
// the JSON produced by ExportAllJSON can be fully re-imported with zero loss or duplication.
func TestExportImportRoundTrip(t *testing.T) {
	// --- Step 1: Export ---
	exportRouter := rtExportRouter(rtTransactions, rtBalances, rtStocks, rtAssets)

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
	require.Len(t, exported.Assets, len(rtAssets), "export must contain all assets")

	// --- Step 2: Import the exact JSON that was exported ---
	txRepo := &mock.TransactionRepository{}
	balRepo := &mock.BalanceRepository{}
	stockRepo := &mock.StockRepository{}
	assetRepo := &mock.AssetRepository{}

	txRepo.On("ListAll").Return([]domain.Transaction{}, nil)
	for _, tx := range rtTransactions {
		txRepo.On("GetByID", tx.ID).Return(nil, errors.New("not found"))
	}
	txRepo.On("Create", tm.AnythingOfType("*domain.Transaction")).Return(nil)
	balRepo.On("List", tm.AnythingOfType("domain.BalanceFilter")).Return([]domain.Balance{}, nil)
	balRepo.On("Create", tm.AnythingOfType("*domain.Balance")).Return(nil)
	stockRepo.On("ListAll").Return([]domain.StockTrade{}, nil)
	stockRepo.On("Create", tm.AnythingOfType("*domain.StockTrade")).Return(nil)
	assetRepo.On("ListAll").Return([]domain.Asset{}, nil)
	assetRepo.On("Create", tm.AnythingOfType("*domain.Asset")).Return(nil)

	body, ct := rtMultipartBody(t, exportedJSON)
	importReq := httptest.NewRequest(http.MethodPost, "/api/v1/import/json", body)
	importReq.Header.Set("Content-Type", ct)
	iw := httptest.NewRecorder()
	rtImportRouter(txRepo, balRepo, stockRepo, assetRepo).ServeHTTP(iw, importReq)

	require.Equal(t, http.StatusOK, iw.Code, "import must succeed")
	var result rtImportResult
	require.NoError(t, json.Unmarshal(iw.Body.Bytes(), &result))

	// --- Step 3: Zero-delta assertions ---
	assert.Equal(t, len(rtTransactions), result.Imported.Transactions, "all transactions must be imported")
	assert.Equal(t, len(rtBalances), result.Imported.Balances, "all balances must be imported")
	assert.Equal(t, len(rtStocks), result.Imported.StockTrades, "all stock trades must be imported")
	assert.Equal(t, len(rtAssets), result.Imported.Assets, "all assets must be imported")
	assert.Equal(t, 0, result.Skipped.Transactions, "no transactions skipped on fresh import")
	assert.Equal(t, 0, result.Skipped.Balances, "no balances skipped on fresh import")
	assert.Equal(t, 0, result.Skipped.StockTrades, "no stock trades skipped on fresh import")
	assert.Equal(t, 0, result.Skipped.Assets, "no assets skipped on fresh import")

	total := result.Imported.Transactions + result.Imported.Balances + result.Imported.StockTrades + result.Imported.Assets
	original := len(rtTransactions) + len(rtBalances) + len(rtStocks) + len(rtAssets)
	assert.Equal(t, original, total, "total imported == total original — zero delta proven")
}

// TestExportImportRoundTrip_AssetFieldsPreserved verifies every asset field —
// including loan terms and nullable dates — survives a full export → import round-trip.
func TestExportImportRoundTrip_AssetFieldsPreserved(t *testing.T) {
	// --- Export ---
	exportRouter := rtExportRouter([]domain.Transaction{}, []domain.Balance{}, []domain.StockTrade{}, rtAssets)

	ew := httptest.NewRecorder()
	exportRouter.ServeHTTP(ew, httptest.NewRequest(http.MethodGet, "/api/v1/export/finances.json", nil))
	require.Equal(t, http.StatusOK, ew.Code)

	// --- Import the exported JSON and capture what was created ---
	txRepo := &mock.TransactionRepository{}
	balRepo := &mock.BalanceRepository{}
	stockRepo := &mock.StockRepository{}
	assetRepo := &mock.AssetRepository{}

	txRepo.On("ListAll").Return([]domain.Transaction{}, nil)
	stockRepo.On("ListAll").Return([]domain.StockTrade{}, nil)
	assetRepo.On("ListAll").Return([]domain.Asset{}, nil)

	var created []*domain.Asset
	assetRepo.On("Create", tm.AnythingOfType("*domain.Asset")).
		Run(func(args tm.Arguments) { created = append(created, args.Get(0).(*domain.Asset)) }).
		Return(nil)

	body, ct := rtMultipartBody(t, ew.Body.Bytes())
	importReq := httptest.NewRequest(http.MethodPost, "/api/v1/import/json", body)
	importReq.Header.Set("Content-Type", ct)
	iw := httptest.NewRecorder()
	rtImportRouter(txRepo, balRepo, stockRepo, assetRepo).ServeHTTP(iw, importReq)

	require.Equal(t, http.StatusOK, iw.Code)
	require.Len(t, created, 3, "all three assets imported")

	car, house, solar := created[0], created[1], created[2]

	assert.Equal(t, "Toyota RAV4 Style Hybrid", car.Name)
	assert.Equal(t, domain.AssetTypeVehicle, car.Type)
	require.NotNil(t, car.PurchaseDate)
	assert.Equal(t, "2020-11-17", car.PurchaseDate.Format("2006-01-02"))
	assert.Equal(t, 34000.0, car.PurchasePrice)
	require.NotNil(t, car.LoanPaidOffDate)
	assert.Equal(t, "2025-11-17", car.LoanPaidOffDate.Format("2006-01-02"))
	assert.Equal(t, 0.0, car.LoanRemaining)

	assert.Equal(t, domain.AssetTypeRealEstate, house.Type)
	assert.Equal(t, 355000.0, house.PurchasePrice)
	assert.Equal(t, 410000.0, house.CurrentValue)
	require.NotNil(t, house.ValuationDate)
	assert.Equal(t, "2025-12-01", house.ValuationDate.Format("2006-01-02"))
	assert.Equal(t, 263568.67, house.LoanRemaining)
	require.NotNil(t, house.LoanRemainingDate)
	assert.Equal(t, "2026-07-20", house.LoanRemainingDate.Format("2006-01-02"))
	assert.Equal(t, "6M EURIBOR + 1.3%", house.LoanRate)
	assert.Equal(t, "seb", house.LoanAccount)

	assert.Equal(t, domain.AssetTypeSolar, solar.Type)
	assert.Nil(t, solar.PurchaseDate, "missing purchase date stays nil after round-trip")
	assert.Equal(t, 4551.50, solar.PurchasePrice)
	assert.Equal(t, 4551.50, solar.CurrentValue)
}

// ---- Edge-case tests ----

func TestImportJSON_MissingFileField_Returns400(t *testing.T) {
	r := rtImportRouter(&mock.TransactionRepository{}, &mock.BalanceRepository{}, &mock.StockRepository{}, &mock.AssetRepository{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/json", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestImportJSON_InvalidJSON_Returns400(t *testing.T) {
	r := rtImportRouter(&mock.TransactionRepository{}, &mock.BalanceRepository{}, &mock.StockRepository{}, &mock.AssetRepository{})
	body, ct := rtMultipartBody(t, []byte(`not valid json {{`))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/json", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestExportImportRoundTrip_AccountFieldsPreserved verifies that debit_account,
// credit_account, and source_account survive a full export → import round-trip.
func TestExportImportRoundTrip_AccountFieldsPreserved(t *testing.T) {
	// --- Export ---
	exportRouter := rtExportRouter(rtTransactions, []domain.Balance{}, []domain.StockTrade{}, []domain.Asset{})

	ew := httptest.NewRecorder()
	exportRouter.ServeHTTP(ew, httptest.NewRequest(http.MethodGet, "/api/v1/export/finances.json", nil))
	require.Equal(t, http.StatusOK, ew.Code)

	var exported rtExport
	require.NoError(t, json.Unmarshal(ew.Body.Bytes(), &exported))

	// Spot-check account fields in the raw JSON export
	assert.Equal(t, "swed", exported.Transactions[0].DebitAccount, "expense debit_account exported")
	assert.Equal(t, "seb", exported.Transactions[1].CreditAccount, "income credit_account exported")
	assert.Equal(t, "swed", exported.Transactions[2].DebitAccount, "investment debit_account exported")
	assert.Equal(t, "swed_etf", exported.Transactions[2].CreditAccount, "investment credit_account exported")

	// --- Import the exported JSON and capture what was created ---
	txRepo := &mock.TransactionRepository{}
	balRepo := &mock.BalanceRepository{}
	stockRepo := &mock.StockRepository{}
	assetRepo := &mock.AssetRepository{}

	txRepo.On("ListAll").Return([]domain.Transaction{}, nil)
	for _, tx := range rtTransactions {
		txRepo.On("GetByID", tx.ID).Return(nil, errors.New("not found"))
	}

	var created []*domain.Transaction
	txRepo.On("Create", tm.AnythingOfType("*domain.Transaction")).
		Run(func(args tm.Arguments) { created = append(created, args.Get(0).(*domain.Transaction)) }).
		Return(nil)
	balRepo.On("List", tm.AnythingOfType("domain.BalanceFilter")).Return([]domain.Balance{}, nil)
	stockRepo.On("ListAll").Return([]domain.StockTrade{}, nil)
	assetRepo.On("ListAll").Return([]domain.Asset{}, nil)

	body, ct := rtMultipartBody(t, ew.Body.Bytes())
	importReq := httptest.NewRequest(http.MethodPost, "/api/v1/import/json", body)
	importReq.Header.Set("Content-Type", ct)
	iw := httptest.NewRecorder()
	rtImportRouter(txRepo, balRepo, stockRepo, assetRepo).ServeHTTP(iw, importReq)

	require.Equal(t, http.StatusOK, iw.Code)

	require.Len(t, created, 3, "all three transactions imported")
	assert.Equal(t, "swed", created[0].DebitAccount, "expense debit_account round-tripped")
	assert.Equal(t, "", created[0].CreditAccount, "expense credit_account empty")
	assert.Equal(t, "seb", created[1].CreditAccount, "income credit_account round-tripped")
	assert.Equal(t, "", created[1].DebitAccount, "income debit_account empty")
	assert.Equal(t, "swed", created[2].DebitAccount, "investment debit_account round-tripped")
	assert.Equal(t, "swed_etf", created[2].CreditAccount, "investment credit_account round-tripped")
}

// TestExportImportRoundTrip_MultiSnapshotDay verifies that several balance
// snapshots taken on the same day (v1.0.68+ feature) survive a full
// export → import round-trip: the export carries id + time-of-day, and the
// import preserves both instead of collapsing the day to a single snapshot.
func TestExportImportRoundTrip_MultiSnapshotDay(t *testing.T) {
	morning := time.Date(2026, 7, 1, 9, 30, 0, 0, time.UTC)
	evening := time.Date(2026, 7, 1, 21, 15, 45, 0, time.UTC)
	snapshots := []domain.Balance{
		{ID: 41, Date: morning, Total: 100000, Seb: 60000},
		{ID: 42, Date: evening, Total: 100500, Seb: 60500},
	}

	// --- Export ---
	exportRouter := rtExportRouter([]domain.Transaction{}, snapshots, []domain.StockTrade{}, []domain.Asset{})
	ew := httptest.NewRecorder()
	exportRouter.ServeHTTP(ew, httptest.NewRequest(http.MethodGet, "/api/v1/export/finances.json", nil))
	require.Equal(t, http.StatusOK, ew.Code)

	var exported rtExport
	require.NoError(t, json.Unmarshal(ew.Body.Bytes(), &exported))
	require.Len(t, exported.Balances, 2)
	assert.Equal(t, uint(41), exported.Balances[0].ID, "snapshot id exported")
	assert.Equal(t, "2026-07-01", exported.Balances[0].Date)
	assert.Equal(t, "09:30:00", exported.Balances[0].Time, "time-of-day exported")
	assert.Equal(t, "21:15:45", exported.Balances[1].Time)

	// --- Import into an empty DB ---
	txRepo := &mock.TransactionRepository{}
	balRepo := &mock.BalanceRepository{}
	stockRepo := &mock.StockRepository{}
	assetRepo := &mock.AssetRepository{}

	txRepo.On("ListAll").Return([]domain.Transaction{}, nil)
	stockRepo.On("ListAll").Return([]domain.StockTrade{}, nil)
	assetRepo.On("ListAll").Return([]domain.Asset{}, nil)
	balRepo.On("GetByID", uint(41)).Return(nil, errors.New("not found"))
	balRepo.On("GetByID", uint(42)).Return(nil, errors.New("not found"))
	var created []*domain.Balance
	balRepo.On("Create", tm.AnythingOfType("*domain.Balance")).
		Run(func(args tm.Arguments) { created = append(created, args.Get(0).(*domain.Balance)) }).
		Return(nil)

	body, ct := rtMultipartBody(t, ew.Body.Bytes())
	importReq := httptest.NewRequest(http.MethodPost, "/api/v1/import/json", body)
	importReq.Header.Set("Content-Type", ct)
	iw := httptest.NewRecorder()
	rtImportRouter(txRepo, balRepo, stockRepo, assetRepo).ServeHTTP(iw, importReq)
	require.Equal(t, http.StatusOK, iw.Code)

	require.Len(t, created, 2, "both same-day snapshots imported")
	assert.Equal(t, uint(41), created[0].ID, "snapshot id preserved")
	assert.Equal(t, "2026-07-01 09:30:00", created[0].Date.Format("2006-01-02 15:04:05"), "timestamp preserved")
	assert.Equal(t, "2026-07-01 21:15:45", created[1].Date.Format("2006-01-02 15:04:05"))
	// Day-based dedup must not have been consulted for rows carrying an ID.
	balRepo.AssertNotCalled(t, "List", tm.Anything)

	// --- Re-import: both snapshots now exist → skipped, nothing created ---
	balRepo2 := &mock.BalanceRepository{}
	balRepo2.On("GetByID", uint(41)).Return(&snapshots[0], nil)
	balRepo2.On("GetByID", uint(42)).Return(&snapshots[1], nil)
	txRepo2 := &mock.TransactionRepository{}
	txRepo2.On("ListAll").Return([]domain.Transaction{}, nil)
	stockRepo2 := &mock.StockRepository{}
	stockRepo2.On("ListAll").Return([]domain.StockTrade{}, nil)
	assetRepo2 := &mock.AssetRepository{}
	assetRepo2.On("ListAll").Return([]domain.Asset{}, nil)

	body2, ct2 := rtMultipartBody(t, ew.Body.Bytes())
	reReq := httptest.NewRequest(http.MethodPost, "/api/v1/import/json", body2)
	reReq.Header.Set("Content-Type", ct2)
	iw2 := httptest.NewRecorder()
	rtImportRouter(txRepo2, balRepo2, stockRepo2, assetRepo2).ServeHTTP(iw2, reReq)
	require.Equal(t, http.StatusOK, iw2.Code)

	var result rtImportResult
	require.NoError(t, json.Unmarshal(iw2.Body.Bytes(), &result))
	assert.Equal(t, 0, result.Imported.Balances)
	assert.Equal(t, 2, result.Skipped.Balances)
	balRepo2.AssertNotCalled(t, "Create", tm.Anything)
}

// TestExportJSON_RangeFilter verifies ?from/?to limit the export to the
// requested period and that a range export is NOT recorded as a full backup.
func TestExportJSON_RangeFilter(t *testing.T) {
	txSvc := &mockTransactionService{}
	balSvc := &mockBalanceService{}
	stockSvc := &mockStockService{}
	assetSvc := &mockAssetService{}
	logRepo := &mockExportLog{}

	txSvc.On("ListAll").Return(rtTransactions, nil)
	balSvc.On("List", tm.AnythingOfType("domain.BalanceFilter"), float64(0)).Return(rtBalances, nil)
	stockSvc.On("ListAll").Return(rtStocks, nil)
	assetSvc.On("ListAll").Return(rtAssets, nil)

	r := gin.New()
	handler.NewExportHandler(txSvc, balSvc, stockSvc, assetSvc, logRepo).RegisterRoutes(r.Group("/api/v1"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/export/finances.json?from=2026-02-01&to=2026-02-28", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var got rtExport
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got.Transactions, 1, "only the February transaction exported")
	assert.Equal(t, "2026-02-01", got.Transactions[0].Date)
	require.Len(t, got.StockTrades, 1, "only the February trade exported")
	assert.Equal(t, "MSFT", got.StockTrades[0].Ticker)
	assert.Len(t, got.Assets, 3, "assets are not date-scoped")

	logRepo.AssertNotCalled(t, "Save", tm.Anything)

	// Invalid date → 400
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/api/v1/export/finances.json?from=02-01-2026", nil))
	assert.Equal(t, http.StatusBadRequest, w2.Code)
}

func TestImportJSON_EmptyPayload_ImportsNothing(t *testing.T) {
	txRepo := &mock.TransactionRepository{}
	txRepo.On("ListAll").Return([]domain.Transaction{}, nil)
	stockRepo := &mock.StockRepository{}
	stockRepo.On("ListAll").Return([]domain.StockTrade{}, nil)
	assetRepo := &mock.AssetRepository{}
	assetRepo.On("ListAll").Return([]domain.Asset{}, nil)

	r := rtImportRouter(txRepo, &mock.BalanceRepository{}, stockRepo, assetRepo)
	body, ct := rtMultipartBody(t, rtBuildJSON(t, nil, nil, nil, nil))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/json", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// An empty payload with no schema version is indistinguishable from a
	// wrong file (any JSON object parses into an all-empty financeExport) —
	// it must be REJECTED, not reported as a successful zero-row restore.
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "no recognized sections")
}

// ---- purpose=export: an AI/analysis download must never masquerade as a backup ----

func TestExportAllJSON_PurposeExport_DoesNotBumpBackupMarker(t *testing.T) {
	txSvc := &mockTransactionService{}
	balSvc := &mockBalanceService{}
	stockSvc := &mockStockService{}
	assetSvc := &mockAssetService{}
	// No Save expectation registered: the mock fails the test if the handler
	// treats this download as a full backup.
	logRepo := &mockExportLog{}

	txSvc.On("ListAll").Return(rtTransactions, nil)
	balSvc.On("List", domain.BalanceFilter{}, float64(0)).Return(rtBalances, nil)
	stockSvc.On("ListAll").Return(rtStocks, nil)
	assetSvc.On("ListAll").Return(rtAssets, nil)

	r := gin.New()
	handler.NewExportHandler(txSvc, balSvc, stockSvc, assetSvc, logRepo).RegisterRoutes(r.Group("/api/v1"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/finances.json?purpose=export", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	cd := w.Header().Get("Content-Disposition")
	assert.Contains(t, cd, "export_finances_all_")
	assert.NotContains(t, cd, "backup_")
	logRepo.AssertNotCalled(t, "Save", "full")

	var exp rtExport
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &exp))
	assert.Len(t, exp.Transactions, len(rtTransactions), "purpose=export must still contain everything")
}

func TestExportAllJSON_NoPurpose_IsFullBackup(t *testing.T) {
	txSvc := &mockTransactionService{}
	balSvc := &mockBalanceService{}
	stockSvc := &mockStockService{}
	assetSvc := &mockAssetService{}
	logRepo := &mockExportLog{}

	txSvc.On("ListAll").Return(rtTransactions, nil)
	balSvc.On("List", domain.BalanceFilter{}, float64(0)).Return(rtBalances, nil)
	stockSvc.On("ListAll").Return(rtStocks, nil)
	assetSvc.On("ListAll").Return(rtAssets, nil)
	logRepo.On("Save", "full").Return(nil)

	r := gin.New()
	handler.NewExportHandler(txSvc, balSvc, stockSvc, assetSvc, logRepo).RegisterRoutes(r.Group("/api/v1"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/finances.json", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Disposition"), "backup_finances_")
	logRepo.AssertCalled(t, "Save", "full")
}
