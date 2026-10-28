package cfo

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/notify"
)

// Alert is something worth a push to the phone: money to move before an
// account goes short, a bill that went up, a charge that looks doubled, a
// month to close. Key identifies it so it is sent once.
type Alert struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Message string `json:"message"`
	URL     string `json:"url"`
}

// Alerts lists what is worth pushing now.
func Alerts(d *sql.DB, now time.Time) ([]Alert, error) {
	o, err := BuildOverview(d, now, 0)
	if err != nil {
		return nil, err
	}
	var out []Alert
	today := now.Format("2006-01-02")
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")

	// Money to move today or tomorrow, or an account goes short.
	if c := o.Cash; c != nil {
		for _, m := range c.Moves {
			if m.By <= tomorrow {
				when := "today"
				if m.By > today {
					when = "tomorrow"
				}
				out = append(out, Alert{Key: fmt.Sprintf("move:%s:%s:%s", m.From, m.To, m.By), Title: fmt.Sprintf("Move %s to %s %s", eurS(m.Amount), m.ToName, when),
					Message: fmt.Sprintf("From %s — %s.", m.FromName, m.For), URL: "/#/?cash=1"})
			}
		}
	}

	// A recurring bill that went up.
	txs, _ := ledger.All(d, ledger.Filter{From: now.AddDate(-2, 0, 0).Format("2006-01-02")})
	if rec, _, err := insights.RecurringCosts(d, txs, now); err == nil {
		for _, r := range rec {
			if r.Changed && r.LastAmount > r.Amount && r.Last >= now.AddDate(0, 0, -7).Format("2006-01-02") {
				out = append(out, Alert{Key: fmt.Sprintf("price:%s:%s", r.Merchant, r.Last), Title: fmt.Sprintf("%s went up", r.Merchant),
					Message: fmt.Sprintf("%s instead of the usual %s (%s).", eurS(r.LastAmount), eurS(r.Amount), r.Cadence), URL: "/#/plan/recurring"})
			}
		}
	}

	// The same amount to the same merchant twice within two days.
	recent := map[string][]ledger.Tx{}
	for _, t := range txs {
		if t.Kind == "expense" && t.Merchant != "" && t.SplitOf == 0 && t.Date >= now.AddDate(0, 0, -3).Format("2006-01-02") {
			k := fmt.Sprintf("%s|%d", t.Merchant, t.Amount)
			recent[k] = append(recent[k], t)
		}
	}
	for _, ts := range recent {
		if len(ts) < 2 {
			continue
		}
		sort.Slice(ts, func(i, j int) bool { return ts[i].Date < ts[j].Date })
		a, b := ts[0], ts[1]
		da, _ := time.Parse("2006-01-02", a.Date)
		db_, _ := time.Parse("2006-01-02", b.Date)
		if db_.Sub(da) <= 48*time.Hour {
			out = append(out, Alert{Key: fmt.Sprintf("dup:%d:%d", a.ID, b.ID), Title: fmt.Sprintf("Charged twice by %s?", a.Merchant),
				Message: fmt.Sprintf("%s on %s and again on %s — check it isn't a double charge.", eurS(a.Amount), shortDay(a.Date), shortDay(b.Date)), URL: "/#/ledger"})
		}
	}

	// The month close, from the 1st.
	if c := o.Close; c != nil && c.Done < len(c.Steps) {
		m, _ := time.Parse("2006-01", c.Month)
		out = append(out, Alert{Key: "close:" + c.Month, Title: "Close " + m.Format("January"),
			Message: fmt.Sprintf("%d of %d steps done — inbox, categories, balances, then the review and the plan.", c.Done, len(c.Steps)), URL: "/#/insights/review?month=" + c.Month})
	}
	return out, nil
}

// SendAlerts pushes alerts not sent before, during waking hours, and
// remembers them for 60 days (settings key alerts_sent). Returns how many
// went out.
func SendAlerts(d *sql.DB, now time.Time) int {
	if s := notify.Load(d); !s.Enabled || s.WebhookURL == "" {
		return 0
	}
	if h := now.Hour(); h < 8 || h >= 21 {
		return 0
	}
	alerts, err := Alerts(d, now)
	var origin string // where the app is opened, remembered by the API
	d.QueryRow(`SELECT value FROM settings WHERE key='app_origin'`).Scan(&origin)
	if err != nil {
		return 0
	}
	sent := map[string]string{}
	var raw string
	if d.QueryRow(`SELECT value FROM settings WHERE key='alerts_sent'`).Scan(&raw) == nil {
		json.Unmarshal([]byte(raw), &sent)
	}
	cutoff := now.AddDate(0, 0, -60).Format("2006-01-02")
	for k, day := range sent {
		if day < cutoff {
			delete(sent, k)
		}
	}
	n := 0
	for _, a := range alerts {
		if _, ok := sent[a.Key]; ok {
			continue
		}
		link := ""
		if origin != "" {
			link = strings.TrimRight(origin, "/") + a.URL
		}
		if err := notify.Post(notify.Load(d), notify.Message{Title: a.Title, Message: a.Message, URL: link, Tag: a.Key}); err != nil {
			continue
		}
		sent[a.Key] = now.Format("2006-01-02")
		n++
	}
	b, _ := json.Marshal(sent)
	d.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('alerts_sent',?)`, string(b))
	return n
}
