package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"ft/internal/backup"
)

// The sync download: a script on the owner's computer fetches the plain
// backup (no secrets) with the sync token, so a local copy can follow
// production without anyone signing in to it.
//
// The path sits outside /api, so nginx lets it past the web password (a
// script can't type it) and authenticate never sees it: the handler checks
// its own credential, the ftsy_ sync token, every time — whether or not the
// app lock is on. A session cookie doesn't open it and API tokens (ftk_,
// ftkw_) are refused, and the sync token opens nothing else (TokenScope
// never accepts it).
func (s *Server) prodSyncRoutes() {
	s.mux.HandleFunc("GET /sync/backup.json", func(w http.ResponseWriter, r *http.Request) {
		bearer, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !s.Auth.IsSyncToken(strings.TrimSpace(bearer)) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="finance-tracker-sync"`)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "a valid sync token is required"})
			return
		}
		f, err := backup.Export(s.DB, false) // never the secrets, whatever the query says
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not export the backup"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Disposition", `attachment; filename="finance-backup-`+time.Now().Format("2006-01-02")+`.json"`)
		// Like ?purpose=export: a sync is not the owner's off-device backup,
		// so it doesn't reset the "last backup downloaded" reminder.
		json.NewEncoder(w).Encode(f)
	})
}
