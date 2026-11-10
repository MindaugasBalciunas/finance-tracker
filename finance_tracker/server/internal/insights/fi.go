package insights

import (
	"math"
	"time"

	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

// Last12 sums cash flow over the last 12 complete months, the window Home's
// 12-month average uses. A window of the last 365 days cut through a month:
// on 9 November it dropped November 2025's salary (paid on the 7th) but kept
// that month's spending, and FI showed €1,437 saved a month against Home's
// €1,701. txs must reach back to the first of the month a year ago.
func Last12(txs []ledger.Tx, cats map[string]ledger.Category, salary []plan.SalaryRule, now time.Time) Flow {
	from, cur := ledger.AddMonths(now, -12), now.Format("2006-01")
	sum := Flow{Period: from + "/" + ledger.AddMonths(now, -1)}
	for _, f := range CashFlow(txs, cats, "month", salary) {
		if f.Period < from || f.Period >= cur {
			continue
		}
		sum.Income += f.Income
		sum.Spending += f.Spending
		sum.Essential += f.Essential
		sum.Discretionary += f.Discretionary
		sum.Refunds += f.Refunds
		sum.Invested += f.Invested
		sum.Principal += f.Principal
		sum.PayrollPension += f.PayrollPension
	}
	sum.Saved = sum.Income - sum.Spending
	if sum.Income > 0 {
		sum.SavingsRate = round3(sum.Saved.Float() / sum.Income.Float())
	}
	return sum
}

// FI is progress to financial independence.
type FI struct {
	AnnualSpend     money.Cents `json:"annual_spend"` // the last 12 complete months, or the target from settings
	SpendSource     string      `json:"spend_source"`
	Target          money.Cents `json:"target"`           // annual spend / withdrawal rate
	Investable      money.Cents `json:"investable"`       // investments + pension + crypto + cash above the emergency buffer
	Progress        float64     `json:"progress"`         // investable / target
	MonthlyInvested money.Cents `json:"monthly_invested"` // average saved a month (income − spending) over the last 12 complete months, all assumed invested
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
	last := Last12(txs, cats, s.Salary(), now)
	spend, essential, saved := last.Spending, last.Essential, last.Saved
	snap := book.SnapshotAt(now.Format("2006-01-02"), s.EmergencyExcludeCash)
	cash := snap.ByGroup["cash"]
	if s.EmergencyExcludeCash { // only money in the bank is the emergency fund
		for id, v := range snap.ByAccount {
			if book.Accounts[id].Kind == "cash" {
				cash -= v
			}
		}
	}
	out := FI{ExpectedReturn: s.ExpectedReturn, WithdrawalRate: s.WithdrawalRate, TargetAge: s.TargetAge}
	out.Emergency = Emergency{Cash: cash, MonthlyEssential: essential / 12, TargetMonths: s.EmergencyMonths}
	out.Emergency.Target = money.FromFloat(out.Emergency.MonthlyEssential.Float() * s.EmergencyMonths)
	if out.Emergency.MonthlyEssential > 0 {
		out.Emergency.Months = math.Round(cash.Float()/out.Emergency.MonthlyEssential.Float()*10) / 10
	}
	out.AnnualSpend, out.SpendSource = spend, "last 12 months' spending"
	if s.FIMonthlySpend > 0 {
		out.AnnualSpend, out.SpendSource = s.FIMonthlySpend*12, "your FI spending target"
	}
	out.Target = money.FromFloat(out.AnnualSpend.Float() * 100 / s.WithdrawalRate)
	// Cash beyond the fund is investable either way: leaving notes out of
	// the emergency fund doesn't take them out of what works toward FI.
	buffer := snap.ByGroup["cash"] - out.Emergency.Target
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
				out.FIDate = ledger.AddMonths(now, m)
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
