package api

import (
	"net/http"
	"time"

	"ft/internal/goals"
	"ft/internal/money"
)

// ── wish list ───────────────────────────────────────────────────────

func (s *Server) goalRoutes() {
	s.handle("GET /api/goals", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := goals.Build(s.DB, r.URL.Query().Get("month"), time.Now())
		if err != nil {
			return nil, bad(err.Error())
		}
		return p, nil
	})
	save := func(w http.ResponseWriter, r *http.Request) (any, error) {
		var g goals.Goal
		if err := decode(r, &g); err != nil {
			return nil, err
		}
		if r.PathValue("id") != "" {
			g.ID, _ = idParam(r, "id")
		}
		if err := goals.Save(s.DB, &g); err != nil {
			return nil, bad(err.Error())
		}
		return goals.Get(s.DB, g.ID)
	}
	s.handle("POST /api/goals", save)
	s.handle("PUT /api/goals/{id}", save)
	s.handle("DELETE /api/goals/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, goals.Delete(s.DB, id)
	})
	s.handle("POST /api/goals/order", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			IDs []int64 `json:"ids"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, goals.Reorder(s.DB, in.IDs)
	})
	// Fund a finished month: the allocations (by default the suggested split).
	s.handle("POST /api/goals/fund", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Month       string             `json:"month"`
			Allocations []goals.Allocation `json:"allocations"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		p, err := goals.Fund(s.DB, in.Month, in.Allocations, time.Now())
		if err != nil {
			return nil, bad(err.Error())
		}
		return p, nil
	})
	// Money put in by hand (a bonus, a gift) or taken out (negative).
	s.handle("POST /api/goals/{id}/moves", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		var in struct {
			Amount money.Cents `json:"amount"`
			Month  string      `json:"month"`
			Note   string      `json:"note"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if in.Month == "" {
			in.Month = time.Now().Format("2006-01")
		}
		m := goals.Move{GoalID: id, Month: in.Month, Amount: in.Amount, Note: in.Note}
		if err := goals.AddMove(s.DB, &m); err != nil {
			return nil, bad(err.Error())
		}
		return m, nil
	})
	s.handle("GET /api/goals/{id}/moves", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return goals.Moves(s.DB, id)
	})
}
