package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ft/internal/ai"
	"ft/internal/notify"
)

// The phone drops the connection while the model is thinking: the answer
// still lands in the history and the Home Assistant webhook is called.
func TestChatContinuesAfterTheAppLeaves(t *testing.T) {
	s, c := newServer(t)
	release := make(chan struct{})
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		<-release
		w.Write([]byte(`{"content":[{"type":"text","text":"Spending is on track."}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`))
	}))
	defer model.Close()
	var hooks atomic.Int32
	var hookBody atomic.Value
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		hookBody.Store(string(b))
		hooks.Add(1)
	}))
	defer hook.Close()
	ai.SaveSettings(s.DB, ai.Settings{GatewayURL: model.URL, APIKey: "sk-ant-x", Model: "claude-opus-5-5", Provider: "anthropic", Enabled: true})
	if err := notify.Save(s.DB, notify.Settings{WebhookURL: hook.URL + "/api/webhook/finance-test-123", Enabled: true}); err != nil {
		t.Fatal(err)
	}

	// Ask, then hang up after a moment (the app went to the background).
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	body, _ := json.Marshal(map[string]string{"text": "How is my month?"})
	req, _ := http.NewRequestWithContext(ctx, "POST", c.base+"/ai/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if _, err := c.http.Do(req); err == nil {
		t.Fatal("expected the request to be cut off")
	}
	var st map[string]any
	c.ok("GET", "/ai/chat/status", nil, &st)
	if st["running"] != true || st["question"] != "How is my month?" {
		t.Fatalf("status while running: %v", st)
	}
	// A second question waits its turn.
	if code, _ := c.do("POST", "/ai/chat", map[string]string{"text": "And another?"}); code != http.StatusConflict {
		t.Fatalf("second question: %d", code)
	}
	// The status poll above means the app is watching again; leave once more.
	c.ok("POST", "/ai/chat/away", nil, nil)
	close(release)
	// Wait on the history (a backgrounded app doesn't poll the status).
	deadline := time.Now().Add(5 * time.Second)
	var hist []ai.ChatMessage
	for {
		c.ok("GET", "/ai/chat", nil, &hist)
		if len(hist) >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(hist) != 2 || hist[0].Content != "How is my month?" || hist[1].Content != "Spending is on track." {
		t.Fatalf("history: %+v", hist)
	}
	for time.Now().Before(deadline) && hooks.Load() == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	if hooks.Load() != 1 || !strings.Contains(hookBody.Load().(string), "answer is ready") || strings.Contains(hookBody.Load().(string), "on track") {
		t.Fatalf("webhook: %d %v (must not carry the answer itself)", hooks.Load(), hookBody.Load())
	}
}
