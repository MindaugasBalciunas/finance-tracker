package handler

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"gorm.io/gorm"
)

// SplitHandler splits one transaction into parts and tracks money other
// people owe. A part is an ordinary transaction row, so every report, budget
// and insight sees the right categories without knowing splits exist.
//
// Balances never move here: the original already moved them once, and the
// parts only re-describe that same money.
type SplitHandler struct {
	db *gorm.DB
}

func NewSplitHandler(db *gorm.DB) *SplitHandler { return &SplitHandler{db: db} }

func (h *SplitHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/transactions")
	g.GET("/owed", h.Owed)
	g.POST("/:id/split", h.Split)
	g.POST("/:id/unsplit", h.Unsplit)
	g.POST("/:id/repayment", h.Repayment)
}

type splitPartInput struct {
	Amount   float64 `json:"amount"`
	Category string  `json:"category"`
	Labels   string  `json:"labels"`
	Comment  string  `json:"comment"`
	// OwedBy files the part as money this person owes you.
	OwedBy string `json:"owed_by"`
}

func cents(v float64) int64 { return int64(math.Round(v * 100)) }

// owedLabels adds the owed pair for a person to a label string.
func owedLabels(labels, person string) (string, error) {
	pl := domain.OwedPersonLabel(person)
	if pl == "" {
		return "", errors.New("name the person who owes it")
	}
	return domain.NormalizeLabels(labels + "," + domain.OwedLabel + "," + pl), nil
}

func (h *SplitHandler) Split(c *gin.Context) {
	id, err := uintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	var in struct {
		Parts []splitPartInput `json:"parts"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	var created []domain.Transaction
	status := http.StatusBadRequest
	err = h.db.Transaction(func(db *gorm.DB) error {
		repo := repository.NewTransactionRepository(db)
		parent, err := repo.GetByID(id)
		if err != nil {
			status = http.StatusNotFound
			return errors.New("transaction not found")
		}
		if parent.SplitOf != 0 {
			return errors.New("this is already a part of a split — undo that split first")
		}
		var n int64
		if err := db.Model(&domain.Transaction{}).Where("split_of = ?", parent.ID).Count(&n).Error; err != nil {
			status = http.StatusInternalServerError
			return err
		}
		if n > 0 {
			return errors.New("this transaction is already split — undo the split to change it")
		}
		if len(in.Parts) == 0 || (len(in.Parts) == 1 && strings.TrimSpace(in.Parts[0].OwedBy) == "") {
			return errors.New("a split needs at least two parts, or one part someone owes you")
		}
		var sum int64
		for _, p := range in.Parts {
			if cents(p.Amount) <= 0 {
				return errors.New("every part needs an amount above zero")
			}
			sum += cents(p.Amount)
		}
		if sum != cents(parent.Amount) {
			return fmt.Errorf("the parts add up to %.2f but the transaction is %.2f", float64(sum)/100, parent.Amount)
		}

		parts := make([]domain.Transaction, len(in.Parts))
		for i, p := range in.Parts {
			t := domain.Transaction{
				Date: parent.Date, Type: parent.Type, Amount: float64(cents(p.Amount)) / 100,
				Category: parent.Category, Labels: domain.NormalizeLabels(p.Labels), Comment: parent.Comment,
				DebitAccount: parent.DebitAccount, CreditAccount: parent.CreditAccount, SourceAccount: parent.SourceAccount,
			}
			if c := strings.TrimSpace(p.Comment); c != "" && i > 0 {
				t.Comment = c
			}
			if strings.TrimSpace(p.OwedBy) != "" {
				t.Category = domain.CategoryTransfers
				if t.Labels, err = owedLabels(t.Labels, p.OwedBy); err != nil {
					return err
				}
			} else if p.Category != "" {
				if !domain.IsValidCategory(domain.Category(p.Category)) {
					return fmt.Errorf("unknown category %q", p.Category)
				}
				t.Category = domain.Category(p.Category)
			}
			parts[i] = t
		}

		// Part one stays the original row — its id, bank id and comment are
		// what dedup and the bank queue know it by.
		parent.Amount = parts[0].Amount
		parent.Category = parts[0].Category
		parent.Labels = parts[0].Labels
		if err := repo.Update(parent); err != nil {
			status = http.StatusInternalServerError
			return err
		}
		created = append(created, *parent)
		for i := 1; i < len(parts); i++ {
			parts[i].SplitOf = parent.ID
			if err := repo.Create(&parts[i]); err != nil {
				status = http.StatusInternalServerError
				return err
			}
			created = append(created, parts[i])
		}
		return nil
	})
	if err != nil {
		c.JSON(status, ErrorResponse{Error: err.Error()})
		return
	}
	for i := range created {
		created[i].AmountMoney = domain.Money{Value: created[i].Amount, Currency: domain.CurrencyEUR}
	}
	c.JSON(http.StatusOK, gin.H{"parts": created})
}

// Unsplit folds every part back into the original. The original keeps part
// one's category and labels; the user re-files it if that is not right.
func (h *SplitHandler) Unsplit(c *gin.Context) {
	id, err := uintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	var out *domain.Transaction
	status := http.StatusBadRequest
	err = h.db.Transaction(func(db *gorm.DB) error {
		repo := repository.NewTransactionRepository(db)
		t, err := repo.GetByID(id)
		if err != nil {
			status = http.StatusNotFound
			return errors.New("transaction not found")
		}
		if t.SplitOf != 0 {
			if t, err = repo.GetByID(t.SplitOf); err != nil {
				status = http.StatusNotFound
				return errors.New("the original of this split is gone")
			}
		}
		var parts []domain.Transaction
		if err := db.Where("split_of = ?", t.ID).Find(&parts).Error; err != nil {
			status = http.StatusInternalServerError
			return err
		}
		if len(parts) == 0 {
			return errors.New("this transaction is not split")
		}
		total := cents(t.Amount)
		for _, p := range parts {
			total += cents(p.Amount)
			if err := repo.Delete(p.ID); err != nil {
				status = http.StatusInternalServerError
				return err
			}
		}
		t.Amount = float64(total) / 100
		if err := repo.Update(t); err != nil {
			status = http.StatusInternalServerError
			return err
		}
		out = t
		return nil
	})
	if err != nil {
		c.JSON(status, ErrorResponse{Error: err.Error()})
		return
	}
	out.AmountMoney = domain.Money{Value: out.Amount, Currency: domain.CurrencyEUR}
	c.JSON(http.StatusOK, out)
}

// Repayment marks an incoming payment as someone paying back what they owe,
// so it settles their balance instead of counting as income.
func (h *SplitHandler) Repayment(c *gin.Context) {
	id, err := uintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	var in struct {
		Person string `json:"person"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	repo := repository.NewTransactionRepository(h.db)
	t, err := repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "transaction not found"})
		return
	}
	if t.Type != domain.TransactionTypeIncome {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "only money coming in can be a repayment"})
		return
	}
	if t.OwedPerson() != "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "this payment is already marked as a repayment"})
		return
	}
	labels, err := owedLabels(t.Labels, in.Person)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	t.Labels = labels
	t.Category = domain.CategoryTransfers
	if err := repo.Update(t); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	t.AmountMoney = domain.Money{Value: t.Amount, Currency: domain.CurrencyEUR}
	c.JSON(http.StatusOK, t)
}

type owedRow struct {
	ID      uint    `json:"id"`
	Date    string  `json:"date"`
	Type    string  `json:"type"`
	Amount  float64 `json:"amount"`
	Comment string  `json:"comment"`
}

type owedPerson struct {
	Person      string    `json:"person"`
	Name        string    `json:"name"`
	Lent        float64   `json:"lent"`
	Repaid      float64   `json:"repaid"`
	Outstanding float64   `json:"outstanding"`
	Rows        []owedRow `json:"rows"`
	last        time.Time
}

// Owed lists, per person, what you fronted for them and what came back.
func (h *SplitHandler) Owed(c *gin.Context) {
	var txs []domain.Transaction
	if err := h.db.Where("labels LIKE ?", "%"+domain.OwedLabelPrefix+"%").Order("date ASC, id ASC").Find(&txs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	byPerson := map[string]*owedPerson{}
	for i := range txs {
		t := &txs[i]
		p := t.OwedPerson()
		if p == "" {
			continue
		}
		o := byPerson[p]
		if o == nil {
			o = &owedPerson{Person: p, Name: displayName(p), Rows: []owedRow{}}
			byPerson[p] = o
		}
		switch t.Type {
		case domain.TransactionTypeIncome:
			o.Repaid += t.Amount
		default:
			o.Lent += t.Amount
		}
		o.Rows = append(o.Rows, owedRow{ID: t.ID, Date: t.Date.Format("2006-01-02"), Type: string(t.Type), Amount: t.Amount, Comment: t.Comment})
		o.last = t.Date
	}
	out := make([]owedPerson, 0, len(byPerson))
	for _, o := range byPerson {
		o.Lent = roundTo2(o.Lent)
		o.Repaid = roundTo2(o.Repaid)
		o.Outstanding = roundTo2(o.Lent - o.Repaid)
		out = append(out, *o)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Outstanding != out[j].Outstanding {
			return out[i].Outstanding > out[j].Outstanding
		}
		return out[i].last.After(out[j].last)
	})
	c.JSON(http.StatusOK, out)
}

func roundTo2(v float64) float64 { return math.Round(v*100) / 100 }

// displayName turns a label slug back into something readable: "tomas-k" →
// "Tomas K". Diacritics were folded away when the label was made.
func displayName(slug string) string {
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
