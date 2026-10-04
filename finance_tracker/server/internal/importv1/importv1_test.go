package importv1

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"ft/internal/db"
	"ft/internal/testutil"
	"ft/internal/money"
	"ft/internal/wealth"
)

func TestMapTx(t *testing.T) {
	cases := []struct {
		name                    string
		tx                      V1Tx
		kind, cat, merchant, to string
		tags                    string
	}{
		{"groceries label + vendor", V1Tx{Type: "expense", Category: "Food", Labels: "groceries,maxima", Comment: "MAXIMA LT, UAB", Debit: "swed"}, "expense", "food.groceries", "Maxima", "", ""},
		{"restaurant filed under entertainment is food", V1Tx{Type: "expense", Category: "Entertainment", Labels: "restaurant", Comment: "Jammi"}, "expense", "food.restaurants", "", "", ""},
		{"aliexpress is shopping", V1Tx{Type: "expense", Category: "Entertainment", Comment: "aliexpress L-1528 Luxembourg"}, "expense", "shopping.online", "AliExpress", "", ""},
		{"trip groceries stay trip money", V1Tx{Type: "expense", Category: "Vacation", Labels: "groceries,trip:rome", Comment: "Coop"}, "expense", "travel.trip", "", "", "trip:rome"},
		{"hotel on a trip", V1Tx{Type: "expense", Category: "Vacation", Labels: "hotel", Comment: "Hotel"}, "expense", "travel.lodging", "", "", ""},
		{"mortgage principal", V1Tx{Type: "expense", Category: "Finance", Labels: "loan", Comment: "Loan return", Debit: "seb"}, "transfer", "transfer.debt", "", "mortgage", ""},
		{"mortgage interest", V1Tx{Type: "expense", Category: "Finance", Labels: "loan", Comment: "Loan interest", Debit: "seb"}, "expense", "housing.mortgage_interest", "", "", ""},
		{"combined payment", V1Tx{Type: "expense", Category: "Finance", Labels: "loan", Comment: "House payment"}, "expense", "housing.mortgage", "", "", ""},
		{"notary with loan label", V1Tx{Type: "expense", Category: "Finance", Labels: "divorce,bank fee,loan", Comment: "Notary services for hipoteka"}, "expense", "finance.legal", "", "", "divorce"},
		{"alimony by text", V1Tx{Type: "expense", Category: "Finance", Labels: "evelina", Comment: "Aliments 2026.09 Evelina"}, "expense", "kids.alimony", "Evelina", "", "evelina"},
		{"alimony comment is not a payee", V1Tx{Type: "expense", Category: "Kids", Labels: "alimony,evelina", Comment: "Aliments 2026.08"}, "expense", "kids.alimony", "Evelina", "", "evelina"},
		{"family support", V1Tx{Type: "expense", Category: "Finance", Comment: "EVELINA BALČIŪNIENĖ (Išlaidoms)"}, "expense", "other.family", "Evelina", "", "evelina"},
		{"mother is not evelina", V1Tx{Type: "expense", Category: "Finance", Labels: "tax", Comment: "VALSTYBINĖ MOKESČIŲ INSPEKCIJA (#ALDONA BALČIŪNIENĖ#)"}, "expense", "finance.taxes", "VMI", "", ""},
		{"bank fee", V1Tx{Type: "expense", Category: "Finance", Labels: "bank fee", Comment: "Swedbank card fee"}, "expense", "finance.bank_fees", "", "", ""},
		{"kids tag dropped inside kids", V1Tx{Type: "expense", Category: "Kids", Labels: "kids,education", Comment: "SKAITLIS"}, "expense", "kids.education", "Skaitlis", "", ""},
		{"kids tag kept elsewhere", V1Tx{Type: "expense", Category: "Food", Labels: "kids,fast food", Comment: "MCDONALDS 123"}, "expense", "food.fast_food", "McDonald's", "", "kids"},
		{"salary employer", V1Tx{Type: "income", Category: "Salary", Labels: "nexos", Comment: "Nexos.ai 2026.09", Credit: "swed"}, "income", "salary", "Nexos.ai", "", ""},
		{"child benefit", V1Tx{Type: "income", Category: "Reimbursement", Comment: "Child benefit (išmoka vaikui)"}, "income", "benefits", "", "", ""},
		{"rent income", V1Tx{Type: "income", Category: "Freelance", Labels: "rent", Comment: "Rent - Levent Sophie"}, "income", "side_income", "", "", ""},
		{"atm", V1Tx{Type: "investment", Category: "Transfers", Labels: "cash", Comment: "Cash withdrawal (ATM)", Debit: "swed", Credit: "cash"}, "transfer", "transfer.internal", "", "cash", ""},
		{"payroll pension", V1Tx{Type: "investment", Category: "Pension", Labels: "artea,payroll", Comment: "Artea (INVL) 3rd pillar pension"}, "transfer", "transfer.pension", "Artea", "artea", ""},
		{"ibkr top up", V1Tx{Type: "investment", Category: "Stocks & ETF", Labels: "ibkr", Comment: "ibkr top up", Debit: "swed"}, "transfer", "transfer.invest", "IBKR", "ibkr", ""},
		{"car leasing principal", V1Tx{Type: "investment", Category: "Vehicle", Labels: "leasing,evelina", Comment: "EVELINA PLYTNIKAITĖ (Lizingas)"}, "transfer", "transfer.debt", "Evelina", "", "evelina"},
		{"car down payment", V1Tx{Type: "investment", Category: "Vehicle", Comment: "RAV4 pradinė įmoka (Tokvila)"}, "transfer", "transfer.asset", "Tokvila", "", ""},
		{"house purchase", V1Tx{Type: "investment", Category: "Real Estate", Comment: "House purchase — Platiniškių 21A"}, "transfer", "transfer.asset", "", "", ""},
		{"legacy source account", V1Tx{Type: "expense", Category: "Food", Source: "rev_m", Comment: "x"}, "expense", "food", "", "", ""},
		{"dating since spring 2026 is kristina", V1Tx{Type: "expense", Category: "Dating", Date: "2026-05-01", Comment: "Cinema"}, "expense", "dating", "", "", "kristina"},
		{"retired gaming category", V1Tx{Type: "expense", Category: "Gaming", Comment: "Steam"}, "expense", "leisure.hobbies", "", "", ""},
	}
	for _, c := range cases {
		m := MapTx(c.tx)
		if m.Kind != c.kind || m.Category != c.cat || m.Merchant != c.merchant || m.ToAccount != c.to || strings.Join(m.Tags, ",") != c.tags {
			t.Errorf("%s: kind=%s cat=%s merchant=%q to=%s tags=%v", c.name, m.Kind, m.Category, m.Merchant, m.ToAccount, m.Tags)
		}
	}
	if m := MapTx(V1Tx{Type: "expense", Category: "Food", Source: "rev_m"}); m.Account != "revolut" {
		t.Error("legacy source_account maps to the account", m.Account)
	}
	if MapAccount("acc_seb_savings") != "seb_savings" || MapAccount("ibkr_stocks") != "ibkr" || MapAccount("") != "" {
		t.Error("account ids")
	}
}

func TestRuleFor(t *testing.T) {
	cases := []struct {
		in        V1Rule
		ok        bool
		cat, merc string
		tags      string
		when      string
	}{
		{V1Rule{"restaurant", "", "jammi"}, true, "food.restaurants", "", "", ""},
		{V1Rule{"maxima", "Food", "maxima"}, true, "food.groceries", "Maxima", "", ""},
		{V1Rule{"kristina", "Dating", ""}, true, "", "", "kristina", "dating"},
		{V1Rule{"trip:rome", "", "roma"}, true, "", "", "trip:rome", ""},
		{V1Rule{"utilities", "Utilities", "ignitis"}, false, "", "", "", ""}, // only restated its category
		{V1Rule{"groceries", "Vacation", "coop"}, false, "", "", "", ""},         // groceries on a trip stay trip money: nothing to translate
		{V1Rule{"hotel", "Vacation", "booking"}, true, "travel.lodging", "", "", "travel"},
	}
	for _, c := range cases {
		r, ok := RuleFor(c.in)
		if ok != c.ok {
			t.Errorf("%+v ok=%v", c.in, ok)
			continue
		}
		if !ok {
			continue
		}
		if r.SetCategory != c.cat || r.SetMerchant != c.merc || strings.Join(r.AddTags, ",") != c.tags || r.WhenCategory != c.when {
			t.Errorf("%+v → %+v", c.in, r)
		}
	}
}

const v1JSON = `{
 "schema_version": 7,
 "transactions": [
  {"id": 1, "date": "2026-01-15", "type": "income", "amount_eur": 3000, "category": "Salary", "comment": "Nexos.ai", "labels": "nexos", "credit_account": "swed"},
  {"id": 2, "date": "2026-01-16", "type": "expense", "amount_eur": 52.3, "category": "Food", "comment": "MAXIMA", "labels": "groceries,maxima", "debit_account": "swed"},
  {"id": 3, "date": "2026-02-17", "type": "expense", "amount_eur": 500, "category": "Finance", "comment": "Loan return", "labels": "loan", "debit_account": "seb"},
  {"id": 4, "date": "2026-02-17", "type": "expense", "amount_eur": 860, "category": "Finance", "comment": "Loan interest", "labels": "loan", "debit_account": "seb"},
  {"id": 5, "date": "2025-12-15", "type": "expense", "amount_eur": 1284, "category": "Finance", "comment": "House payment", "labels": "loan", "debit_account": "swed"},
  {"id": 6, "date": "2026-02-20", "type": "expense", "amount_eur": 12, "category": "Food", "comment": "Lidl", "external_id": "eb:3:ref1", "debit_account": "swed"}
 ],
 "balances": [
  {"id": 1, "date": "2025-12-31", "total_eur": 1500, "swed": 1000, "luminor": 300, "btc_m": 200},
  {"id": 2, "date": "2026-01-31", "total_eur": 1800, "swed": 1100, "luminor": 300, "btc_m": 0.01, "btc_price_eur": 40000},
  {"id": 3, "date": "2026-02-28", "total_eur": 1600, "swed": 1200, "btc_m": 0.01, "btc_price_eur": 40000, "extra": {"acc_seb_savings": 0}}
 ],
 "assets": [{"name": "House X, Vilnius", "type": "real_estate", "purchase_date": "2022-08-17", "purchase_price_eur": 355000, "current_value_eur": 410000, "valuation_date": "2025-12-01",
   "loan_remaining_eur": 262596.03, "loan_remaining_date": "2026-02-17", "loan_account": "seb", "loan_paid_off_date": "2052-07-17", "loan_margin": 1.3, "loan_base_rate": 2.65, "loan_monthly_payment_eur": 1361.66}],
 "stock_trades": [{"date": "2026-01-02", "action": "buy", "ticker": "VWCE", "shares": 2, "price_per_share": 120, "currency": "EUR", "source": "IBKR"}],
 "budgets": [{"name": "Food", "kind": "spending", "category": "Food", "amount": 500, "amounts": [{"from_month": "", "amount": 400}, {"from_month": "2026-10", "amount": 500}]},
             {"name": "Loan payments", "kind": "fixed", "label": "loan", "amount": 1361.66},
             {"name": "Entertainment", "kind": "spending", "category": "Entertainment", "amount": 200}],
 "label_rules": [{"label": "restaurant", "category": "", "comment_match": "jammi"}],
 "budget_settings": {"income_mode": "gross", "gross_salary": 8400, "monthly_deductions": 30},
 "ai_settings": {"gateway_url": "https://api.anthropic.com/v1", "api_key": "sk-ant-x", "model": "claude-opus-5-5", "provider": "anthropic", "enabled": true},
 "ai_context": "brief",
 "accounts": [{"key": "acc_seb_savings", "label": "SEB savings", "group": "cash", "institution": "SEB"}]
}`

func convert(t *testing.T, src *Data) (*sql.DB, *Report) {
	d, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	rep, err := Convert(d, src)
	if err != nil {
		t.Fatal(err)
	}
	return d, rep
}

func requireVerified(t *testing.T, d *sql.DB, src *Data) {
	t.Helper()
	v := Verify(d, src)
	for _, c := range v.Checks {
		if !c.OK {
			t.Errorf("verify %s: %s", c.Name, c.Detail)
		}
	}
}

func TestConvertFromJSON(t *testing.T) {
	src, err := ReadJSON([]byte(v1JSON))
	if err != nil {
		t.Fatal(err)
	}
	d, rep := convert(t, src)
	requireVerified(t, d, src)
	if rep.Transactions != 6 || rep.Budgets != 3 || rep.Trades != 1 || rep.Rules != 1 {
		t.Fatalf("%+v", rep)
	}
	book, _ := wealth.LoadBook(d)
	// BTC: euros on the row without a price, quantity × price on rows with one.
	if p, _ := book.At("btc_m", "2025-12-31"); p.Value != money.FromFloat(200) || p.Quantity != nil {
		t.Errorf("euro-valued BTC row: %+v", p)
	}
	if p, _ := book.At("btc_m", "2026-01-31"); p.Value != money.FromFloat(400) || *p.Quantity != 0.01 {
		t.Errorf("quantity BTC row: %+v", p)
	}
	// Luminor disappeared from the last snapshot: closed, not carried forward.
	if p, _ := book.At("luminor", "2026-02-28"); p.Value != 0 {
		t.Errorf("closed account still counted: %+v", p)
	}
	// Mortgage history reconstructed back to the purchase, principal rows exact.
	if p, _ := book.At("mortgage", "2026-01-17"); p.Value != -money.FromFloat(262596.03+500) || p.Source != "computed" {
		t.Errorf("exact principal backfill: %+v", p)
	}
	if rep.MortgagePts < 40 {
		t.Errorf("mortgage points %d", rep.MortgagePts)
	}
	// Budgets keep their dated steps; the loan line spans principal + interest.
	var cats string
	d.QueryRow(`SELECT category FROM budgets WHERE name='Loan payments'`).Scan(&cats)
	if cats != "housing.mortgage_interest,housing.mortgage,transfer.debt" {
		t.Error(cats)
	}
	var steps int
	d.QueryRow(`SELECT COUNT(*) FROM budget_amounts a JOIN budgets b ON b.id=a.budget_id WHERE b.name='Food'`).Scan(&steps)
	if steps != 2 {
		t.Error("budget steps", steps)
	}
	var name string
	d.QueryRow(`SELECT name FROM budgets WHERE category='leisure,shopping'`).Scan(&name)
	if name != "Leisure & shopping" {
		t.Error("entertainment budget covers leisure and shopping", name)
	}
	// Converting into a non-empty database is refused.
	if _, err := Convert(d, src); err == nil {
		t.Error("double conversion accepted")
	}
}

func TestConvertFromV1DatabaseCarriesSecretsAndBankState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "finance.db")
	v1, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v1.Exec(testutil.V1Schema); err != nil {
		t.Fatal(err)
	}
	ts := "2026-09-01 00:00:00+00:00"
	for _, q := range []string{
		`INSERT INTO transactions(id,date,type,amount,comment,category,labels,debit_account,external_id,created_at) VALUES(10,'` + ts + `','expense',23.4,'Lidl','Food','groceries','swed','eb:1:abc','2026-09-01 10:00:00.123+03:00')`,
		`INSERT INTO transactions(id,date,type,amount,comment,category,labels,credit_account) VALUES(11,'` + ts + `','income',3000,'Nexos','Salary','nexos','swed')`,
		`INSERT INTO balances(date,total,swed,mbtc,btc_price,extra) VALUES('` + ts + `',2400,2000,0.01,40000,'{"acc_seb_savings":0}')`,
		`INSERT INTO accounts(key,label,"group",institution) VALUES('acc_seb_savings','SEB savings','cash','SEB')`,
		`INSERT INTO stock_trades(date,action,ticker,shares,price_per_share,currency,source) VALUES('` + ts + `','buy','AAPL',1,200,'USD','Revolut')`,
		`INSERT INTO budgets(id,name,kind,category,amount,period,fund,start_month) VALUES(1,'Gifts','spending','Gifts',600,'yearly',0,'')`,
		`INSERT INTO budget_amounts(budget_id,from_month,amount) VALUES(1,'',600)`,
		`INSERT INTO budget_settings(income_mode,gross_salary) VALUES('gross',8400)`,
		`INSERT INTO label_rules(label,category,comment_match) VALUES('coffee','','caffeine')`,
		`INSERT INTO ai_settings(gateway_url,api_key,model,provider) VALUES('','sk-x','claude-opus-5-5','anthropic')`,
		`INSERT INTO ai_contexts(content) VALUES('my brief')`,
		`INSERT INTO ai_chat_messages(role,content,created_at) VALUES('user','hi','` + ts + `')`,
		`INSERT INTO ai_spends(kind,model,cost_usd) VALUES('chat','m',0.12)`,
		`INSERT INTO ai_top_ups(amount_usd,note,occurred_on) VALUES(10,'card','` + ts + `')`,
		`INSERT INTO auth_settings(pin_hash,enabled,api_token_hash) VALUES('$2a$10$hash',1,'tok')`,
		`INSERT INTO webauthn_credentials(name,credential) VALUES('iPhone',X'7B22696422')`,
		`INSERT INTO bank_settings(application_id,private_key_pem,environment,redirect_url) VALUES('app','PEM','production','https://x/')`,
		`INSERT INTO bank_connections(id,aspsp_name,session_id,status,valid_until) VALUES(1,'Swedbank','sess','authorized','2027-03-30 16:32:32.94415+03:00')`,
		`INSERT INTO bank_account_links(id,connection_id,identification_hash,uid,iban,display_name,account_key,last_tx_date,bank_balance) VALUES(1,1,'h','u','LT1','Main','swed','` + ts + `',2000)`,
		`INSERT INTO bank_staged_txes(link_id,external_id,raw,date,type,category,comment,labels,debit_account,verdict,state,amount) VALUES(1,'eb:1:dismissed','{}','` + ts + `','expense','Entertainment','X','','swed','new','dismissed',5)`,
		`INSERT INTO bank_staged_txes(link_id,external_id,raw,date,type,category,comment,labels,debit_account,verdict,state,amount,pending) VALUES(1,'eb:1:open','{}','` + ts + `','expense','Food','MAXIMA','groceries','swed','pending','staged',9.5,1)`,
	} {
		if _, err := v1.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	v1.Close()
	src, err := ReadDB(path)
	if err != nil {
		t.Fatal(err)
	}
	d, rep := convert(t, src)
	requireVerified(t, d, src)
	if rep.InboxRows != 2 {
		t.Fatalf("%+v", rep)
	}
	var state, cat string
	d.QueryRow(`SELECT state FROM bank_inbox WHERE external_id='eb:1:dismissed'`).Scan(&state)
	if state != "dismissed" {
		t.Error("dismissals must survive the move", state)
	}
	d.QueryRow(`SELECT state, category FROM bank_inbox WHERE external_id='eb:1:open'`).Scan(&state, &cat)
	if state != "open" || cat != "food.groceries" {
		t.Error("open row mapped", state, cat)
	}
	var period string
	d.QueryRow(`SELECT period FROM budgets WHERE name='Gifts'`).Scan(&period)
	if period != "yearly" {
		t.Error("yearly period lost")
	}
	var created string
	d.QueryRow(`SELECT created_at FROM transactions WHERE id=10`).Scan(&created)
	if !strings.HasPrefix(created, "2026-09-01T07:00:00Z") {
		t.Error("created_at normalised to UTC", created)
	}
	var trAcct string
	d.QueryRow(`SELECT account_id FROM trades`).Scan(&trAcct)
	if trAcct != "revolut_stocks" {
		t.Error(trAcct)
	}
	// Not a v1 database.
	empty := filepath.Join(t.TempDir(), "x.db")
	e, _ := sql.Open("sqlite", "file:"+empty)
	e.Exec(`CREATE TABLE foo(x)`)
	e.Close()
	if _, err := ReadDB(empty); err == nil {
		t.Error("non-v1 database accepted")
	}
}

func TestVerifyCatchesLoss(t *testing.T) {
	src, _ := ReadJSON([]byte(v1JSON))
	d, _ := convert(t, src)
	d.Exec(`DELETE FROM transactions WHERE id=2`)
	d.Exec(`UPDATE balances SET value=value+100 WHERE account_id='swed' AND date='2026-01-31'`)
	v := Verify(d, src)
	if v.OK {
		t.Fatal("verification must fail on a lost row and an altered balance")
	}
	failed := map[string]bool{}
	for _, c := range v.Checks {
		if !c.OK {
			failed[c.Name] = true
		}
	}
	if !failed["every transaction carried over"] || !failed["every balance value carried over"] {
		t.Error(failed)
	}
	b, _ := json.Marshal(v)
	if !strings.Contains(string(b), "missing 1") {
		t.Error(string(b))
	}
}

// A v1 JSON export drops zero fields; an emptied wallet must read 0, not its
// last value carried forward (prod export 2026-10-04: BTC (M) +€509 for years).
func TestJSONExportOmittedZerosRestored(t *testing.T) {
	bs := []V1Balance{
		{ID: 3, Date: "2018-03-31", Values: map[string]float64{"swed": 240}},                                   // btc gone
		{ID: 1, Date: "2017-12-31", Values: map[string]float64{"swed": 10}},                                    // before btc existed
		{ID: 2, Date: "2018-02-28", Values: map[string]float64{"swed": 2, "mbtc": 509.05, "btc_price": 9000}}, // btc appears
		{ID: 4, Date: "2026-02-21", Values: map[string]float64{"swed": 5, "mbtc": 0.01, "btc_price": 60000}},  // back again
	}
	fillOmittedZeros(bs)
	if bs[0].Date != "2017-12-31" || len(bs[0].Values) != 1 {
		t.Fatalf("sorted, nothing invented before first sight: %+v", bs[0])
	}
	if v, ok := bs[2].Values["mbtc"]; !ok || v != 0 {
		t.Errorf("emptied wallet should be 0: %+v", bs[2].Values)
	}
	if _, ok := bs[2].Values["btc_price"]; ok {
		t.Error("a missing price is not a zero price")
	}
	if bs[3].Values["mbtc"] != 0.01 {
		t.Error("later value kept")
	}
}
