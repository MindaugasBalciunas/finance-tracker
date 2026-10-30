// Package decisions is the decision journal: a big purchase written down
// before it is made, held for a cooling-off period, decided, and — a couple
// of months after buying — rated on whether it was worth it. Over time that
// shows which kinds of spending the household actually values.
package decisions

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"ft/internal/db"
	"ft/internal/money"
)

type Decision struct {
	ID        int64       `json:"id"`
	Title     string      `json:"title"`
	Amount    money.Cents `json:"amount"`
	Category  string      `json:"category"`
	Status    string      `json:"status"` // thinking | saving (on the wish list) | bought | skipped
	CoolUntil string      `json:"cool_until"`
	DecidedOn string      `json:"decided_on"`
	ReviewOn  string      `json:"review_on"`
	Worth     string      `json:"worth"` // '' | yes | meh | no
	Note      string      `json:"note"`
	GoalID    int64       `json:"goal_id,omitempty"` // the wish-list goal saving for it
	CreatedAt string      `json:"created_at"`
}

// CoolingOff is how long to sleep on a purchase of this size: nothing under
// €100, a weekend under €500, a week under €2,000, a month above.
func CoolingOff(amount money.Cents) int {
	switch e := amount.Float(); {
	case e < 100:
		return 0
	case e < 500:
		return 2
	case e < 2000:
		return 7
	}
	return 30
}

// ReviewAfter: long enough for the novelty to wear off.
const ReviewAfter = 60

func day(t time.Time) string { return t.Format("2006-01-02") }

func List(q *sql.DB) ([]Decision, error) {
	rows, err := q.Query(`SELECT id,title,amount,category,status,cool_until,decided_on,review_on,worth,note,goal_id,created_at FROM decisions
		ORDER BY status='thinking' DESC, created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Decision{}
	for rows.Next() {
		var x Decision
		if err := rows.Scan(&x.ID, &x.Title, &x.Amount, &x.Category, &x.Status, &x.CoolUntil, &x.DecidedOn, &x.ReviewOn, &x.Worth, &x.Note, &x.GoalID, &x.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func Get(q *sql.DB, id int64) (Decision, error) {
	var x Decision
	err := q.QueryRow(`SELECT id,title,amount,category,status,cool_until,decided_on,review_on,worth,note,goal_id,created_at FROM decisions WHERE id=?`, id).
		Scan(&x.ID, &x.Title, &x.Amount, &x.Category, &x.Status, &x.CoolUntil, &x.DecidedOn, &x.ReviewOn, &x.Worth, &x.Note, &x.GoalID, &x.CreatedAt)
	return x, err
}

func validate(x *Decision) error {
	x.Title = strings.TrimSpace(x.Title)
	if x.Title == "" || len(x.Title) > 120 || len(x.Note) > 1000 || len(x.Category) > 60 {
		return errors.New("say what you're thinking of buying (and keep the note short)")
	}
	if x.Amount <= 0 {
		return errors.New("the price must be more than zero")
	}
	switch x.Status {
	case "thinking", "bought", "skipped":
	case "saving":
		if x.GoalID == 0 {
			return errors.New("saving for it needs a wish-list goal — use Save for it")
		}
	default:
		return errors.New("a decision is thinking, saving, bought or skipped")
	}
	switch x.Worth {
	case "", "yes", "meh", "no":
	default:
		return errors.New("worth it is yes, meh or no")
	}
	if x.Worth != "" && x.Status != "bought" {
		return errors.New("only something bought can be rated")
	}
	for _, d := range []string{x.CoolUntil, x.DecidedOn, x.ReviewOn} {
		if d != "" {
			if _, err := time.Parse("2006-01-02", d); err != nil {
				return errors.New("dates are YYYY-MM-DD")
			}
		}
	}
	return nil
}

// Create writes a new decision down, thinking, with its cooling-off date
// (the owner's own date if given).
func Create(d *sql.DB, x *Decision, now time.Time) error {
	x.ID, x.Status, x.DecidedOn, x.ReviewOn, x.Worth, x.GoalID = 0, "thinking", "", "", "", 0
	if x.CoolUntil == "" {
		x.CoolUntil = day(now.AddDate(0, 0, CoolingOff(x.Amount)))
	}
	if err := validate(x); err != nil {
		return err
	}
	x.CreatedAt = db.Now()
	r, err := d.Exec(`INSERT INTO decisions(title,amount,category,status,cool_until,decided_on,review_on,worth,note,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		x.Title, x.Amount, x.Category, x.Status, x.CoolUntil, x.DecidedOn, x.ReviewOn, x.Worth, x.Note, x.CreatedAt)
	if err != nil {
		return err
	}
	x.ID, _ = r.LastInsertId()
	return nil
}

// Update saves an edit. Deciding stamps the day; buying sets the review
// date; going back to thinking clears both.
func Update(d *sql.DB, x *Decision, now time.Time) error {
	cur, err := Get(d, x.ID)
	if err != nil {
		return err
	}
	x.CreatedAt, x.GoalID = cur.CreatedAt, cur.GoalID // the link is made by SaveFor and the goal, never by an edit
	if cur.Status == "saving" && x.Status != cur.Status {
		return errors.New("this one is on the wish list: buy or drop the goal there")
	}
	if x.Status == "saving" && cur.Status != "saving" {
		return errors.New("Save for it puts a decision on the wish list")
	}
	if x.Status != cur.Status {
		switch x.Status {
		case "thinking":
			x.DecidedOn, x.ReviewOn, x.Worth = "", "", ""
		case "bought":
			x.DecidedOn = day(now)
			x.ReviewOn = day(now.AddDate(0, 0, ReviewAfter))
		case "skipped":
			x.DecidedOn, x.ReviewOn, x.Worth = day(now), "", ""
		}
	} else {
		x.DecidedOn = cur.DecidedOn
		if x.ReviewOn == "" {
			x.ReviewOn = cur.ReviewOn
		}
	}
	if x.CoolUntil == "" {
		x.CoolUntil = cur.CoolUntil
	}
	if err := validate(x); err != nil {
		return err
	}
	_, err = d.Exec(`UPDATE decisions SET title=?,amount=?,category=?,status=?,cool_until=?,decided_on=?,review_on=?,worth=?,note=? WHERE id=?`,
		x.Title, x.Amount, x.Category, x.Status, x.CoolUntil, x.DecidedOn, x.ReviewOn, x.Worth, x.Note, x.ID)
	return err
}

func Delete(d *sql.DB, id int64) error {
	_, err := d.Exec(`DELETE FROM decisions WHERE id=?`, id)
	return err
}

// execer is what Linked/GoalChanged need, so goals can call them on its own
// connection.
type execer interface {
	Exec(string, ...any) (sql.Result, error)
}

// Linked marks a decision as being saved for by wish-list goal goalID.
// It must be thinking: a decided purchase isn't saved for.
func Linked(e execer, id, goalID int64, now time.Time) error {
	r, err := e.Exec(`UPDATE decisions SET status='saving', goal_id=?, decided_on=? WHERE id=? AND status='thinking'`, goalID, day(now), id)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return errors.New("only a decision you're still thinking about can go on the wish list")
	}
	return nil
}

// GoalChanged follows a goal's status into the decision saving for it:
// bought → bought (and the worth-it question later), dropped → let go,
// active again → saving, deleted ("") → back to thinking.
func GoalChanged(e execer, goalID int64, status, doneOn string) error {
	if goalID == 0 {
		return nil
	}
	var err error
	switch status {
	case "bought":
		review := ""
		if t, perr := time.Parse("2006-01-02", doneOn); perr == nil {
			review = day(t.AddDate(0, 0, ReviewAfter))
		}
		_, err = e.Exec(`UPDATE decisions SET status='bought', decided_on=?, review_on=? WHERE goal_id=? AND status<>'bought'`, doneOn, review, goalID)
	case "dropped":
		_, err = e.Exec(`UPDATE decisions SET status='skipped', decided_on=?, review_on='', worth='' WHERE goal_id=?`, doneOn, goalID)
	case "active":
		_, err = e.Exec(`UPDATE decisions SET status='saving', review_on='', worth='' WHERE goal_id=? AND status<>'saving'`, goalID)
	case "":
		_, err = e.Exec(`UPDATE decisions SET status='thinking', goal_id=0, decided_on='', review_on='', worth='' WHERE goal_id=?`, goalID)
	}
	return err
}

// Record is what the journal has taught: money not spent by sleeping on it,
// and how the purchases made turned out.
type Record struct {
	Saving       int         `json:"saving"` // on the wish list
	Skipped      int         `json:"skipped"`
	SkippedSum   money.Cents `json:"skipped_sum"`
	Bought       int         `json:"bought"`
	Yes          int         `json:"yes"`
	Meh          int         `json:"meh"`
	No           int         `json:"no"`
	RegrettedSum money.Cents `json:"regretted_sum"`
	// By category: how many rated purchases were worth it, so "gadgets: 1 of
	// 4" stands next to "travel: 3 of 3".
	ByCategory []CategoryRecord `json:"by_category"`
}

type CategoryRecord struct {
	Category string `json:"category"`
	Rated    int    `json:"rated"`
	Worth    int    `json:"worth"`
}

func Summarise(all []Decision) Record {
	r := Record{ByCategory: []CategoryRecord{}}
	byCat := map[string]*CategoryRecord{}
	for _, x := range all {
		switch x.Status {
		case "saving":
			r.Saving++
		case "skipped":
			r.Skipped++
			r.SkippedSum += x.Amount
		case "bought":
			r.Bought++
			if x.Worth == "" {
				continue
			}
			c := byCat[x.Category]
			if c == nil {
				c = &CategoryRecord{Category: x.Category}
				byCat[x.Category] = c
			}
			c.Rated++
			switch x.Worth {
			case "yes":
				r.Yes++
				c.Worth++
			case "meh":
				r.Meh++
			case "no":
				r.No++
				r.RegrettedSum += x.Amount
			}
		}
	}
	for _, c := range byCat {
		r.ByCategory = append(r.ByCategory, *c)
	}
	sortCats(r.ByCategory)
	return r
}

func sortCats(cs []CategoryRecord) {
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0 && (cs[j].Rated > cs[j-1].Rated || (cs[j].Rated == cs[j-1].Rated && cs[j].Category < cs[j-1].Category)); j-- {
			cs[j], cs[j-1] = cs[j-1], cs[j]
		}
	}
}
