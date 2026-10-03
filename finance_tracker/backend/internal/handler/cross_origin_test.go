package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestCrossOriginGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CrossOriginGuard())
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.POST("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.DELETE("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	cases := []struct {
		name    string
		method  string
		headers map[string]string
		want    int
	}{
		{"same-origin SPA write", "POST", map[string]string{"Sec-Fetch-Site": "same-origin"}, 200},
		{"cross-site form post", "POST", map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		{"same-site other port", "DELETE", map[string]string{"Sec-Fetch-Site": "same-site"}, 403},
		{"cross-site read is harmless", "GET", map[string]string{"Sec-Fetch-Site": "cross-site"}, 200},
		{"old browser, foreign Origin", "POST", map[string]string{"Origin": "https://evil.example"}, 403},
		{"old browser, matching Origin", "POST", map[string]string{"Origin": "http://example.com"}, 200},
		{"non-browser client (curl, MCP)", "POST", nil, 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "http://example.com/x", nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			assert.Equal(t, tc.want, w.Code)
		})
	}
}
