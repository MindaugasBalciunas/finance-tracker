package handler

import (
	"bytes"
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

func doBearerBody(r http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
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

// The read-WRITE token adds a narrow label/rule mutation allowlist on top of
// the read surface — and nothing more. It must never reach delete-all,
// settings or exports, and the read-only token must never gain write access.
func TestAPITokenRWScope(t *testing.T) {
	r := authTestRouter(t)

	w := doJSON(r, "POST", "/api/v1/auth/pin/setup", map[string]string{"pin": "123456"}, "")
	require.Equal(t, 200, w.Code, w.Body.String())
	w = doJSON(r, "POST", "/api/v1/auth/pin/login", map[string]string{"pin": "123456"}, "")
	require.Equal(t, 200, w.Code)
	cookie := w.Header().Get("Set-Cookie")
	require.NotEmpty(t, cookie)

	// Minting the RW token needs an unlocked session.
	w = doJSON(r, "POST", "/api/v1/auth/token/rw", nil, "")
	assert.Equal(t, 401, w.Code)
	w = doJSON(r, "POST", "/api/v1/auth/token/rw", nil, cookie)
	require.Equal(t, 200, w.Code, w.Body.String())
	var rw struct{ Token string }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rw))
	require.Contains(t, rw.Token, "ftkw_")

	w = doJSON(r, "GET", "/api/v1/auth/status", nil, cookie)
	assert.Contains(t, w.Body.String(), `"has_api_token_rw":true`)

	// Reads still work with the RW token.
	assert.Equal(t, 200, doBearer(r, "GET", "/api/v1/transactions", rw.Token).Code)

	// Allowlisted writes pass the middleware (reach the handler → not 401/403).
	w = doBearerBody(r, "POST", "/api/v1/ai/rule-review/apply", rw.Token, map[string]any{"items": []any{}})
	assert.NotEqual(t, 401, w.Code)
	assert.NotEqual(t, 403, w.Code, "rule-review/apply is in the write allowlist")
	w = doBearerBody(r, "POST", "/api/v1/labels/rename", rw.Token, map[string]any{"from": "zzznope", "to": "zzznope2"})
	assert.NotEqual(t, 403, w.Code, "labels/rename is in the write allowlist")

	// Out-of-scope mutations are forbidden even for the RW token.
	assert.Equal(t, 403, doBearer(r, "DELETE", "/api/v1/transactions?confirm=all", rw.Token).Code, "delete-all is never token-writable")
	assert.Equal(t, 403, doBearerBody(r, "POST", "/api/v1/balances", rw.Token, map[string]any{}).Code)
	assert.Equal(t, 403, doBearerBody(r, "PUT", "/api/v1/ai/settings", rw.Token, map[string]any{}).Code, "settings are never token-writable")

	// A read-only token can NOT use the write allowlist.
	w = doJSON(r, "POST", "/api/v1/auth/token", nil, cookie)
	require.Equal(t, 200, w.Code)
	var ro struct{ Token string }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ro))
	assert.Equal(t, 403, doBearerBody(r, "POST", "/api/v1/labels/rename", ro.Token, map[string]any{"from": "a", "to": "b"}).Code,
		"read-only token must never write")

	// Revoking the RW token kills it but leaves the read-only token alive.
	require.Equal(t, 204, doJSON(r, "DELETE", "/api/v1/auth/token/rw", nil, cookie).Code)
	assert.Equal(t, 401, doBearer(r, "GET", "/api/v1/transactions", rw.Token).Code, "revoked RW token is dead")
	assert.Equal(t, 200, doBearer(r, "GET", "/api/v1/transactions", ro.Token).Code, "read-only token unaffected")
}
