package ai

import "testing"

func TestUncheckedAmountsFlagsFiguresNotInTheData(t *testing.T) {
	data := []string{`{"rows":[{"amount":42,"merchant":"360arena"},{"amount":35.7}],"total":116.7}`, `{"saved":664.2}`}
	reply := "360 Arena: **€42.00**, €35.70 and €39 — total €116.70. You saved €664/month. There is a €1,331 reimbursement and 1 331,50 € more; about €18.4k invested."
	got := uncheckedAmounts(reply, data)
	if len(got) != 2 || got[0] != "€1,331" || got[1] != "1 331,50 €" {
		t.Fatalf("got %q", got)
	}
	if uncheckedAmounts("Spend €1,331 less", nil) != nil {
		t.Fatal("no tools ran: nothing to check against")
	}
}
