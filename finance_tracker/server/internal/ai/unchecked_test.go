package ai

import (
	"reflect"
	"testing"
)

func TestUncheckedAmountsFlagsFiguresNotInTheData(t *testing.T) {
	data := []string{`{"rows":[{"amount":42,"merchant":"360arena"},{"amount":35.7}],"total":116.7}`, `{"saved":664.2}`}
	reply := "360 Arena: **€42.00**, €35.70 and €39 — total €116.70. You saved €664/month. There is a €1,331 reimbursement and 1 331,50 € more; about €18.4k invested."
	got := uncheckedAmounts(reply, data, nil)
	if len(got) != 2 || got[0] != "€1,331" || got[1] != "1 331,50 €" {
		t.Fatalf("got %q", got)
	}
	if uncheckedAmounts("Spend €1,331 less", nil, nil) != nil {
		t.Fatal("no tools ran: nothing to check against")
	}
}

// What the box should not call "not found": sums and differences of figures
// it read, figures the owner typed, and figures in a sentence that labels
// itself an estimate or the assistant's own calculation. What it still flags
// is a figure nothing accounts for.
func TestUncheckedAmountsLeavesHonestFiguresAlone(t *testing.T) {
	data := []string{`{"etf":1000,"artea":200,"principal":500,"salary":5049.2,"swed_end":316}`}
	said := []string{"I'm moving to Vialet at 10,000 gross; SEB gets 1 400 on the 16th."}
	reply := "Your core is €1,000 + €200, so €1,200 a month goes out first, plus €1,700 with principal. " +
		"At €10,000 gross, net is probably around €5,900. " +
		"With the IBKR transfer Swedbank ends at about −€684 (my calculation). " +
		"SEB gets €1,400 on the 16th. " +
		"You also paid €2,250 to Tokvila."
	got := uncheckedAmounts(reply, data, said)
	if !reflect.DeepEqual(got, []string{"€2,250"}) {
		t.Fatalf("got %q, want only the figure nothing accounts for", got)
	}
}
