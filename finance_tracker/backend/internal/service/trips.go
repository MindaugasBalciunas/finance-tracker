package service

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/mindaugas/finance-tracker/internal/domain"
)

// Trips group one holiday's transactions under a trip:… label, so a trip
// can be costed as a whole (total, per day, hotel vs flights vs food) and
// optionally budgeted. Category stays Vacation — the label only adds the
// "which trip" dimension the Vacation fund can't give.

// NamedAmount is one slice of a breakdown.
type NamedAmount struct {
	Name   string  `json:"name"`
	Amount float64 `json:"amount"`
}

type TripSummary struct {
	Label  string  `json:"label"` // trip:zakopane-2025
	Name   string  `json:"name"`  // zakopane-2025
	From   string  `json:"from"`
	To     string  `json:"to"`
	Days   int     `json:"days"`
	Count  int     `json:"count"`
	Total  float64 `json:"total"` // expenses − refunds
	PerDay float64 `json:"per_day"`
	// ByCategory/ByLabel break the total down; ByLabel skips the trip label
	// itself and the generic "vacation".
	ByCategory []NamedAmount `json:"by_category"`
	ByLabel    []NamedAmount `json:"by_label"`
	BudgetID   uint          `json:"budget_id,omitempty"`
	Budget     *float64      `json:"budget,omitempty"`
	Remaining  *float64      `json:"remaining,omitempty"`
}

// TripSuggestion is a run of untagged Vacation spending close together in
// time — probably one trip.
type TripSuggestion struct {
	From           string   `json:"from"`
	To             string   `json:"to"`
	Days           int      `json:"days"`
	Count          int      `json:"count"`
	Total          float64  `json:"total"`
	TxIDs          []uint   `json:"tx_ids"`
	SuggestedLabel string   `json:"suggested_label"`
	TopComments    []string `json:"top_comments"`
}

// tripGapDays splits suggestions: Vacation rows further apart than this are
// different trips (a booking paid weeks ahead becomes its own group, which
// the user can merge by giving both the same name).
const tripGapDays = 4

func tripLabels(tx domain.Transaction) []string {
	var out []string
	for _, l := range strings.Split(tx.Labels, ",") {
		if strings.HasPrefix(l, domain.TripLabelPrefix) && len(l) > len(domain.TripLabelPrefix) {
			out = append(out, l)
		}
	}
	return out
}

func isVacationTx(tx domain.Transaction) bool {
	return tx.Type == domain.TransactionTypeExpense &&
		(tx.Category == domain.CategoryVacation || tx.HasLabel("vacation"))
}

func sortedAmounts(m map[string]float64) []NamedAmount {
	out := make([]NamedAmount, 0, len(m))
	for k, v := range m {
		if math.Abs(v) >= 0.005 {
			out = append(out, NamedAmount{Name: k, Amount: math.Round(v*100) / 100})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Amount != out[j].Amount {
			return out[i].Amount > out[j].Amount
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// ComputeTrips summarises every trip:… label, newest trip first.
func ComputeTrips(txs []domain.Transaction, budgets []domain.Budget) []TripSummary {
	type acc struct {
		from, to time.Time
		count    int
		total    float64
		byCat    map[string]float64
		byLabel  map[string]float64
	}
	trips := map[string]*acc{}
	for _, tx := range txs {
		sign := 0.0
		switch tx.Type {
		case domain.TransactionTypeExpense:
			sign = 1
		case domain.TransactionTypeIncome:
			sign = -1 // a refund or a friend paying back their share
		default:
			continue
		}
		for _, label := range tripLabels(tx) {
			a := trips[label]
			if a == nil {
				a = &acc{from: tx.Date, to: tx.Date, byCat: map[string]float64{}, byLabel: map[string]float64{}}
				trips[label] = a
			}
			if tx.Date.Before(a.from) {
				a.from = tx.Date
			}
			if tx.Date.After(a.to) {
				a.to = tx.Date
			}
			a.count++
			a.total += sign * tx.Amount
			a.byCat[string(tx.Category)] += sign * tx.Amount
			for _, l := range strings.Split(tx.Labels, ",") {
				if l == "" || l == "vacation" || strings.HasPrefix(l, domain.TripLabelPrefix) {
					continue
				}
				a.byLabel[l] += sign * tx.Amount
			}
		}
	}
	budgetFor := map[string]domain.Budget{}
	for _, b := range budgets {
		if b.Kind == "trip" {
			for _, l := range splitBudgetLabels(b.Label) {
				budgetFor[l] = b
			}
		}
	}
	out := make([]TripSummary, 0, len(trips)+len(budgetFor))
	for label, a := range trips {
		days := int(a.to.Sub(a.from).Hours()/24) + 1
		s := TripSummary{
			Label: label, Name: strings.TrimPrefix(label, domain.TripLabelPrefix),
			From: a.from.Format("2006-01-02"), To: a.to.Format("2006-01-02"),
			Days: days, Count: a.count, Total: math.Round(a.total*100) / 100,
			PerDay:     math.Round(a.total/float64(days)*100) / 100,
			ByCategory: sortedAmounts(a.byCat), ByLabel: sortedAmounts(a.byLabel),
		}
		if b, ok := budgetFor[label]; ok {
			amount, rem := b.Amount, b.Amount-a.total
			s.BudgetID, s.Budget, s.Remaining = b.ID, &amount, &rem
			delete(budgetFor, label)
		}
		out = append(out, s)
	}
	// A budgeted trip nothing is tagged with yet (planned, not taken).
	for label, b := range budgetFor {
		amount := b.Amount
		rem := amount
		out = append(out, TripSummary{Label: label, Name: strings.TrimPrefix(label, domain.TripLabelPrefix),
			ByCategory: []NamedAmount{}, ByLabel: []NamedAmount{}, BudgetID: b.ID, Budget: &amount, Remaining: &rem})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From > out[j].From || out[j].From == "" && out[i].From != ""
		}
		return out[i].Label < out[j].Label
	})
	// Planned trips (no dates yet) first: they're the ones being saved for.
	sort.SliceStable(out, func(i, j int) bool { return out[i].From == "" && out[j].From != "" })
	return out
}

// SuggestTrips clusters untagged Vacation spending into likely trips,
// newest first. Clusters under €30 are noise (a parking ticket), not trips.
func SuggestTrips(txs []domain.Transaction, existing []TripSummary) []TripSuggestion {
	var pool []domain.Transaction
	for _, tx := range txs {
		if isVacationTx(tx) && len(tripLabels(tx)) == 0 {
			pool = append(pool, tx)
		}
	}
	sort.SliceStable(pool, func(i, j int) bool { return pool[i].Date.Before(pool[j].Date) })

	taken := map[string]bool{}
	for _, t := range existing {
		taken[t.Label] = true
	}
	var out []TripSuggestion
	flush := func(group []domain.Transaction) {
		if len(group) == 0 {
			return
		}
		var total float64
		ids := make([]uint, 0, len(group))
		for _, tx := range group {
			total += tx.Amount
			ids = append(ids, tx.ID)
		}
		if total < 30 {
			return
		}
		from, to := group[0].Date, group[len(group)-1].Date
		byAmount := append([]domain.Transaction(nil), group...)
		sort.SliceStable(byAmount, func(i, j int) bool { return byAmount[i].Amount > byAmount[j].Amount })
		var top []string
		seen := map[string]bool{}
		for _, tx := range byAmount {
			c := strings.TrimSpace(tx.Comment)
			if c == "" || seen[c] {
				continue
			}
			seen[c] = true
			top = append(top, c)
			if len(top) == 3 {
				break
			}
		}
		label := domain.TripLabelPrefix + from.Format("2006-01")
		for n := 2; taken[label]; n++ {
			label = fmt.Sprintf("%s%s-%d", domain.TripLabelPrefix, from.Format("2006-01"), n)
		}
		taken[label] = true
		out = append(out, TripSuggestion{
			From: from.Format("2006-01-02"), To: to.Format("2006-01-02"),
			Days: int(to.Sub(from).Hours()/24) + 1, Count: len(group),
			Total: math.Round(total*100) / 100, TxIDs: ids, SuggestedLabel: label, TopComments: top,
		})
	}
	var group []domain.Transaction
	for _, tx := range pool {
		if len(group) > 0 && tx.Date.Sub(group[len(group)-1].Date) > tripGapDays*24*time.Hour {
			flush(group)
			group = nil
		}
		group = append(group, tx)
	}
	flush(group)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// NormalizeTripLabel turns a user-typed name into a trip label:
// "Zakopane 2025" → "trip:zakopane-2025". Commas would split the label.
func NormalizeTripLabel(name string) (string, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.TrimPrefix(n, domain.TripLabelPrefix)
	n = strings.Join(strings.Fields(strings.ReplaceAll(n, ",", " ")), "-")
	if n == "" {
		return "", errors.New("trip name is required")
	}
	if len(n) > 60 {
		return "", errors.New("trip name is too long (60 characters max)")
	}
	return domain.TripLabelPrefix + n, nil
}

func (s *insightService) Trips() ([]TripSummary, []TripSuggestion, error) {
	if s.budgetRepo == nil {
		return nil, nil, errors.New("budgets unavailable")
	}
	txs, err := s.txSvc.ListAll()
	if err != nil {
		return nil, nil, err
	}
	budgets, err := s.budgetRepo.ListBudgets()
	if err != nil {
		return nil, nil, err
	}
	trips := ComputeTrips(txs, budgets)
	return trips, SuggestTrips(txs, trips), nil
}

// AssignTrip tags (or untags) exactly the given transactions with a trip.
func (s *insightService) AssignTrip(name string, ids []uint, remove bool) (string, int, error) {
	if s.budgetRepo == nil {
		return "", 0, errors.New("budgets unavailable")
	}
	label, err := NormalizeTripLabel(name)
	if err != nil {
		return "", 0, err
	}
	n, err := s.budgetRepo.SetLabelOnTransactions(label, ids, remove)
	return label, n, err
}

// RenameTrip renames a trip everywhere: its transactions' label and its
// trip budget. Renaming onto an existing trip merges the two (a deposit
// paid weeks ahead joining its holiday); when both had a budget the
// amounts are added and one line remains.
func (s *insightService) RenameTrip(from, toName string) (string, domain.RelabelResult, error) {
	if s.budgetRepo == nil {
		return "", domain.RelabelResult{}, errors.New("budgets unavailable")
	}
	from = strings.ToLower(strings.TrimSpace(from))
	if !strings.HasPrefix(from, domain.TripLabelPrefix) {
		return "", domain.RelabelResult{}, errors.New("not a trip label")
	}
	to, err := NormalizeTripLabel(toName)
	if err != nil {
		return "", domain.RelabelResult{}, err
	}
	if to == from {
		return to, domain.RelabelResult{}, nil
	}
	budgets, err := s.budgetRepo.ListBudgets()
	if err != nil {
		return "", domain.RelabelResult{}, err
	}
	var src, dst *domain.Budget
	for i := range budgets {
		b := &budgets[i]
		if b.Kind != "trip" {
			continue
		}
		switch b.Label {
		case from:
			src = b
		case to:
			dst = b
		}
	}
	res, err := s.budgetRepo.RenameLabel(from, to)
	if err != nil {
		return "", res, err
	}
	if res.Transactions == 0 && src == nil {
		return "", res, fmt.Errorf("no trip named %q", strings.TrimPrefix(from, domain.TripLabelPrefix))
	}
	name := strings.TrimPrefix(to, domain.TripLabelPrefix)
	switch {
	case src != nil && dst != nil:
		dst.Amount += src.Amount
		if err := s.budgetRepo.SaveBudget(dst); err != nil {
			return "", res, err
		}
		if err := s.budgetRepo.DeleteBudget(src.ID); err != nil {
			return "", res, err
		}
	case src != nil:
		src.Name = name
		src.Label = to
		if err := s.budgetRepo.SaveBudget(src); err != nil {
			return "", res, err
		}
	}
	return to, res, nil
}
