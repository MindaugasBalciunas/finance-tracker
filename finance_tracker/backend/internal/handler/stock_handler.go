package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/marketdata"
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
	g.GET("/analyst/:ticker", h.GetAnalyst)
	g.GET("/:id", h.GetByID)
	g.PUT("/:id", h.Update)
	g.DELETE("", h.DeleteAll)
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
func (h *StockHandler) DeleteAll(c *gin.Context) {
	if err := h.svc.DeleteAll(); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

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

// yahooCache is a TTL cache for Yahoo Finance responses. Prices move slowly
// relative to page loads, and each miss can cost up to 6 serial round-trips
// through the exchange-suffix fallback.
var yahooCache = struct {
	mu sync.Mutex
	m  map[string]yahooCacheEntry
}{m: make(map[string]yahooCacheEntry)}

type yahooCacheEntry struct {
	val     any
	expires time.Time
}

func yahooCacheGet(key string) (any, bool) {
	yahooCache.mu.Lock()
	defer yahooCache.mu.Unlock()
	e, ok := yahooCache.m[key]
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.val, true
}

func yahooCacheSet(key string, val any, ttl time.Duration) {
	yahooCache.mu.Lock()
	defer yahooCache.mu.Unlock()
	yahooCache.m[key] = yahooCacheEntry{val: val, expires: time.Now().Add(ttl)}
}

// fetchYahooPrice / fetchYahooMeta delegate to the shared marketdata package
// (Yahoo chart API, European-suffix fallback, 5-min price cache) so the price
// path has a single implementation shared with the AI insight service.
func fetchYahooPrice(ticker string) (float64, error) {
	return marketdata.Price(ticker)
}

func fetchYahooMeta(ticker string) (*marketdata.Meta, error) {
	return marketdata.FetchMeta(ticker)
}

type HistoryPoint struct {
	Date  string  `json:"date"`
	Close float64 `json:"close"`
}

// fetchYahooHistory fetches OHLCV history and returns date+close pairs.
// Uses exchange suffix fallback same as fetchYahooPrice.
// Successful lookups are cached for 30 minutes.
func fetchYahooHistory(ticker, rangeParam string) ([]HistoryPoint, error) {
	cacheKey := "history:" + ticker + ":" + rangeParam
	if v, ok := yahooCacheGet(cacheKey); ok {
		return v.([]HistoryPoint), nil
	}

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
			yahooCacheSet(cacheKey, points, 30*time.Minute)
			return points, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// yahooSession caches the crumb + cookie jar needed for Yahoo Finance quoteSummary.
var yahooSession struct {
	mu        sync.Mutex
	crumb     string
	jar       http.CookieJar
	fetchedAt time.Time
}

// getYahooCrumb returns a valid crumb + cookie jar, refreshing if older than 30 min.
func getYahooCrumb() (string, http.CookieJar, error) {
	yahooSession.mu.Lock()
	defer yahooSession.mu.Unlock()

	if yahooSession.crumb != "" && time.Since(yahooSession.fetchedAt) < 30*time.Minute {
		return yahooSession.crumb, yahooSession.jar, nil
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second, Jar: jar}

	// Visit Yahoo Finance to set session cookies
	req, _ := http.NewRequest("GET", "https://finance.yahoo.com", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("yahoo session init: %w", err)
	}
	resp.Body.Close()

	// Fetch crumb token
	req2, _ := http.NewRequest("GET", "https://query1.finance.yahoo.com/v1/test/getcrumb", nil)
	req2.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	resp2, err := client.Do(req2)
	if err != nil {
		return "", nil, fmt.Errorf("yahoo crumb fetch: %w", err)
	}
	defer resp2.Body.Close()
	body, _ := io.ReadAll(resp2.Body)
	crumb := strings.TrimSpace(string(body))
	if crumb == "" || crumb == "null" {
		return "", nil, fmt.Errorf("empty crumb from Yahoo Finance")
	}

	yahooSession.crumb = crumb
	yahooSession.jar = jar
	yahooSession.fetchedAt = time.Now()
	return crumb, jar, nil
}

// AnalystData holds analyst price targets from Yahoo Finance quoteSummary.
type AnalystData struct {
	Ticker         string  `json:"ticker"`
	TargetLow      float64 `json:"target_low"`
	TargetMean     float64 `json:"target_mean"`
	TargetHigh     float64 `json:"target_high"`
	Recommendation string  `json:"recommendation"`
	NumAnalysts    int     `json:"num_analysts"`
	Currency       string  `json:"currency"`
}

// GetAnalyst godoc
// @Summary      Get analyst price targets from Yahoo Finance
// @Tags         stocks
// @Produce      json
// @Param        ticker  path      string  true  "Ticker symbol"
// @Success      200     {object}  AnalystData
// @Failure      404     {object}  ErrorResponse
// @Failure      500     {object}  ErrorResponse
// @Router       /stocks/analyst/{ticker} [get]
func (h *StockHandler) GetAnalyst(c *gin.Context) {
	ticker := strings.ToUpper(c.Param("ticker"))
	data, err := fetchYahooAnalyst(ticker)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("no analyst data for %s: %v", ticker, err)})
		return
	}
	c.JSON(http.StatusOK, data)
}

// fetchYahooAnalyst fetches analyst price targets from Yahoo Finance quoteSummary.
// Falls back to 52-week range heuristic from the v8 chart API if quoteSummary is unavailable.
// Successful lookups are cached for 6 hours — analyst targets change rarely.
func fetchYahooAnalyst(ticker string) (*AnalystData, error) {
	cacheKey := "analyst:" + ticker
	if v, ok := yahooCacheGet(cacheKey); ok {
		return v.(*AnalystData), nil
	}

	candidates := []string{ticker}
	if !strings.Contains(ticker, ".") {
		for _, suffix := range []string{".AS", ".DE", ".L", ".MI", ".PA"} {
			candidates = append(candidates, ticker+suffix)
		}
	}

	// Try crumb-based quoteSummary first
	for _, candidate := range candidates {
		data, err := fetchYahooAnalystRaw(candidate)
		if err == nil && data.TargetMean > 0 {
			data.Ticker = ticker
			yahooCacheSet(cacheKey, data, 6*time.Hour)
			return data, nil
		}
	}

	// Fallback: derive scenarios from 52-week range (chart API — no crumb needed)
	for _, candidate := range candidates {
		m, err := fetchYahooMeta(candidate)
		if err == nil && m.RegularMarketPrice > 0 && m.FiftyTwoWeekHigh > 0 {
			p := m.RegularMarketPrice
			// Downside = distance to 52wk low; upside = distance to 52wk high
			// Base = midpoint between bear and bull
			bear := m.FiftyTwoWeekLow
			bull := m.FiftyTwoWeekHigh
			base := p + (bull-bear)*0.35 // weighted toward current price
			if base > bull {
				base = bull
			}
			data := &AnalystData{
				Ticker:         ticker,
				TargetLow:      bear,
				TargetMean:     base,
				TargetHigh:     bull,
				Recommendation: "52wk-range",
				NumAnalysts:    0,
				Currency:       m.Currency,
			}
			yahooCacheSet(cacheKey, data, 6*time.Hour)
			return data, nil
		}
	}

	return nil, fmt.Errorf("no analyst data available for %s", ticker)
}

func fetchYahooAnalystRaw(ticker string) (*AnalystData, error) {
	crumb, jar, err := getYahooCrumb()
	if err != nil {
		return nil, fmt.Errorf("crumb: %w", err)
	}
	apiURL := fmt.Sprintf(
		"https://query2.finance.yahoo.com/v10/finance/quoteSummary/%s?modules=financialData&crumb=%s",
		ticker, url.QueryEscape(crumb),
	)
	client := &http.Client{Timeout: 10 * time.Second, Jar: jar}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

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
		QuoteSummary struct {
			Result []struct {
				FinancialData struct {
					TargetMeanPrice struct {
						Raw float64 `json:"raw"`
					} `json:"targetMeanPrice"`
					TargetLowPrice struct {
						Raw float64 `json:"raw"`
					} `json:"targetLowPrice"`
					TargetHighPrice struct {
						Raw float64 `json:"raw"`
					} `json:"targetHighPrice"`
					RecommendationKey       string `json:"recommendationKey"`
					NumberOfAnalystOpinions struct {
						Raw int `json:"raw"`
					} `json:"numberOfAnalystOpinions"`
					FinancialCurrency string `json:"financialCurrency"`
				} `json:"financialData"`
			} `json:"result"`
			Error *struct {
				Description string `json:"description"`
			} `json:"error"`
		} `json:"quoteSummary"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	if result.QuoteSummary.Error != nil {
		return nil, fmt.Errorf("yahoo error: %s", result.QuoteSummary.Error.Description)
	}
	if len(result.QuoteSummary.Result) == 0 {
		return nil, fmt.Errorf("no data returned")
	}

	fd := result.QuoteSummary.Result[0].FinancialData
	if fd.TargetMeanPrice.Raw == 0 {
		return nil, fmt.Errorf("no analyst targets (ETF or insufficient coverage)")
	}

	return &AnalystData{
		Ticker:         ticker,
		TargetLow:      fd.TargetLowPrice.Raw,
		TargetMean:     fd.TargetMeanPrice.Raw,
		TargetHigh:     fd.TargetHighPrice.Raw,
		Recommendation: fd.RecommendationKey,
		NumAnalysts:    fd.NumberOfAnalystOpinions.Raw,
		Currency:       fd.FinancialCurrency,
	}, nil
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
