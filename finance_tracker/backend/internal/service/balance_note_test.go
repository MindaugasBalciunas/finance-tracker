package service_test

import (
	"testing"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository/mock"
	"github.com/mindaugas/finance-tracker/internal/service"
	"github.com/stretchr/testify/assert"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// These tests cover the "I scanned a receipt and my balance didn't move"
// report: every path where the auto-snapshot declines must now say so on the
// transaction (BalanceNote) rather than return nil in silence, and the paths
// that previously skipped by accident must adjust.

func txToday(account string) *domain.Transaction {
	return &domain.Transaction{
		Date:         time.Now().Truncate(24 * time.Hour),
		Type:         domain.TransactionTypeExpense,
		Amount:       50,
		DebitAccount: account,
	}
}

func TestSnapshotFromTransaction_AdjustsBalance(t *testing.T) {
	repo := &mock.BalanceRepository{}
	svc := newSvc(repo)
	today := time.Now().Truncate(24 * time.Hour)

	repo.On("GetLatest").Return(&domain.Balance{ID: 7, Date: today, Swed: 1000, Total: 1000}, nil)
	var created *domain.Balance
	repo.On("Create", testifymock.Anything).Run(func(a testifymock.Arguments) {
		created = a.Get(0).(*domain.Balance)
	}).Return(nil)

	tx := txToday("swed")
	require.NoError(t, svc.SnapshotFromTransaction(tx))

	assert.Empty(t, tx.BalanceNote, "a balance that moved carries no note")
	require.NotNil(t, created)
	assert.Equal(t, 950.0, created.Swed)
	assert.Equal(t, 950.0, created.Total)
	assert.Zero(t, created.ID, "a new snapshot row, not an update of the old one")
}

func TestSnapshotFromTransaction_NotesEverySkip(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)

	t.Run("no snapshot to base on", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		repo.On("GetLatest").Return((*domain.Balance)(nil), gorm.ErrRecordNotFound)

		tx := txToday("swed")
		require.NoError(t, newSvc(repo).SnapshotFromTransaction(tx))
		assert.Contains(t, tx.BalanceNote, "no balance snapshot exists yet")
		repo.AssertNotCalled(t, "Create", testifymock.Anything)
	})

	t.Run("backdated before the latest snapshot", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		repo.On("GetLatest").Return(&domain.Balance{Date: today, Swed: 1000, Total: 1000}, nil)

		tx := txToday("swed")
		tx.Date = today.AddDate(0, 0, -5)
		require.NoError(t, newSvc(repo).SnapshotFromTransaction(tx))
		assert.Contains(t, tx.BalanceNote, "before your latest snapshot")
		repo.AssertNotCalled(t, "Create", testifymock.Anything)
	})

	t.Run("no account on the entry", func(t *testing.T) {
		repo := &mock.BalanceRepository{}
		repo.On("GetLatest").Return(&domain.Balance{Date: today, Swed: 1000, Total: 1000}, nil)

		tx := txToday("")
		require.NoError(t, newSvc(repo).SnapshotFromTransaction(tx))
		assert.Contains(t, tx.BalanceNote, "no account was set")
		repo.AssertNotCalled(t, "Create", testifymock.Anything)
	})
}

// An account code matching no column used to write a byte-identical clone of
// the previous snapshot: the trend gained a point, the balance didn't move.
func TestSnapshotFromTransaction_UnknownAccountFailsLoudly(t *testing.T) {
	repo := &mock.BalanceRepository{}
	today := time.Now().Truncate(24 * time.Hour)
	repo.On("GetLatest").Return(&domain.Balance{Date: today, Swed: 1000, Total: 1000}, nil)

	tx := txToday("Swedbank")
	err := newSvc(repo).SnapshotFromTransaction(tx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown debit_account")
	repo.AssertNotCalled(t, "Create", testifymock.Anything)
}

// ---------------------------------------------------------------- create

// The AI scan path writes whatever account string the model produced. A code
// that isn't real must be rejected at create time — stored, it saves fine,
// displays fine, and then never moves a balance.
func TestTransactionCreate_RejectsUnknownAccount(t *testing.T) {
	repo := &mock.TransactionRepository{}
	svc := service.NewTransactionService(repo, nil)

	_, err := svc.Create(service.CreateTransactionInput{
		Date: "2026-01-15", Type: domain.TransactionTypeExpense, Amount: 12,
		Category: domain.CategoryFood, DebitAccount: "swedbank",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown debit_account")
	repo.AssertNotCalled(t, "Create", testifymock.Anything)
}

func TestTransactionCreate_NormalizesAccountCase(t *testing.T) {
	repo := &mock.TransactionRepository{}
	repo.On("Create", testifymock.Anything).Return(nil)

	tx, err := service.NewTransactionService(repo, nil).Create(service.CreateTransactionInput{
		Date: "2026-01-15", Type: domain.TransactionTypeExpense, Amount: 12,
		Category: domain.CategoryFood, DebitAccount: " Swed ",
	})
	require.NoError(t, err)
	assert.Equal(t, "swed", tx.DebitAccount)
}

// The create response carries the skip reason, so the UI (and the AI chat
// tool, which reads the same field) can tell the user the balance stood still.
func TestTransactionCreate_SurfacesBalanceNote(t *testing.T) {
	txRepo := &mock.TransactionRepository{}
	txRepo.On("Create", testifymock.Anything).Return(nil)
	balRepo := &mock.BalanceRepository{}
	balRepo.On("GetLatest").Return((*domain.Balance)(nil), gorm.ErrRecordNotFound)

	svc := service.NewTransactionService(txRepo, newSvc(balRepo))
	tx, err := svc.Create(service.CreateTransactionInput{
		Date: time.Now().Format("2006-01-02"), Type: domain.TransactionTypeExpense,
		Amount: 12, Category: domain.CategoryFood, DebitAccount: "swed",
	})
	require.NoError(t, err)
	assert.Contains(t, tx.BalanceNote, "no balance snapshot exists yet")
}

// ---------------------------------------------------------------- update

// Adding the account a scan missed is the natural fix — and until now it left
// the balance permanently unadjusted, because only Create ever snapshotted.
func TestTransactionUpdate_AddingAccountAdjustsBalanceOnce(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	stored := &domain.Transaction{
		ID: 3, Date: today, Type: domain.TransactionTypeExpense, Amount: 50,
		Category: domain.CategoryFood,
	}
	txRepo := &mock.TransactionRepository{}
	txRepo.On("GetByID", uint(3)).Return(stored, nil)
	txRepo.On("Update", testifymock.Anything).Return(nil)

	balRepo := &mock.BalanceRepository{}
	balRepo.On("GetLatest").Return(&domain.Balance{Date: today, Swed: 1000, Total: 1000}, nil)
	creates := 0
	balRepo.On("Create", testifymock.Anything).Run(func(testifymock.Arguments) { creates++ }).Return(nil)

	svc := service.NewTransactionService(txRepo, newSvc(balRepo))
	acct := "swed"
	tx, err := svc.Update(3, service.UpdateTransactionInput{DebitAccount: &acct})
	require.NoError(t, err)
	assert.Empty(t, tx.BalanceNote)
	assert.Equal(t, 1, creates, "the delta is applied on the none → some transition")

	// Re-editing a row that already has linkage must not double-count.
	other := "seb"
	_, err = svc.Update(3, service.UpdateTransactionInput{DebitAccount: &other})
	require.NoError(t, err)
	assert.Equal(t, 1, creates, "an already-applied row is never re-applied")
}

func TestTransactionUpdate_RejectsUnknownAccount(t *testing.T) {
	txRepo := &mock.TransactionRepository{}
	txRepo.On("GetByID", uint(3)).Return(&domain.Transaction{ID: 3, Date: time.Now(), Amount: 10}, nil)

	bad := "revolut"
	_, err := service.NewTransactionService(txRepo, nil).Update(3, service.UpdateTransactionInput{CreditAccount: &bad})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown credit_account")
	txRepo.AssertNotCalled(t, "Update", testifymock.Anything)
}
