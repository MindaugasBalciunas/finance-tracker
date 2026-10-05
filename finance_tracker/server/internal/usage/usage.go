// Package usage keeps private, on-device analytics of how the app is used:
// which pages, how long, and — most useful — the detours taken before landing
// on the page that was wanted. Nothing leaves the owner's database.
package usage

import (
	"database/sql"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Event struct {
	At      string   `json:"at"` // RFC3339 (client clock)
	Kind    string   `json:"kind"`
	Path    string   `json:"path"`
	From    string   `json:"from"`
	Label   string   `json:"label"`
	DwellMS int64    `json:"dwell_ms"`
	X       *float64 `json:"x,omitempty"`  // click: 0–1 across the page
	Y       *float64 `json:"y,omitempty"`  // click: 0–1 down the page
	VW      int      `json:"vw,omitempty"` // viewport width, px
}

const retention = 180 * 24 * time.Hour

var (
	digits   = regexp.MustCompile(`[0-9]+([.,][0-9]+)?`)
	pathSafe = regexp.MustCompile(`^/[a-z0-9/_-]{0,80}$`)
)

// clean strips anything that could carry personal data: query strings,
// numbers in labels (amounts, counts), overlong text.
func clean(e *Event) error {
	if e.Kind != "view" && e.Kind != "action" {
		return errors.New("kind must be view or action")
	}
	norm := func(p string) string {
		if i := strings.IndexAny(p, "?#"); i >= 0 {
			p = p[:i]
		}
		p = strings.ToLower(strings.TrimRight(p, "/"))
		if p == "" {
			p = "/"
		}
		if !pathSafe.MatchString(p) {
			return ""
		}
		return p
	}
	e.Path, e.From = norm(e.Path), norm(e.From)
	if e.Path == "" {
		return errors.New("invalid path")
	}
	e.Label = strings.TrimSpace(digits.ReplaceAllString(e.Label, "#"))
	if len(e.Label) > 48 {
		e.Label = e.Label[:48]
	}
	if e.DwellMS < 0 || e.DwellMS > int64(6*time.Hour/time.Millisecond) {
		e.DwellMS = 0
	}
	for _, f := range []**float64{&e.X, &e.Y} {
		if *f != nil && (**f < 0 || **f > 1) {
			*f = nil
		}
	}
	if e.VW < 0 || e.VW > 10000 {
		e.VW = 0
	}
	t, err := time.Parse(time.RFC3339Nano, e.At)
	if err != nil || t.After(time.Now().Add(time.Hour)) {
		t = time.Now()
	}
	e.At = t.UTC().Format(time.RFC3339)
	return nil
}

// Record stores a batch (≤200) and prunes events past the retention.
func Record(d *sql.DB, events []Event) (int, error) {
	if len(events) > 200 {
		events = events[:200]
	}
	tx, err := d.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n := 0
	for i := range events {
		e := events[i]
		if clean(&e) != nil {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO usage_events(at,kind,path,from_path,label,dwell_ms,x,y,vw) VALUES(?,?,?,?,?,?,?,?,?)`,
			e.At, e.Kind, e.Path, e.From, e.Label, e.DwellMS, e.X, e.Y, e.VW); err != nil {
			return n, err
		}
		n++
	}
	tx.Exec(`DELETE FROM usage_events WHERE at < ?`, time.Now().Add(-retention).UTC().Format(time.RFC3339))
	return n, tx.Commit()
}

// Events returns the raw log of the last `days` days (for analysis and heat
// maps in the Claude CLI).
func Events(d *sql.DB, days int, now time.Time) ([]Event, error) {
	rows, err := d.Query(`SELECT at,kind,path,from_path,label,dwell_ms,x,y,vw FROM usage_events WHERE at >= ? ORDER BY at, id`,
		now.AddDate(0, 0, -days).UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var x, y sql.NullFloat64
		if err := rows.Scan(&e.At, &e.Kind, &e.Path, &e.From, &e.Label, &e.DwellMS, &x, &y, &e.VW); err != nil {
			return nil, err
		}
		if x.Valid {
			e.X = &x.Float64
		}
		if y.Valid {
			e.Y = &y.Float64
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func Clear(d *sql.DB) error {
	_, err := d.Exec(`DELETE FROM usage_events`)
	return err
}

type PageStat struct {
	Path     string  `json:"path"`
	Views    int     `json:"views"`
	AvgSec   float64 `json:"avg_sec"`
	TotalMin float64 `json:"total_min"`
	Bounces  int     `json:"bounces"` // opened and left within seconds, back to where you came from
	LastSeen string  `json:"last_seen"`
}

type ActionStat struct {
	Path  string `json:"path"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// Hunt is a page you had to look for: several quick hops before staying.
type Hunt struct {
	Target   string   `json:"target"`
	Count    int      `json:"count"`
	AvgHops  float64  `json:"avg_hops"`
	AvgSec   float64  `json:"avg_sec"` // time spent hopping
	Entry    string   `json:"entry"`   // where those searches usually started
	TypicalP []string `json:"typical_path"`
}

type DayCount struct {
	Day   string `json:"day"`
	Views int    `json:"views"`
}

type Summary struct {
	Since       string            `json:"since"`
	Sessions    int               `json:"sessions"`
	Views       int               `json:"views"`
	Pages       []PageStat        `json:"pages"`
	Actions     []ActionStat      `json:"actions"`
	Hunts       []Hunt            `json:"hunts"`
	Days        []DayCount        `json:"days"`
	Unused      []string          `json:"unused"` // known pages not opened in the window
	Suggestions []string          `json:"suggestions"`
	Names       map[string]string `json:"names"` // display name of every path above (one source of truth)
}

// Pages the app has, for "never used" detection.
var KnownPages = []string{"/", "/ledger", "/ledger/inbox", "/ledger/tidy", "/plan", "/plan/trips", "/plan/income", "/wealth", "/wealth/investments",
	"/wealth/loans", "/wealth/history", "/insights", "/insights/spending", "/insights/trends", "/insights/recurring", "/insights/review", "/insights/fi",
	"/ai", "/settings/categories", "/settings/accounts", "/settings/rules", "/settings/tags", "/settings/banks", "/settings/ai", "/settings/security",
	"/settings/data", "/settings/appearance"}

const (
	sessionGap = 30 * time.Minute
	quickMS    = 8_000  // a hop: left within 8 s
	stayMS     = 20_000 // found it: stayed 20 s or more
	minHops    = 3
)

// Summarise reads the last `days` days.
func Summarise(d *sql.DB, days int, now time.Time) (*Summary, error) {
	since := now.AddDate(0, 0, -days).UTC().Format(time.RFC3339)
	rows, err := d.Query(`SELECT at,kind,path,from_path,label,dwell_ms FROM usage_events WHERE at >= ? ORDER BY at, id`, since)
	if err != nil {
		return nil, err
	}
	var evs []Event
	for rows.Next() {
		var e Event
		rows.Scan(&e.At, &e.Kind, &e.Path, &e.From, &e.Label, &e.DwellMS)
		evs = append(evs, e)
	}
	rows.Close()
	s := &Summary{Since: since[:10], Pages: []PageStat{}, Actions: []ActionStat{}, Hunts: []Hunt{}, Days: []DayCount{}, Unused: []string{}, Suggestions: []string{}}

	pages := map[string]*PageStat{}
	actions := map[[2]string]int{}
	byDay := map[string]int{}
	var views []Event
	for _, e := range evs {
		if e.Kind == "action" {
			actions[[2]string{e.Path, e.Label}]++
			continue
		}
		views = append(views, e)
		p := pages[e.Path]
		if p == nil {
			p = &PageStat{Path: e.Path}
			pages[e.Path] = p
		}
		p.Views++
		p.TotalMin += float64(e.DwellMS) / 60000
		if e.At > p.LastSeen {
			p.LastSeen = e.At
		}
		byDay[e.At[:10]]++
	}
	s.Views = len(views)

	// Sessions, bounces and hunts from the ordered views.
	type hop struct {
		path  string
		dwell int64
		at    time.Time
	}
	var session []hop
	huntAgg := map[string]*struct {
		n, hops  int
		secs     float64
		entries  map[string]int
		paths    map[string]int
		pathList map[string][]string
	}{}
	flush := func() {
		if len(session) == 0 {
			return
		}
		s.Sessions++
		for i := 1; i+1 < len(session); i++ { // A → B → A within seconds: B bounced
			if session[i].dwell < quickMS && session[i-1].path == session[i+1].path && session[i].path != session[i-1].path {
				if p := pages[session[i].path]; p != nil {
					p.Bounces++
				}
			}
		}
		run := 0
		for i, h := range session {
			if h.dwell < quickMS {
				run++
				continue
			}
			if h.dwell >= stayMS && run >= minHops {
				hops := session[i-run : i]
				a := huntAgg[h.path]
				if a == nil {
					a = &struct {
						n, hops  int
						secs     float64
						entries  map[string]int
						paths    map[string]int
						pathList map[string][]string
					}{entries: map[string]int{}, paths: map[string]int{}, pathList: map[string][]string{}}
					huntAgg[h.path] = a
				}
				a.n++
				a.hops += len(hops)
				var trail []string
				for _, x := range hops {
					a.secs += float64(x.dwell) / 1000
					trail = append(trail, x.path)
				}
				a.entries[hops[0].path]++
				key := strings.Join(trail, " → ")
				a.paths[key]++
				a.pathList[key] = trail
			}
			run = 0
		}
		session = session[:0]
	}
	var last time.Time
	for _, v := range views {
		t, _ := time.Parse(time.RFC3339, v.At)
		if !last.IsZero() && t.Sub(last) > sessionGap {
			flush()
		}
		session = append(session, hop{v.Path, v.DwellMS, t})
		last = t.Add(time.Duration(v.DwellMS) * time.Millisecond)
	}
	flush()

	for _, p := range pages {
		if p.Views > 0 {
			p.AvgSec = round1(p.TotalMin * 60 / float64(p.Views))
		}
		p.TotalMin = round1(p.TotalMin)
		s.Pages = append(s.Pages, *p)
	}
	sort.Slice(s.Pages, func(i, j int) bool { return s.Pages[i].Views > s.Pages[j].Views })
	for k, n := range actions {
		s.Actions = append(s.Actions, ActionStat{Path: k[0], Label: k[1], Count: n})
	}
	sort.Slice(s.Actions, func(i, j int) bool { return s.Actions[i].Count > s.Actions[j].Count })
	if len(s.Actions) > 30 {
		s.Actions = s.Actions[:30]
	}
	for target, a := range huntAgg {
		h := Hunt{Target: target, Count: a.n, AvgHops: round1(float64(a.hops) / float64(a.n)), AvgSec: round1(a.secs / float64(a.n))}
		h.Entry = top(a.entries)
		h.TypicalP = a.pathList[top(a.paths)]
		s.Hunts = append(s.Hunts, h)
	}
	sort.Slice(s.Hunts, func(i, j int) bool { return s.Hunts[i].Count > s.Hunts[j].Count })
	for day := now.AddDate(0, 0, -days+1); !day.After(now); day = day.AddDate(0, 0, 1) {
		k := day.UTC().Format("2006-01-02")
		s.Days = append(s.Days, DayCount{Day: k, Views: byDay[k]})
	}
	for _, p := range KnownPages {
		if pages[p] == nil {
			s.Unused = append(s.Unused, p)
		}
	}
	if s.Suggestions = suggest(s); s.Suggestions == nil {
		s.Suggestions = []string{}
	}
	s.Names = map[string]string{}
	name := func(p string) {
		if p != "" {
			s.Names[p] = Label(p)
		}
	}
	for _, p := range s.Pages {
		name(p.Path)
	}
	for _, a := range s.Actions {
		name(a.Path)
	}
	for _, h := range s.Hunts {
		name(h.Target)
		name(h.Entry)
		for _, p := range h.TypicalP {
			name(p)
		}
	}
	for _, p := range s.Unused {
		name(p)
	}
	return s, nil
}

// suggest turns the numbers into concrete layout ideas.
func suggest(s *Summary) []string {
	var out []string
	for i, h := range s.Hunts {
		if i >= 3 {
			break
		}
		out = append(out, "You hunted for "+Label(h.Target)+" "+plural(h.Count, "time")+" (≈"+strconv.FormatFloat(h.AvgHops, 'f', 1, 64)+" hops, usually starting on "+Label(h.Entry)+") — put a shortcut to it on "+Label(h.Entry)+".")
	}
	for _, p := range s.Pages {
		if p.Views >= 5 && float64(p.Bounces)/float64(p.Views) >= 0.4 {
			out = append(out, Label(p.Path)+" is often opened and left within seconds ("+plural(p.Bounces, "bounce")+" of "+plural(p.Views, "view")+") — its name may not say what's there.")
		}
	}
	if s.Views >= 50 && len(s.Unused) > 0 {
		names := make([]string, 0, len(s.Unused))
		for _, u := range s.Unused {
			names = append(names, Label(u))
		}
		if len(names) > 5 {
			names = append(names[:5], "…")
		}
		out = append(out, "Never opened in this period: "+strings.Join(names, ", ")+" — candidates to fold away.")
	}
	return out
}

// Label names a route the way the app does.
func Label(p string) string {
	names := map[string]string{"/": "Home", "/ledger": "Ledger", "/ledger/inbox": "Bank inbox", "/ledger/tidy": "Tidy up", "/plan": "Plan", "/plan/trips": "Trips",
		"/plan/income": "Income & goals", "/wealth": "Net worth", "/wealth/investments": "Investments", "/wealth/loans": "Loans", "/wealth/history": "Balance history",
		"/insights": "Cash flow", "/insights/spending": "Spending", "/insights/trends": "Trends", "/insights/recurring": "Recurring", "/insights/review": "Review",
		"/insights/fi": "Independence", "/ai": "Ask CFO"}
	if n, ok := names[p]; ok {
		return n
	}
	settings := map[string]string{"categories": "Categories", "accounts": "Accounts", "rules": "Rules", "tags": "Tags", "banks": "Banks", "ai": "AI",
		"security": "Security", "data": "Data & backup", "usage": "Usage", "appearance": "Appearance"}
	if sec, ok := strings.CutPrefix(p, "/settings/"); ok {
		if n, ok := settings[sec]; ok {
			return "Settings → " + n
		}
	}
	if p == "/settings" {
		return "Settings"
	}
	return p
}

func top(m map[string]int) string {
	best, n := "", -1
	for k, v := range m {
		if v > n || (v == n && k < best) {
			best, n = k, v
		}
	}
	return best
}

func round1(v float64) float64 { return float64(int64(v*10+0.5)) / 10 }
func plural(n int, w string) string {
	if n == 1 {
		return "1 " + w
	}
	return strconv.Itoa(n) + " " + w + "s"
}
