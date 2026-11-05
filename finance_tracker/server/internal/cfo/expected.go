package cfo

import (
	"database/sql"
	"math"
	"strings"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
)

// ExpectSalary marks the months in flows (by month) whose pay hasn't all
// come yet but still may — this month, and last month until its pay window
// closes — with what is still to come and the savings rate counting it.
// Without it November read −157% on the 5th: €100 of gifts in against €257
// spent, with the salary due by 10 Dec. Still to come is the plan's income
// for the month less what already came: in gross mode the plan models the
// salary, so salary received counts against it (an advance on the 15th
// leaves the rest due); a fixed amount or the median models all income, so
// all income received does — other income is never counted twice. Months
// whose window has closed keep their numbers: a missing salary there is real.
func ExpectSalary(d *sql.DB, flows []insights.Flow, now time.Time) error {
	settings := plan.LoadSettings(d)
	rules := settings.Salary()
	nowYM := now.Format("2006-01")
	var income map[string]money.Cents // the last months' income, for the median basis
	for i := range flows {
		f := &flows[i]
		if len(f.Period) != 7 || f.Period > nowYM || plan.SalarySettled(f.Period, rules, now) {
			continue
		}
		if income == nil {
			var err error
			if income, err = monthlyIncome(d, now, rules); err != nil {
				return err
			}
		}
		base, src := plan.IncomeBase(settings, income, f.Period, nowYM)
		received := f.Income
		if strings.HasPrefix(src, "gross") {
			received = salaryIn(f)
		}
		due := base - received
		if due <= 0 {
			continue
		}
		f.ExpectedSalary = due
		in := f.Income + due
		rate := math.Round((in-f.Spending).Float()/in.Float()*1000) / 1000
		f.ExpectedSavingsRate = &rate
	}
	return nil
}

func salaryIn(f *insights.Flow) money.Cents {
	var s money.Cents
	for k, v := range f.IncomeBy {
		if ledger.Top(k) == "salary" {
			s += v
		}
	}
	return s
}

// monthlyIncome is income per month (salary in the month it pays for) over
// the last 14 months, as the plan's median income basis reads it.
func monthlyIncome(d *sql.DB, now time.Time, rules []plan.SalaryRule) (map[string]money.Cents, error) {
	txs, err := ledger.All(d, ledger.Filter{From: now.AddDate(0, -14, 0).Format("2006-01") + "-01"})
	if err != nil {
		return nil, err
	}
	cats, _ := ledger.CategoryMap(d)
	out := map[string]money.Cents{}
	for _, f := range insights.CashFlow(txs, cats, "month", rules) {
		out[f.Period] = f.Income
	}
	return out, nil
}
