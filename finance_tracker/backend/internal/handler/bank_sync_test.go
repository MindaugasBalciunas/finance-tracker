package handler

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/openbanking"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/mindaugas/finance-tracker/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type bankTestEnv struct {
	r      *gin.Engine
	db     *gorm.DB
	h      *BankHandler
	repo   repository.BankRepository
	txRepo repository.TransactionRepository
	link   *domain.BankAccountLink
}

func bankTestRouter(t *testing.T, configured bool) *bankTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.Transaction{}, &domain.Balance{}, &domain.Budget{},
		&domain.LabelRule{}, &domain.BudgetSettings{}, &domain.BankSettings{},
		&domain.BankConnection{}, &domain.BankAccountLink{}, &domain.BankStagedTx{}))

	bankRepo := repository.NewBankRepository(db)
	txRepo := repository.NewTransactionRepository(db)
	txSvc := service.NewTransactionServiceWithRules(txRepo, nil, repository.NewBudgetRepository(db))

	if configured {
		s, err := bankRepo.GetSettings()
		require.NoError(t, err)
		s.ApplicationID = "test-app"
		s.PrivateKeyPEM = testRSAKeyPEM(t)
		s.RedirectURL = "https://example.invalid/"
		require.NoError(t, bankRepo.SaveSettings(s))
	}

	// WithLabeling mirrors main.go: the learned signals are part of the
	// production wiring, so the tests exercise the same handler.
	h := NewBankHandler(bankRepo, txRepo).WithDB(db).WithLabeling(repository.NewBudgetRepository(db))
	r := gin.New()
	v1 := r.Group("/api/v1")
	h.RegisterRoutes(v1)
	NewTransactionHandler(txSvc).WithBanking(bankRepo).RegisterRoutes(v1)

	conn := &domain.BankConnection{
		ASPSPName: "Swedbank", ASPSPCountry: "LT",
		Status: domain.BankConnAuthorized, ValidUntil: time.Now().Add(90 * 24 * time.Hour),
	}
	require.NoError(t, bankRepo.SaveConnection(conn))
	link := &domain.BankAccountLink{
		ConnectionID: conn.ID, IdentificationHash: "hash-1",
		UID: "uid-1", IBAN: "LT160000000000001234", AccountKey: "swed",
	}
	require.NoError(t, bankRepo.SaveLink(link))

	return &bankTestEnv{r: r, db: db, h: h, repo: bankRepo, txRepo: txRepo, link: link}
}

// testRSAKeyPEM mints a throwaway signing key. The handler validates the PEM
// before storing it, so a placeholder string would not get past settings.
func testRSAKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))
}

func bankJSON(t *testing.T, r http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// ebRow builds one booked provider transaction.
func ebRow(ref, date, amount, dk, payee, details string) openbanking.Transaction {
	tx := openbanking.Transaction{
		EntryReference:        ref,
		Status:                openbanking.StatusBooked,
		BookingDate:           date,
		ValueDate:             date,
		TransactionAmount:     openbanking.Amount{Amount: amount, Currency: "EUR"},
		RemittanceInformation: []string{details},
	}
	if dk == "K" {
		tx.CreditDebitIndicator = openbanking.IndicatorCredit
		tx.Debtor.Name = payee
	} else {
		tx.CreditDebitIndicator = openbanking.IndicatorDebit
		tx.Creditor.Name = payee
	}
	return tx
}

func bankFeed() []openbanking.Transaction {
	return []openbanking.Transaction{
		ebRow("7001", "2026-09-10", "23.40", "D", "'50146 LIDL SNIPISKES", "PIRKINYS 516793******2950 2026.09.09 23.40 EUR (294126) 50146 LIDL SNIPISKES"),
		ebRow("7002", "2026-09-12", "8.12", "D", "UAB IGNITIS", "E.Sąskaitos Nr. LDTESB-01464043 apmokėjimas"),
		ebRow("7003", "2026-09-15", "2100.00", "K", "MOBILEPAY A/S LITHUANIA BRANCH", "Pervedimas pagal darbo sutarti su MobilePay A/S 2026/09 men."),
	}
}

func (e *bankTestEnv) stage(t *testing.T, feed []openbanking.Transaction) syncResult {
	t.Helper()
	res, err := e.h.stage(feed, e.link, time.Now().AddDate(0, 0, -30), time.Now())
	require.NoError(t, err)
	return res
}

func (e *bankTestEnv) staged(t *testing.T) []domain.BankStagedTx {
	t.Helper()
	rows, _, err := e.repo.ListStaged(repository.StagedFilter{})
	require.NoError(t, err)
	return rows
}

func TestBankStageAndCommit(t *testing.T) {
	env := bankTestRouter(t, true)
	res := env.stage(t, bankFeed())
	assert.Equal(t, 3, res.Fetched)
	assert.Equal(t, 3, res.StagedNew)
	assert.Equal(t, 0, res.DuplicateContent)

	rows := env.staged(t)
	require.Len(t, rows, 3)
	for _, r := range rows {
		assert.Equal(t, domain.VerdictNew, r.Verdict)
		assert.True(t, r.Preticked(), "an unambiguously new row arrives ticked")
		assert.Equal(t, domain.StagedStateStaged, r.State)
	}

	// The window advances off the newest booking date, not the clock.
	link, err := env.repo.GetLink(env.link.ID)
	require.NoError(t, err)
	require.NotNil(t, link.LastTxDate)
	assert.Equal(t, "2026-09-15", link.LastTxDate.Format("2006-01-02"))

	// Commit one row — one at a time is the primary interaction.
	var target domain.BankStagedTx
	for _, r := range rows {
		if r.Comment == "UAB IGNITIS" {
			target = r
		}
	}
	require.NotZero(t, target.ID)

	rec := bankJSON(t, env.r, "POST", "/api/v1/banking/staged/commit", map[string]any{"ids": []uint{target.ID}})
	require.Equal(t, 200, rec.Code, rec.Body.String())
	var out commitResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, 1, out.Imported)
	assert.Equal(t, 0, out.Skipped)
	require.Len(t, out.ImportedTxIDs, 1)
	// With no snapshot to build on, the honest line is why nothing moved.
	assert.Contains(t, out.BalanceNote, "no balance snapshot exists yet")

	var tx domain.Transaction
	require.NoError(t, env.db.First(&tx, out.ImportedTxIDs[0]).Error)
	assert.Equal(t, target.ExternalID, tx.ExternalID)
	assert.Equal(t, "UAB IGNITIS", tx.Comment)
	assert.Equal(t, "Utilities", string(tx.Category))
	assert.Equal(t, "swed", tx.DebitAccount)

	// Nothing to clone, so nothing was written — a snapshot invented out of
	// thin air would be a number nobody observed.
	var balances int64
	require.NoError(t, env.db.Table("balances").Count(&balances).Error)
	assert.Zero(t, balances)
}

// TestBankSyncIdempotent — pressing Sync twice must not re-offer anything.
func TestBankSyncIdempotent(t *testing.T) {
	env := bankTestRouter(t, true)
	env.stage(t, bankFeed())
	second := env.stage(t, bankFeed())
	assert.Equal(t, 0, second.StagedNew, "nothing new on a re-sync")
	assert.Equal(t, 3, second.Unchanged)
	assert.Len(t, env.staged(t), 3)
}

// TestBankDismissedRowStaysDismissed — the dismissal ledger IS the persisted
// state. Re-offering a row the user already rejected is the one thing a
// re-sync must never do.
func TestBankDismissedRowStaysDismissed(t *testing.T) {
	env := bankTestRouter(t, true)
	env.stage(t, bankFeed())
	rows := env.staged(t)

	rec := bankJSON(t, env.r, "POST", "/api/v1/banking/staged/dismiss", map[string]any{"ids": []uint{rows[0].ID}})
	require.Equal(t, 200, rec.Code, rec.Body.String())

	env.stage(t, bankFeed())
	after, err := env.repo.GetStagedByExternalID(rows[0].ExternalID)
	require.NoError(t, err)
	assert.Equal(t, domain.StagedStateDismissed, after.State)

	// And a mis-tap is recoverable.
	rec = bankJSON(t, env.r, "POST", "/api/v1/banking/staged/restore", map[string]any{"ids": []uint{rows[0].ID}})
	require.Equal(t, 200, rec.Code, rec.Body.String())
	after, err = env.repo.GetStagedByExternalID(rows[0].ExternalID)
	require.NoError(t, err)
	assert.Equal(t, domain.StagedStateStaged, after.State)
}

// TestBankCommittedRowIsDuplicateExactOnResync — layer 1. Once a row is in
// the ledger its provider id is there too, so a re-sync recognises it with
// certainty rather than heuristically.
func TestBankCommittedRowIsDuplicateExactOnResync(t *testing.T) {
	env := bankTestRouter(t, true)
	env.stage(t, bankFeed())
	rows := env.staged(t)
	rec := bankJSON(t, env.r, "POST", "/api/v1/banking/staged/commit", map[string]any{"ids": []uint{rows[0].ID}})
	require.Equal(t, 200, rec.Code, rec.Body.String())

	res := env.stage(t, bankFeed())
	assert.Equal(t, 1, res.DuplicateExact)
	after, err := env.repo.GetStagedByExternalID(rows[0].ExternalID)
	require.NoError(t, err)
	// Already imported, so it stays imported and off the review list.
	assert.Equal(t, domain.StagedStateImported, after.State)
}

// TestBankDuplicateContentIsUntickedNotHidden — layer 2, the ~150-duplicate
// class. A row already in the ledger from a CSV import carries no provider
// id, so only content dedup catches it. It is a heuristic, so it is shown
// with its reason rather than hidden — and left unticked.
func TestBankDuplicateContentIsUntickedNotHidden(t *testing.T) {
	env := bankTestRouter(t, true)
	// Seed the ledger the way a CSV import would: no external id.
	seed := &domain.Transaction{
		Date: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
		Type: domain.TransactionTypeExpense, Amount: 8.12,
		Category: domain.CategoryUtilities, Comment: "UAB IGNITIS",
	}
	require.NoError(t, env.txRepo.Create(seed))

	res := env.stage(t, bankFeed())
	assert.Equal(t, 1, res.DuplicateContent)

	var dup domain.BankStagedTx
	for _, r := range env.staged(t) {
		if r.Verdict == domain.VerdictDuplicateContent {
			dup = r
		}
	}
	require.NotZero(t, dup.ID)
	assert.False(t, dup.Preticked(), "a duplicate never arrives ticked")
	assert.Equal(t, domain.StagedStateStaged, dup.State, "shown with its reason, not hidden")
	assert.Contains(t, dup.VerdictNote, "UAB IGNITIS")
	require.NotNil(t, dup.MatchedTxID)
	assert.Equal(t, seed.ID, *dup.MatchedTxID)

	// Ticked anyway, it still commits: the user saw the warning, and two
	// identical payments on one day are a real thing.
	rec := bankJSON(t, env.r, "POST", "/api/v1/banking/staged/commit", map[string]any{"ids": []uint{dup.ID}})
	require.Equal(t, 200, rec.Code, rec.Body.String())
	var out commitResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, 1, out.Imported)
}

// TestBankCommitReverifiesVerdict — the review list may have been open for an
// hour. A row staged as new that has since been matched in the ledger is held
// back with an explanation, not written blind.
func TestBankCommitReverifiesVerdict(t *testing.T) {
	env := bankTestRouter(t, true)
	env.stage(t, bankFeed())
	var target domain.BankStagedTx
	for _, r := range env.staged(t) {
		if r.Comment == "UAB IGNITIS" {
			target = r
		}
	}
	require.NotZero(t, target.ID)
	require.Equal(t, domain.VerdictNew, target.Verdict)

	// Someone adds the same transaction by hand in the meantime.
	require.NoError(t, env.txRepo.Create(&domain.Transaction{
		Date: target.Date, Type: target.Type, Amount: target.Amount,
		Category: target.Category, Comment: target.Comment,
	}))

	rec := bankJSON(t, env.r, "POST", "/api/v1/banking/staged/commit", map[string]any{"ids": []uint{target.ID}})
	require.Equal(t, 200, rec.Code, rec.Body.String())
	var out commitResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, 0, out.Imported)
	assert.Equal(t, 1, out.Skipped)
	require.NotEmpty(t, out.Notes)

	after, err := env.repo.GetStagedByExternalID(target.ExternalID)
	require.NoError(t, err)
	assert.Equal(t, domain.VerdictDuplicateContent, after.Verdict)
	assert.Equal(t, domain.StagedStateStaged, after.State, "still reviewable, with the warning now attached")
}

// TestBankUndoRestoresStagedRow — deleting the committed transaction returns
// you to the review list rather than dropping the row on the floor.
func TestBankUndoRestoresStagedRow(t *testing.T) {
	env := bankTestRouter(t, true)
	env.stage(t, bankFeed())
	rows := env.staged(t)

	rec := bankJSON(t, env.r, "POST", "/api/v1/banking/staged/commit", map[string]any{"ids": []uint{rows[0].ID, rows[1].ID}})
	require.Equal(t, 200, rec.Code, rec.Body.String())
	var out commitResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.ImportedTxIDs, 2)

	rec = bankJSON(t, env.r, "DELETE", "/api/v1/transactions/batch", map[string]any{"ids": out.ImportedTxIDs})
	require.Equal(t, 204, rec.Code, rec.Body.String())

	var count int64
	require.NoError(t, env.db.Model(&domain.Transaction{}).Count(&count).Error)
	assert.Zero(t, count)

	for _, id := range []uint{rows[0].ID, rows[1].ID} {
		var row domain.BankStagedTx
		require.NoError(t, env.db.First(&row, id).Error)
		assert.Equal(t, domain.StagedStateStaged, row.State)
		assert.Equal(t, domain.VerdictNew, row.Verdict, "the transaction it matched has just been deleted")
		assert.Nil(t, row.ImportedTxID)
	}
}

// TestBankStagesReservationsSeparately — a card reservation is staged so the
// spend is visible days early, but it is marked, never pre-ticked and never
// committable: the amount is not final and the bank re-issues the row under a
// new reference when it books.
func TestBankStagesReservationsSeparately(t *testing.T) {
	env := bankTestRouter(t, true)
	res := env.stage(t, append(bankFeed(), pendingRow("7004", "2026-09-16", "4.20", "D", "Caffeine", "Kava")))

	assert.Equal(t, 0, res.AutoSkipped)
	assert.Equal(t, 4, res.StagedNew)
	assert.Equal(t, 1, res.Pending)

	hold := env.stagedByComment(t, "Caffeine (Kava)")
	assert.True(t, hold.Pending)
	assert.Equal(t, domain.VerdictPending, hold.Verdict)
	assert.False(t, hold.Preticked(), "a reservation never arrives ticked")
	assert.False(t, hold.Committable())

	// Committing one is refused, with a reason rather than a silent skip.
	rec := bankJSON(t, env.r, "POST", "/api/v1/banking/staged/commit", map[string]any{"ids": []uint{hold.ID}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out commitResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, 0, out.Imported)
	assert.Equal(t, 1, out.Skipped)
	assert.Contains(t, strings.Join(out.Notes, " "), "still only reserved")
}

// A status that is neither booked nor reserved never moved money.
func TestBankSkipsRejectedRows(t *testing.T) {
	env := bankTestRouter(t, true)
	rejected := ebRow("7009", "2026-09-16", "4.20", "D", "Caffeine", "Kava")
	rejected.Status = "RJCT"

	res := env.stage(t, []openbanking.Transaction{rejected})
	assert.Equal(t, 1, res.AutoSkipped)
	assert.Equal(t, 0, res.StagedNew)
}

// TestBankInternalRowsStagedNotDropped — the classifier's own-account verdict
// is surfaced unticked rather than silently discarded, so a misfire is
// visible instead of invisible.
func TestBankInternalRowsStagedNotDropped(t *testing.T) {
	env := bankTestRouter(t, true)
	feed := []openbanking.Transaction{
		ebRow("8001", "2026-09-11", "1000.00", "K", "Mindaugas BALCIUNAS", "Transfer between my accounts"),
	}
	res := env.stage(t, feed)
	assert.Equal(t, 1, res.Internal)
	rows := env.staged(t)
	require.Len(t, rows, 1)
	assert.Equal(t, domain.VerdictInternal, rows[0].Verdict)
	assert.False(t, rows[0].Preticked())
}

// TestBankRoutesHiddenWhenUnconfigured — with no application registered there
// is nothing here to forbid, so the feature does not exist. Settings stay
// reachable: they are how the credentials get there in the first place.
func TestBankRoutesHiddenWhenUnconfigured(t *testing.T) {
	env := bankTestRouter(t, false)
	for _, p := range []string{"/api/v1/banking/connections", "/api/v1/banking/staged", "/api/v1/banking/aspsps"} {
		rec := bankJSON(t, env.r, "GET", p, nil)
		assert.Equal(t, http.StatusNotFound, rec.Code, p)
	}
	rec := bankJSON(t, env.r, "GET", "/api/v1/banking/settings", nil)
	require.Equal(t, 200, rec.Code)
	var s bankSettingsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &s))
	assert.False(t, s.Configured)
	assert.False(t, s.HasKey)
}

// TestBankSettingsNeverEchoKey — the private key is write-only over the API.
func TestBankSettingsNeverEchoKey(t *testing.T) {
	env := bankTestRouter(t, true)
	rec := bankJSON(t, env.r, "GET", "/api/v1/banking/settings", nil)
	require.Equal(t, 200, rec.Code)
	assert.NotContains(t, rec.Body.String(), "BEGIN")
	assert.Contains(t, rec.Body.String(), `"has_key":true`)

	// Blank means keep: an unrelated edit must not wipe the stored key.
	rec = bankJSON(t, env.r, "PUT", "/api/v1/banking/settings", map[string]any{"redirect_url": "https://example.invalid/app/"})
	require.Equal(t, 200, rec.Code, rec.Body.String())
	s, err := env.repo.GetSettings()
	require.NoError(t, err)
	assert.NotEmpty(t, s.PrivateKeyPEM)
	assert.Equal(t, "https://example.invalid/app/", s.RedirectURL)

	// And clearing is explicit.
	rec = bankJSON(t, env.r, "PUT", "/api/v1/banking/settings", map[string]any{"clear_key": true})
	require.Equal(t, 200, rec.Code, rec.Body.String())
	s, err = env.repo.GetSettings()
	require.NoError(t, err)
	assert.Empty(t, s.PrivateKeyPEM)
}

// TestExportOmitsBankCredentials — the JSON backup is a file the user
// downloads, mails to themselves and forgets about. The AI key travels in it
// deliberately; a long-lived bank credential is a different class of secret
// and must not. This guards the structure, so adding a bank section to the
// export later fails here first.
func TestExportOmitsBankCredentials(t *testing.T) {
	env := bankTestRouter(t, true)
	require.NoError(t, env.db.AutoMigrate(&domain.StockTrade{}, &domain.Asset{}, &domain.ExportLog{}, &domain.AISettings{}))

	s, err := env.repo.GetSettings()
	require.NoError(t, err)
	s.ApplicationID = "4f1e-secret-app-id"
	require.NoError(t, env.repo.SaveSettings(s))

	conn, err := env.repo.GetConnection(env.link.ConnectionID)
	require.NoError(t, err)
	conn.SessionID = "sess-should-never-be-exported"
	require.NoError(t, env.repo.SaveConnection(conn))

	txRepo := repository.NewTransactionRepository(env.db)
	balRepo := repository.NewBalanceRepository(env.db)
	balSvc := service.NewBalanceService(balRepo, txRepo)
	exp := NewExportHandler(
		service.NewTransactionServiceWithRules(txRepo, balSvc, repository.NewBudgetRepository(env.db)),
		balSvc,
		service.NewStockService(repository.NewStockRepository(env.db)),
		service.NewAssetService(repository.NewAssetRepository(env.db)),
		repository.NewExportLogRepository(env.db),
	)
	r := gin.New()
	exp.RegisterRoutes(r.Group("/api/v1"))

	rec := bankJSON(t, r, "GET", "/api/v1/export/finances.json", nil)
	require.Equal(t, 200, rec.Code, rec.Body.String())
	body := rec.Body.String()
	assert.NotContains(t, body, "4f1e-secret-app-id")
	assert.NotContains(t, body, "sess-should-never-be-exported")
	assert.NotContains(t, body, "BEGIN")
	assert.NotContains(t, body, "private_key")
	assert.NotContains(t, body, "application_id")
}

// TestBankRoutesLocked — the banking routes register after the lock
// middleware, so a locked instance must not answer any of them. The 404 for
// an unconfigured feature is a layer *below* this one: the lock is checked
// first, and a locked instance must not even reveal whether banking is set up.
func TestBankRoutesLocked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.AuthSettings{}, &domain.WebauthnCredential{},
		&domain.Transaction{}, &domain.BankSettings{}, &domain.BankConnection{},
		&domain.BankAccountLink{}, &domain.BankStagedTx{}))

	bankRepo := repository.NewBankRepository(db)
	s, err := bankRepo.GetSettings()
	require.NoError(t, err)
	s.ApplicationID = "test-app"
	s.PrivateKeyPEM = testRSAKeyPEM(t)
	require.NoError(t, bankRepo.SaveSettings(s))

	authHandler := NewAuthHandler(service.NewAuthService(repository.NewAuthRepository(db)))
	r := gin.New()
	v1 := r.Group("/api/v1")
	authHandler.RegisterRoutes(v1)
	v1.Use(authHandler.Middleware())
	NewBankHandler(bankRepo, repository.NewTransactionRepository(db)).WithDB(db).RegisterRoutes(v1)

	// upstream marks the routes that would reach Enable Banking once they get
	// past their own preconditions. They are exercised in the locked pass only
	// — the middleware answers before the handler runs — so the suite never
	// makes an outbound call.
	routes := []struct {
		method, path string
		upstream     bool
	}{
		{method: "GET", path: "/api/v1/banking/settings"},
		{method: "PUT", path: "/api/v1/banking/settings"},
		{method: "GET", path: "/api/v1/banking/aspsps", upstream: true},
		{method: "GET", path: "/api/v1/banking/connections"},
		{method: "POST", path: "/api/v1/banking/connections", upstream: true},
		{method: "POST", path: "/api/v1/banking/connections/callback", upstream: true},
		{method: "DELETE", path: "/api/v1/banking/connections/1"},
		{method: "PUT", path: "/api/v1/banking/accounts/1"},
		{method: "POST", path: "/api/v1/banking/accounts/1/sync"},
		{method: "POST", path: "/api/v1/banking/sync", upstream: true},
		{method: "GET", path: "/api/v1/banking/staged"},
		{method: "PUT", path: "/api/v1/banking/staged/1"},
		{method: "POST", path: "/api/v1/banking/staged/commit"},
		{method: "POST", path: "/api/v1/banking/staged/dismiss"},
		{method: "POST", path: "/api/v1/banking/staged/restore"},
	}

	// Unlocked: the routes answer (any status but 401 — we are proving the
	// middleware is not the thing replying).
	for _, rt := range routes {
		if rt.upstream {
			continue
		}
		rec := bankJSON(t, r, rt.method, rt.path, map[string]any{})
		assert.NotEqual(t, http.StatusUnauthorized, rec.Code, "%s %s with no lock set", rt.method, rt.path)
	}

	rec := bankJSON(t, r, "POST", "/api/v1/auth/pin/setup", map[string]string{"pin": "123456"})
	require.Equal(t, 200, rec.Code, rec.Body.String())

	for _, rt := range routes {
		rec := bankJSON(t, r, rt.method, rt.path, map[string]any{})
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "%s %s must be locked", rt.method, rt.path)
	}
}

// TestBankEditStagedRowCarriesIntoLedger — the point of editing at review time
// is that the correction lands in the ledger, not that it looks right on the
// card. Also pins the two guards: the raw provider columns are not editable,
// and an already-imported row is not editable at all.
func TestBankEditStagedRowCarriesIntoLedger(t *testing.T) {
	env := bankTestRouter(t, true)
	env.stage(t, bankFeed())
	rows := env.staged(t)
	var target domain.BankStagedTx
	for _, r := range rows {
		if r.Comment == "UAB IGNITIS" {
			target = r
		}
	}
	require.NotZero(t, target.ID)

	path := fmt.Sprintf("/api/v1/banking/staged/%d", target.ID)
	rec := bankJSON(t, env.r, "PUT", path, map[string]any{
		"category":      "Housing",
		"comment":       "Electricity — summer house",
		"labels":        "utilities,house",
		"debit_account": "seb",
	})
	require.Equal(t, 200, rec.Code, rec.Body.String())

	var edited stagedResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &edited))
	assert.Equal(t, "Housing", string(edited.Category))
	assert.Equal(t, "Electricity — summer house", edited.Comment)
	assert.Equal(t, "seb", edited.DebitAccount)
	// The audit trail is not editable: raw columns and the dedup key survive.
	assert.Equal(t, target.RawPayee, edited.RawPayee)
	assert.Equal(t, target.ExternalID, edited.ExternalID)
	// Editing does not re-run the verdict — it answers a question about the
	// provider row, which retyping a comment does not change.
	assert.Equal(t, domain.VerdictNew, edited.Verdict)

	rec = bankJSON(t, env.r, "POST", "/api/v1/banking/staged/commit", map[string]any{"ids": []uint{target.ID}})
	require.Equal(t, 200, rec.Code, rec.Body.String())
	var out commitResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.ImportedTxIDs, 1)

	var tx domain.Transaction
	require.NoError(t, env.db.First(&tx, out.ImportedTxIDs[0]).Error)
	assert.Equal(t, "Housing", string(tx.Category))
	assert.Equal(t, "Electricity — summer house", tx.Comment)
	assert.Equal(t, "utilities,house", tx.Labels)
	assert.Equal(t, "seb", tx.DebitAccount)

	// Already imported: editing it would change a card that no longer drives
	// anything, so it is refused rather than silently ignored.
	rec = bankJSON(t, env.r, "PUT", path, map[string]any{"category": "Food"})
	assert.Equal(t, 409, rec.Code)
}

// TestBankEditRejectsUnknownAccount — the mapping dropdown and the commit
// validator read the same list; a typo must not reach the ledger.
func TestBankEditRejectsUnknownAccount(t *testing.T) {
	env := bankTestRouter(t, true)
	env.stage(t, bankFeed())
	rows := env.staged(t)
	require.NotEmpty(t, rows)
	path := fmt.Sprintf("/api/v1/banking/staged/%d", rows[0].ID)

	rec := bankJSON(t, env.r, "PUT", path, map[string]any{"debit_account": "monzo"})
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "monzo")

	// Empty is legitimate — an expense has no credit side.
	rec = bankJSON(t, env.r, "PUT", path, map[string]any{"credit_account": ""})
	assert.Equal(t, 200, rec.Code, rec.Body.String())

	rec = bankJSON(t, env.r, "PUT", path, map[string]any{"date": "15/09/2026"})
	assert.Equal(t, 400, rec.Code)
}

// The API token's view of banking is a keyhole, and these are the edges of
// it. The feature's premise is that a human approves every row one at a time,
// so the grant has to let a model improve a proposal without ever being able
// to act on one — and it must not drag the credentials in behind it.
func TestAPITokenBankingScopeIsExactlyTheReviewQueue(t *testing.T) {
	t.Run("readable", func(t *testing.T) {
		for _, p := range []string{
			"/api/v1/banking/staged",
			"/api/v1/banking/staged/12",
			"/api/v1/banking/connections",
		} {
			if !apiTokenAllowed(p) {
				t.Errorf("%s should be readable by an API token", p)
			}
		}
	})

	// The private key lives behind /banking/settings. Nothing an AI does is
	// worth putting that inside a token's reach.
	t.Run("credentials stay out of reach", func(t *testing.T) {
		for _, p := range []string{
			"/api/v1/banking/settings",
			"/api/v1/banking",
			"/api/v1/banking/aspsps",
			// A sibling that merely starts with an allowed prefix.
			"/api/v1/banking/staged-exports",
			"/api/v1/banking/connections-admin",
		} {
			if apiTokenAllowed(p) {
				t.Errorf("%s must NOT be readable by an API token", p)
			}
		}
	})

	// Editing a proposal is granted; acting on one is not. Commit, dismiss
	// and restore are POSTs under the same prefix — the method is what keeps
	// them out, so this is the assertion that matters most.
	t.Run("edit yes, commit no", func(t *testing.T) {
		if !apiTokenWriteAllowed("PUT", "/api/v1/banking/staged/12") {
			t.Error("PUT /banking/staged/:id should be writable by an ftkw_ token")
		}
		for _, r := range []struct{ method, path string }{
			{"POST", "/api/v1/banking/staged/commit"},
			{"POST", "/api/v1/banking/staged/dismiss"},
			{"POST", "/api/v1/banking/staged/restore"},
			{"POST", "/api/v1/banking/accounts/1/sync"},
			{"POST", "/api/v1/banking/connections"},
			{"DELETE", "/api/v1/banking/connections/1"},
			{"PUT", "/api/v1/banking/settings"},
			{"PUT", "/api/v1/banking/accounts/1"},
		} {
			if apiTokenWriteAllowed(r.method, r.path) {
				t.Errorf("%s %s must NOT be writable by an API token", r.method, r.path)
			}
		}
	})
}

// TestSyncAllReportsEveryAccount — Sync all must not stop at the first
// account that cannot sync, and must say why each one did not.
//
// Only the paths that never reach the provider are exercised: the suite makes
// no outbound calls, and the skip reasons are the part worth pinning anyway —
// an account silently missing from the report is how "I pressed sync and
// nothing came" happens.
func TestSyncAllReportsEveryAccount(t *testing.T) {
	env := bankTestRouter(t, true)

	// The harness link is mapped and authorised, so it would reach the bank.
	// Drop its session: that is the "reconnect" case.
	env.link.UID = ""
	require.NoError(t, env.repo.SaveLink(env.link))

	// An account the user chose not to sync — no line at all, because
	// "Don't sync" is an answer, not a problem.
	require.NoError(t, env.repo.SaveLink(&domain.BankAccountLink{
		ConnectionID: env.link.ConnectionID, IdentificationHash: "hash-2",
		UID: "uid-2", IBAN: "LT160000000000009999", AccountKey: "",
	}))

	// A second bank whose consent has lapsed.
	dead := &domain.BankConnection{
		ASPSPName: "SEB", ASPSPCountry: "LT",
		Status: domain.BankConnAuthorized, ValidUntil: time.Now().Add(-24 * time.Hour),
	}
	require.NoError(t, env.repo.SaveConnection(dead))
	require.NoError(t, env.repo.SaveLink(&domain.BankAccountLink{
		ConnectionID: dead.ID, IdentificationHash: "hash-3",
		UID: "uid-3", IBAN: "LT160000000000008888", AccountKey: "seb",
	}))

	rec := bankJSON(t, env.r, "POST", "/api/v1/banking/sync", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var out syncAllResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, 0, out.Synced)
	assert.Equal(t, 0, out.Failed)
	assert.Equal(t, 2, out.Skipped)
	require.Len(t, out.Accounts, 2, "the unmapped account earns no line")

	reasons := map[string]string{}
	for _, a := range out.Accounts {
		reasons[a.Bank] = a.Skipped
		assert.Nil(t, a.Result)
	}
	assert.Contains(t, reasons["Swedbank"], "reconnect the bank")
	assert.Contains(t, reasons["SEB"], "expired")
}

// TestPendingCountMatchesTheQueue — the badge and the list it labels must
// agree. A likely duplicate still sits in the queue until somebody dismisses
// it, so leaving it out of the count lets the queue grow while the badge
// claims it is empty.
func TestPendingCountMatchesTheQueue(t *testing.T) {
	env := bankTestRouter(t, true)
	env.seedHistory(t, domain.Transaction{
		Date:   time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
		Type:   domain.TransactionTypeExpense,
		Amount: 8.12, Category: "Utilities", Comment: "UAB IGNITIS",
	})

	env.stage(t, bankFeed())

	rows, total, err := env.repo.ListStaged(repository.StagedFilter{State: domain.StagedStateStaged})
	require.NoError(t, err)
	require.Len(t, rows, 3)

	var dupes int
	for _, r := range rows {
		if r.Verdict == domain.VerdictDuplicateContent {
			dupes++
		}
	}
	require.Equal(t, 1, dupes, "the seeded row should make one staged row look like a duplicate")

	counts, err := env.repo.CountPendingByLink()
	require.NoError(t, err)
	assert.Equal(t, int(total), counts[env.link.ID], "the badge counts the whole queue, duplicates included")
}

// TestCommitSnapshotsBalancesOnce — committing bank rows moves the balance
// sheet, and moves it once.
//
// Per-row snapshots would plant a step in the net-worth trend for every row
// added; leaving balances alone (the previous behaviour) meant the only way
// to make the totals follow an import was to edit the last snapshot by hand.
func TestCommitSnapshotsBalancesOnce(t *testing.T) {
	env := bankTestRouter(t, true)
	// Something to build on. Balances are observations — without one there is
	// nothing to apply a delta to.
	require.NoError(t, env.db.Create(&domain.Balance{
		Date: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Swed: 1000, Total: 1000,
	}).Error)

	env.stage(t, bankFeed())
	rows := env.staged(t)
	ids := make([]uint, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}

	rec := bankJSON(t, env.r, "POST", "/api/v1/banking/staged/commit", map[string]any{"ids": ids})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out commitResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Equal(t, 3, out.Imported)
	assert.Contains(t, out.BalanceNote, "new snapshot")

	var balances []domain.Balance
	require.NoError(t, env.db.Order("date ASC, id ASC").Find(&balances).Error)
	require.Len(t, balances, 2, "one new snapshot for the whole commit, not one per row")

	// The seeded snapshot is untouched: history is added to, never rewritten.
	assert.Equal(t, 1000.0, balances[0].Swed)

	// −23.40 Lidl, −8.12 Ignitis, +2100.00 salary, all on swed.
	assert.InDelta(t, 1000-23.40-8.12+2100.00, balances[1].Swed, 0.001)
	// Dated the newest row committed.
	assert.Equal(t, "2026-09-15", balances[1].Date.Format("2006-01-02"))
}

// A row with no account names nothing to move, so the balance sheet is left
// alone and says so rather than cutting a snapshot identical to the last one.
func TestCommitWithoutAccountsLeavesBalancesAlone(t *testing.T) {
	env := bankTestRouter(t, true)
	require.NoError(t, env.db.Create(&domain.Balance{
		Date: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Swed: 1000, Total: 1000,
	}).Error)

	env.stage(t, bankFeed())
	row := env.stagedByComment(t, "UAB IGNITIS")
	rec := bankJSON(t, env.r, "PUT", stagedPath(row.ID), map[string]any{"debit_account": "", "credit_account": ""})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = bankJSON(t, env.r, "POST", "/api/v1/banking/staged/commit", map[string]any{"ids": []uint{row.ID}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out commitResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Equal(t, 1, out.Imported)
	assert.Contains(t, out.BalanceNote, "none of these rows name an account")

	var n int64
	require.NoError(t, env.db.Model(&domain.Balance{}).Count(&n).Error)
	assert.EqualValues(t, 1, n)
}

// TestCommitSkipsRowsTheSnapshotAlreadyCovers — a sync routinely spans the
// latest snapshot, so the batch must drop its older rows rather than fold
// them into the total.
//
// Those rows left the account before the snapshot was taken, so the snapshot
// already shows the money gone. Counting them again would subtract it twice.
func TestCommitSkipsRowsTheSnapshotAlreadyCovers(t *testing.T) {
	env := bankTestRouter(t, true)
	// Taken between the Ignitis row (09-12) and the salary (09-15).
	require.NoError(t, env.db.Create(&domain.Balance{
		Date: time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC),
		Swed: 1000, Total: 1000,
	}).Error)

	env.stage(t, bankFeed())
	ids := []uint{}
	for _, r := range env.staged(t) {
		ids = append(ids, r.ID)
	}

	rec := bankJSON(t, env.r, "POST", "/api/v1/banking/staged/commit", map[string]any{"ids": ids})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out commitResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Equal(t, 3, out.Imported)
	assert.Contains(t, out.BalanceNote, "left out of the balance")

	var balances []domain.Balance
	require.NoError(t, env.db.Order("date ASC, id ASC").Find(&balances).Error)
	require.Len(t, balances, 2)
	// Only the 09-15 salary lands: the two older card rows are already in the
	// 09-14 snapshot.
	assert.InDelta(t, 1000+2100.00, balances[1].Swed, 0.001)
	assert.Equal(t, "2026-09-15", balances[1].Date.Format("2006-01-02"))
}

// pendingRow builds a card reservation.
func pendingRow(ref, date, amount, dk, payee, details string) openbanking.Transaction {
	t := ebRow(ref, date, amount, dk, payee, details)
	t.Status = openbanking.StatusPending
	return t
}

// TestReservationSupersededByItsBooking — the whole point of the lifecycle.
//
// The bank re-issues a card purchase under a NEW entry_reference when it
// books, with a later date and often a different amount. Without pairing the
// two, the queue would offer the same coffee twice.
func TestReservationSupersededByItsBooking(t *testing.T) {
	env := bankTestRouter(t, true)

	// Day one: only the hold exists.
	res := env.stage(t, []openbanking.Transaction{
		pendingRow("hold-1", "2026-09-16", "20.00", "D", "CIRCLE K VILNIUS", "Kuras"),
	})
	require.Equal(t, 1, res.Pending)

	hold := env.stagedByComment(t, "CIRCLE K VILNIUS (Kuras)")
	// The user files it while it is still a hold.
	rec := bankJSON(t, env.r, "PUT", stagedPath(hold.ID), map[string]any{
		"category": "Transport", "labels": "fuel",
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Day three: it books, for the real amount, under a new reference.
	res = env.stage(t, []openbanking.Transaction{
		ebRow("booked-1", "2026-09-18", "17.43", "D", "CIRCLE K VILNIUS", "Kuras"),
	})
	assert.Equal(t, 1, res.Superseded)
	assert.Equal(t, 1, res.StagedNew)

	staged, _, err := env.repo.ListStaged(repository.StagedFilter{State: domain.StagedStateStaged})
	require.NoError(t, err)
	require.Len(t, staged, 1, "the hold and its booking must not both be offered")

	booked := staged[0]
	assert.False(t, booked.Pending)
	assert.True(t, booked.Committable())
	assert.Equal(t, 17.43, booked.Amount, "the booked amount wins, not the hold")
	// The correction made while it was pending carried over — which is what
	// makes reviewing a reservation early worth doing.
	assert.Equal(t, domain.Category("Transport"), booked.Category)
	assert.Equal(t, "fuel", booked.Labels)

	// The hold is retired, not deleted, and points at what replaced it.
	closed, err := env.repo.GetStagedByExternalID(hold.ExternalID)
	require.NoError(t, err)
	assert.Equal(t, domain.StagedStateSuperseded, closed.State)
	require.NotNil(t, closed.SupersededBy)
	assert.Equal(t, booked.ID, *closed.SupersededBy)
}

// A hold the bank stops reporting was released, not booked — a hotel hold
// coming off, a pre-auth reversed. Leaving it on the queue forever is how a
// review list fills with things that never happened.
func TestReleasedReservationIsClosedOut(t *testing.T) {
	env := bankTestRouter(t, true)

	res := env.stage(t, []openbanking.Transaction{
		pendingRow("hold-2", "2026-09-16", "150.00", "D", "HOTEL BALTIJA", "Rezervacija"),
	})
	require.Equal(t, 1, res.Pending)
	hold := env.stagedByComment(t, "HOTEL BALTIJA (Rezervacija)")

	// The next sync re-reads the same window and the bank no longer has it.
	res = env.stage(t, []openbanking.Transaction{})
	assert.Equal(t, 1, res.Released)

	closed, err := env.repo.GetStagedByExternalID(hold.ExternalID)
	require.NoError(t, err)
	assert.Equal(t, domain.StagedStateSuperseded, closed.State)
	assert.Nil(t, closed.SupersededBy)
	assert.Contains(t, closed.VerdictNote, "released")
}

// Two holds at one merchant must be resolved by two bookings, not collapse
// onto one — otherwise a purchase silently disappears.
func TestTwoReservationsResolveToTwoBookings(t *testing.T) {
	env := bankTestRouter(t, true)

	env.stage(t, []openbanking.Transaction{
		pendingRow("hold-a", "2026-09-16", "4.00", "D", "CAFFEINE", "Kava"),
		pendingRow("hold-b", "2026-09-16", "9.00", "D", "CAFFEINE", "Kava"),
	})

	res := env.stage(t, []openbanking.Transaction{
		ebRow("book-a", "2026-09-17", "4.20", "D", "CAFFEINE", "Kava"),
		ebRow("book-b", "2026-09-17", "9.30", "D", "CAFFEINE", "Kava"),
	})
	assert.Equal(t, 2, res.Superseded)

	staged, _, err := env.repo.ListStaged(repository.StagedFilter{State: domain.StagedStateStaged})
	require.NoError(t, err)
	assert.Len(t, staged, 2, "two purchases in, two purchases out")
}

// A booking that resolves nothing is just a new row. Pairing must not reach
// across merchants, directions or months.
func TestBookingDoesNotClaimAnUnrelatedReservation(t *testing.T) {
	env := bankTestRouter(t, true)

	env.stage(t, []openbanking.Transaction{
		pendingRow("hold-3", "2026-09-16", "20.00", "D", "CIRCLE K VILNIUS", "Kuras"),
	})
	res := env.stage(t, []openbanking.Transaction{
		// The bank still reports the hold, and books something else that
		// happens to share its amount and sit a day later.
		pendingRow("hold-3", "2026-09-16", "20.00", "D", "CIRCLE K VILNIUS", "Kuras"),
		ebRow("book-3", "2026-09-17", "20.00", "D", "LIDL SNIPISKES", "Pirkiniai"),
	})
	assert.Equal(t, 0, res.Superseded)
	assert.Equal(t, 0, res.Released)

	staged, _, err := env.repo.ListStaged(repository.StagedFilter{State: domain.StagedStateStaged})
	require.NoError(t, err)
	assert.Len(t, staged, 2, "the hold is still waiting for its own booking")
}

// A reservation is never dedup'd against the ledger: it cannot be committed,
// so a consuming content match would spend the ledger row its own booking has
// to claim days later.
func TestReservationDoesNotConsumeLedgerDedup(t *testing.T) {
	env := bankTestRouter(t, true)
	env.seedHistory(t, domain.Transaction{
		Date:   time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC),
		Type:   domain.TransactionTypeExpense,
		Amount: 4.20, Category: "Food", Comment: "CAFFEINE (Kava)",
	})

	// The hold must not take the ledger match...
	env.stage(t, []openbanking.Transaction{
		pendingRow("hold-4", "2026-09-16", "4.20", "D", "CAFFEINE", "Kava"),
	})
	hold := env.stagedByComment(t, "CAFFEINE (Kava)")
	assert.Equal(t, domain.VerdictPending, hold.Verdict)

	// ...so its booking still sees it and is flagged as the duplicate it is.
	env.stage(t, []openbanking.Transaction{
		ebRow("book-4", "2026-09-16", "4.20", "D", "CAFFEINE", "Kava"),
	})
	staged, _, err := env.repo.ListStaged(repository.StagedFilter{State: domain.StagedStateStaged})
	require.NoError(t, err)
	require.Len(t, staged, 1)
	assert.Equal(t, domain.VerdictDuplicateContent, staged[0].Verdict)
}

// TestResyncReclassifiesUntouchedRows — a classifier fix has to reach rows
// already sitting in the queue.
//
// Otherwise a fix only ever helps rows fetched after it shipped, and a queue
// full of misread rows could only be cleared by dismissing them — which is
// permanent, so they would never come back corrected.
func TestResyncReclassifiesUntouchedRows(t *testing.T) {
	env := bankTestRouter(t, true)
	env.stage(t, bankFeed())

	row := env.stagedByComment(t, "UAB IGNITIS")
	// Simulate a row staged by an older, worse classifier.
	row.Category = "Entertainment"
	row.Comment = "Swedbank card fee"
	require.NoError(t, env.repo.SaveStaged(&row))

	env.stage(t, bankFeed())

	fixed, err := env.repo.GetStagedByExternalID(row.ExternalID)
	require.NoError(t, err)
	assert.Equal(t, "UAB IGNITIS", fixed.Comment)
	assert.Equal(t, domain.Category("Utilities"), fixed.Category)
	assert.Equal(t, row.ID, fixed.ID, "the staged row keeps its identity")
}

// ...but a correction a person made is never overwritten by one.
func TestResyncKeepsUserCorrections(t *testing.T) {
	env := bankTestRouter(t, true)
	env.stage(t, bankFeed())
	row := env.stagedByComment(t, "UAB IGNITIS")

	rec := bankJSON(t, env.r, "PUT", stagedPath(row.ID), map[string]any{
		"category": "Housing", "comment": "Electricity, the cabin",
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	env.stage(t, bankFeed())

	kept, err := env.repo.GetStagedByExternalID(row.ExternalID)
	require.NoError(t, err)
	assert.Equal(t, "Electricity, the cabin", kept.Comment)
	assert.Equal(t, domain.Category("Housing"), kept.Category)
	assert.True(t, kept.Edited)
}
