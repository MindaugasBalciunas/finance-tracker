import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import clsx from 'clsx'
import { useAccounts, useTags } from '../lib/hooks'
import { catColor, useCats } from '../lib/categories'
import { Icon } from './Icon'
import { Sheet } from './ui'
import type { Kind } from '../lib/types'

/** Category picker: a button that opens a searchable, grouped sheet. */
export function CategoryPicker({ value, onChange, kind, allowParents = true, placeholder = 'Category' }: {
  value: string; onChange: (id: string, kind: Kind) => void; kind?: Kind | ''; allowParents?: boolean; placeholder?: string
}) {
  const cats = useCats()
  const [open, setOpen] = useState(false)
  const [q, setQ] = useState('')
  const groups = useMemo(() => {
    const ql = q.trim().toLowerCase()
    return cats.tree
      .filter((p) => !p.archived && (!kind || p.kind === kind))
      .map((p) => ({ ...p, children: p.children.filter((c) => !c.archived && (!ql || c.name.toLowerCase().includes(ql) || p.name.toLowerCase().includes(ql))) }))
      .filter((p) => !ql || p.name.toLowerCase().includes(ql) || p.children.length)
  }, [cats.tree, q, kind])
  const pick = (id: string) => {
    onChange(id, cats.byId[id]?.kind ?? 'expense')
    setOpen(false)
    setQ('')
  }
  return (
    <>
      <button type="button" onClick={() => setOpen(true)} className="input flex items-center gap-2 text-left">
        {value ? (
          <>
            <span className="h-2.5 w-2.5 shrink-0 rounded-[3px]" style={{ background: catColor(value) }} />
            <span className="truncate">{cats.path(value)}</span>
          </>
        ) : (
          <span className="text-muted">{placeholder}</span>
        )}
        <Icon name="chevronD" size={16} className="ml-auto shrink-0 text-muted" />
      </button>
      <Sheet open={open} onClose={() => setOpen(false)} title="Category">
        <input autoFocus className="input mb-3" placeholder="Search categories" value={q} onChange={(e) => setQ(e.target.value)} />
        <div className="space-y-4">
          {groups.map((p) => (
            <div key={p.id}>
              <button onClick={() => allowParents && pick(p.id)} className={clsx('flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm font-semibold', allowParents && 'hover:bg-sunken', value === p.id && 'text-accent')}>
                <span className="h-2.5 w-2.5 rounded-[3px]" style={{ background: catColor(p.id) }} />
                {p.name}
                <span className="ml-auto text-[10px] uppercase tracking-wide text-muted">{p.kind}</span>
              </button>
              <div className="mt-1 flex flex-wrap gap-1.5 pl-6">
                {p.children.map((c) => (
                  <button key={c.id} onClick={() => pick(c.id)} className={value === c.id ? 'chip-on' : 'chip hover:bg-sunken'}>{c.name}</button>
                ))}
              </div>
            </div>
          ))}
        </div>
      </Sheet>
    </>
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
        {value.map((x) => (
          <span key={x} className="chip bg-sunken">
            {x}
            <button type="button" onClick={() => onChange(value.filter((y) => y !== x))} aria-label={`Remove ${x}`}><Icon name="x" size={12} /></button>
          </span>
        ))}
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
            <button type="button" key={x.tag} onClick={() => add(x.tag)} className={clsx('chip hover:bg-sunken', (i === hi || (hi < 0 && x.tag === completion)) && 'ring-1 ring-accent')}>+ {x.tag}</button>
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
  const list = focus ? (data ?? []).filter((s) => s.note !== value) : []
  const take = (v: string) => { onChange(v); setHi(-1) }
  return (
    <div className="relative">
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
      {list.length > 0 && (
        <div className="absolute inset-x-0 top-full z-30 mt-1 overflow-hidden rounded-xl border border-line bg-surface shadow-lg" role="listbox">
          {list.map((s, i) => (
            <button type="button" key={s.note} role="option" aria-selected={i === hi}
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
