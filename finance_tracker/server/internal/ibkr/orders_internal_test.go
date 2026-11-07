package ibkr

import (
	"encoding/json"
	"testing"
)

// The answer IBKR's get_account_orders gives (prod, 6 Nov 2026): the order
// is named in prose, with no symbol or time-in-force field. It reads as
// "buy 6 VWCE, limit 162.8, GTC, 0 filled, 6 left" — not a nameless 0.
func TestAccountOrdersNamedInProseAreRead(t *testing.T) {
	raw := json.RawMessage(`{"orders":[{"order_id":15555562,"order_status":"REPLACED","order_type":"LIMIT","side":"BUY","limit_price":"162.8","total_shares_qty":"6","cum_shares_qty":"0","remaining_shares_qty":"6","primary_description":"Buy 6 VWCE","secondary_description":"Limit 162.8000, GTC","order_time":"2026-11-06T06:24:15Z"}]}`)
	got, err := parseOrders(raw)
	if err != nil || len(got) != 1 {
		t.Fatalf("%v %+v", err, got)
	}
	o := got[0]
	if o.ID != "15555562" || o.Symbol != "VWCE" || o.Side != "buy" || o.Type != "limit" || o.Status != "REPLACED" ||
		o.Quantity != 6 || o.Filled != 0 || o.Remaining != 6 || o.Price != 162.8 || o.TimeInForce != "GTC" || o.Created != "2026-11-06T06:24:15Z" || o.Description != "" {
		t.Errorf("%+v", o)
	}
	// A saved instruction names it in "description".
	got, _ = parseOrders(json.RawMessage(`{"instructions":[{"id":"i1","description":"Buy 10 VALL","direction":"BUY","quantity":10,"order_type":"LIMIT","limit_price":4.2,"time_in_force":"GTC"}]}`))
	if got[0].Symbol != "VALL" || got[0].Description != "" || got[0].TimeInForce != "GTC" || got[0].Remaining != 10 {
		t.Errorf("instruction %+v", got[0])
	}
	// Symbols with a space, names that are not prose, and prose without one.
	for _, c := range []struct{ raw, symbol, desc string }{
		{`[{"primary_description":"Sell 100 BRK B","total_shares_qty":"100"}]`, "BRK B", ""},
		{`[{"company_name":"VANGUARD FTSE ALL-WORLD UCITS ETF","quantity":6}]`, "", "VANGUARD FTSE ALL-WORLD UCITS ETF"},
		{`[{"primary_description":"Buy 6","quantity":6}]`, "", "Buy 6"},
		{`[{"primary_description":"Buy 2 AAPL JAN2027 150 C","quantity":2}]`, "AAPL JAN2027 150 C", ""},
	} {
		got, _ := parseOrders(json.RawMessage(c.raw))
		if got[0].Symbol != c.symbol || got[0].Description != c.desc {
			t.Errorf("%s → symbol %q desc %q", c.raw, got[0].Symbol, got[0].Description)
		}
	}
	// Shares from filled + left when IBKR gives only those; an explicit 0 left stays 0.
	got, _ = parseOrders(json.RawMessage(`[{"remaining_shares_qty":"6","cum_shares_qty":"0"},{"total_shares_qty":"6","cum_shares_qty":"2","remaining_shares_qty":"0"}]`))
	if got[0].Quantity != 6 || got[0].Remaining != 6 || got[1].Remaining != 0 {
		t.Errorf("shares %+v", got)
	}
	// Prose without a ticker at the end keeps the prose as the name.
	got, _ = parseOrders(json.RawMessage(`[{"id":"i2","primary_description":"Rebalance into world fund","quantity":3,"filled_quantity":1}]`))
	if got[0].Symbol != "" || got[0].Description != "Rebalance into world fund" || got[0].Remaining != 2 {
		t.Errorf("%+v", got[0])
	}
}
