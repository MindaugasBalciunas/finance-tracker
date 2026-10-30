// Package logos serves merchant logos: the icon of a merchant's own website,
// for the merchants in a built-in list (never an address from the user or the
// data, so the server can't be pointed at the home network). Of the icons a
// site offers, the sharpest is kept — a scalable SVG or the biggest PNG, not a
// blurry 16-pixel favicon — and saved next to the database, so it survives
// restarts and is fetched again only once a month (a site without one is
// asked again after a week). A merchant without a logo keeps its category icon.
package logos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"ft/internal/merchant"
)

// Domains: merchant (as the ledger names it) → its website. Only names whose
// site is certain: a wrong logo is worse than the category icon.
var Domains = map[string]string{
	"Lidl": "lidl.lt", "Maxima": "maxima.lt", "Rimi": "rimi.lt", "IKI": "iki.lt", "Norfa": "norfa.lt", "Barbora": "barbora.lt",
	"Camelia": "camelia.lt", "Benu": "benu.lt", "Eurovaistinė": "eurovaistine.lt", "Gintarinė vaistinė": "gintarine.lt",
	"AliExpress": "aliexpress.com", "Temu": "temu.com", "eBay": "ebay.com", "Amazon": "amazon.de", "Pigu.lt": "pigu.lt",
	"Varlė.lt": "varle.lt", "Senukai": "senukai.lt", "IKEA": "ikea.com", "Decathlon": "decathlon.lt", "Topo Centras": "topocentras.lt", "Depo": "depo.lt",
	"Neste": "neste.lt", "Circle K": "circlek.lt", "Viada": "viada.lt", "Orlen": "orlen.lt", "Bolt": "bolt.eu", "Uber": "uber.com",
	"Wolt": "wolt.com", "Telia": "telia.lt", "Tele2": "tele2.lt", "Bitė": "bite.lt", "Ignitis": "ignitis.lt", "ESO": "eso.lt",
	"Vilniaus šilumos tinklai": "chc.lt", "Vilniaus vandenys": "vv.lt", "Mano būstas": "manobustas.lt", "Skaitlis": "skaitlis.lt",
	"Moki Veži": "mokivezi.lt", "mParking": "mparking.lt", "Unipark": "unipark.lt", "Gym+": "gymplius.lt",
	"Google": "google.com", "YouTube": "youtube.com", "Spotify": "spotify.com", "Netflix": "netflix.com", "Apple": "apple.com",
	"Disney+": "disneyplus.com", "Microsoft": "microsoft.com", "OpenAI": "openai.com", "Anthropic": "anthropic.com", "Claude": "claude.ai",
	"Swedbank": "swedbank.lt", "SEB": "seb.lt", "Revolut": "revolut.com", "Interactive Brokers": "interactivebrokers.com", "IBKR": "interactivebrokers.com",
	"Luminor": "luminor.lt", "Danske Bank": "danskebank.lt", "Barclays": "barclays.co.uk", "PayPal": "paypal.com", "Paysera": "paysera.com",
	"Evp International": "paysera.com", "Vipps MobilePay": "vippsmobilepay.com",
	"If": "if.lt", "Compensa": "compensa.lt", "Gjensidige": "gjensidige.lt", "VMI": "vmi.lt", "Registrų centras": "registrucentras.lt",
	"McDonald's": "mcdonalds.com", "Hesburger": "hesburger.lt", "KFC": "kfc.lt",
	"Zara": "zara.com", "H&M": "hm.com", "Booking.com": "booking.com", "Airbnb": "airbnb.com", "Ryanair": "ryanair.com",
	"Wizz Air": "wizzair.com", "airBaltic": "airbaltic.com", "Facebook": "facebook.com",
	"Lietuvos paštas": "post.lt", "Akcinė Bendrovė Lietuvos Paštas": "post.lt", "Omniva": "omniva.lt", "DPD": "dpd.com",
	"Steam": "steampowered.com", "Bambu Lab": "bambulab.com", "Nexos.ai": "nexos.ai", "PVcase": "pvcase.com", "DocLogix": "doclogix.com",
	"Lietuvos energijos tiekimas": "ignitis.lt", "Lietuvos dujų tiekimas": "ignitis.lt", "Norfos vaistinė": "norfa.lt",
	"Baltic Petroleum": "balticpetroleum.lt", "Northway": "northway.lt", "Gera Dovana": "geradovana.lt",
	"Autonuoma.com": "autonuoma.com", "Caffeine": "caffeine.lt", "Pepco": "pepco.lt", "CityBee": "citybee.lt",
	"Interneto vizija": "iv.lt", "Vilniaus Gedimino Technikos Universiteta": "vilniustech.lt", "Lietuvos Radijo Ir Televizijos Centras": "telecentras.lt",
}

// Key folds a merchant name for matching: lower case, no accents, letters and
// digits only ("Varlė.lt" → "varlelt", "McDonald's" → "mcdonalds").
func Key(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(merchant.Fold(s)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Index is merchant key → domain, for the web to match ledger merchants.
func Index() map[string]string {
	out := map[string]string{}
	for name, d := range Domains {
		out[Key(name)] = d
	}
	return out
}

func known(domain string) bool {
	for _, d := range Domains {
		if d == domain {
			return true
		}
	}
	return false
}

// publicOnly refuses connections to loopback, private, link-local and other
// non-public addresses, whatever a name resolves to.
func publicOnly(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return err
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return errors.New("not a public address")
	}
	for _, p := range notPublic {
		if p.Contains(ip) {
			return errors.New("not a public address")
		}
	}
	return nil
}

// notPublic: ranges that pass as global unicast but aren't the internet —
// carrier NAT (Tailscale lives there), benchmarking, "this network",
// reserved, documentation, and IPv6 forms that wrap an IPv4 address.
var notPublic = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4",
		"64:ff9b::/96", "64:ff9b:1::/48", "2001::/32", "2001:db8::/32", "2002::/16"} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

var client = &http.Client{
	Timeout: 5 * time.Second,
	Transport: &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 4 * time.Second, Control: publicOnly}).DialContext,
		TLSHandshakeTimeout: 4 * time.Second,
		MaxIdleConns:        4,
	},
	CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 3 || r.URL.Scheme != "https" {
			return errors.New("redirect refused")
		}
		return nil
	},
}

const (
	maxIcon   = 256 << 10
	maxPage   = 512 << 10
	keepFor   = 30 * 24 * time.Hour // a found icon
	retryMiss = 7 * 24 * time.Hour  // a site that had none
	enough    = 128                 // pixels: stop looking once an icon this sharp is found
)

// Fetch is replaceable in tests (they never touch the network). It reads up
// to maxPage: an icon must be far smaller (checked by isImage); a homepage
// only needs its <head>, which comes first. The server says who it is.
// StatusError: the site answered, just not with the file. Unlike a
// network error it says something about the site.
type StatusError string

func (e StatusError) Error() string { return string(e) }

var Fetch = func(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "finance-tracker/logos (self-hosted personal finance app)")
	res, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", StatusError(res.Status)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, maxPage))
	if err != nil || len(b) == 0 {
		return nil, "", errors.New("nothing read")
	}
	return b, res.Header.Get("Content-Type"), nil
}

var svgRisk = regexp.MustCompile(`(?i)<script|javascript:|\bon[a-z]+\s*=|<foreignobject`)

// isImage trusts the bytes, not the header: a PNG, ICO, GIF, JPEG, WebP, or
// an SVG with no script in it (shown through <img>, served with a no-script
// policy too). Anything over maxIcon isn't an icon.
func isImage(b []byte, header string) string {
	if len(b) > maxIcon {
		return ""
	}
	if t := http.DetectContentType(b); strings.HasPrefix(t, "image/") {
		return t
	}
	if len(b) > 4 && b[0] == 0 && b[1] == 0 && b[2] == 1 && b[3] == 0 {
		return "image/x-icon"
	}
	head := strings.ToLower(string(b[:min(len(b), 1024)]))
	if strings.Contains(head, "<svg") && !svgRisk.Match(b) {
		return "image/svg+xml"
	}
	return ""
}

// sharpness: an SVG scales to anything; a bitmap is as sharp as its width;
// an ICO is read from its directory (its biggest entry).
func sharpness(b []byte, mime string) int {
	switch mime {
	case "image/svg+xml":
		return 512
	case "image/x-icon", "image/vnd.microsoft.icon":
		best := 0
		if len(b) >= 6 {
			n := int(b[4]) | int(b[5])<<8
			for i := 0; i < n && 6+16*i+16 <= len(b); i++ {
				w := int(b[6+16*i])
				if w == 0 {
					w = 256
				}
				best = max(best, w)
			}
		}
		return best
	}
	if c, _, err := image.DecodeConfig(bytes.NewReader(b)); err == nil {
		return min(c.Width, c.Height)
	}
	return 0
}

// candidate: an icon the site offers, with the size it claims.
type candidate struct {
	url  string
	size int
}

var linkTag = regexp.MustCompile(`(?i)<link\b[^>]*>`)
var relIcon = regexp.MustCompile(`(?i)\brel\s*=\s*["']?([^"'>]*icon[^"'>]*)`)
var hrefAttr = regexp.MustCompile(`(?i)\bhref\s*=\s*["']([^"']+)["']`)
var sizesAttr = regexp.MustCompile(`(?i)\bsizes\s*=\s*["']?(\d+)x\d+`)
var typeSVG = regexp.MustCompile(`(?i)\btype\s*=\s*["']image/svg`)

// iconLinks: the icons a page links to, with their claimed size (an SVG
// counts as the sharpest; an apple-touch-icon without sizes as 180).
func iconLinks(page []byte) []candidate {
	var out []candidate
	for _, tag := range linkTag.FindAll(page, 60) {
		rel, href := relIcon.FindSubmatch(tag), hrefAttr.FindSubmatch(tag)
		if rel == nil || href == nil {
			continue
		}
		h := strings.TrimSpace(string(href[1]))
		if strings.HasPrefix(h, "data:") || strings.Contains(strings.ToLower(string(rel[1])), "mask") {
			continue // inline, or Safari's one-colour pinned-tab mask
		}
		size := 64
		if m := sizesAttr.FindSubmatch(tag); m != nil {
			size, _ = strconv.Atoi(string(m[1]))
		} else if strings.Contains(strings.ToLower(string(rel[1])), "apple-touch") {
			size = 180
		}
		if typeSVG.Match(tag) || strings.HasSuffix(strings.ToLower(strings.SplitN(h, "?", 2)[0]), ".svg") {
			size = 512
		}
		out = append(out, candidate{url: h, size: size})
	}
	return out
}

// ── the cache: memory, then disk next to the database ──────────────

// pickVersion: bumped when the choice of icon changes, so icons picked by
// older rules (Maxima's wrong one, before linked icons came first) are
// fetched again rather than kept for a month.
const pickVersion = 2

type icon struct {
	V    int       `json:"v"`
	Mime string    `json:"mime"`
	At   time.Time `json:"at"`
	Size int       `json:"size"`
	data []byte
}

func (c icon) fresh(now time.Time) bool {
	if len(c.data) > 0 {
		return now.Sub(c.At) < keepFor
	}
	return now.Sub(c.At) < retryMiss
}

var cache = struct {
	sync.Mutex
	m map[string]icon
}{m: map[string]icon{}}

// CacheDir is where icons are kept on disk ("" = memory only). Set at start
// to a folder beside the database; file names are listed domains only.
var CacheDir string

func load(domain string) (icon, bool) {
	if CacheDir == "" {
		return icon{}, false
	}
	raw, err := os.ReadFile(filepath.Join(CacheDir, domain+".json"))
	if err != nil {
		return icon{}, false
	}
	var c icon
	if json.Unmarshal(raw, &c) != nil || c.V != pickVersion {
		return icon{}, false
	}
	if c.Mime != "" {
		if c.data, err = os.ReadFile(filepath.Join(CacheDir, domain+".img")); err != nil {
			return icon{}, false
		}
	}
	return c, true
}

func store(domain string, c icon) {
	if CacheDir == "" || os.MkdirAll(CacheDir, 0o755) != nil {
		return
	}
	if len(c.data) > 0 && writeAtomic(filepath.Join(CacheDir, domain+".img"), c.data) != nil {
		return
	}
	raw, _ := json.Marshal(c)
	writeAtomic(filepath.Join(CacheDir, domain+".json"), raw)
}

// writeAtomic: a restart mid-write leaves the old file or the new one,
// never half of one.
func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ErrUnknown: not a listed domain, or no icon to be had.
var ErrUnknown = errors.New("no logo for that merchant")

var fetching sync.Map // domain → *sync.Mutex: one fetch per site at a time

// Get returns the sharpest icon of a listed domain, from memory, disk, or the
// site itself.
func Get(ctx context.Context, domain string) ([]byte, string, error) {
	if !known(domain) {
		return nil, "", ErrUnknown
	}
	now := time.Now()
	hit := func(c icon) ([]byte, string, error) {
		if len(c.data) == 0 {
			return nil, "", ErrUnknown
		}
		return c.data, c.Mime, nil
	}
	cache.Lock()
	c, ok := cache.m[domain]
	cache.Unlock()
	if ok && c.fresh(now) {
		return hit(c)
	}
	mu, _ := fetching.LoadOrStore(domain, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()
	if c, ok := load(domain); ok && c.fresh(now) {
		cache.Lock()
		cache.m[domain] = c
		cache.Unlock()
		return hit(c)
	}
	// The fetch outlives the request that asked: a page left mid-load (or
	// a lazy image dropped) mustn't leave the site looking icon-less.
	pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	got, answered := pick(pctx, domain)
	got.At, got.V = now, pickVersion
	if len(got.data) == 0 && !answered {
		// No answer at all (the box offline, the site down): that says
		// nothing about the site, so try again in a few minutes and keep
		// nothing on disk.
		got.At = now.Add(-retryMiss + 10*time.Minute)
		cache.Lock()
		cache.m[domain] = got
		cache.Unlock()
		return hit(got)
	}
	cache.Lock()
	cache.m[domain] = got
	cache.Unlock()
	store(domain, got)
	return hit(got)
}

var manifestLink = regexp.MustCompile(`(?i)<link\b[^>]*\brel\s*=\s*["']?manifest[^>]*>`)

// manifestIcons: the icons a web-app manifest lists (where modern sites keep
// their best: 192 and 512 px), resolved against the manifest's own address.
func manifestIcons(ctx context.Context, page []byte, base *url.URL) []candidate {
	tag := manifestLink.Find(page)
	if tag == nil {
		return nil
	}
	href := hrefAttr.FindSubmatch(tag)
	if href == nil {
		return nil
	}
	u, err := url.Parse(string(href[1]))
	if err != nil {
		return nil
	}
	murl := base.ResolveReference(u)
	if murl.Scheme != "https" {
		return nil
	}
	raw, _, err := Fetch(ctx, murl.String())
	if err != nil {
		return nil
	}
	var m struct {
		Icons []struct {
			Src     string `json:"src"`
			Sizes   string `json:"sizes"`
			Type    string `json:"type"`
			Purpose string `json:"purpose"`
		} `json:"icons"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	var out []candidate
	for _, ic := range m.Icons {
		if ic.Src == "" || strings.Contains(ic.Purpose, "monochrome") {
			continue
		}
		su, err := url.Parse(ic.Src)
		if err != nil {
			continue
		}
		size := 64
		if f := strings.Fields(ic.Sizes); len(f) > 0 {
			if n, err := strconv.Atoi(strings.SplitN(f[len(f)-1], "x", 2)[0]); err == nil {
				size = n
			}
		}
		if strings.Contains(ic.Type, "svg") {
			size = 512
		}
		out = append(out, candidate{url: murl.ResolveReference(su).String(), size: size})
	}
	return out
}

// pick gathers what the site offers and keeps the sharpest icon. What the
// site itself links — its manifest's icons and the icon links in its
// homepage — comes first, biggest first; the usual addresses
// (/apple-touch-icon.png, /favicon.ico) only when it links nothing usable,
// since a leftover file there can be some other picture entirely (Maxima's
// was). Stops once one is sharp enough; fetches at most six.
func pick(ctx context.Context, domain string) (icon, bool) {
	base := &url.URL{Scheme: "https", Host: domain, Path: "/"}
	answered := false
	heard := func(err error) bool {
		var se StatusError
		answered = answered || err == nil || errors.As(err, &se)
		return err == nil
	}
	var linked []candidate
	if page, _, err := Fetch(ctx, base.String()); heard(err) {
		linked = append(manifestIcons(ctx, page, base), iconLinks(page)...)
	}
	sort.SliceStable(linked, func(i, j int) bool { return linked[i].size > linked[j].size })
	fallback := []candidate{{"/apple-touch-icon.png", 180}, {"/favicon.ico", 32}}
	seen := map[string]bool{}
	tried := 0
	try := func(cands []candidate) icon {
		var best icon
		for _, cd := range cands {
			u, err := url.Parse(cd.url)
			if err != nil {
				continue
			}
			abs := base.ResolveReference(u)
			if abs.Scheme != "https" || seen[abs.String()] || tried >= 6 {
				continue // the dialer still refuses private addresses
			}
			seen[abs.String()] = true
			tried++
			b, h, err := Fetch(ctx, abs.String())
			if !heard(err) {
				continue
			}
			mime := isImage(b, h)
			if mime == "" {
				continue
			}
			if s := sharpness(b, mime); s > best.Size {
				best = icon{Mime: mime, Size: s, data: b}
			}
			if best.Size >= enough {
				break
			}
		}
		return best
	}
	if best := try(linked); len(best.data) > 0 {
		return best, true
	}
	best := try(fallback)
	return best, answered || len(best.data) > 0
}
