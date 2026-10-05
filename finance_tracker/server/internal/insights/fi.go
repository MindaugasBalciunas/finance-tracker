package insights

import (
	"math"
	"time"

	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

// FI is progress to financial independence.
type FI struct {
	AnnualSpend     money.Cents `json:"annual_spend"` // trailing 12 months, or the target from settings
	SpendSource     string      `json:"spend_source"`
	Target          money.Cents `json:"target"`           // annual spend / withdrawal rate
	Investable      money.Cents `json:"investable"`       // investments + pension + crypto + cash above the emergency buffer
	Progress        float64     `json:"progress"`         // investable / target
	MonthlyInvested money.Cents `json:"monthly_invested"` // trailing 12-month average saved
	ExpectedReturn  float64     `json:"expected_return"`
	WithdrawalRate  float64     `json:"withdrawal_rate"`
	YearsToFI       float64     `json:"years_to_fi"` // -1 when not reachable in 60 years
	FIDate          string      `json:"fi_date,omitempty"`
	FIAge           float64     `json:"fi_age,omitempty"`
	TargetAge       int         `json:"target_age,omitempty"`
	RequiredMonthly money.Cents `json:"required_monthly,omitempty"` // to hit the target age
	Emergency       Emergency   `json:"emergency"`
	Projection      []FIPoint   `json:"projection"`
}

type FIPoint struct {
	Year       int         `json:"year"`
	Investable money.Cents `json:"investable"`
	Target     money.Cents `json:"target"`
}

// Emergency is the cash buffer against essential spending.
type Emergency struct {
	Cash             money.Cents `json:"cash"`
	MonthlyEssential money.Cents `json:"monthly_essential"`
	Months           float64     `json:"months"`
	TargetMonths     float64     `json:"target_months"`
	Target           money.Cents `json:"target"`
}

func growMonths(start, monthly, rateMonthly float64, months int) float64 {
	v := start
	for i := 0; i < months; i++ {
		v = v*(1+rateMonthly) + monthly
	}
	return v
}

// ComputeFI combines the trailing year of cash flow with today's balances.
func ComputeFI(txs []ledger.Tx, cats map[string]ledger.Category, book *wealth.Book, s plan.Settings, now time.Time) FI {
	from := now.AddDate(-1, 0, 0).Format("2006-01-02")
	var spend, essential, saved money.Cents
	var recent []ledger.Tx
	for _, t := range txs {
		if t.Date > from {
			recent = append(recent, t)
		}
	}
	for _, f := range CashFlow(recent, cats, "year") {
		spend += f.Spending
		essential += f.Essential
		saved += f.Saved
	}
	snap := book.SnapshotAt(now.Format("2006-01-02"), false)
	cash := snap.ByGroup["cash"]
	out := FI{ExpectedReturn: s.ExpectedReturn, WithdrawalRate: s.WithdrawalRate, TargetAge: s.TargetAge}
	out.Emergency = Emergency{Cash: cash, MonthlyEssential: essential / 12, TargetMonths: s.EmergencyMonths}
	out.Emergency.Target = money.FromFloat(out.Emergency.MonthlyEssential.Float() * s.EmergencyMonths)
	if out.Emergency.MonthlyEssential > 0 {
		out.Emergency.Months = math.Round(cash.Float()/out.Emergency.MonthlyEssential.Float()*10) / 10
	}
	out.AnnualSpend, out.SpendSource = spend, "last 12 months' spending"
	if s.FIMonthlySpend > 0 {
		out.AnnualSpend, out.SpendSource = money.FromFloat(s.FIMonthlySpend*12), "your FI spending target"
	}
	out.Target = money.FromFloat(out.AnnualSpend.Float() * 100 / s.WithdrawalRate)
	buffer := cash - out.Emergency.Target
	if buffer < 0 {
		buffer = 0
	}
	out.Investable = snap.ByGroup["investments"] + snap.ByGroup["pension"] + snap.ByGroup["crypto"] + buffer
	if out.Target > 0 {
		out.Progress = round3(out.Investable.Float() / out.Target.Float())
	}
	out.MonthlyInvested = saved / 12
	r := s.ExpectedReturn / 100 / 12
	start, monthly, target := out.Investable.Float(), math.Max(out.MonthlyInvested.Float(), 0), out.Target.Float()
	out.YearsToFI = -1
	if start >= target && target > 0 {
		out.YearsToFI = 0
	} else {
		v := start
		for m := 1; m <= 720; m++ {
			v = v*(1+r) + monthly
			if v >= target {
				out.YearsToFI = math.Round(float64(m)/12*10) / 10
				out.FIDate = now.AddDate(0, m, 0).Format("2006-01")
				break
			}
		}
	}
	if s.BirthYear > 0 && out.YearsToFI >= 0 {
		out.FIAge = math.Round((float64(now.Year()-s.BirthYear)+out.YearsToFI)*10) / 10
	}
	if s.TargetAge > 0 && s.BirthYear > 0 {
		months := (s.BirthYear+s.TargetAge-now.Year())*12 - int(now.Month()) + 1
		if months > 0 {
			// Solve for the monthly contribution that reaches the target.
			lo, hi := 0.0, target
			for i := 0; i < 60; i++ {
				mid := (lo + hi) / 2
				if growMonths(start, mid, r, months) >= target {
					hi = mid
				} else {
					lo = mid
				}
			}
			out.RequiredMonthly = money.FromFloat(math.Ceil(hi/10) * 10)
		}
	}
	v := start
	for y := 0; y <= 25; y++ {
		out.Projection = append(out.Projection, FIPoint{Year: now.Year() + y, Investable: money.FromFloat(v), Target: out.Target})
		v = growMonths(v, monthly, r, 12)
	}
	return out
}
