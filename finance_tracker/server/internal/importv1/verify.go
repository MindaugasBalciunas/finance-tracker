package importv1

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"ft/internal/money"
	"ft/internal/wealth"
)

// Check is one reconciliation assertion.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// Verification proves a conversion lost nothing.
type Verification struct {
	OK     bool    `json:"ok"`
	Checks []Check `json:"checks"`
}

func (v *Verification) add(name string, ok bool, format string, args ...any) {
	v.Checks = append(v.Checks, Check{Name: name, OK: ok, Detail: fmt.Sprintf(format, args...)})
	if !ok {
		v.OK = false
	}
}

// Verify compares a converted v2 database with its v1 source.
func Verify(d *sql.DB, src *Data) *Verification {
	v := &Verification{OK: true}

	// ── transactions: every v1 row, by id, with date, amount and text ──
	type row struct {
		date, note string
		amount     int64
	}
	v2 := map[int64]row{}
	rows, err := d.Query(`SELECT id,date,amount,note FROM transactions`)
	if err != nil {
		v.add("transactions readable", false, "%v", err)
		return v
	}
	for rows.Next() {
		var id int64
		var r row
		rows.Scan(&id, &r.date, &r.amount, &r.note)
		v2[id] = r
	}
	rows.Close()
	missing, changed, expected := 0, 0, 0
	var sample []string
	v1Year := map[string]int64{}
	for _, t := range src.Transactions {
		if t.Amount <= 0 || t.Date == "" {
			continue
		}
		expected++
		v1Year[t.Date[:4]] += int64(money.FromFloat(t.Amount))
		r, ok := v2[t.ID]
		switch {
		case !ok:
			missing++
			if len(sample) < 5 {
				sample = append(sample, fmt.Sprintf("#%d missing", t.ID))
			}
		case r.date != t.Date || r.amount != int64(money.FromFloat(t.Amount)) || r.note != strings.TrimSpace(t.Comment):
			changed++
			if len(sample) < 5 {
				sample = append(sample, fmt.Sprintf("#%d %s %.2f %q → %s %s %q", t.ID, t.Date, t.Amount, t.Comment, r.date, money.Cents(r.amount), r.note))
			}
		}
	}
	v.add("every transaction carried over", missing == 0 && changed == 0 && len(v2) >= expected,
		"v1 %d, v2 %d, missing %d, altered %d %s", expected, len(v2), missing, changed, strings.Join(sample, "; "))

	v2Year := map[string]int64{}
	yr, _ := d.Query(`SELECT substr(date,1,4), SUM(amount) FROM transactions WHERE id IN (SELECT id FROM transactions) GROUP BY 1`)
	for yr != nil && yr.Next() {
		var y string
		var s int64
		yr.Scan(&y, &s)
		v2Year[y] = s
	}
	if yr != nil {
		yr.Close()
	}
	var badYears []string
	for y, s := range v1Year {
		if v2Year[y] < s { // v2 may hold more rows (added later), never less
			badYears = append(badYears, fmt.Sprintf("%s v1 %s v2 %s", y, money.Cents(s), money.Cents(v2Year[y])))
		}
	}
	sort.Strings(badYears)
	v.add("yearly totals preserved", len(badYears) == 0, "%d years compared %s", len(v1Year), strings.Join(badYears, "; "))

	// ── balances: every recorded v1 value exists in v2 ──
	book, err := wealth.LoadBook(d)
	if err != nil {
		v.add("balances readable", false, "%v", err)
		return v
	}
	latest := map[string]map[string]float64{} // date → last snapshot row of that date
	for _, b := range src.Balances {
		latest[b.Date] = b.Values
	}
	mismatch, checked := 0, 0
	var bsample []string
	for date, vals := range latest {
		price := vals["btc_price"]
		for k, val := range vals {
			if k == "btc_price" {
				continue
			}
			want := val
			if (k == "mbtc" || k == "rbtc") && price > 0 {
				want = val * price
			}
			p, ok := book.At(MapAccount(k), date)
			if want == 0 && !ok {
				continue
			}
			checked++
			if !ok || p.Value != money.FromFloat(want) {
				mismatch++
				if len(bsample) < 5 {
					bsample = append(bsample, fmt.Sprintf("%s %s v1 %.2f v2 %s", MapAccount(k), date, want, p.Value))
				}
			}
		}
	}
	v.add("every balance value carried over", mismatch == 0, "%d values checked, %d differ %s", checked, mismatch, strings.Join(bsample, "; "))

	// Latest liquid net worth equals v1's latest total.
	var lastDate string
	for date := range latest {
		if date > lastDate {
			lastDate = date
		}
	}
	if lastDate != "" {
		vals := latest[lastDate]
		var v1Total float64
		for k, val := range vals {
			if k == "btc_price" {
				continue
			}
			if (k == "mbtc" || k == "rbtc") && vals["btc_price"] > 0 {
				val *= vals["btc_price"]
			}
			v1Total += val
		}
		snap := book.SnapshotAt(lastDate, true)
		var v2Total float64
		for id, val := range snap.ByAccount {
			a := book.Accounts[id]
			if a.Kind == "property" || a.Kind == "vehicle" || a.Kind == "loan" {
				continue
			}
			v2Total += val.Float()
		}
		v.add("latest balance-sheet total matches", math.Abs(v1Total-v2Total) < 0.05, "%s: v1 %.2f, v2 %.2f (excluding property, car and mortgage which v1 did not count)", lastDate, v1Total, v2Total)
	}

	// ── everything else ──
	count := func(q string) int {
		var n int
		d.QueryRow(q).Scan(&n)
		return n
	}
	v.add("investment trades", count(`SELECT COUNT(*) FROM trades`) == len(src.Trades), "v1 %d, v2 %d", len(src.Trades), count(`SELECT COUNT(*) FROM trades`))
	v.add("budgets", count(`SELECT COUNT(*) FROM budgets`) == len(src.Budgets), "v1 %d, v2 %d", len(src.Budgets), count(`SELECT COUNT(*) FROM budgets`))
	assetOK := true
	for _, a := range src.Assets {
		var n int
		d.QueryRow(`SELECT COUNT(*) FROM balances b JOIN accounts a ON a.id=b.account_id WHERE a.kind IN ('property','vehicle','other') AND b.value=?`,
			int64(money.FromFloat(a.CurrentValue))).Scan(&n)
		if a.CurrentValue > 0 && n == 0 {
			assetOK = false
		}
	}
	v.add("assets valued", assetOK, "%d assets", len(src.Assets))
	if len(src.Assets) > 0 {
		for _, a := range src.Assets {
			if a.LoanRemaining > 0 {
				p, ok := book.At("mortgage", a.LoanRemainingDate)
				v.add("mortgage balance", ok && math.Abs(-p.Value.Float()-a.LoanRemaining) < 0.01, "v1 %.2f on %s, v2 %s", a.LoanRemaining, a.LoanRemainingDate, p.Value)
			}
		}
	}
	setting := func(key string) map[string]any {
		var raw string
		d.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&raw)
		var m map[string]any
		json.Unmarshal([]byte(raw), &m)
		return m
	}
	if src.AI != nil {
		ai := setting("ai")
		v.add("AI settings", ai["api_key"] == src.AI.APIKey && ai["model"] == src.AI.Model, "model %v", ai["model"])
	}
	if src.AIContext != "" {
		var raw string
		d.QueryRow(`SELECT value FROM settings WHERE key='ai_context'`).Scan(&raw)
		var ctx string
		json.Unmarshal([]byte(raw), &ctx)
		v.add("AI context document", ctx == src.AIContext, "%d chars", len(ctx))
	}
	if src.Auth != nil {
		au := setting("auth")
		v.add("app lock (PIN) and tokens", au["pin_hash"] == src.Auth.PinHash && au["enabled"] == src.Auth.Enabled && au["token_hash"] == src.Auth.TokenHash,
			"lock enabled %v", au["enabled"])
	}
	v.add("passkeys", count(`SELECT COUNT(*) FROM webauthn_credentials`) == len(src.Webauthn), "v1 %d, v2 %d", len(src.Webauthn), count(`SELECT COUNT(*) FROM webauthn_credentials`))
	if src.Bank != nil {
		b := setting("bank")
		v.add("bank credentials", b["application_id"] == src.Bank.AppID && b["private_key_pem"] == src.Bank.PrivateKey, "app %v", b["application_id"])
	}
	v.add("bank connections", count(`SELECT COUNT(*) FROM bank_connections`) == len(src.BankConns), "v1 %d, v2 %d", len(src.BankConns), count(`SELECT COUNT(*) FROM bank_connections`))
	v.add("bank accounts", count(`SELECT COUNT(*) FROM bank_accounts`) == len(src.BankLinks), "v1 %d, v2 %d", len(src.BankLinks), count(`SELECT COUNT(*) FROM bank_accounts`))
	stagedWithID := 0
	for _, s := range src.BankStaged {
		if s.ExternalID != "" {
			stagedWithID++
		}
	}
	v.add("bank inbox and its dismiss history", count(`SELECT COUNT(*) FROM bank_inbox`) == stagedWithID, "v1 %d, v2 %d", stagedWithID, count(`SELECT COUNT(*) FROM bank_inbox`))
	extOK := count(`SELECT COUNT(*) FROM transactions WHERE external_id IS NOT NULL AND external_id != ''`)
	v1Ext := 0
	for _, t := range src.Transactions {
		if t.ExternalID != "" {
			v1Ext++
		}
	}
	v.add("bank ids on transactions (dedup keys)", extOK == v1Ext, "v1 %d, v2 %d", v1Ext, extOK)
	v.add("AI usage history", count(`SELECT COUNT(*) FROM ai_spend`) == len(src.AISpend) && count(`SELECT COUNT(*) FROM ai_topups`) == len(src.AITopups),
		"spend %d, top-ups %d", len(src.AISpend), len(src.AITopups))
	return v
}
