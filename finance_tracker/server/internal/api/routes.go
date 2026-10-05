package api

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ft/internal/ai"
	"ft/internal/auth"
	"ft/internal/bank"
	"ft/internal/cfo"
	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/market"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

type H = func(w http.ResponseWriter, r *http.Request) (any, error)

func (s *Server) routes() {
	s.handle("GET /api/health", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]any{"ok": true, "version": s.Version}, nil
	})
	s.authRoutes()
	s.ledgerRoutes()
	s.wealthRoutes()
	s.planRoutes()
	s.insightRoutes()
	s.bankRoutes()
	s.aiRoutes()
	s.dataRoutes()
}

// ── auth ────────────────────────────────────────────────────────────

func (s *Server) unlocked(r *http.Request) bool {
	if !s.Auth.Enabled() {
		return true
	}
	c, err := r.Cookie(sessionCookie)
	return err == nil && s.Auth.ValidSession(c.Value)
}

var errLocked = &HTTPError{401, "locked"}

func (s *Server) authRoutes() {
	s.handle("GET /api/auth/status", func(w http.ResponseWriter, r *http.Request) (any, error) {
		tok := ""
		if c, err := r.Cookie(sessionCookie); err == nil {
			tok = c.Value
		}
		return s.Auth.Status(tok), nil
	})
	s.handle("POST /api/auth/pin/login", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct{ Pin string }
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		tok, err := s.Auth.VerifyPin(in.Pin)
		if err != nil {
			if errors.Is(err, auth.ErrTooManyAttempts) {
				return nil, &HTTPError{429, err.Error()}
			}
			return nil, &HTTPError{401, err.Error()}
		}
		setSession(w, r, tok)
		return map[string]bool{"ok": true}, nil
	})
	s.handle("POST /api/auth/pin/setup", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !s.unlocked(r) {
			return nil, errLocked
		}
		var in struct {
			CurrentPin string `json:"current_pin"`
			Pin        string `json:"pin"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if err := s.Auth.SetupPin(in.CurrentPin, in.Pin); err != nil {
			return nil, err
		}
		tok, err := s.Auth.VerifyPin(in.Pin)
		if err == nil {
			setSession(w, r, tok)
		}
		return map[string]bool{"ok": true}, nil
	})
	s.handle("POST /api/auth/pin/disable", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !s.unlocked(r) {
			return nil, errLocked
		}
		var in struct{ Pin string }
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, s.Auth.DisableLock(in.Pin)
	})
	s.handle("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if c, err := r.Cookie(sessionCookie); err == nil {
			s.Auth.DestroySession(c.Value)
		}
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
		return map[string]bool{"ok": true}, nil
	})
	s.handle("POST /api/auth/passkey/register/begin", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !s.unlocked(r) {
			return nil, errLocked
		}
		h, o := hostOrigin(r)
		return s.Auth.BeginRegistration(h, o)
	})
	s.handle("POST /api/auth/passkey/register/finish", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !s.unlocked(r) {
			return nil, errLocked
		}
		h, o := hostOrigin(r)
		return map[string]bool{"ok": true}, s.Auth.FinishRegistration(h, o, r.URL.Query().Get("name"), r)
	})
	s.handle("POST /api/auth/passkey/login/begin", func(w http.ResponseWriter, r *http.Request) (any, error) {
		h, o := hostOrigin(r)
		return s.Auth.BeginLogin(h, o)
	})
	s.handle("POST /api/auth/passkey/login/finish", func(w http.ResponseWriter, r *http.Request) (any, error) {
		h, o := hostOrigin(r)
		tok, err := s.Auth.FinishLogin(h, o, r)
		if err != nil {
			return nil, &HTTPError{401, err.Error()}
		}
		setSession(w, r, tok)
		return map[string]bool{"ok": true}, nil
	})
	s.handle("GET /api/auth/passkeys", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !s.unlocked(r) {
			return nil, errLocked
		}
		return s.Auth.Passkeys()
	})
	s.handle("DELETE /api/auth/passkeys/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !s.unlocked(r) {
			return nil, errLocked
		}
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, s.Auth.DeletePasskey(id)
	})
	s.handle("POST /api/auth/token", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !s.unlocked(r) {
			return nil, errLocked
		}
		var in struct{ RW bool }
		decode(r, &in)
		tok, err := s.Auth.MintToken(in.RW)
		return map[string]string{"token": tok}, err
	})
	s.handle("DELETE /api/auth/token", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !s.unlocked(r) {
			return nil, errLocked
		}
		return map[string]bool{"ok": true}, s.Auth.RevokeToken(r.URL.Query().Get("rw") == "1")
	})
}

// ── ledger ──────────────────────────────────────────────────────────

func filterFrom(r *http.Request) ledger.Filter {
	q := r.URL.Query()
	return ledger.Filter{From: q.Get("from"), To: q.Get("to"), Kind: q.Get("kind"), Categories: qlist(r, "category"), Accounts: qlist(r, "account"),
		Tags: qlist(r, "tag"), TagsAll: q.Get("tags_all") == "1", Merchant: q.Get("merchant"), Query: q.Get("q"), Source: q.Get("source"),
		Min: parseCents(q.Get("min")), Max: parseCents(q.Get("max")), Sort: q.Get("sort"), Limit: qint(r, "limit", 100), Offset: qint(r, "offset", 0)}
}

func parseCents(s string) money.Cents {
	var c money.Cents
	if s != "" {
		c.UnmarshalJSON([]byte(s))
	}
	return c
}

type txInput struct {
	ledger.Tx
	AutoFill       bool     `json:"auto_fill"`       // run rules/history on fields left empty
	SuppressedTags []string `json:"suppressed_tags"` // rule tags the user removed in the form
}

func (s *Server) ledgerRoutes() {
	s.handle("GET /api/transactions", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return ledger.List(s.DB, filterFrom(r))
	})
	s.handle("GET /api/transactions/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		t, err := ledger.Get(s.DB, id)
		if err != nil {
			return nil, notFound("transaction not found")
		}
		parts, _ := ledger.All(s.DB, ledger.Filter{})
		var children []ledger.Tx
		for _, p := range parts {
			if p.SplitOf == id {
				children = append(children, p)
			}
		}
		return map[string]any{"transaction": t, "parts": children}, nil
	})
	s.handle("POST /api/transactions", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in txInput
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		t := in.Tx
		t.ID, t.ExternalID, t.SplitOf = 0, "", 0
		if t.Source == "" {
			t.Source = "manual"
		}
		if in.AutoFill || t.Category == "" {
			eng, err := ledger.LoadEngine(s.DB)
			if err != nil {
				return nil, err
			}
			eng.Apply(&t, t.Category == "", in.SuppressedTags)
			if t.Category == "" {
				if t.Kind == "income" {
					t.Category = "other_income"
				} else {
					t.Category, t.Kind = "other", "expense"
				}
			}
		}
		if err := ledger.Validate(s.DB, &t); err != nil {
			return nil, err
		}
		if err := ledger.Insert(s.DB, &t); err != nil {
			return nil, err
		}
		moved := s.moveBalances(t, 1)
		return map[string]any{"transaction": t, "balances_moved": moved}, nil
	})
	s.handle("PUT /api/transactions/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		cur, err := ledger.Get(s.DB, id)
		if err != nil {
			return nil, notFound("transaction not found")
		}
		var in ledger.Tx
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		in.ID, in.ExternalID, in.SplitOf, in.Source = id, cur.ExternalID, cur.SplitOf, cur.Source
		if err := ledger.Validate(s.DB, &in); err != nil {
			return nil, err
		}
		if err := ledger.Update(s.DB, &in); err != nil {
			return nil, err
		}
		if cur.Date != in.Date || cur.Amount != in.Amount || cur.AccountID != in.AccountID || cur.ToAccountID != in.ToAccountID || cur.Kind != in.Kind {
			s.moveBalances(cur, -1)
			s.moveBalances(in, 1)
		}
		return in, nil
	})
	s.handle("DELETE /api/transactions/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		cur, err := ledger.Get(s.DB, id)
		if err != nil {
			return nil, notFound("transaction not found")
		}
		// A deleted bank row goes back to the inbox rather than vanishing.
		s.DB.Exec(`UPDATE bank_inbox SET state='open', imported_tx_id=NULL, matched_tx_id=NULL, verdict='new' WHERE imported_tx_id=?`, id)
		if err := ledger.Delete(s.DB, id); err != nil {
			return nil, err
		}
		s.moveBalances(cur, -1)
		return map[string]bool{"ok": true}, nil
	})
	s.handle("POST /api/transactions/bulk", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			IDs         []int64  `json:"ids"`
			SetCategory string   `json:"set_category"`
			SetMerchant *string  `json:"set_merchant"`
			AddTags     []string `json:"add_tags"`
			RemoveTags  []string `json:"remove_tags"`
			Delete      bool     `json:"delete"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		tx, err := s.DB.Begin()
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		n := 0
		for _, id := range in.IDs {
			t, err := ledger.Get(tx, id)
			if err != nil {
				continue
			}
			if in.Delete {
				ledger.Delete(tx, id)
				n++
				continue
			}
			if in.SetCategory != "" {
				t.Category, t.Kind = in.SetCategory, ""
			}
			if in.SetMerchant != nil {
				t.Merchant = *in.SetMerchant
			}
			t.Tags = append(t.Tags, in.AddTags...)
			if len(in.RemoveTags) > 0 {
				rm := map[string]bool{}
				for _, x := range ledger.NormalizeTags(in.RemoveTags) {
					rm[x] = true
				}
				var keep []string
				for _, x := range t.Tags {
					if !rm[x] {
						keep = append(keep, x)
					}
				}
				t.Tags = keep
			}
			if err := ledger.Validate(tx, &t); err != nil {
				return nil, err
			}
			ledger.Update(tx, &t)
			n++
		}
		return map[string]int{"changed": n}, tx.Commit()
	})
	s.handle("POST /api/transactions/suggest", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in ledger.Tx
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		eng, err := ledger.LoadEngine(s.DB)
		if err != nil {
			return nil, err
		}
		t := in
		out := eng.Apply(&t, true, nil)
		return map[string]any{"suggestion": t, "why": out}, nil
	})
	s.handle("POST /api/transactions/{id}/split", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		var in struct {
			Parts []ledger.SplitPart `json:"parts"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return ledger.Split(s.DB, id, in.Parts)
	})
	s.handle("POST /api/transactions/{id}/unsplit", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, ledger.Unsplit(s.DB, id)
	})
	s.handle("GET /api/owed", func(w http.ResponseWriter, r *http.Request) (any, error) {
		txs, err := ledger.All(s.DB, ledger.Filter{Query: "owed:"})
		if err != nil {
			return nil, err
		}
		by := map[string]money.Cents{}
		for _, t := range txs {
			for _, tag := range t.Tags {
				if p, ok := strings.CutPrefix(tag, "owed:"); ok {
					if t.Kind == "income" {
						by[p] -= t.Amount
					} else {
						by[p] += t.Amount
					}
				}
			}
		}
		return by, nil
	})

	s.handle("GET /api/categories", func(w http.ResponseWriter, r *http.Request) (any, error) { return ledger.ListCategories(s.DB) })
	s.handle("POST /api/categories", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var c ledger.Category
		if err := decode(r, &c); err != nil {
			return nil, err
		}
		return c, ledger.SaveCategory(s.DB, &c)
	})
	s.handle("PUT /api/categories/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var c ledger.Category
		if err := decode(r, &c); err != nil {
			return nil, err
		}
		c.ID = r.PathValue("id")
		return c, ledger.SaveCategory(s.DB, &c)
	})

	s.handle("GET /api/accounts", func(w http.ResponseWriter, r *http.Request) (any, error) {
		accts, err := ledger.ListAccounts(s.DB)
		if err != nil {
			return nil, err
		}
		book, err := wealth.LoadBook(s.DB)
		if err != nil {
			return nil, err
		}
		type row struct {
			ledger.Account
			Balance     *money.Cents `json:"balance"`
			BalanceDate string       `json:"balance_date"`
			Quantity    *float64     `json:"quantity,omitempty"`
			Source      string       `json:"source"`
		}
		out := []row{}
		for _, a := range accts {
			x := row{Account: a}
			if p, ok := book.At(a.ID, today()); ok {
				v := p.Value
				x.Balance, x.BalanceDate, x.Quantity, x.Source = &v, p.Date, p.Quantity, p.Source
			}
			out = append(out, x)
		}
		return out, nil
	})
	saveAccount := func(w http.ResponseWriter, r *http.Request) (any, error) {
		var a ledger.Account
		if err := decode(r, &a); err != nil {
			return nil, err
		}
		if id := r.PathValue("id"); id != "" {
			a.ID = id
		} else if a.ID == "" {
			a.ID = ledger.SlugID(a.Name)
			if _, err := ledger.GetAccount(s.DB, a.ID); err == nil {
				return nil, bad("an account with this name already exists")
			}
		}
		return ledger.SaveAccount(s.DB, a)
	}
	s.handle("POST /api/accounts", saveAccount)
	s.handle("PUT /api/accounts/{id}", saveAccount)
	s.handle("DELETE /api/accounts/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]bool{"ok": true}, ledger.DeleteAccount(s.DB, r.PathValue("id"))
	})

	s.handle("GET /api/tags", func(w http.ResponseWriter, r *http.Request) (any, error) { return ledger.ListTags(s.DB) })
	s.handle("GET /api/tags/suggestions", func(w http.ResponseWriter, r *http.Request) (any, error) {
		tags, err := ledger.ListTags(s.DB)
		if err != nil {
			return nil, err
		}
		sg := ledger.SuggestTagMerges(tags)
		if sg == nil {
			sg = []ledger.MergeSuggestion{}
		}
		return sg, nil
	})
	s.handle("POST /api/tags/rename", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct{ From, To string }
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		n, err := ledger.RenameTag(s.DB, in.From, strings.ToLower(strings.TrimSpace(in.To)))
		return map[string]int{"changed": n}, err
	})
	s.handle("GET /api/merchants", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return ledger.ListMerchants(s.DB, r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	})

	s.handle("GET /api/rules", func(w http.ResponseWriter, r *http.Request) (any, error) { return ledger.ListRules(s.DB) })
	saveRule := func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in ledger.Rule
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if id := r.PathValue("id"); id != "" {
			in.ID, _ = idParam(r, "id")
		}
		if in.Pattern == "" && in.WhenCategory == "" {
			return nil, bad("a rule needs text to match or a category condition")
		}
		return in, ledger.SaveRule(s.DB, &in)
	}
	s.handle("POST /api/rules", saveRule)
	s.handle("PUT /api/rules/{id}", saveRule)
	s.handle("DELETE /api/rules/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, ledger.DeleteRule(s.DB, id)
	})
	s.handle("POST /api/rules/preview", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in ledger.Rule
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		n, sample, err := ledger.PreviewRule(s.DB, in)
		return map[string]any{"matches": n, "sample": sample}, err
	})
	s.handle("POST /api/rules/{id}/apply", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		rules, _ := ledger.ListRules(s.DB)
		for _, ru := range rules {
			if ru.ID == id {
				n, err := ledger.ApplyRuleToHistory(s.DB, ru)
				return map[string]int{"changed": n}, err
			}
		}
		return nil, notFound("rule not found")
	})
}

// vagueCategories are top-level expense categories a row can sit in without
// saying much; the tidy-up queue offers sharper leaves for them.
var vagueCategories = []string{"other", "leisure", "shopping", "food", "housing", "finance", "health", "transport", "utilities", "subscriptions", "kids", "travel"}

type tidyRow struct {
	ledger.Tx
	Suggested string `json:"suggested,omitempty"` // from rules or history
	Why       string `json:"why,omitempty"`
}

// tidyQueue lists the last six months' expenses that only carry a broad
// category, with what rules and history would suggest.
func (s *Server) tidyQueue() ([]tidyRow, error) {
	txs, err := ledger.All(s.DB, ledger.Filter{From: time.Now().AddDate(0, -6, 0).Format("2006-01-02"), Kind: "expense"})
	if err != nil {
		return nil, err
	}
	eng, err := ledger.LoadEngine(s.DB)
	if err != nil {
		return nil, err
	}
	vague := map[string]bool{}
	for _, c := range vagueCategories {
		vague[c] = true
	}
	out := []tidyRow{}
	for i := len(txs) - 1; i >= 0; i-- {
		t := txs[i]
		if !vague[t.Category] {
			continue
		}
		row := tidyRow{Tx: t}
		probe := ledger.Tx{Kind: t.Kind, Merchant: t.Merchant, Note: t.Note}
		res := eng.Apply(&probe, true, nil)
		if probe.Category != "" && probe.Category != t.Category && strings.HasPrefix(probe.Category, t.Category+".") || (t.Category == "other" && probe.Category != "" && probe.Category != "other") {
			row.Suggested = probe.Category
			row.Why = "your rules"
			if res.Learned {
				row.Why = "what you filed " + firstNonEmptyStr(t.Merchant, "it") + " under before"
			}
		}
		out = append(out, row)
		if len(out) >= 200 {
			break
		}
	}
	return out, nil
}

func firstNonEmptyStr(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// moveBalances applies (sign=1) or reverses (sign=-1) a hand-entered
// transaction's effect on account balances. Bank rows never move balances:
// the bank states its own balance on every sync.
func (s *Server) moveBalances(t ledger.Tx, sign money.Cents) []string {
	if t.Source == "bank" || t.Source == "import" || t.SplitOf != 0 {
		return nil
	}
	var moved []string
	for acct, delta := range wealth.TxDeltas(t.Kind, t.AccountID, t.ToAccountID, t.Amount) {
		if ok, err := wealth.ApplyDelta(s.DB, acct, t.Date, delta*sign); err == nil && ok {
			moved = append(moved, acct)
		}
	}
	return moved
}

// ── wealth ──────────────────────────────────────────────────────────

func (s *Server) wealthRoutes() {
	s.handle("GET /api/networth", func(w http.ResponseWriter, r *http.Request) (any, error) {
		book, err := wealth.LoadBook(s.DB)
		if err != nil {
			return nil, err
		}
		d := r.URL.Query().Get("date")
		if d == "" {
			d = today()
		}
		return book.SnapshotAt(d, true), nil
	})
	s.handle("GET /api/networth/history", func(w http.ResponseWriter, r *http.Request) (any, error) {
		book, err := wealth.LoadBook(s.DB)
		if err != nil {
			return nil, err
		}
		q := r.URL.Query()
		hist := book.History(q.Get("from"), q.Get("to"), q.Get("step"))
		if q.Get("accounts") == "1" {
			for i := range hist {
				hist[i] = book.SnapshotAt(hist[i].Date, true)
				hist[i].Stale = nil
			}
		}
		return hist, nil
	})
	s.handle("GET /api/networth/movement", func(w http.ResponseWriter, r *http.Request) (any, error) {
		book, err := wealth.LoadBook(s.DB)
		if err != nil {
			return nil, err
		}
		q := r.URL.Query()
		from, to := q.Get("from"), q.Get("to")
		if to == "" {
			to = today()
		}
		if from == "" {
			from = time.Now().AddDate(-1, 0, 0).Format("2006-01-02")
		}
		mv := book.Movement(from, to)
		if mv == nil {
			mv = []wealth.Move{}
		}
		return mv, nil
	})
	s.handle("GET /api/balances", func(w http.ResponseWriter, r *http.Request) (any, error) {
		book, err := wealth.LoadBook(s.DB)
		if err != nil {
			return nil, err
		}
		pts := book.Series(r.URL.Query().Get("account"))
		if pts == nil {
			pts = []wealth.Point{}
		}
		return pts, nil
	})
	s.handle("POST /api/balances", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Date   string `json:"date"`
			Values []struct {
				AccountID string       `json:"account_id"`
				Value     *money.Cents `json:"value"`
				Quantity  *float64     `json:"quantity"`
				Price     *money.Cents `json:"price"`
			} `json:"values"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if in.Date == "" {
			in.Date = today()
		}
		tx, err := s.DB.Begin()
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		n := 0
		for _, v := range in.Values {
			if _, err := ledger.GetAccount(tx, v.AccountID); err != nil {
				return nil, bad("unknown account " + v.AccountID)
			}
			val := money.Cents(0)
			switch {
			case v.Quantity != nil && v.Price != nil:
				val = money.FromFloat(*v.Quantity * v.Price.Float())
			case v.Value != nil:
				val = *v.Value
			default:
				continue
			}
			if a, _ := ledger.GetAccount(tx, v.AccountID); a.Kind == "loan" && val > 0 {
				val = -val
			}
			if err := wealth.SetBalance(tx, v.AccountID, in.Date, val, v.Quantity, v.Price, "manual"); err != nil {
				return nil, err
			}
			n++
		}
		return map[string]int{"saved": n}, tx.Commit()
	})
	s.handle("DELETE /api/balances", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]bool{"ok": true}, wealth.DeleteBalance(s.DB, r.URL.Query().Get("account"), r.URL.Query().Get("date"))
	})
	s.handle("GET /api/loans", func(w http.ResponseWriter, r *http.Request) (any, error) {
		book, err := wealth.LoadBook(s.DB)
		if err != nil {
			return nil, err
		}
		txs, _ := ledger.All(s.DB, ledger.Filter{From: time.Now().AddDate(-1, 0, 0).Format("2006-01-02")})
		return wealth.Loans(book, txs, time.Now()), nil
	})
	s.handle("GET /api/portfolio", func(w http.ResponseWriter, r *http.Request) (any, error) {
		trades, err := wealth.ListTrades(s.DB)
		if err != nil {
			return nil, err
		}
		return wealth.BuildPortfolio(trades, r.URL.Query().Get("live") != "0"), nil
	})
	s.handle("GET /api/portfolio/history", func(w http.ResponseWriter, r *http.Request) (any, error) {
		trades, err := wealth.ListTrades(s.DB)
		if err != nil {
			return nil, err
		}
		// Closes: daily up to 3 months, weekly beyond (Yahoo's own split).
		rng, from := "max", ""
		now := time.Now()
		switch r.URL.Query().Get("range") {
		case "3m":
			rng, from = "3mo", now.AddDate(0, -3, 0).Format("2006-01-02")
		case "6m":
			rng, from = "6mo", now.AddDate(0, -6, 0).Format("2006-01-02")
		case "ytd":
			rng, from = "ytd", strconv.Itoa(now.Year())+"-01-01"
		case "1y":
			rng, from = "1y", now.AddDate(-1, 0, 0).Format("2006-01-02")
		case "3y":
			rng, from = "5y", now.AddDate(-3, 0, 0).Format("2006-01-02")
		}
		return wealth.PortfolioHistory(trades, from, now.Format("2006-01-02"),
			func(tk string) ([]market.HistoryPoint, error) { return market.History(tk, rng) }, wealth.LiveEUR()), nil
	})
	s.handle("GET /api/portfolio/scenarios", func(w http.ResponseWriter, r *http.Request) (any, error) {
		trades, err := wealth.ListTrades(s.DB)
		if err != nil {
			return nil, err
		}
		return wealth.BuildScenarios(wealth.BuildPortfolio(trades, true), func(t string) (float64, float64, float64, int, string, error) {
			a, err := market.Analyst(t)
			if err != nil {
				return 0, 0, 0, 0, "", err
			}
			basis := "analyst targets"
			if a.NumAnalysts == 0 {
				basis = "52-week range"
			}
			return a.TargetLow, a.TargetMean, a.TargetHigh, a.NumAnalysts, basis, nil
		}), nil
	})
	s.handle("GET /api/trades", func(w http.ResponseWriter, r *http.Request) (any, error) { return wealth.ListTrades(s.DB) })
	saveTrade := func(w http.ResponseWriter, r *http.Request) (any, error) {
		var t wealth.Trade
		if err := decode(r, &t); err != nil {
			return nil, err
		}
		if r.PathValue("id") != "" {
			t.ID, _ = idParam(r, "id")
		}
		return t, wealth.SaveTrade(s.DB, &t)
	}
	s.handle("POST /api/trades", saveTrade)
	s.handle("PUT /api/trades/{id}", saveTrade)
	s.handle("DELETE /api/trades/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, wealth.DeleteTrade(s.DB, id)
	})
	ticker := func(r *http.Request) (string, error) {
		t := strings.ToUpper(r.PathValue("ticker"))
		if !wealth.ValidTicker(t) {
			return "", bad("invalid ticker")
		}
		return t, nil
	}
	s.handle("GET /api/market/quote/{ticker}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		t, err := ticker(r)
		if err != nil {
			return nil, err
		}
		return market.Fetch(t)
	})
	s.handle("GET /api/market/history/{ticker}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		t, err := ticker(r)
		if err != nil {
			return nil, err
		}
		rng := r.URL.Query().Get("range")
		switch rng {
		case "1mo", "3mo", "6mo", "ytd", "1y", "2y", "5y", "max":
		default:
			rng = "1y"
		}
		return market.History(t, rng)
	})
	s.handle("GET /api/market/analyst/{ticker}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		t, err := ticker(r)
		if err != nil {
			return nil, err
		}
		return market.Analyst(t)
	})
}

// ── plan ────────────────────────────────────────────────────────────

func (s *Server) planRoutes() {
	s.handle("GET /api/plan", func(w http.ResponseWriter, r *http.Request) (any, error) {
		m := r.URL.Query().Get("month")
		if _, err := time.Parse("2006-01", m); err != nil {
			m = time.Now().Format("2006-01")
		}
		return cfo.BudgetReport(s.DB, m, time.Now())
	})
	s.handle("GET /api/prefs", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return loadPrefs(s.DB), nil
	})
	s.handle("PUT /api/prefs", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p := loadPrefs(s.DB) // partial updates: absent fields keep their value; periods merge per chart
		if err := decode(r, &p); err != nil {
			return nil, err
		}
		if err := validPrefs(p); err != nil {
			return nil, bad(err.Error())
		}
		return p, savePrefs(s.DB, p)
	})
	s.handle("GET /api/plan/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		st := plan.LoadSettings(s.DB)
		return map[string]any{"settings": st, "net_from_gross": plan.LTNetSalary(st.GrossSalary, st.MonthlyDeductions)}, nil
	})
	s.handle("PUT /api/plan/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var st plan.Settings
		if err := decode(r, &st); err != nil {
			return nil, err
		}
		return st, plan.SaveSettings(s.DB, st)
	})
	s.handle("GET /api/budgets", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return plan.List(s.DB, r.URL.Query().Get("archived") == "1")
	})
	saveBudget := func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			plan.Budget
			FromMonth string `json:"from_month"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		b := in.Budget
		if r.PathValue("id") != "" {
			b.ID, _ = idParam(r, "id")
		}
		return b, plan.Save(s.DB, &b, in.FromMonth)
	}
	s.handle("POST /api/budgets", saveBudget)
	s.handle("PUT /api/budgets/{id}", saveBudget)
	s.handle("DELETE /api/budgets/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, plan.Delete(s.DB, id)
	})
	s.handle("GET /api/trips", func(w http.ResponseWriter, r *http.Request) (any, error) {
		txs, err := ledger.All(s.DB, ledger.Filter{})
		if err != nil {
			return nil, err
		}
		budgets, _ := plan.List(s.DB, false)
		trips, sugg := plan.Trips(txs, budgets)
		if trips == nil {
			trips = []plan.Trip{}
		}
		if sugg == nil {
			sugg = []plan.TripSuggestion{}
		}
		return map[string]any{"trips": trips, "suggestions": sugg}, nil
	})
	s.handle("POST /api/trips/tag", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Name string  `json:"name"`
			IDs  []int64 `json:"ids"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		name := strings.Trim(ledger.SlugID(in.Name), "_")
		name = strings.ReplaceAll(name, "_", "-")
		if name == "" {
			return nil, bad("name the trip")
		}
		tag := "trip:" + name
		tx, err := s.DB.Begin()
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		for _, id := range in.IDs {
			t, err := ledger.Get(tx, id)
			if err != nil {
				continue
			}
			var keep []string
			for _, x := range t.Tags {
				if !ledger.TripTag(x) {
					keep = append(keep, x)
				}
			}
			t.Tags = append(keep, tag)
			ledger.Update(tx, &t)
		}
		return map[string]string{"tag": tag}, tx.Commit()
	})
}

// ── insights ────────────────────────────────────────────────────────

func window(r *http.Request) (string, string) {
	q := r.URL.Query()
	if q.Get("from") != "" && q.Get("to") != "" {
		return q.Get("from"), q.Get("to")
	}
	return insights.Window(q.Get("preset"), time.Now())
}

func (s *Server) insightRoutes() {
	s.handle("GET /api/overview", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return cfo.BuildOverview(s.DB, time.Now(), s.Bank.OpenCount())
	})
	s.handle("GET /api/insights/cashflow", func(w http.ResponseWriter, r *http.Request) (any, error) {
		q := r.URL.Query()
		txs, err := ledger.All(s.DB, ledger.Filter{From: q.Get("from"), To: q.Get("to")})
		if err != nil {
			return nil, err
		}
		cats, _ := ledger.CategoryMap(s.DB)
		g := q.Get("granularity")
		if g != "year" {
			g = "month"
		}
		return insights.CashFlow(txs, cats, g), nil
	})
	s.handle("GET /api/insights/breakdown", func(w http.ResponseWriter, r *http.Request) (any, error) {
		from, to := window(r)
		txs, err := ledger.All(s.DB, ledger.Filter{})
		if err != nil {
			return nil, err
		}
		return map[string]any{"from": from, "to": to, "categories": insights.Breakdown(txs, from, to),
			"merchants": insights.MerchantTotals(txs, from, to, 25), "largest": insights.TopExpenses(txs, from, to, 15),
			"tags": insights.TagTotals(txs, from, to)}, nil
	})
	s.handle("GET /api/insights/trends", func(w http.ResponseWriter, r *http.Request) (any, error) {
		months := qint(r, "months", 24)
		if months < 3 || months > 240 {
			months = 24
		}
		from := time.Now().AddDate(0, -months+1, 0).Format("2006-01") + "-01"
		txs, err := ledger.All(s.DB, ledger.Filter{From: from, Kind: "expense"})
		if err != nil {
			return nil, err
		}
		level := r.URL.Query().Get("level")
		parent := r.URL.Query().Get("parent")
		m := map[string]map[string]money.Cents{}
		for _, t := range txs {
			key := ledger.Top(t.Category)
			if parent != "" {
				if key != parent {
					continue
				}
				key = t.Category
			} else if level == "leaf" {
				key = t.Category
			}
			if m[t.Date[:7]] == nil {
				m[t.Date[:7]] = map[string]money.Cents{}
			}
			m[t.Date[:7]][key] += t.Amount
		}
		var rows []map[string]any
		for i := months - 1; i >= 0; i-- {
			mo := time.Now().AddDate(0, -i, 0).Format("2006-01")
			row := map[string]any{"month": mo}
			for k, v := range m[mo] {
				row[k] = v
			}
			rows = append(rows, row)
		}
		return rows, nil
	})
	s.handle("GET /api/insights/month", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return cfo.BuildMonthReview(s.DB, r.URL.Query().Get("month"), time.Now())
	})
	s.handle("GET /api/checks", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return cfo.Checks(s.DB, time.Now()), nil
	})
	s.handle("GET /api/insights/pace", func(w http.ResponseWriter, r *http.Request) (any, error) {
		txs, err := ledger.All(s.DB, ledger.Filter{From: time.Now().AddDate(0, -7, 0).Format("2006-01") + "-01"})
		if err != nil {
			return nil, err
		}
		return insights.Pace(txs, time.Now()), nil
	})
	s.handle("GET /api/insights/recurring", func(w http.ResponseWriter, r *http.Request) (any, error) {
		txs, err := ledger.All(s.DB, ledger.Filter{From: time.Now().AddDate(-2, 0, 0).Format("2006-01-02")})
		if err != nil {
			return nil, err
		}
		rec, hidden, err := insights.RecurringCosts(s.DB, txs, time.Now())
		if err != nil {
			return nil, err
		}
		var total money.Cents
		for _, x := range rec {
			total += x.Monthly
		}
		if hidden == nil {
			hidden = []insights.Recurring{}
		}
		return map[string]any{"items": rec, "hidden": hidden, "monthly_total": total}, nil
	})
	saveRecurring := func(w http.ResponseWriter, r *http.Request) (any, error) {
		var it insights.RecurringItem
		if err := decode(r, &it); err != nil {
			return nil, err
		}
		if r.Method == http.MethodPut {
			id, err := idParam(r, "id")
			if err != nil {
				return nil, err
			}
			it.ID = id
		}
		if err := insights.SaveRecurringItem(s.DB, &it); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			return nil, bad(err.Error())
		}
		return it, nil
	}
	s.handle("POST /api/recurring", saveRecurring)
	s.handle("PUT /api/recurring/{id}", saveRecurring)
	s.handle("DELETE /api/recurring/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, insights.DeleteRecurringItem(s.DB, id)
	})
	s.handle("GET /api/insights/fi", func(w http.ResponseWriter, r *http.Request) (any, error) {
		txs, err := ledger.All(s.DB, ledger.Filter{From: time.Now().AddDate(-2, 0, 0).Format("2006-01-02")})
		if err != nil {
			return nil, err
		}
		cats, _ := ledger.CategoryMap(s.DB)
		book, err := wealth.LoadBook(s.DB)
		if err != nil {
			return nil, err
		}
		return insights.ComputeFI(txs, cats, book, plan.LoadSettings(s.DB), time.Now()), nil
	})
	s.handle("GET /api/insights/review", func(w http.ResponseWriter, r *http.Request) (any, error) {
		year := r.URL.Query().Get("year")
		if len(year) != 4 {
			year = time.Now().Format("2006")
		}
		return s.yearReview(year)
	})
}

// ── bank ────────────────────────────────────────────────────────────

func (s *Server) bankRoutes() {
	s.handle("GET /api/bank/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		st := bank.LoadSettings(s.DB)
		return map[string]any{"application_id": st.ApplicationID, "has_key": st.PrivateKeyPEM != "", "environment": st.Environment,
			"redirect_url": st.RedirectURL, "owner_names": st.OwnerNames, "consent_days": st.ConsentDays, "configured": st.Configured(),
			"keep_reserved_in_inbox": st.KeepReservedInInbox}, nil
	})
	s.handle("PUT /api/bank/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			ApplicationID *string   `json:"application_id"`
			PrivateKeyPEM string    `json:"private_key_pem"`
			Environment   *string   `json:"environment"`
			RedirectURL   *string   `json:"redirect_url"`
			OwnerNames    *[]string `json:"owner_names"`
			ConsentDays   *int      `json:"consent_days"`
			KeepReserved  *bool     `json:"keep_reserved_in_inbox"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		st := bank.LoadSettings(s.DB)
		if in.KeepReserved != nil {
			st.KeepReservedInInbox = *in.KeepReserved
		}
		if in.ApplicationID != nil {
			st.ApplicationID = strings.TrimSpace(*in.ApplicationID)
		}
		if in.Environment != nil {
			st.Environment = *in.Environment
		}
		if in.RedirectURL != nil {
			st.RedirectURL = strings.TrimSpace(*in.RedirectURL)
		}
		if in.OwnerNames != nil {
			st.OwnerNames = *in.OwnerNames
		}
		if in.ConsentDays != nil && *in.ConsentDays > 0 {
			st.ConsentDays = *in.ConsentDays
		}
		if k := strings.TrimSpace(in.PrivateKeyPEM); k != "" {
			st.PrivateKeyPEM = k
		}
		return map[string]bool{"ok": true}, bank.SaveSettings(s.DB, st)
	})
	s.handle("GET /api/bank/banks", func(w http.ResponseWriter, r *http.Request) (any, error) {
		c := strings.ToUpper(r.URL.Query().Get("country"))
		if c == "" {
			c = "LT"
		}
		return s.Bank.Banks(r.Context(), c)
	})
	s.handle("GET /api/bank/connections", func(w http.ResponseWriter, r *http.Request) (any, error) {
		c, err := s.Bank.Connections()
		if c == nil {
			c = []bank.Connection{}
		}
		return c, err
	})
	s.handle("POST /api/bank/connect", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Bank    string `json:"bank"`
			Country string `json:"country"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		id, u, err := s.Bank.Connect(r.Context(), in.Bank, in.Country, psu(r))
		return map[string]any{"connection_id": id, "url": u}, err
	})
	s.handle("POST /api/bank/callback", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct{ Code, State, URL string }
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		code, state := in.Code, in.State
		if in.URL != "" {
			var err error
			if code, state, err = bank.ParseCallbackURL(in.URL); err != nil {
				return nil, err
			}
		}
		id, err := s.Bank.Callback(r.Context(), strings.TrimSpace(code), strings.TrimSpace(state))
		return map[string]any{"connection_id": id}, err
	})
	s.handle("DELETE /api/bank/connections/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, s.Bank.Disconnect(r.Context(), id)
	})
	s.handle("PUT /api/bank/accounts/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		var in struct {
			AccountID string `json:"account_id"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, s.Bank.MapAccount(id, in.AccountID)
	})
	s.handle("POST /api/bank/sync", func(w http.ResponseWriter, r *http.Request) (any, error) {
		days := qint(r, "days", 0)
		if days < 0 || days > 730 {
			days = 0
		}
		return s.Bank.SyncAll(r.Context(), psu(r), days, int64(qint(r, "account", 0)))
	})
	s.handle("GET /api/bank/inbox", func(w http.ResponseWriter, r *http.Request) (any, error) {
		rows, err := s.Bank.Inbox(r.URL.Query().Get("state"))
		if rows == nil {
			rows = []bank.InboxRow{}
		}
		for i := range rows {
			rows[i].Raw = ""
		}
		return rows, err
	})
	s.handle("GET /api/bank/inbox/{id}/raw", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		raw, err := s.Bank.Raw(id)
		return map[string]string{"raw": raw}, err
	})
	s.handle("PUT /api/bank/inbox/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		var e bank.Edit
		if err := decode(r, &e); err != nil {
			return nil, err
		}
		return s.Bank.Update(id, e)
	})
	s.handle("POST /api/bank/inbox/commit", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			IDs []int64 `json:"ids"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return s.Bank.Commit(in.IDs)
	})
	for _, st := range []string{"dismiss", "restore"} {
		to := "dismissed"
		if st == "restore" {
			to = "open"
		}
		s.handle("POST /api/bank/inbox/"+st, func(w http.ResponseWriter, r *http.Request) (any, error) {
			var in struct {
				IDs []int64 `json:"ids"`
			}
			if err := decode(r, &in); err != nil {
				return nil, err
			}
			n, err := s.Bank.SetState(in.IDs, to)
			return map[string]int{"changed": n}, err
		})
	}
	s.handle("POST /api/bank/inbox/{id}/link", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		var in struct {
			TxID int64 `json:"tx_id"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, s.Bank.Link(id, in.TxID)
	})
	s.handle("POST /api/bank/inbox/{id}/unlink", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, s.Bank.Unlink(id)
	})
}

// ── AI ──────────────────────────────────────────────────────────────

func (s *Server) aiRoutes() {
	s.handle("GET /api/ai/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		st := ai.LoadSettings(s.DB)
		masked := ""
		if n := len(st.APIKey); n > 8 {
			masked = st.APIKey[:6] + "…" + st.APIKey[n-4:]
		}
		return map[string]any{"gateway_url": st.GatewayURL, "model": st.Model, "provider": st.ResolvedProvider(), "enabled": st.Enabled,
			"has_key": st.APIKey != "", "key_hint": masked, "spend": s.AI.Client.Spend()}, nil
	})
	s.handle("PUT /api/ai/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			GatewayURL *string `json:"gateway_url"`
			APIKey     string  `json:"api_key"`
			Model      *string `json:"model"`
			Provider   *string `json:"provider"`
			Enabled    *bool   `json:"enabled"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		st := ai.LoadSettings(s.DB)
		if in.GatewayURL != nil {
			st.GatewayURL = strings.TrimSpace(*in.GatewayURL)
		}
		if k := strings.TrimSpace(in.APIKey); k != "" {
			st.APIKey = k
		}
		if in.Model != nil {
			m := strings.TrimSpace(*in.Model)
			if len(m) > 100 || strings.ContainsAny(m, " \t\r\n\"'<>") {
				return nil, bad("invalid model name")
			}
			st.Model = m
		}
		if in.Provider != nil {
			st.Provider = *in.Provider
		}
		if in.Enabled != nil {
			st.Enabled = *in.Enabled
		}
		return map[string]bool{"ok": true}, ai.SaveSettings(s.DB, st)
	})
	s.handle("POST /api/ai/test", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]bool{"ok": true}, s.AI.Client.Test(r.Context(), ai.LoadSettings(s.DB))
	})
	s.handle("GET /api/ai/models", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.AI.Client.Models(r.Context(), ai.LoadSettings(s.DB))
	})
	s.handle("GET /api/ai/context", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]string{"content": s.AI.Context()}, nil
	})
	s.handle("PUT /api/ai/context", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct{ Content string }
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, s.AI.SaveContext(in.Content)
	})
	s.handle("GET /api/ai/notes", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]string{"content": s.AI.Notes()}, nil
	})
	s.handle("PUT /api/ai/notes", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct{ Content string }
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, s.AI.SaveNotes(in.Content)
	})
	s.handle("GET /api/ai/chat", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.AI.History(200) })
	s.handle("DELETE /api/ai/chat", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]bool{"ok": true}, s.AI.ClearHistory()
	})
	s.handle("POST /api/ai/chat", func(w http.ResponseWriter, r *http.Request) (any, error) {
		text, img, err := readMessage(r)
		if err != nil {
			return nil, err
		}
		return s.AI.Chat(r.Context(), text, img)
	})
	s.handle("POST /api/ai/scan", func(w http.ResponseWriter, r *http.Request) (any, error) {
		_, img, err := readMessage(r)
		if err != nil {
			return nil, err
		}
		if img == nil {
			return nil, bad("attach an image")
		}
		return s.AI.ScanReceipt(r.Context(), *img)
	})
	s.handle("POST /api/ai/assist", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct{ Text string }
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return s.AI.Assist(r.Context(), in.Text)
	})
	s.handle("GET /api/tidy", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.tidyQueue()
	})
	s.handle("POST /api/ai/tidy", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			IDs []int64 `json:"ids"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		var txs []ledger.Tx
		for _, id := range in.IDs {
			if t, err := ledger.Get(s.DB, id); err == nil {
				txs = append(txs, t)
			}
		}
		return s.AI.Tidy(r.Context(), txs)
	})
	s.handle("GET /api/ai/topups", func(w http.ResponseWriter, r *http.Request) (any, error) {
		rows, err := s.DB.Query(`SELECT id, amount_usd, note, occurred_on FROM ai_topups ORDER BY occurred_on DESC, id DESC`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id int64
			var amt float64
			var note, on string
			rows.Scan(&id, &amt, &note, &on)
			out = append(out, map[string]any{"id": id, "amount_usd": amt, "note": note, "occurred_on": on})
		}
		return out, nil
	})
	s.handle("DELETE /api/ai/topups/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		_, err = s.DB.Exec(`DELETE FROM ai_topups WHERE id=?`, id)
		return map[string]bool{"ok": true}, err
	})
	s.handle("POST /api/ai/topups", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			AmountUSD float64 `json:"amount_usd"`
			Note      string  `json:"note"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		_, err := s.DB.Exec(`INSERT INTO ai_topups(amount_usd,note,occurred_on,created_at) VALUES(?,?,?,?)`, in.AmountUSD, in.Note, today(), time.Now().UTC().Format(time.RFC3339))
		return map[string]bool{"ok": true}, err
	})
}

// readMessage accepts multipart (text + image file) or JSON {text}.
func readMessage(r *http.Request) (string, *ai.Image, error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(12 << 20); err != nil {
			return "", nil, bad("image too large (max 10 MB)")
		}
		text := r.FormValue("text")
		f, hdr, err := r.FormFile("image")
		if err != nil {
			return text, nil, nil
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, 10<<20))
		if err != nil {
			return "", nil, err
		}
		mt := hdr.Header.Get("Content-Type")
		if mt == "" || mt == "application/octet-stream" {
			mt = http.DetectContentType(data)
		}
		return text, &ai.Image{MediaType: mt, Data: data}, nil
	}
	var in struct{ Text string }
	if err := decode(r, &in); err != nil {
		return "", nil, err
	}
	return in.Text, nil, nil
}
