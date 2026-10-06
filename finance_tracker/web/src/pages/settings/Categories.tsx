import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { catColor, useCats } from '../../lib/categories'
import { Card, ErrorBox, Field, Segmented, Sheet, Toggle, useToast } from '../../components/ui'
import { Icon } from '../../components/Icon'

// ── categories ──────────────────────────────────────────────────────

export function Categories() {
  const cats = useCats()
  const qc = useQueryClient()
  const toast = useToast()
  const [edit, setEdit] = useState<any>(null)
  const save = useMutation({
    mutationFn: (c: any) => (c.id && cats.byId[c.id] ? api.put(`/categories/${c.id}`, c) : api.post('/categories', c)),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['categories'] }); toast('Saved', 'good'); setEdit(null) },
  })
  return (
    <div className="space-y-3">
      <div className="text-sm text-muted">Categories say what the money was for. Use tags for who and why (kids, trip:rome), and the merchant field for who was paid.</div>
      {cats.tree.map((p) => (
        <Card key={p.id} pad={false} title={<span className="flex items-center gap-2"><span className="h-2.5 w-2.5 rounded-[3px]" style={{ background: catColor(p.id) }} />{p.name}<span className="text-[10px] uppercase tracking-wide text-muted">{p.kind}{p.essential ? ' · essential' : ''}</span></span>}
          action={<button className="btn-ghost h-8 px-2 text-xs" onClick={() => setEdit({ parent: p.id, name: '' })}><Icon name="plus" size={14} />Sub</button>}>
          <div className="flex flex-wrap gap-1.5 px-4 pb-4">
            <button className="chip hover:bg-sunken" onClick={() => setEdit(p)}><Icon name="edit" size={12} />Edit</button>
            {p.children.map((c) => <button key={c.id} className={clsx('chip hover:bg-sunken', c.archived && 'line-through opacity-50')} onClick={() => setEdit(c)}>{c.name}{c.essential && ' ·e'}</button>)}
          </div>
        </Card>
      ))}
      <button className="btn-outline" onClick={() => setEdit({ name: '', kind: 'expense' })}><Icon name="plus" size={16} />Top-level category</button>
      {edit && (
        <Sheet open onClose={() => setEdit(null)} title={edit.id ? `Edit ${edit.name}` : 'New category'} footer={<button className="btn-primary" onClick={() => save.mutate(edit)}>Save</button>}>
          <div className="space-y-4">
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
