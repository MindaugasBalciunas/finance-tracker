// Package ai is the CFO assistant: a chat that answers from the live data
// through tools, receipt scanning, and spend tracking.
//
// Wire format: the Anthropic Messages API over HTTP, for both supported
// providers — api.anthropic.com directly (x-api-key) and gateways such as
// nexos.ai that pass the Messages API through (Bearer token). Only the auth
// header and the base URL differ, so one small client covers both.
package ai

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"ft/internal/db"
)

const (
	AnthropicVersion    = "2023-06-01"
	DefaultAnthropicURL = "https://api.anthropic.com/v1"
	DefaultGatewayURL   = "https://api.nexos.ai/v1"
	DefaultModel        = "claude-opus-5-5"
	// Current models take the dynamic-filtering web search variant.
	webSearchType = "web_search_20260209"
)

type Settings struct {
	GatewayURL string `json:"gateway_url"`
	APIKey     string `json:"api_key"`
	Model      string `json:"model"`
	Provider   string `json:"provider"` // anthropic | gateway | '' (infer)
	Enabled    bool   `json:"enabled"`
}

func (s Settings) ResolvedProvider() string {
	switch s.Provider {
	case "anthropic", "gateway":
		return s.Provider
	}
	if strings.Contains(s.GatewayURL, "anthropic.com") || (s.GatewayURL == "" && strings.HasPrefix(s.APIKey, "sk-ant-")) {
		return "anthropic"
	}
	return "gateway"
}

func (s Settings) BaseURL() string {
	if b := strings.TrimRight(strings.TrimSpace(s.GatewayURL), "/"); b != "" {
		return b
	}
	if s.ResolvedProvider() == "anthropic" {
		return DefaultAnthropicURL
	}
	return DefaultGatewayURL
}

func (s Settings) Configured() bool { return s.APIKey != "" && s.Model != "" }

// LoadSettings reads AI settings, seeding from the add-on options (env) when
// the database has no key — a reinstalled instance comes back with AI working.
func LoadSettings(q interface{ QueryRow(string, ...any) *sql.Row }) Settings {
	s := Settings{Enabled: true}
	var raw string
	if q.QueryRow(`SELECT value FROM settings WHERE key='ai'`).Scan(&raw) == nil {
		json.Unmarshal([]byte(raw), &s)
	}
	if s.APIKey == "" {
		if k := os.Getenv("ANTHROPIC_API_KEY"); k != "" {
			s.APIKey, s.Provider = k, "anthropic"
		} else if k := os.Getenv("NEXOS_API_KEY"); k != "" {
			s.APIKey, s.Provider = k, "gateway"
			s.GatewayURL = os.Getenv("NEXOS_GATEWAY_URL")
			s.Model = os.Getenv("NEXOS_MODEL")
		}
	}
	if s.Model == "" {
		s.Model = DefaultModel
	}
	return s
}

func SaveSettings(e interface {
	Exec(string, ...any) (sql.Result, error)
}, s Settings) error {
	raw, _ := json.Marshal(s)
	_, err := e.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('ai',?)`, string(raw))
	return err
}

func (s Settings) require() error {
	if !s.Enabled {
		return errors.New("AI features are turned off — enable them in Settings → AI")
	}
	if !s.Configured() {
		return errors.New("AI is not configured — add your API key and model in Settings → AI")
	}
	return nil
}

// ── wire types ──────────────────────────────────────────────────────

type block struct {
	Type         string          `json:"type"`
	Text         string          `json:"text,omitempty"`
	ID           string          `json:"id,omitempty"`
	Name         string          `json:"name,omitempty"`
	Input        json.RawMessage `json:"input,omitempty"`
	ToolUseID    string          `json:"tool_use_id,omitempty"`
	Content      string          `json:"content,omitempty"`
	IsError      bool            `json:"is_error,omitempty"`
	Source       *imageSource    `json:"source,omitempty"`
	CacheControl *cacheControl   `json:"cache_control,omitempty"`
}

type imageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type cacheControl struct {
	Type string `json:"type"`
}

type message struct {
	Role    string  `json:"role"`
	Content []block `json:"content"`
}

type tool struct {
	Type        string         `json:"type,omitempty"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
	MaxUses     int            `json:"max_uses,omitempty"`
}

type request struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    []block   `json:"system,omitempty"`
	Messages  []message `json:"messages"`
	Tools     []tool    `json:"tools,omitempty"`
}

// respBlock is narrower than block: server-tool result blocks carry content
// as an array, which must not fail the decode.
type respBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type response struct {
	Content    []respBlock `json:"content"`
	StopReason string      `json:"stop_reason"`
	Usage      struct {
		InputTokens        int     `json:"input_tokens"`
		OutputTokens       int     `json:"output_tokens"`
		CacheCreationInput int     `json:"cache_creation_input_tokens"`
		CacheReadInput     int     `json:"cache_read_input_tokens"`
		NexosCreditsCost   float64 `json:"nexos_credits_cost"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type Usage struct {
	CostUSD      float64 `json:"cost_usd"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	Estimated    bool    `json:"estimated"`
}

// Per-MTok USD prices for estimating cost when the provider does not bill
// back a figure.
var prices = map[string]struct{ in, out, cacheRead float64 }{
	"claude-fable-5-1": {10, 50, 0.25}, "claude-mythos-5-1": {10, 50, 0.25}, "claude-fable-5": {10, 50, 1.00},
	"claude-opus-5-5": {4, 20, 0.20}, "claude-opus-5": {5, 25, 0.50}, "claude-opus-4-8": {5, 25, 0.50},
	"claude-opus-4-7": {5, 25, 0.50}, "claude-opus-4-6": {5, 25, 0.50}, "claude-sonnet-5-5": {2, 10, 0.20},
	"claude-sonnet-5": {2, 10, 0.20}, "claude-sonnet-4-6": {3, 15, 0.30}, "claude-haiku-4-5": {1, 5, 0.10},
}

func estimate(model string, in, out, cacheRead, cacheWrite int) (float64, bool) {
	k := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndex(k, "/"); i >= 0 {
		k = k[i+1:]
	}
	k = strings.ReplaceAll(k, " ", "-")
	p, ok := prices[k]
	if !ok {
		return 0, false
	}
	return (float64(in)*p.in + float64(out)*p.out + float64(cacheRead)*p.cacheRead + float64(cacheWrite)*p.in*1.25) / 1e6, true
}

// Client calls the configured provider and records spend.
type Client struct {
	DB   *sql.DB
	HTTP *http.Client
}

type reply struct {
	Text      string
	ToolCalls []respBlock
	Raw       []respBlock
	Usage     Usage
	Stop      string
}

func (c *Client) call(ctx context.Context, s Settings, req request, kind string) (reply, error) {
	var zero reply
	body, err := json.Marshal(req)
	if err != nil {
		return zero, err
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 180 * time.Second}
	}
	var resp *http.Response
	for attempt := 1; ; attempt++ {
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL()+"/messages", bytes.NewReader(body))
		if err != nil {
			return zero, err
		}
		if s.ResolvedProvider() == "anthropic" {
			r.Header.Set("x-api-key", s.APIKey)
			r.Header.Set("anthropic-version", AnthropicVersion)
		} else {
			r.Header.Set("Authorization", "Bearer "+s.APIKey)
		}
		r.Header.Set("Content-Type", "application/json")
		resp, err = hc.Do(r)
		if err != nil {
			return zero, err
		}
		transient := resp.StatusCode == 429 || resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 529
		if !transient || attempt == 3 {
			break
		}
		delay := time.Duration(attempt) * 2 * time.Second
		if ra, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && ra > 0 && ra <= 60 {
			delay = time.Duration(ra) * time.Second
		}
		resp.Body.Close()
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(delay):
		}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return zero, err
	}
	var out response
	jerr := json.Unmarshal(raw, &out)
	if jerr == nil && out.Error != nil && out.Error.Message != "" {
		msg := out.Error.Message
		if strings.HasPrefix(msg, "model:") {
			msg += " — the provider does not offer this model; pick one from the list in Settings → AI"
		}
		return zero, fmt.Errorf("AI provider: %s", msg)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return zero, fmt.Errorf("AI provider returned %s", resp.Status)
	}
	if jerr != nil {
		return zero, errors.New("AI provider returned an unreadable response")
	}
	u := out.Usage
	cost, est := u.NexosCreditsCost, false
	if cost == 0 {
		cost, est = estimate(s.Model, u.InputTokens, u.OutputTokens, u.CacheReadInput, u.CacheCreationInput)
	}
	rep := reply{Raw: out.Content, Stop: out.StopReason, Usage: Usage{CostUSD: cost, InputTokens: u.InputTokens + u.CacheReadInput + u.CacheCreationInput,
		OutputTokens: u.OutputTokens, Estimated: est}}
	if c.DB != nil {
		c.DB.Exec(`INSERT INTO ai_spend(kind,model,cost_usd,input_tokens,output_tokens,estimated,created_at) VALUES(?,?,?,?,?,?,?)`,
			kind, s.Model, cost, rep.Usage.InputTokens, rep.Usage.OutputTokens, est, db.Now())
	}
	log.Printf("ai: %s in=%d out=%d cache_read=%d cost=$%.4f", kind, u.InputTokens, u.OutputTokens, u.CacheReadInput, cost)
	for _, b := range out.Content {
		switch b.Type {
		case "text":
			rep.Text += b.Text
		case "tool_use":
			rep.ToolCalls = append(rep.ToolCalls, b)
		}
	}
	if out.StopReason == "refusal" {
		return rep, errors.New("the model declined to answer this request")
	}
	if out.StopReason == "max_tokens" && strings.TrimSpace(rep.Text) != "" {
		rep.Text += "\n\n⚠ [answer truncated — the model hit its output limit]"
	}
	return rep, nil
}

// Models lists the models the provider offers.
func (c *Client) Models(ctx context.Context, s Settings) ([]string, error) {
	if s.APIKey == "" {
		return nil, errors.New("add an API key first")
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL()+"/models?limit=100", nil)
	if err != nil {
		return nil, err
	}
	if s.ResolvedProvider() == "anthropic" {
		r.Header.Set("x-api-key", s.APIKey)
		r.Header.Set("anthropic-version", AnthropicVersion)
	} else {
		r.Header.Set("Authorization", "Bearer "+s.APIKey)
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("provider returned %s for /models", resp.Status)
	}
	var body struct {
		Data   []json.RawMessage `json:"data"`
		Models []json.RawMessage `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, item := range append(body.Data, body.Models...) {
		var id string
		if json.Unmarshal(item, &id) != nil {
			var o struct {
				ID string `json:"id"`
			}
			json.Unmarshal(item, &o)
			id = o.ID
		}
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Test sends a one-line request to prove the settings work.
func (c *Client) Test(ctx context.Context, s Settings) error {
	if err := s.require(); err != nil {
		return err
	}
	_, err := c.call(ctx, s, request{Model: s.Model, MaxTokens: 256, Messages: []message{{Role: "user", Content: []block{{Type: "text", Text: "Reply with OK."}}}}}, "test")
	return err
}

// Spend summarises AI cost: this month, all time, balance from top-ups.
type Spend struct {
	MonthUSD   float64 `json:"month_usd"`
	TotalUSD   float64 `json:"total_usd"`
	TopupsUSD  float64 `json:"topups_usd"`
	BalanceUSD float64 `json:"balance_usd"`
	Calls      int     `json:"calls"`
	ByKind     map[string]float64 `json:"by_kind"`
}

func (c *Client) Spend() Spend {
	s := Spend{ByKind: map[string]float64{}}
	month := time.Now().UTC().Format("2006-01")
	rows, err := c.DB.Query(`SELECT kind, cost_usd, created_at FROM ai_spend`)
	if err == nil {
		for rows.Next() {
			var kind, at string
			var cost float64
			rows.Scan(&kind, &cost, &at)
			s.TotalUSD += cost
			s.ByKind[kind] += cost
			s.Calls++
			if strings.HasPrefix(at, month) {
				s.MonthUSD += cost
			}
		}
		rows.Close()
	}
	c.DB.QueryRow(`SELECT COALESCE(SUM(amount_usd),0) FROM ai_topups`).Scan(&s.TopupsUSD)
	s.BalanceUSD = s.TopupsUSD - s.TotalUSD
	return s
}
