package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// ListModels asks the configured gateway which models it offers, so the
// settings UI can present a dropdown instead of a free-text field. It GETs
// <gateway>/models with the API key and accepts the common response shapes
// (OpenAI/nexos {"data":[{"id"}]}, Anthropic {"data":[{"id"}]}, or a plain
// list). An error means "no usable model list" — the UI falls back to text.
func (s *insightService) ListModels(ctx context.Context) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	settings, err := s.repo.GetAISettings()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(settings.GatewayURL) == "" || strings.TrimSpace(settings.APIKey) == "" {
		return nil, fmt.Errorf("gateway not configured")
	}

	url := strings.TrimRight(settings.GatewayURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+settings.APIKey)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gateway returned %s for /models", resp.Status)
	}
	return parseModelList(body)
}

// parseModelList pulls model ids out of the various shapes gateways return.
func parseModelList(body []byte) ([]string, error) {
	// { "data": [ {"id": "..."} | "..." ] } or { "models": [ ... ] }
	var wrapped struct {
		Data   []json.RawMessage `json:"data"`
		Models []json.RawMessage `json:"models"`
	}
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	extract := func(items []json.RawMessage) {
		for _, raw := range items {
			var asString string
			if json.Unmarshal(raw, &asString) == nil {
				add(asString)
				continue
			}
			var obj struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}
			if json.Unmarshal(raw, &obj) == nil {
				if obj.ID != "" {
					add(obj.ID)
				} else {
					add(obj.Name)
				}
			}
		}
	}
	if json.Unmarshal(body, &wrapped) == nil && (len(wrapped.Data) > 0 || len(wrapped.Models) > 0) {
		extract(wrapped.Data)
		extract(wrapped.Models)
	} else {
		// Bare top-level array.
		var arr []json.RawMessage
		if json.Unmarshal(body, &arr) == nil {
			extract(arr)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no models in gateway response")
	}
	sort.Strings(out)
	return out, nil
}
