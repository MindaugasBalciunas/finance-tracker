package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func doBearer(r http.Handler, method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// The API token is a read-only, allowlisted credential for machine clients
// (the MCP server): GET-only, data routes only, never exports or settings.
func TestAPITokenScope(t *testing.T) {
	r := authTestRouter(t)

	// Enable the lock and unlock a session.
	w := doJSON(r, "POST", "/api/v1/auth/pin/setup", map[string]string{"pin": "123456"}, "")
	require.Equal(t, 200, w.Code, w.Body.String())
	w = doJSON(r, "POST", "/api/v1/auth/pin/login", map[string]string{"pin": "123456"}, "")
	require.Equal(t, 200, w.Code)
	cookie := w.Header().Get("Set-Cookie")
	require.NotEmpty(t, cookie)

	// Minting requires an unlocked session.
	w = doJSON(r, "POST", "/api/v1/auth/token", nil, "")
	assert.Equal(t, 401, w.Code, "no session, no token minting")
	w = doJSON(r, "POST", "/api/v1/auth/token", nil, cookie)
	require.Equal(t, 200, w.Code, w.Body.String())
	var minted struct{ Token string }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &minted))
	require.NotEmpty(t, minted.Token)
	assert.Contains(t, minted.Token, "ftk_")

	// Status reports a token exists but never the token itself.
	w = doJSON(r, "GET", "/api/v1/auth/status", nil, cookie)
	assert.Contains(t, w.Body.String(), `"has_api_token":true`)
	assert.NotContains(t, w.Body.String(), minted.Token)

	// Allowlisted GET works.
	w = doBearer(r, "GET", "/api/v1/transactions", minted.Token)
	assert.Equal(t, 200, w.Code, w.Body.String())
	w = doBearer(r, "GET", "/api/v1/ai/report", minted.Token)
	assert.Equal(t, 200, w.Code)

	// Mutations are forbidden even on allowlisted paths.
	w = doBearer(r, "POST", "/api/v1/transactions", minted.Token)
	assert.Equal(t, 403, w.Code, "token must never mutate")

	// Sensitive reads are forbidden: exports carry the AI key, settings are config.
	w = doBearer(r, "GET", "/api/v1/export/finances.json", minted.Token)
	assert.Equal(t, 403, w.Code, "backups are not token-readable")
	w = doBearer(r, "GET", "/api/v1/ai/settings", minted.Token)
	assert.Equal(t, 403, w.Code, "AI settings are not token-readable")

	// Prefix boundaries hold: /labelsx must not ride the /labels prefix.
	w = doBearer(r, "GET", "/api/v1/labelsx", minted.Token)
	assert.Equal(t, 403, w.Code, "allowlist matches whole path segments")

	// Wrong token → 401, and no fallthrough to the locked response.
	w = doBearer(r, "GET", "/api/v1/transactions", "ftk_wrong")
	assert.Equal(t, 401, w.Code)

	// Revoking kills it immediately; re-minting rotates.
	w = doJSON(r, "DELETE", "/api/v1/auth/token", nil, cookie)
	require.Equal(t, 204, w.Code)
	w = doBearer(r, "GET", "/api/v1/transactions", minted.Token)
	assert.Equal(t, 401, w.Code, "revoked token is dead")
	w = doJSON(r, "GET", "/api/v1/auth/status", nil, cookie)
	assert.Contains(t, w.Body.String(), `"has_api_token":false`)
}
