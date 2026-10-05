package ledger

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"ft/internal/db"
	"ft/internal/money"
)

type Tx struct {
	ID          int64       `json:"id"`
	Date        string      `json:"date"`
	Kind        string      `json:"kind"` // income | expense | transfer
	Amount      money.Cents `json:"amount"`
	AccountID   string      `json:"account_id"`
	ToAccountID string      `json:"to_account_id"`
	Category    string      `json:"category"`
	Merchant    string      `json:"merchant"`
	Note        string      `json:"note"`
	Tags        []string    `json:"tags"`
	ExternalID  string      `json:"external_id,omitempty"`
	SplitOf     int64       `json:"split_of,omitempty"`
	Source      string      `json:"source"`
	Pending     bool        `json:"pending,omitempty"` // a card reservation the bank hasn't booked yet
	CreatedAt   string      `json:"created_at,omitempty"`
}

// Signed is the amount as it affects the owner's net cash: income +,
// expense −, transfers 0.
func (t Tx) Signed() money.Cents {
	switch t.Kind {
	case "income":
		return t.Amount
	case "expense":
		return -t.Amount
	}
	return 0
}

const txCols = `id,date,kind,amount,COALESCE(account_id,''),COALESCE(to_account_id,''),category,merchant,note,tags,COALESCE(external_id,''),COALESCE(split_of,0),source,pending,created_at`

func scanTx(s interface{ Scan(...any) error }) (Tx, error) {
	var t Tx
	var tags string
	err := s.Scan(&t.ID, &t.Date, &t.Kind, &t.Amount, &t.AccountID, &t.ToAccountID, &t.Category, &t.Merchant, &t.Note, &tags, &t.ExternalID, &t.SplitOf, &t.Source, &t.Pending, &t.CreatedAt)
	t.Tags = SplitTags(tags)
	if t.Tags == nil {
		t.Tags = []string{}
	}
	return t, err
}

// Filter narrows a transaction query. Zero values mean "any".
type Filter struct {
	From, To   string   // inclusive dates
	Kind       string   // income | expense | transfer
	Categories []string // ids; a parent id includes its children
	Accounts   []string
	Tags       []string
	TagsAll    bool // all tags must be present (default: any)
	Merchant   string
	Query      string // free text over merchant + note
	Min, Max   money.Cents
	Source     string
	Limit      int
	Offset     int
	Sort       string // date | amount (desc); prefix '+' for ascending
}

func (f Filter) where() (string, []any) {
	var conds []string
	var args []any
	if f.From != "" {
		conds = append(conds, "date >= ?")
		args = append(args, f.From)
	}
	if f.To != "" {
		conds = append(conds, "date <= ?")
		args = append(args, f.To)
	}
	if f.Kind != "" {
		conds = append(conds, "kind = ?")
		args = append(args, f.Kind)
	}
	if len(f.Categories) > 0 {
		var or []string
		for _, c := range f.Categories {
			or = append(or, "category = ? OR category LIKE ?")
			args = append(args, c, c+".%")
		}
		conds = append(conds, "("+strings.Join(or, " OR ")+")")
	}
	if len(f.Accounts) > 0 {
		var or []string
		for _, a := range f.Accounts {
			or = append(or, "account_id = ? OR to_account_id = ?")
			args = append(args, a, a)
		}
		conds = append(conds, "("+strings.Join(or, " OR ")+")")
	}
	if len(f.Tags) > 0 {
		var parts []string
		for _, t := range NormalizeTags(f.Tags) {
			parts = append(parts, "tags LIKE ?")
			args = append(args, "%,"+t+",%")
		}
		joiner := " OR "
		if f.TagsAll {
			joiner = " AND "
		}
		conds = append(conds, "("+strings.Join(parts, joiner)+")")
	}
	if f.Merchant != "" {
		conds = append(conds, "merchant = ?")
		args = append(args, f.Merchant)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		conds = append(conds, "(merchant LIKE ? OR note LIKE ? OR tags LIKE ?)")
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	if f.Min > 0 {
		conds = append(conds, "amount >= ?")
		args = append(args, int64(f.Min))
	}
	if f.Max > 0 {
		conds = append(conds, "amount <= ?")
		args = append(args, int64(f.Max))
	}
	if f.Source != "" {
		conds = append(conds, "source = ?")
		args = append(args, f.Source)
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// ListResult is a page of transactions plus totals over the whole filter.
type ListResult struct {
	Items    []Tx        `json:"items"`
	Total    int         `json:"total"`
	Income   money.Cents `json:"income"`
	Expense  money.Cents `json:"expense"`
	Transfer money.Cents `json:"transfer"`
}

func List(q querier, f Filter) (ListResult, error) {
	where, args := f.where()
	var res ListResult
	err := q.QueryRow(`SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN kind='income' THEN amount END),0),
		COALESCE(SUM(CASE WHEN kind='expense' THEN amount END),0),
		COALESCE(SUM(CASE WHEN kind='transfer' THEN amount END),0)
		FROM transactions`+where, args...).Scan(&res.Total, &res.Income, &res.Expense, &res.Transfer)
	if err != nil {
		return res, err
	}
	order := "date DESC, id DESC"
	switch f.Sort {
	case "amount":
		order = "amount DESC, date DESC"
	case "+amount":
		order = "amount ASC, date DESC"
	case "+date":
		order = "date ASC, id ASC"
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 20000 {
		limit = 20000
	}
	rows, err := q.Query(`SELECT `+txCols+` FROM transactions`+where+` ORDER BY `+order+` LIMIT ? OFFSET ?`,
		append(args, limit, f.Offset)...)
	if err != nil {
		return res, err
	}
	defer rows.Close()
	res.Items = []Tx{}
	for rows.Next() {
		t, err := scanTx(rows)
		if err != nil {
			return res, err
		}
		res.Items = append(res.Items, t)
	}
	return res, rows.Err()
}

// All streams every row matching f (no paging) — for analytics and export.
func All(q querier, f Filter) ([]Tx, error) {
	f.Limit, f.Offset = 1_000_000, 0
	where, args := f.where()
	rows, err := q.Query(`SELECT `+txCols+` FROM transactions`+where+` ORDER BY date, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tx
	for rows.Next() {
		t, err := scanTx(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func Get(q querier, id int64) (Tx, error) {
	return scanTx(q.QueryRow(`SELECT `+txCols+` FROM transactions WHERE id=?`, id))
}

// Validate checks a transaction against the category and account tables.
func Validate(q querier, t *Tx) error {
	t.Merchant = strings.TrimSpace(t.Merchant)
	t.Note = strings.TrimSpace(t.Note)
	t.Tags = NormalizeTags(t.Tags)
	if _, err := time.Parse("2006-01-02", t.Date); err != nil {
		return fmt.Errorf("invalid date %q", t.Date)
	}
	if t.Amount <= 0 {
		return errors.New("amount must be positive")
	}
	var catKind string
	if err := q.QueryRow(`SELECT kind FROM categories WHERE id=?`, t.Category).Scan(&catKind); err != nil {
		return fmt.Errorf("unknown category %q", t.Category)
	}
	if t.Kind == "" {
		t.Kind = catKind
	}
	if t.Kind != catKind {
		return fmt.Errorf("category %q is for %s, not %s", t.Category, catKind, t.Kind)
	}
	// Property and vehicles are tracked by valuation: money is never paid
	// from or into them. Loans and pensions can only receive transfers.
	for i, id := range []string{t.AccountID, t.ToAccountID} {
		if id == "" {
			continue
		}
		var kind string
		if err := q.QueryRow(`SELECT kind FROM accounts WHERE id=?`, id).Scan(&kind); err != nil {
			return fmt.Errorf("unknown account %q", id)
		}
		switch {
		case kind == "property" || kind == "vehicle":
			return fmt.Errorf("%s is valued, not paid from — pick a bank, cash or broker account", id)
		case i == 0 && (kind == "loan" || kind == "pension") && t.Kind != "transfer":
			return fmt.Errorf("%s can't pay for things — pick a bank or cash account", id)
		}
	}
	if t.Kind != "transfer" {
		t.ToAccountID = ""
	}
	if t.Source == "" {
		t.Source = "manual"
	}
	return nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(i int64) any {
	if i == 0 {
		return nil
	}
	return i
}

// Insert writes a validated transaction.
func Insert(e execer, t *Tx) error {
	now := db.Now()
	res, err := e.Exec(`INSERT INTO transactions(date,kind,amount,account_id,to_account_id,category,merchant,note,tags,external_id,split_of,source,pending,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.Date, t.Kind, int64(t.Amount), nullStr(t.AccountID), nullStr(t.ToAccountID), t.Category, t.Merchant, t.Note,
		JoinTags(t.Tags), nullStr(t.ExternalID), nullInt(t.SplitOf), t.Source, t.Pending, now, now)
	if err != nil {
		return err
	}
	t.ID, _ = res.LastInsertId()
	t.CreatedAt = now
	return nil
}

// Update rewrites the editable fields of a transaction.
func Update(e execer, t *Tx) error {
	_, err := e.Exec(`UPDATE transactions SET date=?,kind=?,amount=?,account_id=?,to_account_id=?,category=?,merchant=?,note=?,tags=?,updated_at=? WHERE id=?`,
		t.Date, t.Kind, int64(t.Amount), nullStr(t.AccountID), nullStr(t.ToAccountID), t.Category, t.Merchant, t.Note,
		JoinTags(t.Tags), db.Now(), t.ID)
	return err
}

// SettleReservation turns a pending card reservation into the booked
// transaction: the bank's final amount, date and id; the owner's category,
// merchant, tags and notes stay as they are.
func SettleReservation(e execer, id int64, amount money.Cents, date, externalID string) error {
	_, err := e.Exec(`UPDATE transactions SET amount=?, date=?, external_id=?, pending=0, updated_at=? WHERE id=? AND pending=1`,
		int64(amount), date, externalID, db.Now(), id)
	return err
}

// DropReservation removes a reservation the bank released without booking
// (and any split parts made from it). Booked transactions are never touched.
func DropReservation(e execer, id int64) (bool, error) {
	res, err := e.Exec(`DELETE FROM transactions WHERE (id=? OR split_of=?) AND (pending=1 OR split_of=?)
		AND EXISTS (SELECT 1 FROM transactions WHERE id=? AND pending=1)`, id, id, id, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func Delete(e execer, id int64) error {
	_, err := e.Exec(`DELETE FROM transactions WHERE id=?`, id)
	return err
}

// SplitPart is one share of a split transaction.
type SplitPart struct {
	Amount   money.Cents `json:"amount"`
	Category string      `json:"category"`
	Note     string      `json:"note"`
	Tags     []string    `json:"tags"`
	OwedBy   string      `json:"owed_by"` // someone owes you this part
}

// OwedTag marks a part someone owes back.
func OwedTag(person string) string {
	p := strings.ToLower(strings.TrimSpace(person))
	if p == "" {
		return ""
	}
	return "owed:" + p
}

// Split carves parts off a transaction. The parent keeps the remainder, so
// the total — and every balance — is unchanged; each part is a normal row
// that reports and budgets see without knowing splits exist.
func Split(d *sql.DB, id int64, parts []SplitPart) ([]Tx, error) {
	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	parent, err := Get(tx, id)
	if err != nil {
		return nil, errors.New("transaction not found")
	}
	if parent.SplitOf != 0 {
		return nil, errors.New("split the original transaction, not a part")
	}
	var sum money.Cents
	var out []Tx
	for _, p := range parts {
		if p.Amount <= 0 {
			return nil, errors.New("every part needs a positive amount")
		}
		sum += p.Amount
		child := Tx{
			Date: parent.Date, Kind: parent.Kind, Amount: p.Amount, AccountID: parent.AccountID, ToAccountID: parent.ToAccountID,
			Category: firstNonEmpty(p.Category, parent.Category), Merchant: parent.Merchant,
			Note: firstNonEmpty(p.Note, parent.Note), Tags: p.Tags, SplitOf: parent.ID, Source: parent.Source,
		}
		if ot := OwedTag(p.OwedBy); ot != "" {
			child.Tags = append(child.Tags, ot)
		}
		if err := Validate(tx, &child); err != nil {
			return nil, err
		}
		if err := Insert(tx, &child); err != nil {
			return nil, err
		}
		out = append(out, child)
	}
	if sum >= parent.Amount {
		return nil, errors.New("the parts must add up to less than the original — the original keeps the rest")
	}
	parent.Amount -= sum
	if err := Update(tx, &parent); err != nil {
		return nil, err
	}
	return out, tx.Commit()
}

// Unsplit folds every part back into its parent.
func Unsplit(d *sql.DB, id int64) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var sum int64
	tx.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM transactions WHERE split_of=?`, id).Scan(&sum)
	if sum == 0 {
		return errors.New("this transaction has no parts")
	}
	if _, err := tx.Exec(`UPDATE transactions SET amount=amount+?, updated_at=? WHERE id=?`, sum, db.Now(), id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM transactions WHERE split_of=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// TagCount is one tag with its usage.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
	Last  string `json:"last"`
}

// ListTags aggregates every tag in use.
func ListTags(q querier) ([]TagCount, error) {
	rows, err := q.Query(`SELECT tags, date FROM transactions WHERE tags != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]*TagCount{}
	for rows.Next() {
		var tags, date string
		rows.Scan(&tags, &date)
		for _, t := range SplitTags(tags) {
			c := m[t]
			if c == nil {
				c = &TagCount{Tag: t}
				m[t] = c
			}
			c.Count++
			if date > c.Last {
				c.Last = date
			}
		}
	}
	out := make([]TagCount, 0, len(m))
	for _, c := range m {
		out = append(out, *c)
	}
	return out, nil
}

// RenameTag renames (or with to=="" removes) a tag on every transaction.
func RenameTag(d *sql.DB, from, to string) (int, error) {
	from = strings.ToLower(strings.TrimSpace(from))
	rows, err := d.Query(`SELECT id, tags FROM transactions WHERE tags LIKE ?`, "%,"+from+",%")
	if err != nil {
		return 0, err
	}
	type upd struct {
		id   int64
		tags string
	}
	var ups []upd
	for rows.Next() {
		var u upd
		rows.Scan(&u.id, &u.tags)
		var next []string
		for _, t := range SplitTags(u.tags) {
			if t == from {
				if to != "" {
					next = append(next, to)
				}
				continue
			}
			next = append(next, t)
		}
		u.tags = JoinTags(next)
		ups = append(ups, u)
	}
	rows.Close()
	tx, err := d.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, u := range ups {
		if _, err := tx.Exec(`UPDATE transactions SET tags=?, updated_at=? WHERE id=?`, u.tags, db.Now(), u.id); err != nil {
			return 0, err
		}
	}
	// A budget counting this tag follows the rename; one whose tag is
	// removed is archived rather than left matching nothing.
	if to != "" {
		tx.Exec(`UPDATE budgets SET tag=? WHERE tag=?`, to, from)
	} else {
		tx.Exec(`UPDATE budgets SET archived=1 WHERE tag=? AND category=''`, from)
	}
	if _, err := tx.Exec(`UPDATE rules SET add_tags = TRIM(REPLACE(','||add_tags||',', ','||?||',', ','||?||','), ',') WHERE add_tags LIKE ?`, from, to, "%"+from+"%"); err != nil {
		return 0, err
	}
	return len(ups), tx.Commit()
}

// MerchantCount aggregates merchants for autocomplete and reports.
type MerchantCount struct {
	Merchant string      `json:"merchant"`
	Count    int         `json:"count"`
	Total    money.Cents `json:"total"`
	Category string      `json:"category"` // most frequent
	Last     string      `json:"last"`
}

func ListMerchants(q querier, from, to string) ([]MerchantCount, error) {
	rows, err := q.Query(`SELECT merchant, COUNT(*), SUM(amount), MAX(date),
		(SELECT category FROM transactions t2 WHERE t2.merchant=t.merchant GROUP BY category ORDER BY COUNT(*) DESC LIMIT 1)
		FROM transactions t WHERE merchant != '' AND kind='expense' AND date >= ? AND date <= ?
		GROUP BY merchant ORDER BY SUM(amount) DESC`, orMin(from), orMax(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MerchantCount
	for rows.Next() {
		var m MerchantCount
		rows.Scan(&m.Merchant, &m.Count, &m.Total, &m.Last, &m.Category)
		out = append(out, m)
	}
	return out, nil
}

func orMin(s string) string {
	if s == "" {
		return "0000-00-00"
	}
	return s
}

func orMax(s string) string {
	if s == "" {
		return "9999-99-99"
	}
	return s
}
