// How a tag looks wherever it is picked: trips, people and places each get
// an icon and a hue, so a row of tags reads at a glance.
const PEOPLE = new Set(['kristina', 'evelina', 'jekaterina', 'parents', 'friend'])
const GROUPS = new Set(['kids', 'family'])
const PLACE: Record<string, string> = { house: 'home', apartment: 'home', rav4: 'car', car: 'car' }

export const tripName = (tag: string) => { const n = tag.slice(5).replace(/-/g, ' '); return n.charAt(0).toUpperCase() + n.slice(1) }
export const tagLabel = (tag: string) => (tag.startsWith('trip:') ? tripName(tag) : tag.startsWith('owed:') ? `${tag.slice(5)} owes` : tag)

export function tagStyle(tag: string): { icon: string; color: string } {
  if (tag.startsWith('trip:')) return { icon: 'plane', color: 'var(--s4)' }
  if (tag.startsWith('owed:')) return { icon: 'coins', color: 'var(--s6)' }
  if (PEOPLE.has(tag)) return { icon: 'user', color: 'var(--s5)' }
  if (GROUPS.has(tag)) return { icon: 'users', color: 'var(--s5)' }
  if (PLACE[tag]) return { icon: PLACE[tag], color: 'var(--s1)' }
  if (tag === 'work') return { icon: 'briefcase', color: 'var(--s7)' }
  if (tag === 'vacation') return { icon: 'sun', color: 'var(--s4)' }
  return { icon: 'tag', color: 'var(--s-other)' }
}

/** Inline style for a chip tinted with c (on = picked). */
export const tint = (c: string, on: boolean) => on
  ? { background: `color-mix(in oklab, ${c} 18%, transparent)`, borderColor: `color-mix(in oklab, ${c} 60%, transparent)`, color: `color-mix(in oklab, ${c} 75%, rgb(var(--ink)))` }
  : {}
