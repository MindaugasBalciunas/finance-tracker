import type { BankBalanceResult } from '../api/banking'
import { ACCOUNT_LABELS, type AccountKey } from '../types'

// balanceLines turns a sync's balance updates into one line each.
export function balanceLines(bs: BankBalanceResult[] | undefined, fmt: (n: number) => string): { text: string; warn: boolean }[] {
  return (bs ?? []).map((b) => {
    const name = ACCOUNT_LABELS[b.account as AccountKey] ?? b.account
    if (b.skipped) return { text: `${name}: balance not updated — ${b.skipped}`, warn: true }
    if (!b.changed) return { text: `${name}: balance matches the bank (${fmt(b.after)})`, warn: false }
    return { text: `${name}: balance set to ${fmt(b.after)} from the bank (was ${fmt(b.before)})`, warn: false }
  })
}
