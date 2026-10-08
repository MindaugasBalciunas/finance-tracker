package api

import (
	"net/http"

	"ft/internal/notify"
)

// Phone notifications through a Home Assistant webhook (see package notify).
func (s *Server) notifyRoutes() {
	s.handle("GET /api/notify", func(w http.ResponseWriter, r *http.Request) (any, error) {
		st := notify.Load(s.DB)
		return map[string]any{"enabled": st.Enabled, "webhook_url": st.WebhookURL}, nil
	})
	s.handle("PUT /api/notify", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in notify.Settings
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if in.Enabled && in.WebhookURL == "" {
			return nil, bad("add the webhook address first")
		}
		return map[string]bool{"ok": true}, notify.Save(s.DB, in)
	})
	s.handle("POST /api/notify/test", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in notify.Settings
		decode(r, &in)
		st := notify.Load(s.DB)
		if in.WebhookURL != "" {
			if err := notify.Validate(in.WebhookURL); err != nil {
				return nil, bad(err.Error())
			}
			st.WebhookURL = in.WebhookURL
		}
		link := ""
		if o := appOrigin(r); o != "" {
			link = o + "/#/settings/ai"
		}
		if err := notify.Post(st, notify.Message{Title: "Finance", Message: "Test notification — this is how Ask CFO will tell you an answer is ready.", URL: link, Tag: "test"}); err != nil {
			return nil, &HTTPError{http.StatusBadGateway, err.Error()}
		}
		return map[string]bool{"ok": true}, nil
	})
}
