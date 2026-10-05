package importv1

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"ft/internal/db"
	"ft/internal/ledger"
	"ft/internal/money"
)

// Report summarises a conversion, for the log and the UI.
type Report struct {
	Transactions  int            `json:"transactions"`
	BalanceRows   int            `json:"balance_rows"`
	Accounts      int            `json:"accounts"`
	Rules         int            `json:"rules"`
	RulesSkipped  int            `json:"rules_skipped"`
	Budgets       int            `json:"budgets"`
	Trades        int            `json:"trades"`
	InboxRows     int            `json:"inbox_rows"`
	ByCategory    map[string]int `json:"by_category"`
	DroppedLabels map[string]int `json:"dropped_labels"`
	MortgagePts   int            `json:"mortgage_points"`
	Warnings      []string       `json:"warnings,omitempty"`
}

// Convert writes v1 data into an empty v2 database.
func Convert(d *sql.DB, src *Data) (*Report, error) {
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&n)
	if n > 0 {
		return nil, fmt.Errorf("target database already has %d transactions", n)
	}
	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rep := &Report{ByCategory: map[string]int{}, DroppedLabels: map[string]int{}}
	now := db.Now()

	if err := ledger.SeedCategories(tx); err != nil {
		return nil, err
	}

	// ── accounts ───────────────────────────────────────────────────────
	accounts := map[string]bool{}
	addAccount := func(a ledger.Account) error {
		if accounts[a.ID] {
			return nil
		}
		if _, err := ledger.SaveAccount(tx, a); err != nil {
			return fmt.Errorf("account %s: %w", a.ID, err)
		}
		accounts[a.ID] = true
		rep.Accounts++
		return nil
	}
	v1Labels := map[string]V1Account{}
	for _, a := range src.Accounts {
		v1Labels[MapAccount(a.Key)] = a
	}
	// Every account any balance or transaction mentions must exist.
	seen := map[string]bool{}
	for _, b := range src.Balances {
		for k := range b.Values {
			if k != "btc_price" {
				seen[MapAccount(k)] = true
			}
		}
	}
	for _, t := range src.Transactions {
		for _, k := range []string{t.Debit, t.Credit, t.Source} {
			if k != "" {
				seen[MapAccount(k)] = true
			}
		}
	}
	for id := range v1Labels {
		seen[id] = true
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		a := ledger.Account{ID: id, Liquid: true, Sort: 500}
		if def, ok := builtinAccounts[id]; ok {
			a.Name, a.Institution, a.Kind, a.Liquid, a.Sort = def.name, def.institution, def.kind, def.liquid, def.sort
		} else {
			v := v1Labels[id]
			a.Name, a.Institution = firstNonEmpty(v.Label, id), v.Institution
			a.Kind = "checking"
			switch {
			case strings.Contains(id, "saving"):
				a.Kind = "savings"
			case v.Group == "investments":
				a.Kind = "brokerage"
			case v.Group == "pensions":
				a.Kind, a.Liquid = "pension", true
			case v.Group == "crypto":
				a.Kind = "crypto"
			}
			a.Sort = 50
		}
		if v, ok := v1Labels[id]; ok && v.Archived {
			a.Archived = true
		}
		if err := addAccount(a); err != nil {
			return nil, err
		}
	}

	// ── balances ───────────────────────────────────────────────────────
	type bkey struct{ acct, date string }
	type bval struct {
		value money.Cents
		qty   *float64
		price *money.Cents
	}
	bal := map[bkey]bval{}
	nonZero := map[string]bool{}
	for _, b := range src.Balances {
		// BTC columns are coin quantities only on rows that state a BTC
		// price; on rows without one, v1 stored the euro value directly.
		price := b.Values["btc_price"]
		for k, v := range b.Values {
			if k == "btc_price" {
				continue
			}
			id := MapAccount(k)
			bv := bval{value: money.FromFloat(v)}
			if (k == "mbtc" || k == "rbtc") && price > 0 {
				q := v
				pc := money.FromFloat(price)
				bv = bval{value: money.FromFloat(v * price), qty: &q, price: &pc}
			}
			if bv.value == 0 && !nonZero[id] {
				continue
			}
			if bv.value != 0 {
				nonZero[id] = true
			}
			bal[bkey{id, b.Date}] = bv
		}
	}
	// v1 rows were whole snapshots: an account missing from every row after
	// some point was closed, not forgotten (Luminor). Elsewhere a gap is
	// just an unrecorded value and v2 carries the last one forward.
	lastSeen := map[string]int{}
	for i, b := range src.Balances {
		for k := range b.Values {
			if k != "btc_price" {
				lastSeen[MapAccount(k)] = i
			}
		}
	}
	for id, i := range lastSeen {
		if i < len(src.Balances)-1 && nonZero[id] {
			closed := src.Balances[i+1].Date
			if closed == src.Balances[i].Date {
				continue
			}
			bal[bkey{id, closed}] = bval{value: 0}
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s has no value after %s — recorded as closed (0) on %s", id, src.Balances[i].Date, closed))
		}
	}
	for k, v := range bal {
		if _, err := tx.Exec(`INSERT INTO balances(account_id,date,value,quantity,price,source,updated_at) VALUES(?,?,?,?,?,'import',?)`,
			k.acct, k.date, int64(v.value), v.qty, v.price, now); err != nil {
			return nil, fmt.Errorf("balance %s %s: %w", k.acct, k.date, err)
		}
		rep.BalanceRows++
	}
	// An account whose last value is zero and older than a year is history.
	tx.Exec(`UPDATE accounts SET archived=1 WHERE id IN (
		SELECT account_id FROM balances b WHERE date = (SELECT MAX(date) FROM balances WHERE account_id=b.account_id)
		AND value = 0 AND date < date('now','-1 year'))`)

	// ── assets & loans ─────────────────────────────────────────────────
	var mortgage *V1Asset
	for i := range src.Assets {
		a := src.Assets[i]
		kind, id := "property", "house"
		switch a.Type {
		case "vehicle":
			kind, id = "vehicle", "car"
		case "solar":
			id = "solar"
		case "real_estate":
		default:
			kind, id = "other", ledger.SlugID(a.Name)
		}
		if accounts[id] {
			id = ledger.SlugID(a.Name)
		}
		det, _ := json.Marshal(ledger.AssetDetails{PurchaseDate: a.PurchaseDate, PurchasePrice: a.PurchasePrice, Address: a.Name, PaidOffDate: a.LoanPaidOff})
		name := a.Name
		if i := strings.Index(name, ","); i > 0 && kind == "property" {
			name = strings.TrimSpace(name[:i])
		}
		if err := addAccount(ledger.Account{ID: id, Name: name, Kind: kind, Liquid: false, Sort: 400 + i, Notes: a.Notes, Details: det}); err != nil {
			return nil, err
		}
		points := map[string]float64{}
		if a.PurchaseDate != "" && a.PurchasePrice > 0 {
			points[a.PurchaseDate] = a.PurchasePrice
		}
		if a.ValuationDate != "" && a.CurrentValue > 0 {
			points[a.ValuationDate] = a.CurrentValue
		}
		for date, v := range points {
			tx.Exec(`INSERT OR REPLACE INTO balances(account_id,date,value,source,updated_at) VALUES(?,?,?,'manual',?)`, id, date, int64(money.FromFloat(v)), now)
			rep.BalanceRows++
		}
		if a.LoanRemaining > 0 && a.LoanRemainingDate != "" && a.LoanPaidOff > a.LoanRemainingDate {
			a2 := a
			mortgage = &a2
			ld := ledger.LoanDetails{Lender: strings.ToUpper(a.LoanAccount), AssetID: id, BaseRate: a.LoanBaseRate, BaseRateName: "6M EURIBOR",
				Margin: a.LoanMargin, RateResetDate: a.LoanRateReset, MonthlyPayment: a.LoanMonthly, EndDate: a.LoanPaidOff,
				StartDate: a.PurchaseDate, PaymentDay: 17}
			if ld.Lender == "SEB" || ld.Lender == "" {
				ld.Lender = "SEB"
			}
			det, _ := json.Marshal(ld)
			if err := addAccount(ledger.Account{ID: "mortgage", Name: "Mortgage", Institution: ld.Lender, Kind: "loan", Liquid: false, Sort: 450,
				Notes: a.LoanRate, Details: det}); err != nil {
				return nil, err
			}
			tx.Exec(`INSERT OR REPLACE INTO balances(account_id,date,value,source,updated_at) VALUES('mortgage',?,?,'manual',?)`,
				a.LoanRemainingDate, -int64(money.FromFloat(a.LoanRemaining)), now)
			rep.BalanceRows++
		}
	}
	if mortgage == nil && !accounts["mortgage"] {
		// Mortgage transfers still need a target.
		addAccount(ledger.Account{ID: "mortgage", Name: "Mortgage", Kind: "loan", Liquid: false, Sort: 450, Archived: false})
	}

	// ── transactions ───────────────────────────────────────────────────
	for _, t := range src.Transactions {
		if t.Amount <= 0 || t.Date == "" {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("skipped v1 #%d (%s %.2f): no amount or date", t.ID, t.Date, t.Amount))
			continue
		}
		m := MapTx(t)
		for _, a := range []string{m.Account, m.ToAccount} {
			if a != "" && !accounts[a] {
				addAccount(ledger.Account{ID: a, Name: a, Kind: "other", Liquid: true, Sort: 900})
			}
		}
		created := firstNonEmpty(ts(t.CreatedAt), now)
		source := "import"
		if t.ExternalID != "" {
			source = "bank"
		}
		_, err := tx.Exec(`INSERT INTO transactions(id,date,kind,amount,account_id,to_account_id,category,merchant,note,tags,external_id,source,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			t.ID, t.Date, m.Kind, int64(money.FromFloat(t.Amount)), nullStr(m.Account), nullStr(m.ToAccount), m.Category, m.Merchant,
			strings.TrimSpace(t.Comment), ledger.JoinTags(m.Tags), nullStr(t.ExternalID), source, created, now)
		if err != nil {
			return nil, fmt.Errorf("transaction v1 #%d: %w", t.ID, err)
		}
		rep.Transactions++
		rep.ByCategory[m.Category]++
		for _, l := range m.Dropped {
			rep.DroppedLabels[l]++
		}
	}

	if mortgage != nil {
		pts, err := backfillMortgage(tx, mortgage, now)
		if err != nil {
			return nil, err
		}
		rep.MortgagePts = pts
		rep.BalanceRows += pts
	}

	// ── rules ──────────────────────────────────────────────────────────
	seenRule := map[string]bool{}
	for _, r := range src.LabelRules {
		nr, ok := RuleFor(r)
		if !ok {
			rep.RulesSkipped++
			continue
		}
		key := strings.ToLower(nr.Pattern) + "|" + nr.WhenCategory + "|" + nr.SetCategory + "|" + nr.SetMerchant + "|" + strings.Join(nr.AddTags, ",")
		if seenRule[key] {
			rep.RulesSkipped++
			continue
		}
		seenRule[key] = true
		if err := ledger.SaveRule(tx, &nr); err != nil {
			return nil, err
		}
		rep.Rules++
	}

	// ── budgets ────────────────────────────────────────────────────────
	for i, b := range src.Budgets {
		kind, cat, tag, name := budgetMapping(b)
		if kind == "" {
			rep.Warnings = append(rep.Warnings, "budget "+b.Name+" could not be mapped")
			continue
		}
		period := b.Period
		if period != "yearly" {
			period = "monthly"
		}
		res, err := tx.Exec(`INSERT INTO budgets(name,kind,category,tag,period,fund,start_month,sort,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			name, kind, cat, tag, period, b.Fund && kind == "spending", b.StartMonth, (i+1)*10, now)
		if err != nil {
			return nil, err
		}
		id, _ := res.LastInsertId()
		amounts := map[string]float64{"": b.Amount}
		for _, a := range b.Amounts {
			amounts[a.From] = a.Amount
		}
		for from, amt := range amounts {
			tx.Exec(`INSERT OR REPLACE INTO budget_amounts(budget_id,from_month,amount) VALUES(?,?,?)`, id, from, int64(money.FromFloat(amt)))
		}
		rep.Budgets++
	}

	// ── investments ────────────────────────────────────────────────────
	for _, t := range src.Trades {
		acct := "revolut_stocks"
		if strings.EqualFold(t.Source, "IBKR") {
			acct = "ibkr"
		}
		if !accounts[acct] {
			def := builtinAccounts[acct]
			if err := addAccount(ledger.Account{ID: acct, Name: def.name, Institution: def.institution, Kind: def.kind, Liquid: true, Sort: def.sort}); err != nil {
				return nil, err
			}
		}
		tx.Exec(`INSERT INTO trades(date,account_id,action,ticker,shares,price,currency,notes,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			t.Date, nullStr(acct), strings.ToLower(t.Action), strings.ToUpper(t.Ticker), t.Shares, t.Price, firstNonEmpty(t.Currency, "USD"), t.Notes, now)
		rep.Trades++
	}

	// ── settings ───────────────────────────────────────────────────────
	set := func(key string, v any) {
		raw, _ := json.Marshal(v)
		tx.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES(?,?)`, key, string(raw))
	}
	if s := src.BudgetSet; s != nil {
		set("plan", map[string]any{"income_mode": s.IncomeMode, "manual_income": s.ManualIncome, "gross_salary": s.Gross, "monthly_deductions": s.Ded})
	}
	if a := src.AI; a != nil {
		set("ai", map[string]any{"gateway_url": a.GatewayURL, "api_key": a.APIKey, "model": a.Model, "provider": a.Provider, "enabled": !a.Disabled})
	}
	if src.AIContext != "" {
		set("ai_context", src.AIContext)
	}
	if a := src.Auth; a != nil {
		set("auth", map[string]any{"pin_hash": a.PinHash, "enabled": a.Enabled, "token_hash": a.TokenHash, "token_rw_hash": a.TokenRWHash})
	}
	for _, c := range src.Webauthn {
		tx.Exec(`INSERT INTO webauthn_credentials(name,credential,created_at) VALUES(?,?,?)`, c.Name, c.Credential, firstNonEmpty(c.CreatedAt, now))
	}
	for _, m := range src.AIMessages {
		tx.Exec(`INSERT INTO ai_messages(role,content,created_at) VALUES(?,?,?)`, m.Role, m.Content, firstNonEmpty(m.CreatedAt, now))
	}
	for _, s := range src.AISpend {
		tx.Exec(`INSERT INTO ai_spend(kind,model,cost_usd,input_tokens,output_tokens,estimated,created_at) VALUES(?,?,?,?,?,?,?)`,
			s.Kind, s.Model, s.Cost, s.In, s.Out, s.Estimated, firstNonEmpty(s.CreatedAt, now))
	}
	for _, t := range src.AITopups {
		tx.Exec(`INSERT INTO ai_topups(amount_usd,note,occurred_on,created_at) VALUES(?,?,?,?)`, t.Amount, t.Note, t.On, firstNonEmpty(t.CreatedAt, now))
	}

	// ── open banking ───────────────────────────────────────────────────
	if b := src.Bank; b != nil {
		set("bank", map[string]any{"application_id": b.AppID, "private_key_pem": b.PrivateKey, "environment": b.Environment, "redirect_url": b.RedirectURL})
	}
	for _, c := range src.BankConns {
		tx.Exec(`INSERT INTO bank_connections(id,aspsp_name,aspsp_country,session_id,status,valid_until,last_error,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			c.ID, c.ASPSP, c.Country, c.Session, c.Status, ts(c.ValidUntil), c.Error, firstNonEmpty(ts(c.CreatedAt), now), now)
	}
	for _, l := range src.BankLinks {
		var bb any
		if l.BankBalance.Valid {
			bb = int64(money.FromFloat(l.BankBalance.Float64))
		}
		if _, err := tx.Exec(`INSERT INTO bank_accounts(id,connection_id,identification_hash,uid,iban,display_name,account_id,last_synced_at,last_tx_date,
			bank_balance,bank_balance_currency,bank_balance_type,bank_balance_date,bank_balance_fetched,balance_applied_through,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			l.ID, l.ConnectionID, l.Hash, l.UID, l.IBAN, l.Name, MapAccount(l.AccountKey), ts(l.LastSynced), day(l.LastTx), bb, l.BalCurrency, l.BalType,
			day(l.BalDate), ts(l.BalFetched), day(l.AppliedThrough), now, now); err != nil {
			return nil, fmt.Errorf("bank account %d: %w", l.ID, err)
		}
	}
	for _, s := range src.BankStaged {
		if s.ExternalID == "" {
			continue
		}
		m := MapTx(V1Tx{Date: s.Date, Type: s.Type, Amount: s.Amount, Comment: s.Comment, Category: s.Category, Labels: s.Labels, Debit: s.Debit, Credit: s.Credit})
		state := map[string]string{"staged": "open", "imported": "imported", "dismissed": "dismissed", "superseded": "superseded"}[s.State]
		if state == "" {
			state = "dismissed"
		}
		verdict := map[string]string{"duplicate_exact": "duplicate", "duplicate_content": "duplicate", "internal": "internal",
			"needs_review": "needs_review", "pending": "pending", "new": "new"}[s.Verdict]
		if verdict == "" {
			verdict = "new"
		}
		var link any
		if s.LinkID.Valid {
			link = s.LinkID.Int64
		}
		amount := money.FromFloat(s.Amount)
		if amount <= 0 {
			amount = 1
		}
		if _, err := tx.Exec(`INSERT INTO bank_inbox(bank_account_id,external_id,raw,raw_payee,raw_details,raw_currency,booking_date,pending,date,kind,amount,
			account_id,to_account_id,category,merchant,note,tags,edited,verdict,verdict_note,matched_tx_id,state,imported_tx_id,first_seen_at,last_seen_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			link, s.ExternalID, s.Raw, s.RawPayee, s.RawDetails, s.RawCurrency, s.BookingDate, s.Pending, firstNonEmpty(s.Date, s.BookingDate), m.Kind,
			int64(amount), nullStr(m.Account), nullStr(m.ToAccount), m.Category, m.Merchant, s.Comment, ledger.JoinTags(m.Tags), s.Edited,
			verdict, s.VerdictNote, nullInt(s.MatchedTxID), state, nullInt(s.ImportedTxID), firstNonEmpty(ts(s.FirstSeen), now), firstNonEmpty(ts(s.LastSeen), now)); err != nil {
			return nil, fmt.Errorf("inbox row %s: %w", s.ExternalID, err)
		}
		rep.InboxRows++
	}

	set("migrated_from_v1", now)
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return rep, nil
}

// budgetMapping translates a v1 budget into v2 terms.
func budgetMapping(b V1Budget) (kind, category, tag, name string) {
	name = strings.TrimSpace(b.Name)
	label := strings.ToLower(strings.TrimSpace(b.Label))
	switch b.Kind {
	case "fixed":
		kind = "fixed"
	case "investment":
		kind = "saving"
	default:
		kind = "spending"
	}
	switch {
	case label == "loan":
		return kind, "housing.mortgage_interest,housing.mortgage,transfer.debt", "", name
	case label == "alimony":
		return kind, "kids.alimony", "", name
	case label == "gift":
		return kind, "gifts", "", name
	case label != "":
		if strings.HasPrefix(label, "trip:") || keepTags[label] {
			return kind, "", label, name
		}
		for _, lc := range labelCategory {
			if hasAny([]string{label}, lc.labels...) {
				return kind, lc.cat, "", name
			}
		}
		return kind, "", label, name
	}
	switch b.Category {
	case "Stocks & ETF":
		if name == "" || name == "ETF" {
			name = "ETF"
		}
		return kind, "transfer.invest", "", name
	case "Pension":
		return kind, "transfer.pension", "", name
	case "Entertainment":
		return kind, "leisure,shopping", "", "Leisure & shopping"
	case "Clothing":
		return kind, "shopping.clothing", "", name
	}
	if d, ok := expenseDefault[b.Category]; ok {
		return kind, ledger.Top(d), "", name
	}
	return "", "", "", name
}

// backfillMortgage reconstructs month-end mortgage balances backwards from
// the one known balance, using exact principal rows where the bank split
// them and an annuity estimate elsewhere. Marked 'computed' so the UI can
// say so.
func backfillMortgage(tx *sql.Tx, a *V1Asset, now string) (int, error) {
	known := a.LoanRemaining
	knownDate, err := time.Parse("2006-01-02", a.LoanRemainingDate)
	if err != nil {
		return 0, nil
	}
	start, err := time.Parse("2006-01-02", a.PurchaseDate)
	if err != nil {
		return 0, nil
	}
	rate := (a.LoanBaseRate + a.LoanMargin) / 100 / 12
	if rate <= 0 {
		rate = 0.04 / 12
	}
	principal := map[string]float64{}
	combined := map[string]float64{}
	rows, err := tx.Query(`SELECT substr(date,1,7), category, kind, to_account_id, SUM(amount) FROM transactions
		WHERE (kind='transfer' AND to_account_id='mortgage') OR category IN ('housing.mortgage','housing.mortgage_interest') GROUP BY 1,2,3,4`)
	if err != nil {
		return 0, err
	}
	var payments []float64
	for rows.Next() {
		var month, cat, kind string
		var to sql.NullString
		var sum int64
		rows.Scan(&month, &cat, &kind, &to, &sum)
		v := money.Cents(sum).Float()
		switch {
		case kind == "transfer":
			principal[month] += v
		case cat == "housing.mortgage":
			combined[month] += v
			payments = append(payments, v)
		}
	}
	rows.Close()
	fallback := a.LoanMonthly
	if len(payments) > 0 {
		sort.Float64s(payments)
		fallback = payments[len(payments)/2]
	}
	b := known
	n := 0
	cur := time.Date(knownDate.Year(), knownDate.Month(), 1, 0, 0, 0, 0, time.UTC)
	for {
		month := cur.Format("2006-01")
		var prev float64
		if p, ok := principal[month]; ok && p > 0 {
			prev = b + p
		} else {
			pay := combined[month]
			if pay <= 0 {
				pay = fallback
			}
			prev = (b + pay) / (1 + rate)
		}
		cur = cur.AddDate(0, -1, 0)
		if cur.Before(time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)) {
			break
		}
		b = math.Round(prev*100) / 100
		date := time.Date(cur.Year(), cur.Month(), 17, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		if _, err := tx.Exec(`INSERT OR IGNORE INTO balances(account_id,date,value,source,updated_at) VALUES('mortgage',?,?,'computed',?)`,
			date, -int64(money.FromFloat(b)), now); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(v sql.NullInt64) any {
	if v.Valid && v.Int64 != 0 {
		return v.Int64
	}
	return nil
}

// ts normalises a v1 timestamp to RFC3339, or "".
func ts(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05-07:00", time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return s
}
