package domain

import "time"

// AuthSettings is a single-row table holding the app-lock configuration.
type AuthSettings struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	PinHash   string    `json:"-"`
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WebauthnCredential stores one enrolled authenticator (e.g. a phone's
// fingerprint or a laptop's Touch ID) as the marshaled library credential.
type WebauthnCredential struct {
	ID         uint      `gorm:"primarykey" json:"id"`
	Name       string    `json:"name"`
	Credential []byte    `json:"-"`
	CreatedAt  time.Time `json:"created_at"`
}
