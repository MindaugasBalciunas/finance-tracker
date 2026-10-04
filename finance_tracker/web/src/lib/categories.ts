import { useMemo } from 'react'
import { useCategories } from './hooks'
import type { Category } from './types'

// Fixed categorical slots (validated order). Colour follows the category,
// never its rank, so Food is orange on every chart. Categories outside the
// eight core ones share the neutral "Other" swatch in multi-series charts.
// The eight largest spending domains (by three-year spend) get the eight slots.
const SLOT: Record<string, number> = { housing: 1, food: 2, utilities: 3, travel: 4, kids: 5, shopping: 6, health: 7, transport: 8 }
export const CORE = Object.keys(SLOT)
export const slotColor = (n: number) => `var(--s${n})`
export const catColor = (id: string) => {
  const s = SLOT[id.split('.')[0]]
  return s ? slotColor(s) : 'var(--s-other)'
}

// Icon + tint for every top-level category. Core categories use their chart
// slot; the rest borrow a palette hue (icons always sit next to a label, so
// sharing a hue with a chart series is harmless). Income is green.
const CAT_ICON: Record<string, [string, string]> = {
  housing: ['home', 'var(--s1)'], food: ['utensils', 'var(--s2)'], utilities: ['bolt', 'var(--s3)'], travel: ['plane', 'var(--s4)'],
  kids: ['baby', 'var(--s5)'], shopping: ['bag', 'var(--s6)'], health: ['pulse', 'var(--s7)'], transport: ['car', 'var(--s8)'],
  leisure: ['ticket', 'var(--s4)'], dating: ['heart', 'var(--s8)'], gifts: ['gift', 'var(--s5)'], subscriptions: ['repeat', 'var(--s7)'],
  finance: ['percent', 'var(--s3)'], salary: ['briefcase', 'var(--s6)'], side_income: ['key', 'var(--s6)'], benefits: ['shield', 'var(--s6)'],
  investment_income: ['trend', 'var(--s6)'], other_income: ['coins', 'var(--s6)'], refunds: ['undo', 'var(--s6)'],
  transfer: ['swap', 'var(--s-other)'], other: ['dots', 'var(--s-other)'],
}
export const catIcon = (id: string): [string, string] => CAT_ICON[(id || '').split('.')[0]] ?? ['dots', 'var(--s-other)']

export const GROUPS: { id: string; name: string; slot: number }[] = [
  { id: 'cash', name: 'Cash', slot: 1 },
  { id: 'investments', name: 'Investments', slot: 2 },
  { id: 'pension', name: 'Pension', slot: 3 },
  { id: 'crypto', name: 'Crypto', slot: 4 },
  { id: 'real_assets', name: 'Property & car', slot: 5 },
  { id: 'debt', name: 'Debt', slot: 7 },
]
/** Groups counted in "Liquid only" views (II/III pillar pensions can be cashed out). */
export const LIQUID_GROUPS = ['cash', 'investments', 'pension', 'crypto']
export const groupName = (g: string) => GROUPS.find((x) => x.id === g)?.name ?? g

export function useCats() {
  const { data } = useCategories()
  return useMemo(() => {
    const list = data ?? []
    const byId: Record<string, Category> = {}
    list.forEach((c) => (byId[c.id] = c))
    const name = (id: string) => byId[id]?.name ?? id
    const path = (id: string) => {
      const c = byId[id]
      if (!c) return id
      return c.parent ? `${byId[c.parent]?.name ?? c.parent} › ${c.name}` : c.name
    }
    const top = (id: string) => id.split('.')[0]
    const tree = list.filter((c) => !c.parent).map((p) => ({ ...p, children: list.filter((c) => c.parent === p.id) }))
    return { list, byId, name, path, top, tree, ready: !!data }
  }, [data])
}
