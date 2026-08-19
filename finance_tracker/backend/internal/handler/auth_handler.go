package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/service"
)

const sessionCookie = "ft_session"
const sessionMaxAge = 7 * 24 * 60 * 60

type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

func (h *AuthHandler) RegisterRoutes(rg *gin.RouterGroup) {
	auth := rg.Group("/auth")
	{
		auth.GET("/status", h.Status)
		auth.POST("/pin/login", h.PinLogin)
		auth.POST("/pin/setup", h.PinSetup)
		auth.POST("/pin/disable", h.PinDisable)
		auth.POST("/logout", h.Logout)
		auth.POST("/webauthn/register/begin", h.WebauthnRegisterBegin)
		auth.POST("/webauthn/register/finish", h.WebauthnRegisterFinish)
		auth.POST("/webauthn/login/begin", h.WebauthnLoginBegin)
		auth.POST("/webauthn/login/finish", h.WebauthnLoginFinish)
		auth.GET("/webauthn/credentials", h.ListCredentials)
		auth.DELETE("/webauthn/credentials/:id", h.DeleteCredential)
		auth.POST("/token", h.GenerateToken)
		auth.DELETE("/token", h.RevokeToken)
		auth.POST("/token/rw", h.GenerateTokenRW)
		auth.DELETE("/token/rw", h.RevokeTokenRW)
	}
}

// apiTokenWriteRoutes is the NARROW set of mutating routes the read-write
// token may call (in addition to every read route). Deliberately limited to
// label/rule maintenance — never delete-all, settings, auth, or import/export.
// Matched as (METHOD, path-prefix) with the same segment-boundary rule as
// reads, so "/labels/rulesX" can't ride "/labels/rules".
var apiTokenWriteRoutes = []struct{ method, prefix string }{
	{"POST", "/api/v1/ai/rule-review/apply"},
	{"POST", "/api/v1/ai/label-reindex/apply"},
	{"POST", "/api/v1/labels/rename"},
	{"POST", "/api/v1/labels/delete"},
	{"DELETE", "/api/v1/labels/rules"},
}

func apiTokenWriteAllowed(method, path string) bool {
	for _, r := range apiTokenWriteRoutes {
		if method != r.method {
			continue
		}
		if path == r.prefix || (strings.HasPrefix(path, r.prefix) && path[len(r.prefix)] == '/') {
			return true
		}
	}
	return false
}

// apiTokenPrefixes is everything a bearer API token may read. The token is
// strictly read-only and scoped for insight tooling (the MCP server): no
// exports (they carry the AI key), no settings, no auth, no chat history,
// and — enforced separately — never anything but GET.
var apiTokenPrefixes = []string{
	"/api/v1/transactions",
	"/api/v1/labels",
	"/api/v1/budgets",
	"/api/v1/balances",
	"/api/v1/stocks",
	"/api/v1/assets",
	"/api/v1/insights",
	"/api/v1/ai/report",
	"/api/v1/ai/context",
	"/api/v1/health",
}

// apiTokenAllowed matches on path-segment boundaries so "/labelsX" or a
// future "/balances-admin" can never ride an allowlisted prefix.
func apiTokenAllowed(path string) bool {
	for _, prefix := range apiTokenPrefixes {
		if path == prefix || (strings.HasPrefix(path, prefix) && path[len(prefix)] == '/') {
			return true
		}
	}
	return false
}

func bearerToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	if after, ok := strings.CutPrefix(header, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

// Middleware blocks every /api/v1 route (except auth + health) with 401
// unless the app lock is disabled, the request carries a valid session, or
// it presents the read-only API token on an allowlisted GET route.
func (h *AuthHandler) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/v1/auth/") || p == "/api/v1/health" {
			c.Next()
			return
		}
		if !h.svc.Enabled() {
			c.Next()
			return
		}
		token, _ := c.Cookie(sessionCookie)
		if h.svc.ValidSession(token) {
			c.Next()
			return
		}
		if bearer := bearerToken(c); bearer != "" {
			// Read-write token: every read route, PLUS the narrow write
			// allowlist. Checked first because its ftkw_ prefix is distinct.
			if h.svc.ValidAPITokenRW(bearer) {
				if c.Request.Method == http.MethodGet && apiTokenAllowed(p) {
					c.Next()
					return
				}
				if apiTokenWriteAllowed(c.Request.Method, p) {
					c.Next()
					return
				}
				c.AbortWithStatusJSON(http.StatusForbidden, ErrorResponse{Error: "this route is outside the API token's scope"})
				return
			}
			if !h.svc.ValidAPIToken(bearer) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid API token"})
				return
			}
			if c.Request.Method != http.MethodGet || !apiTokenAllowed(p) {
				c.AbortWithStatusJSON(http.StatusForbidden, ErrorResponse{Error: "API token is read-only and limited to data routes"})
				return
			}
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: "locked"})
	}
}

// requireUnlocked guards sensitive auth-management endpoints themselves:
// when the lock is enabled, changing it requires an unlocked session.
func (h *AuthHandler) requireUnlocked(c *gin.Context) bool {
	if !h.svc.Enabled() {
		return true
	}
	token, _ := c.Cookie(sessionCookie)
	if h.svc.ValidSession(token) {
		return true
	}
	c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "locked"})
	return false
}

func (h *AuthHandler) hostAndOrigin(c *gin.Context) (string, string) {
	// Prefer the browser's Origin header: reverse proxies (Tailscale serve,
	// nginx) may rewrite Host, but Origin always names what the user sees —
	// and the WebAuthn RP ID must match that.
	if origin := c.GetHeader("Origin"); origin != "" {
		if u, err := url.Parse(origin); err == nil && u.Host != "" {
			return u.Host, origin
		}
	}
	host := c.Request.Host
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return host, scheme + "://" + host
}

// isHTTPS reports whether the request reached the user over TLS (directly or
// via a reverse proxy that sets X-Forwarded-Proto, e.g. Tailscale serve).
func isHTTPS(c *gin.Context) bool {
	return c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
}

func (h *AuthHandler) setSessionCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	// Secure when served over HTTPS; plain-HTTP LAN access still works.
	c.SetCookie(sessionCookie, token, sessionMaxAge, "/", "", isHTTPS(c), true)
}

func (h *AuthHandler) Status(c *gin.Context) {
	token, _ := c.Cookie(sessionCookie)
	c.JSON(http.StatusOK, h.svc.Status(token))
}

type pinRequest struct {
	Pin        string `json:"pin"`
	CurrentPin string `json:"current_pin"`
}

func (h *AuthHandler) PinLogin(c *gin.Context) {
	var req pinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request"})
		return
	}
	token, err := h.svc.VerifyPin(req.Pin)
	if err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, service.ErrTooManyAttempts) {
			status = http.StatusTooManyRequests
		}
		c.JSON(status, ErrorResponse{Error: err.Error()})
		return
	}
	h.setSessionCookie(c, token)
	c.JSON(http.StatusOK, gin.H{"unlocked": true})
}

func (h *AuthHandler) PinSetup(c *gin.Context) {
	if !h.requireUnlocked(c) {
		return
	}
	var req pinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request"})
		return
	}
	if err := h.svc.SetupPin(req.CurrentPin, req.Pin); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	// Setting up the lock invalidates all sessions — keep this one alive.
	token, err := h.svc.VerifyPin(req.Pin)
	if err == nil {
		h.setSessionCookie(c, token)
	}
	c.JSON(http.StatusOK, gin.H{"enabled": true})
}

func (h *AuthHandler) PinDisable(c *gin.Context) {
	// Disabling the lock is as sensitive as changing the PIN: require an
	// unlocked session on top of the PIN itself, so an anonymous caller
	// can't turn the lock off by guessing the PIN alone.
	if !h.requireUnlocked(c) {
		return
	}
	var req pinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request"})
		return
	}
	if err := h.svc.DisableLock(req.Pin); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrTooManyAttempts) {
			status = http.StatusTooManyRequests
		}
		c.JSON(status, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": false})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	token, _ := c.Cookie(sessionCookie)
	h.svc.DestroySession(token)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, "", -1, "/", "", isHTTPS(c), true)
	c.JSON(http.StatusOK, gin.H{"unlocked": false})
}

func (h *AuthHandler) WebauthnRegisterBegin(c *gin.Context) {
	if !h.requireUnlocked(c) {
		return
	}
	host, origin := h.hostAndOrigin(c)
	options, err := h.svc.BeginRegistration(host, origin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, options)
}

func (h *AuthHandler) WebauthnRegisterFinish(c *gin.Context) {
	if !h.requireUnlocked(c) {
		return
	}
	host, origin := h.hostAndOrigin(c)
	name := c.Query("name")
	if err := h.svc.FinishRegistration(host, origin, name, c.Request); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"registered": true})
}

func (h *AuthHandler) WebauthnLoginBegin(c *gin.Context) {
	host, origin := h.hostAndOrigin(c)
	options, err := h.svc.BeginLogin(host, origin)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, options)
}

func (h *AuthHandler) WebauthnLoginFinish(c *gin.Context) {
	host, origin := h.hostAndOrigin(c)
	token, err := h.svc.FinishLogin(host, origin, c.Request)
	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: err.Error()})
		return
	}
	h.setSessionCookie(c, token)
	c.JSON(http.StatusOK, gin.H{"unlocked": true})
}

func (h *AuthHandler) ListCredentials(c *gin.Context) {
	if !h.requireUnlocked(c) {
		return
	}
	creds, err := h.svc.ListCredentials()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, creds)
}

func (h *AuthHandler) DeleteCredential(c *gin.Context) {
	if !h.requireUnlocked(c) {
		return
	}
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	if err := h.svc.DeleteCredential(id); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// GenerateToken mints the read-only API token (for the MCP server). The
// plaintext is returned exactly once; only its hash is stored. Requires an
// unlocked session — the token cannot be minted from outside.
func (h *AuthHandler) GenerateToken(c *gin.Context) {
	if !h.requireUnlocked(c) {
		return
	}
	token, err := h.svc.GenerateAPIToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"note":  "Shown once — store it now. The token grants read-only access to financial data routes.",
	})
}

// RevokeToken invalidates the API token immediately.
func (h *AuthHandler) RevokeToken(c *gin.Context) {
	if !h.requireUnlocked(c) {
		return
	}
	if err := h.svc.RevokeAPIToken(); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// GenerateTokenRW mints the read-WRITE API token — an opt-in credential that
// can additionally call the label/rule maintenance routes. Returned once;
// only its hash is stored. Requires an unlocked session.
func (h *AuthHandler) GenerateTokenRW(c *gin.Context) {
	if !h.requireUnlocked(c) {
		return
	}
	token, err := h.svc.GenerateAPITokenRW()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"note":  "Shown once — store it now. This token can READ your data AND modify labels and auto-labeling rules. Keep it secret; revoke it if leaked.",
	})
}

// RevokeTokenRW invalidates the read-write API token immediately.
func (h *AuthHandler) RevokeTokenRW(c *gin.Context) {
	if !h.requireUnlocked(c) {
		return
	}
	if err := h.svc.RevokeAPITokenRW(); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
