package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"ft/internal/ledger"
	"ft/internal/market"
	"ft/internal/money"
	"ft/internal/wealth"
)

// BalanceChange is one account whose value a sync moved.
type BalanceChange struct {
	Account string      `json:"account"`
	Name    string      `json:"name"`
	From    money.Cents `json:"from"`
	To      money.Cents `json:"to"`
	Source  string      `json:"source"`
}

var cryptoTicker = regexp.MustCompile(`(?i)\b(btc|bitcoin)\b`)

// revalueCrypto prices each crypto account's latest quantity at today's
// quote and records today's value (source "market"). Only today's point; a
// figure entered by hand today is kept.
func revalueCrypto(s *Server, today string) []string {
	accts, err := ledger.ListAccounts(s.DB)
	if err != nil {
		return nil
	}
	book, err := wealth.LoadBook(s.DB)
	if err != nil {
		return nil
	}
	var notes []string
	for _, a := range accts {
		if a.Kind != "crypto" || a.Archived {
			continue
		}
		pt, ok := book.At(a.ID, today)
		if !ok || pt.Quantity == nil || *pt.Quantity <= 0 || (pt.Date == today && pt.Source == "manual") {
			continue
		}
		ticker := ""
		var det struct {
			Ticker string `json:"ticker"`
		}
		if json.Unmarshal(a.Details, &det) == nil && det.Ticker != "" {
			ticker = strings.ToUpper(det.Ticker)
		} else if cryptoTicker.MatchString(a.Name + " " + a.Institution) {
			ticker = "BTC-EUR"
		}
		if ticker == "" {
			continue
		}
		q, err := market.Fetch(ticker)
		if err != nil || q.Price <= 0 || (q.Currency != "" && !strings.EqualFold(q.Currency, "EUR")) {
			notes = append(notes, a.Name+": no EUR price for "+ticker)
			continue
		}
		price := money.FromFloat(q.Price)
		qty := *pt.Quantity
		if err := wealth.SetBalance(s.DB, a.ID, today, money.FromFloat(qty*q.Price), &qty, &price, "market"); err != nil {
			notes = append(notes, a.Name+": "+err.Error())
		}
	}
	return notes
}

// autoSyncEvery is how often opening the app syncs by itself. Banks allow
// only a few account reads a day, so a phone opened every few minutes must
// not ask each time.
const autoSyncEvery = 30 * time.Minute

// The one-tap sync: banks, Interactive Brokers and crypto prices, then which
// balances moved. Every source writes today's value only. {"auto":true} is
// the app opening: skipped when a sync ran within autoSyncEvery or one is
// running now.
func (s *Server) syncRoutes() {
	s.handle("POST /api/sync", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Auto bool `json:"auto"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.Auto && time.Since(time.Unix(s.lastSync.Load(), 0)) < autoSyncEvery {
			return map[string]any{"skipped": true}, nil
		}
		if !s.syncMu.TryLock() {
			if in.Auto {
				return map[string]any{"skipped": true}, nil
			}
			return nil, &HTTPError{http.StatusConflict, "a sync is already running"}
		}
		defer s.syncMu.Unlock()
		s.lastSync.Store(time.Now().Unix())
		today := time.Now().Format("2006-01-02")
		latest := func() map[string]wealth.Point {
			out := map[string]wealth.Point{}
			if book, err := wealth.LoadBook(s.DB); err == nil {
				if accts, err := ledger.ListAccounts(s.DB); err == nil {
					for _, a := range accts {
						if pt, ok := book.At(a.ID, today); ok {
							out[a.ID] = pt
						}
					}
				}
			}
			return out
		}
		before := latest()
		out := map[string]any{}
		var problems []string

		if conns, err := s.Bank.Connections(); err == nil {
			active := false
			for i := range conns {
				if conns[i].Effective() == "authorized" {
					active = true
				}
			}
			if active {
				if res, err := s.Bank.SyncAll(r.Context(), psu(r), 0, 0); err != nil {
					problems = append(problems, "bank: "+err.Error())
				} else {
					out["bank"] = res
				}
			}
		}
		if s.IBKR != nil && s.IBKR.Connected() {
			if rep, err := s.IBKR.Sync(r.Context(), false); err != nil {
				problems = append(problems, "IBKR: "+err.Error())
			} else {
				out["ibkr"] = rep
			}
		}
		problems = append(problems, revalueCrypto(s, today)...)

		names := map[string]string{}
		if accts, err := ledger.ListAccounts(s.DB); err == nil {
			for _, a := range accts {
				names[a.ID] = a.Name
			}
		}
		changes := []BalanceChange{}
		for id, now := range latest() {
			if was, ok := before[id]; !ok || was.Value != now.Value || was.Date != now.Date {
				changes = append(changes, BalanceChange{Account: id, Name: names[id], From: before[id].Value, To: now.Value, Source: now.Source})
			}
		}
		sort.Slice(changes, func(i, j int) bool { return changes[i].Name < changes[j].Name })
		out["balances"] = changes
		if problems == nil {
			problems = []string{}
		}
		out["problems"] = problems
		return out, nil
	})
}
