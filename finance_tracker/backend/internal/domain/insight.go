package domain

import "time"

// AIInsight stores an LLM-generated financial overview
type AIInsight struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Content   string    `json:"content" gorm:"type:text;not null"`
	CreatedAt time.Time `json:"created_at"`
}

// AISettings is a single-row table configuring the LLM gateway used for AI
// analysis and chat. Calls go to the gateway's Anthropic-native Messages
// endpoint ({base}/messages — nexos.ai passthrough preserves prompt caching);
// any base URL exposing that API works. The key is write-only — json:"-" keeps it
// out of every response and export.
type AISettings struct {
	ID         uint      `gorm:"primarykey" json:"id"`
	GatewayURL string    `json:"gateway_url" gorm:"not null;default:''"`
	APIKey     string    `json:"-" gorm:"not null;default:''"`
	Model      string    `json:"model" gorm:"not null;default:''"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// AIContext is a single-row, user-authored markdown document describing who
// the user is and how the AI should advise them (their "CFO context"):
// income structure, investment framework, standing rules, open decisions,
// communication style. Every AI integration injects it as a system block and
// the MCP server exposes it read-only, so any client advises with the same
// personal framework.
type AIContext struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Content   string    `json:"content" gorm:"type:text;not null;default:''"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DefaultGatewayURL is the nexos.ai gateway API base.
const DefaultGatewayURL = "https://api.nexos.ai/v1"

// Configured reports whether the gateway can be called.
func (s *AISettings) Configured() bool {
	return s.APIKey != "" && s.Model != ""
}

// ChatMessage is one turn of the AI chat (role: user | assistant | system).
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

// AIActivity is a compact digest of one AI interaction — a chat exchange, a
// view review, an analysis, a tagging or rule-review run. It is the shared
// short-term memory of the AI integrations: each one logs a few hundred
// characters here, and prompts include the recent digests instead of full
// transcripts, so the model knows what was already said at minimal token
// cost.
type AIActivity struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Kind      string    `json:"kind" gorm:"not null;index"` // chat | view_summary | analysis | tagging | rule_review
	Scope     string    `json:"scope" gorm:"not null;default:''"`
	Content   string    `json:"content" gorm:"type:text;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"index"`
}
