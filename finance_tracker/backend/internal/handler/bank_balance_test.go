package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedSnapshot(t *testing.T, e *bankTestEnv, at time.Time, swed, seb float64) {
	t.Helper()
	require.NoError(t, e.db.Create(&domain.Balance{Date: at, Swed: swed, Seb: seb, Total: swed + seb}).Error)
}

func setBankBalance(t *testing.T, e *bankTestEnv, l *domain.BankAccountLink, amount float64, ref, fetched time.Time) {
	t.Helper()
	l.BankBalance = amount
	l.BankBalanceCurrency = "EUR"
	l.BankBalanceType = "ITBD"
	l.BankBalanceDate = &ref
	l.BankBalanceFetched = &fetched
	require.NoError(t, e.repo.SaveLink(l))
}

func latestSnapshot(t *testing.T, e *bankTestEnv) domain.Balance {
	t.Helper()
	var b domain.Balance
	require.NoError(t, e.db.Order("date desc, id desc").First(&b).Error)
	return b
}

// Two Swedbank accounts feed one "swed" column: the sheet gets their sum.
func TestApplyBankBalancesSumsLinksSharingAnAccount(t *testing.T) {
	e := bankTestRouter(t, true)
	now := time.Now()
	seedSnapshot(t, e, now.AddDate(0, 0, -3), 1000, 500)

	second := &domain.BankAccountLink{ConnectionID: e.link.ConnectionID, IdentificationHash: "hash-2",
		UID: "uid-2", IBAN: "LT040000000000004526", AccountKey: "swed"}
	require.NoError(t, e.repo.SaveLink(second))
	setBankBalance(t, e, e.link, 1200.50, now, now)
	setBankBalance(t, e, second, 300.25, now, now)

	res := e.h.applyBankBalances([]string{"swed"})
	require.Len(t, res, 1)
	assert.Equal(t, "swed", res[0].Account)
	assert.Equal(t, 1000.0, res[0].Before)
	assert.Equal(t, 1500.75, res[0].After)
	assert.True(t, res[0].Changed)

	snap := latestSnapshot(t, e)
	assert.Equal(t, 1500.75, snap.Swed)
	assert.Equal(t, 500.0, snap.Seb, "accounts the bank did not report stay as they were")
	assert.Equal(t, 2000.75, snap.Total)

	got, err := e.repo.GetLink(second.ID)
	require.NoError(t, err)
	require.NotNil(t, got.BalanceAppliedThrough)
}

// A sibling with a stale balance would produce a sum no bank ever stated.
func TestApplyBankBalancesSkipsWhenASiblingIsStale(t *testing.T) {
	e := bankTestRouter(t, true)
	now := time.Now()
	seedSnapshot(t, e, now.AddDate(0, 0, -3), 1000, 500)
	second := &domain.BankAccountLink{ConnectionID: e.link.ConnectionID, IdentificationHash: "hash-2",
		UID: "uid-2", AccountKey: "swed", DisplayName: "Savings"}
	require.NoError(t, e.repo.SaveLink(second))
	setBankBalance(t, e, e.link, 1200, now, now)
	setBankBalance(t, e, second, 300, now.AddDate(0, 0, -5), now.AddDate(0, 0, -5))

	res := e.h.applyBankBalances([]string{"swed"})
	require.Len(t, res, 1)
	assert.Contains(t, res[0].Skipped, "Savings")
	var n int64
	e.db.Model(&domain.Balance{}).Count(&n)
	assert.EqualValues(t, 1, n, "no snapshot written")
}

// Matching figures write nothing — a sync is not a trend point by itself.
func TestApplyBankBalancesNoSnapshotWhenUnchanged(t *testing.T) {
	e := bankTestRouter(t, true)
	now := time.Now()
	seedSnapshot(t, e, now.AddDate(0, 0, -1), 1000, 500)
	setBankBalance(t, e, e.link, 1000, now, now)

	res := e.h.applyBankBalances([]string{"swed"})
	require.Len(t, res, 1)
	assert.False(t, res[0].Changed)
	var n int64
	e.db.Model(&domain.Balance{}).Count(&n)
	assert.EqualValues(t, 1, n)
}

// A row already inside the applied bank balance must not move the account
// again when it is committed afterwards.
func TestCommitAfterBankBalanceDoesNotDoubleCount(t *testing.T) {
	e := bankTestRouter(t, true)
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	seedSnapshot(t, e, today.AddDate(0, 0, -3), 1000, 500)
	setBankBalance(t, e, e.link, 976.60, today, now)
	e.h.applyBankBalances([]string{"swed"})
	// Reload: a sync stages with the link it read from the database.
	fresh, ferr := e.repo.GetLink(e.link.ID)
	require.NoError(t, ferr)
	e.link = fresh

	feed := bankFeed()[1:2]
	feed[0].BookingDate = today.Format("2006-01-02")
	feed[0].ValueDate = feed[0].BookingDate
	e.stage(t, feed)
	staged, _, err := e.repo.ListStaged(repository.StagedFilter{})
	require.NoError(t, err)
	require.NotEmpty(t, staged)

	rec := bankJSON(t, e.r, "POST", "/api/v1/banking/staged/commit", map[string]any{"ids": []uint{staged[0].ID}})
	require.Equal(t, 200, rec.Code, rec.Body.String())
	var res commitResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	assert.Equal(t, 1, res.Imported)
	assert.Contains(t, res.BalanceNote, "already includes")
	assert.Equal(t, 976.60, latestSnapshot(t, e).Swed)
}
