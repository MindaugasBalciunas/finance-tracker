package domain

import (
	"net/url"
	"strings"
	"time"
)

// AIInsight stores an LLM-generated financial overview
type AIInsight struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Content   string    `json:"content" gorm:"type:text;not null"`
	CreatedAt time.Time `json:"created_at"`
}

// AISettings is a single-row table configuring the LLM provider used for AI
// analysis and chat. Calls go to the provider's Anthropic-native Messages
// endpoint ({base}/messages) — either the nexos.ai passthrough (which
// preserves prompt caching) or api.anthropic.com directly; any base URL
// exposing that API works. The key is write-only — json:"-" keeps it out of
// every response and export.
type AISettings struct {
	ID         uint   `gorm:"primarykey" json:"id"`
	GatewayURL string `json:"gateway_url" gorm:"not null;default:''"`
	APIKey     string `json:"-" gorm:"not null;default:''"`
	Model      string `json:"model" gorm:"not null;default:''"`
	// Provider selects the authentication dialect: ProviderAnthropic sends
	// the direct Claude API headers (x-api-key + anthropic-version),
	// ProviderGateway sends a Bearer token. Empty means "infer from the
	// URL", which is what every row written before v1.53.0 carries.
	Provider string `json:"provider" gorm:"not null;default:''"`
	// Disabled is the master switch for every AI feature, stored INVERTED on
	// purpose: the Go and SQL zero value then means "AI is on", so both a
	// freshly created row and a row written before this column existed come
	// back with AI available instead of silently going dark on upgrade. Read
	// it through Enabled(); the API speaks in terms of "enabled".
	Disabled  bool      `json:"-" gorm:"not null;default:0"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Enabled reports whether the AI master switch is on.
func (s *AISettings) Enabled() bool { return !s.Disabled }

// Provider dialects for AISettings.Provider.
const (
	// ProviderGateway is an OpenAI-style gateway (nexos.ai and friends)
	// exposing the Anthropic Messages API behind a Bearer token.
	ProviderGateway = "gateway"
	// ProviderAnthropic is the first-party Claude API at api.anthropic.com.
	ProviderAnthropic = "anthropic"
)

// ResolvedProvider returns the provider dialect to use, inferring it from the
// base URL when the field is unset (rows written before the provider column
// existed, or a user who only pasted a URL). Anything hosted under
// anthropic.com speaks the first-party dialect; everything else is a gateway.
func (s *AISettings) ResolvedProvider() string {
	switch s.Provider {
	case ProviderAnthropic, ProviderGateway:
		return s.Provider
	}
	host := strings.ToLower(strings.TrimSpace(s.GatewayURL))
	if u, err := url.Parse(host); err == nil && u.Host != "" {
		host = strings.ToLower(u.Hostname())
	}
	if host == "anthropic.com" || strings.HasSuffix(host, ".anthropic.com") {
		return ProviderAnthropic
	}
	return ProviderGateway
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

// DefaultAnthropicURL is the first-party Claude API base.
const DefaultAnthropicURL = "https://api.anthropic.com/v1"

// IsAnthropicModelID reports whether a model string is shaped like a Claude
// API model id. Anthropic ids are lowercase and hyphenated ("claude-opus-5");
// a gateway catalogue may instead list display names ("Claude Opus 5"), and
// pasting one of those into a direct-Claude config fails at request time with
// the opaque upstream message "model: Claude Opus 5". Catching the shape at
// save time turns that into something you can act on.
func IsAnthropicModelID(model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	for _, r := range model {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.':
		default:
			return false
		}
	}
	return true
}

// AnthropicVersion is the API version header the Claude API requires.
const AnthropicVersion = "2023-06-01"

// Configured reports whether the provider can be called. It does NOT consider
// Enabled — callers that must respect the master switch use Active().
func (s *AISettings) Configured() bool {
	return s.APIKey != "" && s.Model != ""
}

// Active reports whether AI features should run: configured AND switched on.
func (s *AISettings) Active() bool {
	return s.Enabled() && s.Configured()
}

// DefaultClaudeModel is what a direct-Claude install starts on when the user
// (or the add-on options) didn't name a model.
const DefaultClaudeModel = "claude-opus-5"

// AIForecast is a saved AI investment projection: the model's blended-return
// scenarios, contribution target, target allocation and narrative as one JSON
// document (Content). Persisted so the dashboard renders the forecast for
// free on every visit — only an explicit regenerate spends gateway tokens.
type AIForecast struct {
	ID      uint    `json:"id" gorm:"primaryKey;autoIncrement"`
	Content string  `json:"content" gorm:"type:text;not null"`
	Model   string  `json:"model" gorm:"not null;default:''"`
	CostUSD float64 `json:"cost_usd"`
	// CostEstimated records that CostUSD was computed from published list
	// prices rather than reported by the provider, so a forecast reloaded
	// months later still says what kind of number it is showing.
	CostEstimated bool      `json:"cost_estimated" gorm:"not null;default:false"`
	InputTokens   int       `json:"input_tokens"`
	OutputTokens  int       `json:"output_tokens"`
	CreatedAt     time.Time `json:"created_at"`
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

// AISpend is one gateway call's cost, recorded so the app can answer "what
// has the AI cost me" — and, with AITopUp, "how much is left".
//
// Anthropic publishes no balance endpoint (GET /v1/organizations/balance is a
// 404, and the Admin API reports spend, not balance), so a running ledger kept
// here is the only way to show a remaining figure at all. It is necessarily an
// estimate: see Estimated below, and note that anything else billed to the
// same API key — another app, another machine — is invisible to this ledger.
type AISpend struct {
	ID    uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	Kind  string `json:"kind" gorm:"not null;default:'other';index"` // chat | view_summary | forecast | analysis | tagging | rule_review | other
	Model string `json:"model" gorm:"not null;default:''"`
	// Provider the call went to, so a ledger that spans a gateway move stays
	// readable rather than silently mixing two billing relationships.
	Provider     string  `json:"provider" gorm:"not null;default:''"`
	CostUSD      float64 `json:"cost_usd" gorm:"not null;default:0"`
	InputTokens  int     `json:"input_tokens" gorm:"not null;default:0"`
	OutputTokens int     `json:"output_tokens" gorm:"not null;default:0"`
	// Estimated marks a cost computed from published list prices because the
	// provider reported none — every direct-Claude call, in practice.
	Estimated bool      `json:"estimated" gorm:"not null;default:false"`
	CreatedAt time.Time `json:"created_at" gorm:"index"`
}

// AITopUp is money the user says they added to their provider account. The
// app cannot observe this — nothing in the API reports it — so it is entered
// by hand, and the balance is only as current as the last entry.
type AITopUp struct {
	ID         uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	AmountUSD  float64   `json:"amount_usd" gorm:"not null"`
	Note       string    `json:"note" gorm:"not null;default:''"`
	OccurredOn time.Time `json:"occurred_on" gorm:"index"`
	CreatedAt  time.Time `json:"created_at"`
}
