// Derives a sensible comment-match pattern for a label rule from a
// transaction comment: the merchant-ish start of the text, minus legal-form
// prefixes and numeric/location noise. The user can always edit it.

const NOISE_PREFIXES = new Set(['uab', 'ab', 'mb', 'vsi', 'vši', 'iį', 'si', 'sį', 'www'])

export function deriveRulePattern(comment: string): string {
  // First segment: up to a separator that usually ends the merchant name.
  const segment = comment.toLowerCase().split(/[.,|(\\\-–—]/)[0] ?? ''
  const words = segment
    .split(/\s+/)
    .map((w) => w.trim())
    .filter(Boolean)

  while (words.length > 0 && NOISE_PREFIXES.has(words[0].replace(/[^\p{L}]/gu, ''))) {
    words.shift()
  }
  // Merchant names rarely need more than two words; stop at numeric/location noise.
  const kept: string[] = []
  for (const w of words) {
    if (kept.length >= 2) break
    if (/\d/.test(w)) break
    if (w.length < 2) break
    kept.push(w)
  }
  return kept.join(' ')
}

// True when an existing rule already labels this comment — no point
// suggesting a duplicate rule.
export function ruleCoversComment(
  rules: { label: string; category: string; comment_match: string }[],
  label: string,
  comment: string,
  category: string,
): boolean {
  const c = comment.toLowerCase()
  return rules.some(
    (r) =>
      r.label === label &&
      (r.comment_match !== '' || r.category !== '') &&
      (r.comment_match === '' || c.includes(r.comment_match.toLowerCase())) &&
      (r.category === '' || r.category === category),
  )
}
