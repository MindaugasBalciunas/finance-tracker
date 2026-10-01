// One place that decides how an AI call's cost is written.
//
// nexos.ai reports what it charged; the Claude API reports only tokens, so
// that cost is computed from Anthropic's published list prices. A computed
// number is prefixed with ~ and says so on hover — it is a good guide to what
// a conversation cost, and it is not a bill.
export function formatAICost(costUsd?: number, estimated?: boolean): string | null {
  if (!costUsd || costUsd <= 0) return null
  // Four decimals: a single cheap answer is worth a fraction of a cent, and
  // rounding it to two would print $0.00 for most of them.
  const amount = `$${parseFloat(costUsd.toFixed(4))}`
  return estimated ? `~${amount}` : amount
}

export const AI_COST_ESTIMATE_HINT =
  'Estimated from Anthropic list prices — your provider reports tokens, not cost'
