package service

import (
	"net/http"
	"strings"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

// This file holds the small provider-dialect layer. Everything else in the AI
// stack speaks one wire format — the Anthropic Messages API — because both
// supported backends expose it: nexos.ai through its Anthropic-native
// passthrough (which preserves prompt caching, unlike its OpenAI translation)
// and api.anthropic.com directly. The only things that actually differ are the
// authentication headers and the base URL, so that is all this layer abstracts.

// applyProviderAuth sets the authentication headers for the configured
// provider. The direct Claude API wants x-api-key plus a pinned API version;
// gateways in front of it (nexos.ai and friends) want a Bearer token.
func applyProviderAuth(req *http.Request, settings *domain.AISettings) {
	if settings.ResolvedProvider() == domain.ProviderAnthropic {
		req.Header.Set("x-api-key", settings.APIKey)
		req.Header.Set("anthropic-version", domain.AnthropicVersion)
		return
	}
	req.Header.Set("Authorization", "Bearer "+settings.APIKey)
}

// providerBaseURL is the trimmed API base to append /messages or /models to,
// falling back to the provider's own default when the user left it blank.
func providerBaseURL(settings *domain.AISettings) string {
	base := strings.TrimRight(strings.TrimSpace(settings.GatewayURL), "/")
	if base != "" {
		return base
	}
	if settings.ResolvedProvider() == domain.ProviderAnthropic {
		return domain.DefaultAnthropicURL
	}
	return domain.DefaultGatewayURL
}
