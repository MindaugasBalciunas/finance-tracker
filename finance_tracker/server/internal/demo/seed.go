// Package demo builds a separate database full of believable, entirely
// fictional finances — to show the app without showing anyone's money.
package demo

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"time"

	"ft/internal/ledger"
	"ft/internal/money"
	"ft/internal/plan"
	"ft/internal/wealth"
)

const months = 24

// Seed fills an empty, migrated database. Deterministic: the same date gives
// the same demo.
func Seed(d *sql.DB, now time.Time) error {
	rng := rand.New(rand.NewSource(42))
	if err := ledger.SeedCategories(d); err != nil {
		return err
	}
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -months, 0)
	houseBought := start.AddDate(-3, 0, 17)
	accounts := []ledger.Account{
		{ID: "swed", Name: "Swedbank", Institution: "Swedbank", Kind: "checking", Liquid: true, Sort: 10},
		{ID: "swed_savings", Name: "Swedbank savings", Institution: "Swedbank", Kind: "savings", Liquid: true, Sort: 20},
		{ID: "seb", Name: "SEB", Institution: "SEB", Kind: "checking", Liquid: true, Sort: 30},
		{ID: "revolut", Name: "Revolut", Institution: "Revolut", Kind: "checking", Liquid: true, Sort: 40},
		{ID: "cash", Name: "Cash", Kind: "cash", Liquid: true, Sort: 50},
		{ID: "broker", Name: "ETF portfolio", Institution: "IBKR", Kind: "brokerage", Liquid: true, Sort: 60},
		{ID: "pension", Name: "Pension fund (III pillar)", Institution: "SEB", Kind: "pension", Liquid: true, Sort: 70},
		{ID: "btc", Name: "Bitcoin", Kind: "crypto", Liquid: true, Sort: 80},
		{ID: "home", Name: "Apartment, Vilnius", Kind: "property", Sort: 90,
			Details: js(map[string]any{"purchase_date": houseBought.Format("2006-01-02"), "purchase_price": 210000, "address": "Demo street 1, Vilnius"})},
		{ID: "car", Name: "Family car", Kind: "vehicle", Sort: 95,
			Details: js(map[string]any{"purchase_date": start.AddDate(-1, 2, 0).Format("2006-01-02"), "purchase_price": 24000, "address": "Hybrid estate"})},
		{ID: "mortgage", Name: "Mortgage", Institution: "SEB", Kind: "loan", Sort: 100,
			Details: js(map[string]any{"lender": "SEB", "asset_id": "home", "base_rate": 2.6, "base_rate_name": "6M EURIBOR", "margin": 1.6,
				"monthly_payment": 960, "payment_day": 17, "start_date": houseBought.Format("2006-01-02"), "start_principal": 168000,
				"end_date": houseBought.AddDate(25, 0, 0).Format("2006-01-02"), "rate_reset_date": now.AddDate(0, 4, 0).Format("2006-01-02")})},
	}
	for _, a := range accounts {
		if _, err := ledger.SaveAccount(d, a); err != nil {
			return fmt.Errorf("account %s: %w", a.ID, err)
		}
	}
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// add records the first failure so a bad seed row can't be skipped silently.
	var seedErr error
	add := func(date time.Time, kind, cat string, eur float64, from, to, merchant, note string, tags ...string) error {
		if date.After(now) || seedErr != nil {
			return seedErr
		}
		t := ledger.Tx{Date: date.Format("2006-01-02"), Kind: kind, Amount: money.FromFloat(math.Round(eur*100) / 100), AccountID: from, ToAccountID: to,
			Category: cat, Merchant: merchant, Note: note, Tags: tags, Source: "demo"}
		if err := ledger.Validate(tx, &t); err != nil {
			seedErr = fmt.Errorf("%s %s: %w", t.Date, merchant, err)
		} else if err := ledger.Insert(tx, &t); err != nil {
			seedErr = err
		}
		return seedErr
	}
	pick := func(xs ...string) string { return xs[rng.Intn(len(xs))] }
	between := func(lo, hi float64) float64 { return lo + rng.Float64()*(hi-lo) }

	checking, savings, broker, pension, btc := 3200.0, 4000.0, 6000.0, 9000.0, 1500.0
	owed := 168000.0 - 960*36*0.35 // three years of principal before the window
	owedAtStart := owed
	rate := (2.6 + 1.6) / 100 / 12
	type snap struct {
		date string
		v    map[string]float64
	}
	var snaps []snap
	var trades []wealth.Trade
	for m := 0; m <= months; m++ {
		first := start.AddDate(0, m, 0)
		day := func(n int) time.Time {
			last := first.AddDate(0, 1, -1).Day()
			if n > last {
				n = last
			}
			return time.Date(first.Year(), first.Month(), n, 0, 0, 0, 0, time.UTC)
		}
		salary := 3900.0 + float64(m/12)*150
		if err := add(day(25), "income", "salary", salary, "swed", "", "Northwind Ltd", "Salary"); err != nil {
			return err
		}
		checking += salary
		if m%4 == 2 {
			side := between(250, 600)
			add(day(12), "income", "side_income", side, "revolut", "", "Freelance client", "Weekend project")
			checking += side
		}
		// Mortgage: interest is a cost, principal builds equity.
		interest := owed * rate
		principal := 960 - interest
		owed -= principal
		add(day(17), "expense", "housing.mortgage_interest", interest, "seb", "", "SEB", "Mortgage interest")
		add(day(17), "transfer", "transfer.debt", principal, "seb", "mortgage", "SEB", "Mortgage principal")
		checking -= 960
		for _, f := range []struct {
			d             int
			cat, merchant string
			lo, hi        float64
		}{
			{5, "utilities.electricity", "Ignitis", 45, 95}, {6, "utilities.heating", "Vilniaus šilumos tinklai", 20, 140},
			{8, "utilities.telecom", "Telia", 29.99, 29.99}, {3, "subscriptions.media", "Netflix", 12.99, 12.99},
			{9, "subscriptions.media", "Spotify", 11.99, 11.99}, {1, "health.fitness", "Gym+", 39, 39},
			{20, "subscriptions.software", "Cloud storage", 2.99, 2.99},
		} {
			v := between(f.lo, f.hi)
			add(day(f.d), "expense", f.cat, v, "swed", "", f.merchant, "")
			checking -= v
		}
		// Daily life.
		for w := 0; w < 4; w++ {
			for k := 0; k < 2+rng.Intn(2); k++ {
				v := between(14, 85)
				add(day(1+w*7+rng.Intn(6)), "expense", "food.groceries", v, "swed", "", pick("Maxima", "Lidl", "Rimi", "Iki"), "")
				checking -= v
			}
			if rng.Float64() < 0.7 {
				v := between(3, 6.5)
				add(day(2+w*7+rng.Intn(5)), "expense", "food.coffee", v, "revolut", "", pick("Caffeine", "Vero Cafe", "Huracan"), "", "work")
				checking -= v
			}
			if rng.Float64() < 0.45 {
				v := between(25, 95)
				add(day(4+w*7+rng.Intn(3)), "expense", "food.restaurants", v, "revolut", "", pick("Lokys", "Džiaugsmas", "Sushi Bar", "Pizza Jazz"), "", "family")
				checking -= v
			}
			if rng.Float64() < 0.6 {
				v := between(40, 75)
				add(day(3+w*7+rng.Intn(4)), "expense", "transport.fuel", v, "swed", "", pick("Circle K", "Viada", "Neste"), "", "car")
				checking -= v
			}
		}
		if rng.Float64() < 0.5 {
			v := between(25, 160)
			add(day(14+rng.Intn(10)), "expense", pick("shopping.clothing", "shopping.online", "shopping.electronics"), v, "swed", "", pick("Zalando", "Varle", "H&M", "Pigu"), "")
			checking -= v
		}
		if rng.Float64() < 0.35 {
			v := between(15, 60)
			add(day(10+rng.Intn(12)), "expense", pick("leisure.events", "leisure.going_out", "leisure.hobbies"), v, "revolut", "", pick("Bilietai.lt", "Forum Cinemas", "Bowling"), "", "family")
			checking -= v
		}
		if rng.Float64() < 0.25 {
			v := between(20, 80)
			add(day(5+rng.Intn(20)), "expense", "health.pharmacy", v, "swed", "", pick("Camelia", "Eurovaistinė"), "")
			checking -= v
		}
		if first.Month() == time.July {
			trip := fmt.Sprintf("trip:summer-%d", first.Year())
			add(day(2), "expense", "travel.flights", between(380, 520), "swed", "", "airBaltic", "Flights", trip)
			add(day(8), "expense", "travel.lodging", between(600, 900), "swed", "", "Booking.com", "Hotel", trip)
			add(day(10), "expense", "food.restaurants", between(150, 260), "revolut", "", "Trattoria", "Holiday dinners", trip)
			checking -= 1500
		}
		if first.Month() == time.December {
			v := between(200, 380)
			add(day(18), "expense", "gifts", v, "swed", "", "Gifts", "Christmas", "family")
			checking -= v
		}
		// Saving and investing on payday.
		add(day(26), "transfer", "transfer.internal", 400, "swed", "swed_savings", "Swedbank", "Monthly saving")
		add(day(26), "transfer", "transfer.invest", 350, "swed", "broker", "IBKR", "ETF purchase")
		add(day(27), "transfer", "transfer.pension", 100, "swed", "pension", "SEB", "Pension top-up")
		checking -= 850
		savings = savings*1.002 + 400
		broker = broker*(1+between(-0.035, 0.05)) + 350
		pension = pension*(1+between(-0.01, 0.015)) + 100 + 180 // + employer
		btc *= 1 + between(-0.18, 0.22)
		if checking > 3800 { // sweep the month's surplus into the portfolio
			extra := math.Floor((checking-3000)/50) * 50
			add(day(28), "transfer", "transfer.invest", extra, "swed", "broker", "IBKR", "Surplus to ETF")
			checking -= extra
			broker += extra
		}
		if checking < 1200 { // a top-up from savings in a heavy month
			add(day(28), "transfer", "transfer.internal", 800, "swed_savings", "swed", "Swedbank", "Top-up")
			checking += 800
			savings -= 800
		}
		// ETF buys at a drifting price.
		price := 98 + float64(m)*0.9 + between(-3, 3)
		if !day(26).After(now) { // saved after the commit: SQLite allows one writer
			trades = append(trades, wealth.Trade{Date: day(26).Format("2006-01-02"), AccountID: "broker", Action: "buy", Ticker: "VWCE.DE",
				Shares: math.Round(350/price*1000) / 1000, Price: math.Round(price*100) / 100, Currency: "EUR"})
		}
		end := first.AddDate(0, 1, -1)
		if end.After(now) {
			end = now
		}
		snaps = append(snaps, snap{end.Format("2006-01-02"), map[string]float64{
			"swed": checking * 0.82, "seb": 400 + between(0, 300), "revolut": checking*0.12 + between(20, 120), "cash": 150 + between(0, 120),
			"swed_savings": savings, "broker": broker, "pension": pension, "btc": btc,
			"home": 210000 + 4500*float64(m+36)/12, "car": math.Max(9000, 24000-280*float64(m+12)), "mortgage": -owed,
		}})
	}
	if seedErr != nil {
		return seedErr
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// A year of balances before the transactions, so long-range charts start
	// with history instead of a jump from zero.
	for m := -12; m < 0; m++ {
		end := start.AddDate(0, m+1, -1)
		f := float64(12+m) / 12 // 0 → 1 across the year
		pre := map[string]float64{
			"swed": 2600 + 400*f, "seb": 450, "revolut": 260, "cash": 180,
			"swed_savings": 2500 + 1500*f, "broker": 2500 + 3500*f, "pension": 6500 + 2500*f, "btc": 900 + 600*f*(1+0.2*math.Sin(float64(m))),
			"home": 210000 + 4500*float64(m+36)/12, "car": math.Max(9000, 24000-280*float64(m+12)),
			"mortgage": -(owedAtStart + (12-float64(12+m))*330),
		}
		for id, v := range pre {
			if err := wealth.SetBalance(d, id, end.Format("2006-01-02"), money.FromFloat(math.Round(v*100)/100), nil, nil, "manual"); err != nil {
				return err
			}
		}
	}
	for i := range trades {
		if err := wealth.SaveTrade(d, &trades[i]); err != nil {
			return err
		}
	}
	for _, s := range snaps {
		for id, v := range s.v {
			if err := wealth.SetBalance(d, id, s.date, money.FromFloat(math.Round(v*100)/100), nil, nil, "manual"); err != nil {
				return fmt.Errorf("balance %s %s: %w", id, s.date, err)
			}
		}
	}
	for _, b := range []plan.Budget{
		{Name: "Home loan", Kind: "fixed", Categories: []string{"housing.mortgage_interest", "transfer.debt"}, Period: "monthly", Amount: money.FromFloat(960)},
		{Name: "Utilities & phone", Kind: "fixed", Categories: []string{"utilities"}, Period: "monthly", Amount: money.FromFloat(190)},
		{Name: "Subscriptions", Kind: "fixed", Categories: []string{"subscriptions", "health.fitness"}, Period: "monthly", Amount: money.FromFloat(70)},
		{Name: "Saving", Kind: "saving", Categories: []string{"transfer.internal"}, Period: "monthly", Amount: money.FromFloat(400)},
		{Name: "Investing", Kind: "saving", Categories: []string{"transfer.invest", "transfer.pension"}, Period: "monthly", Amount: money.FromFloat(450)},
		{Name: "Food", Kind: "spending", Categories: []string{"food"}, Period: "monthly", Amount: money.FromFloat(700)},
		{Name: "Transport", Kind: "spending", Categories: []string{"transport"}, Period: "monthly", Amount: money.FromFloat(220)},
		{Name: "Leisure & shopping", Kind: "spending", Categories: []string{"leisure", "shopping"}, Period: "monthly", Amount: money.FromFloat(250)},
		{Name: "Travel", Kind: "spending", Categories: []string{"travel"}, Period: "yearly", Fund: true, Amount: money.FromFloat(1800)},
	} {
		b := b
		if err := plan.Save(d, &b, start.Format("2006-01")); err != nil {
			return fmt.Errorf("budget %s: %w", b.Name, err)
		}
	}
	return nil
}

func js(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
