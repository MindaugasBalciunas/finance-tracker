package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/mindaugas/finance-tracker/internal/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func authTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.AuthSettings{}, &domain.WebauthnCredential{}))

	authHandler := NewAuthHandler(service.NewAuthService(repository.NewAuthRepository(db)))

	r := gin.New()
	v1 := r.Group("/api/v1")
	authHandler.RegisterRoutes(v1)
	v1.Use(authHandler.Middleware())
	v1.GET("/transactions", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	return r
}

func doJSON(r *gin.Engine, method, path string, body any, cookie string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func sessionCookieFrom(w *httptest.ResponseRecorder) string {
	for _, c := range w.Result().Cookies() {
		if c.Name == "ft_session" && c.Value != "" {
			return c.Name + "=" + c.Value
		}
	}
	return ""
}

func TestAuthFlow_PinLock(t *testing.T) {
	r := authTestRouter(t)

	// Lock disabled — API open, status says unlocked.
	w := doJSON(r, "GET", "/api/v1/transactions", nil, "")
	assert.Equal(t, 200, w.Code)
	w = doJSON(r, "GET", "/api/v1/auth/status", nil, "")
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), `"enabled":false`)
	assert.Contains(t, w.Body.String(), `"unlocked":true`)

	// Enable the lock.
	w = doJSON(r, "POST", "/api/v1/auth/pin/setup", map[string]string{"pin": "123456"}, "")
	require.Equal(t, 200, w.Code)
	setupCookie := sessionCookieFrom(w)
	require.NotEmpty(t, setupCookie, "setup should keep the current session alive")

	// Without a session the API is blocked.
	w = doJSON(r, "GET", "/api/v1/transactions", nil, "")
	assert.Equal(t, 401, w.Code)

	// Wrong PIN rejected.
	w = doJSON(r, "POST", "/api/v1/auth/pin/login", map[string]string{"pin": "000000"}, "")
	assert.Equal(t, 401, w.Code)

	// Correct PIN unlocks and the cookie opens the API.
	w = doJSON(r, "POST", "/api/v1/auth/pin/login", map[string]string{"pin": "123456"}, "")
	require.Equal(t, 200, w.Code)
	cookie := sessionCookieFrom(w)
	require.NotEmpty(t, cookie)

	w = doJSON(r, "GET", "/api/v1/transactions", nil, cookie)
	assert.Equal(t, 200, w.Code)

	// Changing the PIN requires the current one and kills old sessions.
	w = doJSON(r, "POST", "/api/v1/auth/pin/setup", map[string]string{"pin": "654321", "current_pin": "999999"}, cookie)
	assert.Equal(t, 400, w.Code)
	w = doJSON(r, "POST", "/api/v1/auth/pin/setup", map[string]string{"pin": "654321", "current_pin": "123456"}, cookie)
	require.Equal(t, 200, w.Code)
	newCookie := sessionCookieFrom(w)
	require.NotEmpty(t, newCookie)
	w = doJSON(r, "GET", "/api/v1/transactions", nil, cookie)
	assert.Equal(t, 401, w.Code, "old session must be invalid after PIN change")
	w = doJSON(r, "GET", "/api/v1/transactions", nil, newCookie)
	assert.Equal(t, 200, w.Code)

	// Logout kills the session.
	w = doJSON(r, "POST", "/api/v1/auth/logout", nil, newCookie)
	assert.Equal(t, 200, w.Code)
	w = doJSON(r, "GET", "/api/v1/transactions", nil, newCookie)
	assert.Equal(t, 401, w.Code)

	// Disable with wrong PIN fails; with right PIN the API opens again.
	w = doJSON(r, "POST", "/api/v1/auth/pin/disable", map[string]string{"pin": "111111"}, "")
	assert.Equal(t, 400, w.Code)
	w = doJSON(r, "POST", "/api/v1/auth/pin/disable", map[string]string{"pin": "654321"}, "")
	assert.Equal(t, 200, w.Code)
	w = doJSON(r, "GET", "/api/v1/transactions", nil, "")
	assert.Equal(t, 200, w.Code)
}

func TestAuthFlow_PinValidation(t *testing.T) {
	r := authTestRouter(t)

	// Too short / non-numeric PINs rejected.
	w := doJSON(r, "POST", "/api/v1/auth/pin/setup", map[string]string{"pin": "12"}, "")
	assert.Equal(t, 400, w.Code)
	w = doJSON(r, "POST", "/api/v1/auth/pin/setup", map[string]string{"pin": "abcdef"}, "")
	assert.Equal(t, 400, w.Code)
}

func TestAuthFlow_WebauthnBeginRequiresEnrollment(t *testing.T) {
	r := authTestRouter(t)

	// No credentials enrolled — login begin must fail cleanly.
	w := doJSON(r, "POST", "/api/v1/auth/webauthn/login/begin", nil, "")
	assert.Equal(t, 400, w.Code)

	// Registration begin works while unlocked (lock disabled) and returns options.
	w = doJSON(r, "POST", "/api/v1/auth/webauthn/register/begin", nil, "")
	require.Equal(t, 200, w.Code)
	var opts map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &opts))
	pub, ok := opts["publicKey"].(map[string]any)
	require.True(t, ok)
	assert.NotEmpty(t, pub["challenge"])
}

func TestAuthMiddleware_HealthAndAuthExempt(t *testing.T) {
	r := authTestRouter(t)
	w := doJSON(r, "POST", "/api/v1/auth/pin/setup", map[string]string{"pin": "123456"}, "")
	require.Equal(t, 200, w.Code)

	w = doJSON(r, "GET", "/api/v1/auth/status", nil, "")
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), `"enabled":true`)
	assert.Contains(t, w.Body.String(), `"unlocked":false`)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	// health isn't registered in this router, but the middleware must not 401 it
	assert.NotEqual(t, 401, rec.Code)
}
