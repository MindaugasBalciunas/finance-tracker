package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CrossOriginGuard rejects state-changing requests (POST/PUT/DELETE/…) that a
// browser sent on behalf of ANOTHER site — the classic CSRF shape, e.g. an
// attacker page posting a form or a no-cors fetch at this API.
//
// The SPA is always served same-origin (nginx in the add-on, Vite's proxy in
// development), so no legitimate browser caller is cross-origin. It matters
// most when the app lock is off: the auth middleware then lets every request
// through, and without this any web page the owner visits could delete data
// on a reachable instance (localhost:8080, the LAN address).
//
// Built on net/http's CrossOriginProtection: it trusts Sec-Fetch-Site (sent
// by every current browser, and unaffected by proxies rewriting Host) and
// falls back to comparing Origin with Host. Requests carrying neither header
// — curl, the MCP server, scripts — are not browser requests and pass.
func CrossOriginGuard() gin.HandlerFunc {
	cop := http.NewCrossOriginProtection()
	return func(c *gin.Context) {
		if err := cop.Check(c.Request); err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, ErrorResponse{Error: "cross-origin request blocked"})
			return
		}
		c.Next()
	}
}
