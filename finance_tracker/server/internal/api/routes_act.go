package api

import (
	"net/http"
	"time"

	"ft/internal/cfo"
	"ft/internal/decisions"
	"ft/internal/goals"
	"ft/internal/insights"
	"ft/internal/money"
	"ft/internal/wealth"
)

// ── planning tools: act, monitor, decide ───────────────────────────
//
// Read routes sit under /api/plan and /api/insights, so API tokens (MCP)
// read them like the rest of the plan; /api/wealth/ownership is listed on
// its own. Of the writes only logging a decision is open to read-write
// tokens — an assistant may write down what you're considering, never
// decide, rate or delete it here, and never change the ownership figures.
// One deliberate exception: a decision saved for on the wish list follows its
// goal, and read-write tokens already manage goals (PUT /api/goals/), so a
// goal they mark bought or dropped carries its decision with it — the goal
// is the record of whether it was bought.

func (s *Server) actRoutes() {
	s.handle("GET /api/plan/payday", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if r.URL.Query().Get("moves") == "0" { // the web's light read
			return cfo.PaydaySteps(s.DB, time.Now())
		}
		return cfo.BuildRunbook(s.DB, time.Now())
	})
	s.handle("GET /api/plan/next-euro", func(w http.ResponseWriter, r *http.Request) (any, error) {
		sum := money.FromFloat(500)
		if v := r.URL.Query().Get("amount"); v != "" {
			c := parseCents(v)
			if c <= 0 || c > money.FromFloat(10_000_000) {
				return nil, bad("amount must be a positive number of euros")
			}
			sum = c
		}
		return cfo.NextEuroFor(s.DB, sum, time.Now())
	})

	s.handle("GET /api/insights/creep", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return insights.LifestyleCreep(s.DB, time.Now(), qint(r, "years", 1))
	})
	s.handle("GET /api/insights/prices", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return cfo.Prices(s.DB, time.Now())
	})

	s.handle("GET /api/plan/decisions", func(w http.ResponseWriter, r *http.Request) (any, error) {
		all, err := decisions.List(s.DB)
		if err != nil {
			return nil, err
		}
		return map[string]any{"items": all, "record": decisions.Summarise(all)}, nil
	})
	s.handle("GET /api/plan/decisions/impact", func(w http.ResponseWriter, r *http.Request) (any, error) {
		c := parseCents(r.URL.Query().Get("amount"))
		if c <= 0 || c > money.FromFloat(10_000_000) {
			return nil, bad("amount must be a positive number of euros")
		}
		return cfo.DecisionImpact(s.DB, c, time.Now())
	})
	s.handle("POST /api/plan/decisions", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in decisions.Decision
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if err := decisions.Create(s.DB, &in, time.Now()); err != nil {
			return nil, err
		}
		return in, nil
	})
	s.handle("PUT /api/plan/decisions/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		var in decisions.Decision
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		in.ID = id
		if err := decisions.Update(s.DB, &in, time.Now()); err != nil {
			return nil, err
		}
		return in, nil
	})
	// Save for it: the purchase goes on the wish list as a goal. Session-only
	// — like deciding, it's the owner's call.
	s.handle("POST /api/plan/decisions/{id}/save-for", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return goals.FromDecision(s.DB, id, time.Now())
	})
	s.handle("DELETE /api/plan/decisions/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, decisions.Delete(s.DB, id)
	})

	s.handle("GET /api/wealth/ownership", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return cfo.Ownership(s.DB, time.Now())
	})
	s.handle("PUT /api/wealth/ownership/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in wealth.OwnSettings
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, wealth.SaveOwnSettings(s.DB, r.PathValue("id"), in)
	})
}
