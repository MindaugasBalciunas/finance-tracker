package marketdata

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Live market context for the AI stocks review: overall sentiment (Fear &
// Greed), per-ticker headlines and social chatter. Everything here is
// keyless, best-effort and cached — a failed or slow source degrades to an
// absent section, never an error for the caller.

const contextTTL = 10 * time.Minute

// A real browser UA — CNN's endpoint bot-blocks anything less.
const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

func ctxClient() *http.Client { return &http.Client{Timeout: 6 * time.Second} }

func ctxGET(rawURL, referer string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "application/json, text/xml, */*")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := ctxClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

func cacheGetStr(key string) (string, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if e, ok := cache.m[key]; ok && time.Now().Before(e.expires) && e.q != nil {
		return e.q.Currency, true // Currency field doubles as the string slot
	}
	return "", false
}

func cacheSetStr(key, val string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.m[key] = cacheEntry{q: &Quote{Currency: val}, expires: time.Now().Add(contextTTL)}
}

// FearGreed returns a one-line market sentiment reading: CNN's Fear & Greed
// for equities, falling back to alternative.me's crypto index.
func FearGreed() string {
	if v, ok := cacheGetStr("ctx:fng"); ok {
		return v
	}
	var line string
	if body, err := ctxGET("https://production.dataviz.cnn.io/index/fearandgreed/graphdata", "https://edition.cnn.com/"); err == nil {
		var out struct {
			FearAndGreed struct {
				Score         float64 `json:"score"`
				Rating        string  `json:"rating"`
				PreviousClose float64 `json:"previous_close"`
			} `json:"fear_and_greed"`
		}
		if json.Unmarshal(body, &out) == nil && out.FearAndGreed.Rating != "" {
			line = fmt.Sprintf("CNN Fear & Greed (stocks): %.0f — %s (prev close %.0f)",
				out.FearAndGreed.Score, out.FearAndGreed.Rating, out.FearAndGreed.PreviousClose)
		}
	}
	if line == "" {
		if body, err := ctxGET("https://api.alternative.me/fng/", ""); err == nil {
			var out struct {
				Data []struct {
					Value          string `json:"value"`
					Classification string `json:"value_classification"`
				} `json:"data"`
			}
			if json.Unmarshal(body, &out) == nil && len(out.Data) > 0 {
				line = fmt.Sprintf("Crypto Fear & Greed: %s — %s", out.Data[0].Value, out.Data[0].Classification)
			}
		}
	}
	cacheSetStr("ctx:fng", line) // cache even the empty result — don't hammer a down source
	return line
}

var rssTitleRe = regexp.MustCompile(`<title>(?:<!\[CDATA\[)?(.*?)(?:\]\]>)?</title>`)

// Headlines returns up to n recent news titles for a ticker (Yahoo RSS).
func Headlines(ticker string, n int) []string {
	key := "ctx:news:" + ticker
	if v, ok := cacheGetStr(key); ok {
		return splitLines(v)
	}
	var titles []string
	body, err := ctxGET("https://feeds.finance.yahoo.com/rss/2.0/headline?s="+url.QueryEscape(ticker)+"&region=US&lang=en-US", "")
	if err == nil {
		for i, m := range rssTitleRe.FindAllStringSubmatch(string(body), n+1) {
			if i == 0 {
				continue // the channel's own title
			}
			if t := strings.TrimSpace(m[1]); t != "" {
				titles = append(titles, t)
			}
		}
	}
	cacheSetStr(key, strings.Join(titles, "\n"))
	return titles
}

// SocialBuzz returns up to n recent social posts about a ticker — a keyless
// sample of what retail is saying right now. Stocktwits first, Bluesky
// cashtag search as fallback (either may be bot-walled on a given network;
// whichever answers wins, and an empty result is fine).
func SocialBuzz(ticker string, n int) []string {
	key := "ctx:social:" + ticker
	if v, ok := cacheGetStr(key); ok {
		return splitLines(v)
	}
	msgs := stocktwits(ticker, n)
	if len(msgs) == 0 {
		msgs = bluesky(ticker, n)
	}
	cacheSetStr(key, strings.Join(msgs, "\n"))
	return msgs
}

func stocktwits(ticker string, n int) []string {
	body, err := ctxGET("https://api.stocktwits.com/api/2/streams/symbol/"+url.QueryEscape(ticker)+".json", "")
	if err != nil {
		return nil
	}
	var out struct {
		Messages []struct {
			Body string `json:"body"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &out) != nil {
		return nil
	}
	var msgs []string
	for _, m := range out.Messages {
		if t := clip(m.Body); t != "" {
			msgs = append(msgs, t)
		}
		if len(msgs) >= n {
			break
		}
	}
	return msgs
}

func bluesky(ticker string, n int) []string {
	body, err := ctxGET(fmt.Sprintf(
		"https://public.api.bsky.app/xrpc/app.bsky.feed.searchPosts?q=%s&limit=%d&sort=latest",
		url.QueryEscape("$"+ticker), n), "")
	if err != nil {
		return nil
	}
	var out struct {
		Posts []struct {
			Record struct {
				Text string `json:"text"`
			} `json:"record"`
		} `json:"posts"`
	}
	if json.Unmarshal(body, &out) != nil {
		return nil
	}
	var msgs []string
	for _, p := range out.Posts {
		if t := clip(p.Record.Text); t != "" {
			msgs = append(msgs, t)
		}
		if len(msgs) >= n {
			break
		}
	}
	return msgs
}

// clip flattens whitespace and truncates a post to one prompt-friendly line.
func clip(s string) string {
	t := strings.Join(strings.Fields(s), " ")
	if len(t) > 140 {
		t = t[:140] + "…"
	}
	return t
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
