package ai

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ft/internal/bank"
	"ft/internal/cfo"
	"ft/internal/db"
	"ft/internal/ibkr"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
)

const (
	maxToolRounds  = 12
	historyTurns   = 20
	historyWindow  = 7 * 24 * time.Hour
	maxReplayChars = 6000
	chatBudget     = 240 * time.Second
)

type Assistant struct {
	DB     *sql.DB
	Client *Client
	Bank   *bank.Service
	IBKR   *ibkr.Service // read-only Interactive Brokers, when connected
}

func (a *Assistant) Overview(now time.Time) *cfo.Overview {
	n := 0
	if a.Bank != nil {
		n = a.Bank.OpenCount()
	}
	o, _ := cfo.BuildOverview(a.DB, now, n)
	if o != nil {
		o.Spark = nil
		o.Recent = nil
	}
	return o
}

func (a *Assistant) BudgetReport(month string, now time.Time) (*plan.Report, error) {
	return cfo.BudgetReport(a.DB, month, now)
}

// Context is the owner's standing brief for the assistant (who they are,
// their rules, how to advise them). Numbers live in the database, not here.
func (a *Assistant) Context() string {
	var raw string
	if a.DB.QueryRow(`SELECT value FROM settings WHERE key='ai_context'`).Scan(&raw) != nil {
		return ""
	}
	var s string
	if json.Unmarshal([]byte(raw), &s) == nil {
		return s
	}
	return raw
}

func (a *Assistant) SaveContext(s string) error {
	raw, _ := json.Marshal(s)
	_, err := a.DB.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('ai_context',?)`, string(raw))
	return err
}

const systemPrompt = `You are the owner's personal CFO inside their finance app. Lithuania, EUR. You have read tools over the live database — the ledger since 2010, every account balance (including the house, car and mortgage), budgets, investments, loans and the bank inbox. Ground every figure in a tool result; never estimate a number a tool can give you. Today is %s.

How the data is modelled:
- Transactions are income, expense or transfer. Transfers move money between own accounts; transfer.invest / transfer.pension / transfer.debt (mortgage principal) / transfer.asset build wealth and count as "invested", transfer.internal is just shuffling cash.
- Categories are two-level ids (food.groceries). Merchant is who was paid. Tags are who/why/where: people (kids, evelina, kristina), properties (house, apartment), trips (trip:…).
- Savings rate = (income − spending) / income; refunds reduce spending. Mortgage interest is spending; principal is saving.
- Net worth includes property and loans; "liquid" is cash, brokers, crypto and II/III pillar pensions (cashable); it excludes property, car and debt.

Trust rules — the owner acts on what you say:
- Never mention a transaction, refund, payment or balance that no tool returned in this conversation. If a search finds nothing, say so plainly; don't guess.
- Point to the evidence: when you name specific transactions or a set of them, link to the ledger view that shows them, e.g. [3 payments at 360 Arena](#/ledger?merchant=360%%20Arena&period=custom&from=2026-09-01&to=2026-09-30) or [search](#/ledger?q=arena). Links use #/ledger with q, merchant, category, tag, from, to (with period=custom).
- Label anything you calculated yourself (an average, a projection) as your calculation.
- Use the app's own figures instead of rebuilding them, so the same question gets the same numbers every time: a month's income, spending and savings come from cash_flow or get_overview (they put each salary leg in the month it belongs to) — never add up search rows for a total. Cash until payday, per account, is get_overview's cash plan: its items, low, end, needed and moves. Quote those.
- The cash plan only knows recurring items the app has (get_recurring). If the owner's brief or notes mention a standing order or payment the plan lacks, say which one, give its effect as your calculation on top of the plan's figure, and offer to add it with add_recurring so the plan includes it from then on.

Answer style: direct, concise, numbers first. Markdown: bullets, **bold** key figures, compact tables; no top-level headings. When a picture helps (or the user asks to chart/plot/show), add a fenced block tagged chart with ONE JSON object: {"type":"line|bar|area|pie","title":"…","x":"<label field>","unit":"€","series":[{"name":"…","key":"<numeric field>"}],"data":[…]} — real figures only, ≤24 points, ≤4 series, at most 2 charts.

You also have web_search for live facts (rates, prices, tax rules, news) and get_market_buzz for a ticker's news and social chatter — treat chatter as unverified.

Images: the user may attach a receipt or a bank-app screenshot. Read it and propose the transaction (date, amount, category id, merchant, account, tags). Create it with create_transaction only after the user confirms.

Memory: you keep a running memory with the remember tool — it is the only thing that carries over between chats besides the data. Save on your own, without asking, whenever the owner tells you something lasting the database can't show: a decision or plan ("pull €2,200 back from savings before 1 Nov"), a life or income change, a correction of how to read their data, an account being closed, a preference about how you advise them. One short sentence per fact (the date is added for you); don't save figures the tools can give, small talk or what is already in the brief or the remembered list. When you save, end the reply with one line: "Noted: …".

Changing data: create_transaction, update_transaction, add_rule, delete_rule, rename_tag, add_recurring and update_inbox_row write to the database. Use them ONLY after the user explicitly approves that specific change in this conversation. Propose first, then act, then report exactly what changed. You can never accept bank inbox rows into the ledger — the owner does that.`

func (a *Assistant) system(now time.Time) []block {
	var parts []block
	parts = append(parts, block{Type: "text", Text: fmt.Sprintf(systemPrompt, now.Format("Monday 2 January 2006"))})
	if n := strings.TrimSpace(a.Notes()); n != "" {
		parts = append(parts, block{Type: "text", Text: "Decisions and preferences you were asked to remember (newest last):\n" + n})
	}
	if c := strings.TrimSpace(a.Context()); c != "" {
		parts = append(parts, block{Type: "text", Text: "The owner's standing brief (who they are, their framework and rules). Never quote numbers from it — pull them live:\n\n" + c})
	}
	// One cache breakpoint covers tools + system, the stable prefix.
	parts[len(parts)-1].CacheControl = &cacheControl{Type: "ephemeral"}
	return parts
}

// Notes are facts the assistant was asked to remember across chats, the MCP
// server and the app — one dated line each, newest last, capped.
func (a *Assistant) Notes() string {
	var raw string
	if a.DB.QueryRow(`SELECT value FROM settings WHERE key='ai_notes'`).Scan(&raw) != nil {
		return ""
	}
	var s string
	json.Unmarshal([]byte(raw), &s)
	return s
}

func (a *Assistant) Remember(note string) error {
	lines := strings.Split(strings.TrimSpace(a.Notes()), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	lines = append(lines, time.Now().Format("2006-01-02")+": "+strings.ReplaceAll(strings.TrimSpace(note), "\n", " "))
	if len(lines) > 60 {
		lines = lines[len(lines)-60:]
	}
	raw, _ := json.Marshal(strings.Join(lines, "\n"))
	_, err := a.DB.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('ai_notes',?)`, string(raw))
	return err
}

func (a *Assistant) SaveNotes(s string) error {
	raw, _ := json.Marshal(s)
	_, err := a.DB.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('ai_notes',?)`, string(raw))
	return err
}

type ChatMessage struct {
	ID        int64  `json:"id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	// Unchecked: euro amounts the answer gave that no data it read contained.
	Unchecked []string `json:"unchecked,omitempty"`
}

func (a *Assistant) History(limit int) ([]ChatMessage, error) {
	rows, err := a.DB.Query(`SELECT id,role,content,created_at,unchecked FROM (SELECT * FROM ai_messages ORDER BY id DESC LIMIT ?) ORDER BY id`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChatMessage{}
	for rows.Next() {
		var m ChatMessage
		var unchecked string
		rows.Scan(&m.ID, &m.Role, &m.Content, &m.CreatedAt, &unchecked)
		if unchecked != "" {
			json.Unmarshal([]byte(unchecked), &m.Unchecked)
		}
		out = append(out, m)
	}
	return out, nil
}

func (a *Assistant) ClearHistory() error {
	_, err := a.DB.Exec(`DELETE FROM ai_messages`)
	return err
}

type Image struct {
	MediaType string
	Data      []byte
}

var visionTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true, "image/gif": true}

type ChatResult struct {
	Reply string   `json:"reply"`
	Tools []string `json:"tools"`
	// Unchecked are euro amounts in the reply that appear in none of the
	// data the assistant read this turn — its own sums, or a mistake.
	Unchecked []string `json:"unchecked,omitempty"`
	Changed   bool     `json:"changed"` // a write tool ran — refresh views
	Usage     Usage    `json:"usage"`
}

// Chat answers one user turn, running tools as the model asks.
func (a *Assistant) Chat(ctx context.Context, text string, img *Image) (*ChatResult, error) {
	s := LoadSettings(a.DB)
	if err := s.require(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, chatBudget)
	defer cancel()
	now := time.Now()
	hist, err := a.History(historyTurns)
	if err != nil {
		return nil, err
	}
	var msgs []message
	cutoff := now.Add(-historyWindow).UTC().Format(time.RFC3339)
	for _, m := range hist {
		if m.CreatedAt < cutoff || strings.TrimSpace(m.Content) == "" || strings.HasPrefix(m.Content, FailedPrefix) {
			continue
		}
		c := m.Content
		if len(c) > maxReplayChars {
			c = c[:maxReplayChars] + "… [trimmed]"
		}
		if n := len(msgs); n > 0 && msgs[n-1].Role == m.Role {
			msgs[n-1].Content = append(msgs[n-1].Content, block{Type: "text", Text: c})
			continue
		}
		msgs = append(msgs, message{Role: m.Role, Content: []block{{Type: "text", Text: c}}})
	}
	if len(msgs) > 0 && msgs[0].Role != "user" {
		msgs = msgs[1:]
	}
	userText := strings.TrimSpace(text)
	turn := message{Role: "user"}
	if img != nil {
		if !visionTypes[img.MediaType] {
			return nil, fmt.Errorf("unsupported image type %q — use JPEG, PNG, WebP or GIF", img.MediaType)
		}
		if userText == "" {
			userText = "Here is an image (receipt or screenshot). Read it and propose the transaction(s)."
		}
		turn.Content = append(turn.Content, block{Type: "image", Source: &imageSource{Type: "base64", MediaType: img.MediaType, Data: base64.StdEncoding.EncodeToString(img.Data)}})
	}
	if userText == "" {
		return nil, errors.New("say something")
	}
	turn.Content = append(turn.Content, block{Type: "text", Text: userText})
	if n := len(msgs); n > 0 && msgs[n-1].Role == "user" {
		msgs[n-1].Content = append(msgs[n-1].Content, turn.Content...)
	} else {
		msgs = append(msgs, turn)
	}

	// The question is in the history straight away, and the answer — or what
	// went wrong — joins it at the end, so nothing asked is ever lost even if
	// the phone dropped the connection meanwhile.
	a.DB.Exec(`INSERT INTO ai_messages(role,content,created_at) VALUES('user',?,?)`, userText, db.Now())
	res, err := a.answer(ctx, s, now, msgs)
	if err != nil {
		a.DB.Exec(`INSERT INTO ai_messages(role,content,created_at) VALUES('assistant',?,?)`, FailedPrefix+err.Error(), db.Now())
		return nil, err
	}
	unchecked := ""
	if len(res.Unchecked) > 0 {
		b, _ := json.Marshal(res.Unchecked)
		unchecked = string(b)
	}
	a.DB.Exec(`INSERT INTO ai_messages(role,content,created_at,unchecked) VALUES('assistant',?,?,?)`, res.Reply, db.Now(), unchecked)
	return res, nil
}

// FailedPrefix marks an assistant history entry that records a failed turn;
// such entries are shown but never replayed to the model.
const FailedPrefix = "⚠️ Couldn't answer: "

// answer runs the model/tool loop for one turn.
func (a *Assistant) answer(ctx context.Context, s Settings, now time.Time, msgs []message) (*ChatResult, error) {
	res := &ChatResult{Tools: []string{}}
	var seen []string // every tool result this turn, for checking the reply's figures
	tools := toolDefs()
	system := a.system(now)
	for round := 0; ; round++ {
		if round >= maxToolRounds {
			return nil, fmt.Errorf("the assistant needed more than %d steps — ask a narrower question", maxToolRounds)
		}
		rep, err := a.Client.call(ctx, s, request{Model: s.Model, MaxTokens: 16000, System: system, Messages: msgs, Tools: tools}, "chat")
		res.Usage.CostUSD += rep.Usage.CostUSD
		res.Usage.InputTokens += rep.Usage.InputTokens
		res.Usage.OutputTokens += rep.Usage.OutputTokens
		res.Usage.Estimated = res.Usage.Estimated || rep.Usage.Estimated
		if err != nil {
			if ctx.Err() != nil {
				return nil, errors.New("this took too long — try a narrower question")
			}
			return nil, err
		}
		if rep.Stop == "pause_turn" {
			msgs = append(msgs, message{Role: "assistant", Content: echo(rep.Raw)})
			continue
		}
		if len(rep.ToolCalls) == 0 {
			if strings.TrimSpace(rep.Text) == "" {
				return nil, errors.New("the assistant returned an empty answer — try again")
			}
			res.Reply = rep.Text
			res.Unchecked = uncheckedAmounts(rep.Text, seen)
			break
		}
		msgs = append(msgs, message{Role: "assistant", Content: echo(rep.Raw)})
		var results []block
		for _, tc := range rep.ToolCalls {
			res.Tools = append(res.Tools, tc.Name)
			if writeTools[tc.Name] {
				res.Changed = true
			}
			out, terr := a.runTool(tc.Name, tc.Input)
			seen = append(seen, out)
			b := block{Type: "tool_result", ToolUseID: tc.ID, Content: out}
			if terr != nil {
				b.Content, b.IsError = "error: "+terr.Error(), true
			}
			results = append(results, b)
		}
		msgs = append(msgs, message{Role: "user", Content: results})
	}
	return res, nil
}

// echo turns a response's content back into request blocks (text and
// tool_use only; server-tool blocks are re-sent as their text summary).
func echo(raw []respBlock) []block {
	var out []block
	for _, b := range raw {
		switch b.Type {
		case "text":
			if b.Text != "" {
				out = append(out, block{Type: "text", Text: b.Text})
			}
		case "tool_use":
			in := b.Input
			if len(in) == 0 {
				in = json.RawMessage("{}")
			}
			out = append(out, block{Type: "tool_use", ID: b.ID, Name: b.Name, Input: in})
		}
	}
	if len(out) == 0 {
		out = append(out, block{Type: "text", Text: "…"})
	}
	return out
}

// Scan is a transaction read from a photo, for the form to prefill.
type Scan struct {
	Date      string      `json:"date"`
	Amount    money.Cents `json:"amount"`
	Kind      string      `json:"kind"`
	Category  string      `json:"category"`
	Merchant  string      `json:"merchant"`
	Note      string      `json:"note"`
	AccountID string      `json:"account_id"`
	Tags      []string    `json:"tags"`
	Remark    string      `json:"remark"`
	Currency  string      `json:"currency"`
}

func (a *Assistant) ScanReceipt(ctx context.Context, img Image) (*Scan, error) {
	s := LoadSettings(a.DB)
	if err := s.require(); err != nil {
		return nil, err
	}
	if !visionTypes[img.MediaType] {
		return nil, fmt.Errorf("unsupported image type %q — use JPEG, PNG, WebP or GIF", img.MediaType)
	}
	cats, _ := ledger.ListCategories(a.DB)
	var catList []string
	for _, c := range cats {
		if !c.Archived && c.Kind != "transfer" {
			catList = append(catList, c.ID+" ("+c.Name+")")
		}
	}
	accts, _ := ledger.ListAccounts(a.DB)
	var acctList []string
	for _, ac := range accts {
		if !ac.Archived && (ac.Group == "cash") {
			acctList = append(acctList, ac.ID+" ("+ac.Name+")")
		}
	}
	tags, _ := ledger.ListTags(a.DB)
	var tagList []string
	for _, t := range tags {
		if t.Count >= 5 && !ledger.TripTag(t.Tag) {
			tagList = append(tagList, t.Tag)
		}
	}
	prompt := fmt.Sprintf(`Extract ONE transaction from this image (receipt, invoice, or a bank/payment app screenshot). Reply with ONE JSON object and nothing else:
{"kind":"expense|income","date":"YYYY-MM-DD or empty","amount":<total charged, positive>,"currency":"EUR or what the image shows","category":"<one id from the list or empty>","merchant":"<shop / payee>","note":"<short description of what was bought>","account_id":"<id or empty>","tags":["<0-2 from the list>"],"remark":"<one sentence: what you read and any doubt>"}

Categories: %s
Accounts (leave empty for paper receipts; a black/orange "PzM sąskaita" app screenshot is swed): %s
Tags: %s
Never invent a merchant or amount — say so in remark when unreadable.`, strings.Join(catList, ", "), strings.Join(acctList, ", "), strings.Join(tagList, ", "))
	rep, err := a.Client.call(ctx, s, request{Model: s.Model, MaxTokens: 4096, Messages: []message{{Role: "user", Content: []block{
		{Type: "image", Source: &imageSource{Type: "base64", MediaType: img.MediaType, Data: base64.StdEncoding.EncodeToString(img.Data)}},
		{Type: "text", Text: prompt}}}}}, "scan")
	if err != nil {
		return nil, err
	}
	txt := rep.Text
	if i, j := strings.Index(txt, "{"), strings.LastIndex(txt, "}"); i >= 0 && j > i {
		txt = txt[i : j+1]
	}
	var raw struct {
		Kind, Date, Currency, Category, Merchant, Note, Remark string
		AccountID                                              string `json:"account_id"`
		Amount                                                 float64
		Tags                                                   []string
	}
	if json.Unmarshal([]byte(txt), &raw) != nil {
		return nil, errors.New("could not read a transaction from that image — try a clearer photo")
	}
	out := &Scan{Kind: "expense", Date: raw.Date, Amount: money.FromFloat(raw.Amount).Abs(), Merchant: strings.TrimSpace(raw.Merchant),
		Note: strings.TrimSpace(raw.Note), Remark: raw.Remark, Currency: strings.ToUpper(raw.Currency), Tags: []string{}}
	if raw.Kind == "income" {
		out.Kind = "income"
	}
	if _, err := time.Parse("2006-01-02", out.Date); err != nil {
		out.Date = ""
	}
	cm, _ := ledger.CategoryMap(a.DB)
	if c, ok := cm[raw.Category]; ok && c.Kind == out.Kind {
		out.Category = raw.Category
	}
	// Only an account money is actually paid from — never property or a loan.
	for _, ac := range accts {
		if ac.ID == raw.AccountID && !ac.Archived && (ac.Kind == "checking" || ac.Kind == "savings" || ac.Kind == "cash") {
			out.AccountID = ac.ID
		}
	}
	known := map[string]bool{}
	for _, t := range tagList {
		known[t] = true
	}
	for _, t := range raw.Tags {
		if known[strings.ToLower(t)] {
			out.Tags = append(out.Tags, strings.ToLower(t))
		}
	}
	return out, nil
}

var (
	replyEuro = regexp.MustCompile(`(?:€\s?(\d[\d,  ]*(?:\.\d+)?)(?:\s?[kK]\b)?|(\d[\d,  ]*(?:[.,]\d{1,2})?)\s?€)`)
	dataNum   = regexp.MustCompile(`-?\d+(?:\.\d+)?`)
)

// uncheckedAmounts lists euro amounts of €50 or more in reply that match no
// number in the tool results (within a euro, either sign). Thousands written
// as "k" and percentages are left alone. Nothing to check against (no tools
// ran) means nothing is flagged — the reply then made no data claims.
func uncheckedAmounts(reply string, results []string) []string {
	if len(results) == 0 {
		return nil
	}
	var known []float64
	for _, r := range results {
		for _, m := range dataNum.FindAllString(r, -1) {
			if v, err := strconv.ParseFloat(m, 64); err == nil {
				known = append(known, math.Abs(v))
			}
		}
	}
	var out []string
	dup := map[string]bool{}
	for _, m := range replyEuro.FindAllStringSubmatch(reply, -1) {
		if strings.HasSuffix(strings.TrimSpace(m[0]), "k") || strings.HasSuffix(strings.TrimSpace(m[0]), "K") {
			continue
		}
		raw := m[1]
		if raw == "" {
			raw = m[2]
		}
		raw = strings.NewReplacer(",", "", " ", "", " ", "").Replace(strings.TrimSpace(raw))
		if m[2] != "" && strings.Count(m[2], ",") == 1 && !strings.Contains(m[2], ".") && len(m[2])-strings.Index(m[2], ",") <= 3 {
			raw = strings.Replace(strings.TrimSpace(m[2]), ",", ".", 1) // "12,50 €"
			raw = strings.ReplaceAll(raw, " ", "")
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || v < 50 {
			continue
		}
		ok := false
		for _, k := range known {
			if math.Abs(k-v) <= 1 {
				ok = true
				break
			}
		}
		label := strings.TrimSpace(m[0])
		if !ok && !dup[label] {
			dup[label] = true
			out = append(out, label)
		}
	}
	return out
}
