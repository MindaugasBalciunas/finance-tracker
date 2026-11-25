package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"ft/internal/cfo"
	"ft/internal/dip"
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
		t := book.Table(page, size)
		if n := len(t.Rows); n > 0 {
			if rd, err := wealth.Readings(s.DB, t.Rows[n-1].Date, t.Rows[0].Date); err == nil {
				for i := range t.Rows {
					t.Rows[i].Readings = rd[t.Rows[i].Date]
				}
			}
		}
		return t, nil
	})
	s.handle("GET /api/balances", func(w http.ResponseWriter, r *http.Request) (any, error) {
		book, err := wealth.LoadBook(s.DB)
		if err != nil {
			return nil, err
		}
		pts := []wealth.Point{}
		from := r.URL.Query().Get("from")
		for _, p := range book.Series(r.URL.Query().Get("account")) {
			if p.Date >= from {
				pts = append(pts, p)
			}
		}
		if n := qint(r, "limit", 0); n > 0 && len(pts) > n {
			pts = pts[len(pts)-n:] // the latest
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
	// What the portfolio really earned, against its core fund, and where
	// the next contribution should go.
	s.handle("GET /api/portfolio/quality", func(w http.ResponseWriter, r *http.Request) (any, error) {
		trades, err := wealth.ListTrades(s.DB)
		if err != nil {
			return nil, err
		}
		p := wealth.BuildPortfolio(trades, true)
		st := wealth.LoadInvestSettings(s.DB, p)
		q := wealth.BuildQuality(trades, p, st, func(tk string) ([]market.HistoryPoint, error) { return market.History(tk, "max") }, wealth.LiveEUR(), time.Now())
		return map[string]any{"quality": q, "settings": st}, nil
	})
	s.handle("PUT /api/portfolio/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in wealth.InvestSettings
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		var sum float64
		for t, w := range in.Targets {
			if !wealth.ValidTicker(t) || w < 0 || w > 1 {
				return nil, bad("invalid target")
			}
			sum += w
		}
		if sum > 1.0001 || in.Monthly < 0 || in.MaxSingle < 0 || in.MaxSingle > 1 || (in.Benchmark != "" && !wealth.ValidTicker(in.Benchmark)) {
			return nil, bad("targets must add up to 100% or less")
		}
		for _, t := range in.Core {
			if !wealth.ValidTicker(t) {
				return nil, bad("invalid ticker")
			}
		}
		if err := wealth.SaveInvestSettings(s.DB, in); err != nil {
			return nil, err
		}
		return in, nil
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
	// News headlines, public social chatter (unverified) and Fear & Greed — for the assistants.
	s.handle("GET /api/market/buzz/{ticker}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		t, err := ticker(r)
		if err != nil {
			return nil, err
		}
		return map[string]any{"fear_greed": market.FearGreed(), "headlines": market.Headlines(t, 8), "social": market.SocialBuzz(t, 8)}, nil
	})
	// The crash plan: where each ETF sits against its ladder of limit
	// prices. Under /api/market so read tokens (Ask CFO, MCP) can read it;
	// changing the plan stays with the browser session.
	// ?plan=bets reads the single-stock bets at Revolut on the same ladder.
	s.handle("GET /api/market/dips", func(w http.ResponseWriter, r *http.Request) (any, error) {
		plan, err := dipPlan(r.URL.Query().Get("plan"))
		if err != nil {
			return nil, err
		}
		st := dip.LoadPlan(s.DB, plan)
		if plan == dip.Bets && !st.Saved {
			st = s.betsDefaults()
		}
		p := dip.Build(st, s.daily())
		if o, err := cfo.BuildOverview(s.DB, time.Now(), 0); err == nil && o.EmergencyPlan != nil && o.EmergencyPlan.Target > 0 {
			// Funds, then bets, then crypto: what is left of the earlier
			// plans' reserves has the first claim on the cash above the
			// safety net (what was bought already left the bank).
			p.Cash(o.EmergencyPlan.Target, dip.FreeAfter(s.DB, plan, o.EmergencyPlan.Spare))
		}
		if plan == dip.Funds {
			p.Monthly = money.FromFloat(wealth.LoadInvestSettings(s.DB, wealth.Portfolio{}).Monthly)
		}
		if r.URL.Query().Get("brief") == "1" { // the AI tools don't need the chart
			for i := range p.Funds {
				p.Funds[i].Chart = []dip.Point{}
			}
		}
		return p, nil
	})
	s.handle("PUT /api/market/dips/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		plan, err := dipPlan(r.URL.Query().Get("plan"))
		if err != nil {
			return nil, err
		}
		var in dip.Settings
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if in.Funds == nil {
			in.Funds = []dip.Fund{}
		}
		if err := in.Validate(); err != nil {
			return nil, bad(err.Error())
		}
		// One symbol, one plan: the bell and the phone speak per symbol.
		for _, other := range dip.Plans {
			if other == plan {
				continue
			}
			// By ticker, not listing: OKLO and OKLO.DE share one set of
			// trades, so one buy would count on both plans.
			on := map[string]bool{}
			st := dip.LoadPlan(s.DB, other)
			if other == dip.Bets && !st.Saved {
				st = s.betsDefaults()
			}
			for _, f := range st.Funds {
				on[dip.Ticker(f.Symbol)] = true
			}
			for _, f := range in.Funds {
				if on[dip.Ticker(f.Symbol)] {
					return nil, bad(dip.Ticker(f.Symbol) + " is already on the " + other + " plan")
				}
			}
		}
		return in, dip.SavePlan(s.DB, plan, in)
	})
	s.handle("GET /api/market/analyst/{ticker}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		t, err := ticker(r)
		if err != nil {
			return nil, err
		}
		return market.Analyst(t)
	})
}

func (s *Server) daily() func(string) (*market.Series, error) {
	if s.Daily != nil {
		return s.Daily
	}
	return market.Daily
}

// dipPlan reads ?plan= (funds when empty).
func dipPlan(v string) (string, error) {
	if v == "" {
		return dip.Funds, nil
	}
	if !dip.ValidPlan(v) {
		return "", bad("unknown plan")
	}
	return v, nil
}

// betsDefaults is the bets plan before it is saved: the single stocks held
// at Revolut Stocks (funds, and anything on the ETF plan, are left out), each on the Yahoo
// listing with the longest history — 2X1 (ABIVAX) has 18 days on Xetra and
// ten years in Frankfurt.
func (s *Server) betsDefaults() dip.Settings {
	out := dip.Settings{Funds: []dip.Fund{}}
	trades, err := wealth.ListTrades(s.DB)
	if err != nil {
		return out
	}
	held := map[string]float64{}
	order := []string{}
	for _, t := range trades {
		if t.AccountID != "revolut_stocks" || wealth.IsFund(t.Ticker, "") {
			continue
		}
		if _, ok := held[t.Ticker]; !ok {
			order = append(order, t.Ticker)
		}
		if t.Action == "sell" {
			held[t.Ticker] -= t.Shares
		} else {
			held[t.Ticker] += t.Shares
		}
	}
	onFunds := map[string]bool{} // VALL isn't a "well-known" fund, but it is on the ETF plan
	for _, f := range dip.LoadPlan(s.DB, dip.Funds).Funds {
		base, _, _ := strings.Cut(f.Symbol, ".")
		onFunds[base] = true
	}
	daily := s.daily()
	for _, tk := range order {
		if held[tk] <= 1e-6 || onFunds[tk] {
			continue
		}
		best, bars := tk, 0
		for _, sym := range []string{tk, tk + ".DE", tk + ".F"} {
			if ser, err := daily(sym); err == nil && len(ser.Bars) > bars {
				best, bars = sym, len(ser.Bars)
			}
			if bars > 1000 {
				break
			}
		}
		out.Funds = append(out.Funds, dip.Fund{Symbol: best, Name: tk})
	}
	return out
}
