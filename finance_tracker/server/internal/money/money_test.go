package money

import (
	"encoding/json"
	"testing"
)

func TestCentsJSONRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Cents
		out  string
	}{
		{"12.34", 1234, "12.34"}, {"0.1", 10, "0.10"}, {"-5", -500, "-5.00"}, {`"7.5"`, 750, "7.50"}, {"null", 0, "0.00"},
		{"0.005", 1, "0.01"}, {"1361.66", 136166, "1361.66"},
	} {
		var c Cents
		if err := json.Unmarshal([]byte(tc.in), &c); err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if c != tc.want {
			t.Errorf("%s → %d, want %d", tc.in, c, tc.want)
		}
		b, _ := json.Marshal(c)
		if string(b) != tc.out {
			t.Errorf("%d marshals to %s, want %s", c, b, tc.out)
		}
	}
	var c Cents
	if err := json.Unmarshal([]byte(`"abc"`), &c); err == nil {
		t.Error("garbage must not parse")
	}
}

func TestFloatAndAbs(t *testing.T) {
	if FromFloat(0.1+0.2) != 30 {
		t.Error("float rounding")
	}
	if Cents(-250).Abs() != 250 || Cents(-250).Float() != -2.5 {
		t.Error("abs/float")
	}
}
