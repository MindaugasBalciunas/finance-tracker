package service

import "strings"

// Cost estimation for providers that bill but do not report.
//
// nexos.ai returns nexos_credits_cost on every response, so the cost badge has
// always had a real number behind it there. The Claude API returns token
// counts only — you are billed, but the response never says how much — so a
// direct-Claude install showed a token count and no cost at all.
//
// These are Anthropic's published list prices, so what comes out is an
// ESTIMATE and is labelled as one everywhere it surfaces. It will not match an
// invoice that carries a discount, a batch rate or a priority tier. An unknown
// model yields no estimate rather than a plausible-looking wrong one: a number
// you cannot trust is worse here than a blank.
//
// Prices in USD per million tokens, as published on 2026-09-25.

type modelPrice struct {
	in, out float64
	// cacheRead is the per-MTok rate for tokens served from the prompt cache.
	// Usually 0.1x input, but deliberately cheaper on some models, so it is
	// stored rather than derived.
	cacheRead float64
}

// cacheWriteMultiple is the premium on a 5-minute cache write (the default
// TTL; a 1-hour write is 2x, which this app never asks for).
const cacheWriteMultiple = 1.25

var modelPrices = map[string]modelPrice{
	"claude-fable-5-1":  {in: 10, out: 50, cacheRead: 0.25},
	"claude-mythos-5-1": {in: 10, out: 50, cacheRead: 0.25},
	"claude-fable-5":    {in: 10, out: 50, cacheRead: 1.00},
	"claude-opus-5-5":   {in: 4, out: 20, cacheRead: 0.20},
	"claude-opus-5":     {in: 5, out: 25, cacheRead: 0.50},
	"claude-opus-4-8":   {in: 5, out: 25, cacheRead: 0.50},
	"claude-opus-4-7":   {in: 5, out: 25, cacheRead: 0.50},
	"claude-opus-4-6":   {in: 5, out: 25, cacheRead: 0.50},
	"claude-sonnet-5-5": {in: 2, out: 10, cacheRead: 0.20},
	"claude-sonnet-5":   {in: 2, out: 10, cacheRead: 0.20},
	"claude-sonnet-4-6": {in: 3, out: 15, cacheRead: 0.30},
	"claude-haiku-4-5":  {in: 1, out: 5, cacheRead: 0.10},
}

// normaliseModelKey makes a gateway's display name and the Claude API's id
// meet in the middle: "Claude Opus 5" and "claude-opus-5" are the same model,
// and a nexos install names it the first way. A trailing date snapshot
// ("claude-opus-4-5-20251101") drops to its base id.
func normaliseModelKey(model string) string {
	k := strings.ToLower(strings.TrimSpace(model))
	k = strings.ReplaceAll(k, " ", "-")
	k = strings.ReplaceAll(k, ".", "-")
	// Strip a vendor prefix some gateways prepend ("anthropic/claude-opus-5").
	if i := strings.LastIndex(k, "/"); i >= 0 {
		k = k[i+1:]
	}
	// A dated snapshot ends in -YYYYMMDD; the price is the base model's.
	if i := strings.LastIndex(k, "-"); i > 0 && len(k)-i-1 == 8 {
		if _, ok := modelPrices[k[:i]]; ok {
			return k[:i]
		}
	}
	return k
}

// estimateCostUSD prices one response from its token counts. The second
// return reports whether the model was known at all — callers must not treat
// a zero as "this was free".
func estimateCostUSD(model string, inTok, outTok, cacheRead, cacheWrite int) (float64, bool) {
	p, ok := modelPrices[normaliseModelKey(model)]
	if !ok {
		return 0, false
	}
	const perToken = 1_000_000.0
	cost := float64(inTok)*p.in/perToken +
		float64(outTok)*p.out/perToken +
		float64(cacheRead)*p.cacheRead/perToken +
		float64(cacheWrite)*p.in*cacheWriteMultiple/perToken
	return cost, true
}
