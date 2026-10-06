package api

import (
	"net/http"
	"strings"

	"ft/internal/bank"
)

// ── bank ────────────────────────────────────────────────────────────

func (s *Server) bankRoutes() {
	s.handle("GET /api/bank/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		st := bank.LoadSettings(s.DB)
		return map[string]any{"application_id": st.ApplicationID, "has_key": st.PrivateKeyPEM != "", "environment": st.Environment,
			"redirect_url": st.RedirectURL, "owner_names": st.OwnerNames, "consent_days": st.ConsentDays, "configured": st.Configured(),
			"keep_reserved_in_inbox": st.KeepReservedInInbox}, nil
	})
	s.handle("PUT /api/bank/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			ApplicationID *string   `json:"application_id"`
			PrivateKeyPEM string    `json:"private_key_pem"`
			Environment   *string   `json:"environment"`
			RedirectURL   *string   `json:"redirect_url"`
			OwnerNames    *[]string `json:"owner_names"`
			ConsentDays   *int      `json:"consent_days"`
			KeepReserved  *bool     `json:"keep_reserved_in_inbox"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		st := bank.LoadSettings(s.DB)
		if in.KeepReserved != nil {
			st.KeepReservedInInbox = *in.KeepReserved
		}
		if in.ApplicationID != nil {
			st.ApplicationID = strings.TrimSpace(*in.ApplicationID)
		}
		if in.Environment != nil {
			st.Environment = *in.Environment
		}
		if in.RedirectURL != nil {
			st.RedirectURL = strings.TrimSpace(*in.RedirectURL)
		}
		if in.OwnerNames != nil {
			st.OwnerNames = *in.OwnerNames
		}
		if in.ConsentDays != nil && *in.ConsentDays > 0 {
			st.ConsentDays = *in.ConsentDays
		}
		if k := strings.TrimSpace(in.PrivateKeyPEM); k != "" {
			st.PrivateKeyPEM = k
		}
		return map[string]bool{"ok": true}, bank.SaveSettings(s.DB, st)
	})
	s.handle("GET /api/bank/banks", func(w http.ResponseWriter, r *http.Request) (any, error) {
		c := strings.ToUpper(r.URL.Query().Get("country"))
		if c == "" {
			c = "LT"
		}
		return s.Bank.Banks(r.Context(), c)
	})
	s.handle("GET /api/bank/connections", func(w http.ResponseWriter, r *http.Request) (any, error) {
		c, err := s.Bank.Connections()
		if c == nil {
			c = []bank.Connection{}
		}
		return c, err
	})
	s.handle("POST /api/bank/connect", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Bank    string `json:"bank"`
			Country string `json:"country"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		id, u, err := s.Bank.Connect(r.Context(), in.Bank, in.Country, psu(r))
		return map[string]any{"connection_id": id, "url": u}, err
	})
	s.handle("POST /api/bank/callback", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct{ Code, State, URL string }
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		code, state := in.Code, in.State
		if in.URL != "" {
			var err error
			if code, state, err = bank.ParseCallbackURL(in.URL); err != nil {
				return nil, err
			}
		}
		id, err := s.Bank.Callback(r.Context(), strings.TrimSpace(code), strings.TrimSpace(state))
		return map[string]any{"connection_id": id}, err
	})
	s.handle("DELETE /api/bank/connections/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, s.Bank.Disconnect(r.Context(), id)
	})
	s.handle("PUT /api/bank/accounts/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		var in struct {
			AccountID string `json:"account_id"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, s.Bank.MapAccount(id, in.AccountID)
	})
	s.handle("POST /api/bank/sync", func(w http.ResponseWriter, r *http.Request) (any, error) {
		days := qint(r, "days", 0)
		if days < 0 || days > 730 {
			days = 0
		}
		return s.Bank.SyncAll(r.Context(), psu(r), days, int64(qint(r, "account", 0)))
	})
	s.handle("GET /api/bank/inbox", func(w http.ResponseWriter, r *http.Request) (any, error) {
		rows, err := s.Bank.Inbox(r.URL.Query().Get("state"))
		if rows == nil {
			rows = []bank.InboxRow{}
		}
		for i := range rows {
			rows[i].Raw = ""
		}
		return rows, err
	})
	s.handle("GET /api/bank/inbox/{id}/raw", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		raw, err := s.Bank.Raw(id)
		return map[string]string{"raw": raw}, err
	})
	s.handle("PUT /api/bank/inbox/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		var e bank.Edit
		if err := decode(r, &e); err != nil {
			return nil, err
		}
		return s.Bank.Update(id, e)
	})
	s.handle("POST /api/bank/inbox/commit", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			IDs []int64 `json:"ids"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return s.Bank.Commit(in.IDs)
	})
	for _, st := range []string{"dismiss", "restore"} {
		to := "dismissed"
		if st == "restore" {
			to = "open"
		}
		s.handle("POST /api/bank/inbox/"+st, func(w http.ResponseWriter, r *http.Request) (any, error) {
			var in struct {
				IDs []int64 `json:"ids"`
			}
			if err := decode(r, &in); err != nil {
				return nil, err
			}
			n, err := s.Bank.SetState(in.IDs, to)
			return map[string]int{"changed": n}, err
		})
	}
	s.handle("POST /api/bank/inbox/{id}/link", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		var in struct {
			TxID int64 `json:"tx_id"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, s.Bank.Link(id, in.TxID)
	})
	s.handle("POST /api/bank/inbox/{id}/unlink", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, s.Bank.Unlink(id)
	})
}
