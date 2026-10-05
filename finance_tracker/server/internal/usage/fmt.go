package usage

import "strconv"

func fmtFloat(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }
func itoa(n int) string         { return strconv.Itoa(n) }
