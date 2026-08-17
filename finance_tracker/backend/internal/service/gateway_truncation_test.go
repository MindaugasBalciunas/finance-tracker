package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

func TestLengthFinishSurfacesTruncation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "Your spending was"},
				"finish_reason": "length",
			}},
		})
	}))
	defer srv.Close()
	settings := &domain.AISettings{GatewayURL: srv.URL, APIKey: "k", Model: "m"}
	msg, err := callGatewayFull(settings, []gatewayMessage{{Role: "user", Content: "x"}}, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg.Content, "truncated") {
		t.Fatalf("expected truncation marker, got: %q", msg.Content)
	}
}
