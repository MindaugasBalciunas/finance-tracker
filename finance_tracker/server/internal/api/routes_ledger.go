package api

import (
	"database/sql"
	"net/http"
	"slices"
	"strings"
	"time"

	"ft/internal/entry"
	"ft/internal/insights"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

// ── ledger ──────────────────────────────────────────────────────────

func filterFrom(r *http.Request) ledger.Filter {
	q := r.URL.Query()
	return ledger.Filter{From: q.Get("from"), To: q.Get("to"), Kind: q.Get("kind"), Categories: qlist(r, "category"), Accounts: qlist(r, "account"),
		Tags: qlist(r, "tag"), TagsAll: q.Get("tags_all") == "1", Merchant: q.Get("merchant"), Query: q.Get("q"), Source: q.Get("source"),
		Min: parseCents(q.Get("min")), Max: parseCents(q.Get("max")), Sort: q.Get("sort"), Limit: qint(r, "limit", 100), Offset: qint(r, "offset", 0),
		Choices: q.Get("choices") == "1"}
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
		f := filterFrom(r)
		// salary=month: a salary counts in the month it pays for, as in cash flow.
		if r.URL.Query().Get("salary") == "month" {
			for _, w := range plan.LoadSettings(s.DB).Salary() {
				f.Salary = append(f.Salary, ledger.SalaryWindow{From: w.From, By: w.PaidByDay})
			}
		}
		return ledger.List(s.DB, f)
	})
	// What a new expense most likely is: one tap and an amount.
	s.handle("GET /api/transactions/quick", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return ledger.Quick(s.DB, time.Now())
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
		t.ID, t.ExternalID, t.SplitOf, t.Pending = 0, "", 0, false
		// Bank and import rows only come from sync and conversion; a client
		// claiming either would skip the balance move a hand entry makes.
		if t.Source == "" || t.Source == "bank" || t.Source == "import" {
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
		var moved []string
		err := entry.Do(s.DB, func(tx *sql.Tx) (err error) {
			moved, err = entry.Create(tx, &t)
			return err
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"transaction": t, "balances_moved": moved}, nil
	})
	s.handle("PUT /api/transactions/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		var in ledger.Tx
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		// The stored row is read inside the transaction, so a sync settling
		// it meanwhile can't have its balance reversed from stale values.
		err = entry.Do(s.DB, func(tx *sql.Tx) error {
			cur, err := ledger.Get(tx, id)
			if err != nil {
				return entry.ErrNotFound
			}
			return entry.Update(tx, cur, &in)
		})
		if err != nil {
			return nil, err
		}
		return in, nil
	})
	s.handle("DELETE /api/transactions/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		if err := entry.Do(s.DB, func(tx *sql.Tx) error { return entry.Delete(tx, id) }); err != nil {
			return nil, err
		}
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
		n := 0
		err := entry.Do(s.DB, func(tx *sql.Tx) error {
			for _, id := range in.IDs {
				cur, err := ledger.Get(tx, id)
				if err != nil {
					continue
				}
				if in.Delete {
					if err := entry.Delete(tx, id); err != nil {
						return err
					}
					n++
					continue
				}
				t := cur
				t.Tags = slices.Clone(cur.Tags)
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
					t.Tags = slices.DeleteFunc(t.Tags, func(x string) bool { return rm[x] })
				}
				if err := entry.Update(tx, cur, &t); err != nil {
					return err
				}
				n++
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return map[string]int{"changed": n}, nil
	})
	// One-tap tags for the form: the trip by date, the merchant's usual tags, the most used.
	s.handle("GET /api/tags/suggest", func(w http.ResponseWriter, r *http.Request) (any, error) {
		q := r.URL.Query()
		return ledger.SuggestTags(s.DB, q.Get("merchant"), q.Get("date"), q.Get("category"), q.Get("kind"), time.Now())
	})
	// One-tap categories for the form: the merchant's usual ones, then the most used lately.
	s.handle("GET /api/categories/suggest", func(w http.ResponseWriter, r *http.Request) (any, error) {
		q := r.URL.Query()
		return ledger.SuggestCategories(s.DB, q.Get("merchant"), q.Get("kind"))
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
		return ledger.Owed(s.DB)
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
		prev, _ := ledger.GetAccount(s.DB, a.ID)
		saved, err := ledger.SaveAccount(s.DB, a)
		// Hiding an account closes it: from today it holds €0, so net worth
		// matches the lists it leaves; its history stays. Showing it again
		// takes that closing value back out.
		if err == nil && a.Archived != prev.Archived && prev.ID != "" {
			if a.Archived {
				err = wealth.CloseAccount(s.DB, a.ID, time.Now())
			} else {
				err = wealth.ReopenAccount(s.DB, a.ID)
			}
		}
		if err == nil && a.Kind == "loan" {
			// Start date / original principal re-draw the reconstructed history.
			if _, rerr := wealth.RebuildLoanHistory(s.DB, a.ID); rerr != nil {
				return nil, rerr
			}
		}
		return saved, err
	}
	s.handle("POST /api/accounts", saveAccount)
	s.handle("PUT /api/accounts/{id}", saveAccount)
	// A hidden account that still holds a value (hidden before hiding closed
	// accounts) is closed at €0 from today on request.
	s.handle("POST /api/accounts/{id}/close", func(w http.ResponseWriter, r *http.Request) (any, error) {
		a, err := ledger.GetAccount(s.DB, r.PathValue("id"))
		if err != nil {
			return nil, notFound("account not found")
		}
		if !a.Archived {
			return nil, bad("hide the account first")
		}
		return map[string]bool{"ok": true}, wealth.CloseAccount(s.DB, a.ID, time.Now())
	})
	s.handle("DELETE /api/accounts/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]bool{"ok": true}, ledger.DeleteAccount(s.DB, r.PathValue("id"))
	})

	s.handle("GET /api/tags", func(w http.ResponseWriter, r *http.Request) (any, error) { return ledger.ListTags(s.DB) })
	// Housekeeping views: what each tag, rule and category amounts to.
	ledgerAndRules := func() ([]ledger.Tx, []ledger.Rule, error) {
		txs, err := ledger.All(s.DB, ledger.Filter{})
		if err != nil {
			return nil, nil, err
		}
		rules, err := ledger.ListRules(s.DB)
		return txs, rules, err
	}
	s.handle("GET /api/tags/stats", func(w http.ResponseWriter, r *http.Request) (any, error) {
		txs, rules, err := ledgerAndRules()
		if err != nil {
			return nil, err
		}
		return insights.TagStats(txs, rules, time.Now()), nil
	})
	s.handle("GET /api/rules/stats", func(w http.ResponseWriter, r *http.Request) (any, error) {
		txs, rules, err := ledgerAndRules()
		if err != nil {
			return nil, err
		}
		return insights.RuleStats(txs, rules, time.Now()), nil
	})
	s.handle("GET /api/accounts/usage", func(w http.ResponseWriter, r *http.Request) (any, error) {
		txs, err := ledger.All(s.DB, ledger.Filter{})
		if err != nil {
			return nil, err
		}
		return insights.AccountUsage(txs), nil
	})
	s.handle("GET /api/categories/usage", func(w http.ResponseWriter, r *http.Request) (any, error) {
		txs, rules, err := ledgerAndRules()
		if err != nil {
			return nil, err
		}
		budgets, err := plan.List(s.DB, false)
		if err != nil {
			return nil, err
		}
		covered := map[string]bool{}
		for _, b := range budgets {
			for _, c := range b.Categories {
				covered[c] = true
			}
		}
		return insights.CategoryUsage(txs, rules, covered, time.Now()), nil
	})
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

	s.handle("GET /api/notes/suggest", func(w http.ResponseWriter, r *http.Request) (any, error) {
		q := r.URL.Query()
		return ledger.SuggestNotes(s.DB, q.Get("merchant"), q.Get("q"), 6)
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
