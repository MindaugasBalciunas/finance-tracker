// Package notify sends phone notifications through a Home Assistant webhook.
//
// The add-on holds no Home Assistant token: it posts a small JSON message to
// a webhook URL the owner creates in Home Assistant, and an automation there
// forwards it to their phone (notify.mobile_app_…). Least privilege — a
// compromised app could at most trigger that one automation. Messages never
// carry amounts or balances: push services relay them through third parties.
package notify

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Settings live under the "notify" settings key (a secret: the webhook URL
// is the only thing guarding the automation).
type Settings struct {
	WebhookURL string `json:"webhook_url"`
	Enabled    bool   `json:"enabled"`
}

type Message struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	URL     string `json:"url,omitempty"` // opens the app there when tapped
	Tag     string `json:"tag,omitempty"` // replaces an earlier notification with the same tag
}

func Load(d *sql.DB) Settings {
	var raw string
	var s Settings
	if d.QueryRow(`SELECT value FROM settings WHERE key='notify'`).Scan(&raw) == nil {
		json.Unmarshal([]byte(raw), &s)
	}
	return s
}

func Save(d *sql.DB, s Settings) error {
	s.WebhookURL = strings.TrimSpace(s.WebhookURL)
	if s.WebhookURL != "" {
		if err := Validate(s.WebhookURL); err != nil {
			return err
		}
	}
	raw, _ := json.Marshal(s)
	_, err := d.Exec(`INSERT INTO settings(key,value) VALUES('notify',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(raw))
	return err
}

// Validate accepts only a Home Assistant webhook address.
func Validate(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("enter the full webhook address, e.g. http://homeassistant:8123/api/webhook/…")
	}
	if !strings.HasPrefix(u.Path, "/api/webhook/") || len(strings.TrimPrefix(u.Path, "/api/webhook/")) < 8 {
		return errors.New("that isn't a Home Assistant webhook address (…/api/webhook/<id>, id of 8+ characters)")
	}
	return nil
}

var client = &http.Client{Timeout: 10 * time.Second}

// Post sends m to the configured webhook regardless of Enabled (for the
// test button); errors say what went wrong.
func Post(s Settings, m Message) error {
	if s.WebhookURL == "" {
		return errors.New("no webhook address set")
	}
	body, _ := json.Marshal(m)
	resp, err := client.Post(s.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("Home Assistant didn't answer: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("Home Assistant answered %s", resp.Status)
	}
	return nil
}

// Send delivers m when notifications are on; best effort.
func Send(d *sql.DB, m Message) {
	if s := Load(d); s.Enabled && s.WebhookURL != "" {
		Post(s, m)
	}
}
