package logos

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

// Your own logos: for a merchant whose site refuses to share its icon (VMI,
// Bolt), or whose icon came out wrong, the owner uploads one. It lives in the
// settings — so it is in every backup — and wins over the automatic pick.
// Nothing is fetched for it: the bytes are checked to be a small image.

type custom struct {
	Name string `json:"name"` // the merchant as written
	Mime string `json:"mime"`
	Data string `json:"data"` // base64
}

// One settings row per logo ("logo_custom:<merchant key>"): an upload
// rewrites only its own row, and showing one logo reads only that one.
const (
	customPrefix = "logo_custom:"
	maxCustom    = 200
)

func customKeys(d *sql.DB) []string {
	rows, err := d.Query(`SELECT substr(key, ?) FROM settings WHERE key LIKE ? ESCAPE '\'`, len(customPrefix)+1, `logo\_custom:%`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if rows.Scan(&k) == nil {
			out = append(out, k)
		}
	}
	return out
}

// SetCustom stores an uploaded logo for a merchant.
func SetCustom(d *sql.DB, merchantName string, data []byte) error {
	k := Key(merchantName)
	if k == "" || len(merchantName) > 120 {
		return errors.New("which merchant?")
	}
	mime := isImage(data, "")
	if mime == "" {
		return errors.New("that isn't a small image — a PNG, JPEG, ICO or clean SVG under 256 KB")
	}
	if keys := customKeys(d); len(keys) >= maxCustom && !slices.Contains(keys, k) {
		return errors.New("too many custom logos")
	}
	raw, _ := json.Marshal(custom{Name: strings.TrimSpace(merchantName), Mime: mime, Data: base64.StdEncoding.EncodeToString(data)})
	_, err := d.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES(?,?)`, customPrefix+k, string(raw))
	return err
}

// DeleteCustom goes back to the automatic logo (or the category icon).
func DeleteCustom(d *sql.DB, key string) error {
	_, err := d.Exec(`DELETE FROM settings WHERE key=?`, customPrefix+key)
	return err
}

// Custom returns an uploaded logo by merchant key. Its type is read from
// the bytes again, never taken from the stored row: a hand-edited backup
// can't get something served as a web page.
func Custom(d *sql.DB, key string) ([]byte, string, error) {
	var raw string
	if d.QueryRow(`SELECT value FROM settings WHERE key=?`, customPrefix+key).Scan(&raw) != nil {
		return nil, "", ErrUnknown
	}
	var c custom
	if json.Unmarshal([]byte(raw), &c) != nil {
		return nil, "", ErrUnknown
	}
	b, err := base64.StdEncoding.DecodeString(c.Data)
	if err != nil {
		return nil, "", ErrUnknown
	}
	mime := isImage(b, "")
	if mime == "" {
		return nil, "", ErrUnknown
	}
	return b, mime, nil
}

// CustomID is how the index names an uploaded logo (never a domain).
const CustomID = "custom-"

// IndexWith is Index plus the owner's uploads, which win.
func IndexWith(d *sql.DB) map[string]string {
	out := Index()
	for _, k := range customKeys(d) {
		out[k] = CustomID + k
	}
	return out
}
