package wealth

import (
	"database/sql"
	"encoding/json"
	"math"
	"time"

	"ft/internal/db"

	"ft/internal/ledger"
	"ft/internal/money"
)

// LoanView is a loan's position: balance, rate, what is left to pay and how
// the next payment splits. Interest is computed on the current balance at
// the contracted rate (base + margin).
type LoanView struct {
	AccountID       string             `json:"account_id"`
	Name            string             `json:"name"`
	Details         ledger.LoanDetails `json:"details"`
	Balance         money.Cents        `json:"balance"` // positive amount owed
	BalanceDate     string             `json:"balance_date"`
	Rate            float64            `json:"rate"` // % p.a.
	MonthsLeft      int                `json:"months_left"`
	NextInterest    money.Cents        `json:"next_interest"`
	NextPrincipal   money.Cents        `json:"next_principal"`
	TotalInterest   money.Cents        `json:"total_interest_left"`
	PayoffDate      string             `json:"payoff_date"`
	AssetValue      money.Cents        `json:"asset_value,omitempty"`
	Equity          money.Cents        `json:"equity,omitempty"`
	LTV             float64            `json:"ltv,omitempty"`
	DaysToReset     int                `json:"days_to_reset,omitempty"`
	Schedule        []SchedulePoint    `json:"schedule"`
	PaidPrincipal12 money.Cents        `json:"paid_principal_12m"`
	StartPrincipal  money.Cents        `json:"start_principal,omitempty"`
	Repaid          money.Cents        `json:"repaid,omitempty"` // original principal − owed now
	RepaidPct       float64            `json:"repaid_pct,omitempty"`
	// Where the equity came from: own money at purchase, principal repaid,
	// and the asset's change in value since purchase (sums to Equity).
	PurchasePrice  money.Cents `json:"purchase_price,omitempty"`
	DownPayment    money.Cents `json:"down_payment,omitempty"`
	Appreciation   money.Cents `json:"appreciation,omitempty"`
	PaidInterest12 money.Cents `json:"paid_interest_12m"`
}

type SchedulePoint struct {
	Month     string      `json:"month"`
	Balance   money.Cents `json:"balance"`
	Interest  money.Cents `json:"interest"`
	Principal money.Cents `json:"principal"`
}

// Loans builds a view of every loan account.
func Loans(book *Book, txs []ledger.Tx, now time.Time) []LoanView {
	var out []LoanView
	today := now.Format("2006-01-02")
	yearAgo := now.AddDate(-1, 0, 0).Format("2006-01-02")
	for id, a := range book.Accounts {
		if a.Kind != "loan" || a.Archived {
			continue
		}
		var d ledger.LoanDetails
		json.Unmarshal(a.Details, &d)
		p, ok := book.At(id, today)
		if !ok {
			continue
		}
		v := LoanView{AccountID: id, Name: a.Name, Details: d, Balance: -p.Value, BalanceDate: p.Date, Rate: d.BaseRate + d.Margin}
		if d.StartPrincipal > 0 {
			v.StartPrincipal = money.FromFloat(d.StartPrincipal)
			v.Repaid = v.StartPrincipal - v.Balance
			v.RepaidPct = math.Round(v.Repaid.Float()/d.StartPrincipal*1000) / 1000
		}
		if d.AssetID != "" {
			if ap, ok := book.At(d.AssetID, today); ok {
				v.AssetValue = ap.Value
				v.Equity = ap.Value - v.Balance
				if asset, ok := book.Accounts[d.AssetID]; ok {
					var ad struct {
						PurchasePrice float64 `json:"purchase_price"`
					}
					json.Unmarshal(asset.Details, &ad)
					if ad.PurchasePrice > 0 {
						v.PurchasePrice = money.FromFloat(ad.PurchasePrice)
						v.Appreciation = ap.Value - v.PurchasePrice
						if d.StartPrincipal > 0 {
							v.DownPayment = v.PurchasePrice - money.FromFloat(d.StartPrincipal)
						}
					}
				}
				if ap.Value > 0 {
					v.LTV = math.Round(v.Balance.Float()/ap.Value.Float()*1000) / 1000
				}
			}
		}
		if d.RateResetDate != "" {
			if t, err := time.Parse("2006-01-02", d.RateResetDate); err == nil {
				v.DaysToReset = int(t.Sub(now).Hours() / 24)
			}
		}
		for _, t := range txs {
			if t.Date <= yearAgo {
				continue
			}
			if t.Kind == "transfer" && t.ToAccountID == id {
				v.PaidPrincipal12 += t.Amount
			}
			if t.Category == "housing.mortgage_interest" {
				v.PaidInterest12 += t.Amount
			}
		}
		// Forward schedule at today's rate.
		r := v.Rate / 100 / 12
		bal := v.Balance.Float()
		pay := d.MonthlyPayment
		if pay <= 0 && d.EndDate != "" {
			if end, err := time.Parse("2006-01-02", d.EndDate); err == nil {
				n := int(end.Sub(now).Hours()/24/30.44) + 1
				if r > 0 && n > 0 {
					pay = bal * r / (1 - math.Pow(1+r, -float64(n)))
				}
			}
		}
		var totalInt float64
		cur := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		for m := 0; bal > 0.005 && m < 600 && pay > 0; m++ {
			interest := bal * r
			principal := math.Min(pay-interest, bal)
			if principal <= 0 {
				break // payment does not cover interest
			}
			if m == 0 {
				v.NextInterest, v.NextPrincipal = money.FromFloat(interest), money.FromFloat(principal)
			}
			bal -= principal
			totalInt += interest
			cur = cur.AddDate(0, 1, 0)
			if m%12 == 0 || bal <= 0.005 {
				v.Schedule = append(v.Schedule, SchedulePoint{Month: cur.Format("2006-01"), Balance: money.FromFloat(bal),
					Interest: money.FromFloat(interest), Principal: money.FromFloat(principal)})
			}
			v.MonthsLeft = m + 1
		}
		v.TotalInterest = money.FromFloat(totalInt)
		if v.MonthsLeft > 0 {
			v.PayoffDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, v.MonthsLeft, 0).Format("2006-01")
		}
		out = append(out, v)
	}
	return out
}

// RebuildLoanHistory re-draws a loan's reconstructed ('computed') monthly
// balances so they run exactly from the original principal on the start date
// to the first real balance (bank, manual or imported) after it. Months where
// the bank split out principal use it exactly; the other months are estimated
// from the annuity and scaled so both ends match. Real balances are never
// touched. Returns the number of months written (0 when there is nothing to
// anchor on).
func RebuildLoanHistory(d *sql.DB, loanID string) (int, error) {
	var kind, raw string
	if err := d.QueryRow(`SELECT kind, details FROM accounts WHERE id=?`, loanID).Scan(&kind, &raw); err != nil || kind != "loan" {
		return 0, err
	}
	var det ledger.LoanDetails
	json.Unmarshal([]byte(raw), &det)
	start, err := time.Parse("2006-01-02", det.StartDate)
	if err != nil || det.StartPrincipal <= 0 {
		return 0, nil
	}
	// The first real balance after the start is the anchor.
	var anchorDate string
	var anchorVal int64
	err = d.QueryRow(`SELECT date, value FROM balances WHERE account_id=? AND source<>'computed' AND date>? ORDER BY date LIMIT 1`, loanID, det.StartDate).Scan(&anchorDate, &anchorVal)
	if err != nil {
		return 0, nil
	}
	anchor, _ := time.Parse("2006-01-02", anchorDate)
	owedAtAnchor := -money.Cents(anchorVal).Float()
	day := det.PaymentDay
	if day < 1 || day > 28 {
		day = start.Day()
		if day > 28 {
			day = 28
		}
	}
	// Months with a payment: the month after the start through the anchor's month.
	var months []string
	for m := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0); !m.After(time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, time.UTC)); m = m.AddDate(0, 1, 0) {
		months = append(months, m.Format("2006-01"))
	}
	if len(months) == 0 {
		return 0, nil
	}
	// Recorded principal (transfers into the loan) and combined payments per month.
	known, combined := map[string]float64{}, map[string]float64{}
	rows, err := d.Query(`SELECT substr(date,1,7), kind, COALESCE(to_account_id,''), category, SUM(amount) FROM transactions
		WHERE ((kind='transfer' AND to_account_id=?) OR category='housing.mortgage') AND date>? AND date<=? GROUP BY 1,2,3,4`, loanID, det.StartDate, anchorDate)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var m, k, to, cat string
		var sum int64
		rows.Scan(&m, &k, &to, &cat, &sum)
		if k == "transfer" && to == loanID {
			known[m] += money.Cents(sum).Float()
		} else if cat == "housing.mortgage" {
			combined[m] += money.Cents(sum).Float()
		}
	}
	rows.Close()
	rate := (det.BaseRate + det.Margin) / 100 / 12
	if rate <= 0 {
		rate = 0.04 / 12
	}
	pay := det.MonthlyPayment
	// Estimate principal for months the bank didn't split, walking forward.
	est := map[string]float64{}
	var sumKnown, sumEst float64
	b := det.StartPrincipal
	for _, m := range months {
		if p, ok := known[m]; ok && p > 0 {
			b -= p
			sumKnown += p
			continue
		}
		p := pay
		if c := combined[m]; c > 0 {
			p = c
		}
		pr := math.Max(0, p-b*rate)
		est[m] = pr
		sumEst += pr
		b -= pr
	}
	// Scale the estimates so the path lands exactly on the anchor.
	need := det.StartPrincipal - owedAtAnchor - sumKnown
	scale := 1.0
	switch {
	case sumEst > 0 && need > 0:
		scale = need / sumEst
	case len(est) > 0 && need > 0: // no estimate to scale: spread evenly
		for m := range est {
			est[m] = need / float64(len(est))
		}
	}
	tx, err := d.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM balances WHERE account_id=? AND source='computed' AND date>=? AND date<?`, loanID, det.StartDate, anchorDate); err != nil {
		return 0, err
	}
	now := db.Now()
	put := func(date string, owed float64) error {
		_, err := tx.Exec(`INSERT INTO balances(account_id,date,value,source,updated_at) VALUES(?,?,?,'computed',?)
			ON CONFLICT(account_id,date) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at WHERE balances.source='computed'`,
			loanID, date, -int64(money.FromFloat(owed)), now)
		return err
	}
	if err := put(det.StartDate, det.StartPrincipal); err != nil {
		return 0, err
	}
	n := 1
	b = det.StartPrincipal
	for _, m := range months[:len(months)-1] { // the anchor month is the real balance itself
		if p, ok := known[m]; ok && p > 0 {
			b -= p
		} else {
			b -= est[m] * scale
		}
		t, _ := time.Parse("2006-01", m)
		if err := put(time.Date(t.Year(), t.Month(), day, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), math.Round(b*100)/100); err != nil {
			return 0, err
		}
		n++
	}
	return n, tx.Commit()
}
