package cfo

import (
	"database/sql"
	"sort"
	"strings"
	"time"

	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

// Cash until payday: the plan counts money by the month it belongs to; this
// counts it by the day it moves, per account. Each everyday account starts
// from its balance and walks to the next payday through what is due from it
// (obligations, bills, standing orders, planned transfers) and an allowance
// for day-to-day spending; where it would go below zero it says how much to
// move, from where, and by when. Read-only: it never writes anything.

type CashPlan struct {
	Payday        string        `json:"payday"`         // next salary, by the salary rules ("" = unknown)
	SalaryAccount string        `json:"salary_account"` // where it lands
	Accounts      []CashAccount `json:"accounts"`
	Moves         []CashMove    `json:"moves"`      // suggested top-ups
	ToSavings     []CashMove    `json:"to_savings"` // idle money that could earn
	Short         money.Cents   `json:"short"`      // total still missing after the moves (savings not enough)
	Cash          money.Cents   `json:"cash"`       // physical cash on hand — can be deposited if savings fall short
	DailyAccount  string        `json:"daily_account"`
	Daily         money.Cents   `json:"daily"` // day-to-day allowance per day used in the forecast
}

type CashAccount struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Balance     money.Cents `json:"balance"`
	BalanceDate string      `json:"balance_date"`
	Items       []CashItem  `json:"items"`
	End         money.Cents `json:"end"`      // balance the day before payday
	Low         money.Cents `json:"low"`      // lowest balance on the way
	LowDate     string      `json:"low_date"` //
	Needed      money.Cents `json:"needed"`   // to stay at or above zero
	NeededBy    string      `json:"needed_by"`
	// Top-ups staged as late as possible: savings keep earning until each
	// big payment (and each new month of day-to-day spending) needs them.
	Stages []CashStage `json:"stages"`
	// The floor the account is kept at: a heavy week of its own unplanned
	// spending (90th percentile of 7-day totals, last 6 months), or the
	// owner's own figure.
	Buffer     money.Cents `json:"buffer"`
	BufferAuto bool        `json:"buffer_auto"`
	BufferWhy  string      `json:"buffer_why"`
	// Idle: what stays above the buffer all the way to payday — could earn in
	// savings instead.
	Idle money.Cents `json:"idle"`
}

type CashStage struct {
	Amount money.Cents `json:"amount"`
	By     string      `json:"by"`  // move it by this day (the day before it would go short)
	For    string      `json:"for"` // what it pays for
}

// bigPayment is the size from which a payment gets its own top-up.
const bigPayment = 100 * 100

// oneOff: single payments this large are one-offs, not what a buffer is for.
const oneOff = 250 * 100

type CashItem struct {
	Date   string      `json:"date"`
	Label  string      `json:"label"`
	Kind   string      `json:"kind"`   // obligation | bill | transfer | income | spending
	Amount money.Cents `json:"amount"` // signed: + in, − out
	// Within is true for small bills paid from the everyday account: the
	// day-to-day allowance already covers them, so they are shown, not
	// subtracted again.
	Within bool `json:"within,omitempty"`
}

type CashMove struct {
	From     string      `json:"from"`
	FromName string      `json:"from_name"`
	To       string      `json:"to"`
	ToName   string      `json:"to_name"`
	Amount   money.Cents `json:"amount"`
	By       string      `json:"by"`
	For      string      `json:"for"`
}

// nextPayday is the first day after today on which, under the salary rule in
// force that day, salary is due (its "paid by" day). "" when no rule says.
func nextPayday(rules []plan.SalaryRule, now time.Time) string {
	rules = append([]plan.SalaryRule(nil), rules...)
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].From < rules[j].From })
	for i := 1; i <= 70; i++ {
		d := now.AddDate(0, 0, i)
		ds := d.Format("2006-01-02")
		day := 0
		for _, r := range rules { // the latest rule that has started
			if r.From == "" || r.From <= ds {
				day = r.PaidByDay
			}
		}
		if day > 0 && d.Day() == day {
			return ds
		}
	}
	return ""
}

func within(cat string, cats []string) bool {
	for _, c := range cats {
		if cat == c || strings.HasPrefix(cat, c+".") {
			return true
		}
	}
	return false
}

// usualAccount is the account most of the matching rows came from (last 90 days).
func usualAccount(txs []ledger.Tx, now time.Time, match func(*ledger.Tx) bool) string {
	since := now.AddDate(0, 0, -90).Format("2006-01-02")
	n := map[string]money.Cents{}
	for i := range txs {
		t := &txs[i]
		if t.Date >= since && t.AccountID != "" && match(t) {
			n[t.AccountID] += t.Amount
		}
	}
	best := ""
	for a, v := range n {
		if v > n[best] || (v == n[best] && a < best) {
			best = a
		}
	}
	return best
}

func buildCashPlan(d *sql.DB, txs []ledger.Tx, p *PlanPulse, fixedCats [][]string, book *wealth.Book, now time.Time) *CashPlan {
	settings := plan.LoadSettings(d)
	today := now.Format("2006-01-02")
	cp := &CashPlan{Payday: nextPayday(settings.Salary(), now), Accounts: []CashAccount{}, Moves: []CashMove{}, ToSavings: []CashMove{}}
	if cp.Payday == "" {
		return cp
	}
	accts, err := ledger.ListAccounts(d)
	if err != nil {
		return cp
	}
	byID := map[string]ledger.Account{}
	for _, a := range accts {
		byID[a.ID] = a
	}
	balance := func(id string) (money.Cents, string, bool) {
		if pt, ok := book.At(id, today); ok {
			return pt.Value, pt.Date, true
		}
		return 0, "", false
	}
	everyday := func(id string) bool { // money you can move by bank transfer
		a, ok := byID[id]
		return ok && !a.Archived && (a.Kind == "checking" || a.Kind == "savings")
	}
	cp.SalaryAccount = settings.SalaryAccount
	if cp.SalaryAccount == "" { // where the last salary went
		last := ""
		for i := range txs {
			t := &txs[i]
			if t.Kind == "income" && ledger.Top(t.Category) == "salary" && t.AccountID != "" && t.Date >= last {
				last, cp.SalaryAccount = t.Date, t.AccountID
			}
		}
	}

	items := map[string][]CashItem{}
	add := func(acct, date, label, kind string, amt money.Cents) {
		if acct == "" || amt == 0 || date < today || date >= cp.Payday {
			return
		}
		items[acct] = append(items[acct], CashItem{Date: date, Label: label, Kind: kind, Amount: amt})
	}
	// 1. Fixed obligations still to pay, from the account that usually pays them.
	for i, f := range p.Fixed {
		if f.Spent >= f.Budgeted || f.Due == "" || i >= len(fixedCats) {
			continue
		}
		cats := fixedCats[i]
		acct := usualAccount(txs, now, func(t *ledger.Tx) bool { return t.Kind != "income" && within(t.Category, cats) })
		add(acct, f.Due, f.Name, "obligation", -(f.Budgeted - f.Spent))
	}
	// 2. Recurring bills, standing orders, planned transfers and income — every
	// occurrence before payday; those inside an obligation are counted there.
	var allCats []string
	for _, c := range fixedCats {
		allCats = append(allCats, c...)
	}
	cp.DailyAccount = usualAccount(txs, now, func(t *ledger.Tx) bool { return t.Kind == "expense" && !within(t.Category, allCats) })
	if rec, _, err := insights.RecurringAll(d, txs, now); err == nil {
		for _, r := range rec {
			if r.Next == "" || within(r.Category, allCats) {
				continue
			}
			for date, n := r.Next, 0; date != "" && date < cp.Payday && n < 3; n++ {
				switch r.Kind {
				case "bill":
					add(firstNonEmpty(r.FromAccount, cp.DailyAccount), date, r.Merchant, "bill", -r.Amount)
				case "transfer":
					if everyday(r.FromAccount) {
						add(r.FromAccount, date, transferLabel(r.Merchant, "to", byID[r.ToAccount].Name), "transfer", -r.Amount)
					}
					if everyday(r.ToAccount) {
						add(r.ToAccount, date, transferLabel(r.Merchant, "from", byID[r.FromAccount].Name), "transfer", r.Amount)
					}
				case "income":
					add(r.ToAccount, date, r.Merchant, "income", r.Amount)
				}
				date = nextOccurrence(r, date)
			}
		}
	}
	// 3. Day-to-day spending at the plan's daily allowance, from the account
	// most of it comes from.
	if p.PerDayLeft > 0 && everyday(cp.DailyAccount) {
		cp.Daily = p.PerDayLeft
	}

	var spentToday money.Cents
	for i := range txs {
		if t := &txs[i]; t.Date == today && t.Kind == "expense" && t.AccountID == cp.DailyAccount && !within(t.Category, allCats) && t.Amount < oneOff {
			spentToday += t.Amount
		}
	}

	ids := map[string]bool{cp.SalaryAccount: everyday(cp.SalaryAccount), cp.DailyAccount: cp.Daily > 0}
	for id := range items {
		ids[id] = true
	}
	for id, ok := range ids {
		if !ok || !everyday(id) {
			continue
		}
		a := byID[id]
		ca := CashAccount{ID: id, Name: a.Name, Items: items[id]}
		if id == cp.DailyAccount && cp.Daily > 0 {
			for k := range ca.Items {
				ca.Items[k].Within = ca.Items[k].Kind == "bill"
			}
		}
		ca.Balance, ca.BalanceDate, _ = balance(id)
		if v, ok := settings.Buffers[id]; ok && v > 0 {
			ca.Buffer, ca.BufferWhy = money.FromFloat(v), "your setting"
		} else {
			var allowance money.Cents
			if id == cp.DailyAccount {
				allowance = cp.Daily
			}
			ca.Buffer, ca.BufferWhy = autoBuffer(txs, id, allCats, allowance, now)
			ca.BufferAuto = true
		}
		sort.SliceStable(ca.Items, func(i, j int) bool { return ca.Items[i].Date < ca.Items[j].Date })
		// Walk day by day: items, plus the allowance on the spending account.
		type dayBal struct {
			date  string
			bal   money.Cents
			big   []string // big payments that day
			split bool     // a new segment starts (big payment, or a new month)
		}
		var walk []dayBal
		bal, low, lowDate := ca.Balance, ca.Balance, today
		var spend money.Cents
		for day := now; day.Format("2006-01-02") < cp.Payday; day = day.AddDate(0, 0, 1) {
			ds := day.Format("2006-01-02")
			db := dayBal{date: ds, split: day.Day() == 1 && ds != today}
			for _, it := range ca.Items {
				if it.Date == ds && !it.Within {
					bal += it.Amount
					if it.Amount <= -bigPayment {
						db.big = append(db.big, strings.TrimPrefix(it.Label, "to "))
						db.split = true
					}
				}
			}
			if id == cp.DailyAccount && cp.Daily > 0 {
				d := cp.Daily
				if ds == today {
					// What was spent today already left the balance (the bank
					// states it after card holds); only the rest of today's
					// allowance is still to come.
					d -= spentToday
					if d < 0 {
						d = 0
					}
				}
				bal -= d
				spend += d
			}
			if bal < low {
				low, lowDate = bal, ds
			}
			if bal < ca.Buffer && ca.NeededBy == "" {
				ca.NeededBy = ds
			}
			db.bal = bal
			walk = append(walk, db)
		}
		// Stages: per segment, top up just enough — by the day before the
		// balance would first dip below zero — to last until the next one.
		var topped money.Cents
		for i := 0; i < len(walk); {
			j := i + 1
			for j < len(walk) && !walk[j].split {
				j++
			}
			lowest := ca.Buffer // keep the account at or above its buffer
			first := ""
			for k := i; k < j; k++ {
				if b := walk[k].bal + topped; b < lowest {
					lowest = b
					if first == "" {
						first = walk[k].date
					}
				}
			}
			if lowest < ca.Buffer {
				amt := roundUp10(ca.Buffer - lowest)
				topped += amt
				by := first
				if t, err := time.Parse("2006-01-02", first); err == nil && first > today {
					by = t.AddDate(0, 0, -1).Format("2006-01-02")
				}
				var what []string
				for k := i; k < j; k++ {
					what = append(what, walk[k].big...)
				}
				if id == cp.DailyAccount && cp.Daily > 0 {
					what = append(what, "day-to-day spending to "+shortDay(walk[j-1].date))
				}
				if len(what) > 0 {
					what[0] = strings.ToUpper(what[0][:1]) + what[0][1:]
				}
				ca.Stages = append(ca.Stages, CashStage{Amount: amt, By: by, For: strings.Join(what, ", ")})
			}
			i = j
		}
		if spend > 0 {
			ca.Items = append(ca.Items, CashItem{Date: today, Label: "Day-to-day spending until payday", Kind: "spending", Amount: -spend})
		}
		ca.End, ca.Low, ca.LowDate = bal, low, lowDate
		if low < ca.Buffer {
			ca.Needed = roundUp10(ca.Buffer - low)
		} else if a.Kind == "checking" {
			ca.Idle = (low - ca.Buffer) / 5000 * 5000 // whole €50s that stay above the buffer to payday
		}
		cp.Accounts = append(cp.Accounts, ca)
	}
	sort.Slice(cp.Accounts, func(i, j int) bool {
		if (cp.Accounts[i].Needed > 0) != (cp.Accounts[j].Needed > 0) {
			return cp.Accounts[i].Needed > 0
		}
		return cp.Accounts[i].Name < cp.Accounts[j].Name
	})
	planMoves(cp, accts, everyday, balance)
	for _, a := range accts {
		if a.Kind == "cash" && !a.Archived {
			if v, _, ok := balance(a.ID); ok && v > 0 {
				cp.Cash += v
			}
		}
	}
	// Idle money: from an everyday account to savings at the same bank (or
	// another savings account) — it stays above the buffer until payday.
	for _, a := range cp.Accounts {
		if a.Idle < 50*100 {
			continue
		}
		var best ledger.Account
		for _, s := range accts {
			if s.Kind != "savings" || s.Archived {
				continue
			}
			if best.ID == "" || (s.Institution == byID[a.ID].Institution && best.Institution != byID[a.ID].Institution) {
				best = s
			}
		}
		if best.ID != "" {
			cp.ToSavings = append(cp.ToSavings, CashMove{From: a.ID, FromName: a.Name, To: best.ID, ToName: best.Name, Amount: a.Idle, By: today,
				For: "stays above the " + eur(a.Buffer) + " buffer until payday"})
		}
	}
	return cp
}

// autoBuffer sizes an everyday account's floor from its own history: a
// heavy-but-ordinary week — the 75th percentile of its 7-day spending over the
// last 6 months, leaving out obligations (planned and staged) and single
// payments of €250 or more (one-offs belong to the plan and its funds) —
// rounded up to €50; at least €50 for fees and card holds.
func autoBuffer(txs []ledger.Tx, id string, planned []string, allowance money.Cents, now time.Time) (money.Cents, string) {
	since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, -6, 0)
	days := int(now.Sub(since).Hours()/24) + 1
	daily := make([]money.Cents, days)
	for i := range txs {
		t := &txs[i]
		if t.AccountID != id || t.Kind != "expense" || within(t.Category, planned) || t.Amount >= oneOff {
			continue
		}
		d, err := time.Parse("2006-01-02", t.Date)
		if err != nil || d.Before(since) || !d.Before(now) {
			continue
		}
		if k := int(d.Sub(since).Hours() / 24); k >= 0 && k < days {
			daily[k] += t.Amount
		}
	}
	var weeks []money.Cents
	var sum money.Cents
	for i, v := range daily {
		sum += v
		if i >= 7 {
			sum -= daily[i-7]
		}
		if i >= 6 {
			weeks = append(weeks, sum)
		}
	}
	const floor = 50 * 100
	if len(weeks) == 0 {
		return floor, "minimum for fees and card holds"
	}
	sort.Slice(weeks, func(i, j int) bool { return weeks[i] < weeks[j] })
	p75 := weeks[len(weeks)*3/4]
	why := "a busy week of everyday spending from it (one-offs over €250 left out)"
	if allowance > 0 {
		// The forecast already spends the daily allowance every day; the
		// buffer only has to absorb a week busier than that pace.
		p75 -= 7 * allowance
		why = "how far a busy week runs past your daily allowance (one-offs over €250 left out)"
	}
	if p75 <= floor {
		return floor, "minimum for fees and card holds"
	}
	return (p75 + 5000 - 1) / 5000 * 5000, why
}

// nextOccurrence steps a recurring item to its following date.
func nextOccurrence(r insights.Recurring, date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return ""
	}
	switch {
	case r.EveryDays > 0:
		return t.AddDate(0, 0, r.EveryDays).Format("2006-01-02")
	case r.Cadence == "quarterly":
		return t.AddDate(0, 3, 0).Format("2006-01-02")
	case r.Cadence == "yearly":
		return t.AddDate(1, 0, 0).Format("2006-01-02")
	}
	return t.AddDate(0, 1, 0).Format("2006-01-02")
}

func roundUp10(c money.Cents) money.Cents {
	const ten = 1000 // €10 in cents
	return (c + ten - 1) / ten * ten
}

// planMoves covers each staged top-up, earliest first, from savings: the same
// bank first, then one account covering the rest when one can, otherwise
// split, largest source first. Sources keep what they need themselves;
// amounts are whole €10s where the source allows. What savings can't cover is
// reported as Short.
func planMoves(cp *CashPlan, accts []ledger.Account, everyday func(string) bool, balance func(string) (money.Cents, string, bool)) {
	spare := map[string]money.Cents{}
	inPlan := map[string]CashAccount{}
	for _, a := range cp.Accounts {
		inPlan[a.ID] = a
	}
	var sources []ledger.Account
	for _, a := range accts {
		b, _, ok := balance(a.ID)
		if !everyday(a.ID) || !ok || a.Kind != "savings" { // top-ups come from savings only
			continue
		}
		if ca, ok := inPlan[a.ID]; ok {
			b = ca.Low - ca.Buffer // what it can give and still keep its own buffer
		}
		if b = b / 1000 * 1000; b > 0 { // whole €10s
			spare[a.ID] = b
			sources = append(sources, a)
		}
	}
	type stage struct {
		CashStage
		acct string
	}
	var stages []stage
	for _, a := range cp.Accounts {
		for _, st := range a.Stages {
			stages = append(stages, stage{st, a.ID})
		}
	}
	sort.SliceStable(stages, func(i, j int) bool { return stages[i].By < stages[j].By })
	for _, need := range stages {
		to := accountByID(accts, need.acct)
		rank := func(a ledger.Account) (bool, bool) { return a.Kind == "savings", a.Institution == to.Institution }
		sort.SliceStable(sources, func(i, j int) bool {
			si, bi := rank(sources[i])
			sj, bj := rank(sources[j])
			if si != sj {
				return si
			}
			if bi != bj {
				return bi
			}
			return spare[sources[i].ID] > spare[sources[j].ID]
		})
		move := func(s ledger.Account, amt money.Cents) {
			spare[s.ID] -= amt
			cp.Moves = append(cp.Moves, CashMove{From: s.ID, FromName: s.Name, To: to.ID, ToName: to.Name, Amount: amt, By: need.By, For: need.For})
		}
		left := need.Amount
		for _, s := range sources { // savings at the same bank first: moves there are instant and free
			if left > 0 && s.ID != to.ID && s.Kind == "savings" && s.Institution == to.Institution && spare[s.ID] > 0 {
				amt := min(left, spare[s.ID])
				move(s, amt)
				left -= amt
			}
		}
		for _, s := range sources { // then one source covering the rest
			if left > 0 && s.ID != to.ID && spare[s.ID] >= left {
				move(s, left)
				left = 0
				break
			}
		}
		for _, s := range sources { // otherwise split
			if left <= 0 {
				break
			}
			if s.ID == to.ID || spare[s.ID] <= 0 {
				continue
			}
			amt := min(left, spare[s.ID])
			move(s, amt)
			left -= amt
		}
		cp.Short += left
	}
	sort.SliceStable(cp.Moves, func(i, j int) bool { return cp.Moves[i].By < cp.Moves[j].By })
}

// transferLabel names one side of a transfer without repeating the account
// ("Artea" to "Artea (III pillar)" reads "to Artea (III pillar)").
func transferLabel(name, dir, other string) string {
	if other == "" || strings.Contains(strings.ToLower(other), strings.ToLower(name)) || strings.Contains(strings.ToLower(name), strings.ToLower(other)) {
		return dir + " " + firstNonEmpty(other, name)
	}
	return name + " " + dir + " " + other
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func accountByID(accts []ledger.Account, id string) ledger.Account {
	for _, a := range accts {
		if a.ID == id {
			return a
		}
	}
	return ledger.Account{}
}
