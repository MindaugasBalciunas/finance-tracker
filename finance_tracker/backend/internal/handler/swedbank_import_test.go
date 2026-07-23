package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/mindaugas/finance-tracker/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const swedFixture = `"Sąskaitos Nr.","","Data","Gavėjas","Paaiškinimai","Suma","Valiuta","D/K","Įrašo Nr.","Kodas","Įmokos kodas","Dok. Nr.",
"LT16","10","2022-01-01","","Likutis pradžiai","34639.41","EUR","K","","AS","","",
"LT16","20","2022-01-04","","Paslaugų plano ""Patogu"" programos ""Auksinė paslauga"" dalyviams mokestis 2021.12","0.70","EUR","D","1001","M","","",
"LT16","20","2022-01-06","Wolt 00180 Helsinki","PIRKINYS 516793******2950 2022.01.04 15.15 EUR (522658) Wolt 00180 Helsinki","15.15","EUR","D","1002","K","","",
"LT16","20","2022-01-09","UAB IGNITIS","E.Sąskaitos Nr. LDTESB-01464043 apmokėjimas","8.12","EUR","D","1003","MK","","SOP","",
"LT16","20","2022-01-13","VILNIAUS MIESTO SAVIVALDYBĖS ADMINISTRACIJA","Išmoka vaikui","70.00","EUR","K","1004","MK","","1841","",
"LT16","20","2022-01-14","MOBILEPAY A/S LITHUANIA BRANCH","TMP -įeinantis-  Pervedimas pagal darbo sutarti su MobilePay A/S 2022/01 men.","830.00","EUR","K","1005","MK","","",
"LT16","20","2022-01-29","EVELINA PLYTNIKAITĖ","Lizingas","300.00","EUR","D","1006","MK","","",
"LT16","20","2022-01-29","EVELINA BALČIŪNIENĖ","Vaiku darželis","210.00","EUR","D","1007","MK","","",
"LT16","20","2022-09-19","EVELINA BALČIŪNIENĖ","Namo paskola","1132.00","EUR","D","1008","MK","","",
"LT16","20","2022-05-30","'50146 LIDL SNIPISKES","PIRKINYS 516793******2950 2022.05.25 83.06 EUR (294126) 50146 LIDL SNIPISKES   08204 VILNIUS      ","83.06","EUR","D","1009","K","","",
"LT16","20","2022-08-26","","GRYNIEJI 516793******2950 26.08.22 09:22 600.00 EUR (465020) H836/HB LUKSIO G.23>VILNIUS LT","600.00","EUR","D","1010","K","","",
"LT16","20","2022-12-15","MADKLUBBEN VEST","PIRKINYS 516793******2950 2022.12.13 425.00 DKK VALIUTOS KURSAS 7.436570, VALIUTOS KEITIMO MOK 1.40 EUR (562023) MADKLUBBEN VEST        1620 Koebenhavn V ","58.55","EUR","D","1011","K","","",
"LT16","20","2023-11-27","Mindaugas BALCIUNAS","Transfer between my accounts","1000.00","EUR","K","1012","MK","","",
"LT16","20","2022-08-04","ARŪNAS KUGINYS","NT sandoris (Vienbutį gyvenamąjį namą)","26625.00","EUR","D","1013","MK","","12","",
"LT16","20","2023-02-07","","GRĄŽINIMAS 516793******2950 2023.02.06 8.96 EUR () WWW.NARYSTE.SVAROSBROLIAI","8.96","EUR","K","1014","K","","",
"LT16","20","2022-08-23","MOKI VEZI 06229 VILNIUS","PIRKINYS 516793******2950 2022.08.20 259.99 EUR (990886) MOKI VEZI 06229 VILNIUS","259.99","EUR","D","1015","K","","",
"LT16","20","2014-06-10","MAXIMA LT, X-096 VILNIUS","Pirkinys parduotuveje","34.53","LTL","D","1016","K","","",
"LT16","20","2015-01-01","","LTL -> EUR 4523.09 kursas 3.45280","15617.34","LTL","D","1017","K","","",
"LT16","20","2015-01-01","","LTL 15617.34 -> EUR kursas 3.45280","4523.09","EUR","K","1018","K","","",
"LT16","20","2012-01-11","","INESIMAS 6763769029222241 10.01.12 18:28 200.00 LTL (601019) H743/HB","200.00","LTL","K","1019","K","","",
"LT16","20","2018-01-31","DANSKE BANK A/S LIETUVOS FILIALAS","Danske Bank A/S. Saskaitos papildymas 2018/01 men.","830.00","EUR","K","1020","MK","","",
"LT16","20","2024-05-02","MINDAUGAS BALČIŪNAS","Pervedimas į Taupyklės sąskaitą po 5.9 EUR mokėjimo","5.90","EUR","D","1021","MK","","",
"LT16","20","2020-04-30","MINDAUGAS BALČIŪNAS","Kredito padengimas","325.45","EUR","D","1022","MK","","",
"LT16","20","2020-09-23","UAB TOKVILA","Pradine imoka uz automobili.  VIN JTMW53FV00D502936","3336.00","EUR","D","1023","MK","","",
"LT16","82","2024-01-01","","Apyvarta","163294.81","EUR","D","","K2","","",
"LT16","86","2024-01-01","","Likutis pabaigai","19104.63","EUR","K","","LS","","",
`

func swedImport(t *testing.T, r http.Handler, csvBody string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "statement.csv")
	require.NoError(t, err)
	_, err = fw.Write([]byte(csvBody))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/import/swedbank", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestSwedbankImport(t *testing.T) {
	r, db := swedTestRouter(t)

	// Rule so the re-apply pass has something to do.
	w := budgetDoJSON(r, "POST", "/api/v1/labels/apply", map[string]any{
		"label": "groceries", "comment_match": "lidl", "create_rule": true,
	})
	require.Equal(t, 200, w.Code)

	rec := swedImport(t, r, swedFixture)
	require.Equal(t, 200, rec.Code, rec.Body.String())
	var res map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	assert.EqualValues(t, 19, res["imported"], "all real rows imported")
	assert.EqualValues(t, 4, res["internal"], "own transfers + LTL conversion pair + taupyklė skipped")
	assert.EqualValues(t, 0, res["duplicate"])
	assert.EqualValues(t, 24, res["balances"], "one snapshot per statement month")
	assert.Equal(t, "2012-01-10", res["date_from"])

	get := func(comment string) domain.Transaction {
		var tx domain.Transaction
		require.NoError(t, db.Where("comment = ?", comment).First(&tx).Error, comment)
		return tx
	}

	// Fees
	fee := get("Swedbank plan fee")
	assert.Equal(t, "Finance", string(fee.Category))
	assert.Contains(t, fee.Labels, "fees")

	// Card purchase re-dated to purchase date
	wolt := get("Wolt 00180 Helsinki")
	assert.Equal(t, "2022-01-04", wolt.Date.Format("2006-01-02"))
	assert.Equal(t, "Food", string(wolt.Category))

	// Salary / benefit / utilities
	assert.Equal(t, "Salary", string(get("MOBILEPAY A/S LITHUANIA BRANCH").Category))
	assert.Equal(t, "Reimbursement", string(get("Child benefit (išmoka vaikui)").Category))
	assert.Equal(t, "Utilities", string(get("UAB IGNITIS").Category))

	// Evelina case
	leasing := get("EVELINA PLYTNIKAITĖ (Lizingas)")
	assert.Equal(t, domain.TransactionTypeInvestment, leasing.Type)
	assert.Equal(t, "Vehicle", string(leasing.Category))
	assert.Contains(t, leasing.Labels, "leasing")
	assert.Contains(t, leasing.Labels, "evelina")
	loan := get("EVELINA BALČIŪNIENĖ (Būsto paskola)")
	assert.Equal(t, "Finance", string(loan.Category))
	assert.Contains(t, loan.Labels, "loan")
	kids := get("EVELINA BALČIŪNIENĖ (Vaiku darželis)")
	assert.Equal(t, "Kids - General", string(kids.Category))

	// Groceries labeled by rules
	lidl := get("'50146 LIDL SNIPISKES")
	assert.Equal(t, "Food", string(lidl.Category))
	assert.Contains(t, lidl.Labels, "groceries")

	// Cash withdrawal → own transfer to cash
	cash := get("Cash withdrawal (ATM)")
	assert.Equal(t, domain.TransactionTypeInvestment, cash.Type)
	assert.Equal(t, "cash", cash.CreditAccount)
	assert.Equal(t, "2022-08-26", cash.Date.Format("2006-01-02"))

	// Foreign-currency purchase → Vacation
	assert.Equal(t, "Vacation", string(get("MADKLUBBEN VEST").Category))

	// House purchase → Real Estate investment
	house := get("House purchase — Platiniškių 21A (NT sandoris, ARŪNAS KUGINYS)")
	assert.Equal(t, domain.TransactionTypeInvestment, house.Type)
	assert.Equal(t, "Real Estate", string(house.Category))

	// Refund → income
	refund := get("Card refund (Swedbank)")
	assert.Equal(t, domain.TransactionTypeIncome, refund.Type)

	// DIY chain
	moki := get("Moki Veži")
	assert.Equal(t, "Housing", string(moki.Category))

	// LTL rows convert at the official 3.4528 rate.
	maxima := get("MAXIMA LT, X-096 VILNIUS")
	assert.InDelta(t, 10.00, maxima.Amount, 0.001, "34.53 LTL → 10.00 EUR")
	assert.Equal(t, "Food", string(maxima.Category))

	// ATM cash deposit → own transfer from the cash pocket, LTL converted.
	dep := get("Cash deposit (ATM)")
	assert.Equal(t, domain.TransactionTypeInvestment, dep.Type)
	assert.Equal(t, "cash", dep.DebitAccount)
	assert.InDelta(t, 57.92, dep.Amount, 0.01)
	assert.Equal(t, "2012-01-10", dep.Date.Format("2006-01-02"), "re-dated to deposit date")

	// Employers → Salary with an employer label.
	danske := get("DANSKE BANK A/S LIETUVOS FILIALAS")
	assert.Equal(t, "Salary", string(danske.Category))
	assert.Contains(t, danske.Labels, "danske")

	// Lithuanian credit repayments join the loan bucket.
	var creditRows []domain.Transaction
	require.NoError(t, db.Where("comment = 'Credit repayment'").Find(&creditRows).Error)
	assert.Len(t, creditRows, 1)
	assert.Contains(t, creditRows[0].Labels, "loan")

	// RAV4 down payment is car capital, not servicing.
	rav4 := get("RAV4 pradinė įmoka (Tokvila)")
	assert.Equal(t, domain.TransactionTypeInvestment, rav4.Type)
	assert.Equal(t, "Vehicle", string(rav4.Category))

	// Month-end balances replayed from the opening balance: 2022-01 through
	// 2023-12. First month = opening + all flows posted through 2022-01-31
	// (incl. the synthetic pre-2022 fixture rows); the last month carries
	// every flow except the 2024 Taupyklė sweep.
	var bals []domain.Balance
	require.NoError(t, db.Order("date").Find(&bals).Error)
	require.Len(t, bals, 24)
	assert.Equal(t, "2022-01-31", bals[0].Date.Format("2006-01-02"))
	assert.InDelta(t, 32221.91, bals[0].Swed, 0.005)
	assert.Equal(t, "2023-12-31", bals[23].Date.Format("2006-01-02"))
	assert.InDelta(t, 4472.27, bals[23].Swed, 0.005)

	// Idempotent: importing again skips everything, incl. balances.
	rec2 := swedImport(t, r, swedFixture)
	require.Equal(t, 200, rec2.Code)
	var res2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &res2))
	assert.EqualValues(t, 0, res2["imported"])
	assert.EqualValues(t, 19, res2["duplicate"])
	assert.EqualValues(t, 0, res2["balances"], "monthly snapshots dedup on re-import")
}

// swedTestRouter is budgetTestRouter plus the import routes: transactions,
// budgets/labels and the Swedbank importer all against one in-memory DB.
func swedTestRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.Budget{}, &domain.LabelRule{},
		&domain.BudgetSettings{}, &domain.Balance{}, &domain.StockTrade{}, &domain.Asset{}))

	budgetRepo := repository.NewBudgetRepository(db)
	txRepo := repository.NewTransactionRepository(db)
	txSvc := service.NewTransactionServiceWithRules(txRepo, nil, budgetRepo)

	r := gin.New()
	v1 := r.Group("/api/v1")
	NewBudgetHandler(budgetRepo).RegisterRoutes(v1)
	NewTransactionHandler(txSvc).RegisterRoutes(v1)
	NewImportHandler(txRepo, repository.NewBalanceRepository(db), repository.NewStockRepository(db),
		repository.NewAssetRepository(db)).WithBudgets(budgetRepo).RegisterRoutes(v1)
	return r, db
}
