// Package wealth answers "what do I own and owe": balances, net worth over
// time, investments and loans.
package wealth

import (
	"database/sql"
	"errors"
	"sort"
	"time"

	"ft/internal/db"
	"ft/internal/ledger"
	"ft/internal/money"
)

// Point is one account value on one day.
type Point struct {
	AccountID string       `json:"account_id"`
	Date      string       `json:"date"`
	Value     money.Cents  `json:"value"`
	Quantity  *float64     `json:"quantity,omitempty"`
	Price     *money.Cents `json:"price,omitempty"`
	Source    string       `json:"source"`
	// At is when the value was recorded (RFC3339), set only when that was
	// on the day itself — an import made later has no meaningful time.
	At string `json:"at,omitempty"`
}

// Book is every balance point, indexed for carry-forward lookups.
type Book struct {
	Accounts map[string]ledger.Account
	series   map[string][]Point // per account, ascending by date
}

// LoadBook reads every account and balance.
func LoadBook(d *sql.DB) (*Book, error) {
	accts, err := ledger.ListAccounts(d)
	if err != nil {
		return nil, err
	}
	b := &Book{Accounts: map[string]ledger.Account{}, series: map[string][]Point{}}
	for _, a := range accts {
		b.Accounts[a.ID] = a
	}
	rows, err := d.Query(`SELECT account_id,date,value,quantity,price,source,updated_at FROM balances ORDER BY account_id, date`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p Point
		var q sql.NullFloat64
		var pr sql.NullInt64
		var upd string
		if err := rows.Scan(&p.AccountID, &p.Date, &p.Value, &q, &pr, &p.Source, &upd); err != nil {
			return nil, err
		}
		if p.Source != "import" && p.Source != "computed" {
			if t, err := time.Parse(time.RFC3339, upd); err == nil && t.Local().Format("2006-01-02") == p.Date {
				p.At = upd
			}
		}
		if q.Valid {
			v := q.Float64
			p.Quantity = &v
		}
		if pr.Valid {
			v := money.Cents(pr.Int64)
			p.Price = &v
		}
		b.series[p.AccountID] = append(b.series[p.AccountID], p)
	}
	return b, rows.Err()
}

// At returns an account's latest point on or before date.
func (b *Book) At(account, date string) (Point, bool) {
	s := b.series[account]
	i := sort.Search(len(s), func(i int) bool { return s[i].Date > date })
	if i == 0 {
		return Point{}, false
	}
	return s[i-1], true
}

// Series returns all points of one account.
func (b *Book) Series(account string) []Point { return b.series[account] }

// FirstDate is the earliest balance on record.
func (b *Book) FirstDate() string {
	first := ""
	for _, s := range b.series {
		if len(s) > 0 && (first == "" || s[0].Date < first) {
			first = s[0].Date
		}
	}
	return first
}

// Snapshot is net worth on one day.
type Snapshot struct {
	Date      string                 `json:"date"`
	NetWorth  money.Cents            `json:"net_worth"`
	Liquid    money.Cents            `json:"liquid"` // liquid accounts only (cash, brokers, pensions, crypto)
	Assets    money.Cents            `json:"assets"`
	Debt      money.Cents            `json:"debt"` // positive number
	ByGroup   map[string]money.Cents `json:"by_group"`
	ByAccount map[string]money.Cents `json:"by_account,omitempty"`
	Stale     map[string]string      `json:"stale,omitempty"` // account → date of its last value, when older than 45 days
}

// SnapshotAt computes net worth on a date, carrying each account's latest
// value forward. Archived accounts count only up to their last value.
func (b *Book) SnapshotAt(date string, withAccounts bool) Snapshot {
	s := Snapshot{Date: date, ByGroup: map[string]money.Cents{}}
	if withAccounts {
		s.ByAccount = map[string]money.Cents{}
	}
	d, _ := time.Parse("2006-01-02", date)
	for id, a := range b.Accounts {
		p, ok := b.At(id, date)
		if !ok {
			continue
		}
		v := p.Value
		s.NetWorth += v
		s.ByGroup[a.Group] += v
		if v < 0 {
			s.Debt -= v
		} else {
			s.Assets += v
		}
		if a.Liquid && v > 0 {
			s.Liquid += v
		}
		if withAccounts {
			s.ByAccount[id] = v
			if !a.Archived && a.Kind != "property" && a.Kind != "vehicle" && v != 0 {
				if pd, err := time.Parse("2006-01-02", p.Date); err == nil && d.Sub(pd) > 45*24*time.Hour {
					if s.Stale == nil {
						s.Stale = map[string]string{}
					}
					s.Stale[id] = p.Date
				}
			}
		}
	}
	return s
}

// History samples net worth at month ends (and today) from..to.
func (b *Book) History(from, to string, step string) []Snapshot {
	if from == "" {
		from = b.FirstDate()
	}
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}
	start, err1 := time.Parse("2006-01-02", from)
	end, err2 := time.Parse("2006-01-02", to)
	if err1 != nil || err2 != nil || start.After(end) {
		return nil
	}
	var out []Snapshot
	switch step {
	case "day":
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			out = append(out, b.SnapshotAt(d.Format("2006-01-02"), false))
		}
	case "week":
		for d := start; !d.After(end); d = d.AddDate(0, 0, 7) {
			out = append(out, b.SnapshotAt(d.Format("2006-01-02"), false))
		}
		// Always end on the last day, so a balance updated today shows up.
		if last := end.Format("2006-01-02"); len(out) > 0 && out[len(out)-1].Date != last {
			out = append(out, b.SnapshotAt(last, false))
		}
	default: // month ends
		cur := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
		for !cur.After(end) {
			me := cur.AddDate(0, 1, -1)
			if me.After(end) {
				me = end
			}
			out = append(out, b.SnapshotAt(me.Format("2006-01-02"), false))
			cur = cur.AddDate(0, 1, 0)
		}
	}
	return out
}

// SetBalance records an account value for a day (upsert).
func SetBalance(e interface {
	Exec(string, ...any) (sql.Result, error)
}, account, date string, value money.Cents, qty *float64, price *money.Cents, source string) error {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return errors.New("invalid date")
	}
	if source == "" {
		source = "manual"
	}
	var q, p any
	if qty != nil {
		q = *qty
	}
	if price != nil {
		p = int64(*price)
	}
	_, err := e.Exec(`INSERT INTO balances(account_id,date,value,quantity,price,source,updated_at) VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(account_id,date) DO UPDATE SET value=excluded.value, quantity=excluded.quantity, price=excluded.price,
		source=excluded.source, updated_at=excluded.updated_at`, account, date, int64(value), q, p, source, db.Now())
	if err != nil || date != time.Now().Format("2006-01-02") {
		return err
	}
	// Today's value: keep the reading with its time, unless it repeats the
	// last one logged today (a sync every half hour adds nothing new).
	_, err = e.Exec(`INSERT INTO balance_log(account_id,date,at,value,quantity,price,source)
		SELECT ?,?,?,?,?,?,? WHERE NOT EXISTS (
			SELECT 1 FROM (SELECT value, source FROM balance_log WHERE account_id=? AND date=? ORDER BY id DESC LIMIT 1) WHERE value=? AND source=?)`,
		account, date, db.Now(), int64(value), q, p, source, account, date, int64(value), source)
	return err
}

// Reading is one logged value of an account at a moment of the day.
type Reading struct {
	AccountID string      `json:"account_id"`
	At        string      `json:"at"`
	Value     money.Cents `json:"value"`
	Source    string      `json:"source"`
}

// Readings returns the intraday trail for the days from..to (inclusive),
// by day, oldest reading first.
func Readings(d *sql.DB, from, to string) (map[string][]Reading, error) {
	rows, err := d.Query(`SELECT date, account_id, at, value, source FROM balance_log WHERE date>=? AND date<=? ORDER BY date, at, id`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]Reading{}
	for rows.Next() {
		var day string
		var r Reading
		if err := rows.Scan(&day, &r.AccountID, &r.At, &r.Value, &r.Source); err != nil {
			return nil, err
		}
		out[day] = append(out[day], r)
	}
	return out, rows.Err()
}

// DeleteBalance removes one point.
func DeleteBalance(e interface {
	Exec(string, ...any) (sql.Result, error)
}, account, date string) error {
	if _, err := e.Exec(`DELETE FROM balances WHERE account_id=? AND date=?`, account, date); err != nil {
		return err
	}
	_, err := e.Exec(`DELETE FROM balance_log WHERE account_id=? AND date=?`, account, date)
	return err
}

// ClosedSource marks the €0 written when an account is hidden.
const ClosedSource = "closed"

// CloseAccount records €0 for today when the account still holds (or owes)
// something, so a hidden account stops counting from today on.
func CloseAccount(d *sql.DB, account string, now time.Time) error {
	var v int64
	if d.QueryRow(`SELECT value FROM balances WHERE account_id=? AND date<=? ORDER BY date DESC LIMIT 1`, account, now.Format("2006-01-02")).Scan(&v) != nil || v == 0 {
		return nil
	}
	return SetBalance(d, account, now.Format("2006-01-02"), 0, nil, nil, ClosedSource)
}

// ReopenAccount removes the closing €0 written by CloseAccount, if it is
// still the account's latest value.
func ReopenAccount(d *sql.DB, account string) error {
	var date, src string
	if d.QueryRow(`SELECT date, source FROM balances WHERE account_id=? ORDER BY date DESC LIMIT 1`, account).Scan(&date, &src) != nil || src != ClosedSource {
		return nil
	}
	return DeleteBalance(d, account, date)
}

// ApplyDelta moves an account's balance for a manual transaction, as v1 did:
// a transaction dated on or after the latest recorded value shifts it (a
// new point on the transaction's date); one dated earlier is already
// inside that recorded value and changes nothing. Bank-synced accounts are
// left alone — the bank's own balance is the truth there. Returns whether
// the balance moved.
func ApplyDelta(d interface {
	Exec(string, ...any) (sql.Result, error)
	QueryRow(string, ...any) *sql.Row
}, account, date string, delta money.Cents) (bool, error) {
	if account == "" || delta == 0 {
		return false, nil
	}
	var synced int
	d.QueryRow(`SELECT COUNT(*) FROM bank_accounts WHERE account_id=? AND uid != ''`, account).Scan(&synced)
	if synced > 0 {
		return false, nil
	}
	var kind string
	if d.QueryRow(`SELECT kind FROM accounts WHERE id=?`, account).Scan(&kind) != nil {
		return false, nil
	}
	switch kind {
	case "property", "vehicle", "crypto":
		return false, nil
	}
	var lastDate string
	var lastVal int64
	if err := d.QueryRow(`SELECT date, value FROM balances WHERE account_id=? ORDER BY date DESC LIMIT 1`, account).Scan(&lastDate, &lastVal); err != nil {
		return false, nil // never recorded: nothing to move
	}
	if date < lastDate {
		return false, nil
	}
	return true, SetBalance(d, account, date, money.Cents(lastVal)+delta, nil, nil, "manual")
}

// TxDeltas lists the balance moves a transaction implies.
func TxDeltas(kind, account, to string, amount money.Cents) map[string]money.Cents {
	out := map[string]money.Cents{}
	switch kind {
	case "expense":
		out[account] -= amount
	case "income":
		out[account] += amount
	case "transfer":
		out[account] -= amount
		out[to] += amount
	}
	delete(out, "")
	return out
}

// Move is one account's change across a window.
type Move struct {
	AccountID string      `json:"account_id"`
	Name      string      `json:"name"`
	Group     string      `json:"group"`
	Start     money.Cents `json:"start"`
	End       money.Cents `json:"end"`
	Change    money.Cents `json:"change"`
}

// Movement lists which accounts drove net worth between two dates, largest
// change first.
func (b *Book) Movement(from, to string) []Move {
	s, e := b.SnapshotAt(from, true), b.SnapshotAt(to, true)
	var out []Move
	for id, a := range b.Accounts {
		st, en := s.ByAccount[id], e.ByAccount[id]
		if st == 0 && en == 0 {
			continue
		}
		out = append(out, Move{AccountID: id, Name: a.Name, Group: a.Group, Start: st, End: en, Change: en - st})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Change.Abs() > out[j].Change.Abs() })
	return out
}

// TableCell is one account on one snapshot date: the value in force, and
// whether it was recorded that day (with its source) or carried forward.
type TableCell struct {
	Value    money.Cents `json:"value"`
	Recorded bool        `json:"recorded,omitempty"`
	Source   string      `json:"source,omitempty"`
	At       string      `json:"at,omitempty"`
}

type TableRow struct {
	Date     string               `json:"date"`
	Cells    map[string]TableCell `json:"cells"`
	Readings []Reading            `json:"readings,omitempty"` // every reading that day, with times`
	NetWorth money.Cents          `json:"net_worth"`
	Liquid   money.Cents          `json:"liquid"`
}

// BalanceTable pages through every date any balance was recorded, newest first.
type BalanceTable struct {
	Accounts []string   `json:"accounts"` // accounts with any recorded value
	Rows     []TableRow `json:"rows"`
	Page     int        `json:"page"`
	Pages    int        `json:"pages"`
	Dates    int        `json:"dates"`
	Size     int        `json:"size"`
}

// Table returns page (1-based) of size snapshot dates across all accounts.
func (b *Book) Table(page, size int) BalanceTable {
	if size <= 0 || size > 500 {
		size = 50
	}
	recorded := map[string]map[string]Point{} // account → date → point
	dateSet := map[string]bool{}
	var accounts []string
	for id, s := range b.series {
		if len(s) == 0 {
			continue
		}
		accounts = append(accounts, id)
		m := make(map[string]Point, len(s))
		for _, p := range s {
			m[p.Date] = p
			dateSet[p.Date] = true
		}
		recorded[id] = m
	}
	sort.Strings(accounts)
	dates := make([]string, 0, len(dateSet))
	for d := range dateSet {
		dates = append(dates, d)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))
	t := BalanceTable{Accounts: accounts, Rows: []TableRow{}, Dates: len(dates), Size: size}
	t.Pages = (len(dates) + size - 1) / size
	if t.Pages == 0 {
		t.Pages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > t.Pages {
		page = t.Pages
	}
	t.Page = page
	lo := (page - 1) * size
	hi := lo + size
	if hi > len(dates) {
		hi = len(dates)
	}
	for _, d := range dates[lo:hi] {
		snap := b.SnapshotAt(d, true)
		row := TableRow{Date: d, Cells: map[string]TableCell{}, NetWorth: snap.NetWorth, Liquid: snap.Liquid}
		for _, id := range accounts {
			c := TableCell{Value: snap.ByAccount[id]}
			if p, ok := recorded[id][d]; ok {
				c.Recorded, c.Source, c.Value, c.At = true, p.Source, p.Value, p.At
			}
			if c.Value != 0 || c.Recorded {
				row.Cells[id] = c
			}
		}
		t.Rows = append(t.Rows, row)
	}
	return t
}
