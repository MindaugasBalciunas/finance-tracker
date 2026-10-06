package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"ft/internal/cfo"
	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

// ── insights ────────────────────────────────────────────────────────

func window(r *http.Request) (string, string) {
	q := r.URL.Query()
	if q.Get("from") != "" && q.Get("to") != "" {
		return q.Get("from"), q.Get("to")
	}
	return insights.Window(q.Get("preset"), time.Now())
}

func (s *Server) insightRoutes() {
	s.handle("GET /api/overview", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return cfo.BuildOverview(s.DB, time.Now(), s.Bank.OpenCount())
	})
	s.handle("GET /api/insights/cashflow", func(w http.ResponseWriter, r *http.Request) (any, error) {
		q := r.URL.Query()
		from, to := q.Get("from"), q.Get("to")
		salary := plan.LoadSettings(s.DB).Salary()
		loadTo := to
		if t, err := time.Parse("2006-01-02", to); err == nil { // a late salary still reaches its month
			loadTo = t.AddDate(0, 0, plan.MaxSalaryDays(salary)).Format("2006-01-02")
		}
		txs, err := ledger.All(s.DB, ledger.Filter{From: from, To: loadTo})
		if err != nil {
			return nil, err
		}
		cats, _ := ledger.CategoryMap(s.DB)
		g := q.Get("granularity")
		if g != "year" {
			g = "month"
		}
		return insights.CashFlowRange(txs, cats, g, from, to, salary), nil
	})
	s.handle("GET /api/insights/breakdown", func(w http.ResponseWriter, r *http.Request) (any, error) {
		from, to := window(r)
		txs, err := ledger.All(s.DB, ledger.Filter{})
		if err != nil {
			return nil, err
		}
		return map[string]any{"from": from, "to": to, "categories": insights.Breakdown(txs, from, to),
			"merchants": insights.MerchantTotals(txs, from, to, 25), "largest": insights.TopExpenses(txs, from, to, 15),
			"tags": insights.TagTotals(txs, from, to)}, nil
	})
	s.handle("GET /api/insights/trends", func(w http.ResponseWriter, r *http.Request) (any, error) {
		months := qint(r, "months", 24)
		if months < 3 || months > 240 {
			months = 24
		}
		from := time.Now().AddDate(0, -months+1, 0).Format("2006-01") + "-01"
		txs, err := ledger.All(s.DB, ledger.Filter{From: from, Kind: "expense"})
		if err != nil {
			return nil, err
		}
		level := r.URL.Query().Get("level")
		parent := r.URL.Query().Get("parent")
		m := map[string]map[string]money.Cents{}
		for _, t := range txs {
			key := ledger.Top(t.Category)
			if parent != "" {
				if key != parent {
					continue
				}
				key = t.Category
			} else if level == "leaf" {
				key = t.Category
			}
			if m[t.Date[:7]] == nil {
				m[t.Date[:7]] = map[string]money.Cents{}
			}
			m[t.Date[:7]][key] += t.Amount
		}
		var rows []map[string]any
		for i := months - 1; i >= 0; i-- {
			mo := time.Now().AddDate(0, -i, 0).Format("2006-01")
			row := map[string]any{"month": mo}
			for k, v := range m[mo] {
				row[k] = v
			}
			rows = append(rows, row)
		}
		return rows, nil
	})
	s.handle("GET /api/insights/month", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return cfo.BuildMonthReview(s.DB, r.URL.Query().Get("month"), time.Now())
	})
	s.handle("GET /api/checks", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return cfo.Checks(s.DB, time.Now()), nil
	})
	s.handle("GET /api/insights/pace", func(w http.ResponseWriter, r *http.Request) (any, error) {
		txs, err := ledger.All(s.DB, ledger.Filter{From: time.Now().AddDate(0, -7, 0).Format("2006-01") + "-01"})
		if err != nil {
			return nil, err
		}
		return insights.Pace(txs, time.Now()), nil
	})
	s.handle("GET /api/insights/recurring", func(w http.ResponseWriter, r *http.Request) (any, error) {
		txs, err := ledger.All(s.DB, ledger.Filter{From: time.Now().AddDate(-2, 0, 0).Format("2006-01-02")})
		if err != nil {
			return nil, err
		}
		rec, hidden, err := insights.RecurringAll(s.DB, txs, time.Now())
		if err != nil {
			return nil, err
		}
		var total money.Cents // costs only: transfers and income are not spending
		for _, x := range rec {
			if x.Kind == "bill" {
				total += x.Monthly
			}
		}
		if hidden == nil {
			hidden = []insights.Recurring{}
		}
		// Suggestions skip anything already listed, saved or dismissed.
		taken := map[string]bool{}
		for _, x := range append(append([]insights.Recurring{}, rec...), hidden...) {
			taken[strings.ToLower(x.Merchant)] = true
		}
		sugg := insights.SuggestRecurring(txs, time.Now(), taken)
		if sugg == nil {
			sugg = []insights.RecurringSuggestion{}
		}
		return map[string]any{"items": rec, "hidden": hidden, "monthly_total": total, "suggestions": sugg}, nil
	})
	saveRecurring := func(w http.ResponseWriter, r *http.Request) (any, error) {
		var it insights.RecurringItem
		if err := decode(r, &it); err != nil {
			return nil, err
		}
		if r.Method == http.MethodPut {
			id, err := idParam(r, "id")
			if err != nil {
				return nil, err
			}
			it.ID = id
		}
		if err := insights.SaveRecurringItem(s.DB, &it); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			return nil, bad(err.Error())
		}
		return it, nil
	}
	s.handle("POST /api/recurring", saveRecurring)
	s.handle("PUT /api/recurring/{id}", saveRecurring)
	s.handle("DELETE /api/recurring/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, insights.DeleteRecurringItem(s.DB, id)
	})
	s.handle("GET /api/insights/fi", func(w http.ResponseWriter, r *http.Request) (any, error) {
		txs, err := ledger.All(s.DB, ledger.Filter{From: time.Now().AddDate(-2, 0, 0).Format("2006-01-02")})
		if err != nil {
			return nil, err
		}
		cats, _ := ledger.CategoryMap(s.DB)
		book, err := wealth.LoadBook(s.DB)
		if err != nil {
			return nil, err
		}
		return insights.ComputeFI(txs, cats, book, plan.LoadSettings(s.DB), time.Now()), nil
	})
	s.handle("GET /api/insights/review", func(w http.ResponseWriter, r *http.Request) (any, error) {
		year := r.URL.Query().Get("year")
		if len(year) != 4 {
			year = time.Now().Format("2006")
		}
		return s.yearReview(year)
	})
}
