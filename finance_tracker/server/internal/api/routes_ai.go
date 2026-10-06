package api

import (
	"io"
	"net/http"
	"strings"
	"time"

	"ft/internal/ai"
	"ft/internal/ledger"
)

// ── AI ──────────────────────────────────────────────────────────────

func (s *Server) aiRoutes() {
	s.handle("GET /api/ai/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		st := ai.LoadSettings(s.DB)
		masked := ""
		if n := len(st.APIKey); n > 8 {
			masked = st.APIKey[:6] + "…" + st.APIKey[n-4:]
		}
		return map[string]any{"gateway_url": st.GatewayURL, "model": st.Model, "provider": st.ResolvedProvider(), "enabled": st.Enabled,
			"has_key": st.APIKey != "", "key_hint": masked, "spend": s.AI.Client.Spend()}, nil
	})
	s.handle("PUT /api/ai/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			GatewayURL *string `json:"gateway_url"`
			APIKey     string  `json:"api_key"`
			Model      *string `json:"model"`
			Provider   *string `json:"provider"`
			Enabled    *bool   `json:"enabled"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		st := ai.LoadSettings(s.DB)
		if in.GatewayURL != nil {
			st.GatewayURL = strings.TrimSpace(*in.GatewayURL)
		}
		if k := strings.TrimSpace(in.APIKey); k != "" {
			st.APIKey = k
		}
		if in.Model != nil {
			m := strings.TrimSpace(*in.Model)
			if len(m) > 100 || strings.ContainsAny(m, " \t\r\n\"'<>") {
				return nil, bad("invalid model name")
			}
			st.Model = m
		}
		if in.Provider != nil {
			st.Provider = *in.Provider
		}
		if in.Enabled != nil {
			st.Enabled = *in.Enabled
		}
		return map[string]bool{"ok": true}, ai.SaveSettings(s.DB, st)
	})
	s.handle("POST /api/ai/test", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]bool{"ok": true}, s.AI.Client.Test(r.Context(), ai.LoadSettings(s.DB))
	})
	s.handle("GET /api/ai/models", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.AI.Client.Models(r.Context(), ai.LoadSettings(s.DB))
	})
	s.handle("GET /api/ai/context", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]string{"content": s.AI.Context()}, nil
	})
	s.handle("PUT /api/ai/context", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct{ Content string }
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, s.AI.SaveContext(in.Content)
	})
	s.handle("GET /api/ai/notes", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]string{"content": s.AI.Notes()}, nil
	})
	s.handle("PUT /api/ai/notes", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct{ Content string }
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, s.AI.SaveNotes(in.Content)
	})
	s.handle("GET /api/ai/chat", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.AI.History(200) })
	s.handle("DELETE /api/ai/chat", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]bool{"ok": true}, s.AI.ClearHistory()
	})
	s.handle("POST /api/ai/chat", func(w http.ResponseWriter, r *http.Request) (any, error) {
		text, img, err := readMessage(r)
		if err != nil {
			return nil, err
		}
		return s.AI.Chat(r.Context(), text, img)
	})
	s.handle("POST /api/ai/scan", func(w http.ResponseWriter, r *http.Request) (any, error) {
		_, img, err := readMessage(r)
		if err != nil {
			return nil, err
		}
		if img == nil {
			return nil, bad("attach an image")
		}
		return s.AI.ScanReceipt(r.Context(), *img)
	})
	s.handle("POST /api/ai/assist", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct{ Text string }
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return s.AI.Assist(r.Context(), in.Text)
	})
	s.handle("GET /api/tidy", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.tidyQueue()
	})
	s.handle("POST /api/ai/tidy", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			IDs []int64 `json:"ids"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		var txs []ledger.Tx
		for _, id := range in.IDs {
			if t, err := ledger.Get(s.DB, id); err == nil {
				txs = append(txs, t)
			}
		}
		return s.AI.Tidy(r.Context(), txs)
	})
	s.handle("GET /api/ai/topups", func(w http.ResponseWriter, r *http.Request) (any, error) {
		rows, err := s.DB.Query(`SELECT id, amount_usd, note, occurred_on FROM ai_topups ORDER BY occurred_on DESC, id DESC`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id int64
			var amt float64
			var note, on string
			rows.Scan(&id, &amt, &note, &on)
			out = append(out, map[string]any{"id": id, "amount_usd": amt, "note": note, "occurred_on": on})
		}
		return out, nil
	})
	s.handle("DELETE /api/ai/topups/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		_, err = s.DB.Exec(`DELETE FROM ai_topups WHERE id=?`, id)
		return map[string]bool{"ok": true}, err
	})
	s.handle("POST /api/ai/topups", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			AmountUSD float64 `json:"amount_usd"`
			Note      string  `json:"note"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		_, err := s.DB.Exec(`INSERT INTO ai_topups(amount_usd,note,occurred_on,created_at) VALUES(?,?,?,?)`, in.AmountUSD, in.Note, today(), time.Now().UTC().Format(time.RFC3339))
		return map[string]bool{"ok": true}, err
	})
}

// readMessage accepts multipart (text + image file) or JSON {text}.
func readMessage(r *http.Request) (string, *ai.Image, error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(12 << 20); err != nil {
			return "", nil, bad("image too large (max 10 MB)")
		}
		text := r.FormValue("text")
		f, hdr, err := r.FormFile("image")
		if err != nil {
			return text, nil, nil
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, 10<<20))
		if err != nil {
			return "", nil, err
		}
		mt := hdr.Header.Get("Content-Type")
		if mt == "" || mt == "application/octet-stream" {
			mt = http.DetectContentType(data)
		}
		return text, &ai.Image{MediaType: mt, Data: data}, nil
	}
	var in struct{ Text string }
	if err := decode(r, &in); err != nil {
		return "", nil, err
	}
	return in.Text, nil, nil
}
