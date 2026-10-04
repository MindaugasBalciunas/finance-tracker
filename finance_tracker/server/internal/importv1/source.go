// Package importv1 converts a v1 Finance Tracker (the 1.x line) into the v2
// model, from either its SQLite database or a finances.json backup. The v1
// source is only ever read.
package importv1

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

// Data is everything v1 knew, normalised to v1's database vocabulary.
type Data struct {
	Transactions []V1Tx
	Balances     []V1Balance
	Accounts     []V1Account
	Trades       []V1Trade
	Assets       []V1Asset
	Budgets      []V1Budget
	BudgetSet    *V1BudgetSettings
	LabelRules   []V1Rule
	AI           *V1AI
	AIContext    string
	AIMessages   []V1Message
	AISpend      []V1Spend
	AITopups     []V1Topup
	Auth         *V1Auth
	Webauthn     []V1Cred
	Bank         *V1BankSettings
	BankConns    []V1Conn
	BankLinks    []V1Link
	BankStaged   []V1Staged
}

type V1Tx struct {
	ID         int64
	Date       string
	Type       string
	Amount     float64
	Comment    string
	Category   string
	Labels     string
	Debit      string
	Credit     string
	Source     string
	ExternalID string
	CreatedAt  string
}

type V1Balance struct {
	ID     int64
	Date   string
	Values map[string]float64 // v1 db column keys: seb, swed, seb_pen, rev_m, mbtc (qty), btc_price… plus acc_* extras
}

type V1Account struct {
	Key, Label, Group, Institution string
	Archived                       bool
	Sort                           int
}

type V1Trade struct {
	Date, Action, Ticker, Currency, Source, Notes string
	Shares, Price                                 float64
}

type V1Asset struct {
	Name, Type, PurchaseDate, ValuationDate, Notes          string
	PurchasePrice, CurrentValue                             float64
	LoanRemaining                                           float64
	LoanRemainingDate, LoanRate, LoanAccount, LoanPaidOff   string
	LoanMargin, LoanBaseRate, LoanMonthly                   float64
	LoanLabel, LoanRateReset                                string
}

type V1Budget struct {
	Name, Kind, Label, Category, StartMonth, Period string
	Amount                                          float64
	Fund                                    bool
	Amounts                                 []struct {
		From   string
		Amount float64
	}
}

type V1BudgetSettings struct {
	IncomeMode               string
	ManualIncome, Gross, Ded float64
}

type V1Rule struct{ Label, Category, Match string }

type V1AI struct {
	GatewayURL, APIKey, Model, Provider string
	Disabled                            bool
}

type V1Message struct{ Role, Content, CreatedAt string }

type V1Spend struct {
	Kind, Model, CreatedAt string
	Cost                   float64
	In, Out                int64
	Estimated              bool
}

type V1Topup struct {
	Amount              float64
	Note, On, CreatedAt string
}

type V1Auth struct {
	PinHash, TokenHash, TokenRWHash string
	Enabled                         bool
}

type V1Cred struct {
	Name       string
	Credential []byte
	CreatedAt  string
}

type V1BankSettings struct{ AppID, PrivateKey, Environment, RedirectURL string }

type V1Conn struct {
	ID                                                 int64
	ASPSP, Country, Session, Status, ValidUntil, Error string
	CreatedAt                                          string
}

type V1Link struct {
	ID, ConnectionID                                         int64
	Hash, UID, IBAN, Name, AccountKey, LastSynced, LastTx    string
	BankBalance                                              sql.NullFloat64
	BalCurrency, BalType, BalDate, BalFetched, AppliedThrough string
}

type V1Staged struct {
	LinkID                                                    sql.NullInt64
	ExternalID, Raw, RawPayee, RawDetails, RawCurrency        string
	BookingDate, Date, Type, Category, Comment, Labels        string
	Debit, Credit, Verdict, VerdictNote, State                string
	Amount                                                    float64
	Pending, Edited                                           bool
	MatchedTxID, ImportedTxID                                 sql.NullInt64
	FirstSeen, LastSeen                                       string
}

// day trims v1's "2010-06-10 00:00:00+00:00" to "2010-06-10".
func day(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

// ReadDB loads a v1 SQLite database read-only.
func ReadDB(path string) (*Data, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	d, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(10000)")
	if err != nil {
		return nil, err
	}
	defer d.Close()
	has := func(table string) bool {
		var n int
		d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n)
		return n > 0
	}
	if !has("transactions") || !has("balances") {
		return nil, fmt.Errorf("%s is not a v1 database", path)
	}
	out := &Data{}
	q := func(query string, scan func(*sql.Rows) error) error {
		rows, err := d.Query(query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			if err := scan(rows); err != nil {
				return err
			}
		}
		return rows.Err()
	}

	if err := q(`SELECT id,date,type,amount,COALESCE(comment,''),COALESCE(category,''),COALESCE(labels,''),
		COALESCE(debit_account,''),COALESCE(credit_account,''),COALESCE(source_account,''),COALESCE(external_id,''),COALESCE(created_at,'')
		FROM transactions ORDER BY id`, func(r *sql.Rows) error {
		var t V1Tx
		err := r.Scan(&t.ID, &t.Date, &t.Type, &t.Amount, &t.Comment, &t.Category, &t.Labels, &t.Debit, &t.Credit, &t.Source, &t.ExternalID, &t.CreatedAt)
		t.Date = day(t.Date)
		out.Transactions = append(out.Transactions, t)
		return err
	}); err != nil {
		return nil, fmt.Errorf("transactions: %w", err)
	}

	cols := []string{"seb", "swed", "swed_etf", "seb_pen", "luminor", "art", "cash", "rev_m", "rev_r", "rbtc", "mbtc", "btc_price", "rev_stocks", "ibkr_stocks"}
	if err := q(`SELECT id,date,`+strings.Join(cols, ",")+`,COALESCE(extra,'{}') FROM balances ORDER BY date, id`, func(r *sql.Rows) error {
		var b V1Balance
		vals := make([]sql.NullFloat64, len(cols))
		ptrs := []any{&b.ID, &b.Date}
		for i := range vals {
			ptrs = append(ptrs, &vals[i])
		}
		var extra string
		ptrs = append(ptrs, &extra)
		if err := r.Scan(ptrs...); err != nil {
			return err
		}
		b.Date = day(b.Date)
		b.Values = map[string]float64{}
		for i, c := range cols {
			if vals[i].Valid {
				b.Values[c] = vals[i].Float64
			}
		}
		var ex map[string]float64
		if json.Unmarshal([]byte(extra), &ex) == nil {
			for k, v := range ex {
				b.Values[k] = v
			}
		}
		out.Balances = append(out.Balances, b)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("balances: %w", err)
	}

	if has("accounts") {
		q(`SELECT key,label,"group",COALESCE(institution,''),archived,sort_order FROM accounts`, func(r *sql.Rows) error {
			var a V1Account
			err := r.Scan(&a.Key, &a.Label, &a.Group, &a.Institution, &a.Archived, &a.Sort)
			out.Accounts = append(out.Accounts, a)
			return err
		})
	}
	q(`SELECT date,action,ticker,shares,price_per_share,COALESCE(currency,'USD'),COALESCE(source,''),COALESCE(notes,'') FROM stock_trades ORDER BY date,id`, func(r *sql.Rows) error {
		var t V1Trade
		err := r.Scan(&t.Date, &t.Action, &t.Ticker, &t.Shares, &t.Price, &t.Currency, &t.Source, &t.Notes)
		t.Date = day(t.Date)
		out.Trades = append(out.Trades, t)
		return err
	})
	q(`SELECT name,type,COALESCE(purchase_date,''),purchase_price,COALESCE(current_value,0),COALESCE(valuation_date,''),COALESCE(notes,''),
		COALESCE(loan_remaining,0),COALESCE(loan_remaining_date,''),COALESCE(loan_rate,''),COALESCE(loan_account,''),COALESCE(loan_paid_off_date,''),
		COALESCE(loan_margin,0),COALESCE(loan_label,''),COALESCE(loan_base_rate,0),COALESCE(loan_rate_reset_date,''),COALESCE(loan_monthly_payment,0) FROM assets`, func(r *sql.Rows) error {
		var a V1Asset
		err := r.Scan(&a.Name, &a.Type, &a.PurchaseDate, &a.PurchasePrice, &a.CurrentValue, &a.ValuationDate, &a.Notes,
			&a.LoanRemaining, &a.LoanRemainingDate, &a.LoanRate, &a.LoanAccount, &a.LoanPaidOff, &a.LoanMargin, &a.LoanLabel, &a.LoanBaseRate, &a.LoanRateReset, &a.LoanMonthly)
		a.PurchaseDate, a.ValuationDate, a.LoanRemainingDate, a.LoanPaidOff, a.LoanRateReset = day(a.PurchaseDate), day(a.ValuationDate), day(a.LoanRemainingDate), day(a.LoanPaidOff), day(a.LoanRateReset)
		out.Assets = append(out.Assets, a)
		return err
	})
	budgetIdx := map[int64]int{}
	q(`SELECT id,name,kind,label,category,amount,fund,start_month,COALESCE(period,'monthly') FROM budgets ORDER BY id`, func(r *sql.Rows) error {
		var b V1Budget
		var id int64
		err := r.Scan(&id, &b.Name, &b.Kind, &b.Label, &b.Category, &b.Amount, &b.Fund, &b.StartMonth, &b.Period)
		budgetIdx[id] = len(out.Budgets)
		out.Budgets = append(out.Budgets, b)
		return err
	})
	if has("budget_amounts") {
		q(`SELECT budget_id,from_month,amount FROM budget_amounts ORDER BY id`, func(r *sql.Rows) error {
			var id int64
			var from string
			var amt float64
			err := r.Scan(&id, &from, &amt)
			if i, ok := budgetIdx[id]; ok {
				out.Budgets[i].Amounts = append(out.Budgets[i].Amounts, struct {
					From   string
					Amount float64
				}{from, amt})
			}
			return err
		})
	}
	q(`SELECT income_mode,manual_income,gross_salary,monthly_deductions FROM budget_settings LIMIT 1`, func(r *sql.Rows) error {
		var s V1BudgetSettings
		err := r.Scan(&s.IncomeMode, &s.ManualIncome, &s.Gross, &s.Ded)
		out.BudgetSet = &s
		return err
	})
	q(`SELECT label,category,comment_match FROM label_rules ORDER BY id`, func(r *sql.Rows) error {
		var x V1Rule
		err := r.Scan(&x.Label, &x.Category, &x.Match)
		out.LabelRules = append(out.LabelRules, x)
		return err
	})
	q(`SELECT gateway_url,api_key,model,provider,disabled FROM ai_settings LIMIT 1`, func(r *sql.Rows) error {
		var a V1AI
		err := r.Scan(&a.GatewayURL, &a.APIKey, &a.Model, &a.Provider, &a.Disabled)
		out.AI = &a
		return err
	})
	q(`SELECT content FROM ai_contexts ORDER BY id DESC LIMIT 1`, func(r *sql.Rows) error { return r.Scan(&out.AIContext) })
	q(`SELECT role,content,COALESCE(created_at,'') FROM ai_chat_messages ORDER BY id`, func(r *sql.Rows) error {
		var m V1Message
		err := r.Scan(&m.Role, &m.Content, &m.CreatedAt)
		out.AIMessages = append(out.AIMessages, m)
		return err
	})
	if has("ai_spends") {
		q(`SELECT kind,model,cost_usd,input_tokens,output_tokens,estimated,COALESCE(created_at,'') FROM ai_spends ORDER BY id`, func(r *sql.Rows) error {
			var s V1Spend
			err := r.Scan(&s.Kind, &s.Model, &s.Cost, &s.In, &s.Out, &s.Estimated, &s.CreatedAt)
			out.AISpend = append(out.AISpend, s)
			return err
		})
		q(`SELECT amount_usd,note,COALESCE(occurred_on,''),COALESCE(created_at,'') FROM ai_top_ups ORDER BY id`, func(r *sql.Rows) error {
			var t V1Topup
			err := r.Scan(&t.Amount, &t.Note, &t.On, &t.CreatedAt)
			t.On = day(t.On)
			out.AITopups = append(out.AITopups, t)
			return err
		})
	}
	q(`SELECT COALESCE(pin_hash,''),COALESCE(enabled,0),COALESCE(api_token_hash,''),COALESCE(api_token_rw_hash,'') FROM auth_settings LIMIT 1`, func(r *sql.Rows) error {
		var a V1Auth
		err := r.Scan(&a.PinHash, &a.Enabled, &a.TokenHash, &a.TokenRWHash)
		out.Auth = &a
		return err
	})
	q(`SELECT COALESCE(name,''),credential,COALESCE(created_at,'') FROM webauthn_credentials ORDER BY id`, func(r *sql.Rows) error {
		var c V1Cred
		err := r.Scan(&c.Name, &c.Credential, &c.CreatedAt)
		out.Webauthn = append(out.Webauthn, c)
		return err
	})
	if has("bank_settings") {
		q(`SELECT application_id,private_key_pem,environment,redirect_url FROM bank_settings LIMIT 1`, func(r *sql.Rows) error {
			var b V1BankSettings
			err := r.Scan(&b.AppID, &b.PrivateKey, &b.Environment, &b.RedirectURL)
			out.Bank = &b
			return err
		})
		q(`SELECT id,aspsp_name,aspsp_country,session_id,status,COALESCE(valid_until,''),last_error,COALESCE(created_at,'') FROM bank_connections ORDER BY id`, func(r *sql.Rows) error {
			var c V1Conn
			err := r.Scan(&c.ID, &c.ASPSP, &c.Country, &c.Session, &c.Status, &c.ValidUntil, &c.Error, &c.CreatedAt)
			out.BankConns = append(out.BankConns, c)
			return err
		})
		q(`SELECT id,COALESCE(connection_id,0),COALESCE(identification_hash,''),COALESCE(uid,''),COALESCE(iban,''),COALESCE(display_name,''),account_key,
			COALESCE(last_synced_at,''),COALESCE(last_tx_date,''),bank_balance,bank_balance_currency,bank_balance_type,COALESCE(bank_balance_date,''),
			COALESCE(bank_balance_fetched,''),COALESCE(balance_applied_through,'') FROM bank_account_links ORDER BY id`, func(r *sql.Rows) error {
			var l V1Link
			err := r.Scan(&l.ID, &l.ConnectionID, &l.Hash, &l.UID, &l.IBAN, &l.Name, &l.AccountKey, &l.LastSynced, &l.LastTx,
				&l.BankBalance, &l.BalCurrency, &l.BalType, &l.BalDate, &l.BalFetched, &l.AppliedThrough)
			out.BankLinks = append(out.BankLinks, l)
			return err
		})
		q(`SELECT link_id,COALESCE(external_id,''),COALESCE(raw,''),COALESCE(raw_payee,''),COALESCE(raw_details,''),COALESCE(raw_currency,''),
			COALESCE(booking_date,''),COALESCE(date,''),COALESCE(type,''),COALESCE(category,''),COALESCE(comment,''),labels,debit_account,credit_account,
			COALESCE(verdict,''),verdict_note,COALESCE(state,''),COALESCE(amount,0),COALESCE(pending,0),COALESCE(edited,0),matched_tx_id,imported_tx_id,
			COALESCE(first_seen_at,''),COALESCE(last_seen_at,'') FROM bank_staged_txes ORDER BY id`, func(r *sql.Rows) error {
			var s V1Staged
			err := r.Scan(&s.LinkID, &s.ExternalID, &s.Raw, &s.RawPayee, &s.RawDetails, &s.RawCurrency, &s.BookingDate, &s.Date, &s.Type, &s.Category,
				&s.Comment, &s.Labels, &s.Debit, &s.Credit, &s.Verdict, &s.VerdictNote, &s.State, &s.Amount, &s.Pending, &s.Edited, &s.MatchedTxID,
				&s.ImportedTxID, &s.FirstSeen, &s.LastSeen)
			s.Date, s.BookingDate = day(s.Date), day(s.BookingDate)
			out.BankStaged = append(out.BankStaged, s)
			return err
		})
	}
	return out, nil
}

// ReadJSON loads a v1 finances.json backup (schema 1–7).
func ReadJSON(raw []byte) (*Data, error) {
	var in struct {
		Transactions []struct {
			ID         int64   `json:"id"`
			Date       string  `json:"date"`
			Type       string  `json:"type"`
			Amount     float64 `json:"amount_eur"`
			Category   string  `json:"category"`
			Comment    string  `json:"comment"`
			Labels     string  `json:"labels"`
			Debit      string  `json:"debit_account"`
			Credit     string  `json:"credit_account"`
			Source     string  `json:"source_account"`
			ExternalID string  `json:"external_id"`
		} `json:"transactions"`
		Balances []map[string]any `json:"balances"`
		Trades   []struct {
			Date     string  `json:"date"`
			Action   string  `json:"action"`
			Ticker   string  `json:"ticker"`
			Shares   float64 `json:"shares"`
			Price    float64 `json:"price_per_share"`
			Currency string  `json:"currency"`
			Source   string  `json:"source"`
			Notes    string  `json:"notes"`
		} `json:"stock_trades"`
		Assets []struct {
			Name              string  `json:"name"`
			Type              string  `json:"type"`
			PurchaseDate      string  `json:"purchase_date"`
			PurchasePrice     float64 `json:"purchase_price_eur"`
			CurrentValue      float64 `json:"current_value_eur"`
			ValuationDate     string  `json:"valuation_date"`
			Notes             string  `json:"notes"`
			LoanRemaining     float64 `json:"loan_remaining_eur"`
			LoanRemainingDate string  `json:"loan_remaining_date"`
			LoanRate          string  `json:"loan_rate"`
			LoanAccount       string  `json:"loan_account"`
			LoanPaidOff       string  `json:"loan_paid_off_date"`
			LoanMargin        float64 `json:"loan_margin"`
			LoanLabel         string  `json:"loan_label"`
			LoanBaseRate      float64 `json:"loan_base_rate"`
			LoanRateReset     string  `json:"loan_rate_reset_date"`
			LoanMonthly       float64 `json:"loan_monthly_payment_eur"`
		} `json:"assets"`
		Budgets []struct {
			Name       string  `json:"name"`
			Kind       string  `json:"kind"`
			Label      string  `json:"label"`
			Category   string  `json:"category"`
			Amount     float64 `json:"amount"`
			Fund       bool    `json:"fund"`
			StartMonth string  `json:"start_month"`
			Period     string  `json:"period"`
			Amounts    []struct {
				From   string  `json:"from_month"`
				Amount float64 `json:"amount"`
			} `json:"amounts"`
		} `json:"budgets"`
		LabelRules []struct {
			Label    string `json:"label"`
			Category string `json:"category"`
			Match    string `json:"comment_match"`
		} `json:"label_rules"`
		BudgetSettings *struct {
			IncomeMode string  `json:"income_mode"`
			Manual     float64 `json:"manual_income"`
			Gross      float64 `json:"gross_salary"`
			Ded        float64 `json:"monthly_deductions"`
		} `json:"budget_settings"`
		AISettings *struct {
			GatewayURL string `json:"gateway_url"`
			APIKey     string `json:"api_key"`
			Model      string `json:"model"`
			Provider   string `json:"provider"`
			Enabled    *bool  `json:"enabled"`
		} `json:"ai_settings"`
		AIContext string `json:"ai_context"`
		Accounts  []struct {
			Key         string `json:"key"`
			Label       string `json:"label"`
			Group       string `json:"group"`
			Institution string `json:"institution"`
			Archived    bool   `json:"archived"`
			Sort        int    `json:"sort_order"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("not a v1 backup: %w", err)
	}
	out := &Data{AIContext: in.AIContext}
	for _, t := range in.Transactions {
		out.Transactions = append(out.Transactions, V1Tx{ID: t.ID, Date: day(t.Date), Type: t.Type, Amount: t.Amount, Comment: t.Comment,
			Category: t.Category, Labels: t.Labels, Debit: t.Debit, Credit: t.Credit, Source: t.Source, ExternalID: t.ExternalID})
	}
	// JSON keys → v1 db column keys.
	jsonKey := map[string]string{"seb_pension": "seb_pen", "revolut_m": "rev_m", "revolut_r": "rev_r", "btc_r": "rbtc", "btc_m": "mbtc",
		"btc_price_eur": "btc_price", "revolut_stocks": "rev_stocks", "swed_pension": "seb_pen"}
	for i, b := range in.Balances {
		v := V1Balance{ID: int64(i + 1), Values: map[string]float64{}}
		if id, ok := b["id"].(float64); ok {
			v.ID = int64(id)
		}
		v.Date, _ = b["date"].(string)
		v.Date = day(v.Date)
		for k, val := range b {
			switch k {
			case "id", "date", "total_eur", "time", "note":
				continue
			case "extra":
				if m, ok := val.(map[string]any); ok {
					for ek, ev := range m {
						if f, ok := ev.(float64); ok {
							v.Values[ek] = f
						}
					}
				}
				continue
			}
			f, ok := val.(float64)
			if !ok {
				continue
			}
			if mk, ok := jsonKey[k]; ok {
				k = mk
			}
			v.Values[k] = f
		}
		out.Balances = append(out.Balances, v)
	}
	for _, t := range in.Trades {
		out.Trades = append(out.Trades, V1Trade{Date: day(t.Date), Action: t.Action, Ticker: t.Ticker, Shares: t.Shares, Price: t.Price, Currency: t.Currency, Source: t.Source, Notes: t.Notes})
	}
	for _, a := range in.Assets {
		out.Assets = append(out.Assets, V1Asset{Name: a.Name, Type: a.Type, PurchaseDate: day(a.PurchaseDate), PurchasePrice: a.PurchasePrice,
			CurrentValue: a.CurrentValue, ValuationDate: day(a.ValuationDate), Notes: a.Notes, LoanRemaining: a.LoanRemaining,
			LoanRemainingDate: day(a.LoanRemainingDate), LoanRate: a.LoanRate, LoanAccount: a.LoanAccount, LoanPaidOff: day(a.LoanPaidOff),
			LoanMargin: a.LoanMargin, LoanLabel: a.LoanLabel, LoanBaseRate: a.LoanBaseRate, LoanRateReset: day(a.LoanRateReset), LoanMonthly: a.LoanMonthly})
	}
	for _, b := range in.Budgets {
		vb := V1Budget{Name: b.Name, Kind: b.Kind, Label: b.Label, Category: b.Category, Amount: b.Amount, Fund: b.Fund, StartMonth: b.StartMonth, Period: b.Period}
		for _, a := range b.Amounts {
			vb.Amounts = append(vb.Amounts, struct {
				From   string
				Amount float64
			}{a.From, a.Amount})
		}
		out.Budgets = append(out.Budgets, vb)
	}
	for _, r := range in.LabelRules {
		out.LabelRules = append(out.LabelRules, V1Rule{r.Label, r.Category, r.Match})
	}
	if s := in.BudgetSettings; s != nil {
		out.BudgetSet = &V1BudgetSettings{IncomeMode: s.IncomeMode, ManualIncome: s.Manual, Gross: s.Gross, Ded: s.Ded}
	}
	if a := in.AISettings; a != nil {
		out.AI = &V1AI{GatewayURL: a.GatewayURL, APIKey: a.APIKey, Model: a.Model, Provider: a.Provider, Disabled: a.Enabled != nil && !*a.Enabled}
	}
	for _, a := range in.Accounts {
		out.Accounts = append(out.Accounts, V1Account{Key: a.Key, Label: a.Label, Group: a.Group, Institution: a.Institution, Archived: a.Archived, Sort: a.Sort})
	}
	return out, nil
}
