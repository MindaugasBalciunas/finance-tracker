package insights

import (
	"sort"
	"time"

	"ft/internal/ledger"
	"ft/internal/merchant"
	"ft/internal/money"
)

// Habits: how the money goes out, not just how much — how often you buy,
// how big a usual purchase is, the small buys that add up, when in the
// month and the week it happens, eating out against cooking, and which
// months run dear. All on spending by choice (ledger.Obligation and the
// people paid one left out: alimony on the 17th is no habit), complete
// months only, refunds not netted.

type HabitMonth struct {
	Month      string      `json:"month"`
	Spent      money.Cents `json:"spent"`     // spending by choice
	Purchases  int         `json:"purchases"` // receipts: a split is one purchase
	Median     money.Cents `json:"median"`    // the usual purchase
	Small      int         `json:"small"`     // purchases under €10
	SmallSpent money.Cents `json:"small_spent"`
	Days       int         `json:"days"`  // days with any spending by choice
	Early      money.Cents `json:"early"` // days 1–10
	Mid        money.Cents `json:"mid"`   // 11–20
	Late       money.Cents `json:"late"`  // 21–end
	Weekend    money.Cents `json:"weekend"`
	EatingOut  money.Cents `json:"eating_out"` // food other than groceries, meals under €200
	Groceries  money.Cents `json:"groceries"`
}

type HabitWeekday struct {
	Day     int         `json:"day"`     // 1 = Monday … 7 = Sunday
	Average money.Cents `json:"average"` // spent on such a day, on average
}

type HabitSeason struct {
	Month   int         `json:"month"`   // 1–12
	Average money.Cents `json:"average"` // that calendar month, averaged over the years seen
	Years   int         `json:"years"`
}

// HabitChange compares the last three complete months with the same three
// a year earlier, a month's worth each (shares and the usual purchase as is).
type HabitChange struct {
	Key  string  `json:"key"`
	Now  float64 `json:"now"`
	Then float64 `json:"then"`
}

type HabitsReport struct {
	Months   []HabitMonth   `json:"months"`
	Weekdays []HabitWeekday `json:"weekdays"`
	Season   []HabitSeason  `json:"season"`
	Recent   [2]string      `json:"recent"`   // first and last month compared
	YearAgo  [2]string      `json:"year_ago"` // the same months a year earlier
	Changes  []HabitChange  `json:"changes"`
}

const (
	smallBuy = 1000 // cents: "under €10"
	// A meal of €200 or more is an occasion, not a habit: a €1,113.50
	// family-reunion dinner made September's eating out read +313%.
	occasion = 20000
)

// HabitsWindow is how far back Habits needs txs: the span asked for, three
// years of seasons, and the year-ago comparison.
func HabitsWindow(now time.Time, months int) string {
	return ledger.AddMonths(now, -max(months, 36)-1) + "-01"
}

// Habits over the `months` complete months before now's month. txs are
// expenses from HabitsWindow on; people are the merchant keys ever paid an
// obligation (ledger.ObligationPayees — an ex-partner, family, the mortgage
// bank aren't shops, whatever the category says), nil to take them from txs.
func Habits(txs []ledger.Tx, people map[string]bool, now time.Time, months int) HabitsReport {
	cur := now.Format("2006-01")
	if people == nil {
		people = map[string]bool{}
		for i := range txs {
			if t := &txs[i]; t.Merchant != "" && ledger.PaysAPerson(t.Category) {
				people[merchant.Key(t.Merchant)] = true
			}
		}
	}
	type acc struct {
		HabitMonth
		days     map[string]bool
		receipts map[int64]money.Cents
	}
	by := map[string]*acc{}
	daySpent := map[string]money.Cents{}
	var seq int64
	for i := range txs {
		t := &txs[i]
		m := t.Date[:7]
		if t.Kind != "expense" || t.Pending || m >= cur || ledger.Obligation(t.Category) || (t.Merchant != "" && people[merchant.Key(t.Merchant)]) {
			continue
		}
		a := by[m]
		if a == nil {
			a = &acc{HabitMonth: HabitMonth{Month: m}, days: map[string]bool{}, receipts: map[int64]money.Cents{}}
			by[m] = a
		}
		a.Spent += t.Amount
		a.days[t.Date] = true
		daySpent[t.Date] += t.Amount
		// A receipt split across categories is one purchase: its parts carve
		// the parent, so parent + parts is what was paid.
		id := t.ID
		if t.SplitOf != 0 {
			id = t.SplitOf
		}
		if id == 0 { // not stored: a purchase of its own
			seq--
			id = seq
		}
		a.receipts[id] += t.Amount
		d, _ := time.Parse("2006-01-02", t.Date)
		switch {
		case d.Day() <= 10:
			a.Early += t.Amount
		case d.Day() <= 20:
			a.Mid += t.Amount
		default:
			a.Late += t.Amount
		}
		if wd := d.Weekday(); wd == time.Saturday || wd == time.Sunday {
			a.Weekend += t.Amount
		}
		switch {
		case t.Category == "food.groceries":
			a.Groceries += t.Amount
		case ledger.Top(t.Category) == "food" && t.Category != "food" && t.Amount < occasion:
			a.EatingOut += t.Amount
		}
	}
	tickets := map[string][]money.Cents{}
	for m, a := range by {
		a.Days = len(a.days)
		for _, v := range a.receipts {
			a.Purchases++
			tickets[m] = append(tickets[m], v)
			if v < smallBuy {
				a.Small++
				a.SmallSpent += v
			}
		}
		a.Median = median(tickets[m])
	}
	month := func(m string) HabitMonth {
		if a := by[m]; a != nil {
			return a.HabitMonth
		}
		return HabitMonth{Month: m}
	}

	rep := HabitsReport{Months: []HabitMonth{}, Weekdays: []HabitWeekday{}, Season: []HabitSeason{}, Changes: []HabitChange{}}
	first := ""
	for i := range txs {
		if first == "" || txs[i].Date < first {
			first = txs[i].Date
		}
	}
	for i := months; i >= 1; i-- {
		m := ledger.AddMonths(now, -i)
		if first == "" || m < first[:7] { // no history yet: nothing to show
			continue
		}
		rep.Months = append(rep.Months, month(m))
	}

	// The week: what a Monday, a Tuesday… costs on average over the span.
	if len(rep.Months) > 0 {
		from, _ := time.Parse("2006-01", rep.Months[0].Month)
		to, _ := time.Parse("2006-01", cur)
		var sum [7]money.Cents
		var n [7]int
		for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
			k := (int(d.Weekday()) + 6) % 7 // Monday first
			sum[k] += daySpent[d.Format("2006-01-02")]
			n[k]++
		}
		for k := 0; k < 7; k++ {
			w := HabitWeekday{Day: k + 1}
			if n[k] > 0 {
				w.Average = sum[k] / money.Cents(n[k])
			}
			rep.Weekdays = append(rep.Weekdays, w)
		}
	}

	// Seasons: each calendar month over the last three years of history.
	var sea [12]money.Cents
	var yrs [12]int
	for i := 1; i <= 36; i++ {
		m := ledger.AddMonths(now, -i)
		if first == "" || m < first[:7] {
			continue
		}
		k := int(m[5]-'0')*10 + int(m[6]-'0') - 1
		sea[k] += month(m).Spent
		yrs[k]++
	}
	for k := 0; k < 12; k++ {
		s := HabitSeason{Month: k + 1, Years: yrs[k]}
		if yrs[k] > 0 {
			s.Average = sea[k] / money.Cents(yrs[k])
		}
		rep.Season = append(rep.Season, s)
	}

	// The last three complete months against the same three a year before.
	rep.Recent = [2]string{ledger.AddMonths(now, -3), ledger.AddMonths(now, -1)}
	rep.YearAgo = [2]string{ledger.AddMonths(now, -15), ledger.AddMonths(now, -13)}
	if first == "" || rep.YearAgo[0] < first[:7] {
		return rep // no year to compare with
	}
	type sums struct {
		spent, small, eat, weekend money.Cents
		purchases, days, smallN    int
		tickets                    []money.Cents
	}
	window := func(back int) sums {
		var s sums
		for i := back; i < back+3; i++ {
			m := ledger.AddMonths(now, -i)
			h := month(m)
			s.spent += h.Spent
			s.purchases += h.Purchases
			s.days += h.Days
			s.smallN += h.Small
			s.eat += h.EatingOut
			s.weekend += h.Weekend
			s.tickets = append(s.tickets, tickets[m]...)
		}
		return s
	}
	a, b := window(1), window(13)
	share := func(part, whole money.Cents) float64 {
		if whole <= 0 {
			return 0
		}
		return round3(part.Float() / whole.Float())
	}
	rep.Changes = []HabitChange{
		{"spent", (a.spent / 3).Float(), (b.spent / 3).Float()},
		{"purchases", float64(a.purchases) / 3, float64(b.purchases) / 3},
		{"median", median(a.tickets).Float(), median(b.tickets).Float()},
		{"small", float64(a.smallN) / 3, float64(b.smallN) / 3},
		{"eating_out", (a.eat / 3).Float(), (b.eat / 3).Float()},
		{"weekend", share(a.weekend, a.spent), share(b.weekend, b.spent)},
		{"days", float64(a.days) / 3, float64(b.days) / 3},
	}
	return rep
}

func median(v []money.Cents) money.Cents {
	if len(v) == 0 {
		return 0
	}
	s := append([]money.Cents(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	if n := len(s); n%2 == 1 {
		return s[n/2]
	} else {
		return (s[n/2-1] + s[n/2]) / 2
	}
}
