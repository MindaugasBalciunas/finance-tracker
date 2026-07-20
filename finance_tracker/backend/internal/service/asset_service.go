package service

import (
	"errors"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/mindaugas/finance-tracker/pkg/timeutil"
)

// CreateAssetInput is also used for updates: PUT replaces the full record.
type CreateAssetInput struct {
	Name          string           `json:"name" binding:"required"`
	Type          domain.AssetType `json:"type" binding:"required,oneof=vehicle real_estate solar other"`
	PurchaseDate  string           `json:"purchase_date"` // optional YYYY-MM-DD
	PurchasePrice float64          `json:"purchase_price" binding:"required,gt=0"`
	CurrentValue  float64          `json:"current_value" binding:"omitempty,gte=0"`
	ValuationDate string           `json:"valuation_date"`
	Notes         string           `json:"notes"`

	LoanRemaining     float64 `json:"loan_remaining" binding:"omitempty,gte=0"`
	LoanRemainingDate string  `json:"loan_remaining_date"`
	LoanRate          string  `json:"loan_rate"`
	LoanAccount       string  `json:"loan_account"`
	LoanPaidOffDate   string  `json:"loan_paid_off_date"`
}

type AssetService interface {
	Create(input CreateAssetInput) (*domain.Asset, error)
	GetByID(id uint) (*domain.Asset, error)
	Update(id uint, input CreateAssetInput) (*domain.Asset, error)
	Delete(id uint) error
	DeleteAll() error
	ListAll() ([]domain.Asset, error)
	ListSince(since time.Time) ([]domain.Asset, error)
	GetSummary() (*domain.AssetSummary, error)
}

type assetService struct {
	repo repository.AssetRepository
}

func NewAssetService(repo repository.AssetRepository) AssetService {
	return &assetService{repo: repo}
}

// parseOptionalDate returns nil for an empty string, an error for a malformed one.
func parseOptionalDate(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := timeutil.ParseDate(s)
	if err != nil {
		return nil, errors.New("invalid date format, use YYYY-MM-DD")
	}
	return &t, nil
}

func assetFromInput(input CreateAssetInput) (*domain.Asset, error) {
	purchaseDate, err := parseOptionalDate(input.PurchaseDate)
	if err != nil {
		return nil, err
	}
	valuationDate, err := parseOptionalDate(input.ValuationDate)
	if err != nil {
		return nil, err
	}
	loanRemainingDate, err := parseOptionalDate(input.LoanRemainingDate)
	if err != nil {
		return nil, err
	}
	loanPaidOffDate, err := parseOptionalDate(input.LoanPaidOffDate)
	if err != nil {
		return nil, err
	}

	currentValue := input.CurrentValue
	if currentValue == 0 {
		currentValue = input.PurchasePrice
	}

	a := &domain.Asset{
		Name:              input.Name,
		Type:              input.Type,
		PurchaseDate:      purchaseDate,
		PurchasePrice:     input.PurchasePrice,
		CurrentValue:      currentValue,
		ValuationDate:     valuationDate,
		Notes:             input.Notes,
		LoanRemaining:     input.LoanRemaining,
		LoanRemainingDate: loanRemainingDate,
		LoanRate:          input.LoanRate,
		LoanAccount:       input.LoanAccount,
		LoanPaidOffDate:   loanPaidOffDate,
	}
	return a, nil
}

func populateAsset(a *domain.Asset) {
	a.Equity = a.ComputeEquity()
}

func populateAssets(assets []domain.Asset) {
	for i := range assets {
		populateAsset(&assets[i])
	}
}

func (s *assetService) Create(input CreateAssetInput) (*domain.Asset, error) {
	a, err := assetFromInput(input)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(a); err != nil {
		return nil, err
	}
	populateAsset(a)
	return a, nil
}

func (s *assetService) GetByID(id uint) (*domain.Asset, error) {
	a, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	populateAsset(a)
	return a, nil
}

// Update fully replaces the asset's fields with the given input.
func (s *assetService) Update(id uint, input CreateAssetInput) (*domain.Asset, error) {
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	a, err := assetFromInput(input)
	if err != nil {
		return nil, err
	}
	a.ID = existing.ID
	a.CreatedAt = existing.CreatedAt
	if err := s.repo.Update(a); err != nil {
		return nil, err
	}
	populateAsset(a)
	return a, nil
}

func (s *assetService) Delete(id uint) error {
	if _, err := s.repo.GetByID(id); err != nil {
		return err
	}
	return s.repo.Delete(id)
}

func (s *assetService) DeleteAll() error {
	return s.repo.DeleteAll()
}

func (s *assetService) ListAll() ([]domain.Asset, error) {
	assets, err := s.repo.ListAll()
	if err != nil {
		return nil, err
	}
	populateAssets(assets)
	return assets, nil
}

func (s *assetService) ListSince(since time.Time) ([]domain.Asset, error) {
	assets, err := s.repo.ListSince(since)
	if err != nil {
		return nil, err
	}
	populateAssets(assets)
	return assets, nil
}

func (s *assetService) GetSummary() (*domain.AssetSummary, error) {
	assets, err := s.repo.ListAll()
	if err != nil {
		return nil, err
	}
	summary := &domain.AssetSummary{Count: len(assets)}
	for _, a := range assets {
		summary.TotalPurchasePrice += a.PurchasePrice
		summary.TotalValue += a.CurrentValue
		summary.TotalLoans += a.LoanRemaining
	}
	summary.NetEquity = summary.TotalValue - summary.TotalLoans
	return summary, nil
}
