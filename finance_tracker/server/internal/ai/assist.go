package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ft/internal/ledger"
	"ft/internal/money"
)

func (a *Assistant) vocabulary() (cats []string, accts []string, tags []string) {
	list, _ := ledger.ListCategories(a.DB)
	for _, c := range list {
		if !c.Archived {
			cats = append(cats, c.ID+" ("+c.Name+", "+c.Kind+")")
		}
	}
	al, _ := ledger.ListAccounts(a.DB)
	for _, ac := range al {
		if !ac.Archived && (ac.Kind == "checking" || ac.Kind == "savings" || ac.Kind == "cash") {
			accts = append(accts, ac.ID+" ("+ac.Name+")")
		}
	}
	tl, _ := ledger.ListTags(a.DB)
	for _, t := range tl {
		if t.Count >= 5 {
			tags = append(tags, t.Tag)
		}
	}
	return
}

// jsonObject extracts the JSON value from a reply that may wrap it in prose:
// whichever of an object or an array opens first.
func jsonObject(s string) string {
	o, a := strings.Index(s, "{"), strings.Index(s, "[")
	if a >= 0 && (o < 0 || a < o) {
		if j := strings.LastIndex(s, "]"); j > a {
			return s[a : j+1]
		}
	}
	if o >= 0 {
		if j := strings.LastIndex(s, "}"); j > o {
			return s[o : j+1]
		}
	}
	return s
}

// Assist turns a sentence ("coffee 4.50 cash yesterday") into form fields.
func (a *Assistant) Assist(ctx context.Context, text string) (*Scan, error) {
	s := LoadSettings(a.DB)
	if err := s.require(); err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("describe the transaction")
	}
	cats, accts, tags := a.vocabulary()
	prompt := fmt.Sprintf(`Today is %s. Turn this description of ONE transaction into JSON and reply with the JSON only:
{"kind":"expense|income|transfer","date":"YYYY-MM-DD","amount":<positive EUR>,"category":"<id from the list>","merchant":"<who was paid, or empty>","note":"<short note>","account_id":"<id or empty>","tags":["<0-2 from the list>"],"remark":"<one sentence on anything you were unsure about>"}

Categories: %s
Accounts: %s
Tags: %s

Description: %q`, time.Now().Format("2006-01-02 (Monday)"), strings.Join(cats, "; "), strings.Join(accts, ", "), strings.Join(tags, ", "), text)
	rep, err := a.Client.call(ctx, s, request{Model: s.Model, MaxTokens: 2048, Messages: []message{{Role: "user", Content: []block{{Type: "text", Text: prompt}}}}}, "assist")
	if err != nil {
		return nil, err
	}
	var raw struct {
		Kind, Date, Category, Merchant, Note, Remark string
		AccountID                                    string `json:"account_id"`
		Amount                                       float64
		Tags                                         []string
	}
	if json.Unmarshal([]byte(jsonObject(rep.Text)), &raw) != nil {
		return nil, errors.New("couldn't read that — try e.g. \"lunch 12.50 card today\"")
	}
	return a.sanitize(raw.Kind, raw.Date, raw.Category, raw.Merchant, raw.Note, raw.AccountID, raw.Remark, raw.Amount, raw.Tags), nil
}

// sanitize keeps only values the app can use.
func (a *Assistant) sanitize(kind, date, cat, merch, note, acct, remark string, amount float64, tags []string) *Scan {
	out := &Scan{Kind: "expense", Amount: money.FromFloat(amount).Abs(), Merchant: strings.TrimSpace(merch), Note: strings.TrimSpace(note), Remark: remark, Tags: []string{}}
	if kind == "income" || kind == "transfer" {
		out.Kind = kind
	}
	if _, err := time.Parse("2006-01-02", date); err == nil {
		out.Date = date
	}
	if cm, _ := ledger.CategoryMap(a.DB); cm != nil {
		if c, ok := cm[cat]; ok && c.Kind == out.Kind {
			out.Category = cat
		}
	}
	if al, _ := ledger.ListAccounts(a.DB); al != nil {
		for _, ac := range al {
			if ac.ID == acct && !ac.Archived && (ac.Kind == "checking" || ac.Kind == "savings" || ac.Kind == "cash") {
				out.AccountID = ac.ID
			}
		}
	}
	known := map[string]bool{}
	tl, _ := ledger.ListTags(a.DB)
	for _, t := range tl {
		known[t.Tag] = true
	}
	for _, t := range tags {
		if t = strings.ToLower(t); known[t] {
			out.Tags = append(out.Tags, t)
		}
	}
	return out
}

// TidyProposal is a sharper category (and merchant) for one transaction.
type TidyProposal struct {
	ID       int64  `json:"id"`
	Category string `json:"category"`
	Merchant string `json:"merchant,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// Tidy asks the model to place transactions that only carry a broad
// category. Proposals only — the owner applies them.
func (a *Assistant) Tidy(ctx context.Context, txs []ledger.Tx) ([]TidyProposal, error) {
	s := LoadSettings(a.DB)
	if err := s.require(); err != nil {
		return nil, err
	}
	if len(txs) == 0 {
		return []TidyProposal{}, nil
	}
	if len(txs) > 80 {
		txs = txs[:80]
	}
	cats, _, _ := a.vocabulary()
	var lines []string
	for _, t := range txs {
		lines = append(lines, fmt.Sprintf(`{"id":%d,"date":"%s","amount":%s,"category":"%s","merchant":%q,"note":%q}`, t.ID, t.Date, t.Amount, t.Category, t.Merchant, t.Note))
	}
	prompt := fmt.Sprintf(`These expenses only have a broad category. For each, pick the most specific fitting category id from the list (keep the same kind), and a clean merchant name if the current one is empty or messy. Reply with a JSON array only: [{"id":..,"category":"..","merchant":"..","reason":"<few words>"}]. Skip any you cannot place confidently.

Categories: %s

Transactions:
%s`, strings.Join(cats, "; "), strings.Join(lines, "\n"))
	rep, err := a.Client.call(ctx, s, request{Model: s.Model, MaxTokens: 8000, Messages: []message{{Role: "user", Content: []block{{Type: "text", Text: prompt}}}}}, "tidy")
	if err != nil {
		return nil, err
	}
	var raw []TidyProposal
	if json.Unmarshal([]byte(jsonObject(rep.Text)), &raw) != nil {
		return nil, errors.New("the model's answer could not be read — try again")
	}
	cm, _ := ledger.CategoryMap(a.DB)
	byID := map[int64]ledger.Tx{}
	for _, t := range txs {
		byID[t.ID] = t
	}
	out := []TidyProposal{}
	for _, p := range raw {
		t, ok := byID[p.ID]
		c, okc := cm[p.Category]
		if !ok || !okc || c.Kind != t.Kind || p.Category == t.Category {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}
