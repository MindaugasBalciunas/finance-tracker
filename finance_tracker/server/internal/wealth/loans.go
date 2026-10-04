package wealth

import (
	"encoding/json"
	"math"
	"time"

	"ft/internal/ledger"
	"ft/internal/money"
)

// LoanView is a loan's position: balance, rate, what is left to pay and how
// the next payment splits. Interest is computed on the current balance at
// the contracted rate (base + margin).
type LoanView struct {
	AccountID      string           `json:"account_id"`
	Name           string           `json:"name"`
	Details        ledger.LoanDetails `json:"details"`
	Balance        money.Cents      `json:"balance"` // positive amount owed
	BalanceDate    string           `json:"balance_date"`
	Rate           float64          `json:"rate"` // % p.a.
	MonthsLeft     int              `json:"months_left"`
	NextInterest   money.Cents      `json:"next_interest"`
	NextPrincipal  money.Cents      `json:"next_principal"`
	TotalInterest  money.Cents      `json:"total_interest_left"`
	PayoffDate     string           `json:"payoff_date"`
	AssetValue     money.Cents      `json:"asset_value,omitempty"`
	Equity         money.Cents      `json:"equity,omitempty"`
	LTV            float64          `json:"ltv,omitempty"`
	DaysToReset    int              `json:"days_to_reset,omitempty"`
	Schedule       []SchedulePoint  `json:"schedule"`
	PaidPrincipal12 money.Cents     `json:"paid_principal_12m"`
	PaidInterest12  money.Cents     `json:"paid_interest_12m"`
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
		if d.AssetID != "" {
			if ap, ok := book.At(d.AssetID, today); ok {
				v.AssetValue = ap.Value
				v.Equity = ap.Value - v.Balance
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
