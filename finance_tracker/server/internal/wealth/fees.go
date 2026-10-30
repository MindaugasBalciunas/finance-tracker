package wealth

import (
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"

	"ft/internal/money"
)

// The fee analyser: what each fund costs a year (its ongoing charge plus any
// platform fee), and what that drags off over 20 years against a low-cost
// index fund. Stocks held directly have no ongoing charge. Funds held as
// whole accounts (bank funds, pension funds) need the owner's figure from the
// fund's factsheet; well-known ETFs start from a typical figure, labelled so.

type FeeSetting struct {
	TER      float64 `json:"ter"`      // % a year, ongoing charge
	Platform float64 `json:"platform"` // % a year, custody / platform
}

// typicalTER: ongoing charges of common UCITS ETFs, % a year — a starting
// point the page labels "typical"; the owner's own figure replaces it.
var typicalTER = map[string]float64{
	"VWCE": 0.22, "VWRL": 0.22, "VWRA": 0.22, "IWDA": 0.20, "EUNL": 0.20, "SWDA": 0.20, "SPYL": 0.03, "SPYI": 0.17,
	"CSPX": 0.07, "SXR8": 0.07, "VUAA": 0.07, "VUSA": 0.07, "IUSQ": 0.20, "SSAC": 0.20, "EIMI": 0.18, "VFEM": 0.22,
	"VAGF": 0.10, "AGGH": 0.10, "XDWD": 0.19, "ISAC": 0.20,
}

// LowCost is the yardstick: a broad index fund's typical yearly charge, %.
const LowCost = 0.20

func LoadFeeSettings(d *sql.DB) map[string]FeeSetting {
	out := map[string]FeeSetting{}
	var raw string
	if d.QueryRow(`SELECT value FROM settings WHERE key='fees'`).Scan(&raw) == nil {
		json.Unmarshal([]byte(raw), &out)
	}
	return out
}

// SaveFeeSetting stores one holding's figures: id is a ticker or an account id.
func SaveFeeSetting(d *sql.DB, id string, s FeeSetting) error {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 64 {
		return errors.New("which holding?")
	}
	if s.TER < 0 || s.TER > 5 || s.Platform < 0 || s.Platform > 5 {
		return errors.New("a yearly fee between 0% and 5%")
	}
	all := LoadFeeSettings(d)
	all[id] = s
	raw, _ := json.Marshal(all)
	_, err := d.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('fees',?)`, string(raw))
	return err
}

type FeeLine struct {
	ID       string      `json:"id"` // ticker or account id
	Name     string      `json:"name"`
	Kind     string      `json:"kind"` // fund | stock | account
	Value    money.Cents `json:"value"`
	TER      float64     `json:"ter"`
	Platform float64     `json:"platform"`
	Source   string      `json:"source"` // yours | typical | none (stocks) | missing
	Yearly   money.Cents `json:"yearly"` // a year at today's value
	Drag20   money.Cents `json:"drag_20y"`
	Saving20 money.Cents `json:"saving_20y"` // against the low-cost yardstick, when dearer
}

type Fees struct {
	Lines    []FeeLine   `json:"lines"`
	Value    money.Cents `json:"value"`
	Yearly   money.Cents `json:"yearly"`
	Weighted float64     `json:"weighted"` // % a year across what has a figure
	Drag20   money.Cents `json:"drag_20y"`
	Saving20 money.Cents `json:"saving_20y"`
	Missing  int         `json:"missing"` // funds without a figure yet
	Return   float64     `json:"return"`  // % a year assumed (nominal)
	LowCost  float64     `json:"low_cost"`
}

// drag is what a yearly fee takes off value over years at gross return r
// (%). Compounding needs floats; the result comes back as cents.
func drag(value money.Cents, r, fee float64, years int) money.Cents {
	return money.FromFloat(value.Float() * (math.Pow(1+r/100, float64(years)) - math.Pow(1+(r-fee)/100, float64(years))))
}

// Fees builds the analysis. holdings are the trade-based positions (ticker,
// name, value in EUR); fundAccounts the balance-only fund and pension accounts
// (id → name, value); r the expected nominal return, %.
// core: tickers the owner marked as core funds — a fund the table doesn't
// know is asked for, not taken for a share with no charge.
func AnalyseFees(settings map[string]FeeSetting, holdings []FeeLine, fundAccounts []FeeLine, r float64, core ...string) *Fees {
	isCore := map[string]bool{}
	for _, c := range core {
		isCore[strings.ToUpper(c)] = true
	}
	// The owner's figures match whatever case the ticker was saved in.
	mine := map[string]FeeSetting{}
	for id, s := range settings {
		mine[strings.ToUpper(id)] = s
	}
	out := &Fees{Lines: []FeeLine{}, Return: r, LowCost: LowCost}
	var withFee money.Cents
	add := func(l FeeLine) {
		if s, ok := mine[strings.ToUpper(l.ID)]; ok {
			l.TER, l.Platform, l.Source = s.TER, s.Platform, "yours"
			if l.Kind == "stock" { // a fee given means it's a fund the table didn't know
				l.Kind = "fund"
			}
		}
		fee := l.TER + l.Platform
		l.Yearly = money.FromFloat(l.Value.Float() * fee / 100)
		l.Drag20 = drag(l.Value, r, fee, 20)
		if fee > LowCost && l.Kind != "stock" {
			l.Saving20 = l.Drag20 - drag(l.Value, r, LowCost, 20)
		}
		if l.Source == "missing" {
			out.Missing++
		} else {
			withFee += l.Value
		}
		out.Value += l.Value
		out.Yearly += l.Yearly
		out.Drag20 += l.Drag20
		out.Saving20 += l.Saving20
		out.Lines = append(out.Lines, l)
	}
	for _, h := range holdings {
		t := strings.ToUpper(h.ID)
		if ter, ok := typicalTER[t]; ok {
			h.Kind, h.TER, h.Source = "fund", ter, "typical"
		} else if isCore[t] {
			h.Kind, h.Source = "fund", "missing"
		} else if h.Kind == "" {
			h.Kind, h.Source = "stock", "none"
		}
		add(h)
	}
	for _, a := range fundAccounts {
		a.Kind, a.Source = "account", "missing"
		add(a)
	}
	if withFee > 0 {
		out.Weighted = math.Round(out.Yearly.Float()/withFee.Float()*100*100) / 100
	}
	sort.SliceStable(out.Lines, func(i, j int) bool { return out.Lines[i].Yearly > out.Lines[j].Yearly })
	return out
}
