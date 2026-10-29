import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import clsx from 'clsx'
import { useAccounts, useMerchants, useTags } from '../lib/hooks'
import { catIcon, useCats } from '../lib/categories'
import { tagLabel, tagStyle, tint } from '../lib/tags'
import { Icon, IconTile } from './Icon'
import { Sheet } from './ui'
import type { Kind } from '../lib/types'

export type CatHint = { id: string; why?: string }

// Phones get no keyboard until they ask for it: it would cover the list.
const coarse = () => typeof window !== 'undefined' && !!window.matchMedia?.('(pointer: coarse)').matches

/** Category picker: a button that opens a sheet of category tiles. Tap a
 *  group to see its subcategories (big rows, thumb-sized); type to search
 *  them all; the suggested ones sit on top. */
export function CategoryPicker({ value, onChange, kind, allowParents = true, placeholder = 'Category', suggest = [], badge }: {
  value: string; onChange: (id: string, kind: Kind) => void; kind?: Kind | ''; allowParents?: boolean; placeholder?: string
  suggest?: CatHint[]; badge?: string
}) {
  const cats = useCats()
  const [open, setOpen] = useState(false)
  const [q, setQ] = useState('')
  const [group, setGroup] = useState('')
  const groups = useMemo(() => cats.tree
    .filter((p) => !p.archived && (!kind || p.kind === kind))
    .map((p) => ({ ...p, children: p.children.filter((c) => !c.archived) })), [cats.tree, kind])
  // Search: every match in one list, best first (name starts with it, then contains it, then its group does).
  const hits = useMemo(() => {
    const ql = q.trim().toLowerCase()
    if (!ql) return []
    const out: { id: string; score: number }[] = []
    for (const p of groups) {
      const pn = p.name.toLowerCase()
      if (allowParents && pn.includes(ql)) out.push({ id: p.id, score: pn.startsWith(ql) ? 0 : 2 })
      for (const c of p.children) {
        const cn = c.name.toLowerCase()
        if (cn.includes(ql)) out.push({ id: c.id, score: cn.startsWith(ql) ? 0 : 1 })
        else if (pn.includes(ql)) out.push({ id: c.id, score: 3 })
      }
    }
    return out.sort((a, b) => a.score - b.score).map((x) => x.id)
  }, [groups, q, allowParents])
  const show = (o: boolean) => {
    setOpen(o)
    setQ('')
    setGroup(o && value ? cats.top(value) : '')
  }
  const pick = (id: string) => {
    onChange(id, cats.byId[id]?.kind ?? 'expense')
    show(false)
  }
  const hints = suggest.filter((h) => cats.byId[h.id] && (!kind || cats.byId[h.id].kind === kind)).slice(0, 5)
  const g = groups.find((p) => p.id === group)
  const [icon, color] = catIcon(value)
  return (
    <>
      <button type="button" onClick={() => show(true)} className="input flex h-auto min-h-11 items-center gap-2.5 py-1.5 text-left">
        {value ? (
          <>
            <IconTile name={icon} color={color} size={30} />
            <span className="min-w-0 flex-1 leading-tight">
              <span className="block truncate font-medium">{cats.name(value)}</span>
              {cats.byId[value]?.parent && <span className="block truncate text-[11px] text-muted">{cats.name(cats.byId[value].parent!)}</span>}
            </span>
            {badge && <span className="shrink-0 rounded-full bg-accent/10 px-2 py-0.5 text-[10px] font-medium text-accent">{badge}</span>}
          </>
        ) : (
          <span className="flex-1 text-muted">{placeholder}</span>
        )}
        <Icon name="chevronD" size={16} className="shrink-0 text-muted" />
      </button>
      <Sheet open={open} onClose={() => show(false)} title="Category" tall>
        <div className="sticky -top-4 z-10 -mx-4 -mt-4 mb-3 bg-surface px-4 pb-2 pt-4">
          <div className="relative">
            <Icon name="search" size={16} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted" />
            <input autoFocus={!coarse()} className="input pl-9" placeholder="Search categories" value={q} enterKeyHint="done"
              onChange={(e) => setQ(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && hits[0] && (e.preventDefault(), pick(hits[0]))} />
            {q && <button type="button" className="absolute right-1 top-1/2 grid h-8 w-8 -translate-y-1/2 place-items-center text-muted" onClick={() => setQ('')} aria-label="Clear search"><Icon name="x" size={14} /></button>}
          </div>
        </div>
        {q.trim() ? (
          hits.length ? (
            <div className="-mx-1">{hits.map((id) => <CatRow key={id} id={id} on={id === value} onPick={pick} sub={cats.byId[id]?.parent ? cats.name(cats.byId[id].parent!) : 'Whole group'} />)}</div>
          ) : <div className="py-8 text-center text-sm text-muted">No category matches “{q.trim()}”</div>
        ) : g ? (
          <div>
            <button type="button" className="mb-2 flex h-10 items-center gap-1 text-sm text-accent" onClick={() => setGroup('')}><Icon name="chevronL" size={16} />All categories</button>
            <div className="-mx-1">
              {allowParents && !g.children.some((c) => /general/i.test(c.name)) && <CatRow id={g.id} on={value === g.id} onPick={pick} label={`${g.name} — general`} />}
              {g.children.map((c) => <CatRow key={c.id} id={c.id} on={value === c.id} onPick={pick} />)}
            </div>
          </div>
        ) : (
          <div className="space-y-4">
            {hints.length > 0 && (
              <div>
                <div className="mb-1.5 text-xs font-medium text-muted">Suggested</div>
                <div className="-mx-1">{hints.map((h) => <CatRow key={h.id} id={h.id} on={h.id === value} onPick={pick} sub={h.why ?? (cats.byId[h.id]?.parent ? cats.name(cats.byId[h.id].parent!) : undefined)} />)}</div>
              </div>
            )}
            <div>
              {hints.length > 0 && <div className="mb-1.5 text-xs font-medium text-muted">All categories</div>}
              <div className="grid grid-cols-3 gap-2 sm:grid-cols-4">
                {groups.map((p) => {
                  const [ic, co] = catIcon(p.id)
                  const here = !!value && cats.top(value) === p.id
                  return (
                    <button key={p.id} type="button" onClick={() => (p.children.length ? setGroup(p.id) : pick(p.id))}
                      className={clsx('flex min-h-[5.25rem] flex-col items-center justify-center gap-1.5 rounded-xl border px-1 py-2 text-center text-xs leading-tight transition active:scale-[.97]',
                        here ? 'border-accent bg-accent/5 font-semibold' : 'border-line hover:bg-sunken')}>
                      <IconTile name={ic} color={co} size={34} />
                      <span className="line-clamp-2">{p.name}</span>
                    </button>
                  )
                })}
              </div>
            </div>
          </div>
        )}
      </Sheet>
    </>
  )
}

function CatRow({ id, on, onPick, label, sub }: { id: string; on: boolean; onPick: (id: string) => void; label?: string; sub?: string }) {
  const cats = useCats()
  const [icon, color] = catIcon(id)
  return (
    <button type="button" onClick={() => onPick(id)} aria-pressed={on}
      className={clsx('flex min-h-12 w-full items-center gap-3 rounded-xl px-2 py-1.5 text-left transition', on ? 'bg-accent/10' : 'hover:bg-sunken active:bg-sunken')}>
      <IconTile name={icon} color={color} size={32} />
      <span className="min-w-0 flex-1 leading-tight">
        <span className={clsx('block truncate text-sm', on ? 'font-semibold text-accent' : 'text-ink')}>{label ?? cats.name(id)}</span>
        {sub && <span className="block truncate text-[11px] text-muted">{sub}</span>}
      </span>
      {on && <Icon name="check" size={18} className="text-accent" />}
    </button>
  )
}

/** One-tap categories under the picker: the merchant's usual ones, then the
 *  most used lately. The picked one is highlighted. */
export function CategoryChips({ value, hints, onPick }: { value: string; hints: CatHint[]; onPick: (id: string) => void }) {
  const cats = useCats()
  const list = hints.filter((h) => cats.byId[h.id])
  if (!list.length) return null
  return (
    <div className="no-scrollbar -mx-1 mt-2 flex gap-1.5 overflow-x-auto px-1 pb-0.5">
      {list.map((h) => {
        const on = h.id === value
        const [icon, color] = catIcon(h.id)
        return (
          <button key={h.id} type="button" onClick={() => onPick(h.id)} aria-pressed={on} title={cats.path(h.id)}
            className="chip h-8 shrink-0 gap-1.5 pl-1.5 hover:bg-sunken" style={tint(color, on)}>
            <IconTile name={icon} color={color} size={20} round />
            {cats.name(h.id)}
            {h.why && <span className="text-[10px] opacity-70">· {h.why}</span>}
          </button>
        )
      })}
    </div>
  )
}

// Which accounts can sit on each side of a transaction. Property and vehicles
// are tracked by valuation, never as the account money is paid from.
export const SPEND_KINDS = ['checking', 'savings', 'cash']
export const TRANSFER_FROM_KINDS = [...SPEND_KINDS, 'brokerage', 'crypto']
export const TRANSFER_TO_KINDS = [...TRANSFER_FROM_KINDS, 'pension', 'loan']

export function AccountSelect({ value, onChange, placeholder = 'No account', kinds }: { value: string; onChange: (v: string) => void; placeholder?: string; kinds?: string[] }) {
  const { data } = useAccounts()
  const filter = kinds ? (k: string) => kinds.includes(k) : undefined
  return (
    <select className="input select-pad" value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="">{placeholder}</option>
      {(data ?? []).filter((a) => (!a.archived || a.id === value) && (!filter || filter(a.kind) || a.id === value)).map((a) => (
        <option key={a.id} value={a.id}>{a.name}{a.institution && a.institution !== a.name ? ` · ${a.institution}` : ''}</option>
      ))}
    </select>
  )
}

/** Tag input: chips plus a field with suggestions from tags in use. */
export function TagInput({ value, onChange, placeholder = 'Add tag' }: { value: string[]; onChange: (v: string[]) => void; placeholder?: string }) {
  const { data } = useTags()
  const [text, setText] = useState('')
  const [hi, setHi] = useState(-1) // highlighted suggestion (arrow keys)
  const ref = useRef<HTMLInputElement>(null)
  // Tags you used before: those starting with what you typed first, then
  // ones containing it, most used first.
  const sugg = useMemo(() => {
    const t = text.trim().toLowerCase()
    return (data ?? [])
      .filter((x) => !value.includes(x.tag) && (!t || x.tag.includes(t)))
      .sort((a, b) => Number(b.tag.startsWith(t)) - Number(a.tag.startsWith(t)) || b.count - a.count)
      .slice(0, t ? 8 : 6)
  }, [data, text, value])
  const t = text.trim().toLowerCase()
  // Typing the start of a known tag completes it: Enter or Tab takes it,
  // a comma keeps exactly what you typed.
  const completion = t && sugg[0]?.tag.startsWith(t) && sugg[0].tag !== t ? sugg[0].tag : ''
  const pick = hi >= 0 && hi < sugg.length ? sugg[hi].tag : completion
  const add = (v: string) => {
    v = v.trim().toLowerCase().replace(/,/g, '')
    if (v && !value.includes(v)) onChange([...value, v])
    setText(''); setHi(-1)
    ref.current?.focus()
  }
  return (
    <div>
      <div className="input flex h-auto min-h-10 flex-wrap items-center gap-1.5 py-1.5" onClick={() => ref.current?.focus()}>
        {value.map((x) => {
          const st = tagStyle(x)
          return (
            <span key={x} className="chip gap-1 pr-1" style={tint(st.color, true)} title={x}>
              <Icon name={st.icon} size={13} />
              {tagLabel(x)}
              <button type="button" className="grid h-6 w-6 place-items-center rounded-full hover:bg-black/5" onClick={(e) => { e.stopPropagation(); onChange(value.filter((y) => y !== x)) }} aria-label={`Remove ${x}`}><Icon name="x" size={12} /></button>
            </span>
          )
        })}
        <span className="relative min-w-[6rem] flex-1">
          {completion && hi < 0 && <span aria-hidden className="pointer-events-none absolute inset-0 truncate text-sm"><span className="invisible">{text}</span><span className="text-muted">{completion.slice(text.length)}</span></span>}
          <input ref={ref} value={text} onChange={(e) => { setText(e.target.value); setHi(-1) }} placeholder={value.length ? '' : placeholder}
            autoComplete="off" autoCapitalize="off" spellCheck={false}
            onKeyDown={(e) => {
              if (e.key === 'ArrowDown' && sugg.length) { e.preventDefault(); setHi((hi + 1) % sugg.length) }
              else if (e.key === 'ArrowUp' && sugg.length) { e.preventDefault(); setHi(hi <= 0 ? sugg.length - 1 : hi - 1) }
              else if ((e.key === 'Enter' || (e.key === 'Tab' && pick)) && (pick || text.trim())) { e.preventDefault(); add(pick || text) }
              else if (e.key === ',' && text.trim()) { e.preventDefault(); add(text) }
              else if (e.key === 'Backspace' && !text && value.length) onChange(value.slice(0, -1))
            }}
            className="relative w-full bg-transparent text-sm outline-none" />
        </span>
      </div>
      {sugg.length > 0 && (
        <div className="mt-1.5 flex flex-wrap gap-1.5">
          {sugg.map((x, i) => (
            <button type="button" key={x.tag} onClick={() => add(x.tag)} title={x.tag} className={clsx('chip h-8 hover:bg-sunken', (i === hi || (hi < 0 && x.tag === completion)) && 'ring-1 ring-accent')}>
              <Icon name={tagStyle(x.tag).icon} size={13} className="opacity-70" style={{ color: tagStyle(x.tag).color }} />{tagLabel(x.tag)}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

/** The note field, filled from notes you wrote before: ones used with this
 *  merchant come first (shown even before typing), then any containing what
 *  you type. Tap one, or use the arrow keys and Enter. */
export function NoteInput({ value, onChange, merchant = '', placeholder = 'What was it for?' }: { value: string; onChange: (v: string) => void; merchant?: string; placeholder?: string }) {
  const [focus, setFocus] = useState(false)
  const [hi, setHi] = useState(-1)
  const [q, setQ] = useState(value)
  useEffect(() => { const id = setTimeout(() => setQ(value), 150); return () => clearTimeout(id) }, [value])
  const { data } = useQuery({
    queryKey: ['note-suggest', merchant.trim().toLowerCase(), q.trim().toLowerCase()],
    queryFn: () => api.get<{ note: string; count: number; same_merchant: boolean }[]>('/notes/suggest', { merchant: merchant.trim(), q: q.trim() }),
    enabled: focus && (!!q.trim() || !!merchant.trim()), staleTime: 60_000, placeholderData: (p) => p,
  })
  const list = focus ? (data ?? []).filter((s) => s.note !== value).slice(0, 5) : []
  const take = (v: string) => { onChange(v); setHi(-1) }
  const box = useRef<HTMLDivElement>(null)
  useRevealOnce(box, list.length > 0)
  return (
    <div className="relative scroll-mt-16" ref={box}>
      <input className="input" value={value} placeholder={placeholder} autoComplete="off"
        onChange={(e) => { onChange(e.target.value); setHi(-1) }}
        onFocus={() => setFocus(true)} onBlur={() => setFocus(false)}
        onKeyDown={(e) => {
          if (!list.length) return
          if (e.key === 'ArrowDown') { e.preventDefault(); setHi((hi + 1) % list.length) }
          else if (e.key === 'ArrowUp') { e.preventDefault(); setHi(hi <= 0 ? list.length - 1 : hi - 1) }
          else if (e.key === 'Enter' && hi >= 0) { e.preventDefault(); take(list[hi].note) }
          else if (e.key === 'Escape') setFocus(false)
        }} />
      {/* In the page's flow, not an overlay: a pop-up under the field ends up behind the phone's keyboard. */}
      {list.length > 0 && (
        <div className="mt-1 overflow-hidden rounded-xl border border-line bg-surface shadow-sm" role="listbox">
          {list.map((s, i) => (
            <button type="button" key={s.note} role="option" aria-selected={i === hi} tabIndex={-1}
              onMouseDown={(e) => { e.preventDefault(); take(s.note) }}
              className={clsx('flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-sm hover:bg-sunken', i === hi && 'bg-sunken')}>
              <span className="truncate">{s.note}</span>
              <span className="shrink-0 text-[11px] text-muted">{s.same_merchant && merchant ? merchant : ''}{s.count > 1 ? ` ×${s.count}` : ''}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

/** A text field that offers what you typed before: up to six matches as
 *  one-tap chips right under it (starting with what you typed first, then
 *  containing it, most used first). In the page's flow, so the phone
 *  keyboard never covers them. Arrow keys and Enter pick one too. */
export function SuggestInput({ value, onChange, options, placeholder, className = 'input', upper = false, max = 6 }: {
  value: string; onChange: (v: string) => void; options: { value: string; count?: number; hint?: string }[]
  placeholder?: string; className?: string; upper?: boolean; max?: number
}) {
  const [focus, setFocus] = useState(false)
  const [hi, setHi] = useState(-1)
  const id = useId()
  const t = value.trim().toLowerCase()
  const list = useMemo(() => {
    if (!focus) return []
    return options
      .filter((o) => o.value && o.value.toLowerCase() !== t && (!t || o.value.toLowerCase().includes(t)))
      .sort((a, b) => Number(b.value.toLowerCase().startsWith(t)) - Number(a.value.toLowerCase().startsWith(t)) || (b.count ?? 0) - (a.count ?? 0))
      .slice(0, max)
  }, [options, t, focus, max])
  const box = useRef<HTMLDivElement>(null)
  useRevealOnce(box, focus && list.length > 0)
  // The field keeps focus (chips prevent mousedown), so after a pick the
  // list just re-filters — a correction shows suggestions again.
  const take = (v: string) => { onChange(v); setHi(-1) }
  return (
    <div ref={box} className="scroll-mt-16">
      <input className={className} value={value} placeholder={placeholder} autoComplete="off" spellCheck={false}
        role="combobox" aria-expanded={list.length > 0} aria-controls={id} aria-autocomplete="list"
        onChange={(e) => { onChange(upper ? e.target.value.toUpperCase() : e.target.value); setHi(-1) }}
        onFocus={() => setFocus(true)} onBlur={() => { setFocus(false); setHi(-1) }}
        onKeyDown={(e) => {
          if (!list.length) return
          if (e.key === 'ArrowDown') { e.preventDefault(); setHi((hi + 1) % list.length) }
          else if (e.key === 'ArrowUp') { e.preventDefault(); setHi(hi <= 0 ? list.length - 1 : hi - 1) }
          else if (e.key === 'Enter' && hi >= 0) { e.preventDefault(); take(list[hi].value) }
          else if (e.key === 'Escape') setFocus(false)
        }} />
      {list.length > 0 && (
        <div id={id} className="mt-1.5 flex flex-wrap gap-1.5" role="listbox">
          {list.map((o, i) => (
            <button type="button" key={o.value} role="option" aria-selected={i === hi} tabIndex={-1} title={o.hint}
              onMouseDown={(e) => e.preventDefault()} onClick={() => take(o.value)}
              className={clsx('chip h-10 max-w-full hover:bg-sunken', i === hi && 'ring-1 ring-accent')}><span className="truncate">{o.value}</span>{o.hint && <span className="text-muted">· {o.hint}</span>}</button>
          ))}
        </div>
      )}
    </div>
  )
}

/** The merchant field: merchants you paid before. */
export function MerchantInput({ value, onChange, placeholder = 'e.g. Maxima', className }: { value: string; onChange: (v: string) => void; placeholder?: string; className?: string }) {
  const { data } = useMerchants()
  const options = useMemo(() => (data ?? []).map((m) => ({ value: m.merchant, count: m.count })), [data])
  return <SuggestInput value={value} onChange={onChange} options={options} placeholder={placeholder} className={className} />
}

/** The first time suggestions open after a field gets focus, scroll so the
 *  field and its suggestions are both in view (once: not on every keystroke). */
function useRevealOnce(ref: React.RefObject<HTMLElement | null>, open: boolean) {
  const done = useRef(false)
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const reset = () => { done.current = false }
    el.addEventListener('focusout', reset)
    return () => el.removeEventListener('focusout', reset)
  }, [ref])
  useEffect(() => {
    if (!open || done.current) return
    done.current = true
    // Inside a sheet on a phone the sheet already puts the field at the top.
    if (ref.current?.closest('[data-sheet]') && window.matchMedia?.('(pointer: coarse)').matches) return
    const t = setTimeout(() => ref.current?.scrollIntoView({ block: 'nearest', behavior: 'smooth' }), 120)
    return () => clearTimeout(t)
  }, [open, ref])
}
