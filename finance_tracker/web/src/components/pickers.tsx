import { useMemo, useRef, useState } from 'react'
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
    <select className="input" value={value} onChange={(e) => onChange(e.target.value)}>
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
  const ref = useRef<HTMLInputElement>(null)
  const sugg = useMemo(() => {
    const t = text.trim().toLowerCase()
    return (data ?? [])
      .filter((x) => !value.includes(x.tag) && (!t || x.tag.includes(t)))
      .sort((a, b) => b.count - a.count)
      .slice(0, t ? 8 : 6)
  }, [data, text, value])
  const add = (t: string) => {
    t = t.trim().toLowerCase().replace(/,/g, '')
    if (t && !value.includes(t)) onChange([...value, t])
    setText('')
    ref.current?.focus()
  }
  return (
    <div>
      <div className="input flex h-auto min-h-10 flex-wrap items-center gap-1.5 py-1.5" onClick={() => ref.current?.focus()}>
        {value.map((t) => (
          <span key={t} className="chip bg-sunken">
            {t}
            <button type="button" onClick={() => onChange(value.filter((x) => x !== t))} aria-label={`Remove ${t}`}><Icon name="x" size={12} /></button>
          </span>
        ))}
        <input ref={ref} value={text} onChange={(e) => setText(e.target.value)} placeholder={value.length ? '' : placeholder}
          onKeyDown={(e) => {
            if ((e.key === 'Enter' || e.key === ',') && text.trim()) {
              e.preventDefault()
              add(text)
            } else if (e.key === 'Backspace' && !text && value.length) onChange(value.slice(0, -1))
          }}
          className="min-w-[6rem] flex-1 bg-transparent text-sm outline-none" />
      </div>
      {sugg.length > 0 && (
        <div className="mt-1.5 flex flex-wrap gap-1.5">
          {sugg.map((s) => (
            <button type="button" key={s.tag} onClick={() => add(s.tag)} className="chip hover:bg-sunken">+ {s.tag}</button>
          ))}
        </div>
      )}
    </div>
  )
}
