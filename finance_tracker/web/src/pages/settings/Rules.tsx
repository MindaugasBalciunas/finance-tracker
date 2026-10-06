import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { useRefresh, useRules } from '../../lib/hooks'
import { useCats } from '../../lib/categories'
import type { Rule } from '../../lib/types'
import { Card, Field, Loading, NumberInput, Sheet, Spinner, Toggle, useToast } from '../../components/ui'
import { CategoryPicker, TagInput } from '../../components/pickers'
import { TxRow } from '../../components/TxEditor'
import { Icon } from '../../components/Icon'

// ── rules ───────────────────────────────────────────────────────────

export function Rules() {
  const { data: rules, isLoading } = useRules()
  const cats = useCats()
  const [edit, setEdit] = useState<Partial<Rule> | null>(null)
  const [q, setQ] = useState('')
  if (isLoading) return <Loading />
  const list = (rules ?? []).filter((r) => !q || [r.pattern, r.set_category, r.set_merchant, r.add_tags.join(' ')].join(' ').toLowerCase().includes(q.toLowerCase()))
  return (
    <div className="space-y-3">
      <div className="text-sm text-muted">Rules fill in merchant, category and tags for new transactions — typed, scanned or from the bank. History (what you filed similar merchants under) fills in the rest.</div>
      <div className="flex gap-2">
        <input className="input" placeholder="Search rules" value={q} onChange={(e) => setQ(e.target.value)} />
        <button className="btn-primary" onClick={() => setEdit({ pattern: '', enabled: true, add_tags: [], priority: 100 })}><Icon name="plus" size={16} />Rule</button>
      </div>
      <Card pad={false}>
        <div className="divide-y divide-line">
          {list.map((r) => (
            <button key={r.id} onClick={() => setEdit(r)} className={clsx('flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-sunken/40', !r.enabled && 'opacity-50')}>
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm"><span className="font-mono text-xs">{r.pattern || '∗'}</span>{r.when_category && <span className="text-muted"> in {cats.name(r.when_category)}</span>}</div>
                <div className="truncate text-xs text-muted">→ {[r.set_category && cats.path(r.set_category), r.set_merchant && `merchant ${r.set_merchant}`, r.add_tags.length && `tags ${r.add_tags.join(', ')}`].filter(Boolean).join(' · ')}</div>
              </div>
            </button>
          ))}
        </div>
      </Card>
      {edit && <RuleEditor rule={edit} onClose={() => setEdit(null)} />}
    </div>
  )
}

function RuleEditor({ rule, onClose }: { rule: Partial<Rule>; onClose: () => void }) {
  const [r, setR] = useState(rule)
  const qc = useQueryClient()
  const refresh = useRefresh()
  const toast = useToast()
  const preview = useQuery({ queryKey: ['rule-preview', r.pattern, r.when_category], queryFn: () => api.post<any>('/rules/preview', r), enabled: !!(r.pattern || r.when_category) })
  const save = async (apply: boolean) => {
    try {
      const saved = r.id ? await api.put<Rule>(`/rules/${r.id}`, r) : await api.post<Rule>('/rules', r)
      if (apply) {
        const res = await api.post<any>(`/rules/${saved.id}/apply`)
        toast(`Rule saved · ${res.changed} transactions updated`, 'good')
        refresh()
      } else toast('Rule saved', 'good')
      qc.invalidateQueries({ queryKey: ['rules'] })
      onClose()
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  const del = async () => {
    await api.del(`/rules/${r.id}`)
    qc.invalidateQueries({ queryKey: ['rules'] })
    onClose()
  }
  return (
    <Sheet open onClose={onClose} title={r.id ? 'Edit rule' : 'New rule'} wide footer={<>
      {r.id && <button className="btn-danger mr-auto" onClick={del}>Delete</button>}
      <button className="btn-ghost" onClick={() => save(false)}>Save</button>
      <button className="btn-primary" onClick={() => save(true)}>Save & apply to history</button>
    </>}>
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <div className="space-y-4">
          <Field label="When merchant or note contains" hint="Case and accents ignored. Start with ^ to match only the beginning."><input className="input font-mono" value={r.pattern ?? ''} onChange={(e) => setR({ ...r, pattern: e.target.value })} /></Field>
          <div><div className="label">…and the row is in (optional)</div><CategoryPicker value={r.when_category ?? ''} onChange={(id) => setR({ ...r, when_category: id })} placeholder="Any category" /></div>
          <div><div className="label">Set category</div><CategoryPicker value={r.set_category ?? ''} onChange={(id) => setR({ ...r, set_category: id })} placeholder="Leave as is" /></div>
          <Field label="Set merchant"><input className="input" value={r.set_merchant ?? ''} onChange={(e) => setR({ ...r, set_merchant: e.target.value })} /></Field>
          <div><div className="label">Add tags</div><TagInput value={r.add_tags ?? []} onChange={(v) => setR({ ...r, add_tags: v })} /></div>
          <div className="flex items-center gap-4">
            <Toggle checked={r.enabled ?? true} onChange={(v) => setR({ ...r, enabled: v })} label="Enabled" />
            <Field label="Priority"><NumberInput integer className="input h-8 w-20 tnum" value={r.priority ?? 100} onChange={(v) => setR({ ...r, priority: v ?? 100 })} /></Field>
          </div>
        </div>
        <div>
          <div className="label">Matches in your ledger {preview.data && <b className="text-ink">{preview.data.matches}</b>}</div>
          <div className="max-h-96 divide-y divide-line overflow-y-auto rounded-xl border border-line">
            {preview.isFetching ? <div className="p-4"><Spinner /></div> : (preview.data?.sample ?? []).map((t: any) => <TxRow key={t.id} t={t} showDate />)}
          </div>
        </div>
      </div>
    </Sheet>
  )
}
