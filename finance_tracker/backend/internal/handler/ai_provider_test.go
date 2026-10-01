package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// authCapture records the auth-related headers of the last upstream call, so
// the tests can assert which provider dialect was spoken.
type authCapture struct {
	Path      string
	Bearer    string
	APIKey    string
	Version   string
	Requested bool
}

// fakeProvider is an Anthropic-Messages stub that records how it was
// authenticated rather than what was asked.
func fakeProvider(t *testing.T, reply string) (*httptest.Server, *authCapture) {
	t.Helper()
	cap := &authCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.Requested = true
		cap.Path = r.URL.Path
		cap.Bearer = r.Header.Get("Authorization")
		cap.APIKey = r.Header.Get("x-api-key")
		cap.Version = r.Header.Get("anthropic-version")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(anthropicText(reply)))
	}))
	t.Cleanup(srv.Close)
	return srv, cap
}

func putSettings(t *testing.T, r http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequest("PUT", "/api/v1/ai/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func getSettings(t *testing.T, r http.Handler) map[string]any {
	t.Helper()
	req, _ := http.NewRequest("GET", "/api/v1/ai/settings", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out
}

// A gateway provider authenticates with a Bearer token, as nexos.ai expects.
func TestProvider_GatewaySendsBearer(t *testing.T) {
	r, _ := aiTestRouter(t)
	srv, cap := fakeProvider(t, "ok")

	require.Equal(t, 200, putSettings(t, r, `{"gateway_url":"`+srv.URL+`","model":"gpt-5","api_key":"nxs-key","provider":"gateway"}`).Code)

	req, _ := http.NewRequest("POST", "/api/v1/ai/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code, w.Body.String())

	assert.True(t, cap.Requested)
	assert.Equal(t, "/messages", cap.Path)
	assert.Equal(t, "Bearer nxs-key", cap.Bearer)
	assert.Empty(t, cap.APIKey, "a gateway must not receive the first-party header")
}

// The direct Claude API authenticates with x-api-key + a pinned version.
func TestProvider_AnthropicSendsAPIKeyAndVersion(t *testing.T) {
	r, _ := aiTestRouter(t)
	srv, cap := fakeProvider(t, "ok")

	require.Equal(t, 200, putSettings(t, r,
		`{"gateway_url":"`+srv.URL+`","model":"claude-opus-5","api_key":"sk-ant-123","provider":"anthropic"}`).Code)

	req, _ := http.NewRequest("POST", "/api/v1/ai/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code, w.Body.String())

	assert.Equal(t, "/messages", cap.Path)
	assert.Equal(t, "sk-ant-123", cap.APIKey)
	assert.Equal(t, domain.AnthropicVersion, cap.Version)
	assert.Empty(t, cap.Bearer, "the first-party API must not receive a Bearer token")
}

// A row written before the provider column existed carries provider:"" —
// the dialect is then inferred from the host, so existing nexos installs
// keep their Bearer auth and an api.anthropic.com URL just works.
func TestProvider_InferredFromURL(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://api.nexos.ai/v1", domain.ProviderGateway},
		{"https://api.anthropic.com/v1", domain.ProviderAnthropic},
		{"https://anthropic.com/v1", domain.ProviderAnthropic},
		{"http://localhost:1234/v1", domain.ProviderGateway},
		{"", domain.ProviderGateway},
	}
	for _, c := range cases {
		s := &domain.AISettings{GatewayURL: c.url}
		assert.Equal(t, c.want, s.ResolvedProvider(), c.url)
	}
	// An explicit provider always wins over the URL.
	s := &domain.AISettings{GatewayURL: "https://api.anthropic.com/v1", Provider: domain.ProviderGateway}
	assert.Equal(t, domain.ProviderGateway, s.ResolvedProvider())
}

// Switching to the direct Claude API without naming a URL lands on
// api.anthropic.com rather than the nexos default.
func TestProvider_DefaultURLFollowsProvider(t *testing.T) {
	r, _ := aiTestRouter(t)
	require.Equal(t, 200, putSettings(t, r, `{"gateway_url":"","model":"claude-opus-5","api_key":"sk-ant","provider":"anthropic"}`).Code)

	got := getSettings(t, r)
	assert.Equal(t, domain.DefaultAnthropicURL, got["gateway_url"])
	assert.Equal(t, domain.ProviderAnthropic, got["provider"])
}

func TestProvider_UnknownRejected(t *testing.T) {
	r, _ := aiTestRouter(t)
	w := putSettings(t, r, `{"model":"m","api_key":"k","provider":"openai"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "unknown provider")
}

// ---------------------------------------------------------------- enabled

// AI is on by default, so upgrading an existing install doesn't go dark.
func TestAIEnabled_DefaultsOn(t *testing.T) {
	r, _ := aiTestRouter(t)
	assert.Equal(t, true, getSettings(t, r)["enabled"])
}

// The master switch survives a round-trip. (GORM's default:1 on a bool would
// silently refuse to store false — the column is stored inverted for exactly
// this reason.)
func TestAIEnabled_OffPersists(t *testing.T) {
	r, db := aiTestRouter(t)
	require.Equal(t, 200, putSettings(t, r, `{"enabled":false}`).Code)
	assert.Equal(t, false, getSettings(t, r)["enabled"])

	var s domain.AISettings
	require.NoError(t, db.First(&s, 1).Error)
	assert.True(t, s.Disabled)
	assert.False(t, s.Enabled())

	require.Equal(t, 200, putSettings(t, r, `{"enabled":true}`).Code)
	assert.Equal(t, true, getSettings(t, r)["enabled"])
}

// A partial PUT (just the toggle) must not blank the saved URL/model/key.
func TestAIEnabled_TogglePreservesConfig(t *testing.T) {
	r, _ := aiTestRouter(t)
	require.Equal(t, 200, putSettings(t, r,
		`{"gateway_url":"https://api.nexos.ai/v1","model":"gpt-5","api_key":"nxs-keep","provider":"gateway"}`).Code)

	require.Equal(t, 200, putSettings(t, r, `{"enabled":false}`).Code)
	got := getSettings(t, r)
	assert.Equal(t, "https://api.nexos.ai/v1", got["gateway_url"])
	assert.Equal(t, "gpt-5", got["model"])
	assert.Equal(t, true, got["has_key"])
	assert.Equal(t, false, got["enabled"])
}

// With the switch off, AI endpoints are gone — the UI hides them, and the
// API (and MCP behind it) must not serve them either.
func TestAIEnabled_OffBlocksAIRoutes(t *testing.T) {
	r, _ := aiTestRouter(t)
	srv, cap := fakeProvider(t, "ok")
	require.Equal(t, 200, putSettings(t, r, `{"gateway_url":"`+srv.URL+`","model":"m","api_key":"k"}`).Code)
	require.Equal(t, 200, putSettings(t, r, `{"enabled":false}`).Code)

	for _, rt := range []struct{ method, path string }{
		{"POST", "/api/v1/ai/test"},
		{"POST", "/api/v1/ai/chat"},
		{"GET", "/api/v1/ai/chat/history"},
		{"GET", "/api/v1/ai/forecast"},
		{"GET", "/api/v1/ai/context"},
		{"POST", "/api/v1/insights/generate"},
		{"GET", "/api/v1/insights/latest"},
	} {
		req, _ := http.NewRequest(rt.method, rt.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code, "%s %s", rt.method, rt.path)
	}
	assert.False(t, cap.Requested, "no provider call may escape while AI is off")

	// Settings stay reachable — they are how AI gets switched back on.
	require.Equal(t, 200, putSettings(t, r, `{"enabled":true}`).Code)
	req, _ := http.NewRequest("POST", "/api/v1/ai/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code, w.Body.String())
}
