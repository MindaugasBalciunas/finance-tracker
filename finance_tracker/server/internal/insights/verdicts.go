package insights

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"ft/internal/db"
	"ft/internal/money"
)

// Verdicts: the owner's own judgement on a regular optional place or
// category — worth it (quality of life), cut (a leak) or not sure. The data
// can show patterns; only the owner can say what a bar tab or a gym is worth
// to them. "usual" is what it cost a month when judged, so a cut can be
// followed up against it.

type Verdict struct {
	Kind     string      `json:"kind"` // merchant | category
	Key      string      `json:"key"`  // merchant key or category id
	Name     string      `json:"name"`
	Verdict  string      `json:"verdict"` // worth | cut | unsure
	Usual    money.Cents `json:"usual"`
	JudgedOn string      `json:"judged_on"`
}

func validVerdict(v Verdict) error {
	if v.Kind != "merchant" && v.Kind != "category" {
		return errors.New("a verdict is about a place or a category")
	}
	if strings.TrimSpace(v.Key) == "" {
		return errors.New("say which place or category")
	}
	if v.Verdict != "worth" && v.Verdict != "cut" && v.Verdict != "unsure" {
		return errors.New("the verdict is worth it, cut or not sure")
	}
	return nil
}

// SaveVerdict records or changes a verdict. Changing it restarts the
// follow-up from today and today's usual.
func SaveVerdict(e interface {
	Exec(string, ...any) (sql.Result, error)
}, v Verdict, now time.Time) error {
	if err := validVerdict(v); err != nil {
		return err
	}
	_, err := e.Exec(`INSERT INTO optional_verdicts(kind,key,name,verdict,usual,judged_on,updated_at) VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(kind,key) DO UPDATE SET name=excluded.name, verdict=excluded.verdict, usual=excluded.usual, judged_on=excluded.judged_on, updated_at=excluded.updated_at`,
		v.Kind, v.Key, v.Name, v.Verdict, int64(v.Usual), now.Format("2006-01-02"), db.Now())
	return err
}

func DeleteVerdict(e interface {
	Exec(string, ...any) (sql.Result, error)
}, kind, key string) error {
	_, err := e.Exec(`DELETE FROM optional_verdicts WHERE kind=? AND key=?`, kind, key)
	return err
}

// Verdicts returns every verdict by "kind:key".
func Verdicts(q interface {
	Query(string, ...any) (*sql.Rows, error)
}) (map[string]Verdict, error) {
	rows, err := q.Query(`SELECT kind,key,name,verdict,usual,judged_on FROM optional_verdicts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Verdict{}
	for rows.Next() {
		var v Verdict
		var usual int64
		if err := rows.Scan(&v.Kind, &v.Key, &v.Name, &v.Verdict, &usual, &v.JudgedOn); err != nil {
			return nil, err
		}
		v.Usual = money.Cents(usual)
		out[v.Kind+":"+v.Key] = v
	}
	return out, rows.Err()
}
