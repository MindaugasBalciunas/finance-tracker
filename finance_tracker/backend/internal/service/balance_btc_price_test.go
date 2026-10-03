package service_test

import (
	"testing"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository/mock"
	"github.com/mindaugas/finance-tracker/internal/service"
	"github.com/stretchr/testify/assert"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// A snapshot saved before the form's live BTC price loaded arrives with
// btc_price 0 and a client total that leaves the coins out. It must carry
// the previous snapshot's price and recompute the total.
func TestBalanceService_Create_MissingBtcPriceInheritsPrevious(t *testing.T) {
	repo := &mock.BalanceRepository{}
	svc := newSvc(repo)
	repo.On("GetLatest").Return(&domain.Balance{BtcPrice: 75000}, nil)
	repo.On("Create", testifymock.AnythingOfType("*domain.Balance")).Return(nil)

	b, err := svc.Create(service.CreateBalanceInput{
		Date: "2026-09-29", Swed: 1000, RBTC: 0.02, Total: 1000,
	})
	require.NoError(t, err)
	assert.Equal(t, 75000.0, b.BtcPrice)
	assert.Equal(t, 2500.0, b.Total)
}

// A price the client did send is kept, and no lookup happens.
func TestBalanceService_Create_ExplicitBtcPriceKept(t *testing.T) {
	repo := &mock.BalanceRepository{}
	svc := newSvc(repo)
	repo.On("Create", testifymock.AnythingOfType("*domain.Balance")).Return(nil)

	b, err := svc.Create(service.CreateBalanceInput{Date: "2026-09-29", RBTC: 0.02, BtcPrice: 60000})
	require.NoError(t, err)
	assert.Equal(t, 1200.0, b.Total)
	repo.AssertNotCalled(t, "GetLatest")
}
