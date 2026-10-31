package insights

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
)

// Drift: the slow changes nobody notices month to month. Lifestyle creep
// compares how fast spending grew with how fast income did and names what
// grew; price creep follows each regular bill's price year on year and
// warns before a renewal; the personal inflation rate is those bills'
// combined price change, weighted by what each costs a year.

// ── lifestyle creep ─────────────────────────────────────────────────

type CreepWindow struct {
	From     string      `json:"from"` // YYYY-MM
	To       string      `json:"to"`
	Income   money.Cents `json:"income"`
	Spending money.Cents `json:"spending"`
	Fixed    money.Cents `json:"fixed"` // essential spending
}

type CreepDriver struct {
	Category string      `json:"category"`
	Then     money.Cents `json:"then"` // a year, in the first window
	Now      money.Cents `json:"now"`
	Change   money.Cents `json:"change"`
}

type Commitment struct {
	Merchant string      `json:"merchant"`
	Category string      `json:"category"`
	Monthly  money.Cents `json:"monthly"`
	Since    string      `json:"since"` // first payment
}

type Creep struct {
	Years          int           `json:"years"` // how many years apart the windows are (0 = not enough history)
	Then           CreepWindow   `json:"then"`
	Now            CreepWindow   `json:"now"`
	IncomeGrowth   float64       `json:"income_growth"`   // %
	SpendingGrowth float64       `json:"spending_growth"` // %
	FixedGrowth    float64       `json:"fixed_growth"`    // %
	Gap            float64       `json:"gap"`             // spending growth − income growth, points
	Tone           string        `json:"tone"`            // good | warn | bad
	Text           string        `json:"text"`
	Drivers        []CreepDriver `json:"drivers"` // what grew most
	Fallers        []CreepDriver `json:"fallers"` // what fell most (Change negative)
	New            []Commitment  `json:"new"`     // regular bills that started since the first window
	NewMonthly     money.Cents   `json:"new_monthly"`
}

// window is 12 complete months ending with the month before end (YYYY-MM).
func window(txs []ledger.Tx, cats map[string]ledger.Category, salary []plan.SalaryRule, endExcl string) (CreepWindow, map[string]money.Cents) {
	e, _ := time.Parse("2006-01", endExcl)
	from, to := e.AddDate(-1, 0, 0).Format("2006-01"), e.AddDate(0, -1, 0).Format("2006-01")
	w := CreepWindow{From: from, To: to}
	by := map[string]money.Cents{}
	for _, f := range CashFlowRange(txs, cats, "month", from+"-01", to+"-31", salary) {
		w.Income += f.Income
		w.Spending += f.Spending
		w.Fixed += f.Essential
		for k, v := range f.SpendingBy {
			by[k] += v
		}
	}
	return w, by
}

func growth(then, now money.Cents) float64 {
	if then <= 0 {
		return 0
	}
	return math.Round((now.Float()/then.Float()-1)*1000) / 10
}

// LifestyleCreep compares the last 12 complete months with the same 12
// months `years` earlier (1 or 3), or as far back as the history goes.
func LifestyleCreep(d *sql.DB, now time.Time, years int) (*Creep, error) {
	if years < 1 || years > 3 {
		years = 1
	}
	settings := plan.LoadSettings(d)
	cats, err := ledger.CategoryMap(d)
	if err != nil {
		return nil, err
	}
	txs, err := ledger.All(d, ledger.Filter{From: now.AddDate(-4, -1, 0).Format("2006-01") + "-01", To: now.Format("2006-01-02")})
	if err != nil {
		return nil, err
	}
	c := &Creep{Drivers: []CreepDriver{}, Fallers: []CreepDriver{}, New: []Commitment{}}
	if len(txs) == 0 {
		return c, nil
	}
	first := txs[0].Date
	for _, t := range txs {
		if t.Date < first {
			first = t.Date
		}
	}
	cur := now.Format("2006-01")
	nowW, nowBy := window(txs, cats, settings.Salary(), cur)
	var thenBy map[string]money.Cents
	for y := years; y >= 1; y-- {
		end := now.AddDate(-y, 0, 0).Format("2006-01")
		w, by := window(txs, cats, settings.Salary(), end)
		if w.From+"-01" >= first[:7]+"-01" && w.Income > 0 && w.Spending > 0 {
			c.Years, c.Then, thenBy = y, w, by
			break
		}
	}
	c.Now = nowW
	if c.Years == 0 {
		c.Tone, c.Text = "good", "Two full years of history are needed to compare."
		return c, nil
	}
	c.IncomeGrowth = growth(c.Then.Income, c.Now.Income)
	c.SpendingGrowth = growth(c.Then.Spending, c.Now.Spending)
	c.FixedGrowth = growth(c.Then.Fixed, c.Now.Fixed)
	c.Gap = math.Round((c.SpendingGrowth-c.IncomeGrowth)*10) / 10
	since := fmt.Sprintf("in %d year", c.Years)
	if c.Years > 1 {
		since += "s"
	}
	verb := fmt.Sprintf("grew %.0f%%", c.SpendingGrowth)
	if c.SpendingGrowth < 0 {
		verb = fmt.Sprintf("fell %.0f%%", -c.SpendingGrowth)
	}
	inc := fmt.Sprintf("%.0f%%", c.IncomeGrowth)
	switch {
	case c.Gap > 5:
		c.Tone = "bad"
		c.Text = fmt.Sprintf("Spending %s %s, income %s: your lifestyle is outgrowing your pay.", verb, since, inc)
	case c.Gap > 0:
		c.Tone = "warn"
		c.Text = fmt.Sprintf("Spending %s %s, a little faster than income (%s).", verb, since, inc)
	default:
		c.Tone = "good"
		c.Text = fmt.Sprintf("Spending %s %s, income %s — no creep: your pay is going further than your lifestyle.", verb, since, inc)
	}

	// What grew and what fell, so a total that fell doesn't hide one
	// category that grew (and the other way round).
	keys := map[string]bool{}
	for k := range nowBy {
		keys[k] = true
	}
	for k := range thenBy {
		keys[k] = true
	}
	for k := range keys {
		// Refunds sit in spending as a negative: fewer refunds isn't more
		// spending, and they read as nonsense either way ("−€6,947 → −€1,394").
		if thenBy[k] < 0 || nowBy[k] < 0 {
			continue
		}
		ch := nowBy[k] - thenBy[k]
		dr := CreepDriver{Category: k, Then: thenBy[k], Now: nowBy[k], Change: ch}
		switch {
		case ch > 0:
			c.Drivers = append(c.Drivers, dr)
		case ch < 0:
			c.Fallers = append(c.Fallers, dr)
		}
	}
	sort.Slice(c.Drivers, func(i, j int) bool { return c.Drivers[i].Change > c.Drivers[j].Change })
	sort.Slice(c.Fallers, func(i, j int) bool { return c.Fallers[i].Change < c.Fallers[j].Change })
	c.Drivers, c.Fallers = c.Drivers[:min(5, len(c.Drivers))], c.Fallers[:min(5, len(c.Fallers))]

	// Regular bills whose first payment came after the first window began.
	bills, _, err := RecurringCosts(d, txs, now)
	if err != nil {
		return nil, err
	}
	for _, r := range bills {
		start := ""
		for _, t := range txs {
			if t.Kind == "expense" && t.Category == r.Category && strings.EqualFold(t.Merchant, r.Merchant) && (start == "" || t.Date < start) {
				start = t.Date
			}
		}
		if start != "" && start[:7] > c.Then.To {
			c.New = append(c.New, Commitment{Merchant: r.Merchant, Category: r.Category, Monthly: r.Monthly, Since: start})
			c.NewMonthly += r.Monthly
		}
	}
	sort.Slice(c.New, func(i, j int) bool { return c.New[i].Monthly > c.New[j].Monthly })
	return c, nil
}

// ── price creep, renewals and personal inflation ───────────────────

type PriceWatch struct {
	Merchant    string      `json:"merchant"`
	Category    string      `json:"category"`
	Cadence     string      `json:"cadence"`
	Now         money.Cents `json:"now"` // the latest charge
	LastDate    string      `json:"last_date"`
	YearAgo     money.Cents `json:"year_ago"` // the charge about a year earlier (0 = none to compare)
	YearAgoDate string      `json:"year_ago_date,omitempty"`
	Change      float64     `json:"change"` // %
	Yearly      money.Cents `json:"yearly"` // what it costs a year now
	Extra       money.Cents `json:"extra"`  // a year, compared with a year ago
	Next        string      `json:"next,omitempty"`
	RenewsSoon  bool        `json:"renews_soon"`
	Advice      string      `json:"advice,omitempty"`
	// PlanChange: moved more than 30% — a different contract, not a price
	// change; it stays out of the personal inflation rate.
	PlanChange bool `json:"plan_change,omitempty"`
}

type Prices struct {
	Items  []PriceWatch `json:"items"` // renewals soon and the biggest rises first
	Rising int          `json:"rising"`
	Extra  money.Cents  `json:"extra"` // a year, all bills together
	// Personal inflation: the bills' combined price change, weighted by what
	// each cost a year ago; Basket is how many bills it is measured on.
	Personal *float64    `json:"personal"`
	Basket   int         `json:"basket"`
	Weight   money.Cents `json:"weight"` // what the basket cost a year ago, a year
	// Excluded: bills left out of the rate because they changed plan.
	Excluded int `json:"excluded"`
}

// planChange: a year-on-year move this large (%) is a different contract.
const planChange = 30.0

func perYear(cadence string) int64 {
	switch cadence {
	case "yearly":
		return 1
	case "quarterly":
		return 4
	}
	return 12
}

// renewWindow: how early a renewal is flagged — enough to get a quote.
const renewWindow = 45

// BillHistory is what the bill views read: two years and two months to
// today, enough to see a yearly bill renew twice.
func BillHistory(d *sql.DB, now time.Time) ([]ledger.Tx, error) {
	return ledger.All(d, ledger.Filter{From: now.AddDate(-2, -2, 0).Format("2006-01-02"), To: now.Format("2006-01-02")})
}

// PriceCreep follows every regular bill's price year on year.
func PriceCreep(d *sql.DB, now time.Time) (*Prices, error) {
	txs, err := BillHistory(d, now)
	if err != nil {
		return nil, err
	}
	return PriceCreepIn(d, txs, now)
}

// PriceCreepIn is PriceCreep over an already read BillHistory.
func PriceCreepIn(d *sql.DB, txs []ledger.Tx, now time.Time) (*Prices, error) {
	bills, _, err := RecurringCosts(d, txs, now)
	if err != nil {
		return nil, err
	}
	// The recurring list only sees a yearly bill while both of its last two
	// payments sit inside 13 months — i.e. just after it renewed, which is
	// exactly when a warning is useless. Look back two years for them here.
	bills = append(bills, yearlyRenewals(txs, bills, now)...)
	out := &Prices{Items: []PriceWatch{}}
	soon := now.AddDate(0, 0, renewWindow).Format("2006-01-02")
	today := now.Format("2006-01-02")
	var thenSum, nowSum money.Cents
	for _, r := range bills {
		var pays []ledger.Tx
		for _, t := range txs {
			if t.Kind == "expense" && !t.Pending && t.Category == r.Category && strings.EqualFold(t.Merchant, r.Merchant) {
				pays = append(pays, t)
			}
		}
		if len(pays) == 0 {
			continue
		}
		sort.Slice(pays, func(i, j int) bool { return pays[i].Date < pays[j].Date })
		last := pays[len(pays)-1]
		pw := PriceWatch{Merchant: r.Merchant, Category: r.Category, Cadence: r.Cadence, Now: last.Amount, LastDate: last.Date, Next: r.Next}
		pw.Yearly = last.Amount * money.Cents(perYear(r.Cadence))
		// The charge closest to a year before the latest one, within six weeks.
		lt, _ := time.Parse("2006-01-02", last.Date)
		target := lt.AddDate(-1, 0, 0)
		best := 46 * 24 * time.Hour
		for _, p := range pays[:len(pays)-1] {
			pt, _ := time.Parse("2006-01-02", p.Date)
			if dist := absDur(pt.Sub(target)); dist < best {
				best, pw.YearAgo, pw.YearAgoDate = dist, p.Amount, p.Date
			}
		}
		if pw.YearAgo > 0 {
			pw.Change = growth(pw.YearAgo, pw.Now)
			pw.Extra = (pw.Now - pw.YearAgo) * money.Cents(perYear(r.Cadence))
			// A bill that moved more than 30% changed plan (Telia €41.70 →
			// €14.95 on a cheaper contract), not price: it stays on the list,
			// but would swamp the inflation rate, so it isn't in the basket.
			if math.Abs(pw.Change) <= planChange {
				thenSum += pw.YearAgo * money.Cents(perYear(r.Cadence))
				nowSum += pw.Yearly
				out.Basket++
			} else {
				out.Excluded++
				pw.PlanChange = true
			}
			if pw.Change >= 5 {
				out.Rising++
				out.Extra += pw.Extra
			}
		}
		pw.RenewsSoon = r.Cadence != "monthly" && r.Next != "" && r.Next >= today && r.Next <= soon
		switch {
		case pw.RenewsSoon && pw.Change >= 5:
			pw.Advice = fmt.Sprintf("Renews %s and cost %.0f%% more last time — worth getting a quote first.", r.Next, pw.Change)
		case pw.RenewsSoon:
			pw.Advice = fmt.Sprintf("Renews %s — a good moment to compare.", r.Next)
		case pw.Change >= 10 && !pw.PlanChange:
			pw.Advice = fmt.Sprintf("%.0f%% dearer than a year ago: %s a year more.", pw.Change, eurDrift(pw.Extra))
		}
		out.Items = append(out.Items, pw)
	}
	if thenSum > 0 {
		v := growth(thenSum, nowSum)
		out.Personal, out.Weight = &v, thenSum
	}
	sort.SliceStable(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if a.RenewsSoon != b.RenewsSoon {
			return a.RenewsSoon
		}
		return a.Extra > b.Extra
	})
	return out, nil
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func eurDrift(c money.Cents) string {
	return fmt.Sprintf("€%.0f", math.Round(c.Float()))
}

// yearlyRenewals finds bills paid about a year apart (330–400 days, the
// amount within half of the last), not on the recurring list already.
func yearlyRenewals(txs []ledger.Tx, known []Recurring, now time.Time) []Recurring {
	have := map[string]bool{}
	for _, r := range known {
		have[strings.ToLower(r.Merchant)+"|"+r.Category] = true
	}
	by := map[string][]ledger.Tx{}
	for _, t := range txs {
		if t.Kind != "expense" || t.Pending || t.Merchant == "" {
			continue
		}
		k := strings.ToLower(t.Merchant) + "|" + t.Category
		if !have[k] {
			by[k] = append(by[k], t)
		}
	}
	var out []Recurring
	for _, ps := range by {
		if len(ps) < 2 || len(ps) > 3 {
			continue
		}
		sort.Slice(ps, func(i, j int) bool { return ps[i].Date < ps[j].Date })
		a, b := ps[len(ps)-2], ps[len(ps)-1]
		ta, _ := time.Parse("2006-01-02", a.Date)
		tb, _ := time.Parse("2006-01-02", b.Date)
		gap := tb.Sub(ta).Hours() / 24
		if gap < 330 || gap > 400 || a.Amount*2 < b.Amount || b.Amount*2 < a.Amount {
			continue
		}
		next := tb.AddDate(1, 0, 0)
		if next.Before(now.AddDate(0, 0, -30)) { // lapsed: it didn't renew
			continue
		}
		out = append(out, Recurring{Source: "detected", Kind: "bill", Merchant: b.Merchant, Category: b.Category, Cadence: "yearly",
			Amount: b.Amount, Monthly: b.Amount / 12, Last: b.Date, Next: next.Format("2006-01-02"), Count: len(ps), LastAmount: b.Amount})
	}
	return out
}
