package logos

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func reset() {
	cache.Lock()
	cache.m = map[string]icon{}
	cache.Unlock()
}

// pngOf is a real PNG of a given width, so sharpness reads it.
func pngOf(w int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, w))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

func TestKeyMatchesLedgerSpellings(t *testing.T) {
	for in, want := range map[string]string{"Varlė.lt": "varlelt", "McDonald's": "mcdonalds", "Circle K": "circlek", "Bitė": "bite", "Vilniaus šilumos tinklai": "vilniaussilumostinklai"} {
		if got := Key(in); got != want {
			t.Errorf("Key(%q) = %q, want %q", in, got, want)
		}
	}
	if Index()["lidl"] != "lidl.lt" || Index()["evpinternational"] != "paysera.com" {
		t.Error("index misses a merchant")
	}
}

// Of what a site offers, the sharpest wins: its homepage links a 16px
// favicon and a 192px PNG; the 192 is kept, the favicon never needed.
func TestPickKeepsTheSharpestIcon(t *testing.T) {
	reset()
	CacheDir = ""
	var asked []string
	Fetch = func(_ context.Context, url string) ([]byte, string, error) {
		asked = append(asked, url)
		switch url {
		case "https://lidl.lt/":
			return []byte(`<link rel="icon" sizes="16x16" href="/f16.png"><link rel="icon" sizes="192x192" href="/f192.png"><link rel="mask-icon" href="/m.svg">`), "text/html", nil
		case "https://lidl.lt/f16.png":
			return pngOf(16), "image/png", nil
		case "https://lidl.lt/f192.png":
			return pngOf(192), "image/png", nil
		}
		return nil, "", errors.New("404")
	}
	b, mime, err := Get(context.Background(), "lidl.lt")
	if err != nil || mime != "image/png" || sharpness(b, mime) != 192 {
		t.Fatalf("picked %s %d (%v)", mime, sharpness(b, mime), err)
	}
	for _, u := range asked {
		if u == "https://lidl.lt/f16.png" || u == "https://lidl.lt/m.svg" {
			t.Errorf("fetched %s after a sharp icon was found", u)
		}
	}
}

// Only listed sites are fetched; a page that isn't an image is no icon; a
// site without one is remembered; nothing touches the network.
func TestGetFetchesOnlyListedSitesAndOnlyImages(t *testing.T) {
	reset()
	CacheDir = ""
	calls := 0
	Fetch = func(_ context.Context, url string) ([]byte, string, error) {
		calls++
		return []byte("<html>not here</html>"), "image/png", nil // lies about its type
	}
	if _, _, err := Get(context.Background(), "192.168.1.10"); err != ErrUnknown || calls != 0 {
		t.Fatalf("an unlisted host was fetched: %v (%d calls)", err, calls)
	}
	if _, _, err := Get(context.Background(), "rimi.lt"); err == nil {
		t.Error("an HTML page passed as an icon")
	}
	before := calls
	Get(context.Background(), "rimi.lt")
	if calls != before {
		t.Errorf("a site without an icon asked again at once: %d more calls", calls-before)
	}
}

// Kept beside the database: after a restart (memory empty) the icon comes
// from disk, without the network; a month on it is fetched afresh.
func TestIconsSurviveARestart(t *testing.T) {
	reset()
	CacheDir = t.TempDir()
	calls := 0
	Fetch = func(_ context.Context, url string) ([]byte, string, error) {
		calls++
		if url == "https://maxima.lt/apple-touch-icon.png" {
			return pngOf(180), "image/png", nil
		}
		return nil, "", errors.New("404")
	}
	if _, _, err := Get(context.Background(), "maxima.lt"); err != nil {
		t.Fatal(err)
	}
	first := calls
	reset() // the app restarts
	b, _, err := Get(context.Background(), "maxima.lt")
	if err != nil || sharpness(b, "image/png") != 180 || calls != first {
		t.Fatalf("after restart: %v, %d more calls", err, calls-first)
	}
	if c, ok := load("maxima.lt"); !ok || !c.fresh(time.Now()) || c.fresh(time.Now().Add(31*24*time.Hour)) {
		t.Errorf("cache freshness %+v %v", c, ok)
	}
	// An icon picked by older rules is fetched again.
	os.WriteFile(filepath.Join(CacheDir, "maxima.lt.json"), []byte(`{"v":1,"mime":"image/png","at":"2099-01-01T00:00:00Z","size":180}`), 0o644)
	if _, ok := load("maxima.lt"); ok {
		t.Error("an icon from older rules was kept")
	}
}

func TestPublicOnlyRefusesTheHomeNetwork(t *testing.T) {
	for _, a := range []string{"127.0.0.1:443", "10.0.0.5:443", "192.168.1.20:443", "[::1]:443", "169.254.169.254:443", "0.0.0.0:443",
		"100.100.100.100:443", "100.64.1.2:443", "198.18.0.1:443", "0.1.2.3:443", "240.0.0.1:443", "[64:ff9b::a00:5]:443", "[::ffff:10.0.0.5]:443", "[fd7a:115c:a1e0::1]:443"} {
		if publicOnly("tcp", a, nil) == nil {
			t.Errorf("%s allowed", a)
		}
	}
	if err := publicOnly("tcp", "93.184.216.34:443", nil); err != nil {
		t.Errorf("a public address refused: %v", err)
	}
}

func TestIsImageAcceptsCleanSVGOnly(t *testing.T) {
	if isImage([]byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0h10v10z"/></svg>`), "") != "image/svg+xml" {
		t.Error("a clean SVG refused")
	}
	for _, bad := range []string{`<svg><script>alert(1)</script></svg>`, `<svg onload="alert(1)"></svg>`, `<svg><a href="javascript:x"/></svg>`, `<html>hi</html>`} {
		if isImage([]byte(bad), "image/svg+xml") != "" {
			t.Errorf("accepted %q", bad)
		}
	}
	if isImage(append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, maxIcon)...), "") != "" {
		t.Error("an oversized icon accepted")
	}
}

func TestIconLinksReadSizesAndSkipMasks(t *testing.T) {
	got := iconLinks([]byte(`<link rel="shortcut icon" href="/f.ico"><link rel="apple-touch-icon" href="https://cdn.x/t.png"><link rel="icon" type="image/svg+xml" href="/i.svg"><link rel="mask-icon" href="/m.svg"><link rel="icon" href="data:image/png;base64,AA">`))
	if len(got) != 3 || got[1].size != 180 || got[2].size != 512 || got[0].size != 64 {
		t.Errorf("links %+v", got)
	}
	// An ICO's directory names its biggest entry.
	ico := []byte{0, 0, 1, 0, 2, 0, 16, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 48, 48, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if s := sharpness(ico, "image/x-icon"); s != 48 {
		t.Errorf("ico sharpness %d", s)
	}
}

// What the site links wins over a leftover file at the usual address: Maxima
// links a manifest with a 192px app icon; its /apple-touch-icon.png is some
// other picture and is never fetched.
func TestPickPrefersWhatTheSiteLinks(t *testing.T) {
	reset()
	CacheDir = ""
	var asked []string
	Fetch = func(_ context.Context, url string) ([]byte, string, error) {
		asked = append(asked, url)
		switch url {
		case "https://maxima.lt/":
			return []byte(`<link rel="shortcut icon" href="/f.ico"><link rel="manifest" href="/app/manifest.webmanifest">`), "text/html", nil
		case "https://maxima.lt/app/manifest.webmanifest":
			return []byte(`{"icons":[{"src":"icons/app-192.png","sizes":"192x192","type":"image/png"},{"src":"/mono.png","sizes":"512x512","purpose":"monochrome"}]}`), "application/json", nil
		case "https://maxima.lt/app/icons/app-192.png":
			return pngOf(192), "image/png", nil
		case "https://maxima.lt/apple-touch-icon.png":
			return pngOf(180), "image/png", nil
		}
		return nil, "", errors.New("404")
	}
	b, _, err := Get(context.Background(), "maxima.lt")
	if err != nil || sharpness(b, "image/png") != 192 {
		t.Fatalf("picked %d (%v)", sharpness(b, "image/png"), err)
	}
	for _, u := range asked {
		if u == "https://maxima.lt/apple-touch-icon.png" || u == "https://maxima.lt/mono.png" {
			t.Errorf("fetched %s", u)
		}
	}
}

// An uploaded logo wins in the index, survives as a setting (so in backups),
// and anything that isn't a small image is refused.
func TestCustomLogos(t *testing.T) {
	d := testDB(t)
	if err := SetCustom(d, "VMI", pngOf(64)); err != nil {
		t.Fatal(err)
	}
	if err := SetCustom(d, "Bolt", []byte("<html>hi</html>")); err == nil {
		t.Error("an HTML page accepted as a logo")
	}
	if IndexWith(d)["vmi"] != "custom-vmi" {
		t.Errorf("index %v", IndexWith(d)["vmi"])
	}
	if b, mime, err := Custom(d, "vmi"); err != nil || mime != "image/png" || sharpness(b, mime) != 64 {
		t.Errorf("custom %v %s", err, mime)
	}
	// A restored backup can't change what a logo is served as: the type
	// comes from the bytes, and a page in a logo's row isn't served.
	d.Exec(`INSERT OR REPLACE INTO settings(key,value) VALUES('logo_custom:bolt', ?)`,
		`{"name":"Bolt","mime":"text/html","data":"`+base64.StdEncoding.EncodeToString([]byte("<html><script>x</script></html>"))+`"}`)
	if _, _, err := Custom(d, "bolt"); err != ErrUnknown {
		t.Errorf("an HTML page served as a logo: %v", err)
	}
	d.Exec(`UPDATE settings SET value=? WHERE key='logo_custom:vmi'`, `{"name":"VMI","mime":"text/html","data":"`+base64.StdEncoding.EncodeToString(pngOf(64))+`"}`)
	if _, mime, _ := Custom(d, "vmi"); mime != "image/png" {
		t.Errorf("stored type trusted: %s", mime)
	}
	DeleteCustom(d, "vmi")
	if IndexWith(d)["vmi"] != "vmi.lt" {
		t.Errorf("after delete %v", IndexWith(d)["vmi"])
	}
}

// No answer at all (the box offline): nothing is kept on disk, so the site
// isn't written off as icon-less for a week.
func TestNoAnswerIsNotAMiss(t *testing.T) {
	reset()
	CacheDir = t.TempDir()
	Fetch = func(_ context.Context, url string) ([]byte, string, error) {
		return nil, "", errors.New("dial tcp: network is unreachable")
	}
	if _, _, err := Get(context.Background(), "rimi.lt"); err != ErrUnknown {
		t.Fatalf("offline: %v", err)
	}
	if _, ok := load("rimi.lt"); ok {
		t.Error("an offline miss was stored")
	}
	// The site answering "404" everywhere is a real miss, kept.
	reset()
	Fetch = func(_ context.Context, url string) ([]byte, string, error) {
		return nil, "", StatusError("404 Not Found")
	}
	Get(context.Background(), "rimi.lt")
	if _, ok := load("rimi.lt"); !ok {
		t.Error("a site's own 404s weren't remembered")
	}
}
