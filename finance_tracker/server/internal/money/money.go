// Package money holds the one numeric type the app uses for EUR amounts.
package money

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Cents is an amount of EUR in cents. On the wire it is a decimal number of
// euros (12.34), so the frontend never deals in cents.
type Cents int64

func FromFloat(eur float64) Cents { return Cents(math.Round(eur * 100)) }

func (c Cents) Float() float64 { return float64(c) / 100 }

func (c Cents) String() string {
	sign := ""
	v := int64(c)
	if v < 0 {
		sign, v = "-", -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/100, v%100)
}

func (c Cents) MarshalJSON() ([]byte, error) { return []byte(c.String()), nil }

func (c *Cents) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*c = 0
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("invalid amount %q", s)
	}
	*c = FromFloat(f)
	return nil
}

// Abs returns |c|.
func (c Cents) Abs() Cents {
	if c < 0 {
		return -c
	}
	return c
}

var _ json.Marshaler = Cents(0)
