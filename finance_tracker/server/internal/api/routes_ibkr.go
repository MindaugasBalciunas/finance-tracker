package api

import (
	"net/http"
	"strings"

	"ft/internal/ibkr"
	"ft/internal/ledger"
)

// Interactive Brokers, read-only through IBKR's MCP server: connect (IBKR's
// own consent page), sync the account value, compare positions and trades,
// import the trades you pick.
func (s *Server) ibkrRoutes() {
	s.handle("GET /api/ibkr", func(w http.ResponseWriter, r *http.Request) (any, error) {
		rep, _ := s.IBKR.LastReport()
		return map[string]any{"status": s.IBKR.Status(), "report": rep}, nil
	})
	s.handle("POST /api/ibkr/connect", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			RedirectURI string `json:"redirect_uri"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		u, err := s.IBKR.Connect(r.Context(), strings.TrimSpace(in.RedirectURI))
		if err != nil {
			return nil, bad(err.Error())
		}
		return map[string]string{"url": u}, nil
	})
	s.handle("POST /api/ibkr/callback", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct{ Code, State string }
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if !ibkr.IsCallbackState(in.State) {
			return nil, bad("not an IBKR sign-in")
		}
		if err := s.IBKR.Callback(r.Context(), strings.TrimSpace(in.Code), in.State); err != nil {
			return nil, bad(err.Error())
		}
		return s.IBKR.Status(), nil
	})
	s.handle("PUT /api/ibkr/account", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Account string `json:"account"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if _, err := ledger.GetAccount(s.DB, in.Account); err != nil {
			return nil, bad("unknown account")
		}
		return map[string]bool{"ok": true}, s.IBKR.SetAccount(in.Account)
	})
	s.handle("POST /api/ibkr/sync", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.IBKR.Sync(r.Context(), false)
	})
	s.handle("POST /api/ibkr/import", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			IDs []string `json:"ids"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		n, err := s.IBKR.Import(in.IDs)
		if err != nil {
			return nil, bad(err.Error())
		}
		return map[string]int{"imported": n}, nil
	})
	s.handle("DELETE /api/ibkr", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]bool{"ok": true}, s.IBKR.Disconnect(r.Context())
	})
}
