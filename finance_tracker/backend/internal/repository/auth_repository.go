package repository

import (
	"github.com/mindaugas/finance-tracker/internal/domain"
	"gorm.io/gorm"
)

type AuthRepository interface {
	GetSettings() (*domain.AuthSettings, error)
	SaveSettings(s *domain.AuthSettings) error
	ListCredentials() ([]domain.WebauthnCredential, error)
	SaveCredential(c *domain.WebauthnCredential) error
	DeleteCredential(id uint) error
}

type authRepository struct {
	db *gorm.DB
}

func NewAuthRepository(db *gorm.DB) AuthRepository {
	return &authRepository{db: db}
}

func (r *authRepository) GetSettings() (*domain.AuthSettings, error) {
	var s domain.AuthSettings
	if err := r.db.FirstOrCreate(&s, domain.AuthSettings{ID: 1}).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *authRepository) SaveSettings(s *domain.AuthSettings) error {
	s.ID = 1
	return r.db.Save(s).Error
}

func (r *authRepository) ListCredentials() ([]domain.WebauthnCredential, error) {
	var creds []domain.WebauthnCredential
	if err := r.db.Order("id ASC").Find(&creds).Error; err != nil {
		return nil, err
	}
	return creds, nil
}

func (r *authRepository) SaveCredential(c *domain.WebauthnCredential) error {
	return r.db.Save(c).Error
}

func (r *authRepository) DeleteCredential(id uint) error {
	return r.db.Delete(&domain.WebauthnCredential{}, id).Error
}
