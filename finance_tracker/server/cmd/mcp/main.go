// finance-tracker-mcp exposes the Finance Tracker API as MCP tools for
// Claude Desktop, Claude Code or any MCP client.
//
// It never touches the database: every tool is an HTTP call to the app's
// API with a bearer token, and the server enforces the token's scope.
//
//   - read-only token (ftk_…): every read tool.
//   - read-write token (ftkw_…): also create/update transactions, rules and
//     tags, improve bank-inbox proposals, and manage the plan (budget lines,
//     plan settings, trips, recurring items) and the assistant's memory.
//     Accepting bank rows into the ledger, app settings, exports and backups
//     stay out of reach.
//
// Configuration:
//
//	FT_API_URL   e.g. https://finance.example/api  (or http://homeassistant.local:8099/api)
//	FT_API_TOKEN the token minted in Settings → Security → API tokens
//	FT_BASIC_AUTH optional "user:password" when nginx basic auth fronts the API
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"ft/internal/mcptools"
)

var (
	apiURL, apiToken, basicAuth string
	client                      = &http.Client{Timeout: 60 * time.Second}
)

func call(method, path string, q url.Values, body any) ([]byte, error) {
	u := apiURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, u, rd)
	if err != nil {
		return nil, err
	}
	// nginx basic auth and the app token both want Authorization; the app
	// also accepts the token in X-API-Token for exactly this case.
	if basicAuth != "" {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(basicAuth)))
		req.Header.Set("X-API-Token", apiToken)
	} else {
		req.Header.Set("Authorization", "Bearer "+apiToken)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("finance tracker unreachable at %s: %w", apiURL, err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, fmt.Errorf("token rejected — mint a new one in Settings → Security → API tokens")
	case resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("outside this token's scope (writes need a read-write ftkw_ token)")
	case resp.StatusCode >= 300:
		return nil, fmt.Errorf("API returned %s: %s", resp.Status, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func main() {
	apiURL = strings.TrimRight(os.Getenv("FT_API_URL"), "/")
	apiToken = os.Getenv("FT_API_TOKEN")
	basicAuth = os.Getenv("FT_BASIC_AUTH")
	if apiURL == "" || apiToken == "" {
		log.Fatal("set FT_API_URL (…/api) and FT_API_TOKEN (Settings → Security → API tokens)")
	}
	if err := mcptools.New(call).Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
