import { useState, useSyncExternalStore, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import { catIcon } from '../lib/categories'
import { accountIcon } from '../lib/brand'
import { IconTile } from './Icon'
import type { Account, Tx } from '../lib/types'

// Merchant logos: the server knows a list of merchants and their websites
// (/api/logos) and serves their icons; a merchant it doesn't know keeps its
// category icon. Matching folds names the way the server does — accents off,
// letters and digits only — then tries the first word ("YouTube Premium" →
// YouTube).
const FOLD: Record<string, string> = { ą: 'a', č: 'c', ę: 'e', ė: 'e', į: 'i', š: 's', ų: 'u', ū: 'u', ž: 'z' }
export const logoKey = (s: string) =>
  s.toLowerCase().replace(/[ąčęėįšųūž]/g, (c) => FOLD[c]).normalize('NFD').replace(/[^a-z0-9]/g, '')

export function useLogoIndex() {
  return useQuery({ queryKey: ['logos'], queryFn: () => api.get<Record<string, string>>('/logos'), staleTime: Infinity, retry: false })
}

export function logoDomain(index: Record<string, string> | undefined, merchant?: string) {
  if (!index || !merchant) return undefined
  return index[logoKey(merchant)] ?? index[logoKey(merchant.split(/\s+/)[0] ?? '')]
}

// Bank fees and loan interest rarely carry a merchant — the bank charged
// them. The bank named in the note wins ("Seb bank fee" paid from Swedbank),
// else the bank of the account it was paid from, else (loan interest with no
// account, as a recurring bill) the bank that holds the loan.
const BANK_CHARGES = new Set(['finance.bank_fees', 'housing.mortgage_interest'])
export function logoName(t: Pick<Tx, 'merchant' | 'note' | 'category' | 'account_id'>, accounts: Account[] | undefined) {
  if (t.merchant || !BANK_CHARGES.has(t.category) || !accounts) return t.merchant
  const banks = [...new Set(accounts.map((a) => a.institution.trim()).filter(Boolean))]
  const words = new Set(t.note.split(/[^\p{L}\p{N}]+/u).map(logoKey))
  return banks.find((b) => words.has(logoKey(b))) ?? accounts.find((a) => a.id === t.account_id)?.institution.trim()
    ?? (t.category === 'housing.mortgage_interest' ? accounts.find((a) => a.kind === 'loan' && a.institution.trim())?.institution.trim() : undefined) ?? ''
}

type Look = { bg: string; full: boolean }
// Per logo, worked out once and kept on this device: the tile takes the
// logo's own colour. (The icons themselves are cached by the browser for a
// month and by the server beside its database.)
const LOOKS = 'ft-logo-looks-v5' // v5: clear marks on white; icons under 32px inset
const looks = new Map<string, Look>(((): [string, Look][] => {
  try { return Object.entries(JSON.parse(localStorage.getItem(LOOKS) ?? '{}')) } catch { return [] }
})())
// An uploaded logo replaced under the same id: forget its look and load the
// new picture (the server sends it uncached; the version breaks the img's).
const versions = new Map<string, number>()
export function logoReplaced(id: string) {
  looks.delete(id)
  versions.set(id, Date.now())
  try { localStorage.setItem(LOOKS, JSON.stringify(Object.fromEntries(looks))) } catch {}
}
// Sites known by name whose icon couldn't be had (some block automated
// requests): kept for this visit so Settings → Logos can list them.
const missing = new Set<string>()
let missingVer = 0
const listeners = new Set<() => void>()
const markMissing = (domain: string) => {
  if (missing.has(domain)) return
  missing.add(domain); missingVer++
  listeners.forEach((f) => f())
}
export function useMissingLogos() {
  useSyncExternalStore((f) => { listeners.add(f); return () => { listeners.delete(f) } }, () => missingVer)
  return missing
}
/** Load a logo off-screen just to learn whether its site shares one. */
export function probeLogo(domain: string) {
  if (missing.has(domain) || domain.startsWith('custom-')) return
  const img = new Image()
  img.onerror = () => markMissing(domain)
  img.src = `api/logos/${encodeURIComponent(domain)}`
}
const remember = (domain: string, l: Look) => {
  looks.set(domain, l)
  try { localStorage.setItem(LOOKS, JSON.stringify(Object.fromEntries(looks))) } catch {}
}

/** What colour the tile behind a logo should be, read from its pixels: an
 *  icon with a solid background (its corners opaque) fills the tile in that
 *  colour, like an app icon; a transparent mark sits on a soft tint of its
 *  main colour, the way category icons sit on theirs. */
export function lookOf(img: HTMLImageElement): Look | null {
  try {
    const n = 24
    const c = document.createElement('canvas')
    c.width = c.height = n
    const g = c.getContext('2d', { willReadFrequently: true })
    if (!g) return null
    g.drawImage(img, 0, 0, n, n)
    const px = g.getImageData(0, 0, n, n).data
    const at = (x: number, y: number) => { const i = (y * n + x) * 4; return [px[i], px[i + 1], px[i + 2], px[i + 3]] }
    const corners = [at(1, 1), at(n - 2, 1), at(1, n - 2), at(n - 2, n - 2)]
    if (corners.every((p) => p[3] > 200)) {
      const avg = [0, 1, 2].map((k) => Math.round(corners.reduce((a, p) => a + p[k], 0) / 4))
      // Tiles are 32–36px: an icon under 32px (a 16px favicon) would be blown
      // up and blurry, so it sits inset on its own background colour instead.
      return { bg: `rgb(${avg.join(' ')})`, full: (img.naturalWidth || 0) >= 32 }
    }
    // Transparent: the main colour of the opaque pixels, greys left out
    // where there is colour to be had.
    let r = 0, gg = 0, b = 0, w = 0, rg = 0, gg2 = 0, bg2 = 0, w2 = 0
    for (let i = 0; i < px.length; i += 4) {
      if (px[i + 3] < 128) continue
      const [R, G, B] = [px[i], px[i + 1], px[i + 2]]
      r += R; gg += G; b += B; w++
      if (Math.max(R, G, B) - Math.min(R, G, B) > 40) { rg += R; gg2 += G; bg2 += B; w2++ }
    }
    if (!w) return null
    const [R, G, B] = w2 > w / 8 ? [rg / w2, gg2 / w2, bg2 / w2] : [r / w, gg / w, b / w]
    // A mark on a clear background sits on white, the way brands draw
    // themselves (SEB's black, Norfa's red, Facebook's blue) — only a white
    // mark, which would vanish there, gets a dark tile.
    const lum = (0.2126 * R + 0.7152 * G + 0.0722 * B) / 255
    return { bg: lum > 0.85 ? 'rgb(38 38 36)' : 'rgb(255 255 255)', full: false }
  } catch {
    return null
  }
}

/** The merchant's logo on a tile in its own colour, or the category icon
 *  (or `fallback`, e.g. a transfer's arrows). */
export function MerchantMark({ merchant, category = '', size = 36, transfer = false, fallback }: { merchant?: string; category?: string; size?: number; transfer?: boolean; fallback?: ReactNode }) {
  const { data: index } = useLogoIndex()
  const [icon, color] = catIcon(category)
  return <LogoTile domain={transfer ? undefined : logoDomain(index, merchant)} size={size} fallback={fallback ?? <IconTile name={icon} color={transfer ? 'var(--s-other)' : color} size={size} round />} />
}

/** The bank an account is held at, as a logo domain: its institution, else
 *  its name ("Swedbank savings"). A house or car is never shown as the bank
 *  that lent for it, and cash has no bank. */
export function accountLogo(index: Record<string, string> | undefined, a: Pick<Account, 'institution' | 'name' | 'kind'>) {
  if (['property', 'vehicle', 'cash'].includes(a.kind)) return undefined
  return logoDomain(index, a.institution?.trim()) ?? logoDomain(index, a.name)
}

/** An account's bank logo, or its kind icon in the bank's colour. */
export function AccountMark({ a, color, size = 34 }: { a: Pick<Account, 'institution' | 'name' | 'kind'>; color: string; size?: number }) {
  const { data: index } = useLogoIndex()
  return <LogoTile domain={accountLogo(index, a)} size={size} fallback={<IconTile name={accountIcon(a)} color={color} size={size} />} />
}

function LogoTile({ domain, size, fallback }: { domain?: string; size: number; fallback: ReactNode }) {
  // Kept per logo id: an upload swaps vmi.lt (which failed) for custom-vmi,
  // and the new picture must get its own chance and its own colour.
  const [failed, setFailed] = useState<string>()
  const [look, setLook] = useState<[string, Look]>()
  if (!domain || failed === domain) return <>{fallback}</>
  const l = (look?.[0] === domain ? look[1] : undefined) ?? looks.get(domain)
  // A full-bleed square logo (VMI's green block) sits at 80% on a tile of
  // its own corner colour: the edge can't be seen, and the circle no
  // longer clips letters near its corners.
  const inner = Math.round(size * (l?.full ? 0.8 : 0.62))
  return (
    <span className="grid shrink-0 place-items-center overflow-hidden rounded-full ring-1 ring-line" style={{ width: size, height: size, background: l?.bg ?? 'rgb(255 255 255)' }}>
      <img src={`api/logos/${encodeURIComponent(domain)}${versions.has(domain) ? `?v=${versions.get(domain)}` : ''}`} alt="" loading="lazy" width={inner} height={inner}
        className="object-contain"
        onLoad={(e) => { if (!looks.has(domain)) { const v = lookOf(e.currentTarget); if (v) { remember(domain, v); setLook([domain, v]) } } }}
        onError={() => { setFailed(domain); markMissing(domain) }} />
    </span>
  )
}
