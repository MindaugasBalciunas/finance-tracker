package service

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

// Auto AI review of the view the user is currently looking at: a compact,
// view-specific slice of data goes to the gateway once, and the short blurb
// is cached server-side so tab-hopping (or a second device) doesn't trigger
// another paid call for 15 minutes.

const viewSummaryTTL = 15 * time.Minute

type viewCacheEntry struct {
	text    string
	expires time.Time
}

var viewCache = struct {
	sync.Mutex
	m map[string]viewCacheEntry
}{m: map[string]viewCacheEntry{}}

var validViews = map[string]bool{
	"dashboard": true, "transactions": true, "balances": true, "stocks": true,
	"assets": true, "budget": true, "reports": true, "labels": true,
}

// ViewSummary returns a 2-4 sentence AI review of one app view for the
// selected period. refresh busts the cache.
func (s *insightService) ViewSummary(view string, from, to *time.Time, refresh bool) (string, error) {
	if !validViews[view] {
		return "", fmt.Errorf("unknown view %q", view)
	}
	settings, err := s.repo.GetAISettings()
	if err != nil {
		return "", err
	}
	if !settings.Configured() {
		return "", errors.New("AI gateway not configured")
	}

	key := view + "|" + periodLabel(from, to)
	if !refresh {
		viewCache.Lock()
		e, ok := viewCache.m[key]
		viewCache.Unlock()
		if ok && time.Now().Before(e.expires) {
			return e.text, nil
		}
	}

	context, err := s.viewContext(view, from, to)
	if err != nil {
		return "", fmt.Errorf("building view context: %w", err)
	}

	prompt := fmt.Sprintf(`You are a personal finance assistant. Below is the data behind the "%s" view the user has open right now (period: %s). In 2-4 short sentences, point out the most interesting, unusual or actionable things in THIS view. Quote concrete EUR figures. Plain text only — no headers, no bullet points, no preamble, no restating what the view is.

%s`, view, periodLabel(from, to), context)

	text, err := callGateway(settings, []domain.ChatMessage{{Role: "user", Content: prompt}}, 2048)
	if err != nil {
		return "", err
	}

	viewCache.Lock()
	viewCache.m[key] = viewCacheEntry{text: text, expires: time.Now().Add(viewSummaryTTL)}
	viewCache.Unlock()
	return text, nil
}

// viewContext assembles the compact data slice for one view — targeted, so
// the model reviews what's actually on screen rather than the whole report.
func (s *insightService) viewContext(view string, from, to *time.Time) (string, error) {
	now := time.Now()
	switch view {
	case "dashboard":
		latest, err := s.balSvc.GetLatest(0)
		if err != nil {
			return "", err
		}
		allTxs, err := s.txSvc.ListAll()
		if err != nil {
			return "", err
		}
		parts := []string{s.snapshotLines(latest), s.currentMonthSection(allTxs, now)}
		summary, err := s.txSvc.GetSummary(domain.TransactionFilter{})
		if err == nil {
			if sec := s.budgetStatusSection(allTxs, summary, now); sec != "" {
				parts = append(parts, sec)
			}
		}
		return strings.Join(parts, "\n\n"), nil

	case "transactions", "reports":
		summary, err := s.txSvc.GetSummary(domain.TransactionFilter{DateFrom: from, DateTo: to})
		if err != nil {
			return "", err
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Income €%.0f · Expenses €%.0f · Invested €%.0f · Net €%.0f\n",
			summary.TotalIncome, summary.TotalExpenses, summary.TotalInvestments,
			summary.TotalIncome-summary.TotalExpenses-summary.TotalInvestments)
		b.WriteString("Top expense categories:\n")
		count := 0
		for _, c := range summary.ByCategory {
			if c.Type == domain.TransactionTypeExpense && count < 8 {
				fmt.Fprintf(&b, "  - %s: €%.0f (%d tx)\n", c.Category, c.Total, c.Count)
				count++
			}
		}
		months := summary.ByMonth
		if len(months) > 12 {
			months = months[len(months)-12:]
		}
		b.WriteString("Monthly cash flow:\n")
		for _, m := range months {
			fmt.Fprintf(&b, "  - %04d-%02d: out €%.0f, in €%.0f, invested €%.0f\n", m.Year, m.Month, m.Expenses, m.Income, m.Investments)
		}
		if view == "transactions" {
			expense := domain.TransactionTypeExpense
			page, err := s.txSvc.List(domain.TransactionFilter{
				DateFrom: from, DateTo: to, Type: &expense,
				Sort: "amount", Dir: "desc", Page: 1, PageSize: 5,
			})
			if err == nil && len(page.Data) > 0 {
				b.WriteString("Largest expenses in the period:\n")
				for _, tx := range page.Data {
					fmt.Fprintf(&b, "  - %s €%.0f %s (%s)\n", tx.Date.Format("2006-01-02"), tx.Amount, tx.Comment, tx.Category)
				}
			}
		}
		return b.String(), nil

	case "balances":
		balances, err := s.balSvc.List(domain.BalanceFilter{DateFrom: from, DateTo: to}, 0)
		if err != nil {
			return "", err
		}
		if len(balances) == 0 {
			return "No balance snapshots in the period.", nil
		}
		sort.Slice(balances, func(i, j int) bool { return balances[i].Date.Before(balances[j].Date) })
		first, last := balances[0], balances[len(balances)-1]
		var b strings.Builder
		fmt.Fprintf(&b, "Snapshots in period: %d\nStart (%s): total €%.0f\nEnd (%s): total €%.0f (change %+.0f)\n\n",
			len(balances), first.Date.Format("2006-01-02"), first.Total,
			last.Date.Format("2006-01-02"), last.Total, last.Total-first.Total)
		b.WriteString(s.snapshotLines(&last))
		return b.String(), nil

	case "stocks":
		latest, _ := s.balSvc.GetLatest(0)
		sec := s.stockPositionsSection(latest)
		if sec == "" {
			return "No stock positions recorded.", nil
		}
		return sec, nil

	case "assets":
		if s.assetSvc == nil {
			return "", errors.New("assets unavailable")
		}
		assets, err := s.assetSvc.ListAll()
		if err != nil {
			return "", err
		}
		summary, err := s.assetSvc.GetSummary()
		if err != nil {
			return "", err
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Assets: %d · total value €%.0f · loans €%.0f · net equity €%.0f\n",
			summary.Count, summary.TotalValue, summary.TotalLoans, summary.NetEquity)
		for _, a := range assets {
			fmt.Fprintf(&b, "  - %s (%s): value €%.0f, loan €%.0f, equity €%.0f", a.Name, a.Type, a.CurrentValue, a.LoanRemaining, a.CurrentValue-a.LoanRemaining)
			if a.LoanMargin > 0 || a.LoanBaseRate > 0 {
				fmt.Fprintf(&b, ", rate %.2f%%", a.LoanMargin+a.LoanBaseRate)
			}
			if a.LoanRateResetDate != nil {
				fmt.Fprintf(&b, ", rate resets %s", a.LoanRateResetDate.Format("2006-01-02"))
			}
			b.WriteString("\n")
		}
		return b.String(), nil

	case "budget":
		allTxs, err := s.txSvc.ListAll()
		if err != nil {
			return "", err
		}
		summary, err := s.txSvc.GetSummary(domain.TransactionFilter{})
		if err != nil {
			return "", err
		}
		parts := []string{s.currentMonthSection(allTxs, now)}
		if sec := s.budgetStatusSection(allTxs, summary, now); sec != "" {
			parts = append(parts, sec)
		}
		return strings.Join(parts, "\n\n"), nil

	case "labels":
		if s.budgetRepo == nil {
			return "", errors.New("labels unavailable")
		}
		stats, err := s.budgetRepo.LabelStats()
		if err != nil {
			return "", err
		}
		if len(stats) > 15 {
			stats = stats[:15]
		}
		var b strings.Builder
		b.WriteString("Top labels by transaction count:\n")
		for _, st := range stats {
			fmt.Fprintf(&b, "  - %s: %d tx, €%.0f volume, last used %s\n", st.Label, st.Transactions, st.Amount, st.LastUsed)
		}
		return b.String(), nil
	}
	return "", fmt.Errorf("unknown view %q", view)
}

// snapshotLines is the compact balance-snapshot block shared by views.
func (s *insightService) snapshotLines(latest *domain.Balance) string {
	if latest == nil {
		return "No balance snapshot available."
	}
	freeCash := latest.Seb + latest.Swed + latest.Luminor + latest.Cash + latest.RevM + latest.RevR
	investments := latest.SwedETF + latest.RevStocks + latest.IBKRStocks
	pensions := latest.SebPen + latest.Art
	cryptoEur := latest.RBTC + latest.MBTC
	if latest.BtcPrice > 0 {
		cryptoEur = (latest.RBTC + latest.MBTC) * latest.BtcPrice
	}
	return fmt.Sprintf("Balance snapshot (%s): net worth €%.0f — free cash €%.0f, investments €%.0f, pensions €%.0f, crypto €%.0f",
		latest.Date.Format("2006-01-02"), latest.Total, freeCash, investments, pensions, cryptoEur)
}