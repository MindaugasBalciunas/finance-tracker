package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/service"
)

type StockHandler struct {
	svc service.StockService
}

func NewStockHandler(svc service.StockService) *StockHandler {
	return &StockHandler{svc: svc}
}

func (h *StockHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/stocks")
	g.POST("", h.Create)
	g.GET("", h.ListAll)
	g.GET("/portfolio", h.GetPortfolio)
	g.GET("/price/:ticker", h.GetPrice)
	g.GET("/history/:ticker", h.GetHistory)
	g.GET("/:id", h.GetByID)
	g.PUT("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
}

// Create godoc
// @Summary      Create a stock trade
// @Tags         stocks
// @Accept       json
// @Produce      json
// @Param        input  body      service.CreateStockTradeInput  true  "Trade input"
// @Success      201    {object}  domain.StockTrade
// @Failure      400    {object}  ErrorResponse
// @Failure      500    {object}  ErrorResponse
// @Router       /stocks [post]
func (h *StockHandler) Create(c *gin.Context) {
	var input service.CreateStockTradeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	t, err := h.svc.Create(input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, t)
}

// GetByID godoc
// @Summary      Get a stock trade by ID
// @Tags         stocks
// @Produce      json
// @Param        id   path      int  true  "Trade ID"
// @Success      200  {object}  domain.StockTrade
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /stocks/{id} [get]
func (h *StockHandler) GetByID(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	t, err := h.svc.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "trade not found"})
		return
	}
	c.JSON(http.StatusOK, t)
}

// Update godoc
// @Summary      Update a stock trade
// @Tags         stocks
// @Accept       json
// @Produce      json
// @Param        id     path      int                            true  "Trade ID"
// @Param        input  body      service.UpdateStockTradeInput  true  "Update input"
// @Success      200    {object}  domain.StockTrade
// @Failure      400    {object}  ErrorResponse
// @Failure      404    {object}  ErrorResponse
// @Router       /stocks/{id} [put]
func (h *StockHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}
	var input service.UpdateStockTradeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	t, err := h.svc.Update(id, input)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

// Delete godoc
// @Summary      Delete a stock trade
// @Tags         stocks
// @Param        id  path  int  true  "Trade ID"
// @Success      204
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /stocks/{id} [delete]
func (h *StockHandler) Delete(c *gin.Context) {
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

// ListAll godoc
// @Summary      List all stock trades
// @Tags         stocks
// @Produce      json
// @Success      200  {array}   domain.StockTrade
// @Failure      500  {object}  ErrorResponse
// @Router       /stocks [get]
func (h *StockHandler) ListAll(c *gin.Context) {
	trades, err := h.svc.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, trades)
}

// GetPortfolio godoc
// @Summary      Get computed stock portfolio
// @Tags         stocks
// @Produce      json
// @Success      200  {object}  domain.StockPortfolio
// @Failure      500  {object}  ErrorResponse
// @Router       /stocks/portfolio [get]
func (h *StockHandler) GetPortfolio(c *gin.Context) {
	portfolio, err := h.svc.GetPortfolio()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, portfolio)
}

// GetPrice godoc
// @Summary      Get current stock price from Yahoo Finance
// @Tags         stocks
// @Produce      json
// @Param        ticker  path      string  true  "Ticker symbol"
// @Success      200     {object}  map[string]interface{}
// @Failure      500     {object}  ErrorResponse
// @Router       /stocks/price/{ticker} [get]
func (h *StockHandler) GetPrice(c *gin.Context) {
	ticker := strings.ToUpper(c.Param("ticker"))
	price, err := fetchYahooPrice(ticker)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("could not fetch price for %s: %v", ticker, err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ticker": ticker, "price": price})
}

// GetHistory godoc
// @Summary      Get price history from Yahoo Finance
// @Tags         stocks
// @Produce      json
// @Param        ticker  path      string  true  "Ticker symbol"
// @Param        range   query     string  false "Range: 1mo 3mo 6mo 1y 2y 5y (default 1y)"
// @Success      200     {object}  map[string]interface{}
// @Failure      500     {object}  ErrorResponse
// @Router       /stocks/history/{ticker} [get]
func (h *StockHandler) GetHistory(c *gin.Context) {
	ticker := strings.ToUpper(c.Param("ticker"))
	rangeParam := c.DefaultQuery("range", "1y")
	points, err := fetchYahooHistory(ticker, rangeParam)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("could not fetch history for %s: %v", ticker, err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ticker": ticker, "points": points})
}

// fetchYahooPrice calls Yahoo Finance to get the latest price for a ticker.
// If the ticker has no exchange suffix and the direct lookup fails, it tries
// common European exchange suffixes (.AS, .DE, .L, .MI, .PA).
func fetchYahooPrice(ticker string) (float64, error) {
	candidates := []string{ticker}
	if !strings.Contains(ticker, ".") {
		for _, suffix := range []string{".AS", ".DE", ".L", ".MI", ".PA"} {
			candidates = append(candidates, ticker+suffix)
		}
	}
	var lastErr error
	for _, candidate := range candidates {
		price, err := fetchYahooPriceRaw(candidate)
		if err == nil && price > 0 {
			return price, nil
		}
		lastErr = err
	}
	return 0, lastErr
}

func fetchYahooPriceRaw(ticker string) (float64, error) {
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=1d", ticker)
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result struct {
		Chart struct {
			Result []struct {
				Meta struct {
					RegularMarketPrice float64 `json:"regularMarketPrice"`
				} `json:"meta"`
			} `json:"result"`
			Error *struct {
				Description string `json:"description"`
			} `json:"error"`
		} `json:"chart"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("parsing response: %w", err)
	}
	if result.Chart.Error != nil {
		return 0, fmt.Errorf("yahoo error: %s", result.Chart.Error.Description)
	}
	if len(result.Chart.Result) == 0 {
		return 0, fmt.Errorf("no data for ticker %s", ticker)
	}

	price := result.Chart.Result[0].Meta.RegularMarketPrice
	priceStr := strconv.FormatFloat(price, 'f', 4, 64)
	return strconv.ParseFloat(priceStr, 64)
}

type HistoryPoint struct {
	Date  string  `json:"date"`
	Close float64 `json:"close"`
}

// fetchYahooHistory fetches OHLCV history and returns date+close pairs.
// Uses exchange suffix fallback same as fetchYahooPrice.
func fetchYahooHistory(ticker, rangeParam string) ([]HistoryPoint, error) {
	interval := "1wk"
	if rangeParam == "1mo" || rangeParam == "3mo" {
		interval = "1d"
	}

	candidates := []string{ticker}
	if !strings.Contains(ticker, ".") {
		for _, suffix := range []string{".AS", ".DE", ".L", ".MI", ".PA"} {
			candidates = append(candidates, ticker+suffix)
		}
	}

	var lastErr error
	for _, candidate := range candidates {
		points, err := fetchYahooHistoryRaw(candidate, interval, rangeParam)
		if err == nil && len(points) > 0 {
			return points, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func fetchYahooHistoryRaw(ticker, interval, rangeParam string) ([]HistoryPoint, error) {
	url := fmt.Sprintf(
		"https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=%s&range=%s",
		ticker, interval, rangeParam,
	)
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Chart struct {
			Result []struct {
				Timestamps []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Close []float64 `json:"close"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error *struct {
				Description string `json:"description"`
			} `json:"error"`
		} `json:"chart"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	if result.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo error: %s", result.Chart.Error.Description)
	}
	if len(result.Chart.Result) == 0 || len(result.Chart.Result[0].Timestamps) == 0 {
		return nil, fmt.Errorf("no history for ticker %s", ticker)
	}

	r := result.Chart.Result[0]
	closes := r.Indicators.Quote[0].Close
	var points []HistoryPoint
	for i, ts := range r.Timestamps {
		if i >= len(closes) || closes[i] == 0 {
			continue
		}
		t := time.Unix(ts, 0).UTC()
		points = append(points, HistoryPoint{
			Date:  t.Format("2006-01-02"),
			Close: closes[i],
		})
	}
	return points, nil
}
