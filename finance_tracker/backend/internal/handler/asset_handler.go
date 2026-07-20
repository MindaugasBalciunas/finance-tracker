package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/service"
)

type AssetHandler struct {
	svc service.AssetService
}

func NewAssetHandler(svc service.AssetService) *AssetHandler {
	return &AssetHandler{svc: svc}
}

func (h *AssetHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/assets")
	g.POST("", h.Create)
	g.GET("", h.ListAll)
	g.GET("/summary", h.GetSummary)
	g.GET("/:id", h.GetByID)
	g.PUT("/:id", h.Update)
	g.DELETE("", h.DeleteAll)
	g.DELETE("/:id", h.Delete)
}

// Create godoc
// @Summary      Create an asset
// @Tags         assets
// @Accept       json
// @Produce      json
// @Param        input  body      service.CreateAssetInput  true  "Asset input"
// @Success      201    {object}  domain.Asset
// @Failure      400    {object}  ErrorResponse
// @Failure      500    {object}  ErrorResponse
// @Router       /assets [post]
func (h *AssetHandler) Create(c *gin.Context) {
	var input service.CreateAssetInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	a, err := h.svc.Create(input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, a)
}

// GetByID godoc
// @Summary      Get an asset by ID
// @Tags         assets
// @Produce      json
// @Param        id   path      int  true  "Asset ID"
// @Success      200  {object}  domain.Asset
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /assets/{id} [get]
func (h *AssetHandler) GetByID(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	a, err := h.svc.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "asset not found"})
		return
	}
	c.JSON(http.StatusOK, a)
}

// Update godoc
// @Summary      Update an asset (full replace)
// @Tags         assets
// @Accept       json
// @Produce      json
// @Param        id     path      int                       true  "Asset ID"
// @Param        input  body      service.CreateAssetInput  true  "Asset input"
// @Success      200    {object}  domain.Asset
// @Failure      400    {object}  ErrorResponse
// @Failure      404    {object}  ErrorResponse
// @Router       /assets/{id} [put]
func (h *AssetHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	var input service.CreateAssetInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	a, err := h.svc.Update(id, input)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, a)
}

// Delete godoc
// @Summary      Delete an asset
// @Tags         assets
// @Param        id  path  int  true  "Asset ID"
// @Success      204
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /assets/{id} [delete]
func (h *AssetHandler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	if err := h.svc.Delete(id); err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteAll godoc
// @Summary      Delete all assets
// @Tags         assets
// @Success      204
// @Failure      500  {object}  ErrorResponse
// @Router       /assets [delete]
func (h *AssetHandler) DeleteAll(c *gin.Context) {
	if err := h.svc.DeleteAll(); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// ListAll godoc
// @Summary      List all assets
// @Tags         assets
// @Produce      json
// @Success      200  {array}   domain.Asset
// @Failure      500  {object}  ErrorResponse
// @Router       /assets [get]
func (h *AssetHandler) ListAll(c *gin.Context) {
	assets, err := h.svc.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, assets)
}

// GetSummary godoc
// @Summary      Get aggregated asset summary (value, loans, equity)
// @Tags         assets
// @Produce      json
// @Success      200  {object}  domain.AssetSummary
// @Failure      500  {object}  ErrorResponse
// @Router       /assets/summary [get]
func (h *AssetHandler) GetSummary(c *gin.Context) {
	summary, err := h.svc.GetSummary()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, summary)
}
