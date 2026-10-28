package cfo

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

// Summary is the first Insights page: where a typical month's money goes,
// then the few findings worth acting on, each with the numbers behind it
// and where to act. Everything comes from the app's own figures (cash flow,
// the plan engine, recurring costs) so it agrees with the other pages.
type Summary struct {
	From     string    `json:"from"` // first complete month covered (YYYY-MM)
	To       string    `json:"to"`   // last complete month
	Months   int       `json:"months"`
	Flow     FlowSplit `json:"flow"`
	Findings []Finding `json:"findings"`
}

// FlowSplit is a typical month (averages over the window): income split
// into what it went on. Fixed + Everyday + OneOffs + Saved = Income.
type FlowSplit struct {
	Income   money.Cents `json:"income"`
	Fixed    money.Cents `json:"fixed"`    // obligations paid as expenses (alimony, mortgage interest)
	Everyday money.Cents `json:"everyday"` // the rest of spending, net of refunds
	OneOffs  money.Cents `json:"one_offs"` // single payments of €250+ outside fixed costs
	Saved    money.Cents `json:"saved"`    // income − spending (negative = lived off savings)
	Invested money.Cents `json:"invested"` // moved into investments, pension, loan principal — from any source
}

type Finding struct {
	Key    string        `json:"key"`
	Tone   string        `json:"tone"` // bad | warn | info | good
	Title  string        `json:"title"`
	Detail string        `json:"detail"`
	Amount money.Cents   `json:"amount"`         // the headline figure, per month unless the title says otherwise
	Link   string        `json:"link,omitempty"` // app route to act on it
	Items  []FindingItem `json:"items,omitempty"`
}

type FindingItem struct {
	Label  string      `json:"label"`
	Sub    string      `json:"sub,omitempty"`
	Amount money.Cents `json:"amount"`
	Tone   string      `json:"tone,omitempty"` // bad | good when the sign carries meaning
}

var toneRank = map[string]int{"bad": 0, "warn": 1, "info": 2, "good": 3}

// BuildSummary covers the last twelve complete months before now.
func BuildSummary(d *sql.DB, now time.Time) (*Summary, error) {
	last := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, time.UTC)
	first := last.AddDate(0, -11, 0)
	from, to := first.Format("2006-01"), last.Format("2006-01")
	toDay := last.AddDate(0, 1, -1).Format("2006-01-02")
	txs, err := ledger.All(d, ledger.Filter{})
	if err != nil {
		return nil, err
	}
	cats, err := ledger.CategoryMap(d)
	if err != nil {
		return nil, err
	}
	settings := plan.LoadSettings(d)
	budgets, err := plan.List(d, false)
	if err != nil {
		return nil, err
	}
	var fixed, spending []plan.Budget
	for _, b := range budgets {
		switch b.Kind {
		case "fixed":
			fixed = append(fixed, b)
		case "spending":
			spending = append(spending, b)
		}
	}
	isFixed := func(t *ledger.Tx) bool {
		for _, b := range fixed {
			if b.Matches(t) {
				return true
			}
		}
		return false
	}

	s := &Summary{From: from, To: to, Findings: []Finding{}}

	// A typical month.
	flows := insights.CashFlowRange(txs, cats, "month", from+"-01", toDay, settings.Salary())
	var n int64
	var f FlowSplit
	var spent money.Cents
	var shortMonths []FindingItem // months spending beat income
	var savedByMonth []money.Cents
	for _, fl := range flows {
		if fl.Income <= 0 && fl.Spending <= 0 {
			continue
		}
		n++
		f.Income += fl.Income
		spent += fl.Spending
		f.Saved += fl.Saved
		f.Invested += fl.Invested
		savedByMonth = append(savedByMonth, fl.Saved)
		if fl.Saved < 0 {
			shortMonths = append(shortMonths, FindingItem{Label: monthName(fl.Period), Sub: "spent more than came in", Amount: fl.Saved, Tone: "bad"})
		}
	}
	s.Months = int(n)
	if n == 0 {
		return s, nil
	}
	var oneOffs []ledger.Tx
	oneOffMonths := map[string]bool{}
	for i := range txs {
		t := &txs[i]
		if t.Kind != "expense" || t.Date < from+"-01" || t.Date > toDay {
			continue
		}
		if isFixed(t) {
			f.Fixed += t.Amount
			continue
		}
		if t.Amount >= plan.OneOff {
			oneOffs = append(oneOffs, *t)
			oneOffMonths[t.Date[:7]] = true
			f.OneOffs += t.Amount
		}
	}
	f.Everyday = spent - f.Fixed - f.OneOffs
	for _, p := range []*money.Cents{&f.Income, &f.Fixed, &f.Everyday, &f.OneOffs, &f.Saved, &f.Invested} {
		*p /= money.Cents(n)
	}
	// Keep the parts adding up after rounding: saved is what is left.
	f.Saved = f.Income - f.Fixed - f.Everyday - f.OneOffs
	s.Flow = f

	// 1. Does the plan fit? Recent months measured against today's lines and
	// the income each one actually had.
	cur, err := BudgetReport(d, now.Format("2006-01"), now)
	if err != nil {
		return nil, err
	}
	freePlanned := cur.IncomeBase - cur.FixedPlanned - cur.SavingPlanned - cur.FundContributions
	var track []FindingItem
	var trackSpent []money.Cents
	overs := 0
	for i := 6; i >= 1; i-- {
		m := time.Date(now.Year(), now.Month()-time.Month(i), 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
		r, err := BudgetReport(d, m, now)
		if err != nil || r.IncomeActual <= 0 {
			continue
		}
		sp := r.FreeSpentByMonth[m] + r.FundOverdrawn
		trackSpent = append(trackSpent, sp)
		it := FindingItem{Label: monthName(m), Sub: "left over", Amount: r.SafeToSpend, Tone: "good"}
		if r.SafeToSpend < 0 {
			overs++
			it.Sub, it.Tone = "over plan", "bad"
		}
		track = append(track, it)
	}
	if len(track) >= 3 {
		typical := medianCents(trackSpent)
		fd := Finding{Key: "plan_fit", Link: "/plan", Items: track, Amount: typical - freePlanned,
			Detail: fmt.Sprintf("Day-to-day spending runs about %s a month; after fixed costs, saving and funds the plan leaves %s.", eurN(typical), eurN(freePlanned))}
		switch {
		case overs*3 >= len(track)*2:
			fd.Tone = "bad"
			fd.Title = fmt.Sprintf("The plan ended over in %d of the last %d months", overs, len(track))
			fd.Detail += " A plan that is missed every month stops guiding anything — set the lines to what is real, or lower the saving target to what the month can fund."
		case overs > 1:
			fd.Tone = "warn"
			fd.Title = fmt.Sprintf("The plan ended over in %d of the last %d months", overs, len(track))
		default:
			fd.Tone, fd.Amount = "good", 0
			fd.Title = fmt.Sprintf("The plan held in %d of the last %d months", len(track)-overs, len(track))
		}
		s.Findings = append(s.Findings, fd)
	}

	// 2. One-offs: big single payments that keep coming.
	if len(oneOffs) > 0 {
		sort.SliceStable(oneOffs, func(i, j int) bool { return oneOffs[i].Amount > oneOffs[j].Amount })
		fd := Finding{Key: "one_offs", Tone: "info", Amount: f.OneOffs, Link: "/plan",
			Title:  fmt.Sprintf("One-offs of €250+ average %s a month", eurN(f.OneOffs)),
			Detail: fmt.Sprintf("They came in %d of the last %d months. ", len(oneOffMonths), n)}
		if len(oneOffMonths)*2 >= int(n) {
			fd.Tone = "warn"
			fd.Detail += fmt.Sprintf("That is a regular cost, not a surprise: a fund of about %s a month (or yearly lines for the known ones) keeps them from tipping single months.", eurN(f.OneOffs))
		}
		for _, t := range oneOffs[:min(6, len(oneOffs))] {
			fd.Items = append(fd.Items, FindingItem{Label: txLabel(t), Sub: t.Date + " · " + catName(cats, t.Category), Amount: t.Amount})
		}
		s.Findings = append(s.Findings, fd)
	}

	// 3. Spending no budget line covers, and lines far from how money is spent.
	// The last complete month's report carries exactly the window's twelve
	// months of history.
	lastR, err := BudgetReport(d, to, now)
	if err != nil {
		return nil, err
	}
	complete := map[string]bool{}
	for _, m := range lastR.Months {
		if m >= from && m <= to {
			complete[m] = true
		}
	}
	var unb []FindingItem
	var unbTotal money.Cents
	for _, u := range lastR.Unbudgeted {
		var sum money.Cents
		for _, c := range u.History {
			if complete[c.Month] {
				sum += c.Spent
			}
		}
		if avg := sum / money.Cents(max(len(complete), 1)); avg >= money.FromFloat(20) {
			unb = append(unb, FindingItem{Label: catName(cats, u.Category), Sub: "no budget line", Amount: avg})
			unbTotal += avg
		}
	}
	if unbTotal > 0 {
		sort.SliceStable(unb, func(i, j int) bool { return unb[i].Amount > unb[j].Amount })
		tone := "info"
		if unbTotal >= money.FromFloat(100) {
			tone = "warn"
		}
		s.Findings = append(s.Findings, Finding{Key: "unbudgeted", Tone: tone, Amount: unbTotal, Link: "/plan", Items: unb[:min(6, len(unb))],
			Title:  fmt.Sprintf("%s a month has no budget line", eurN(unbTotal)),
			Detail: "This spending counts against left to spend but no line plans for it, so the plan can't warn you about it. Add lines or fold the categories into existing ones."})
	}
	var off []FindingItem
	for _, l := range cur.Lines {
		if l.Kind != "spending" || l.Fund || l.Period != "monthly" {
			continue
		}
		var hist []money.Cents
		for _, c := range l.History {
			if complete[c.Month] && c.Month >= time.Date(now.Year(), now.Month()-6, 1, 0, 0, 0, 0, time.UTC).Format("2006-01") {
				hist = append(hist, c.Spent)
			}
		}
		if len(hist) < 3 {
			continue
		}
		usual, budget := medianCents(hist), l.MonthlyShare
		diff := usual - budget
		if diff.Abs() >= money.FromFloat(40) && (budget == 0 || float64(diff.Abs())/float64(budget) >= 0.25) {
			it := FindingItem{Label: l.Name, Sub: fmt.Sprintf("budget %s · usually %s", eurN(budget), eurN(usual)), Amount: diff, Tone: "bad"}
			if diff < 0 {
				it.Sub, it.Tone = fmt.Sprintf("budget %s · usually %s", eurN(budget), eurN(usual)), "good"
			}
			off = append(off, it)
		}
	}
	if len(off) > 0 {
		sort.SliceStable(off, func(i, j int) bool { return off[i].Amount.Abs() > off[j].Amount.Abs() })
		var net money.Cents
		for _, o := range off {
			net += o.Amount
		}
		s.Findings = append(s.Findings, Finding{Key: "lines_off", Tone: "warn", Amount: net, Link: "/plan", Items: off,
			Title:  fmt.Sprintf("%d budget lines are far from how you spend", len(off)),
			Detail: "Median of the last six months against the line's monthly amount. Lines set below reality make every month look like a failure; lines set above hide money that could be saved."})
	}

	// 4. Saving from income vs the plan — on a usual month (median), since
	// a bonus month lifts the average far above what most months leave.
	if cur.SavingPlanned > 0 {
		usual := medianCents(savedByMonth)
		fd := Finding{Key: "saving", Amount: usual, Link: "/insights/cashflow", Items: shortMonths}
		switch {
		case usual >= cur.SavingPlanned:
			fd.Tone = "good"
			fd.Title = fmt.Sprintf("A usual month leaves %s to save — above the %s planned", eurN(usual), eurN(cur.SavingPlanned))
		default:
			fd.Tone = "warn"
			if usual < cur.SavingPlanned/2 {
				fd.Tone = "bad"
			}
			fd.Title = fmt.Sprintf("A usual month leaves %s to save; the plan sets aside %s", eurN(usual), eurN(cur.SavingPlanned))
		}
		if f.Saved-usual >= money.FromFloat(200) {
			fd.Detail = fmt.Sprintf("The %d-month average is %s, lifted by months with extra income (a bonus, side income). ", n, eurN(f.Saved))
		}
		if f.Invested > f.Saved && f.Saved > 0 {
			fd.Detail += fmt.Sprintf("You moved %s a month into investments, pension and loan principal — %s more than income left, so the rest came from savings or one-off money. ", eurN(f.Invested), eurN(f.Invested-f.Saved))
		}
		if len(shortMonths) > 0 {
			fd.Detail += fmt.Sprintf("In %d of %d months spending was higher than income.", len(shortMonths), n)
		}
		s.Findings = append(s.Findings, fd)
	}

	// 5. Categories rising: the last three months against the three before.
	from3 := time.Date(now.Year(), now.Month()-3, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	var rising []FindingItem
	var risingTotal money.Cents
	for _, c := range insights.Breakdown(txs, from3, toDay) {
		delta := (c.Total - c.Previous) / 3
		if c.Previous > 0 && c.Change >= 0.25 && delta >= money.FromFloat(50) {
			rising = append(rising, FindingItem{Label: catName(cats, c.Category), Sub: fmt.Sprintf("+%.0f%% · %s/mo vs %s", c.Change*100, eurN(c.Total/3), eurN(c.Previous/3)), Amount: delta, Tone: "bad"})
			risingTotal += delta
		}
	}
	if len(rising) > 0 {
		sort.SliceStable(rising, func(i, j int) bool { return rising[i].Amount > rising[j].Amount })
		s.Findings = append(s.Findings, Finding{Key: "rising", Tone: "warn", Amount: risingTotal, Link: "/insights/spending", Items: rising[:min(5, len(rising))],
			Title:  fmt.Sprintf("Spending up %s a month in %d categories", eurN(risingTotal), len(rising)),
			Detail: "The last three complete months against the three before them, one-offs included — check whether it is a one-off or a new habit."})
	}

	// 6. Bills and subscriptions.
	if rec, _, err := insights.RecurringCosts(d, txs, now); err == nil {
		var bills []FindingItem
		var total money.Cents
		for _, r := range rec {
			// Obligations (alimony, the loan) are fixed costs, not bills to review.
			if (r.Kind != "" && r.Kind != "bill") || isFixed(&ledger.Tx{Kind: "expense", Category: r.Category, Merchant: r.Merchant}) {
				continue
			}
			total += r.Monthly
			bills = append(bills, FindingItem{Label: r.Merchant, Sub: r.Cadence + " · next " + r.Next, Amount: r.Monthly})
		}
		if total > 0 {
			sort.SliceStable(bills, func(i, j int) bool { return bills[i].Amount > bills[j].Amount })
			s.Findings = append(s.Findings, Finding{Key: "bills", Tone: "info", Amount: total, Link: "/plan/recurring", Items: bills[:min(6, len(bills))],
				Title:  fmt.Sprintf("Bills and subscriptions: %s a month across %d", eurN(total), len(bills)),
				Detail: fmt.Sprintf("That is %.0f%% of income before anything else is spent. Review the small ones once a year — they rarely get cancelled on their own.", 100*float64(total)/float64(max(f.Income, 1)))})
		}
	}

	// 7. Emergency fund.
	if book, err := wealth.LoadBook(d); err == nil {
		e := insights.ComputeFI(txs, cats, book, settings, now).Emergency
		if e.MonthlyEssential > 0 {
			fd := Finding{Key: "emergency", Amount: e.Cash, Link: "/wealth/fi",
				Title:  fmt.Sprintf("Cash covers %.1f months of essentials", e.Months),
				Detail: fmt.Sprintf("Essentials run %s a month; the target is %.0f months (%s).", eurN(e.MonthlyEssential), e.TargetMonths, eurN(e.Target))}
			fd.Tone = "good"
			if e.Months < e.TargetMonths {
				fd.Tone = "warn"
				fd.Detail += fmt.Sprintf(" %s more would reach it.", eurN(e.Target-e.Cash))
			}
			s.Findings = append(s.Findings, fd)
		}
	}

	sort.SliceStable(s.Findings, func(i, j int) bool { return toneRank[s.Findings[i].Tone] < toneRank[s.Findings[j].Tone] })
	return s, nil
}

func medianCents(v []money.Cents) money.Cents {
	if len(v) == 0 {
		return 0
	}
	s := append([]money.Cents(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// eurN is eurS keeping the sign: −€120 when money ran short.
func eurN(c money.Cents) string {
	if c < 0 {
		return "−" + eurS(c)
	}
	return eurS(c)
}

func monthName(ym string) string {
	t, err := time.Parse("2006-01", ym)
	if err != nil {
		return ym
	}
	return t.Format("Jan 2006")
}

func catName(cats map[string]ledger.Category, id string) string {
	if c, ok := cats[id]; ok && c.Name != "" {
		if p, ok := cats[ledger.Top(id)]; ok && ledger.Top(id) != id {
			return p.Name + " · " + c.Name
		}
		return c.Name
	}
	return id
}

func txLabel(t ledger.Tx) string {
	switch {
	case t.Merchant != "" && t.Note != "" && strings.HasPrefix(strings.ToLower(t.Note), strings.ToLower(t.Merchant)):
		return t.Note
	case t.Merchant != "" && t.Note != "":
		return t.Merchant + " — " + t.Note
	case t.Merchant != "":
		return t.Merchant
	case t.Note != "":
		return t.Note
	}
	return "Payment"
}
