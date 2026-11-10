package cfo

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/wealth"
)

// A stress test asks "what if a crisis hit next month?" — a lost job, a
// long illness, a market crash, a rate rise, prices like 2022, a big bill.
// It takes the cash forecast as it stands (the same income, obligations,
// saving, everyday spending and dated bills) and changes it the way the
// crisis would, so "how bad is it" is the plan's own arithmetic. When cash
// would go below zero it sells what can be sold — investments, then
// crypto, then the III pillar after tax — and says when even that runs out.
// The II pillar, property and the car are left alone: they can't pay a
// bill next month.

// Shock is one crisis. Percentages change the plan's own figures; a zero
// field leaves that figure as it is. The crisis starts next month; Months
// is how long income, spending and saving stay changed (0 = the whole
// period). Market drops happen at once and don't recover within the period;
// a rate rise stays.
type Shock struct {
	Months      int     `json:"months,omitempty"`       // how long the crisis lasts
	IncomePct   float64 `json:"income_pct,omitempty"`   // −100 = no pay at all
	Benefit     float64 `json:"benefit,omitempty"`      // € a month coming in on top while it lasts (unemployment or sick pay)
	EverydayPct float64 `json:"everyday_pct,omitempty"` // +20 = prices up 20%, −20 = cutting back
	FixedPct    float64 `json:"fixed_pct,omitempty"`    // obligations (alimony, loan, rent)
	OneOff      float64 `json:"one_off,omitempty"`      // an unexpected bill in the first month, €
	RatePP      float64 `json:"rate_pp,omitempty"`      // base rate up on variable loans, percentage points
	StocksPct   float64 `json:"stocks_pct,omitempty"`   // brokerage accounts
	CryptoPct   float64 `json:"crypto_pct,omitempty"`
	PensionPct  float64 `json:"pension_pct,omitempty"` // II and III pillar funds
	PropertyPct float64 `json:"property_pct,omitempty"`
	PauseSaving bool    `json:"pause_saving,omitempty"` // stop saving and investing while it lasts
	PauseGoals  bool    `json:"pause_goals,omitempty"`  // put off wish-list goals dated while it lasts
}

// Validate keeps a shock inside what could happen; the message reaches the
// owner as it is.
func (s Shock) Validate(horizon int) error {
	in := func(v, lo, hi float64) bool { return v >= lo && v <= hi && !math.IsNaN(v) }
	switch {
	case s.Months < 0 || s.Months > horizon:
		return fmt.Errorf("a crisis lasts 0 to %d months", horizon)
	case !in(s.IncomePct, -100, 100):
		return errors.New("income can change by −100% to +100%")
	case !in(s.Benefit, 0, 100000), !in(s.OneOff, 0, 10000000):
		return errors.New("amounts must be between €0 and €10,000,000")
	case !in(s.EverydayPct, -90, 200), !in(s.FixedPct, -90, 200):
		return errors.New("spending can change by −90% to +200%")
	case !in(s.RatePP, -5, 10):
		return errors.New("the rate can move −5 to +10 points")
	case !in(s.StocksPct, -100, 100), !in(s.CryptoPct, -100, 100), !in(s.PensionPct, -100, 100), !in(s.PropertyPct, -100, 100):
		return errors.New("values can change by −100% to +100%")
	}
	return nil
}

// pillar3Tax is what is withheld when the III pillar is cashed out early
// (the tax relief given back) — an estimate: the exact figure depends on
// the contract.
const pillar3Tax = 0.15

// StressPreset is a crisis the page offers ready-made.
type StressPreset struct {
	ID    string
	Group string // income · markets · costs · combined: how the page groups them
	Name  string
	Blurb string
	Shock Shock
}

// StressGroups name the groups, in page order.
var StressGroups = []StressGroup{
	{"income", "Losing income"}, {"markets", "Markets falling"}, {"costs", "Costs rising"}, {"combined", "Several at once"},
}

// StressPresets are the ready-made crises: each one a thing that happens
// to households, at a size taken from the last decades (1970s inflation,
// 2008's markets and house prices in Lithuania, 2022's prices and rates,
// crypto's 2018 and 2022 winters).
var StressPresets = []StressPreset{
	// Losing income
	{ID: "job_loss", Group: "income", Name: "Job loss",
		Blurb: "No pay for 6 months while you look for work. Saving and wish-list goals paused. Unemployment benefit isn't counted — add it in Build your own.",
		Shock: Shock{Months: 6, IncomePct: -100, PauseSaving: true, PauseGoals: true}},
	{ID: "long_search", Group: "income", Name: "Long job search",
		Blurb: "A whole year without pay — a senior role in a slow market can take that long. Saving and wish-list goals paused.",
		Shock: Shock{Months: 12, IncomePct: -100, PauseSaving: true, PauseGoals: true}},
	{ID: "pay_cut", Group: "income", Name: "Pay cut",
		Blurb: "20% less pay for a year — short hours, a lost bonus or a lower-paid job. Saving goes on.",
		Shock: Shock{Months: 12, IncomePct: -20}},
	{ID: "illness", Group: "income", Name: "Long illness",
		Blurb: "6 months on sick pay — about 40% less coming in — and €1,500 of treatment. Wish-list goals paused, saving goes on.",
		Shock: Shock{Months: 6, IncomePct: -40, OneOff: 1500, PauseGoals: true}},
	{ID: "parental", Group: "income", Name: "Parental leave",
		Blurb: "A year on parental benefit — about 30% less pay — with everyday costs up 15% and €2,500 for the things a baby needs.",
		Shock: Shock{Months: 12, IncomePct: -30, EverydayPct: 15, OneOff: 2500, PauseGoals: true}},
	// Markets falling
	{ID: "market_crash", Group: "markets", Name: "Market crash",
		Blurb: "Markets fall like 2008: stocks −40%, crypto −65%, pension funds −25%. Pay goes on, so cash is fine — it is what you own that shrinks.",
		Shock: Shock{StocksPct: -40, CryptoPct: -65, PensionPct: -25}},
	{ID: "crypto_winter", Group: "markets", Name: "Crypto winter",
		Blurb: "Crypto loses 80% as in 2018 and 2022, and stocks slip 15% with it.",
		Shock: Shock{CryptoPct: -80, StocksPct: -15}},
	{ID: "housing_slump", Group: "markets", Name: "Housing slump",
		Blurb: "House prices fall 25%, as in Lithuania after 2008, while Euribor rises 2 points — less equity and dearer loans.",
		Shock: Shock{PropertyPct: -25, RatePP: 2}},
	// Costs rising
	{ID: "rates", Group: "costs", Name: "Rate rise",
		Blurb: "Euribor up 3 points: variable-rate loan payments rise from the next reset and stay up.",
		Shock: Shock{RatePP: 3}},
	{ID: "inflation", Group: "costs", Name: "Inflation like 2022",
		Blurb: "Everyday prices up 20% for good and Euribor up 2 points, while pay stays where it is.",
		Shock: Shock{EverydayPct: 20, RatePP: 2}},
	{ID: "big_bill", Group: "costs", Name: "Big bill",
		Blurb: "€10,000 due next month — a roof, a car, a health bill.",
		Shock: Shock{OneOff: 10000}},
	// Several at once
	{ID: "recession", Group: "combined", Name: "Deep recession",
		Blurb: "The double hit: 9 months without pay while stocks fall 35%, crypto 60%, pensions 20% and house prices 15% — the worst time to sell.",
		Shock: Shock{Months: 9, IncomePct: -100, StocksPct: -35, CryptoPct: -60, PensionPct: -20, PropertyPct: -15, PauseSaving: true, PauseGoals: true}},
	{ID: "stagflation", Group: "combined", Name: "Stagflation",
		Blurb: "The 1970s: prices up 15% and Euribor up 4 points while pay falls 10% and stocks lose 30%, pensions 15%.",
		Shock: Shock{IncomePct: -10, EverydayPct: 15, RatePP: 4, StocksPct: -30, PensionPct: -15}},
	{ID: "perfect_storm", Group: "combined", Name: "Everything at once",
		Blurb: "A year without pay, markets like 2008, house prices −20%, Euribor +3 points and a €5,000 bill. Unlikely — but it shows where the floor is.",
		Shock: Shock{Months: 12, IncomePct: -100, StocksPct: -40, CryptoPct: -70, PensionPct: -25, PropertyPct: -20, RatePP: 3, OneOff: 5000, PauseSaving: true, PauseGoals: true}},
}

// StressReport is every ready-made crisis against the plan, plus one the
// owner built (Custom).
type StressReport struct {
	Start     string         `json:"start"`   // first month (this month)
	Horizon   int            `json:"horizon"` // months looked at
	Cash      money.Cents    `json:"cash"`    // checking, savings, cash today
	Floor     money.Cents    `json:"floor"`   // the emergency target
	NetWorth  money.Cents    `json:"net_worth"`
	Reserves  []Reserve      `json:"reserves"` // what could be sold, in the order it would be
	Locked    money.Cents    `json:"locked"`   // II pillar: no use before retirement
	RealAsset money.Cents    `json:"real_assets"`
	Runway    *Runway        `json:"runway,omitempty"` // nil until there is a usual month to go by
	Base      []money.Cents  `json:"base"`             // cash at each month end at the current plan
	Groups    []StressGroup  `json:"groups"`
	Scenarios []StressResult `json:"scenarios"`
	Custom    *StressResult  `json:"custom,omitempty"`
	Notes     []string       `json:"notes"`
}

type StressGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Reserve is money that could be raised within weeks.
type Reserve struct {
	Key   string      `json:"key"` // investments | crypto | pillar3
	Label string      `json:"label"`
	Value money.Cents `json:"value"` // today, before any tax
	Net   money.Cents `json:"net"`   // what selling all of it brings in today
}

// Runway is how long the money lasts with no income at all and saving
// stopped: the usual month's obligations, everyday spending and one-offs.
type Runway struct {
	Burn          money.Cents `json:"burn"`           // a month without income
	CashMonths    float64     `json:"cash_months"`    // on cash alone
	ReserveMonths float64     `json:"reserve_months"` // selling the reserves too
}

// StressResult is one crisis played out month by month.
type StressResult struct {
	ID         string        `json:"id"`
	Group      string        `json:"group,omitempty"`
	Name       string        `json:"name"`
	Blurb      string        `json:"blurb,omitempty"`
	Shock      Shock         `json:"shock"`
	Months     []StressMonth `json:"months,omitempty"`
	Level      string        `json:"level"` // ok · tight (under the emergency target) · sell (reserves needed) · short (they run out too)
	Lowest     money.Cents   `json:"lowest"`
	LowestOn   string        `json:"lowest_on"`
	BelowFloor string        `json:"below_floor,omitempty"` // first month under the emergency target
	CashOut    string        `json:"cash_out,omitempty"`    // first month cash alone would be gone
	Sold       []Sale        `json:"sold,omitempty"`        // reserves sold to stay above zero
	SoldTotal  money.Cents   `json:"sold_total"`            // gross value sold
	Broke      string        `json:"broke,omitempty"`       // first month even the reserves are gone
	Short      money.Cents   `json:"short"`                 // still missing at the end, after everything
	MarketHit  money.Cents   `json:"market_hit"`            // value lost at the drop (positive)
	LoanExtra  money.Cents   `json:"loan_extra"`            // loan payments up a month, once the rate resets
	MonthlyGap money.Cents   `json:"monthly_gap"`           // worst month's net (negative = cash going out)
	NetWorth   money.Cents   `json:"net_worth_end"`         // at the end of the period
	BaseWorth  money.Cents   `json:"base_net_worth_end"`    // the same at the current plan
	Cost       money.Cents   `json:"cost"`                  // net worth lost against the plan
	Summary    []string      `json:"summary"`
	Levers     []Lever       `json:"levers,omitempty"`
}

type StressMonth struct {
	Month  string      `json:"month"`
	Income money.Cents `json:"income"`
	Out    money.Cents `json:"out"`
	Sold   money.Cents `json:"sold,omitempty"` // brought in by selling reserves
	End    money.Cents `json:"end"`            // cash at month end
	Base   money.Cents `json:"base"`           // the same at the current plan
}

type Sale struct {
	Key    string      `json:"key"`
	Label  string      `json:"label"`
	Amount money.Cents `json:"amount"` // value sold
	Net    money.Cents `json:"net"`    // cash it brought in
}

// Lever is something the owner could do about it, worth Amount a month
// (or once, for goals).
type Lever struct {
	Label   string      `json:"label"`
	Amount  money.Cents `json:"amount"`
	Monthly bool        `json:"monthly"`
}

// stressBook is what a crisis acts on besides the forecast: today's
// holdings by kind and the loans.
type stressBook struct {
	netWorth money.Cents
	stocks   money.Cents
	crypto   money.Cents
	pillar2  money.Cents
	pillar3  money.Cents
	property money.Cents
	vehicle  money.Cents
	loans    []wealth.LoanView
	nowMonth string
}

// StressTest runs every ready-made crisis over horizon months (default 24)
// and, when custom is set, the owner's own.
func StressTest(d *sql.DB, now time.Time, horizon int, custom *Shock) (*StressReport, error) {
	if horizon <= 0 || horizon > 36 {
		horizon = 24
	}
	if custom != nil {
		if err := custom.Validate(horizon); err != nil {
			return nil, err
		}
	}
	base, err := ForecastFrom(d, now, horizon, nil)
	if err != nil {
		return nil, err
	}
	book, err := wealth.LoadBook(d)
	if err != nil {
		return nil, err
	}
	txs, err := ledger.All(d, ledger.Filter{From: ledger.AddMonths(now, -13) + "-01"})
	if err != nil {
		return nil, err
	}
	sb := stressBook{loans: wealth.Loans(book, txs, now), nowMonth: now.Format("2006-01")}
	snap := book.SnapshotAt(now.Format("2006-01-02"), true)
	sb.netWorth = snap.NetWorth
	for id, v := range snap.ByAccount {
		a := book.Accounts[id]
		if v <= 0 {
			continue
		}
		switch a.Kind {
		case "brokerage":
			sb.stocks += v
		case "crypto":
			sb.crypto += v
		case "pension":
			if a.Pillar() == 3 {
				sb.pillar3 += v
			} else {
				sb.pillar2 += v
			}
		case "property":
			sb.property += v
		case "vehicle":
			sb.vehicle += v
		}
	}
	return buildStress(base, sb, horizon, custom), nil
}

// buildStress is the arithmetic, apart from the database.
func buildStress(base *Forecast, sb stressBook, horizon int, custom *Shock) *StressReport {
	r := &StressReport{Start: base.Start, Groups: StressGroups, Horizon: horizon, Cash: base.Cash, Floor: base.Floor, NetWorth: sb.netWorth,
		Locked: sb.pillar2, RealAsset: sb.property + sb.vehicle, Base: []money.Cents{}, Scenarios: []StressResult{}}
	r.Reserves = reserves(sb, Shock{})
	for _, m := range base.Months {
		r.Base = append(r.Base, m.End)
	}
	a := base.Assumed
	if burn := a.Fixed + a.Everyday + a.OneOffs; burn > 0 {
		r.Runway = &Runway{Burn: burn}
		var net money.Cents
		for _, res := range r.Reserves {
			net += res.Net
		}
		r.Runway.CashMonths = math.Round(max(0, base.Cash.Float())/r.Runway.Burn.Float()*10) / 10
		r.Runway.ReserveMonths = math.Round(max(0, (base.Cash+net).Float())/r.Runway.Burn.Float()*10) / 10
	}
	for _, p := range StressPresets {
		res := runShock(base, sb, p.ID, p.Name, p.Blurb, p.Shock)
		res.Group = p.Group
		r.Scenarios = append(r.Scenarios, res)
	}
	if custom != nil {
		c := runShock(base, sb, "custom", "Your crisis", "", *custom)
		r.Custom = &c
	}
	r.Notes = []string{
		"Starts from the cash forecast: the plan's income, obligations and saving, usual everyday spending and the bills already dated.",
		"Market drops don't recover within the period and investment returns are left out on both sides, so the cost is the crisis alone.",
		fmt.Sprintf("Reserves are sold only once cash would go below zero: investments first, then crypto, then the III pillar with about %.0f%% tax withheld. The II pillar, property and the car are never sold.", pillar3Tax*100),
	}
	return r
}

// reserves after a shock's market drop, in the order they would be sold.
func reserves(sb stressBook, sh Shock) []Reserve {
	drop := func(v money.Cents, pct float64) money.Cents { return max(0, money.FromFloat(v.Float()*(1+pct/100))) }
	out := []Reserve{}
	if v := drop(sb.stocks, sh.StocksPct); v > 0 {
		out = append(out, Reserve{Key: "investments", Label: "Investments", Value: v, Net: v})
	}
	if v := drop(sb.crypto, sh.CryptoPct); v > 0 {
		out = append(out, Reserve{Key: "crypto", Label: "Crypto", Value: v, Net: v})
	}
	if v := drop(sb.pillar3, sh.PensionPct); v > 0 {
		out = append(out, Reserve{Key: "pillar3", Label: "III pillar", Value: v, Net: money.FromFloat(v.Float() * (1 - pillar3Tax))})
	}
	return out
}

// loanRise is one variable loan's payment with the base rate pp points
// higher: how much more a month, from which month (its next reset, or next
// month when none is known). A loan with no base rate is fixed and
// unaffected.
type loanRise struct {
	from  string
	extra money.Cents
}

func loanRises(loans []wealth.LoanView, pp float64, next string) []loanRise {
	if pp == 0 {
		return nil
	}
	annuity := func(bal, ratePct float64, n int) float64 {
		r := ratePct / 100 / 12
		if r <= 0 {
			return bal / float64(n)
		}
		return bal * r / (1 - math.Pow(1+r, -float64(n)))
	}
	var out []loanRise
	for _, l := range loans {
		if l.Details.BaseRate == 0 || l.MonthsLeft <= 0 || l.Balance <= 0 {
			continue
		}
		bal := l.Balance.Float()
		now := annuity(bal, l.Rate, l.MonthsLeft)
		if l.Details.MonthlyPayment > 0 {
			now = l.Details.MonthlyPayment
		}
		from := next
		if len(l.Details.RateResetDate) >= 7 && l.Details.RateResetDate[:7] > next {
			from = l.Details.RateResetDate[:7]
		}
		out = append(out, loanRise{from: from, extra: money.FromFloat(annuity(bal, max(0, l.Rate+pp), l.MonthsLeft) - now)})
	}
	return out
}

func runShock(base *Forecast, sb stressBook, id, name, blurb string, sh Shock) StressResult {
	res := StressResult{ID: id, Name: name, Blurb: blurb, Shock: sh, Months: []StressMonth{}}
	pct := func(v money.Cents, p float64) money.Cents { return max(0, money.FromFloat(v.Float()*(1+p/100))) }

	// The drop, at once, on what is owned.
	hit := func(v money.Cents, p float64) money.Cents { return v - pct(v, p) }
	res.MarketHit = hit(sb.stocks, sh.StocksPct) + hit(sb.crypto, sh.CryptoPct) + hit(sb.pillar2+sb.pillar3, sh.PensionPct) + hit(sb.property, sh.PropertyPct)
	left := reserves(sb, sh)

	next := ""
	if len(base.Months) > 1 {
		next = base.Months[1].Month
	}
	rises := loanRises(sb.loans, sh.RatePP, next)
	for _, l := range rises {
		res.LoanExtra += l.extra
	}

	cash := base.Cash
	var saved, gross, deferred money.Cents
	sold := map[string]*Sale{}
	res.Lowest, res.LowestOn = cash, base.Start
	gapSet := false
	for i, bm := range base.Months {
		// The crisis starts next month: this month's money is mostly
		// settled already.
		on := i >= 1 && (sh.Months == 0 || i <= sh.Months)
		in, fixed, saving, everyday := bm.Income, bm.Fixed, bm.Saving, bm.Everyday
		var dated money.Cents
		for _, it := range bm.Dated {
			if on && sh.PauseGoals && it.Kind == "goal" {
				deferred += it.Amount // put off, not given up
				continue
			}
			dated += it.Amount
		}
		// Goals put off during the crisis are bought the month it ends.
		if !on && i >= 1 && deferred > 0 {
			dated += deferred
			deferred = 0
		}
		if on {
			in = pct(in, sh.IncomePct) + money.FromFloat(sh.Benefit)
			fixed = pct(fixed, sh.FixedPct)
			everyday = pct(everyday, sh.EverydayPct)
			if sh.PauseSaving {
				saving = 0
			}
		}
		if i == 1 {
			dated += money.FromFloat(sh.OneOff)
		}
		for _, l := range rises {
			if i >= 1 && bm.Month >= l.from {
				fixed += l.extra
			}
		}
		out := fixed + saving + everyday + bm.OneOffs + dated
		net := in - out
		if i >= 1 && (!gapSet || net < res.MonthlyGap) {
			res.MonthlyGap, gapSet = net, true
		}
		cash += net
		saved += saving
		if cash < 0 && res.CashOut == "" {
			res.CashOut = bm.Month
		}
		// Sell to get back to zero, in order.
		var brought money.Cents
		for k := range left {
			if cash >= 0 {
				break
			}
			rv := &left[k]
			if rv.Net <= 0 {
				continue
			}
			need := -cash
			take, value := rv.Net, rv.Value
			if need < rv.Net {
				take = need
				value = money.FromFloat(rv.Value.Float() * take.Float() / rv.Net.Float())
			}
			rv.Net -= take
			rv.Value -= value
			cash += take
			brought += take
			gross += value
			s := sold[rv.Key]
			if s == nil {
				s = &Sale{Key: rv.Key, Label: rv.Label}
				sold[rv.Key] = s
			}
			s.Amount += value
			s.Net += take
		}
		if cash < 0 && res.Broke == "" {
			res.Broke = bm.Month
		}
		res.Months = append(res.Months, StressMonth{Month: bm.Month, Income: in, Out: out, Sold: brought, End: cash, Base: bm.End})
		if cash < res.Lowest {
			res.Lowest, res.LowestOn = cash, bm.Month
		}
		if res.BelowFloor == "" && base.Floor > 0 && cash < base.Floor {
			res.BelowFloor = bm.Month
		}
	}
	for _, k := range []string{"investments", "crypto", "pillar3"} {
		if s := sold[k]; s != nil {
			res.Sold = append(res.Sold, *s)
		}
	}
	res.SoldTotal = gross
	// The deepest it went, not where it ends: pay coming back can refill
	// cash after months of owing money.
	res.Short = max(0, -res.Lowest)

	// Net worth at the end: what is owned after the drop, plus what cash
	// did, plus what was saved into investments, less what was sold
	// (selling the III pillar early loses the tax). A shortfall is owed to
	// someone, so it counts against it too.
	end := base.Months[len(base.Months)-1].End
	res.BaseWorth = sb.netWorth + (end - base.Cash) + base.Invested
	// Goals still put off at the end are spending to come, as in the plan.
	res.NetWorth = sb.netWorth - res.MarketHit + (cash - base.Cash) + saved - gross - deferred
	res.Cost = max(0, res.BaseWorth-res.NetWorth)

	switch {
	case res.Broke != "":
		res.Level = "short"
	case res.CashOut != "":
		res.Level = "sell"
	case res.BelowFloor != "":
		res.Level = "tight"
	default:
		res.Level = "ok"
	}
	res.Summary = stressSummary(base, res)
	res.Levers = stressLevers(base, res, sh)
	return res
}

func stressSummary(base *Forecast, r StressResult) []string {
	var s []string
	switch r.Level {
	case "short":
		s = append(s, fmt.Sprintf("Cash runs out in %s and the reserves are gone by %s — %s short at worst.", monthName(r.CashOut), monthName(r.Broke), eurS(r.Short)))
	case "sell":
		s = append(s, fmt.Sprintf("Cash runs out in %s; selling %s of reserves gets you through.", monthName(r.CashOut), eurS(r.SoldTotal)))
	case "tight":
		line := fmt.Sprintf("Cash drops below your %s emergency target in %s (lowest %s in %s) but never runs out", eurS(base.Floor), monthName(r.BelowFloor), eurN(r.Lowest), monthName(r.LowestOn))
		// The plan may dip under the target on its own; say so rather than
		// blame the crisis for it.
		if base.BelowFloor != "" && base.BelowFloor <= r.BelowFloor {
			line += " — the plan dips there too"
		}
		s = append(s, line+".")
	default:
		s = append(s, fmt.Sprintf("Cash stays above your %s emergency target throughout — lowest %s in %s.", eurS(base.Floor), eurN(r.Lowest), monthName(r.LowestOn)))
	}
	if r.MarketHit > 0 {
		s = append(s, fmt.Sprintf("What you own loses %s at the drop.", eurS(r.MarketHit)))
	}
	if r.LoanExtra > 0 {
		s = append(s, fmt.Sprintf("Loan payments go up %s a month.", eurS(r.LoanExtra)))
	}
	if r.Cost > 0 {
		s = append(s, fmt.Sprintf("Over %d months it costs %s of net worth against the plan.", len(r.Months), eurS(r.Cost)))
	}
	return s
}

// stressLevers: what would soften it, only when it needs softening and the
// lever isn't pulled already.
func stressLevers(base *Forecast, r StressResult, sh Shock) []Lever {
	if r.Level == "ok" {
		return nil
	}
	a := base.Assumed
	var out []Lever
	if !sh.PauseSaving && a.Saving > 0 {
		out = append(out, Lever{Label: "Pause saving and investing", Amount: a.Saving, Monthly: true})
	}
	if sh.EverydayPct > -20 {
		ev := money.FromFloat(a.Everyday.Float() * (1 + sh.EverydayPct/100))
		out = append(out, Lever{Label: "Cut everyday spending by a fifth", Amount: money.FromFloat(ev.Float() * 0.2), Monthly: true})
	}
	if !sh.PauseGoals {
		var goals money.Cents
		for i, m := range base.Months {
			if i >= 1 && (sh.Months == 0 || i <= sh.Months) {
				for _, it := range m.Dated {
					if it.Kind == "goal" {
						goals += it.Amount
					}
				}
			}
		}
		if goals > 0 {
			out = append(out, Lever{Label: "Put off the wish-list goals due meanwhile", Amount: goals})
		}
	}
	return out
}

// ParseShock reads a shock from ?shock=<json>, for the read-only GET.
func ParseShock(raw string) (*Shock, error) {
	if raw == "" {
		return nil, nil
	}
	var s Shock
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return nil, errors.New("invalid shock: " + err.Error())
	}
	return &s, nil
}
