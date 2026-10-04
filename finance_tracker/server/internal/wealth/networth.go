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
	rows, err := d.Query(`SELECT account_id,date,value,quantity,price,source FROM balances ORDER BY account_id, date`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p Point
		var q sql.NullFloat64
		var pr sql.NullInt64
		if err := rows.Scan(&p.AccountID, &p.Date, &p.Value, &q, &pr, &p.Source); err != nil {
			return nil, err
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
	Liquid    money.Cents            `json:"liquid"` // liquid accounts only (cash, brokers, crypto)
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
	return err
}

// DeleteBalance removes one point.
func DeleteBalance(e interface {
	Exec(string, ...any) (sql.Result, error)
}, account, date string) error {
	_, err := e.Exec(`DELETE FROM balances WHERE account_id=? AND date=?`, account, date)
	return err
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
