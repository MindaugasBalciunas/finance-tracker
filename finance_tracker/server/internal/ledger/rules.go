package ledger

import (
	"database/sql"
	"strings"

	"ft/internal/db"
	"ft/internal/merchant"
)

// Rule fills in a transaction the user did not describe fully. Every new row
// — typed in, scanned from a receipt, or fetched from the bank — goes
// through the same Apply, so the three paths cannot disagree.
type Rule struct {
	ID           int64    `json:"id"`
	Priority     int      `json:"priority"`
	Pattern      string   `json:"pattern"`
	WhenCategory string   `json:"when_category"`
	WhenKind     string   `json:"when_kind"`
	SetCategory  string   `json:"set_category"`
	SetMerchant  string   `json:"set_merchant"`
	AddTags      []string `json:"add_tags"`
	Enabled      bool     `json:"enabled"`
	Hits         int      `json:"hits"`
}

func ListRules(q querier) ([]Rule, error) {
	rows, err := q.Query(`SELECT id,priority,pattern,when_category,when_kind,set_category,set_merchant,add_tags,enabled FROM rules ORDER BY priority, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Rule
	for rows.Next() {
		var r Rule
		var tags string
		if err := rows.Scan(&r.ID, &r.Priority, &r.Pattern, &r.WhenCategory, &r.WhenKind, &r.SetCategory, &r.SetMerchant, &tags, &r.Enabled); err != nil {
			return nil, err
		}
		r.AddTags = SplitTags(tags)
		if r.AddTags == nil {
			r.AddTags = []string{}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func SaveRule(e execer, r *Rule) error {
	r.Pattern = strings.TrimSpace(r.Pattern)
	tags := strings.Join(NormalizeTags(r.AddTags), ",")
	if r.Priority == 0 {
		r.Priority = 100
	}
	if r.ID == 0 {
		res, err := e.Exec(`INSERT INTO rules(priority,pattern,when_category,when_kind,set_category,set_merchant,add_tags,enabled,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			r.Priority, r.Pattern, r.WhenCategory, r.WhenKind, r.SetCategory, r.SetMerchant, tags, r.Enabled, db.Now())
		if err != nil {
			return err
		}
		r.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := e.Exec(`UPDATE rules SET priority=?,pattern=?,when_category=?,when_kind=?,set_category=?,set_merchant=?,add_tags=?,enabled=? WHERE id=?`,
		r.Priority, r.Pattern, r.WhenCategory, r.WhenKind, r.SetCategory, r.SetMerchant, tags, r.Enabled, r.ID)
	return err
}

func DeleteRule(e execer, id int64) error {
	_, err := e.Exec(`DELETE FROM rules WHERE id=?`, id)
	return err
}

// Matches reports whether the rule applies to t as it currently stands.
func (r Rule) Matches(t *Tx) bool {
	if !r.Enabled {
		return false
	}
	return r.MatchesFolded(t, FoldTx(t))
}

// Folded is a transaction's merchant and note, folded once for matching many
// rules against it.
type Folded struct{ Merchant, Note string }

func FoldTx(t *Tx) Folded { return Folded{merchant.Fold(t.Merchant), merchant.Fold(t.Note)} }

// MatchesFolded is Matches with the transaction text pre-folded; it ignores
// Enabled so statistics can cover switched-off rules too.
func (r Rule) MatchesFolded(t *Tx, f Folded) bool { return NewMatcher(r).Match(t, f) }

// Matcher is a rule with its pattern folded once, for replaying many rules
// over the whole ledger.
type Matcher struct {
	Rule
	pat string
}

func NewMatcher(r Rule) Matcher { return Matcher{r, merchant.Fold(r.Pattern)} }

func (m Matcher) Match(t *Tx, f Folded) bool {
	r := m.Rule
	if r.WhenKind != "" && r.WhenKind != t.Kind {
		return false
	}
	if r.WhenCategory != "" && t.Category != r.WhenCategory && !strings.HasPrefix(t.Category, r.WhenCategory+".") {
		return false
	}
	if r.Pattern == "" {
		return r.WhenCategory != "" || r.WhenKind != ""
	}
	p := m.pat
	if strings.HasPrefix(p, "^") {
		p = strings.TrimPrefix(p, "^")
		return strings.HasPrefix(f.Merchant, p) || strings.HasPrefix(f.Note, p)
	}
	return strings.Contains(f.Merchant, p) || strings.Contains(f.Note, p)
}

// Engine applies rules plus what the ledger's history says.
type Engine struct {
	rules []Rule
	cats  map[string]Category
	// learned: merchant → most frequent category, per kind.
	learned map[string]string
}

// LoadEngine snapshots the rules and the merchant→category history.
func LoadEngine(q querier) (*Engine, error) {
	rules, err := ListRules(q)
	if err != nil {
		return nil, err
	}
	cats, err := CategoryMap(q)
	if err != nil {
		return nil, err
	}
	e := &Engine{rules: rules, cats: cats, learned: map[string]string{}}
	rows, err := q.Query(`SELECT kind, merchant, category, COUNT(*) n FROM transactions
		WHERE merchant != '' AND date >= date('now','-3 years') GROUP BY kind, merchant, category ORDER BY n`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, m, c string
		var n int
		rows.Scan(&kind, &m, &c, &n)
		e.learned[kind+"|"+strings.ToLower(m)] = c // ascending, so the most frequent wins
	}
	return e, nil
}

// Outcome reports what Apply changed, for the UI's "why" hint.
type Outcome struct {
	Category string   `json:"category,omitempty"`
	Merchant string   `json:"merchant,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Learned  bool     `json:"learned,omitempty"`
	RuleIDs  []int64  `json:"rule_ids,omitempty"`
}

// Apply fills merchant, category and tags. guessedCategory says the current
// category is a placeholder that may be replaced; an explicit category is
// never overwritten. suppressed lists tags the user removed by hand.
func (e *Engine) Apply(t *Tx, guessedCategory bool, suppressed []string) Outcome {
	var out Outcome
	userMerchant := t.Merchant != ""
	ruleMerchant := false
	if t.Merchant == "" {
		if m := merchant.Resolve(t.Note); m != "" {
			t.Merchant = m
			out.Merchant = m
		}
	}
	catSet := !guessedCategory && t.Category != ""
	sup := map[string]bool{}
	for _, s := range suppressed {
		sup[strings.ToLower(s)] = true
	}
	for _, r := range e.rules {
		if !r.Matches(t) {
			continue
		}
		out.RuleIDs = append(out.RuleIDs, r.ID)
		if r.SetMerchant != "" && !userMerchant && !ruleMerchant {
			t.Merchant = r.SetMerchant
			out.Merchant = r.SetMerchant
			ruleMerchant = true
		}
		if r.SetCategory != "" && !catSet {
			if c, ok := e.cats[r.SetCategory]; ok && (t.Kind == "" || c.Kind == t.Kind) {
				t.Category = r.SetCategory
				t.Kind = c.Kind
				out.Category = r.SetCategory
				catSet = true
			}
		}
		for _, tag := range r.AddTags {
			if !sup[tag] && !HasTag(JoinTags(t.Tags), tag) {
				t.Tags = append(t.Tags, tag)
				out.Tags = append(out.Tags, tag)
			}
		}
	}
	if !catSet && t.Merchant != "" {
		if c, ok := e.learned[t.Kind+"|"+strings.ToLower(t.Merchant)]; ok {
			t.Category = c
			out.Category = c
			out.Learned = true
		}
	}
	t.Tags = NormalizeTags(t.Tags)
	return out
}

// Suggest returns the category history suggests for a merchant/kind, or "".
func (e *Engine) Suggest(kind, m string) string {
	return e.learned[kind+"|"+strings.ToLower(m)]
}

// PreviewRule counts and samples the existing transactions a rule would touch.
func PreviewRule(d *sql.DB, r Rule) (int, []Tx, error) {
	all, err := All(d, Filter{})
	if err != nil {
		return 0, nil, err
	}
	r.Enabled = true
	n := 0
	var sample []Tx
	for i := len(all) - 1; i >= 0; i-- {
		t := all[i]
		if r.Matches(&t) {
			n++
			if len(sample) < 20 {
				sample = append(sample, t)
			}
		}
	}
	return n, sample, nil
}

// ApplyRuleToHistory runs one rule over existing rows: sets its category
// (only on rows whose category differs) and adds its tags. Returns rows changed.
func ApplyRuleToHistory(d *sql.DB, r Rule) (int, error) {
	all, err := All(d, Filter{})
	if err != nil {
		return 0, err
	}
	r.Enabled = true
	tx, err := d.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n := 0
	for _, t := range all {
		if !r.Matches(&t) {
			continue
		}
		changed := false
		if r.SetCategory != "" && t.Category != r.SetCategory {
			var kind string
			if tx.QueryRow(`SELECT kind FROM categories WHERE id=?`, r.SetCategory).Scan(&kind) == nil && kind == t.Kind {
				t.Category = r.SetCategory
				changed = true
			}
		}
		if r.SetMerchant != "" && t.Merchant != r.SetMerchant {
			t.Merchant = r.SetMerchant
			changed = true
		}
		for _, tag := range r.AddTags {
			if !HasTag(JoinTags(t.Tags), tag) {
				t.Tags = append(t.Tags, tag)
				changed = true
			}
		}
		if changed {
			if err := Update(tx, &t); err != nil {
				return 0, err
			}
			n++
		}
	}
	return n, tx.Commit()
}
