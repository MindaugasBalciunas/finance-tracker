package cfo

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

// TaxYear estimates what a Lithuanian annual income declaration (GPM311)
// will show for a year: salary above the 25% bracket that monthly 20%
// withholding doesn't cover, taxable interest and investment gains above
// their €500 exemptions, and the 3rd-pillar refund where the contract still
// qualifies. Estimates from the ledger — VMI's pre-filled declaration is
// what counts — so each item says what it is based on.
type TaxYear struct {
	Year     int         `json:"year"`
	Opens    string      `json:"opens"`    // the declaration opens (VMI pre-fills it)
	Deadline string      `json:"deadline"` // file and pay by
	Items    []TaxItem   `json:"items"`
	Due      money.Cents `json:"due"`    // estimated tax to pay with the declaration
	Refund   money.Cents `json:"refund"` // estimated refund
	Pillar3  *bool       `json:"pillar3_before_2025"`
}

type TaxItem struct {
	Key    string      `json:"key"`
	Kind   string      `json:"kind"` // due | refund | exempt | info
	Title  string      `json:"title"`
	Detail string      `json:"detail"`
	Amount money.Cents `json:"amount"`
}

// Annual income where the 25% rate starts: 36 average wages (from 2026;
// before that the next rate started at 60 AW, which this salary is far from).
var bracket25 = map[int]float64{2026: 83237}

const (
	gpmInvest     = 0.15 // interest and investment gains above the exemptions
	exemptGains   = 500  // € of net gain from financial instruments a year
	exemptInt     = 500  // € of deposit interest a year
	pillar3Max    = 1500 // € of 3rd-pillar contributions that earn the refund
	pillar3Refund = 0.20
)

// TaxSettings are the owner's answers the ledger can't give.
type TaxSettings struct {
	// Pillar3Before2025: the 3rd-pillar contract was signed before 2025,
	// so its contributions still earn the 20% refund (for 10 years).
	Pillar3Before2025 *bool `json:"pillar3_before_2025,omitempty"`
}

func LoadTaxSettings(d *sql.DB) TaxSettings {
	var s TaxSettings
	var raw string
	if d.QueryRow(`SELECT value FROM settings WHERE key='tax'`).Scan(&raw) == nil {
		json.Unmarshal([]byte(raw), &s)
	}
	return s
}

func SaveTaxSettings(d *sql.DB, s TaxSettings) error {
	raw, _ := json.Marshal(s)
	_, err := d.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('tax',?)`, string(raw))
	return err
}

// Tax estimates the declaration for year (filed the following spring).
func Tax(d *sql.DB, year int, now time.Time) (*TaxYear, error) {
	from, to := fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year)
	ts := LoadTaxSettings(d)
	t := &TaxYear{Year: year, Opens: fmt.Sprintf("%d-03-01", year+1), Deadline: fmt.Sprintf("%d-05-01", year+1), Items: []TaxItem{}, Pillar3: ts.Pillar3Before2025}
	settings := plan.LoadSettings(d)
	txs, err := ledger.All(d, ledger.Filter{From: from, To: to})
	if err != nil {
		return nil, err
	}

	// Salary above the 25% bracket.
	if limit, ok := bracket25[year]; ok {
		if settings.GrossSalary > 0 {
			annual := settings.GrossSalary * 12
			if over := annual - limit; over > 0 {
				due := money.FromFloat(over * 0.05)
				t.add(TaxItem{Key: "bracket", Kind: "due", Amount: due, Title: "Salary above the 25% bracket",
					Detail: fmt.Sprintf("€%s of your €%s a year (gross %s × 12) is above €%s, taxed 25%% instead of 20%%. If your employer withholds 20%% each month, about %s is left to pay with the declaration.",
						thousands(over), thousands(annual), eurS(money.FromFloat(settings.GrossSalary)), thousands(limit), eurS(due))})
			} else {
				t.add(TaxItem{Key: "bracket", Kind: "info", Title: "Salary within the 20% bracket", Detail: fmt.Sprintf("€%s a year is under €%s.", thousands(annual), thousands(limit))})
			}
		} else {
			t.add(TaxItem{Key: "bracket", Kind: "info", Title: "Gross salary not set", Detail: "Set your gross salary in Plan → Income & goals to estimate the 25% bracket."})
		}
	}

	// Deposit interest (and other investment income booked as such).
	var interest money.Cents
	for i := range txs {
		if txs[i].Kind == "income" && ledger.Top(txs[i].Category) == "investment_income" {
			interest += txs[i].Amount
		}
	}
	if interest > 0 {
		it := TaxItem{Key: "interest", Kind: "exempt", Amount: interest, Title: "Interest",
			Detail: fmt.Sprintf("%s this year — tax-free up to €%d.", eurS(interest), exemptInt)}
		if over := interest - money.FromFloat(exemptInt); over > 0 {
			it.Kind, it.Amount = "due", money.FromFloat(over.Float()*gpmInvest)
			it.Detail = fmt.Sprintf("%s this year; the %s above €%d is taxed 15%%.", eurS(interest), eurS(over), exemptInt)
		}
		t.add(it)
	}

	// Gains realised on investments sold this year.
	if trades, err := wealth.ListTrades(d); err == nil && len(trades) > 0 {
		realisedBy := func(date string) float64 {
			var in []wealth.Trade
			for _, tr := range trades {
				if tr.Date <= date {
					in = append(in, tr)
				}
			}
			return wealth.BuildPortfolio(in, false).RealizedEUR
		}
		gain := money.FromFloat(realisedBy(to) - realisedBy(fmt.Sprintf("%d-12-31", year-1)))
		switch {
		case gain > money.FromFloat(exemptGains):
			over := gain - money.FromFloat(exemptGains)
			t.add(TaxItem{Key: "gains", Kind: "due", Amount: money.FromFloat(over.Float() * gpmInvest), Title: "Investment gains",
				Detail: fmt.Sprintf("%s realised from sales this year; the %s above €%d is taxed 15%%. Losses on other sales in the year offset it.", eurS(gain), eurS(over), exemptGains)})
		case gain > 0:
			t.add(TaxItem{Key: "gains", Kind: "exempt", Amount: gain, Title: "Investment gains", Detail: fmt.Sprintf("%s realised this year — under the €%d exemption.", eurS(gain), exemptGains)})
		case gain < 0:
			t.add(TaxItem{Key: "gains", Kind: "info", Amount: gain, Title: "Investment losses", Detail: fmt.Sprintf("%s realised this year — no tax; declare it to offset gains.", eurN(gain))})
		}
		t.add(TaxItem{Key: "dividends", Kind: "info", Title: "Dividends", Detail: "Taxed 15%; tax withheld abroad usually counts against it — the broker's annual report shows both."})
	}

	// 3rd pillar: the refund survives only for contracts signed before 2025.
	var pillar money.Cents
	for i := range txs {
		if txs[i].Kind == "transfer" && txs[i].Category == "transfer.pension" {
			pillar += txs[i].Amount
		}
	}
	if pillar > 0 {
		base := min(pillar, money.FromFloat(pillar3Max))
		if settings.GrossSalary > 0 {
			base = min(base, money.FromFloat(settings.GrossSalary*12*0.25))
		}
		refund := money.FromFloat(base.Float() * pillar3Refund)
		it := TaxItem{Key: "pillar3", Title: "3rd-pillar pension contributions", Amount: refund}
		switch {
		case ts.Pillar3Before2025 == nil:
			it.Kind = "info"
			it.Detail = fmt.Sprintf("%s paid in this year. If your contract was signed before 2025, 20%% of up to €%d comes back: %s. Contracts from 2025 on no longer earn it — tell the app which applies.", eurS(pillar), pillar3Max, eurS(refund))
		case *ts.Pillar3Before2025:
			it.Kind = "refund"
			it.Detail = fmt.Sprintf("%s paid in this year; 20%% of %s comes back with the declaration (the contract predates 2025).", eurS(pillar), eurS(base))
		default:
			it.Kind, it.Amount = "info", 0
			it.Detail = fmt.Sprintf("%s paid in this year — no refund: contracts signed from 2025 on don't earn one.", eurS(pillar))
		}
		t.add(it)
	}
	return t, nil
}

func (t *TaxYear) add(it TaxItem) {
	t.Items = append(t.Items, it)
	switch it.Kind {
	case "due":
		t.Due += it.Amount
	case "refund":
		t.Refund += it.Amount
	}
}

func thousands(v float64) string {
	s := eurS(money.FromFloat(v))
	return strings.TrimPrefix(s, "€")
}
