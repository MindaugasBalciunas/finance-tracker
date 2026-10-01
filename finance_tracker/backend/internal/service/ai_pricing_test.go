package service

import (
	"math"
	"testing"
)

func TestEstimateCostUSD(t *testing.T) {
	// 1M in + 1M out on Opus 5.5 at $4/$20 is the easiest arithmetic to
	// check against the published price list by eye.
	t.Run("plain input and output", func(t *testing.T) {
		got, ok := estimateCostUSD("claude-opus-5-5", 1_000_000, 1_000_000, 0, 0)
		if !ok {
			t.Fatal("claude-opus-5-5 should be a known model")
		}
		if math.Abs(got-24.0) > 1e-9 {
			t.Errorf("got $%.6f, want $24.00", got)
		}
	})

	// Cache reads are the whole reason the gateway uses the Anthropic-native
	// passthrough, so pricing them at the full input rate would overstate
	// every cached conversation — the common case here.
	t.Run("cache read is cheaper than input", func(t *testing.T) {
		cached, _ := estimateCostUSD("claude-opus-5-5", 0, 0, 1_000_000, 0)
		fresh, _ := estimateCostUSD("claude-opus-5-5", 1_000_000, 0, 0, 0)
		if math.Abs(cached-0.20) > 1e-9 {
			t.Errorf("cache read: got $%.6f, want $0.20", cached)
		}
		if cached >= fresh {
			t.Errorf("a cache read ($%.4f) must cost less than fresh input ($%.4f)", cached, fresh)
		}
	})

	t.Run("cache write carries its premium", func(t *testing.T) {
		got, _ := estimateCostUSD("claude-opus-5-5", 0, 0, 0, 1_000_000)
		if math.Abs(got-5.0) > 1e-9 { // $4 input x 1.25
			t.Errorf("cache write: got $%.6f, want $5.00", got)
		}
	})

	// A nexos install names the model "Claude Opus 5"; the Claude API calls
	// the same thing "claude-opus-5". Both must price.
	t.Run("display names and ids agree", func(t *testing.T) {
		id, ok1 := estimateCostUSD("claude-opus-5", 1000, 1000, 0, 0)
		display, ok2 := estimateCostUSD("Claude Opus 5", 1000, 1000, 0, 0)
		if !ok1 || !ok2 {
			t.Fatal("both spellings should resolve")
		}
		if id != display {
			t.Errorf("same model priced differently: %v vs %v", id, display)
		}
	})

	t.Run("vendor prefixes and dated snapshots", func(t *testing.T) {
		for _, m := range []string{"anthropic/claude-sonnet-5-5", "claude-sonnet-5-5"} {
			if _, ok := estimateCostUSD(m, 1, 1, 0, 0); !ok {
				t.Errorf("%q should resolve", m)
			}
		}
	})

	// The important negative: an unknown model must report "I don't know",
	// never a confident zero that the UI would render as a free call.
	t.Run("unknown model yields no estimate", func(t *testing.T) {
		for _, m := range []string{"", "gpt-5", "claude-opus-99", "some-local-llama"} {
			cost, ok := estimateCostUSD(m, 1_000_000, 1_000_000, 0, 0)
			if ok {
				t.Errorf("%q should be unknown, got $%.4f", m, cost)
			}
			if cost != 0 {
				t.Errorf("%q: unknown model must cost 0, got %v", m, cost)
			}
		}
	})

	// Relative sanity across the line-up — catches a transposed row in the
	// table, which arithmetic tests on one model never would.
	t.Run("the tiers are in the right order", func(t *testing.T) {
		price := func(m string) float64 {
			c, ok := estimateCostUSD(m, 1_000_000, 1_000_000, 0, 0)
			if !ok {
				t.Fatalf("%s missing from the price table", m)
			}
			return c
		}
		haiku, sonnet, opus, fable := price("claude-haiku-4-5"), price("claude-sonnet-5-5"),
			price("claude-opus-5-5"), price("claude-fable-5-1")
		if !(haiku < sonnet && sonnet < opus && opus < fable) {
			t.Errorf("tier order wrong: haiku=%v sonnet=%v opus=%v fable=%v", haiku, sonnet, opus, fable)
		}
		// Opus 5.5 is the cheaper successor to Opus 5 — an easy one to get
		// backwards when updating the table.
		if price("claude-opus-5-5") >= price("claude-opus-5") {
			t.Error("claude-opus-5-5 should price below claude-opus-5")
		}
	})
}
