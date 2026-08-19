package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// streamJSONResult runs a slow producer (an AI/gateway call) and streams
// whitespace heartbeats to the client while it works, then writes the JSON
// result. Keeping bytes flowing stops any reverse proxy in front of the app
// from firing its read-timeout and returning a bare 504 on a long call — the
// backend keeps working and the answer still lands.
//
// The 200 is committed before the result is known, so a producer error is
// delivered as {"error": …} in the body rather than an HTTP status. Leading
// heartbeat whitespace is ignored by JSON parsers, so the final body still
// parses; clients on these endpoints must check the body's "error" field.
//
// Do input validation (and return real 4xx statuses) BEFORE calling this.
func streamJSONResult(c *gin.Context, produce func() (any, error)) {
	type outcome struct {
		val any
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		val, err := produce()
		done <- outcome{val: val, err: err}
	}()

	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.Writer.WriteHeader(http.StatusOK)
	flusher, _ := c.Writer.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case o := <-done:
			var payload any = o.val
			if o.err != nil {
				payload = gin.H{"error": o.err.Error()}
			}
			b, _ := json.Marshal(payload)
			_, _ = c.Writer.Write(b)
			return
		case <-ticker.C:
			// One space — ignored leading JSON whitespace — keeps the
			// connection active so proxy read-timeouts never fire.
			if _, err := c.Writer.Write([]byte(" ")); err != nil {
				return // client/proxy went away; stop heartbeating
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}
