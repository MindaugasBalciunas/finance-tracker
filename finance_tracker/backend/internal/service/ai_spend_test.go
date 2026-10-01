package service

import (
	"math"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
)

func spendService(t *testing.T) (*insightService, repository.InsightRepository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.AISpend{}, &domain.AITopUp{}, &domain.AISettings{}))
	repo := repository.NewInsightRepository(db)
	return &insightService{repo: repo}, repo, db
}

func spendAt(t *testing.T, db *gorm.DB, cost float64, kind string, when time.Time) {
	t.Helper()
	rec := &domain.AISpend{Kind: kind, CostUSD: cost, Estimated: true}
	require.NoError(t, db.Create(rec).Error)
	// CreatedAt is set by GORM, so back-date it explicitly.
	require.NoError(t, db.Model(rec).UpdateColumn("created_at", when).Error)
}

// Without a top-up there is nothing to count down from. Showing 0 would read
// as "you are out of credit", which is a different and alarming claim.
func TestRemainingIsAbsentUntilATopUpExists(t *testing.T) {
	svc, _, db := spendService(t)
	spendAt(t, db, 1.25, "chat", time.Now())

	rep, err := svc.SpendReport()
	require.NoError(t, err)
	require.Nil(t, rep.Remaining, "no top-up recorded yet, so there is no balance to report")
	require.InDelta(t, 1.25, rep.AllTime, 1e-9)
}

// The balance counts down from the first top-up. Spend before that was paid
// for by money this ledger never saw — charging it would understate what is
// left, and the number has to be defensible to be worth showing at all.
func TestRemainingIgnoresSpendBeforeTheFirstTopUp(t *testing.T) {
	svc, _, db := spendService(t)
	now := time.Now()
	topUpDay := now.AddDate(0, 0, -10)

	spendAt(t, db, 7.00, "chat", now.AddDate(0, 0, -30)) // long before — not counted
	spendAt(t, db, 2.00, "chat", now.AddDate(0, 0, -5))  // after the top-up
	spendAt(t, db, 0.50, "forecast", now.AddDate(0, 0, -1))

	_, err := svc.AddTopUp(20, "console top-up", topUpDay)
	require.NoError(t, err)

	rep, err := svc.SpendReport()
	require.NoError(t, err)
	require.NotNil(t, rep.Remaining)
	require.InDelta(t, 17.50, *rep.Remaining, 1e-9, "20 - (2.00 + 0.50), not 20 - 9.50")
	require.InDelta(t, 20.0, rep.ToppedUp, 1e-9)
	require.InDelta(t, 9.50, rep.AllTime, 1e-9, "all-time spend still counts everything")
	require.Equal(t, topUpDay.Format("2006-01-02"), rep.SinceDate)
}

func TestTopUpsAccumulate(t *testing.T) {
	svc, _, db := spendService(t)
	now := time.Now()
	_, err := svc.AddTopUp(20, "first", now.AddDate(0, 0, -20))
	require.NoError(t, err)
	_, err = svc.AddTopUp(30, "second", now.AddDate(0, 0, -2))
	require.NoError(t, err)
	spendAt(t, db, 12.0, "chat", now.AddDate(0, 0, -10))

	rep, err := svc.SpendReport()
	require.NoError(t, err)
	require.InDelta(t, 50.0, rep.ToppedUp, 1e-9)
	require.NotNil(t, rep.Remaining)
	require.InDelta(t, 38.0, *rep.Remaining, 1e-9)
}

func TestSpendWindowsAndBreakdown(t *testing.T) {
	svc, _, db := spendService(t)
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	spendAt(t, db, 1.0, "chat", now.Add(-2*time.Hour))                // this month + 30d
	spendAt(t, db, 2.0, "view_summary", monthStart.AddDate(0, 0, -3)) // last month, within 30d
	spendAt(t, db, 4.0, "forecast", now.AddDate(0, 0, -200))          // all time only

	rep, err := svc.SpendReport()
	require.NoError(t, err)
	require.InDelta(t, 1.0, rep.ThisMonth, 1e-9)
	require.InDelta(t, 3.0, rep.Last30, 1e-9)
	require.InDelta(t, 7.0, rep.AllTime, 1e-9)
	require.InDelta(t, 1.0, rep.ByKind["chat"], 1e-9)
	require.InDelta(t, 4.0, rep.ByKind["forecast"], 1e-9)
}

func TestTopUpValidationAndDelete(t *testing.T) {
	svc, _, _ := spendService(t)
	for _, bad := range []float64{0, -5} {
		if _, err := svc.AddTopUp(bad, "", time.Now()); err == nil {
			t.Errorf("amount %v should be rejected", bad)
		}
	}
	tu, err := svc.AddTopUp(10, "test", time.Time{})
	require.NoError(t, err)
	require.False(t, tu.OccurredOn.IsZero(), "a missing date defaults to today")

	require.NoError(t, svc.DeleteTopUp(tu.ID))
	rep, err := svc.SpendReport()
	require.NoError(t, err)
	require.Nil(t, rep.Remaining, "the last top-up is gone, so there is nothing to count down from")
}

// Floating-point money: a long ledger must not drift visibly.
func TestManySmallCallsSumCleanly(t *testing.T) {
	svc, _, db := spendService(t)
	now := time.Now()
	_, err := svc.AddTopUp(5, "", now.AddDate(0, 0, -1))
	require.NoError(t, err)
	for i := 0; i < 1000; i++ {
		spendAt(t, db, 0.001, "chat", now)
	}
	rep, err := svc.SpendReport()
	require.NoError(t, err)
	require.NotNil(t, rep.Remaining)
	if math.Abs(*rep.Remaining-4.0) > 0.0001 {
		t.Errorf("1000 x $0.001 against a $5 top-up should leave $4.00, got $%.6f", *rep.Remaining)
	}
}
