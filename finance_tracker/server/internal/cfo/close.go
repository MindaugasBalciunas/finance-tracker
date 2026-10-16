package cfo

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"ft/internal/db"
	"ft/internal/goals"
	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/wealth"
)

// MonthClose is the routine that finishes a month: the data first (inbox,
// categories, balances against the banks), then the decisions (read the
// review, rebalance the plan, fund the wish list). Data steps tick
// themselves from the data; the owner ticks the decisions.
type MonthClose struct {
	Month    string      `json:"month"`
	Steps    []CloseStep `json:"steps"`
	Done     int         `json:"done"`
	Closed   bool        `json:"closed"`
	ClosedAt string      `json:"closed_at,omitempty"`
}

type CloseStep struct {
	Key    string `json:"key"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Link   string `json:"link"`
	Done   bool   `json:"done"`
	Manual bool   `json:"manual"` // ticked by the owner, not by the data
}

// closeState is what the owner ticked, per month, under the "month_close"
// settings key (backed up with the rest of the settings).
type closeState map[string]struct {
	Ticked   map[string]bool `json:"ticked,omitempty"`
	ClosedAt string          `json:"closed_at,omitempty"`
}

var manualSteps = map[string]bool{"review": true, "plan": true}

func loadClose(d *sql.DB) closeState {
	st := closeState{}
	var raw string
	if d.QueryRow(`SELECT value FROM settings WHERE key='month_close'`).Scan(&raw) == nil {
		json.Unmarshal([]byte(raw), &st)
	}
	return st
}

func saveClose(d *sql.DB, st closeState) error {
	raw, _ := json.Marshal(st)
	_, err := d.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('month_close',?)`, string(raw))
	return err
}

// TickClose records a decision step as done (or undone).
func TickClose(d *sql.DB, month, step string, done bool) error {
	if _, err := time.Parse("2006-01", month); err != nil {
		return errors.New("month is YYYY-MM")
	}
	if !manualSteps[step] {
		return fmt.Errorf("%q ticks itself from the data", step)
	}
	st := loadClose(d)
	m := st[month]
	if m.Ticked == nil {
		m.Ticked = map[string]bool{}
	}
	m.Ticked[step] = done
	st[month] = m
	return saveClose(d, st)
}

// FinishClose marks the month closed (open steps may remain — it's the
// owner's call).
func FinishClose(d *sql.DB, month string) error {
	if _, err := time.Parse("2006-01", month); err != nil {
		return errors.New("month is YYYY-MM")
	}
	st := loadClose(d)
	m := st[month]
	m.ClosedAt = db.Now()
	st[month] = m
	return saveClose(d, st)
}

// Close builds the checklist for a month (YYYY-MM).
func Close(d *sql.DB, month string, now time.Time) (*MonthClose, error) {
	start, err := time.Parse("2006-01", month)
	if err != nil {
		return nil, errors.New("month is YYYY-MM")
	}
	from := start.Format("2006-01-02")
	to := start.AddDate(0, 1, -1).Format("2006-01-02")
	st := loadClose(d)[month]
	c := &MonthClose{Month: month, ClosedAt: st.ClosedAt, Closed: st.ClosedAt != ""}
	name := start.Format("January")

	// 1. Bank rows from the month waiting for review.
	var waiting int
	d.QueryRow(`SELECT COUNT(*) FROM bank_inbox WHERE state='open' AND pending=0 AND date <= ?`, to).Scan(&waiting)
	c.add(CloseStep{Key: "inbox", Title: "Clear the bank inbox", Link: "/ledger/inbox", Done: waiting == 0,
		Detail: pick(waiting == 0, "Every bank row is in the ledger.", fmt.Sprintf("%d bank %s waiting.", waiting, plural(waiting, "row", "rows")))})

	// 2. Expenses left with only a broad category.
	var vague int
	d.QueryRow(`SELECT COUNT(*) FROM transactions WHERE kind='expense' AND instr(category,'.') = 0 AND category NOT IN ('gifts','dating','other_income') AND date BETWEEN ? AND ?`, from, to).Scan(&vague)
	c.add(CloseStep{Key: "categories", Title: "Give every expense a precise category", Link: "/ledger/tidy", Done: vague == 0,
		Detail: pick(vague == 0, "Every expense has a precise category.", fmt.Sprintf("%d %s only a broad category.", vague, plural(vague, "expense has", "expenses have")))})

	// 3. Bank-synced accounts whose balances the month's transactions explain.
	var off []string
	if book, err := wealth.LoadBook(d); err == nil {
		// Reconcile runs from the month's start to each account's latest
		// balance, so it needs every transaction since then.
		txs, _ := ledger.All(d, ledger.Filter{From: from})
		for _, a := range insights.Reconcile(book, txs, from) {
			if a.Source == "bank" && !a.Reconciles {
				off = append(off, fmt.Sprintf("%s (%s)", a.Name, signedN(a.Unexplained)))
			}
		}
	}
	c.add(CloseStep{Key: "balances", Title: "Match balances to the banks", Link: "/ai?q=" + url.QueryEscape("Reconcile my bank accounts since "+start.Format("1 January")+" with get_reconciliation: explain each gap and what to record."), Done: len(off) == 0,
		Detail: pick(len(off) == 0, "Every bank account's balance is explained by its transactions.", "Unexplained: "+strings.Join(off, ", ")+". Ask CFO to reconcile.")})

	// 4–5. The decisions.
	c.add(CloseStep{Key: "review", Title: "Read " + name + "'s review", Link: "/insights/review?month=" + month, Manual: true, Done: st.Ticked["review"],
		Detail: "How the month went against the plan and the last six months."})
	c.add(CloseStep{Key: "plan", Title: "Rebalance the plan for next month", Link: "/plan", Manual: true, Done: st.Ticked["plan"],
		Detail: "Lines that keep going over, funds that run short, the saving target."})

	// 6. What was left over goes to the wish list.
	if p, err := goals.Build(d, month, now); err == nil {
		f := p.Funding
		done := f.Available <= 0 || len(p.Goals) == 0
		detail := "Nothing was left over to put into goals."
		switch {
		case f.Funded > 0 && f.Available <= 0:
			detail = fmt.Sprintf("%s went into goals.", eurS(f.Funded))
		case f.Available > 0:
			detail = fmt.Sprintf("%s left over to put into goals.", eurS(f.Available))
		}
		c.add(CloseStep{Key: "goals", Title: "Fund the wish list", Link: "/plan/wishlist", Done: done, Detail: detail})
	}
	return c, nil
}

func (c *MonthClose) add(s CloseStep) {
	c.Steps = append(c.Steps, s)
	if s.Done {
		c.Done++
	}
}

func pick(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}
