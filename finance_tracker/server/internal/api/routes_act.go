package api

import (
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"ft/internal/cfo"
	"ft/internal/decisions"
	"ft/internal/goals"
	"ft/internal/insights"
	"ft/internal/logos"
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

// ── more planning: milestones, subscriptions, fees, calendar, FI plan, logos
//
// Reads: /api/plan/* already, /api/wealth/{milestones,fees,fi-plan} listed
// exactly. Writes (subscription verdicts, fee figures) are the owner's own,
// session-only. Logos are session-only too: they trigger outbound fetches.

func (s *Server) moreRoutes() {
	s.handle("GET /api/wealth/milestones", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return cfo.Milestones(s.DB, time.Now())
	})
	s.handle("GET /api/wealth/fi-plan", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return cfo.BuildFIPlan(s.DB, time.Now())
	})
	s.handle("GET /api/wealth/fees", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return cfo.Fees(s.DB, time.Now(), r.URL.Query().Get("live") != "0")
	})
	s.handle("PUT /api/wealth/fees/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in wealth.FeeSetting
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, wealth.SaveFeeSetting(s.DB, r.PathValue("id"), in)
	})
	s.handle("GET /api/plan/subscriptions", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return insights.AuditSubscriptions(s.DB, time.Now())
	})
	s.handle("PUT /api/plan/subscriptions", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Key string `json:"key"`
			insights.SubVerdict
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, insights.SetSubVerdict(s.DB, in.Key, in.SubVerdict, time.Now())
	})
	s.handle("GET /api/plan/calendar", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return cfo.CashFlowCalendar(s.DB, time.Now(), qint(r, "days", 60))
	})
	s.handle("GET /api/logos", func(w http.ResponseWriter, r *http.Request) (any, error) {
		w.Header().Set("Cache-Control", "private, no-cache") // an upload must show at once
		return logos.IndexWith(s.DB), nil
	})
	// Your own logo for a merchant: uploaded, never fetched (base64 in JSON,
	// well inside the 2 MB body limit; the bytes are checked to be an image).
	s.handle("PUT /api/logos/custom", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Merchant string `json:"merchant"`
			Data     string `json:"data"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		b, err := base64.StdEncoding.DecodeString(in.Data)
		if err != nil {
			return nil, bad("the image didn't arrive intact")
		}
		if err := logos.SetCustom(s.DB, in.Merchant, b); err != nil {
			return nil, bad(err.Error())
		}
		return map[string]string{"id": logos.CustomID + logos.Key(in.Merchant)}, nil
	})
	s.handle("DELETE /api/logos/custom/{key}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]bool{"ok": true}, logos.DeleteCustom(s.DB, r.PathValue("key"))
	})
	s.handle("GET /api/logos/{domain}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id := r.PathValue("domain")
		var b []byte
		var mime string
		var err error
		if key, ok := strings.CutPrefix(id, logos.CustomID); ok {
			b, mime, err = logos.Custom(s.DB, key)
			w.Header().Set("Cache-Control", "private, no-cache") // replaced uploads show at once
		} else {
			b, mime, err = logos.Get(r.Context(), id)
		}
		if err != nil {
			return nil, notFound("no logo")
		}
		w.Header().Set("Content-Type", mime)
		if w.Header().Get("Cache-Control") == "" {
			w.Header().Set("Cache-Control", "private, max-age=2592000")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// An SVG opened on its own can't run script or reach anything: the
		// headers are the guard, the SVG check before storing a convenience.
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		w.Write(b)
		return nil, nil
	})
}
