package ibkr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"ft/internal/db"
	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/wealth"
)

// Report is one sync: the account value, positions checked against the
// app's trades, and IBKR trades the app doesn't have yet (proposed, never
// imported without the owner).
type Report struct {
	At             string     `json:"at"`
	Account        string     `json:"account"`
	Currency       string     `json:"currency"`
	NetLiquidation float64    `json:"net_liquidation"`
	Cash           float64    `json:"cash"`
	AppBalance     *float64   `json:"app_balance,omitempty"` // the app's value before this sync
	BalanceWritten bool       `json:"balance_written"`
	Positions      []Position `json:"positions"`
	Trades         []Proposal `json:"trades"`
	New            int        `json:"new"`
	Mismatches     int        `json:"mismatches"`
	// Read only: live orders and saved order instructions (not yet orders).
	Orders       []Order `json:"orders"`
	Instructions []Order `json:"instructions"`
	OrdersError  string  `json:"orders_error,omitempty"`
	// Which trade window answered (YEAR_TO_DATE unless IBKR failed on it),
	// and why none did.
	TradesWindow string `json:"trades_window,omitempty"`
	TradesError  string `json:"trades_error,omitempty"`
}

// tradeWindows are tried in turn until IBKR answers one.
var tradeWindows = []string{"YEAR_TO_DATE", "DAYS_90", "DAYS_30", "MONTH_TO_DATE"}

// Order is a live order or a saved instruction, read as IBKR describes it.
type Order struct {
	ID          string  `json:"id"`
	Symbol      string  `json:"symbol"`
	Description string  `json:"description,omitempty"`
	Side        string  `json:"side"`
	Type        string  `json:"type"`
	Status      string  `json:"status,omitempty"`
	Quantity    float64 `json:"quantity"`
	Price       float64 `json:"price,omitempty"`
	Filled      float64 `json:"filled,omitempty"`
	TimeInForce string  `json:"tif,omitempty"`
	Created     string  `json:"created,omitempty"`
	Expires     string  `json:"expires,omitempty"`
}

// parseOrders reads IBKR's order list without depending on exact field
// names: the first array in the answer, each entry's usual keys.
func parseOrders(raw json.RawMessage) ([]Order, error) {
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err != nil {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, err
		}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if json.Unmarshal(obj[k], &arr) == nil {
				break
			}
		}
	}
	str := func(m map[string]any, keys ...string) string {
		for _, k := range keys {
			switch v := m[k].(type) {
			case string:
				if v != "" {
					return v
				}
			case float64:
				return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%f", v), "0"), ".")
			}
		}
		return ""
	}
	num := func(m map[string]any, keys ...string) float64 {
		for _, k := range keys {
			switch v := m[k].(type) {
			case float64:
				return v
			case string:
				var f float64
				if _, err := fmt.Sscanf(v, "%g", &f); err == nil {
					return f
				}
			}
		}
		return 0
	}
	out := []Order{}
	for _, m := range arr {
		out = append(out, Order{
			ID:          str(m, "order_id", "orderId", "instruction_id", "id"),
			Symbol:      strings.ToUpper(str(m, "symbol", "ticker", "contract_description")),
			Description: str(m, "description", "company_name"),
			Side:        strings.ToLower(str(m, "side", "action", "direction")),
			Type:        strings.ToLower(str(m, "order_type", "orderType", "type")),
			Status:      str(m, "status", "order_status"),
			Quantity:    num(m, "quantity", "total_size", "totalSize", "size"),
			Price:       num(m, "limit_price", "limitPrice", "price"),
			Filled:      num(m, "filled_quantity", "filledQuantity", "filled", "cum_fill"),
			TimeInForce: str(m, "time_in_force", "tif"),
			Created:     str(m, "creation_time", "created", "created_at", "order_time"),
			Expires:     str(m, "expiration", "expires", "expires_at"),
		})
	}
	return out, nil
}

type Position struct {
	Ticker      string  `json:"ticker"`
	Description string  `json:"description"`
	Currency    string  `json:"currency"`
	Shares      float64 `json:"shares"`     // at IBKR
	AppShares   float64 `json:"app_shares"` // from the app's trades on this account
	Price       float64 `json:"price"`
	Value       float64 `json:"value"`
	AvgPrice    float64 `json:"avg_price"`
	Unrealized  float64 `json:"unrealized"`
}

// Proposal is an IBKR trade, and whether the app already has it.
type Proposal struct {
	ExternalID string  `json:"external_id"`
	Date       string  `json:"date"` // local date of the trade
	Action     string  `json:"action"`
	Ticker     string  `json:"ticker"`
	Name       string  `json:"name"`
	Currency   string  `json:"currency"`
	Shares     float64 `json:"shares"`
	Price      float64 `json:"price"` // per share, as executed
	Commission float64 `json:"commission"`
	Recorded   bool    `json:"recorded"`
	MatchedID  int64   `json:"matched_id,omitempty"` // the app trade it matches
}

const reportKey = "ibkr_report"

type ibkrSummary struct {
	Currency       string  `json:"currency"`
	NetLiquidation float64 `json:"net_liquidation"`
	TotalCash      float64 `json:"total_cash_value"`
}

type ibkrPosition struct {
	Description string  `json:"contract_description"`
	Position    float64 `json:"position"`
	MarketPrice float64 `json:"market_price"`
	MarketValue float64 `json:"market_value"`
	Currency    string  `json:"currency"`
	AvgPrice    float64 `json:"average_price"`
	Unrealized  float64 `json:"unrealized_pnl"`
	AssetClass  string  `json:"asset_class"`
}

type ibkrTrade struct {
	TradeID    string  `json:"trade_id"`
	Symbol     string  `json:"symbol"`
	Company    string  `json:"company_name"`
	SecType    string  `json:"sec_type"`
	Currency   string  `json:"currency"`
	Side       string  `json:"side"`
	Size       float64 `json:"size"`
	Price      float64 `json:"price"`
	TradeTime  string  `json:"trade_time"`
	Commission float64 `json:"commission"`
}

// tickerOf turns "VWCE @IBIS2" into "VWCE".
func tickerOf(desc string) string {
	desc = strings.TrimSpace(desc)
	if i := strings.IndexAny(desc, " @"); i > 0 {
		desc = desc[:i]
	}
	return strings.ToUpper(desc)
}

func localDate(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		if len(ts) >= 10 {
			return ts[:10]
		}
		return ""
	}
	return t.In(time.Local).Format("2006-01-02")
}

func daysApart(a, b string) float64 {
	ta, e1 := time.Parse("2006-01-02", a)
	tb, e2 := time.Parse("2006-01-02", b)
	if e1 != nil || e2 != nil {
		return 999
	}
	return math.Abs(ta.Sub(tb).Hours() / 24)
}

// Sync reads the account from IBKR, records today's account value on the
// mapped account, and compares positions and trades with the app. With
// balanceOnly it stops after the value (the daily refresh).
func (s *Service) Sync(ctx context.Context, balanceOnly bool) (*Report, error) {
	r, err := s.sync(ctx, balanceOnly)
	if err != nil {
		s.note(err.Error(), false)
		return nil, err
	}
	s.note("", true)
	return r, nil
}

func (s *Service) sync(ctx context.Context, balanceOnly bool) (*Report, error) {
	st := s.load()
	if _, err := ledger.GetAccount(s.DB, st.Account); err != nil {
		return nil, fmt.Errorf("the IBKR account %q doesn't exist in the app — pick one in Settings → Banks", st.Account)
	}
	c, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := c.call(ctx, "get_account_summary", nil)
	if err != nil {
		return nil, err
	}
	var sum ibkrSummary
	if err := json.Unmarshal(raw, &sum); err != nil {
		return nil, fmt.Errorf("reading the IBKR summary: %w", err)
	}
	r := &Report{At: s.Now().UTC().Format(time.RFC3339), Account: st.Account, Currency: sum.Currency, NetLiquidation: sum.NetLiquidation, Cash: sum.TotalCash,
		Positions: []Position{}, Trades: []Proposal{}}
	today := s.Now().In(time.Local).Format("2006-01-02")
	if book, err := wealth.LoadBook(s.DB); err == nil {
		if pt, ok := book.At(st.Account, today); ok {
			v := pt.Value.Float()
			r.AppBalance = &v
		}
	}
	// Today's value only: earlier balance history is never touched.
	if strings.EqualFold(sum.Currency, "EUR") && sum.NetLiquidation > 0 {
		if err := wealth.SetBalance(s.DB, st.Account, today, money.FromFloat(sum.NetLiquidation), nil, nil, "broker"); err != nil {
			return nil, err
		}
		r.BalanceWritten = true
	}
	if balanceOnly {
		return s.saveReport(r, true)
	}

	trades, err := wealth.ListTrades(s.DB)
	if err != nil {
		return nil, err
	}
	appShares := map[string]float64{}
	for _, t := range trades {
		if t.AccountID != st.Account {
			continue
		}
		if t.Action == "sell" {
			appShares[t.Ticker] -= t.Shares
		} else {
			appShares[t.Ticker] += t.Shares
		}
	}

	if raw, err = c.call(ctx, "get_account_positions", nil); err != nil {
		return nil, err
	}
	var pos struct {
		Positions []ibkrPosition `json:"positions"`
	}
	if err := json.Unmarshal(raw, &pos); err != nil {
		return nil, fmt.Errorf("reading IBKR positions: %w", err)
	}
	seen := map[string]bool{}
	for _, p := range pos.Positions {
		if p.AssetClass != "" && p.AssetClass != "STK" {
			continue
		}
		tk := tickerOf(p.Description)
		seen[tk] = true
		x := Position{Ticker: tk, Description: p.Description, Currency: p.Currency, Shares: p.Position, AppShares: round6(appShares[tk]),
			Price: p.MarketPrice, Value: p.MarketValue, AvgPrice: p.AvgPrice, Unrealized: p.Unrealized}
		if math.Abs(x.Shares-x.AppShares) > 1e-6 {
			r.Mismatches++
		}
		r.Positions = append(r.Positions, x)
	}
	for tk, n := range appShares { // held in the app, not at IBKR
		if !seen[tk] && math.Abs(n) > 1e-6 {
			r.Positions = append(r.Positions, Position{Ticker: tk, AppShares: round6(n)})
			r.Mismatches++
		}
	}
	sort.Slice(r.Positions, func(i, j int) bool { return r.Positions[i].Value > r.Positions[j].Value })

	// Trades: the year so far, or — when IBKR fails on that window, as it
	// can for days — the widest shorter one that answers. New trades are
	// recent, so a shorter window still finds them. If none answers, the
	// value and positions still stand; the report says trades are missing.
	raw = nil
	for _, period := range tradeWindows {
		if raw, err = c.call(ctx, "get_account_trades", map[string]any{"period": period}); err == nil {
			r.TradesWindow = period
			break
		}
		r.TradesError = err.Error()
		if ctx.Err() != nil {
			break
		}
	}
	if raw == nil {
		raw = []byte(`{"trades":[]}`)
	} else {
		r.TradesError = ""
	}
	var tr struct {
		Trades []ibkrTrade `json:"trades"`
	}
	if err := json.Unmarshal(raw, &tr); err != nil {
		return nil, fmt.Errorf("reading IBKR trades: %w", err)
	}
	claimed := map[int64]bool{}
	byExt := map[string]int64{}
	for _, t := range trades {
		if t.ExternalID != "" {
			byExt[t.ExternalID] = t.ID
		}
	}
	for _, t := range tr.Trades {
		if t.SecType != "STK" || t.TradeID == "" || t.Size <= 0 {
			continue // currency conversions and the like are not portfolio trades
		}
		p := Proposal{ExternalID: t.TradeID, Date: localDate(t.TradeTime), Action: strings.ToLower(t.Side), Ticker: strings.ToUpper(t.Symbol),
			Name: t.Company, Currency: strings.ToUpper(t.Currency), Shares: t.Size, Price: t.Price, Commission: t.Commission}
		if id, ok := byExt[t.TradeID]; ok {
			p.Recorded, p.MatchedID = true, id
		} else {
			// Entered by hand before the sync existed: same ticker, side and
			// shares on this account, within three days.
			for _, a := range trades {
				if !claimed[a.ID] && a.ExternalID == "" && a.AccountID == st.Account && a.Ticker == p.Ticker && a.Action == p.Action &&
					math.Abs(a.Shares-p.Shares) < 1e-6 && daysApart(a.Date, p.Date) <= 3 {
					p.Recorded, p.MatchedID = true, a.ID
					claimed[a.ID] = true
					break
				}
			}
		}
		if !p.Recorded {
			r.New++
		}
		r.Trades = append(r.Trades, p)
	}
	sort.SliceStable(r.Trades, func(i, j int) bool { return r.Trades[i].Date > r.Trades[j].Date })

	// Open orders and saved instructions: shown, never acted on. A refusal
	// (an older sign-in, say) doesn't fail the sync.
	r.Orders, r.Instructions = []Order{}, []Order{}
	for tool, dst := range map[string]*[]Order{"get_account_orders": &r.Orders, "get_order_instructions": &r.Instructions} {
		raw, err := c.call(ctx, tool, nil)
		if err == nil {
			var list []Order
			if list, err = parseOrders(raw); err == nil {
				*dst = list
				continue
			}
		}
		r.OrdersError = "orders couldn't be read: " + err.Error()
	}
	return s.saveReport(r, false)
}

func round6(v float64) float64 { return math.Round(v*1e6) / 1e6 }

// saveReport keeps the latest report; a balance-only refresh keeps the last
// full comparison and updates just the value.
func (s *Service) saveReport(r *Report, balanceOnly bool) (*Report, error) {
	if balanceOnly {
		if prev, err := s.LastReport(); err == nil && prev != nil {
			prev.At, prev.NetLiquidation, prev.Cash, prev.Currency, prev.AppBalance, prev.BalanceWritten = r.At, r.NetLiquidation, r.Cash, r.Currency, r.AppBalance, r.BalanceWritten
			r = prev
		}
	}
	raw, _ := json.Marshal(r)
	_, err := s.DB.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, reportKey, string(raw))
	return r, err
}

func (s *Service) LastReport() (*Report, error) {
	var raw string
	if err := s.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, reportKey).Scan(&raw); err != nil {
		return nil, nil
	}
	var r Report
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Import adds the chosen IBKR trades from the last sync as app trades on the
// IBKR account. Commission goes into the per-share price (cost basis: added
// on buys, taken off sells) and is noted. A trade already in the app is
// skipped, never duplicated.
func (s *Service) Import(ids []string) (int, error) {
	r, err := s.LastReport()
	if err != nil || r == nil {
		return 0, errors.New("sync IBKR first")
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n := 0
	for i := range r.Trades {
		p := &r.Trades[i]
		if !want[p.ExternalID] || p.Recorded {
			continue
		}
		var exists int
		tx.QueryRow(`SELECT COUNT(*) FROM trades WHERE external_id=?`, p.ExternalID).Scan(&exists)
		if exists > 0 {
			p.Recorded = true
			continue
		}
		if !wealth.ValidTicker(p.Ticker) {
			return 0, fmt.Errorf("IBKR ticker %q can't be stored", p.Ticker)
		}
		price := p.Price
		if p.Shares > 0 {
			if p.Action == "sell" {
				price = (p.Shares*p.Price - p.Commission) / p.Shares
			} else {
				price = (p.Shares*p.Price + p.Commission) / p.Shares
			}
		}
		price = math.Round(price*1e6) / 1e6
		note := "IBKR"
		if p.Commission > 0 {
			note = fmt.Sprintf("IBKR · incl. %.2f %s commission", p.Commission, p.Currency)
		}
		res, err := tx.Exec(`INSERT INTO trades(date,account_id,action,ticker,shares,price,currency,notes,external_id,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			p.Date, r.Account, p.Action, p.Ticker, p.Shares, price, p.Currency, note, p.ExternalID, db.Now())
		if err != nil {
			return 0, err
		}
		p.MatchedID, _ = res.LastInsertId()
		p.Recorded = true
		r.New--
		n++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	_, err = s.saveReport(r, false)
	return n, err
}
