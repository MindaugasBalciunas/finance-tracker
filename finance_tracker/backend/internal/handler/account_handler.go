package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/domain"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"gorm.io/gorm"
)

// AccountHandler manages balance-sheet accounts. Built-in ones can be
// renamed; added ones can also be regrouped and archived. Nothing is ever
// deleted — an account's history lives in every snapshot that held it.
type AccountHandler struct {
	repo repository.AccountRepository
}

func NewAccountHandler(repo repository.AccountRepository) *AccountHandler {
	return &AccountHandler{repo: repo}
}

func (h *AccountHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/accounts")
	g.GET("", h.List)
	g.POST("", h.Create)
	g.PUT("/:id", h.Update)
}

func (h *AccountHandler) List(c *gin.Context) {
	accs, err := h.repo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	if accs == nil {
		accs = []domain.Account{}
	}
	c.JSON(http.StatusOK, accs)
}

type accountInput struct {
	Label     string              `json:"label"`
	Group     domain.AccountGroup `json:"group"`
	Archived  *bool               `json:"archived"`
	SortOrder *int                `json:"sort_order"`
}

func (h *AccountHandler) Create(c *gin.Context) {
	var in accountInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	in.Label = strings.TrimSpace(in.Label)
	if in.Label == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "give the account a name"})
		return
	}
	if !domain.IsValidAccountGroup(in.Group) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("unknown group %q", in.Group)})
		return
	}
	// The key is derived from the name and never changes after, so a rename
	// does not orphan the values already stored under it.
	base := domain.CustomAccountKey(in.Label)
	key := base
	for n := 2; ; n++ {
		_, err := h.repo.GetByKey(key)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			break
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
		key = fmt.Sprintf("%s_%d", base, n)
	}
	a := &domain.Account{Key: key, Label: in.Label, Group: in.Group, SortOrder: 100}
	if in.SortOrder != nil {
		a.SortOrder = *in.SortOrder
	}
	if err := h.repo.Create(a); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, a)
}

func (h *AccountHandler) Update(c *gin.Context) {
	id, err := uintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	a, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "account not found"})
		return
	}
	var in accountInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	// A built-in account can be renamed, nothing more: its group is wired
	// into the net-worth cards and its column can never be hidden.
	if a.Builtin && ((in.Group != "" && in.Group != a.Group) || in.Archived != nil) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "a built-in account can only be renamed"})
		return
	}
	if l := strings.TrimSpace(in.Label); l != "" {
		a.Label = l
	}
	if in.Group != "" {
		if !domain.IsValidAccountGroup(in.Group) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("unknown group %q", in.Group)})
			return
		}
		a.Group = in.Group
	}
	if in.Archived != nil {
		a.Archived = *in.Archived
	}
	if in.SortOrder != nil {
		a.SortOrder = *in.SortOrder
	}
	if err := h.repo.Save(a); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, a)
}
