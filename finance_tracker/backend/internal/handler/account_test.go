package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

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

type accountEnv struct {
	r      *gin.Engine
	db     *gorm.DB
	balSvc service.BalanceService
}

func accountTestEnv(t *testing.T) *accountEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Balance{}, &domain.Account{}, &domain.Transaction{}))
	accRepo := repository.NewAccountRepository(db)
	for _, a := range domain.BuiltinAccounts {
		row := a
		row.Builtin = true
		require.NoError(t, accRepo.Create(&row))
	}
	balSvc := service.WithAccounts(service.NewBalanceService(repository.NewBalanceRepository(db), nil), accRepo)
	r := gin.New()
	v1 := r.Group("/api/v1")
	NewAccountHandler(accRepo).RegisterRoutes(v1)
	NewBalanceHandler(balSvc).RegisterRoutes(v1)
	return &accountEnv{r: r, db: db, balSvc: balSvc}
}

func (e *accountEnv) addAccount(t *testing.T, label, group string) domain.Account {
	t.Helper()
	rec := bankJSON(t, e.r, http.MethodPost, "/api/v1/accounts", map[string]any{"label": label, "group": group})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var a domain.Account
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &a))
	return a
}

func TestAccountKeyFromLabelIsStableAndUnique(t *testing.T) {
	e := accountTestEnv(t)
	a := e.addAccount(t, "Paysera taupymas", "cash")
	assert.Equal(t, "acc_paysera_taupymas", a.Key)
	b := e.addAccount(t, "Paysera taupymas", "cash")
	assert.Equal(t, "acc_paysera_taupymas_2", b.Key)
	c := e.addAccount(t, "Šiaulių bankas", "cash")
	assert.Equal(t, "acc_siauliu_bankas", c.Key)

	// Renaming keeps the key, so stored values stay attached.
	rec := bankJSON(t, e.r, http.MethodPut, "/api/v1/accounts/"+fmt.Sprint(a.ID), map[string]any{"label": "Paysera"})
	require.Equal(t, http.StatusOK, rec.Code)
	var got domain.Account
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "Paysera", got.Label)
	assert.Equal(t, a.Key, got.Key)
}

func TestBuiltinAccountsRenameAndRegroupButNeverArchive(t *testing.T) {
	e := accountTestEnv(t)
	var swed domain.Account
	require.NoError(t, e.db.Where("key = ?", "swed").First(&swed).Error)
	url := "/api/v1/accounts/" + fmt.Sprint(swed.ID)
	assert.Equal(t, http.StatusBadRequest, bankJSON(t, e.r, http.MethodPut, url, map[string]any{"archived": true}).Code)

	rec := bankJSON(t, e.r, http.MethodPut, url, map[string]any{"label": "Swedbank main", "group": "cash"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got domain.Account
	require.NoError(t, e.db.First(&got, swed.ID).Error)
	assert.Equal(t, "Swedbank main", got.Label)
	assert.Equal(t, "swed", got.Key, "the key never changes")

	require.NoError(t, e.db.Create(&domain.Balance{Date: time.Now(), Swed: 100, Seb: 50, Total: 150}).Error)
	alloc, err := e.balSvc.GetAllocation()
	require.NoError(t, err)
	names := map[string]float64{}
	for _, a := range alloc {
		names[a.Account] = a.Amount
	}
	assert.Equal(t, 100.0, names["Swedbank main"], "allocation uses the new name")
	assert.NotContains(t, names, "Swedbank")
	assert.Equal(t, 50.0, names["Seb"], "untouched accounts keep their wording")
}

func TestAddedAccountCountsInTotalsGroupsAndTrend(t *testing.T) {
	e := accountTestEnv(t)
	a := e.addAccount(t, "Paysera", "cash")

	rec := bankJSON(t, e.r, http.MethodPost, "/api/v1/balances", map[string]any{
		"date": "2026-10-01", "swed": 1000, "extra": map[string]float64{a.Key: 250.5, "not a key": 99},
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	latest, err := e.balSvc.GetLatest(0)
	require.NoError(t, err)
	assert.Equal(t, 1250.5, latest.Total)
	assert.Equal(t, domain.AccountValues{a.Key: 250.5}, latest.Extra, "malformed keys are dropped")
	assert.Equal(t, 250.5, latest.ExtraGroups["cash"])

	// A client that predates added accounts sends no extra: keep them.
	rec = bankJSON(t, e.r, http.MethodPut, "/api/v1/balances/"+fmt.Sprint(latest.ID), map[string]any{"date": "2026-10-01", "swed": 900})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	latest, err = e.balSvc.GetLatest(0)
	require.NoError(t, err)
	assert.Equal(t, 250.5, latest.Extra[a.Key])
	assert.Equal(t, 1150.5, latest.Total)

	trend, err := e.balSvc.GetTrend(domain.BalanceFilter{})
	require.NoError(t, err)
	assert.Equal(t, []float64{250.5}, trend.Accounts[a.Key])

	alloc, err := e.balSvc.GetAllocation()
	require.NoError(t, err)
	found := false
	for _, x := range alloc {
		if x.Account == "Paysera" {
			found = true
			assert.Equal(t, 250.5, x.Amount)
		}
	}
	assert.True(t, found, "allocation names the added account by label")
}

// A transaction or bank commit on an added account moves its value, and the
// clone never shares the map with the snapshot it came from.
func TestDeltasMoveAddedAccounts(t *testing.T) {
	e := accountTestEnv(t)
	a := e.addAccount(t, "Paysera", "cash")
	day := time.Now().Truncate(24 * time.Hour)
	require.NoError(t, e.db.Create(&domain.Balance{Date: day.AddDate(0, 0, -1), Swed: 100, Total: 300,
		Extra: domain.AccountValues{a.Key: 200}}).Error)

	_, err := e.balSvc.SnapshotFromDeltas([]service.AccountDelta{{Date: day, Account: a.Key, Amount: -50}})
	require.NoError(t, err)
	latest, err := e.balSvc.GetLatest(0)
	require.NoError(t, err)
	assert.Equal(t, 150.0, latest.Extra[a.Key])
	assert.Equal(t, 250.0, latest.Total)

	var first domain.Balance
	require.NoError(t, e.db.Order("date asc").First(&first).Error)
	assert.Equal(t, 200.0, first.Extra[a.Key], "earlier snapshot untouched")

	sets, err := e.balSvc.SnapshotFromAbsolute(map[string]float64{a.Key: 175}, time.Now())
	require.NoError(t, err)
	require.Len(t, sets, 1)
	assert.Equal(t, 150.0, sets[0].Before)
	latest, err = e.balSvc.GetLatest(0)
	require.NoError(t, err)
	assert.Equal(t, 175.0, latest.Extra[a.Key])
}

// Snapshots written before this change have no extra and read exactly as before.
func TestLegacySnapshotsReadUnchanged(t *testing.T) {
	e := accountTestEnv(t)
	require.NoError(t, e.db.Exec("INSERT INTO balances (date, total, swed, extra, created_at, updated_at) VALUES (?, 500, 500, '{}', ?, ?)",
		time.Now(), time.Now(), time.Now()).Error)
	latest, err := e.balSvc.GetLatest(0)
	require.NoError(t, err)
	assert.Nil(t, latest.Extra)
	assert.Nil(t, latest.ExtraGroups)
	assert.Equal(t, 500.0, latest.Total)
	raw, _ := json.Marshal(latest)
	assert.NotContains(t, string(raw), "extra")
}

// v7: added accounts and their snapshot values survive a wipe-and-restore.
func TestBackupRoundtrip_AddedAccounts(t *testing.T) {
	_, src := importRouterFor(t)
	require.NoError(t, src.AutoMigrate(&domain.ExportLog{}, &domain.Account{}))
	require.NoError(t, src.Create(&domain.Account{Key: "acc_paysera", Label: "Paysera", Group: domain.AccountGroupCash, Institution: "Paysera"}).Error)
	require.NoError(t, src.Create(&domain.Account{Key: "swed", Label: "Swedbank main", Group: domain.AccountGroupCash, Builtin: true}).Error)
	require.NoError(t, src.Create(&domain.Account{Key: "seb", Label: "SEB", Group: domain.AccountGroupCash, Builtin: true}).Error)
	require.NoError(t, src.Create(&domain.Account{Key: "swed_etf", Label: "Swed ETF", Group: domain.AccountGroupCash, Builtin: true}).Error)
	require.NoError(t, src.Create(&domain.Balance{Date: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Swed: 10, Total: 60,
		Extra: domain.AccountValues{"acc_paysera": 50}}).Error)

	exp := gin.New()
	NewExportHandler(service.NewTransactionService(repository.NewTransactionRepository(src), nil),
		service.NewBalanceService(repository.NewBalanceRepository(src), nil),
		service.NewStockService(repository.NewStockRepository(src)),
		service.NewAssetService(repository.NewAssetRepository(src)),
		repository.NewExportLogRepository(src)).WithAccounts(repository.NewAccountRepository(src)).
		RegisterRoutes(exp.Group("/api/v1"))
	rec := bankJSON(t, exp, http.MethodGet, "/api/v1/export/finances.json", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	body := rec.Body.Bytes()

	_, dst := importRouterFor(t)
	require.NoError(t, dst.AutoMigrate(&domain.Account{}))
	require.NoError(t, dst.Create(&domain.Account{Key: "swed", Label: "Swedbank", Group: domain.AccountGroupCash, Builtin: true}).Error)
	require.NoError(t, dst.Create(&domain.Account{Key: "swed_etf", Label: "Swed ETF", Group: domain.AccountGroupInvestments, Builtin: true}).Error)
	imp := gin.New()
	NewImportHandler(repository.NewTransactionRepository(dst), repository.NewBalanceRepository(dst),
		repository.NewStockRepository(dst), repository.NewAssetRepository(dst)).WithDB(dst).
		RegisterRoutes(imp.Group("/api/v1"))
	v5Import(t, imp, body)

	var acc domain.Account
	require.NoError(t, dst.Where("key = ?", "acc_paysera").First(&acc).Error)
	assert.Equal(t, "Paysera", acc.Label)
	assert.Equal(t, "Paysera", acc.Institution, "the bank travels with the backup")
	assert.Equal(t, domain.AccountGroupCash, acc.Group)
	var swed domain.Account
	require.NoError(t, dst.Where("key = ?", "swed").First(&swed).Error)
	assert.Equal(t, "Swedbank main", swed.Label, "a renamed built-in keeps its name through a restore")
	var n int64
	dst.Model(&domain.Account{}).Count(&n)
	assert.EqualValues(t, 3, n, "an unchanged built-in is not exported, a changed one is not duplicated")
	var etf domain.Account
	require.NoError(t, dst.Where("key = ?", "swed_etf").First(&etf).Error)
	assert.Equal(t, domain.AccountGroupCash, etf.Group, "a regrouped built-in keeps its group through a restore")
	var b domain.Balance
	require.NoError(t, dst.First(&b).Error)
	assert.Equal(t, 50.0, b.Extra["acc_paysera"])
	assert.Equal(t, 60.0, b.Total)
}

// Moving a built-in to another group moves its money there — in every
// snapshot, so history and today agree — and backend free cash follows.
func TestRegroupedBuiltinMovesItsMoney(t *testing.T) {
	e := accountTestEnv(t)
	require.NoError(t, e.db.Create(&domain.Balance{Date: time.Now().AddDate(0, -1, 0), Swed: 100, SwedETF: 40, Seb: 10, Total: 150}).Error)
	latest, err := e.balSvc.GetLatest(0)
	require.NoError(t, err)
	assert.Equal(t, 110.0, latest.Groups["cash"])
	assert.Equal(t, 40.0, latest.Groups["investments"])
	assert.Equal(t, 110.0, latest.FreeCash())

	var etf domain.Account
	require.NoError(t, e.db.Where("key = ?", "swed_etf").First(&etf).Error)
	rec := bankJSON(t, e.r, http.MethodPut, "/api/v1/accounts/"+fmt.Sprint(etf.ID), map[string]any{"group": "cash"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	latest, err = e.balSvc.GetLatest(0)
	require.NoError(t, err)
	assert.Equal(t, 150.0, latest.Groups["cash"])
	assert.Zero(t, latest.Groups["investments"])
	assert.Equal(t, 150.0, latest.FreeCash())
	assert.Equal(t, 150.0, latest.Total, "the total never changes")
}

// Without account rows (fake databases) the original fixed free cash stands.
func TestFreeCashWithoutGroups(t *testing.T) {
	b := domain.Balance{Seb: 1, Swed: 2, Cash: 3, RevM: 4, RevR: 5, SwedETF: 100}
	assert.Equal(t, 15.0, b.FreeCash())
}

func TestInferInstitution(t *testing.T) {
	known := []string{"SEB", "Swedbank", "Revolut", "Cash"}
	assert.Equal(t, "Swedbank", domain.InferInstitution("Swedbank savings", known))
	assert.Equal(t, "SEB", domain.InferInstitution("seb", known))
	assert.Equal(t, "", domain.InferInstitution("Sebastian's fund", known), "a word prefix is not a bank")
	assert.Equal(t, "", domain.InferInstitution("Paysera", known))
}

// A new account picks its bank from its name unless one is given.
func TestCreateAccountInstitution(t *testing.T) {
	e := accountTestEnv(t)
	require.NoError(t, e.db.Model(&domain.Account{}).Where("key = ?", "swed").Update("institution", "Swedbank").Error)
	a := e.addAccount(t, "Swedbank savings", "cash")
	assert.Equal(t, "Swedbank", a.Institution)

	rec := bankJSON(t, e.r, http.MethodPost, "/api/v1/accounts", map[string]any{"label": "Wallet", "group": "cash", "institution": "Paysera"})
	require.Equal(t, http.StatusCreated, rec.Code)
	var b domain.Account
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &b))
	assert.Equal(t, "Paysera", b.Institution)

	// Built-ins can be moved to another bank too.
	var swed domain.Account
	require.NoError(t, e.db.Where("key = ?", "swed").First(&swed).Error)
	rec = bankJSON(t, e.r, http.MethodPut, "/api/v1/accounts/"+fmt.Sprint(swed.ID), map[string]any{"institution": "Swedbank LT"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, e.db.First(&swed, swed.ID).Error)
	assert.Equal(t, "Swedbank LT", swed.Institution)
}
