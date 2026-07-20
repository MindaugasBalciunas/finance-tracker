package service

import (
	"errors"
	"testing"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository/mock"
	"github.com/stretchr/testify/assert"
	tm "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestAssetService_Create(t *testing.T) {
	t.Run("creates asset with all fields", func(t *testing.T) {
		repo := &mock.AssetRepository{}
		svc := NewAssetService(repo)

		var created *domain.Asset
		repo.On("Create", tm.AnythingOfType("*domain.Asset")).
			Run(func(args tm.Arguments) { created = args.Get(0).(*domain.Asset) }).
			Return(nil)

		a, err := svc.Create(CreateAssetInput{
			Name:              "House Platiniškių 21A",
			Type:              domain.AssetTypeRealEstate,
			PurchaseDate:      "2022-08-17",
			PurchasePrice:     355000,
			CurrentValue:      410000,
			ValuationDate:     "2025-12-01",
			LoanRemaining:     263568.67,
			LoanRemainingDate: "2026-07-20",
			LoanRate:          "6M EURIBOR + 1.3%",
			LoanAccount:       "seb",
		})
		require.NoError(t, err)
		require.NotNil(t, created)

		assert.Equal(t, "House Platiniškių 21A", a.Name)
		assert.Equal(t, domain.AssetTypeRealEstate, a.Type)
		require.NotNil(t, a.PurchaseDate)
		assert.Equal(t, "2022-08-17", a.PurchaseDate.Format("2006-01-02"))
		assert.Equal(t, 355000.0, a.PurchasePrice)
		assert.Equal(t, 410000.0, a.CurrentValue)
		require.NotNil(t, a.ValuationDate)
		assert.Equal(t, "2025-12-01", a.ValuationDate.Format("2006-01-02"))
		assert.Equal(t, 263568.67, a.LoanRemaining)
		require.NotNil(t, a.LoanRemainingDate)
		assert.Equal(t, "2026-07-20", a.LoanRemainingDate.Format("2006-01-02"))
		assert.Equal(t, "6M EURIBOR + 1.3%", a.LoanRate)
		assert.Equal(t, "seb", a.LoanAccount)
		assert.InDelta(t, 410000-263568.67, a.Equity, 0.001, "equity = current value − loan remaining")
	})

	t.Run("current value defaults to purchase price when omitted", func(t *testing.T) {
		repo := &mock.AssetRepository{}
		svc := NewAssetService(repo)
		repo.On("Create", tm.AnythingOfType("*domain.Asset")).Return(nil)

		a, err := svc.Create(CreateAssetInput{
			Name:          "Toyota RAV4 Style Hybrid",
			Type:          domain.AssetTypeVehicle,
			PurchaseDate:  "2020-11-17",
			PurchasePrice: 34000,
		})
		require.NoError(t, err)
		assert.Equal(t, 34000.0, a.CurrentValue)
		assert.Equal(t, 34000.0, a.Equity, "no loan → equity equals current value")
	})

	t.Run("purchase date is optional", func(t *testing.T) {
		repo := &mock.AssetRepository{}
		svc := NewAssetService(repo)
		repo.On("Create", tm.AnythingOfType("*domain.Asset")).Return(nil)

		a, err := svc.Create(CreateAssetInput{
			Name:          "Solar panels 10.35 kWp",
			Type:          domain.AssetTypeSolar,
			PurchasePrice: 4551.50,
		})
		require.NoError(t, err)
		assert.Nil(t, a.PurchaseDate)
	})

	t.Run("rejects malformed date", func(t *testing.T) {
		repo := &mock.AssetRepository{}
		svc := NewAssetService(repo)

		_, err := svc.Create(CreateAssetInput{
			Name:          "X",
			Type:          domain.AssetTypeOther,
			PurchaseDate:  "17-11-2020",
			PurchasePrice: 100,
		})
		require.Error(t, err)
		repo.AssertNotCalled(t, "Create", tm.Anything)
	})

	t.Run("propagates repo error", func(t *testing.T) {
		repo := &mock.AssetRepository{}
		svc := NewAssetService(repo)
		repo.On("Create", tm.AnythingOfType("*domain.Asset")).Return(errors.New("db down"))

		_, err := svc.Create(CreateAssetInput{Name: "X", Type: domain.AssetTypeOther, PurchasePrice: 100})
		require.Error(t, err)
	})
}

func TestAssetService_Update(t *testing.T) {
	t.Run("full replace preserves ID and CreatedAt", func(t *testing.T) {
		repo := &mock.AssetRepository{}
		svc := NewAssetService(repo)

		createdAt := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
		repo.On("GetByID", uint(7)).Return(&domain.Asset{ID: 7, Name: "Old", CreatedAt: createdAt}, nil)

		var updated *domain.Asset
		repo.On("Update", tm.AnythingOfType("*domain.Asset")).
			Run(func(args tm.Arguments) { updated = args.Get(0).(*domain.Asset) }).
			Return(nil)

		a, err := svc.Update(7, CreateAssetInput{
			Name:          "Toyota RAV4 Style Hybrid",
			Type:          domain.AssetTypeVehicle,
			PurchasePrice: 34000,
			LoanRemaining: 0,
		})
		require.NoError(t, err)
		assert.Equal(t, uint(7), updated.ID)
		assert.Equal(t, createdAt, updated.CreatedAt)
		assert.Equal(t, "Toyota RAV4 Style Hybrid", a.Name)
		assert.Equal(t, 0.0, a.LoanRemaining, "loan can be cleared to zero on update")
	})

	t.Run("404 when asset missing", func(t *testing.T) {
		repo := &mock.AssetRepository{}
		svc := NewAssetService(repo)
		repo.On("GetByID", uint(99)).Return(nil, errors.New("not found"))

		_, err := svc.Update(99, CreateAssetInput{Name: "X", Type: domain.AssetTypeOther, PurchasePrice: 1})
		require.Error(t, err)
		repo.AssertNotCalled(t, "Update", tm.Anything)
	})
}

func TestAssetService_GetSummary(t *testing.T) {
	repo := &mock.AssetRepository{}
	svc := NewAssetService(repo)

	repo.On("ListAll").Return([]domain.Asset{
		{Name: "RAV4", PurchasePrice: 34000, CurrentValue: 34000},
		{Name: "House", PurchasePrice: 355000, CurrentValue: 410000, LoanRemaining: 263568.67},
		{Name: "Solar", PurchasePrice: 4551.50, CurrentValue: 4551.50},
	}, nil)

	s, err := svc.GetSummary()
	require.NoError(t, err)
	assert.Equal(t, 3, s.Count)
	assert.InDelta(t, 393551.50, s.TotalPurchasePrice, 0.001)
	assert.InDelta(t, 448551.50, s.TotalValue, 0.001)
	assert.InDelta(t, 263568.67, s.TotalLoans, 0.001)
	assert.InDelta(t, 184982.83, s.NetEquity, 0.001)
}

func TestAssetService_GetSummary_Empty(t *testing.T) {
	repo := &mock.AssetRepository{}
	svc := NewAssetService(repo)
	repo.On("ListAll").Return([]domain.Asset{}, nil)

	s, err := svc.GetSummary()
	require.NoError(t, err)
	assert.Equal(t, 0, s.Count)
	assert.Equal(t, 0.0, s.NetEquity)
}

func TestAssetService_ListAll_PopulatesEquity(t *testing.T) {
	repo := &mock.AssetRepository{}
	svc := NewAssetService(repo)
	repo.On("ListAll").Return([]domain.Asset{
		{Name: "House", CurrentValue: 410000, LoanRemaining: 263568.67},
	}, nil)

	assets, err := svc.ListAll()
	require.NoError(t, err)
	require.Len(t, assets, 1)
	assert.InDelta(t, 146431.33, assets[0].Equity, 0.001)
}

func TestAssetService_Delete(t *testing.T) {
	t.Run("deletes existing asset", func(t *testing.T) {
		repo := &mock.AssetRepository{}
		svc := NewAssetService(repo)
		repo.On("GetByID", uint(1)).Return(&domain.Asset{ID: 1}, nil)
		repo.On("Delete", uint(1)).Return(nil)
		require.NoError(t, svc.Delete(1))
	})

	t.Run("error when missing", func(t *testing.T) {
		repo := &mock.AssetRepository{}
		svc := NewAssetService(repo)
		repo.On("GetByID", uint(2)).Return(nil, errors.New("not found"))
		require.Error(t, svc.Delete(2))
		repo.AssertNotCalled(t, "Delete", tm.Anything)
	})
}
