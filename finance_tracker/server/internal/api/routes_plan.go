package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"ft/internal/cfo"
	"ft/internal/goals"
	"ft/internal/ledger"
	"ft/internal/plan"
	"ft/internal/usage"
)

// ── plan ────────────────────────────────────────────────────────────

func (s *Server) planRoutes() {
	s.handle("GET /api/plan", func(w http.ResponseWriter, r *http.Request) (any, error) {
		m := r.URL.Query().Get("month")
		if _, err := time.Parse("2006-01", m); err != nil {
			m = time.Now().Format("2006-01")
		}
		return cfo.BudgetReport(s.DB, m, time.Now())
	})
	// Once-a-year costs and whether a fund sets money aside for them.
	s.handle("GET /api/plan/yearly", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return cfo.YearlyCosts(s.DB, time.Now())
	})
	// The year ahead: cash month by month, and a what-if beside it.
	// A what-if comes as ?scenario=<json> too: it only computes, so the
	// read-only MCP tools (GET) can compare one with the baseline.
	s.handle("GET /api/forecast", func(w http.ResponseWriter, r *http.Request) (any, error) {
		raw := r.URL.Query().Get("scenario")
		if raw == "" {
			return cfo.CompareForecast(s.DB, time.Now(), qint(r, "months", 12), nil)
		}
		var sc cfo.Scenario
		if err := json.Unmarshal([]byte(raw), &sc); err != nil {
			return nil, bad("invalid scenario: " + err.Error())
		}
		return s.scenario(r, &sc)
	})
	s.handle("POST /api/forecast", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var sc cfo.Scenario
		if err := decode(r, &sc); err != nil {
			return nil, err
		}
		return s.scenario(r, &sc)
	})
	s.handle("GET /api/prefs", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return loadPrefs(s.DB), nil
	})
	s.handle("PUT /api/prefs", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p := loadPrefs(s.DB) // partial updates: absent fields keep their value; periods merge per chart
		if err := decode(r, &p); err != nil {
			return nil, err
		}
		if err := validPrefs(p); err != nil {
			return nil, bad(err.Error())
		}
		return p, savePrefs(s.DB, p)
	})
	s.handle("POST /api/usage", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if loadPrefs(s.DB).UsageOff {
			return map[string]int{"recorded": 0}, nil
		}
		var in struct {
			Events []usage.Event `json:"events"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		n, err := usage.Record(s.DB, in.Events)
		return map[string]int{"recorded": n}, err
	})
	usageDays := func(r *http.Request, def int) int {
		d, _ := strconv.Atoi(r.URL.Query().Get("days"))
		if d <= 0 || d > 180 {
			return def
		}
		return d
	}
	s.handle("GET /api/usage/summary", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return usage.Summarise(s.DB, usageDays(r, 30), time.Now())
	})
	s.handle("GET /api/usage/events", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return usage.Events(s.DB, usageDays(r, 90), time.Now())
	})
	s.handle("DELETE /api/usage", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]bool{"ok": true}, usage.Clear(s.DB)
	})
	s.handle("GET /api/plan/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		st := plan.LoadSettings(s.DB)
		return map[string]any{"settings": st, "net_from_gross": plan.LTNetSalary(st.GrossFor(time.Now().Format("2006-01")), st.MonthlyDeductions)}, nil
	})
	s.handle("PUT /api/plan/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var st plan.Settings
		if err := decode(r, &st); err != nil {
			return nil, err
		}
		if err := plan.ValidateSettings(s.DB, st); err != nil {
			return nil, bad(err.Error())
		}
		return st, plan.SaveSettings(s.DB, st)
	})
	// Partial update for the assistants: absent fields keep their value.
	s.handle("PATCH /api/plan/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var raw json.RawMessage
		if err := decode(r, &raw); err != nil {
			return nil, err
		}
		st, err := plan.PatchSettings(s.DB, raw)
		if err != nil {
			return nil, bad(err.Error())
		}
		return st, nil
	})
	// Field-by-field budget change (id 0 creates a line); an amount change
	// starts this month unless from_month or all_months says otherwise.
	s.handle("PATCH /api/budgets", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var p plan.BudgetPatch
		if err := decode(r, &p); err != nil {
			return nil, err
		}
		b, err := plan.Patch(s.DB, p, time.Now())
		if err != nil {
			return nil, bad(err.Error())
		}
		return b, nil
	})
	s.handle("GET /api/budgets", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return plan.List(s.DB, r.URL.Query().Get("archived") == "1")
	})
	saveBudget := func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			plan.Budget
			FromMonth string `json:"from_month"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		b := in.Budget
		if r.PathValue("id") != "" {
			b.ID, _ = idParam(r, "id")
		}
		return b, plan.Save(s.DB, &b, in.FromMonth)
	}
	s.handle("POST /api/budgets", saveBudget)
	s.handle("PUT /api/budgets/{id}", saveBudget)
	s.handle("DELETE /api/budgets/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, plan.Delete(s.DB, id)
	})
	s.handle("GET /api/trips", func(w http.ResponseWriter, r *http.Request) (any, error) {
		txs, err := ledger.All(s.DB, ledger.Filter{})
		if err != nil {
			return nil, err
		}
		budgets, _ := plan.List(s.DB, false)
		trips, sugg := plan.Trips(txs, budgets)
		if trips == nil {
			trips = []plan.Trip{}
		}
		if sugg == nil {
			sugg = []plan.TripSuggestion{}
		}
		dates, _ := ledger.ListTripDates(s.DB)
		// Over and done with: in history, by hand or a month after the trip.
		states, _ := ledger.TripStates(s.DB)
		planned := plannedTrips(s)
		saving := map[string]bool{}
		for _, g := range planned {
			saving[g.Tag] = true
		}
		plan.MarkDone(trips, dates, states, saving, time.Now())
		return map[string]any{"trips": trips, "suggestions": sugg, "planned": planned, "dates": dates}, nil
	})
	// A trip's real dates: set, clear, and what they show it missed.
	s.handle("PUT /api/trips/dates", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in ledger.TripDates
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if err := ledger.SaveTripDates(s.DB, in); err != nil {
			return nil, bad(err.Error())
		}
		return ledger.ListTripDates(s.DB)
	})
	s.handle("DELETE /api/trips/dates", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if err := ledger.DeleteTripDates(s.DB, r.URL.Query().Get("tag")); err != nil {
			return nil, err
		}
		return ledger.ListTripDates(s.DB)
	})
	// A trip to history (done) or back onto the list.
	s.handle("PUT /api/trips/history", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Tag  string `json:"tag"`
			Done bool   `json:"done"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if err := ledger.SetTripState(s.DB, in.Tag, in.Done); err != nil {
			return nil, bad(err.Error())
		}
		return map[string]bool{"done": in.Done}, nil
	})
	s.handle("GET /api/trips/untagged", func(w http.ResponseWriter, r *http.Request) (any, error) {
		likely, maybe, book, err := ledger.TripUntagged(s.DB, r.URL.Query().Get("tag"))
		if err != nil {
			return nil, bad(err.Error())
		}
		return map[string]any{"in_dates": likely, "maybe": maybe, "bookings": book}, nil
	})
	s.handle("POST /api/trips/tag", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Name string  `json:"name"`
			IDs  []int64 `json:"ids"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		tag, _, err := plan.TagTrip(s.DB, in.Name, in.IDs)
		if err != nil {
			return nil, bad(err.Error())
		}
		return map[string]string{"tag": tag}, nil
	})
}

// plannedTrips live on the wish list: active goals with a trip tag.
func plannedTrips(s *Server) []goals.Goal {
	planned := []goals.Goal{}
	if all, err := goals.List(s.DB); err == nil {
		for _, g := range all {
			if g.Tag != "" && g.Status == "active" {
				planned = append(planned, g)
			}
		}
	}
	return planned
}

func (s *Server) scenario(r *http.Request, sc *cfo.Scenario) (any, error) {
	if sc.Gross < 0 || sc.Gross > 1e6 || sc.EverydayPct < -90 || sc.EverydayPct > 300 || len(sc.Purchases) > 50 {
		return nil, bad("scenario out of range")
	}
	return cfo.CompareForecast(s.DB, time.Now(), qint(r, "months", 12), sc)
}
