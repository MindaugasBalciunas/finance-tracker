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

export const GROUPS: { id: string; name: string; slot: number }[] = [
  { id: 'cash', name: 'Cash', slot: 1 },
  { id: 'investments', name: 'Investments', slot: 2 },
  { id: 'pension', name: 'Pension', slot: 3 },
  { id: 'crypto', name: 'Crypto', slot: 4 },
  { id: 'real_assets', name: 'Property & car', slot: 5 },
  { id: 'debt', name: 'Debt', slot: 7 },
]
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
