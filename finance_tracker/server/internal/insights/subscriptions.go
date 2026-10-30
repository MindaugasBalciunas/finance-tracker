package insights

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"ft/internal/ledger"
	"ft/internal/money"
)

// The subscription audit: every regular charge you could cancel, what it
// costs a year, which are new, which started as a free trial, and your own
// verdict — keep it, or cancel it by a date. Obligations (the mortgage,
// alimony, utilities, taxes) aren't subscriptions and stay out.

type SubVerdict struct {
	Verdict   string `json:"verdict"`             // keep | cancel
	CancelBy  string `json:"cancel_by,omitempty"` // YYYY-MM-DD
	DecidedOn string `json:"decided_on,omitempty"`
}

type Subscription struct {
	Key       string      `json:"key"` // merchant|category, lower case
	Merchant  string      `json:"merchant"`
	Category  string      `json:"category"`
	Cadence   string      `json:"cadence"`
	Amount    money.Cents `json:"amount"` // latest charge
	Yearly    money.Cents `json:"yearly"`
	Since     string      `json:"since"` // first charge
	Last      string      `json:"last"`
	Next      string      `json:"next,omitempty"`
	New       bool        `json:"new"`                  // first charged within 90 days
	TrialFrom string      `json:"trial_from,omitempty"` // a tiny first charge, then the full price
	Verdict   *SubVerdict `json:"verdict,omitempty"`
	// ChargedAfterCancel: a charge came after you marked it to cancel.
	ChargedAfterCancel string `json:"charged_after_cancel,omitempty"`
}

type Subscriptions struct {
	Items    []Subscription `json:"items"`
	Monthly  money.Cents    `json:"monthly"`
	Yearly   money.Cents    `json:"yearly"`
	ToCancel money.Cents    `json:"to_cancel"` // a year, the ones marked cancel
}

// notASubscription: regular, but nothing you'd cancel.
func notASubscription(cat string) bool {
	for _, p := range []string{"housing", "kids.alimony", "finance.taxes", "finance.legal", "finance.divorce", "utilities.electricity", "utilities.heating", "utilities.water", "utilities.building"} {
		if cat == p || strings.HasPrefix(cat, p+".") {
			return true
		}
	}
	return false
}

func SubKey(merchant, category string) string {
	return strings.ToLower(strings.TrimSpace(merchant)) + "|" + category
}

func LoadSubVerdicts(d *sql.DB) map[string]SubVerdict {
	out := map[string]SubVerdict{}
	var raw string
	if d.QueryRow(`SELECT value FROM settings WHERE key='subscriptions'`).Scan(&raw) == nil {
		json.Unmarshal([]byte(raw), &out)
	}
	return out
}

// SetSubVerdict records keep / cancel-by for one subscription ("" clears it).
func SetSubVerdict(d *sql.DB, key string, v SubVerdict, now time.Time) error {
	if key == "" || len(key) > 300 || !strings.Contains(key, "|") {
		return errors.New("which subscription?")
	}
	all := LoadSubVerdicts(d)
	switch v.Verdict {
	case "":
		delete(all, key)
	case "keep":
		all[key] = SubVerdict{Verdict: "keep", DecidedOn: now.Format("2006-01-02")}
	case "cancel":
		if _, err := time.Parse("2006-01-02", v.CancelBy); err != nil {
			return errors.New("cancel by which day?")
		}
		all[key] = SubVerdict{Verdict: "cancel", CancelBy: v.CancelBy, DecidedOn: now.Format("2006-01-02")}
	default:
		return errors.New("keep it or cancel it")
	}
	raw, _ := json.Marshal(all)
	_, err := d.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('subscriptions',?)`, string(raw))
	return err
}

func AuditSubscriptions(d *sql.DB, now time.Time) (*Subscriptions, error) {
	txs, err := ledger.All(d, ledger.Filter{From: now.AddDate(-2, -2, 0).Format("2006-01-02"), To: now.Format("2006-01-02")})
	if err != nil {
		return nil, err
	}
	bills, _, err := RecurringCosts(d, txs, now)
	if err != nil {
		return nil, err
	}
	bills = append(bills, yearlyRenewals(txs, bills, now)...)
	bills = append(bills, newMonthly(txs, bills, now)...)
	verdicts := LoadSubVerdicts(d)
	out := &Subscriptions{Items: []Subscription{}}
	recent := now.AddDate(0, 0, -90).Format("2006-01-02")
	for _, r := range bills {
		if notASubscription(r.Category) {
			continue
		}
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
		s := Subscription{Key: SubKey(r.Merchant, r.Category), Merchant: r.Merchant, Category: r.Category, Cadence: r.Cadence,
			Amount: last.Amount, Since: pays[0].Date, Last: last.Date, Next: r.Next}
		s.Yearly = last.Amount * money.Cents(perYear(r.Cadence))
		s.New = s.Since >= recent
		// A trial: the first charge tiny (under €2, or under a fifth of the
		// next), the next one the full price within six weeks.
		if len(pays) >= 2 {
			a, b := pays[0], pays[1]
			ta, _ := time.Parse("2006-01-02", a.Date)
			tb, _ := time.Parse("2006-01-02", b.Date)
			if (a.Amount < 200 || a.Amount*5 < b.Amount) && b.Amount >= 300 && tb.Sub(ta) <= 45*24*time.Hour {
				s.TrialFrom = a.Date
			}
		}
		if v, ok := verdicts[s.Key]; ok {
			v := v
			s.Verdict = &v
			if v.Verdict == "cancel" {
				out.ToCancel += s.Yearly
				// Charges up to the cancel-by date are expected (the
				// notice period); only one after it means it wasn't cancelled.
				for _, p := range pays {
					if p.Date > max(v.CancelBy, v.DecidedOn) {
						s.ChargedAfterCancel = p.Date
					}
				}
			}
		}
		out.Yearly += s.Yearly
		out.Items = append(out.Items, s)
	}
	out.Monthly = out.Yearly / 12
	// Undecided first, the dearest at the top; then the ones to cancel; kept last.
	rank := func(s Subscription) int {
		switch {
		case s.Verdict == nil:
			return 0
		case s.Verdict.Verdict == "cancel":
			return 1
		}
		return 2
	}
	sort.SliceStable(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if rank(a) != rank(b) {
			return rank(a) < rank(b)
		}
		return a.Yearly > b.Yearly
	})
	return out, nil
}

// subscriptionLike: categories where a monthly charge is a subscription you
// chose, so two similar charges a month apart are enough to call it one.
func subscriptionLike(cat string) bool {
	for _, p := range []string{"subscriptions", "utilities.telecom", "health.fitness", "finance.insurance", "other.education", "leisure.hobbies"} {
		if cat == p || strings.HasPrefix(cat, p+".") {
			return true
		}
	}
	return false
}

// newMonthly finds subscriptions too new for the recurring list (it waits for
// five months of charges — too late to catch a trial turning into a bill):
// the last two charges within a quarter of each other, 25–35 days apart, the
// latest within 40 days.
func newMonthly(txs []ledger.Tx, known []Recurring, now time.Time) []Recurring {
	have := map[string]bool{}
	for _, r := range known {
		have[SubKey(r.Merchant, r.Category)] = true
	}
	by := map[string][]ledger.Tx{}
	for _, t := range txs {
		if t.Kind == "expense" && !t.Pending && t.Merchant != "" && subscriptionLike(t.Category) && !have[SubKey(t.Merchant, t.Category)] {
			k := SubKey(t.Merchant, t.Category)
			by[k] = append(by[k], t)
		}
	}
	var out []Recurring
	recent := now.AddDate(0, 0, -40).Format("2006-01-02")
	for _, ps := range by {
		if len(ps) < 2 {
			continue
		}
		sort.Slice(ps, func(i, j int) bool { return ps[i].Date < ps[j].Date })
		a, b := ps[len(ps)-2], ps[len(ps)-1]
		ta, _ := time.Parse("2006-01-02", a.Date)
		tb, _ := time.Parse("2006-01-02", b.Date)
		gap := tb.Sub(ta).Hours() / 24
		if b.Date < recent || gap < 25 || gap > 35 || a.Amount*4 < b.Amount*3 || b.Amount*4 < a.Amount*3 {
			continue
		}
		out = append(out, Recurring{Source: "detected", Kind: "bill", Merchant: b.Merchant, Category: b.Category, Cadence: "monthly",
			Amount: b.Amount, Monthly: b.Amount, Last: b.Date, Next: tb.AddDate(0, 1, 0).Format("2006-01-02"), Count: len(ps), LastAmount: b.Amount})
	}
	return out
}
