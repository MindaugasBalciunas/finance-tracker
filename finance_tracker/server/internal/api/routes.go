package api

import (
	"net/http"
)

type H = func(w http.ResponseWriter, r *http.Request) (any, error)

func (s *Server) routes() {
	s.handle("GET /api/health", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]any{"ok": true, "version": s.Version}, nil
	})
	s.authRoutes()
	s.ledgerRoutes()
	s.wealthRoutes()
	s.planRoutes()
	s.insightRoutes()
	s.bankRoutes()
	s.aiRoutes()
	s.dataRoutes()
}
