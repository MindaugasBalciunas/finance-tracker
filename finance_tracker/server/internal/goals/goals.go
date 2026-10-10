// Package goals is the wish list: things saved up for in priority order,
// funded from what is left at month end — after paying yourself first
// (investing, pensions, mortgage principal) and every obligation and expense.
// Goals are earmarks on savings; nothing here moves money between accounts.
package goals

import (
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"ft/internal/db"
	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
)

type Goal struct {
	ID         int64       `json:"id"`
	Name       string      `json:"name"`
	Target     money.Cents `json:"target"`
	Priority   int         `json:"priority"`
	TargetDate string      `json:"target_date"`
	Note       string      `json:"note"`
	URL        string      `json:"url"`
	// Tag trip:<name> makes the goal a planned trip: spending tagged with it
	// (flights booked early, then the trip) is Spent against what was saved.
	Tag       string      `json:"tag"`
	Spent     money.Cents `json:"spent,omitempty"`
	Status    string      `json:"status"` // active | bought (done; travelled for a trip) | dropped
	DoneOn    string      `json:"done_on"`
	Saved     money.Cents `json:"saved"`
	Remaining money.Cents `json:"remaining"`
	// ETA: the month the goal is fully funded at the average left over,
	// filling goals in priority order ("" = no surplus to go on).
	ETA string `json:"eta,omitempty"`
	// PerMonth needed to reach the target by TargetDate.
	PerMonth money.Cents `json:"per_month,omitempty"`
}

type Move struct {
	ID        int64       `json:"id"`
	GoalID    int64       `json:"goal_id"`
	Month     string      `json:"month"`
	Amount    money.Cents `json:"amount"`
	Kind      string      `json:"kind"`
	Note      string      `json:"note"`
	CreatedAt string      `json:"created_at"`
}

// List returns every goal: active ones by priority, then bought/dropped.
func List(d *sql.DB) ([]Goal, error) {
	rows, err := d.Query(`SELECT g.id,g.name,g.target,g.priority,g.target_date,g.note,g.url,g.tag,g.status,g.done_on,
		COALESCE((SELECT SUM(amount) FROM goal_moves m WHERE m.goal_id=g.id),0)
		FROM goals g ORDER BY g.status='active' DESC, g.priority, g.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Goal{}
	for rows.Next() {
		var g Goal
		if err := rows.Scan(&g.ID, &g.Name, &g.Target, &g.Priority, &g.TargetDate, &g.Note, &g.URL, &g.Tag, &g.Status, &g.DoneOn, &g.Saved); err != nil {
			return nil, err
		}
		g.Remaining = max(0, g.Target-g.Saved)
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Trips: what has been spent under the tag so far (expenses − refunds).
	for i := range out {
		if out[i].Tag == "" {
			continue
		}
		txs, err := ledger.All(d, ledger.Filter{Tags: []string{out[i].Tag}})
		if err != nil {
			return nil, err
		}
		for _, t := range txs {
			switch t.Kind {
			case "expense":
				out[i].Spent += t.Amount
			case "income":
				out[i].Spent -= t.Amount
			}
		}
	}
	return out, nil
}

func Get(d *sql.DB, id int64) (Goal, error) {
	all, err := List(d)
	if err != nil {
		return Goal{}, err
	}
	for _, g := range all {
		if g.ID == id {
			return g, nil
		}
	}
	return Goal{}, errors.New("no goal with that id")
}

// Save creates (ID 0, appended last in priority) or updates a goal.
func Save(d *sql.DB, g *Goal) error {
	g.Name = strings.TrimSpace(g.Name)
	if g.Name == "" || len(g.Name) > 120 || len(g.Note) > 1000 || len(g.URL) > 500 {
		return errors.New("name the goal (and keep the note and link short)")
	}
	if g.Target <= 0 {
		return errors.New("what does it cost? the target must be above €0")
	}
	if g.TargetDate != "" {
		if _, err := time.Parse("2006-01-02", g.TargetDate); err != nil {
			return errors.New("target date is YYYY-MM-DD")
		}
	}
	if g.URL != "" && !strings.HasPrefix(g.URL, "https://") && !strings.HasPrefix(g.URL, "http://") {
		return errors.New("the link must start with https://")
	}
	if g.Tag != "" {
		raw := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(g.Tag)), "trip:"))
		name := ""
		if raw != "" {
			name = strings.ReplaceAll(strings.Trim(ledger.SlugID(raw), "_"), "_", "-")
		}
		if name == "" {
			return errors.New("name the trip")
		}
		g.Tag = "trip:" + name
	}
	if g.Status == "" {
		g.Status = "active"
	}
	switch g.Status {
	case "active":
		g.DoneOn = ""
	case "bought", "dropped":
		if g.DoneOn == "" {
			g.DoneOn = time.Now().Format("2006-01-02")
		}
	default:
		return errors.New("status is active, bought or dropped")
	}
	now := db.Now()
	if g.ID == 0 {
		d.QueryRow(`SELECT COALESCE(MAX(priority),0)+1 FROM goals`).Scan(&g.Priority)
		res, err := d.Exec(`INSERT INTO goals(name,target,priority,target_date,note,url,tag,status,done_on,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			g.Name, g.Target, g.Priority, g.TargetDate, g.Note, g.URL, g.Tag, g.Status, g.DoneOn, now, now)
		if err != nil {
			return err
		}
		g.ID, _ = res.LastInsertId()
		return nil
	}
	res, err := d.Exec(`UPDATE goals SET name=?,target=?,target_date=?,note=?,url=?,tag=?,status=?,done_on=?,updated_at=? WHERE id=?`,
		g.Name, g.Target, g.TargetDate, g.Note, g.URL, g.Tag, g.Status, g.DoneOn, now, g.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("no goal with that id")
	}
	return nil
}

func Delete(d *sql.DB, id int64) error {
	_, err := d.Exec(`DELETE FROM goals WHERE id=?`, id)
	return err
}

// Reorder sets the priority: ids first to last; goals not named keep their
// relative order after them.
func Reorder(d *sql.DB, ids []int64) error {
	all, err := List(d)
	if err != nil {
		return err
	}
	seen := map[int64]bool{}
	order := []int64{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			order = append(order, id)
		}
	}
	for _, g := range all {
		if !seen[g.ID] {
			order = append(order, g.ID)
		}
	}
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range order {
		if _, err := tx.Exec(`UPDATE goals SET priority=? WHERE id=?`, i+1, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AddMove puts money into a goal (or takes it out with a negative amount).
// A goal never holds less than €0.
func AddMove(d *sql.DB, m *Move) error {
	if _, err := time.Parse("2006-01", m.Month); err != nil {
		return errors.New("month is YYYY-MM")
	}
	if m.Amount == 0 {
		return errors.New("amount is required")
	}
	if m.Kind == "" {
		m.Kind = "manual"
	}
	g, err := Get(d, m.GoalID)
	if err != nil {
		return err
	}
	if g.Saved+m.Amount < 0 {
		return errors.New("a goal can't hold less than €0")
	}
	m.CreatedAt = db.Now()
	res, err := d.Exec(`INSERT INTO goal_moves(goal_id,month,amount,kind,note,created_at) VALUES(?,?,?,?,?,?)`, m.GoalID, m.Month, m.Amount, m.Kind, strings.TrimSpace(m.Note), m.CreatedAt)
	if err != nil {
		return err
	}
	m.ID, _ = res.LastInsertId()
	return nil
}

func Moves(d *sql.DB, goalID int64) ([]Move, error) {
	rows, err := d.Query(`SELECT id,goal_id,month,amount,kind,note,created_at FROM goal_moves WHERE goal_id=? ORDER BY month DESC, id DESC`, goalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Move{}
	for rows.Next() {
		var m Move
		rows.Scan(&m.ID, &m.GoalID, &m.Month, &m.Amount, &m.Kind, &m.Note, &m.CreatedAt)
		out = append(out, m)
	}
	return out, rows.Err()
}

// Month is one month's money after the strategy: income, then paying
// yourself first, then spending (obligations included) — the rest is what
// goals can be funded from.
type Month struct {
	Month     string      `json:"month"`
	Income    money.Cents `json:"income"`
	Invested  money.Cents `json:"invested"` // paid yourself first: investing, pensions, mortgage principal
	Spending  money.Cents `json:"spending"` // obligations and everyday spending
	LeftOver  money.Cents `json:"left_over"`
	Funded    money.Cents `json:"funded"`    // already put into goals for this month
	Available money.Cents `json:"available"` // left over not yet in goals
	Complete  bool        `json:"complete"`  // the month has ended
}

type Allocation struct {
	GoalID int64       `json:"goal_id"`
	Name   string      `json:"name"`
	Amount money.Cents `json:"amount"`
}

type Plan struct {
	Goals []Goal `json:"goals"`
	// Funding is the month being funded (last complete month by default),
	// with the suggested split by priority.
	Funding  Month        `json:"funding"`
	Proposal []Allocation `json:"proposal"`
	// History: the last six complete months; AvgLeftOver drives the ETAs.
	History     []Month     `json:"history"`
	AvgLeftOver money.Cents `json:"avg_left_over"`
	TotalSaved  money.Cents `json:"total_saved"`
	TotalNeeded money.Cents `json:"total_needed"`
}

// Build computes the wish list for funding month ym ("" = last complete month).
func Build(d *sql.DB, ym string, now time.Time) (*Plan, error) {
	if ym == "" {
		ym = time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, time.Local).Format("2006-01")
	}
	if _, err := time.Parse("2006-01", ym); err != nil {
		return nil, errors.New("month is YYYY-MM")
	}
	gs, err := List(d)
	if err != nil {
		return nil, err
	}
	from := time.Date(now.Year(), now.Month()-7, 1, 0, 0, 0, 0, time.Local).Format("2006-01-02")
	if f := ym + "-01"; f < from {
		from = f
	}
	txs, err := ledger.All(d, ledger.Filter{From: from})
	if err != nil {
		return nil, err
	}
	cats, _ := ledger.CategoryMap(d)
	flows := map[string]insights.Flow{}
	for _, f := range insights.CashFlow(txs, cats, "month", plan.LoadSettings(d).Salary()) {
		flows[f.Period] = f
	}
	funded := map[string]money.Cents{}
	rows, err := d.Query(`SELECT month, SUM(amount) FROM goal_moves WHERE kind='funding' GROUP BY month`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var m string
		var v money.Cents
		rows.Scan(&m, &v)
		funded[m] = v
	}
	rows.Close()
	cur := now.Format("2006-01")
	month := func(m string) Month {
		f := flows[m]
		x := Month{Month: m, Income: f.Income, Invested: f.Invested, Spending: f.Spending, Funded: funded[m], Complete: m < cur}
		x.LeftOver = f.Income - f.Spending - f.Invested
		x.Available = max(0, x.LeftOver-x.Funded)
		return x
	}
	p := &Plan{Goals: gs, Funding: month(ym), Proposal: []Allocation{}, History: []Month{}}
	var sum money.Cents
	for i := 6; i >= 1; i-- {
		m := month(time.Date(now.Year(), now.Month()-time.Month(i), 1, 0, 0, 0, 0, time.Local).Format("2006-01"))
		p.History = append(p.History, m)
		sum += max(0, m.LeftOver)
	}
	p.AvgLeftOver = sum / 6

	// Suggested split: fill goals in priority order, each up to what it
	// still needs.
	left := p.Funding.Available
	for _, g := range gs {
		if g.Status != "active" || left <= 0 {
			continue
		}
		if a := min(left, g.Remaining); a > 0 {
			p.Proposal = append(p.Proposal, Allocation{GoalID: g.ID, Name: g.Name, Amount: a})
			left -= a
		}
	}
	// ETAs at the average left over, in priority order; and what a target
	// date needs per month.
	var ahead money.Cents
	for i := range p.Goals {
		g := &p.Goals[i]
		if g.Status != "active" {
			continue
		}
		p.TotalSaved += g.Saved
		p.TotalNeeded += g.Remaining
		ahead += g.Remaining
		switch {
		case g.Remaining == 0:
			g.ETA = cur
		case p.AvgLeftOver > 0:
			months := int((ahead + p.AvgLeftOver - 1) / p.AvgLeftOver)
			g.ETA = time.Date(now.Year(), now.Month()+time.Month(months), 1, 0, 0, 0, 0, time.Local).Format("2006-01")
		}
		if g.TargetDate != "" && g.Remaining > 0 {
			if t, err := time.Parse("2006-01-02", g.TargetDate); err == nil {
				n := (t.Year()-now.Year())*12 + int(t.Month()-now.Month())
				if n < 1 {
					n = 1
				}
				g.PerMonth = (g.Remaining + money.Cents(n) - 1) / money.Cents(n)
			}
		}
	}
	return p, nil
}

// Fund records a month's allocations as funding moves, within what that
// month left over and what each goal still needs.
func Fund(d *sql.DB, ym string, allocs []Allocation, now time.Time) (*Plan, error) {
	p, err := Build(d, ym, now)
	if err != nil {
		return nil, err
	}
	if !p.Funding.Complete {
		return nil, errors.New("fund a month once it has ended — its left over isn't known yet")
	}
	need := map[int64]money.Cents{}
	for _, g := range p.Goals {
		if g.Status == "active" {
			need[g.ID] = g.Remaining
		}
	}
	var total money.Cents
	for _, a := range allocs {
		if a.Amount < 0 {
			return nil, errors.New("funding amounts are positive — take money out on the goal itself")
		}
		r, ok := need[a.GoalID]
		if !ok {
			return nil, errors.New("only active goals can be funded")
		}
		if a.Amount > r {
			return nil, errors.New("that is more than the goal still needs")
		}
		need[a.GoalID] -= a.Amount
		total += a.Amount
	}
	if total > p.Funding.Available {
		return nil, errors.New("that is more than the month left over (€" + p.Funding.Available.String() + ")")
	}
	sort.SliceStable(allocs, func(i, j int) bool { return allocs[i].GoalID < allocs[j].GoalID })
	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, a := range allocs {
		if a.Amount == 0 {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO goal_moves(goal_id,month,amount,kind,note,created_at) VALUES(?,?,?,'funding','',?)`, a.GoalID, ym, a.Amount, db.Now()); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return Build(d, ym, now)
}
