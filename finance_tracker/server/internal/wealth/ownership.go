package wealth

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"ft/internal/ledger"
	"ft/internal/money"
)

// True cost of owning: what the car or the house really costs a year, not
// just the bills — running costs (everything tagged with it), the loan's
// interest, the change in its value (a car losing €3,000 a year costs
// €3,000 a year), and what the money tied up in it would earn invested.
// Set beside the alternative the owner names — a taxi and rentals per km, or
// a month's rent — it answers "is it still worth owning?" with a number.

type OwnSettings struct {
	Tag      string  `json:"tag"`        // spending tagged with this is the asset's
	KmYear   float64 `json:"km_year"`    // vehicles: km driven a year
	AltPerKm float64 `json:"alt_per_km"` // vehicles: € per km without one (taxi, rentals, transit)
	Rent     float64 `json:"rent"`       // property: € a month to rent the same
}

func LoadOwnSettings(d *sql.DB) map[string]OwnSettings {
	out := map[string]OwnSettings{}
	var raw string
	if d.QueryRow(`SELECT value FROM settings WHERE key='ownership'`).Scan(&raw) == nil {
		json.Unmarshal([]byte(raw), &out)
	}
	return out
}

// SaveOwnSettings stores one asset's figures.
func SaveOwnSettings(d *sql.DB, id string, s OwnSettings) error {
	var a ledger.Account
	if err := d.QueryRow(`SELECT kind FROM accounts WHERE id=?`, id).Scan(&a.Kind); err != nil {
		return errors.New("no such account")
	}
	if a.Kind != "property" && a.Kind != "vehicle" {
		return errors.New("only a property or a vehicle has an ownership cost")
	}
	if t := ledger.NormalizeTags([]string{s.Tag}); len(t) == 1 {
		s.Tag = t[0]
	} else {
		s.Tag = ""
	}
	if s.KmYear < 0 || s.KmYear > 200000 || s.AltPerKm < 0 || s.AltPerKm > 10 || s.Rent < 0 || s.Rent > 100000 {
		return errors.New("those figures are out of range")
	}
	all := LoadOwnSettings(d)
	all[id] = s
	raw, _ := json.Marshal(all)
	_, err := d.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('ownership',?)`, string(raw))
	return err
}

type OwnLine struct {
	Key      string      `json:"key"` // running | interest | value | opportunity
	Label    string      `json:"label"`
	Amount   money.Cents `json:"amount"` // a year; + costs, − earns
	Estimate bool        `json:"estimate,omitempty"`
	Link     string      `json:"link,omitempty"`
}

type OwnCost struct {
	AccountID string      `json:"account_id"`
	Name      string      `json:"name"`
	Kind      string      `json:"kind"`
	Settings  OwnSettings `json:"settings"`
	Value     money.Cents `json:"value"`
	Equity    money.Cents `json:"equity"`
	Lines     []OwnLine   `json:"lines"`
	Running   []OwnLine   `json:"running"` // running costs by category
	Year      money.Cents `json:"year"`    // total a year
	Month     money.Cents `json:"month"`
	PerKm     float64     `json:"per_km,omitempty"`
	AltYear   money.Cents `json:"alt_year,omitempty"` // the alternative, a year
	Verdict   string      `json:"verdict,omitempty"`
	Tone      string      `json:"tone,omitempty"` // good | warn
	Missing   []string    `json:"missing"`        // what to fill in for a verdict
}

// Ownership builds the cost of every property and vehicle. expectedNominal
// is the % a year the money would earn invested instead.
func Ownership(d *sql.DB, book *Book, txs []ledger.Tx, expectedNominal float64, now time.Time) ([]OwnCost, error) {
	cats, err := ledger.CategoryMap(d)
	if err != nil {
		return nil, err
	}
	settings := LoadOwnSettings(d)
	loans := Loans(book, txs, now)
	today := now.Format("2006-01-02")
	yearAgo := now.AddDate(-1, 0, 0).Format("2006-01-02")
	tags := map[string]bool{}
	for _, t := range txs {
		for _, g := range t.Tags {
			tags[g] = true
		}
	}
	out := []OwnCost{}
	var ids []string
	for id, a := range book.Accounts {
		if (a.Kind == "property" || a.Kind == "vehicle") && !a.Archived {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		a := book.Accounts[id]
		s := settings[id]
		if s.Tag == "" {
			s.Tag = guessTag(a, tags)
		}
		c := OwnCost{AccountID: id, Name: a.Name, Kind: a.Kind, Settings: s, Lines: []OwnLine{}, Running: []OwnLine{}, Missing: []string{}}
		p, _ := book.At(id, today)
		c.Value, c.Equity = p.Value, p.Value

		// Interest on loans secured on it; that interest is a line of its own,
		// so a tagged interest payment isn't counted twice.
		var interest money.Cents
		for _, l := range loans {
			if l.Details.AssetID == id {
				interest += l.PaidInterest12
				c.Equity -= l.Balance
			}
		}
		// Running costs: everything tagged with it over the last 12 months.
		by := map[string]money.Cents{}
		if s.Tag != "" {
			for _, t := range txs {
				if t.Date <= yearAgo || t.Date > today || !hasTag(t.Tags, s.Tag) {
					continue
				}
				if interest > 0 && t.Category == "housing.mortgage_interest" {
					continue
				}
				switch t.Kind {
				case "expense":
					by[ledger.Top(t.Category)] += t.Amount
				case "income": // refunds, insurance payouts
					by[ledger.Top(t.Category)] -= t.Amount
				}
			}
		} else {
			c.Missing = append(c.Missing, "tag")
		}
		var running money.Cents
		for k, v := range by {
			name := k
			if cat, ok := cats[k]; ok {
				name = cat.Name
			}
			c.Running = append(c.Running, OwnLine{Key: k, Label: name, Amount: v, Link: ledger.Link("from", yearAgo, "to", today, "tag", s.Tag, "category", k)})
			running += v
		}
		sort.Slice(c.Running, func(i, j int) bool { return c.Running[i].Amount > c.Running[j].Amount })
		c.Lines = append(c.Lines, OwnLine{Key: "running", Label: "Running costs", Amount: running, Link: ledger.Link("from", yearAgo, "to", today, "tag", s.Tag)})
		if interest > 0 {
			c.Lines = append(c.Lines, OwnLine{Key: "interest", Label: "Loan interest", Amount: interest, Link: "/wealth/loans"})
		}
		// Value change over the year: a loss is a cost, a gain earns.
		if then, ok := book.At(id, yearAgo); ok && p.Date > then.Date {
			c.Lines = append(c.Lines, OwnLine{Key: "value", Label: valueLabel(p.Value - then.Value), Amount: then.Value - p.Value})
		} else {
			c.Missing = append(c.Missing, "value")
		}
		// The owner's own money in it would earn if it were invested.
		if c.Equity > 0 && expectedNominal > 0 {
			c.Lines = append(c.Lines, OwnLine{Key: "opportunity", Label: fmt.Sprintf("What %s of your money in it would earn invested (%.1f%%)", eurO(c.Equity), expectedNominal),
				Amount: money.FromFloat(c.Equity.Float() * expectedNominal / 100), Estimate: true})
		}
		for _, l := range c.Lines {
			c.Year += l.Amount
		}
		c.Month = c.Year / 12

		switch a.Kind {
		case "vehicle":
			if s.KmYear > 0 {
				c.PerKm = math.Round(c.Year.Float()/s.KmYear*100) / 100
			} else {
				c.Missing = append(c.Missing, "km_year")
			}
			if s.KmYear > 0 && s.AltPerKm > 0 {
				c.AltYear = money.FromFloat(s.KmYear * s.AltPerKm)
				c.verdict(fmt.Sprintf("€%.2f a km to own, against €%.2f without it.", c.PerKm, s.AltPerKm))
			} else if s.AltPerKm == 0 {
				c.Missing = append(c.Missing, "alt_per_km")
			}
		case "property":
			if s.Rent > 0 {
				c.AltYear = money.FromFloat(s.Rent * 12)
				c.verdict(fmt.Sprintf("%s a month to own, against %s to rent the same.", eurO(c.Month), eurO(money.FromFloat(s.Rent))))
			} else {
				c.Missing = append(c.Missing, "rent")
			}
		}
		out = append(out, c)
	}
	return out, nil
}

func (c *OwnCost) verdict(lead string) {
	diff := c.Year - c.AltYear
	switch {
	case diff > c.AltYear/10:
		c.Tone = "warn"
		c.Verdict = fmt.Sprintf("%s Owning costs %s a year more — worth asking whether it still earns its keep.", lead, eurO(diff))
	case diff < -c.AltYear/10:
		c.Tone = "good"
		c.Verdict = fmt.Sprintf("%s Owning saves %s a year.", lead, eurO(-diff))
	default:
		c.Tone = "good"
		c.Verdict = lead + " About the same either way — it comes down to convenience."
	}
}

func valueLabel(change money.Cents) string {
	if change < 0 {
		return "Lost value"
	}
	return "Gained value"
}

func guessTag(a ledger.Account, tags map[string]bool) string {
	for _, c := range []string{strings.ToLower(a.ID), strings.ToLower(a.Name)} {
		if tags[c] {
			return c
		}
	}
	if a.Kind == "vehicle" && tags["car"] {
		return "car"
	}
	return ""
}

func hasTag(tags []string, t string) bool {
	for _, x := range tags {
		if x == t {
			return true
		}
	}
	return false
}

func eurO(c money.Cents) string {
	v := math.Round(c.Float())
	s := fmt.Sprintf("%.0f", math.Abs(v))
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if v < 0 {
		return "−€" + s
	}
	return "€" + s
}
