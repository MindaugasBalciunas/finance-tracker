package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"ft/internal/ledger"
	"ft/internal/market"
	"ft/internal/money"
	"ft/internal/wealth"
)

// ── wealth ──────────────────────────────────────────────────────────

func (s *Server) wealthRoutes() {
	s.handle("GET /api/networth", func(w http.ResponseWriter, r *http.Request) (any, error) {
		book, err := wealth.LoadBook(s.DB)
		if err != nil {
			return nil, err
		}
		d := r.URL.Query().Get("date")
		if d == "" {
			d = today()
		}
		return book.SnapshotAt(d, true), nil
	})
	s.handle("GET /api/networth/history", func(w http.ResponseWriter, r *http.Request) (any, error) {
		book, err := wealth.LoadBook(s.DB)
		if err != nil {
			return nil, err
		}
		q := r.URL.Query()
		hist := book.History(q.Get("from"), q.Get("to"), q.Get("step"))
		if q.Get("accounts") == "1" {
			for i := range hist {
				hist[i] = book.SnapshotAt(hist[i].Date, true)
				hist[i].Stale = nil
			}
		}
		return hist, nil
	})
	s.handle("GET /api/networth/movement", func(w http.ResponseWriter, r *http.Request) (any, error) {
		book, err := wealth.LoadBook(s.DB)
		if err != nil {
			return nil, err
		}
		q := r.URL.Query()
		from, to := q.Get("from"), q.Get("to")
		if to == "" {
			to = today()
		}
		if from == "" {
			from = time.Now().AddDate(-1, 0, 0).Format("2006-01-02")
		}
		mv := book.Movement(from, to)
		if mv == nil {
			mv = []wealth.Move{}
		}
		return mv, nil
	})
	s.handle("GET /api/balances/table", func(w http.ResponseWriter, r *http.Request) (any, error) {
		book, err := wealth.LoadBook(s.DB)
		if err != nil {
			return nil, err
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		size, _ := strconv.Atoi(r.URL.Query().Get("size"))
		return book.Table(page, size), nil
	})
	s.handle("GET /api/balances", func(w http.ResponseWriter, r *http.Request) (any, error) {
		book, err := wealth.LoadBook(s.DB)
		if err != nil {
			return nil, err
		}
		pts := book.Series(r.URL.Query().Get("account"))
		if pts == nil {
			pts = []wealth.Point{}
		}
		return pts, nil
	})
	s.handle("POST /api/balances", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Date   string `json:"date"`
			Values []struct {
				AccountID string       `json:"account_id"`
				Value     *money.Cents `json:"value"`
				Quantity  *float64     `json:"quantity"`
				Price     *money.Cents `json:"price"`
			} `json:"values"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if in.Date == "" {
			in.Date = today()
		}
		tx, err := s.DB.Begin()
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		n := 0
		for _, v := range in.Values {
			if _, err := ledger.GetAccount(tx, v.AccountID); err != nil {
				return nil, bad("unknown account " + v.AccountID)
			}
			val := money.Cents(0)
			switch {
			case v.Quantity != nil && v.Price != nil:
				val = money.FromFloat(*v.Quantity * v.Price.Float())
			case v.Value != nil:
				val = *v.Value
			default:
				continue
			}
			if a, _ := ledger.GetAccount(tx, v.AccountID); a.Kind == "loan" && val > 0 {
				val = -val
			}
			if err := wealth.SetBalance(tx, v.AccountID, in.Date, val, v.Quantity, v.Price, "manual"); err != nil {
				return nil, err
			}
			n++
		}
		return map[string]int{"saved": n}, tx.Commit()
	})
	s.handle("DELETE /api/balances", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]bool{"ok": true}, wealth.DeleteBalance(s.DB, r.URL.Query().Get("account"), r.URL.Query().Get("date"))
	})
	s.handle("GET /api/loans", func(w http.ResponseWriter, r *http.Request) (any, error) {
		book, err := wealth.LoadBook(s.DB)
		if err != nil {
			return nil, err
		}
		txs, _ := ledger.All(s.DB, ledger.Filter{From: time.Now().AddDate(-1, 0, 0).Format("2006-01-02")})
		return wealth.Loans(book, txs, time.Now()), nil
	})
	s.handle("GET /api/portfolio", func(w http.ResponseWriter, r *http.Request) (any, error) {
		trades, err := wealth.ListTrades(s.DB)
		if err != nil {
			return nil, err
		}
		return wealth.BuildPortfolio(trades, r.URL.Query().Get("live") != "0"), nil
	})
	s.handle("GET /api/portfolio/history", func(w http.ResponseWriter, r *http.Request) (any, error) {
		trades, err := wealth.ListTrades(s.DB)
		if err != nil {
			return nil, err
		}
		// Closes: daily up to 3 months, weekly beyond (Yahoo's own split).
		rng, from := "max", ""
		now := time.Now()
		switch r.URL.Query().Get("range") {
		case "3m":
			rng, from = "3mo", now.AddDate(0, -3, 0).Format("2006-01-02")
		case "6m":
			rng, from = "6mo", now.AddDate(0, -6, 0).Format("2006-01-02")
		case "ytd":
			rng, from = "ytd", strconv.Itoa(now.Year())+"-01-01"
		case "1y":
			rng, from = "1y", now.AddDate(-1, 0, 0).Format("2006-01-02")
		case "3y":
			rng, from = "5y", now.AddDate(-3, 0, 0).Format("2006-01-02")
		}
		return wealth.PortfolioHistory(trades, from, now.Format("2006-01-02"),
			func(tk string) ([]market.HistoryPoint, error) { return market.History(tk, rng) }, wealth.LiveEUR()), nil
	})
	s.handle("GET /api/portfolio/scenarios", func(w http.ResponseWriter, r *http.Request) (any, error) {
		trades, err := wealth.ListTrades(s.DB)
		if err != nil {
			return nil, err
		}
		return wealth.BuildScenarios(wealth.BuildPortfolio(trades, true), func(t string) (float64, float64, float64, int, string, error) {
			a, err := market.Analyst(t)
			if err != nil {
				return 0, 0, 0, 0, "", err
			}
			basis := "analyst targets"
			if a.NumAnalysts == 0 {
				basis = "52-week range"
			}
			return a.TargetLow, a.TargetMean, a.TargetHigh, a.NumAnalysts, basis, nil
		}), nil
	})
	s.handle("GET /api/trades", func(w http.ResponseWriter, r *http.Request) (any, error) { return wealth.ListTrades(s.DB) })
	saveTrade := func(w http.ResponseWriter, r *http.Request) (any, error) {
		var t wealth.Trade
		if err := decode(r, &t); err != nil {
			return nil, err
		}
		if r.PathValue("id") != "" {
			t.ID, _ = idParam(r, "id")
		}
		return t, wealth.SaveTrade(s.DB, &t)
	}
	s.handle("POST /api/trades", saveTrade)
	s.handle("PUT /api/trades/{id}", saveTrade)
	s.handle("DELETE /api/trades/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, wealth.DeleteTrade(s.DB, id)
	})
	ticker := func(r *http.Request) (string, error) {
		t := strings.ToUpper(r.PathValue("ticker"))
		if !wealth.ValidTicker(t) {
			return "", bad("invalid ticker")
		}
		return t, nil
	}
	s.handle("GET /api/market/quote/{ticker}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		t, err := ticker(r)
		if err != nil {
			return nil, err
		}
		return market.Fetch(t)
	})
	s.handle("GET /api/market/history/{ticker}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		t, err := ticker(r)
		if err != nil {
			return nil, err
		}
		rng := r.URL.Query().Get("range")
		switch rng {
		case "1mo", "3mo", "6mo", "ytd", "1y", "2y", "5y", "max":
		default:
			rng = "1y"
		}
		return market.History(t, rng)
	})
	s.handle("GET /api/market/analyst/{ticker}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		t, err := ticker(r)
		if err != nil {
			return nil, err
		}
		return market.Analyst(t)
	})
}
