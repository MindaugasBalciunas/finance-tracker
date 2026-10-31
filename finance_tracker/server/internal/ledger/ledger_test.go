package ledger_test

import (
	"strings"
	"testing"
	"time"

	"ft/internal/ledger"
	"ft/internal/plan"
	. "ft/internal/testutil"
)

func TestTagsNormalizeAndStorage(t *testing.T) {
	got := ledger.NormalizeTags([]string{" Kids", "kids", "", "Trip:Rome", ","})
	if strings.Join(got, "|") != "kids|trip:rome" {
		t.Fatal(got)
	}
	if ledger.JoinTags(got) != ",kids,trip:rome," || !ledger.HasTag(",kids,trip:rome,", "Kids") {
		t.Fatal("storage form")
	}
	if ledger.JoinTags(nil) != "" || len(ledger.SplitTags("")) != 0 {
		t.Fatal("empty")
	}
}

func TestValidate(t *testing.T) {
	d := DB(t)
	ok := ledger.Tx{Date: "2026-10-01", Amount: E(10), Category: "food.groceries", AccountID: "swed"}
	if err := ledger.Validate(d, &ok); err != nil || ok.Kind != "expense" {
		t.Fatalf("kind inferred from category: %v %q", err, ok.Kind)
	}
	bad := []struct {
		name string
		tx   ledger.Tx
		want string
	}{
		{"bad date", ledger.Tx{Date: "2026-13-01", Amount: E(1), Category: "food"}, "invalid date"},
		{"zero amount", ledger.Tx{Date: "2026-10-01", Category: "food"}, "positive"},
		{"unknown category", ledger.Tx{Date: "2026-10-01", Amount: E(1), Category: "nope"}, "unknown category"},
		{"kind mismatch", ledger.Tx{Date: "2026-10-01", Amount: E(1), Category: "salary", Kind: "expense"}, "is for income"},
		{"unknown account", ledger.Tx{Date: "2026-10-01", Amount: E(1), Category: "food", AccountID: "nope"}, "unknown account"},
		// Daily-transaction logic: the house, solar or a car are never "paid from".
		{"property payer", ledger.Tx{Date: "2026-10-01", Amount: E(1), Category: "food", AccountID: "house"}, "valued, not paid from"},
		{"property destination", ledger.Tx{Date: "2026-10-01", Amount: E(1), Category: "transfer.asset", AccountID: "swed", ToAccountID: "house"}, "valued, not paid from"},
		{"loan pays groceries", ledger.Tx{Date: "2026-10-01", Amount: E(1), Category: "food", AccountID: "mortgage"}, "can't pay for things"},
		{"pension pays groceries", ledger.Tx{Date: "2026-10-01", Amount: E(1), Category: "food", AccountID: "artea"}, "can't pay for things"},
	}
	for _, b := range bad {
		tx := b.tx
		err := ledger.Validate(d, &tx)
		if err == nil || !strings.Contains(err.Error(), b.want) {
			t.Errorf("%s: got %v, want %q", b.name, err, b.want)
		}
	}
	// Principal into the mortgage and contributions into a pension are fine.
	for _, tx := range []ledger.Tx{
		{Date: "2026-10-01", Amount: E(500), Category: "transfer.debt", AccountID: "seb", ToAccountID: "mortgage"},
		{Date: "2026-10-01", Amount: E(200), Category: "transfer.pension", ToAccountID: "artea"},
	} {
		if err := ledger.Validate(d, &tx); err != nil {
			t.Errorf("%s: %v", tx.Category, err)
		}
	}
	// A non-transfer never keeps a destination account.
	inc := ledger.Tx{Date: "2026-10-01", Amount: E(1), Category: "salary", AccountID: "swed", ToAccountID: "seb"}
	ledger.Validate(d, &inc)
	if inc.ToAccountID != "" {
		t.Error("income kept a to-account")
	}
}

func TestListFilters(t *testing.T) {
	d := DB(t)
	Tx(t, d, ledger.Tx{Date: "2026-09-01", Amount: E(50), Category: "food.groceries", Merchant: "Maxima", AccountID: "swed", Tags: []string{"kids"}})
	Tx(t, d, ledger.Tx{Date: "2026-09-02", Amount: E(30), Category: "food.restaurants", Merchant: "Jammi", AccountID: "swed", Tags: []string{"kristina"}})
	Tx(t, d, ledger.Tx{Date: "2026-09-03", Amount: E(3000), Category: "salary", Merchant: "Nexos", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: "2026-09-04", Amount: E(1000), Category: "transfer.invest", AccountID: "swed", ToAccountID: "ibkr"})
	Tx(t, d, ledger.Tx{Date: "2026-10-01", Amount: E(20), Category: "transport.fuel", Merchant: "Neste", AccountID: "cash", Note: "roadtrip", Tags: []string{"kids", "trip:zakopane"}})

	count := func(f ledger.Filter) int {
		r, err := ledger.List(d, f)
		if err != nil {
			t.Fatal(err)
		}
		return r.Total
	}
	cases := []struct {
		name string
		f    ledger.Filter
		want int
	}{
		{"all", ledger.Filter{}, 5},
		{"parent category includes leaves", ledger.Filter{Categories: []string{"food"}}, 2},
		{"leaf", ledger.Filter{Categories: []string{"food.groceries"}}, 1},
		{"date range", ledger.Filter{From: "2026-09-02", To: "2026-09-30"}, 3},
		{"kind", ledger.Filter{Kind: "transfer"}, 1},
		{"account either side", ledger.Filter{Accounts: []string{"ibkr"}}, 1},
		{"tags any", ledger.Filter{Tags: []string{"kids", "kristina"}}, 3},
		{"tags all", ledger.Filter{Tags: []string{"kids", "trip:zakopane"}, TagsAll: true}, 1},
		{"query merchant", ledger.Filter{Query: "nest"}, 1},
		{"query note", ledger.Filter{Query: "roadtrip"}, 1},
		{"min", ledger.Filter{Min: E(1000)}, 2},
		{"max", ledger.Filter{Max: E(30)}, 2},
		{"merchant exact", ledger.Filter{Merchant: "Maxima"}, 1},
	}
	for _, c := range cases {
		if got := count(c.f); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
	r, _ := ledger.List(d, ledger.Filter{From: "2026-09-01", To: "2026-09-30"})
	if r.Income != E(3000) || r.Expense != E(80) || r.Transfer != E(1000) {
		t.Errorf("totals %v %v %v", r.Income, r.Expense, r.Transfer)
	}
	r, _ = ledger.List(d, ledger.Filter{Sort: "amount", Limit: 1})
	if len(r.Items) != 1 || r.Items[0].Amount != E(3000) || r.Total != 5 {
		t.Error("sort+limit")
	}
}

func TestSplitAndUnsplit(t *testing.T) {
	d := DB(t)
	orig := Tx(t, d, ledger.Tx{Date: "2026-09-10", Amount: E(100), Category: "food.groceries", Merchant: "Maxima", AccountID: "swed"})
	parts, err := ledger.Split(d, orig.ID, []ledger.SplitPart{
		{Amount: E(30), Category: "kids.general"},
		{Amount: E(20), Category: "food.groceries", OwedBy: "Kristina"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 2 || parts[1].Tags[0] != "owed:kristina" || parts[0].SplitOf != orig.ID {
		t.Fatalf("parts %+v", parts)
	}
	all, _ := ledger.All(d, ledger.Filter{})
	var sum int64
	for _, x := range all {
		sum += int64(x.Amount)
	}
	if sum != int64(E(100)) {
		t.Fatalf("split must keep the total: %d", sum)
	}
	if p, _ := ledger.Get(d, orig.ID); p.Amount != E(50) {
		t.Fatalf("parent keeps the remainder: %v", p.Amount)
	}
	if _, err := ledger.Split(d, orig.ID, []ledger.SplitPart{{Amount: E(50)}}); err == nil {
		t.Error("parts must leave a remainder")
	}
	if _, err := ledger.Split(d, parts[0].ID, []ledger.SplitPart{{Amount: E(1)}}); err == nil {
		t.Error("a part cannot be split again")
	}
	if err := ledger.Unsplit(d, orig.ID); err != nil {
		t.Fatal(err)
	}
	if p, _ := ledger.Get(d, orig.ID); p.Amount != E(100) {
		t.Fatal("unsplit restores the amount")
	}
	if all, _ := ledger.All(d, ledger.Filter{}); len(all) != 1 {
		t.Fatal("unsplit removes the parts")
	}
}

func TestRenameTagUpdatesRulesAndBudgets(t *testing.T) {
	d := DB(t)
	Tx(t, d, ledger.Tx{Date: "2026-09-10", Amount: E(10), Category: "food", Tags: []string{"kristina", "date"}})
	r := ledger.Rule{Pattern: "jammi", AddTags: []string{"kristina"}, Enabled: true}
	ledger.SaveRule(d, &r)
	b := plan.Budget{Name: "K", Kind: "spending", Tag: "kristina", Amount: E(100)}
	if err := plan.Save(d, &b, ""); err != nil {
		t.Fatal(err)
	}
	n, err := ledger.RenameTag(d, "kristina", "partner")
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	all, _ := ledger.All(d, ledger.Filter{})
	if strings.Join(all[0].Tags, ",") != "date,partner" {
		t.Error(all[0].Tags)
	}
	rules, _ := ledger.ListRules(d)
	if rules[0].AddTags[0] != "partner" {
		t.Error("rule tag", rules[0].AddTags)
	}
	bs, _ := plan.List(d, true)
	if bs[0].Tag != "partner" {
		t.Error("budget tag", bs[0].Tag)
	}
	// Removing a tag archives a budget that matched only by it.
	ledger.RenameTag(d, "partner", "")
	bs, _ = plan.List(d, true)
	if !bs[0].Archived {
		t.Error("budget on a removed tag must be archived")
	}
}

func TestRuleMatching(t *testing.T) {
	tx := func(cat, m, note string) *ledger.Tx {
		return &ledger.Tx{Kind: "expense", Category: cat, Merchant: m, Note: note}
	}
	cases := []struct {
		r    ledger.Rule
		t    *ledger.Tx
		want bool
	}{
		{ledger.Rule{Pattern: "maxima"}, tx("food", "MAXIMA LT", ""), true},
		{ledger.Rule{Pattern: "silumos"}, tx("utilities", "Vilniaus šilumos tinklai", ""), true}, // diacritics folded
		{ledger.Rule{Pattern: "^iki"}, tx("food", "IKI Express", ""), true},
		{ledger.Rule{Pattern: "^iki"}, tx("food", "Mokėjimas", "galioja iki rytojaus"), false}, // "until"
		{ledger.Rule{Pattern: "wolt", WhenCategory: "food"}, tx("food.delivery", "Wolt", ""), true},
		{ledger.Rule{Pattern: "wolt", WhenCategory: "food"}, tx("leisure", "Wolt", ""), false},
		{ledger.Rule{WhenCategory: "dating"}, tx("dating", "x", ""), true},
		{ledger.Rule{}, tx("dating", "x", ""), false}, // an empty rule matches nothing
		{ledger.Rule{Pattern: "maxima", WhenKind: "income"}, tx("food", "Maxima", ""), false},
	}
	for i, c := range cases {
		c.r.Enabled = true
		if got := c.r.Matches(c.t); got != c.want {
			t.Errorf("case %d: %v, want %v", i, got, c.want)
		}
	}
	if (ledger.Rule{Pattern: "maxima"}).Matches(tx("food", "Maxima", "")) {
		t.Error("disabled rule matched")
	}
}

func TestEngineApply(t *testing.T) {
	d := DB(t)
	// History: Lidl is groceries.
	for i := 0; i < 3; i++ {
		Tx(t, d, ledger.Tx{Date: "2026-09-0" + string(rune('1'+i)), Amount: E(20), Category: "food.groceries", Merchant: "Lidl", AccountID: "swed"})
	}
	Tx(t, d, ledger.Tx{Date: "2026-09-05", Amount: E(20), Category: "food.restaurants", Merchant: "Lidl", AccountID: "swed"})
	r1 := ledger.Rule{Pattern: "wolt", SetCategory: "food.delivery", SetMerchant: "Wolt", AddTags: []string{"delivery-app"}, Enabled: true, Priority: 10}
	r2 := ledger.Rule{WhenCategory: "dating", AddTags: []string{"kristina"}, Enabled: true}
	ledger.SaveRule(d, &r1)
	ledger.SaveRule(d, &r2)
	eng, err := ledger.LoadEngine(d)
	if err != nil {
		t.Fatal(err)
	}

	// Learned from history: most frequent category wins.
	a := ledger.Tx{Kind: "expense", Note: "LIDL VILNIUS 0123"}
	out := eng.Apply(&a, true, nil)
	if a.Merchant != "Lidl" || a.Category != "food.groceries" || !out.Learned {
		t.Errorf("learned: %+v %+v", a, out)
	}
	// Rule beats history and sets merchant; tags added.
	b := ledger.Tx{Kind: "expense", Note: "WOLT HELSINKI"}
	eng.Apply(&b, true, nil)
	if b.Category != "food.delivery" || b.Merchant != "Wolt" || !ledger.HasTag(ledger.JoinTags(b.Tags), "delivery-app") {
		t.Errorf("rule: %+v", b)
	}
	// An explicit category is never overwritten; tags the user removed stay removed.
	c := ledger.Tx{Kind: "expense", Category: "dating", Note: "WOLT"}
	eng.Apply(&c, false, []string{"delivery-app"})
	if c.Category != "dating" || ledger.HasTag(ledger.JoinTags(c.Tags), "delivery-app") || !ledger.HasTag(ledger.JoinTags(c.Tags), "kristina") {
		t.Errorf("explicit category / suppressed: %+v", c)
	}
	// A merchant the user typed is kept even when a rule sets one.
	u := ledger.Tx{Kind: "expense", Merchant: "Wolt Market", Note: "wolt"}
	eng.Apply(&u, true, nil)
	if u.Merchant != "Wolt Market" {
		t.Errorf("user merchant overwritten: %q", u.Merchant)
	}
	// A rule setting an income category never turns an expense into income.
	r3 := ledger.Rule{Pattern: "refund", SetCategory: "refunds", Enabled: true}
	ledger.SaveRule(d, &r3)
	eng, _ = ledger.LoadEngine(d)
	e := ledger.Tx{Kind: "expense", Note: "refund shop"}
	eng.Apply(&e, true, nil)
	if e.Category == "refunds" {
		t.Error("kind crossed by a rule")
	}
}

func TestPreviewAndApplyRuleToHistory(t *testing.T) {
	d := DB(t)
	Tx(t, d, ledger.Tx{Date: "2026-09-01", Amount: E(9), Category: "leisure", Merchant: "Patreon", AccountID: "swed"})
	Tx(t, d, ledger.Tx{Date: "2026-09-02", Amount: E(9), Category: "leisure", Merchant: "Steam", AccountID: "swed"})
	r := ledger.Rule{Pattern: "patreon", SetCategory: "subscriptions.creators", AddTags: []string{"creator"}}
	n, sample, err := ledger.PreviewRule(d, r)
	if err != nil || n != 1 || sample[0].Merchant != "Patreon" {
		t.Fatal(n, err)
	}
	changed, err := ledger.ApplyRuleToHistory(d, r)
	if err != nil || changed != 1 {
		t.Fatal(changed, err)
	}
	all, _ := ledger.All(d, ledger.Filter{Merchant: "Patreon"})
	if all[0].Category != "subscriptions.creators" || all[0].Tags[0] != "creator" {
		t.Error(all[0])
	}
	if again, _ := ledger.ApplyRuleToHistory(d, r); again != 0 {
		t.Error("applying twice must be a no-op")
	}
}

func TestCategoriesAndAccounts(t *testing.T) {
	d := DB(t)
	c := ledger.Category{Parent: "food", Name: "Bakery"}
	if err := ledger.SaveCategory(d, &c); err != nil || c.ID != "food.bakery" || c.Kind != "expense" {
		t.Fatal(c, err)
	}
	if err := ledger.SaveCategory(d, &ledger.Category{Parent: "food", Name: "Bakery"}); err == nil {
		t.Error("duplicate category accepted")
	}
	if err := ledger.SaveCategory(d, &ledger.Category{Parent: "food.groceries", Name: "X"}); err == nil {
		t.Error("third level accepted")
	}
	if ledger.Top("food.groceries") != "food" || ledger.Top("food") != "food" {
		t.Error("Top")
	}
	if ledger.SlugID("Swedbank Savings Ąžuolas") != "swedbank_savings_azuolas" {
		t.Error(ledger.SlugID("Swedbank Savings Ąžuolas"))
	}
	if _, err := ledger.SaveAccount(d, ledger.Account{Name: "X", Kind: "spaceship"}); err == nil {
		t.Error("unknown kind accepted")
	}
	Tx(t, d, ledger.Tx{Date: "2026-09-01", Amount: E(1), Category: "food", AccountID: "seb"})
	if err := ledger.DeleteAccount(d, "seb"); err == nil {
		t.Error("deleting a used account must fail")
	}
	if err := ledger.DeleteAccount(d, "revolut"); err != nil {
		t.Error(err)
	}
	if ledger.KindGroup("loan") != "debt" || ledger.KindGroup("vehicle") != "real_assets" {
		t.Error("groups")
	}
}

func TestTagsAndMerchantsAggregate(t *testing.T) {
	d := DB(t)
	Tx(t, d, ledger.Tx{Date: "2026-09-01", Amount: E(10), Category: "food.groceries", Merchant: "Lidl", AccountID: "swed", Tags: []string{"kids"}})
	Tx(t, d, ledger.Tx{Date: "2026-09-05", Amount: E(15), Category: "food.groceries", Merchant: "Lidl", AccountID: "swed", Tags: []string{"kids"}})
	tags, _ := ledger.ListTags(d)
	if len(tags) != 1 || tags[0].Count != 2 || tags[0].Last != "2026-09-05" {
		t.Error(tags)
	}
	m, _ := ledger.ListMerchants(d, "", "")
	if len(m) != 1 || m[0].Total != E(25) || m[0].Category != "food.groceries" {
		t.Error(m)
	}
}

func TestSuggestTagMerges(t *testing.T) {
	got := ledger.SuggestTagMerges([]ledger.TagCount{
		{Tag: "kid", Count: 3}, {Tag: "kids", Count: 300}, {Tag: "car-wash", Count: 2}, {Tag: "carwash", Count: 9},
		{Tag: "kristina", Count: 60}, {Tag: "kristna", Count: 1}, {Tag: "trip:rome", Count: 5}, {Tag: "rome", Count: 5}, {Tag: "gift", Count: 1}, {Tag: "lift", Count: 1},
	})
	want := map[string]string{"kid": "kids", "car-wash": "carwash", "kristna": "kristina"}
	if len(got) != len(want) {
		t.Fatalf("%+v", got)
	}
	for _, g := range got {
		if want[g.From] != g.To {
			t.Errorf("%+v", g)
		}
	}
}

func TestAccountInstitutionTrimmed(t *testing.T) {
	d := DB(t)
	// A stray space ("Revolut ") split one bank into two in the Banks view.
	d.Exec(`INSERT INTO accounts(id,name,institution,kind,currency,liquid,archived,sort,notes,details,created_at,updated_at) VALUES('btc_x',' BTC ','Revolut ','crypto','EUR',1,0,0,'','{}','x','x')`)
	a, err := ledger.GetAccount(d, "btc_x")
	if err != nil || a.Institution != "Revolut" || a.Name != "BTC" {
		t.Fatalf("%q %q %v", a.Institution, a.Name, err)
	}
	saved, err := ledger.SaveAccount(d, ledger.Account{ID: "x2", Name: "X", Institution: " SEB ", Kind: "checking"})
	if err != nil || saved.Institution != "SEB" {
		t.Fatalf("%+v %v", saved, err)
	}
}

func TestSuggestNotesFromEarlierEntries(t *testing.T) {
	d := DB(t)
	day := time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	add := func(m, note string) {
		Tx(t, d, ledger.Tx{Date: day, Kind: "expense", Amount: E(10), Category: "food.groceries", Merchant: m, Note: note, AccountID: "swed"})
	}
	add("Norfa", "small groceries")
	add("Norfa", "small groceries")
	add("Maxima", "weekly shop")
	add("Maxima", "groceries for kids")
	add("Lidl", "Lidl") // a narrative repeating the merchant is not a note
	add("Norfa", "NORFA - BT")
	add("Norfa", `NORFA\ZUJUNU G. 1\VILNIUS`)
	got, err := ledger.SuggestNotes(d, "Norfa", "gro", 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Note != "small groceries" || !got[0].Same || got[0].Count != 2 || got[1].Note != "groceries for kids" {
		t.Fatalf("got %+v", got)
	}
	got, _ = ledger.SuggestNotes(d, "Maxima", "", 6)
	if len(got) != 2 || !got[0].Same || !got[1].Same {
		t.Fatalf("merchant's own notes first: %+v", got)
	}
	if got, _ := ledger.SuggestNotes(d, "", "small groceries", 6); len(got) != 0 {
		t.Fatalf("what is already typed is not suggested back: %+v", got)
	}
}

// Totals need to know which way wealth moves: a 3rd-pillar payout into the
// bank is not investing, and a broker-to-pension move is neither.
func TestWealthDirection(t *testing.T) {
	d := DB(t)
	for _, x := range []ledger.Tx{
		{Date: "2026-09-01", Kind: "transfer", Amount: E(1000), Category: "transfer.invest", AccountID: "swed", ToAccountID: "ibkr"},
		{Date: "2026-09-02", Kind: "transfer", Amount: E(17197.92), Category: "transfer.pension", AccountID: "artea", ToAccountID: "swed"},
		{Date: "2026-09-03", Kind: "transfer", Amount: E(500), Category: "transfer.pension", AccountID: "ibkr", ToAccountID: "artea"},
		{Date: "2026-09-04", Kind: "transfer", Amount: E(450), Category: "transfer.debt", AccountID: "swed", ToAccountID: "mortgage"},
		{Date: "2026-09-05", Kind: "transfer", Amount: E(300), Category: "transfer.internal", AccountID: "swed", ToAccountID: "seb"},
		{Date: "2026-09-06", Kind: "transfer", Amount: E(200), Category: "transfer.invest", AccountID: "ibkr"},
		{Date: "2026-09-07", Kind: "transfer", Amount: E(900), Category: "transfer.pension", ToAccountID: "swed"},  // a bank row: payout arriving
		{Date: "2026-09-08", Kind: "transfer", Amount: E(120), Category: "transfer.pension", ToAccountID: "artea"}, // payroll pension
	} {
		Tx(t, d, x)
	}
	txs, err := ledger.All(d, ledger.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, -1, 0, 1, 0, -1, -1, 1}
	for i, x := range txs {
		if got := x.WealthDirection(); got != want[i] {
			t.Errorf("%s %s→%s (%s→%s): %d, want %d", x.Category, x.AccountID, x.ToAccountID, x.FromKind, x.ToKind, got, want[i])
		}
	}
	// Rows built without accounts (older callers, tests) still count as investing.
	if (ledger.Tx{Kind: "transfer", Category: "transfer.invest"}).WealthDirection() != 1 {
		t.Error("unknown accounts")
	}
}

// A merchant filter matches whatever case the bank used, so a figure that
// adds up "NESTE" and "Neste" lists both.
func TestMerchantFilterIgnoresCase(t *testing.T) {
	d := DB(t)
	for _, m := range []string{"Neste", "NESTE", "Circle K"} {
		Tx(t, d, ledger.Tx{Date: "2026-09-01", Kind: "expense", Amount: E(50), Category: "transport.fuel", Merchant: m, AccountID: "swed"})
	}
	if got, _ := ledger.All(d, ledger.Filter{Merchant: "neste"}); len(got) != 2 {
		t.Errorf("merchant filter: %d rows", len(got))
	}
	if l := ledger.MonthLink("2026-09", "category", "food", "tag", ""); l != "/ledger?category=food&from=2026-09-01&to=2026-09-30" {
		t.Errorf("month link %q", l)
	}
}

// A trip goes to history by hand, or by itself a month after it ends —
// unless it is kept on the list.
func TestTripHistory(t *testing.T) {
	d := DB(t)
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	// With dates: 30 days after the end. Without: 90 days after the last row
	// (a trip booked ahead has only its old prepayment tagged).
	if !ledger.TripDone("", "2026-09-01", true, now) || ledger.TripDone("", "2026-10-15", true, now) || ledger.TripDone("", "", true, now) {
		t.Error("automatic with dates: ended over 30 days ago")
	}
	if ledger.TripDone("", "2026-09-01", false, now) || !ledger.TripDone("", "2026-07-01", false, now) {
		t.Error("automatic without dates: last row over 90 days ago")
	}
	if err := ledger.SetTripState(d, "trip:egypt", true); err != nil {
		t.Fatal(err)
	}
	if err := ledger.SetTripState(d, "Trip:Rome", false); err != nil {
		t.Fatal(err)
	}
	if err := ledger.SetTripState(d, "egypt", true); err == nil {
		t.Error("a trip tag is trip:<name>")
	}
	st, _ := ledger.TripStates(d)
	if !ledger.TripDone(st["trip:egypt"], "2026-10-20", true, now) || ledger.TripDone(st["trip:rome"], "2025-01-01", true, now) {
		t.Errorf("by hand: %v", st)
	}
	// Brought back, then given its real dates: the automatic rule applies again.
	if err := ledger.SaveTripDates(d, ledger.TripDates{Tag: "trip:rome", Start: "2026-08-01", End: "2026-08-10"}); err != nil {
		t.Fatal(err)
	}
	if st, _ = ledger.TripStates(d); st["trip:rome"] != "" || st["trip:egypt"] != "done" {
		t.Errorf("after dates: %v", st)
	}
}

// On the 29th–31st, stepping months from today's date rolls over ("31
// September" = 1 October): last month read as this month and the Trends
// chart drew every 30-day month twice.
func TestAddMonthsCountsFromTheFirst(t *testing.T) {
	d := func(s string) time.Time { x, _ := time.Parse("2006-01-02", s); return x }
	for _, c := range []struct {
		from string
		n    int
		want string
	}{
		{"2026-10-31", -1, "2026-09"}, {"2026-03-31", -1, "2026-02"}, {"2026-01-31", 1, "2026-02"},
		{"2026-10-31", 0, "2026-10"}, {"2026-01-15", -1, "2025-12"}, {"2026-12-31", 1, "2027-01"}, {"2026-10-31", -23, "2024-11"},
	} {
		if got := ledger.AddMonths(d(c.from), c.n); got != c.want {
			t.Errorf("%s %+d → %s, want %s", c.from, c.n, got, c.want)
		}
	}
	seen := map[string]bool{}
	for i := 0; i < 24; i++ {
		m := ledger.AddMonths(d("2026-10-31"), -i)
		if seen[m] {
			t.Fatalf("%s twice in 24 months", m)
		}
		seen[m] = true
	}
}
