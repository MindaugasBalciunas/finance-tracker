package domain

import "time"

// AuthSettings is a single-row table holding the app-lock configuration.
type AuthSettings struct {
	ID      uint   `gorm:"primarykey" json:"id"`
	PinHash string `json:"-"`
	Enabled bool   `json:"enabled"`
	// APITokenHash is the SHA-256 of the read-only machine token (for the
	// MCP server and other API clients). Only the hash is ever stored.
	APITokenHash string `json:"-"`
	// APITokenRWHash is the SHA-256 of the read-WRITE machine token — a
	// separate, opt-in credential that can additionally hit a narrow
	// allowlist of label/rule mutation routes. Kept distinct so the
	// read-only token can never gain write access.
	APITokenRWHash string    `json:"-"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// WebauthnCredential stores one enrolled authenticator (e.g. a phone's
// fingerprint or a laptop's Touch ID) as the marshaled library credential.
type WebauthnCredential struct {
	ID         uint      `gorm:"primarykey" json:"id"`
	Name       string    `json:"name"`
	Credential []byte    `json:"-"`
	CreatedAt  time.Time `json:"created_at"`
}
