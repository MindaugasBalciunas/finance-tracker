package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"ft/internal/cfo"
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
		return map[string]any{"settings": st, "net_from_gross": plan.LTNetSalary(st.GrossSalary, st.MonthlyDeductions)}, nil
	})
	s.handle("PUT /api/plan/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var st plan.Settings
		if err := decode(r, &st); err != nil {
			return nil, err
		}
		if err := plan.ValidSalaryRules(st.SalaryRules); err != nil {
			return nil, bad(err.Error())
		}
		if st.SalaryAccount != "" {
			if _, err := ledger.GetAccount(s.DB, st.SalaryAccount); err != nil {
				return nil, bad("unknown salary account")
			}
		}
		for id, v := range st.Buffers {
			if _, err := ledger.GetAccount(s.DB, id); err != nil {
				return nil, bad("unknown account in buffers: " + id)
			}
			if v < 0 || v > 1e6 {
				return nil, bad("a buffer is between €0 and €1,000,000")
			}
		}
		return st, plan.SaveSettings(s.DB, st)
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
		return map[string]any{"trips": trips, "suggestions": sugg}, nil
	})
	s.handle("POST /api/trips/tag", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Name string  `json:"name"`
			IDs  []int64 `json:"ids"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		name := strings.Trim(ledger.SlugID(in.Name), "_")
		name = strings.ReplaceAll(name, "_", "-")
		if name == "" {
			return nil, bad("name the trip")
		}
		tag := "trip:" + name
		tx, err := s.DB.Begin()
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		for _, id := range in.IDs {
			t, err := ledger.Get(tx, id)
			if err != nil {
				continue
			}
			var keep []string
			for _, x := range t.Tags {
				if !ledger.TripTag(x) {
					keep = append(keep, x)
				}
			}
			t.Tags = append(keep, tag)
			ledger.Update(tx, &t)
		}
		return map[string]string{"tag": tag}, tx.Commit()
	})
}
