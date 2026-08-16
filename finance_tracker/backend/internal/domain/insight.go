package domain

import "time"

// AIInsight stores an LLM-generated financial overview
type AIInsight struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Content   string    `json:"content" gorm:"type:text;not null"`
	CreatedAt time.Time `json:"created_at"`
}

// AISettings is a single-row table configuring the LLM gateway used for AI
// analysis and chat. The nexos.ai gateway exposes an OpenAI-compatible API;
// any compatible base URL works. The key is write-only — json:"-" keeps it
// out of every response and export.
type AISettings struct {
	ID         uint      `gorm:"primarykey" json:"id"`
	GatewayURL string    `json:"gateway_url" gorm:"not null;default:''"`
	APIKey     string    `json:"-" gorm:"not null;default:''"`
	Model      string    `json:"model" gorm:"not null;default:''"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// DefaultGatewayURL is the nexos.ai OpenAI-compatible endpoint.
const DefaultGatewayURL = "https://api.nexos.ai/v1"

// Configured reports whether the gateway can be called.
func (s *AISettings) Configured() bool {
	return s.APIKey != "" && s.Model != ""
}

// ChatMessage is one turn of the AI chat, OpenAI wire format.
type ChatMessage struct {
	Role    string `json:"role"` // "system" | "user" | "assistant"
	Content string `json:"content"`
}

// AIChatMessage is a persisted chat turn — history lives server-side so the
// conversation follows the user across phone and browser.
type AIChatMessage struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Role      string    `json:"role" gorm:"not null"` // "user" | "assistant"
	Content   string    `json:"content" gorm:"type:text;not null"`
	CreatedAt time.Time `json:"created_at"`
}
