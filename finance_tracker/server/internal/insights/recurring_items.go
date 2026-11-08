package insights

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"ft/internal/db"
	"ft/internal/ledger"
	"ft/internal/money"
)

// RecurringItem is the owner's word on a recurring cost: one added by hand,
// a detected one with its amount/cadence/date corrected, or a detected one
// hidden because it isn't really recurring. Matched to detection by merchant;
// several items for one merchant (Telia phone, Telia internet) are told apart
// by their note.
type RecurringItem struct {
	ID       int64       `json:"id"`
	Merchant string      `json:"merchant"`
	Category string      `json:"category"`
	Cadence  string      `json:"cadence"`
	Amount   money.Cents `json:"amount"`
	NextDate string      `json:"next_date"`
	Note     string      `json:"note"`
	Hidden   bool        `json:"hidden"`
	// EveryDays > 0: a flexible rhythm ("about every 35 days") instead of a
	// calendar cadence; the next date follows the last matching payment.
	EveryDays int `json:"every_days"`
	// Kind: bill (money leaving), transfer (between accounts — a standing
	// order, or one you plan to make) or income (money arriving). Accounts:
	// bill = paid from; transfer = from → to; income = paid into.
	Kind        string `json:"kind"`
	FromAccount string `json:"from_account"`
	ToAccount   string `json:"to_account"`
	Day         int    `json:"day"` // usual day of month (0 = from the next date)
}

var recurringKinds = map[string]bool{"bill": true, "transfer": true, "income": true}

var cadenceMonths = map[string]int{"monthly": 1, "quarterly": 3, "yearly": 12}

func ListRecurringItems(q *sql.DB) ([]RecurringItem, error) {
	rows, err := q.Query(`SELECT id,merchant,category,cadence,amount,next_date,note,hidden,every_days,kind,from_account,to_account,day FROM recurring_items ORDER BY lower(merchant)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RecurringItem{}
	for rows.Next() {
		var it RecurringItem
		if err := rows.Scan(&it.ID, &it.Merchant, &it.Category, &it.Cadence, &it.Amount, &it.NextDate, &it.Note, &it.Hidden, &it.EveryDays, &it.Kind, &it.FromAccount, &it.ToAccount, &it.Day); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// SaveRecurringItem inserts or updates; saving the same merchant and note
// again updates that item (a different note adds a second one).
func SaveRecurringItem(d *sql.DB, it *RecurringItem) error {
	it.Merchant = strings.TrimSpace(it.Merchant)
	if it.Merchant == "" {
		return errors.New("merchant is required")
	}
	if len(it.Merchant) > 120 || len(it.Category) > 64 || len(it.Note) > 500 {
		return errors.New("merchant, category or note is too long")
	}
	if it.Cadence == "" {
		it.Cadence = "monthly"
	}
	if _, ok := cadenceMonths[it.Cadence]; !ok {
		return errors.New("cadence must be monthly, quarterly or yearly")
	}
	if it.Amount < 0 {
		return errors.New("amount must be positive")
	}
	if it.EveryDays != 0 && (it.EveryDays < 7 || it.EveryDays > 400) {
		return errors.New("a flexible rhythm must be every 7–400 days")
	}
	if it.NextDate != "" {
		if _, err := time.Parse("2006-01-02", it.NextDate); err != nil {
			return errors.New("invalid next date")
		}
	}
	if it.Kind == "" {
		it.Kind = "bill"
	}
	if !recurringKinds[it.Kind] {
		return errors.New("kind must be bill, transfer or income")
	}
	if it.Day < 0 || it.Day > 31 {
		return errors.New("day of month must be 1–31")
	}
	for _, a := range []string{it.FromAccount, it.ToAccount} {
		if a == "" {
			continue
		}
		var n int
		d.QueryRow(`SELECT COUNT(*) FROM accounts WHERE id=?`, a).Scan(&n)
		if n == 0 {
			return errors.New("unknown account " + a)
		}
	}
	if it.Kind == "transfer" && it.FromAccount != "" && it.FromAccount == it.ToAccount {
		return errors.New("a transfer needs two different accounts")
	}
	now := db.Now()
	if it.ID == 0 {
		err := d.QueryRow(`INSERT INTO recurring_items(merchant,category,cadence,amount,next_date,note,hidden,every_days,kind,from_account,to_account,day,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(lower(merchant), lower(note)) DO UPDATE SET category=excluded.category, cadence=excluded.cadence, amount=excluded.amount,
				next_date=excluded.next_date, note=excluded.note, hidden=excluded.hidden, every_days=excluded.every_days,
				kind=excluded.kind, from_account=excluded.from_account, to_account=excluded.to_account, day=excluded.day, updated_at=excluded.updated_at
			RETURNING id`, it.Merchant, it.Category, it.Cadence, it.Amount, it.NextDate, it.Note, it.Hidden, it.EveryDays,
			it.Kind, it.FromAccount, it.ToAccount, it.Day, now, now).Scan(&it.ID)
		return err
	}
	res, err := d.Exec(`UPDATE recurring_items SET merchant=?,category=?,cadence=?,amount=?,next_date=?,note=?,hidden=?,every_days=?,kind=?,from_account=?,to_account=?,day=?,updated_at=? WHERE id=?`,
		it.Merchant, it.Category, it.Cadence, it.Amount, it.NextDate, it.Note, it.Hidden, it.EveryDays, it.Kind, it.FromAccount, it.ToAccount, it.Day, now, it.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func DeleteRecurringItem(d *sql.DB, id int64) error {
	if _, err := d.Exec(`DELETE FROM recurring_items WHERE id=?`, id); err != nil {
		return err
	}
	// Payments marked as this item are just transactions again.
	_, err := d.Exec(`UPDATE transactions SET recurring_id=0 WHERE recurring_id=?`, id)
	return err
}

// MergeRecurring applies the owner's items to what detection found. Hidden
// items drop out (returned separately so they can be restored); edits
// override amount, cadence, category and next date; hand-added items appear
// even without matching transactions.
func MergeRecurring(detected []Recurring, items []RecurringItem, now time.Time) (list, hidden []Recurring) {
	return mergeRecurring(detected, items, now, nil)
}

func mergeRecurring(detected []Recurring, items []RecurringItem, now time.Time, lastPaid map[int64]string) (list, hidden []Recurring) {
	byKey := map[string]RecurringItem{} // the item that edits a detection
	for _, it := range items {
		if _, ok := byKey[strings.ToLower(it.Merchant)]; !ok {
			byKey[strings.ToLower(it.Merchant)] = it
		}
	}
	today := now.Format("2006-01-02")
	apply := func(r Recurring, it RecurringItem) Recurring {
		r.ID, r.Note = it.ID, it.Note
		if it.Kind != "" {
			r.Kind = it.Kind
		}
		if it.FromAccount != "" {
			r.FromAccount = it.FromAccount
		}
		if it.ToAccount != "" {
			r.ToAccount = it.ToAccount
		}
		if it.Day > 0 {
			r.Day = it.Day
		}
		if it.Category != "" {
			r.Category = it.Category
		}
		if it.Cadence != "" {
			r.Cadence = it.Cadence
		}
		if it.Amount > 0 {
			r.Amount = it.Amount
		}
		if it.NextDate != "" {
			r.Next = it.NextDate
		}
		// The owner's item knows its own last payment (a merchant with two
		// items — Telia phone and internet — is matched per item).
		if last, ok := lastPaid[it.ID]; ok && (last > r.Last || r.Source == "detected") {
			r.Last = last
		}
		if it.EveryDays > 0 { // flexible: about every N days after the last payment
			r.EveryDays = it.EveryDays
			if r.Last != "" && (it.NextDate == "" || it.NextDate <= r.Last) {
				r.Next = addDays(r.Last, it.EveryDays)
			}
			r.Monthly = money.FromFloat(r.Amount.Float() * 30.44 / float64(it.EveryDays))
			r.Next = rollDays(r.Next, it.EveryDays, today)
			return r
		}
		r.Monthly = r.Amount / money.Cents(cadenceMonths[r.Cadence])
		r.Next = rollForward(r.Next, r.Cadence, today)
		return r
	}
	used := map[int64]bool{}
	for _, r := range detected {
		k := strings.ToLower(r.Merchant)
		it, ok := byKey[k]
		if ok {
			used[it.ID] = true
		}
		if !ok {
			r.Next = rollForward(r.Next, r.Cadence, today)
			list = append(list, r)
			continue
		}
		r = apply(r, it)
		r.Source = "edited"
		if it.Hidden {
			hidden = append(hidden, r)
		} else {
			list = append(list, r)
		}
	}
	for _, it := range items {
		if used[it.ID] {
			continue
		}
		r := apply(Recurring{Source: "manual", Kind: it.Kind, Merchant: it.Merchant, Cadence: "monthly"}, it)
		r.LastAmount = r.Amount
		if it.Hidden {
			r.Source = "edited" // a hidden detection that no longer shows up
			hidden = append(hidden, r)
		} else {
			list = append(list, r)
		}
	}
	for i := range list {
		settle(&list[i], now)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Monthly > list[j].Monthly })
	return list, hidden
}

// settle fills this month's state. With a usual day (standing orders,
// salary) the next date is that day this month — or next month once it has
// happened; anything not yet done whose day has passed is overdue.
func settle(r *Recurring, now time.Time) {
	if r.Kind == "" {
		r.Kind = "bill"
	}
	month := now.Format("2006-01")
	r.Done = r.Last != "" && r.Last[:7] == month
	// Paid this month: the next one is a period on, not later this month —
	// Argus paid on 5 Nov read "done · next 10 Nov" and came round again in
	// the cash calendar on the 10th.
	if r.Done && r.EveryDays == 0 && strings.HasPrefix(r.Next, month) {
		if t, err := time.Parse("2006-01-02", r.Next); err == nil {
			r.Next = t.AddDate(0, max(1, cadenceMonths[r.Cadence]), 0).Format("2006-01-02")
		}
	}
	if r.Day <= 0 || r.EveryDays > 0 || (r.Cadence != "" && r.Cadence != "monthly") {
		return
	}
	at := func(m time.Time) string {
		dim := time.Date(m.Year(), m.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		return time.Date(m.Year(), m.Month(), min(r.Day, dim), 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	if r.Done {
		r.Next = at(first.AddDate(0, 1, 0))
		return
	}
	r.Next = at(first)
	r.Overdue = r.Next < now.Format("2006-01-02")
}

// rollForward moves a past next-date on by its cadence until it is today or later.
func rollForward(next, cadence, today string) string {
	t, err := time.Parse("2006-01-02", next)
	if err != nil {
		return next
	}
	m := cadenceMonths[cadence]
	if m == 0 {
		m = 1
	}
	for i := 0; t.Format("2006-01-02") < today && i < 600; i++ {
		t = t.AddDate(0, m, 0)
	}
	return t.Format("2006-01-02")
}

// RecurringCosts is the recurring list of costs everyone uses (totals, FI,
// coming up): detected and saved bills only — money movements between your
// own accounts are not costs.
func RecurringCosts(d *sql.DB, txs []ledger.Tx, now time.Time) (list, hidden []Recurring, err error) {
	all, hid, err := RecurringAll(d, txs, now)
	if err != nil {
		return nil, nil, err
	}
	return onlyKind(all, "bill"), onlyKind(hid, "bill"), nil
}

// RecurringAll is every recurring money movement: bills, transfers (detected
// standing orders and planned ones) and income.
func RecurringAll(d *sql.DB, txs []ledger.Tx, now time.Time) (list, hidden []Recurring, err error) {
	items, err := ListRecurringItems(d)
	if err != nil {
		return nil, nil, err
	}
	names := map[string]string{}
	if accts, err := ledger.ListAccounts(d); err == nil {
		for _, a := range accts {
			names[a.ID] = a.Name
		}
	}
	detected := append(DetectRecurring(txs, now), DetectTransfers(txs, now, names)...)
	list, hidden = mergeRecurring(detected, items, now, lastPayments(txs, items))
	return list, hidden, nil
}

func onlyKind(rs []Recurring, kind string) []Recurring {
	out := []Recurring{}
	for _, r := range rs {
		if r.Kind == kind || (kind == "bill" && r.Kind == "") {
			out = append(out, r)
		}
	}
	return out
}

// DetectTransfers finds standing orders: transfers between the same two
// accounts in at least three of the last six months, on about the same day
// (within four days) and for about the same amount (±15%), the last one
// within 45 days. Irregular moves (top-ups when needed) are left out.
func DetectTransfers(txs []ledger.Tx, now time.Time, names map[string]string) []Recurring {
	since := now.AddDate(0, -6, 0).Format("2006-01-02")
	active := now.AddDate(0, 0, -45).Format("2006-01-02")
	type key struct{ from, to, cat string }
	by := map[key][]ledger.Tx{}
	for _, t := range txs {
		if t.Kind != "transfer" || t.Date < since || t.AccountID == "" || t.ToAccountID == "" || t.RecurringID != 0 {
			continue
		}
		k := key{t.AccountID, t.ToAccountID, t.Category}
		by[k] = append(by[k], t)
	}
	var out []Recurring
	for k, ts := range by {
		sort.Slice(ts, func(i, j int) bool { return ts[i].Date < ts[j].Date })
		months := map[string]bool{}
		var amts []float64
		var days []int
		merchants := map[string]int{}
		for _, t := range ts {
			months[t.Date[:7]] = true
			amts = append(amts, t.Amount.Float())
			days = append(days, dayOfMonth(t.Date))
			if t.Merchant != "" {
				merchants[t.Merchant]++
			}
		}
		last := ts[len(ts)-1]
		if len(months) < 3 || len(ts) > len(months)+1 || last.Date < active {
			continue
		}
		sort.Float64s(amts)
		med := amts[len(amts)/2]
		near := 0
		for _, a := range amts {
			if math.Abs(a-med) <= 0.15*med {
				near++
			}
		}
		sort.Ints(days)
		if near*10 < len(amts)*8 || days[len(days)-1]-days[0] > 4 {
			continue
		}
		name := ""
		for m, n := range merchants {
			if n > merchants[name] || (n == merchants[name] && m < name) {
				name = m
			}
		}
		if name == "" {
			to := firstNonEmpty(names[k.to], k.to)
			if k.cat == "transfer.debt" {
				name = to + " principal"
			} else {
				name = "To " + to
			}
		}
		out = append(out, Recurring{Source: "detected", Kind: "transfer", Merchant: name, Category: k.cat, Cadence: "monthly",
			Amount: money.FromFloat(med), Monthly: money.FromFloat(med), Last: last.Date, Count: len(ts), LastAmount: last.Amount,
			FromAccount: k.from, ToAccount: k.to, Day: days[len(days)/2]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Monthly > out[j].Monthly })
	return out
}

func dayOfMonth(date string) int {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0
	}
	return t.Day()
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// lastPayments finds each saved item's most recent payment: a transaction
// marked as it first of all; else by merchant, or — for a habit without one
// (haircuts at whichever salon) — by category at a similar amount (±25%).
// A transaction marked as one item never counts for another.
func lastPayments(txs []ledger.Tx, items []RecurringItem) map[int64]string {
	out := map[int64]string{}
	siblings := map[string][]RecurringItem{}
	for _, it := range items {
		siblings[strings.ToLower(it.Merchant)] = append(siblings[strings.ToLower(it.Merchant)], it)
	}
	for _, it := range items {
		sib := siblings[strings.ToLower(it.Merchant)]
		for i := range txs {
			t := &txs[i]
			if t.Date > out[it.ID] && itemPays(it, sib, t) {
				out[it.ID] = t.Date
			}
		}
	}
	return out
}

// itemPays: is t a payment of it? sib are the items sharing its merchant.
func itemPays(it RecurringItem, sib []RecurringItem, t *ledger.Tx) bool {
	if t.RecurringID != 0 {
		return t.RecurringID == it.ID
	}
	if it.Kind != "transfer" && it.Kind != "income" && t.Kind != "expense" {
		return false
	}
	m := strings.ToLower(it.Merchant)
	byMerchant := t.Merchant != "" && strings.ToLower(t.Merchant) == m
	similar := it.Amount > 0 && math.Abs(t.Amount.Float()-it.Amount.Float()) <= 0.25*it.Amount.Float()
	if byMerchant && len(sib) > 1 && !noteFits(it, sib, t) {
		byMerchant = false
	}
	switch it.Kind {
	case "transfer": // the same accounts (and a similar amount, or the name)
		return t.Kind == "transfer" && (it.FromAccount != "" || it.ToAccount != "") &&
			(it.FromAccount == "" || t.AccountID == it.FromAccount) && (it.ToAccount == "" || t.ToAccountID == it.ToAccount) && (similar || byMerchant)
	case "income": // into the same account, by name or a similar amount
		return t.Kind == "income" && (it.ToAccount == "" || t.AccountID == it.ToAccount) && (byMerchant || (similar && it.Category != "" && t.Category == it.Category))
	}
	return byMerchant || (it.Category != "" && t.Category == it.Category && similar)
}

// MatchingItem is the saved item a transaction pays: the one it is marked
// as, else one whose merchant it carries, else one whose habit (category at
// a similar amount) it fits — and how it matched: marked | merchant | habit.
func MatchingItem(t *ledger.Tx, items []RecurringItem) (id int64, by string) {
	if t.RecurringID != 0 {
		for _, it := range items {
			if it.ID == t.RecurringID {
				return it.ID, "marked"
			}
		}
	}
	siblings := map[string][]RecurringItem{}
	for _, it := range items {
		siblings[strings.ToLower(it.Merchant)] = append(siblings[strings.ToLower(it.Merchant)], it)
	}
	probe := *t
	probe.RecurringID = 0
	for _, merchantOnly := range []bool{true, false} {
		for _, it := range items {
			named := probe.Merchant != "" && strings.EqualFold(probe.Merchant, it.Merchant)
			if it.Hidden || named != merchantOnly || !itemPays(it, siblings[strings.ToLower(it.Merchant)], &probe) {
				continue
			}
			if merchantOnly {
				return it.ID, "merchant"
			}
			return it.ID, "habit"
		}
	}
	return 0, ""
}

// itemKind is the recurring kind a transaction would be: bill for spending.
func itemKind(t *ledger.Tx) string {
	switch t.Kind {
	case "income":
		return "income"
	case "transfer":
		return "transfer"
	}
	return "bill"
}

// LinkTx marks t as a payment of a recurring item: id (it must be the same
// kind — an income never pays a bill), a new monthly item made from t when
// create (reusing the item its merchant already has, of the same kind and
// closest in amount, shown again if it was hidden), or none (id 0, not
// create). names are account names, for naming a transfer with no merchant.
func LinkTx(d *sql.DB, t *ledger.Tx, create bool, id int64, names map[string]string) (*RecurringItem, error) {
	items, err := ListRecurringItems(d)
	if err != nil {
		return nil, err
	}
	kind := itemKind(t)
	var item *RecurringItem
	switch {
	case create:
		it := ItemFromTx(t)
		if it.Merchant == "" && kind == "transfer" && names[t.ToAccountID] != "" {
			it.Merchant = "To " + names[t.ToAccountID]
		}
		if it.Merchant == "" {
			return nil, errors.New("give it a merchant or a note first — that is the recurring payment's name")
		}
		best := -1
		for i := range items {
			if items[i].Kind == kind && strings.EqualFold(items[i].Merchant, it.Merchant) &&
				(best < 0 || math.Abs(items[i].Amount.Float()-t.Amount.Float()) < math.Abs(items[best].Amount.Float()-t.Amount.Float())) {
				best = i
			}
		}
		if best >= 0 {
			item = &items[best]
			if item.Hidden { // dismissed once; marking a payment says it is recurring after all
				item.Hidden = false
				if err := SaveRecurringItem(d, item); err != nil {
					return nil, err
				}
			}
		} else {
			if err := SaveRecurringItem(d, &it); err != nil {
				return nil, err
			}
			item = &it
		}
	case id != 0:
		for i := range items {
			if items[i].ID == id {
				item = &items[i]
			}
		}
		if item == nil {
			return nil, errors.New("that recurring payment doesn't exist")
		}
		if item.Kind != kind {
			return nil, fmt.Errorf("%s is a recurring %s — this transaction can't pay it", item.Merchant, item.Kind)
		}
	}
	link := int64(0)
	if item != nil {
		link = item.ID
	}
	return item, ledger.SetRecurring(d, t.ID, link)
}

// ItemFromTx is a monthly recurring item shaped like the payment: its
// merchant (or note), category, amount, accounts and day of the month.
func ItemFromTx(t *ledger.Tx) RecurringItem {
	name := firstNonEmpty(t.Merchant, t.Note)
	if r := []rune(name); len(r) > 60 {
		name = strings.TrimSpace(string(r[:60]))
	}
	it := RecurringItem{Merchant: name, Category: t.Category, Cadence: "monthly", Amount: t.Amount, Day: dayOfMonth(t.Date)}
	switch itemKind(t) {
	case "income":
		it.Kind, it.ToAccount = "income", t.AccountID
	case "transfer":
		it.Kind, it.FromAccount, it.ToAccount = "transfer", t.AccountID, t.ToAccountID
	default:
		it.Kind, it.FromAccount = "bill", t.AccountID
	}
	return it
}

func addDays(date string, n int) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

// rollDays moves a past date on by n-day steps until it is today or later.
func rollDays(next string, n int, today string) string {
	t, err := time.Parse("2006-01-02", next)
	if err != nil || n <= 0 {
		return next
	}
	for i := 0; t.Format("2006-01-02") < today && i < 600; i++ {
		t = t.AddDate(0, 0, n)
	}
	return t.Format("2006-01-02")
}

// noteFits tells which of several items for one merchant a payment is: the
// one whose note shares a word with the payment's note, else — when no
// sibling's note fits either — the one closest in amount.
func noteFits(it RecurringItem, sib []RecurringItem, t *ledger.Tx) bool {
	hit := func(x RecurringItem) bool {
		tn := strings.ToLower(t.Note)
		for _, w := range strings.Fields(strings.ToLower(x.Note)) {
			if len([]rune(w)) >= 3 && strings.Contains(tn, w) {
				return true
			}
		}
		return false
	}
	if hit(it) {
		return true
	}
	best, diff := int64(0), math.MaxFloat64
	for _, x := range sib {
		if hit(x) {
			return false
		}
		if d := math.Abs(t.Amount.Float() - x.Amount.Float()); d < diff {
			best, diff = x.ID, d
		}
	}
	return best == it.ID
}
