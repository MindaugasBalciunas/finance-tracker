import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { catColor, useCats } from '../../lib/categories'
import { eur, shortDate } from '../../lib/format'
import type { CategoryUse } from '../../lib/types'
import { ErrorBox, Field, hue, Segmented, Sheet, Toggle, useToast } from '../../components/ui'
import { Icon } from '../../components/Icon'

// ── categories ──────────────────────────────────────────────────────

// Each kind of category in its meaning: money in, money out, money moved.
const KIND_HUE: Record<string, string> = { income: 'var(--hue-income)', expense: 'var(--hue-spending)', transfer: 'var(--hue-invest)' }

export function Categories() {
  const cats = useCats()
  const qc = useQueryClient()
  const toast = useToast()
  const [edit, setEdit] = useState<any>(null)
  const [open, setOpen] = useState<Set<string>>(new Set())
  const toggle = (id: string) => setOpen((o) => { const n = new Set(o); n.has(id) ? n.delete(id) : n.add(id); return n })
  const { data: usage } = useQuery({ queryKey: ['category-usage'], queryFn: () => api.get<Record<string, CategoryUse>>('/categories/usage') })
  const use = (id: string) => usage?.[id]
  // A parent's figures include its subcategories.
  const family = (id: string) => Object.values(usage ?? {}).filter((u) => u.category === id || u.category.startsWith(id + '.'))
  const unused = cats.tree.flatMap((p) => p.children).filter((c) => !c.archived && usage && !use(c.id)?.count)
  const save = useMutation({
    mutationFn: (c: any) => (c.id && cats.byId[c.id] ? api.put(`/categories/${c.id}`, c) : api.post('/categories', c)),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['categories'] }); toast('Saved', 'good'); setEdit(null) },
  })
  return (
    <div className="space-y-3">
      <div className="text-sm text-muted">Categories say what the money was for. Use tags for who and why (kids, trip:rome), and the merchant field for who was paid. Each chip shows how many transactions use it.</div>
      {unused.length > 0 && (
        <div className="rounded-xl border border-line bg-sunken/40 px-3 py-2 text-xs text-ink2">
          <b>{unused.length}</b> subcategories have never been used: {unused.map((c) => c.name).join(', ')}. Archive the ones you won't need to keep pickers short.
        </div>
      )}
      {/* One line per category, folded: open one to see and edit its subcategories. */}
      {(['income', 'expense', 'transfer', 'other'] as const).map((kind) => {
        const list = cats.tree.filter((p) => kind === 'other' ? !['income', 'expense', 'transfer'].includes(p.kind) : p.kind === kind)
        if (!list.length) return null
        return (
          <section key={kind} className="card overflow-hidden" style={hue(KIND_HUE[kind])}>
            <div className="px-4 pb-1 pt-3 text-[11px] font-semibold uppercase tracking-wide" style={{ color: 'var(--c)' }}>{{ income: 'Income', expense: 'Spending', transfer: 'Transfers', other: 'Other' }[kind]}</div>
            <div className="divide-y divide-line">
              {list.map((p) => {
                const fam = family(p.id)
                const isOpen = open.has(p.id)
                return (
                  <div key={p.id}>
                    <button type="button" aria-expanded={isOpen} onClick={() => toggle(p.id)} className="flex min-h-11 w-full items-center gap-2.5 px-4 py-2 text-left hover:bg-sunken/40">
                      <span className="h-2.5 w-2.5 shrink-0 rounded-[3px]" style={{ background: catColor(p.id) }} />
                      <span className="min-w-0 flex-1">
                        <span className="text-sm font-medium">{p.name}</span>{p.essential && <span className="ml-1.5 text-[10px] uppercase tracking-wide text-muted">essential</span>}
                        <span className="block text-xs text-muted">{(() => { const n = p.children.filter((c) => !c.archived).length; return n ? `${n} ${n === 1 ? 'subcategory' : 'subcategories'}` : 'no subcategories' })()}{usage && <span className="tnum"> · {fam.reduce((a, u) => a + u.count, 0)} rows · {eur(fam.reduce((a, u) => a + u.year, 0))} 12 mo</span>}</span>
                      </span>
                      <Icon name="chevronD" size={16} className={clsx('shrink-0 text-muted transition', isOpen && 'rotate-180')} />
                    </button>
                    {isOpen && (
                      <div className="flex flex-wrap gap-1.5 px-4 pb-3">
                        <button className="chip hover:bg-sunken" onClick={() => setEdit(p)}><Icon name="edit" size={12} />Edit</button>
                        {p.children.map((c) => (
                          <button key={c.id} className={clsx('chip hover:bg-sunken', c.archived && 'line-through opacity-50', usage && !use(c.id)?.count && !c.archived && 'border-dashed text-muted')} onClick={() => setEdit(c)}>
                            {c.name}{c.essential && ' ·e'}{usage && <span className="text-muted tnum">{use(c.id)?.count ?? 0}</span>}
                          </button>
                        ))}
                        <button className="chip text-accent hover:bg-sunken" onClick={() => setEdit({ parent: p.id, name: '' })}><Icon name="plus" size={12} />Sub</button>
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          </section>
        )
      })}
      <button className="btn-outline" onClick={() => setEdit({ name: '', kind: 'expense' })}><Icon name="plus" size={16} />Top-level category</button>
      {edit && (
        <Sheet open onClose={() => setEdit(null)} title={edit.id ? `Edit ${edit.name}` : 'New category'} footer={<button className="btn-primary" onClick={() => save.mutate(edit)}>Save</button>}>
          <div className="space-y-4">
            {edit.id && usage && (() => {
              const u = edit.parent ? use(edit.id) : family(edit.id).reduce<CategoryUse>((a, x) => ({ ...a, count: a.count + x.count, year: a.year + x.year, last: x.last > a.last ? x.last : a.last, rules: a.rules + x.rules, budgeted: a.budgeted || x.budgeted }), { category: edit.id, count: 0, year: 0, last: '', rules: 0, budgeted: false })
              return (
                <div className="grid grid-cols-2 gap-x-4 gap-y-2 rounded-xl border border-line bg-sunken/40 p-3 text-sm sm:grid-cols-4">
                  <div><div className="text-xs text-muted">Transactions</div><a className="tnum font-semibold text-accent" href={`#/ledger?period=all&category=${encodeURIComponent(edit.id)}`}>{u?.count ?? 0}</a></div>
                  <div><div className="text-xs text-muted">Last 12 months</div><div className="tnum font-semibold">{eur(u?.year ?? 0)}</div></div>
                  <div><div className="text-xs text-muted">Last used</div><div className="font-semibold">{u?.last ? shortDate(u.last) : 'never'}</div></div>
                  <div><div className="text-xs text-muted">Rules · budget</div><div className="font-semibold">{u?.rules ?? 0} · {u?.budgeted ? 'yes' : 'no'}</div></div>
                </div>
              )
            })()}
            <Field label="Name"><input className="input" value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} /></Field>
            {!edit.id && !edit.parent && (
              <Segmented value={edit.kind} onChange={(k) => setEdit({ ...edit, kind: k })} options={[{ value: 'expense', label: 'Expense' }, { value: 'income', label: 'Income' }, { value: 'transfer', label: 'Transfer' }]} />
            )}
            <Toggle checked={!!edit.essential} onChange={(v) => setEdit({ ...edit, essential: v })} label="Essential (counts toward the emergency fund and needs-vs-wants)" />
            {edit.id && <Toggle checked={!!edit.archived} onChange={(v) => setEdit({ ...edit, archived: v })} label="Archived (hidden from pickers)" />}
            <ErrorBox error={save.error} />
          </div>
        </Sheet>
      )}
    </div>
  )
}
