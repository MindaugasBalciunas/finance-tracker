package dip

import (
	"sort"
	"time"

	"ft/internal/money"
)

// UsualMonthly is what usually goes into the brokers each month, for the
// plan's "every month, whatever happens" line when Investments' "Invested
// each month" is left empty: the median of the last three complete months'
// transfer.invest into brokerage accounts from outside them (a move
// between brokers, or money taken back out, isn't new money). Zero when
// fewer than two of those months had any.
func UsualMonthly(q queryRower, now time.Time) money.Cents {
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	from, to := first.AddDate(0, -3, 0).Format("2006-01-02"), first.Format("2006-01-02")
	rows, err := q.Query(`SELECT substr(t.date,1,7), SUM(t.amount) FROM transactions t
		JOIN accounts dst ON dst.id = t.to_account_id
		LEFT JOIN accounts src ON src.id = t.account_id
		WHERE t.kind = 'transfer' AND t.category = 'transfer.invest' AND dst.kind = 'brokerage'
		  AND COALESCE(src.kind, '') NOT IN ('brokerage', 'crypto', 'pension')
		  AND t.date >= ? AND t.date < ? AND t.pending = 0
		GROUP BY 1`, from, to)
	if err != nil {
		return 0
	}
	defer rows.Close()
	byMonth := map[string]money.Cents{}
	for rows.Next() {
		var m string
		var c money.Cents
		if rows.Scan(&m, &c) == nil {
			byMonth[m] = c
		}
	}
	if len(byMonth) < 2 {
		return 0
	}
	v := []money.Cents{}
	for i := 1; i <= 3; i++ { // the months with none count as zero
		v = append(v, byMonth[first.AddDate(0, -i, 0).Format("2006-01")])
	}
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	return v[1]
}
