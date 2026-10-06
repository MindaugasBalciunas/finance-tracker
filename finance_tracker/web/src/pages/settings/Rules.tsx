import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { useRefresh, useRules } from '../../lib/hooks'
import { useCats } from '../../lib/categories'
import type { Rule, RuleReport, RuleStat } from '../../lib/types'
import { Card, Field, Loading, NumberInput, Segmented, Sheet, Spinner, Stat, Toggle, useToast } from '../../components/ui'
import { pct, shortDate } from '../../lib/format'
import { CategoryPicker, TagInput } from '../../components/pickers'
import { TxRow } from '../../components/TxEditor'
import { Icon } from '../../components/Icon'

// ── rules ───────────────────────────────────────────────────────────

type Issue = '' | 'unused' | 'shadowed' | 'overridden' | 'duplicate' | 'off'

/** What a rule's numbers say about it, worst first ('' = healthy). */
function issueOf(r: Rule, st?: RuleStat): Issue {
  if (!r.enabled) return 'off'
  if (!st) return ''
  if (st.duplicate) return 'duplicate'
  if (st.matches === 0) return 'unused'
  if (st.shadowed) return 'shadowed'
  if (st.decides >= 3 && st.overridden * 3 >= st.decides) return 'overridden'
  return ''
}

const ISSUE: Record<Exclude<Issue, ''>, { label: string; hint: string; tone: string }> = {
  duplicate: { label: 'duplicate', hint: 'Same conditions and actions as an earlier rule — safe to delete.', tone: 'text-warn' },
  unused: { label: 'never matches', hint: 'No transaction in the ledger matches it. Old merchant, or a typo in the pattern.', tone: 'text-muted' },
  shadowed: { label: 'always beaten', hint: 'It only sets a category, and an earlier rule always sets it first — it never has an effect.', tone: 'text-warn' },
  overridden: { label: 'often re-filed', hint: 'You moved many of its rows to another category by hand. Narrow the pattern or add an "in category" condition.', tone: 'text-bad' },
  off: { label: 'off', hint: 'Switched off.', tone: 'text-muted' },
}

export function Rules() {
  const { data: rules, isLoading } = useRules()
  const { data: stats } = useQuery({ queryKey: ['rule-stats'], queryFn: () => api.get<RuleReport>('/rules/stats') })
  const cats = useCats()
  const qc = useQueryClient()
  const toast = useToast()
  const [edit, setEdit] = useState<Partial<Rule> | null>(null)
  const [q, setQ] = useState('')
  const [show, setShow] = useState<Issue | 'all'>('all')
  const [sort, setSort] = useState('priority')
  if (isLoading) return <Loading />
  const byId = new Map((stats?.rules ?? []).map((x) => [x.id, x]))
  const all = rules ?? []
  const count = (i: Issue) => all.filter((r) => issueOf(r, byId.get(r.id)) === i).length
  const list = all
    .filter((r) => !q || [r.pattern, r.set_category, r.set_merchant, r.add_tags.join(' ')].join(' ').toLowerCase().includes(q.toLowerCase()))
    .filter((r) => show === 'all' || issueOf(r, byId.get(r.id)) === show)
    .sort((a, b) => sort === 'busy' ? (byId.get(b.id)?.recent ?? 0) - (byId.get(a.id)?.recent ?? 0) || (byId.get(b.id)?.matches ?? 0) - (byId.get(a.id)?.matches ?? 0)
      : sort === 'recent' ? (byId.get(b.id)?.last ?? '').localeCompare(byId.get(a.id)?.last ?? '') : a.priority - b.priority || a.id - b.id)
  const removable = show === 'duplicate' || show === 'unused' ? list : []
  const removeAll = async () => {
    if (!confirm(`Delete ${removable.length} rules? Transactions keep what the rules already filled in.`)) return
    for (const r of removable) await api.del(`/rules/${r.id}`)
    qc.invalidateQueries({ queryKey: ['rules'] }); qc.invalidateQueries({ queryKey: ['rule-stats'] })
    toast(`${removable.length} rules deleted`, 'good'); setShow('all')
  }
  const chips: { v: Issue | 'all'; label: string; n: number }[] = [
    { v: 'all', label: 'All', n: all.length },
    ...(['overridden', 'shadowed', 'duplicate', 'unused', 'off'] as const).map((v) => ({ v, label: ISSUE[v].label, n: count(v) })).filter((c) => c.n > 0),
  ]
  return (
    <div className="space-y-4">
      <div className="text-sm text-muted">Rules fill in merchant, category and tags for new transactions — typed, scanned or from the bank. History (what you filed similar merchants under) fills in the rest. Earlier rules (lower priority number) set the category first.</div>
      {stats && (
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <Stat icon="rule" color="var(--s5)" label="Rules" value={all.filter((r) => r.enabled).length} sub={`${all.length - all.filter((r) => r.enabled).length} switched off`} />
          <Stat icon="check" color="var(--s6)" label="Coverage · 90 d" value={pct(stats.coverage)} sub="new rows a rule matched" />
          <Stat icon="alert" color="var(--s2)" label="Need a look" value={count('overridden') + count('shadowed')} sub={`${count('overridden')} re-filed · ${count('shadowed')} beaten`} tone={count('overridden') + count('shadowed') > 0 ? 'warn' : undefined} onClick={() => setShow(count('overridden') ? 'overridden' : 'shadowed')} />
          <Stat icon="trash" color="var(--s-other)" label="Clean-up" value={count('unused') + count('duplicate')} sub={`${count('unused')} unused · ${count('duplicate')} dupes`} onClick={() => setShow(count('duplicate') ? 'duplicate' : 'unused')} />
        </div>
      )}
      {stats?.top?.length ? (
        <Card title="Busiest in the last 90 days">
          <div className="space-y-1.5">
            {stats.top.map((id) => {
              const r = all.find((x) => x.id === id)
              const st = byId.get(id)
              if (!r || !st) return null
              const max = byId.get(stats.top![0])?.recent ?? 1
              return (
                <button key={id} onClick={() => setEdit(r)} className="block w-full text-left">
                  <div className="flex justify-between gap-2 text-sm"><span className="truncate"><span className="font-mono text-xs">{r.pattern || '∗'}</span> <span className="text-muted">→ {describe(r, cats)}</span></span><span className="shrink-0 tnum text-ink2">{st.recent}×</span></div>
                  <div className="mt-1 h-1 rounded-full bg-sunken"><div className="h-full rounded-full bg-accent/70" style={{ width: `${(st.recent / max) * 100}%` }} /></div>
                </button>
              )
            })}
          </div>
        </Card>
      ) : null}
      <div className="flex gap-2">
        <input className="input" placeholder="Search rules" value={q} onChange={(e) => setQ(e.target.value)} />
        <button className="btn-primary" onClick={() => setEdit({ pattern: '', enabled: true, add_tags: [], priority: 100 })}><Icon name="plus" size={16} />Rule</button>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="no-scrollbar -mx-4 flex gap-1.5 overflow-x-auto overflow-y-hidden px-4 sm:mx-0 sm:px-0">
          {chips.map((c) => <button key={c.v} className={show === c.v ? 'chip-on' : 'chip'} onClick={() => setShow(c.v)}>{c.label} <span className="text-muted">{c.n}</span></button>)}
        </div>
        <Segmented size="sm" value={sort} onChange={setSort} options={[{ value: 'priority', label: 'Order' }, { value: 'busy', label: 'Busiest' }, { value: 'recent', label: 'Last used' }]} />
      </div>
      {show !== 'all' && show !== '' && (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-line bg-sunken/40 px-3 py-2 text-xs text-ink2">
          <span>{ISSUE[show].hint}</span>
          {removable.length > 0 && <button className="btn-danger h-8 px-2.5 text-xs" onClick={removeAll}>Delete all {removable.length}</button>}
        </div>
      )}
      <Card pad={false}>
        <div className="divide-y divide-line">
          {list.length === 0 && <div className="px-4 py-6 text-center text-sm text-muted">No rules here.</div>}
          {list.map((r) => {
            const st = byId.get(r.id)
            const issue = issueOf(r, st)
            return (
              <button key={r.id} onClick={() => setEdit(r)} className={clsx('flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-sunken/40', !r.enabled && 'opacity-50')}>
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm"><span className="font-mono text-xs">{r.pattern || '∗'}</span>{r.when_category && <span className="text-muted"> in {cats.name(r.when_category)}</span>}
                    {issue && <span className={clsx('ml-1.5 text-[11px]', ISSUE[issue].tone)}>· {ISSUE[issue].label}{issue === 'overridden' && st ? ` ${st.overridden}/${st.decides}` : ''}</span>}</div>
                  <div className="truncate text-xs text-muted">→ {describe(r, cats)}</div>
                </div>
                {st && <div className="shrink-0 text-right text-xs"><div className="tnum text-ink2">{st.matches}×</div><div className="text-muted">{st.last ? shortDate(st.last) : '—'}</div></div>}
              </button>
            )
          })}
        </div>
      </Card>
      {edit && <RuleEditor rule={edit} stat={edit.id ? byId.get(edit.id) : undefined} onClose={() => setEdit(null)} />}
    </div>
  )
}

function describe(r: Rule, cats: ReturnType<typeof useCats>) {
  return [r.set_category && cats.path(r.set_category), r.set_merchant && `merchant ${r.set_merchant}`, r.add_tags.length && `tags ${r.add_tags.join(', ')}`].filter(Boolean).join(' · ') || 'nothing'
}

function RuleEditor({ rule, stat, onClose }: { rule: Partial<Rule>; stat?: RuleStat; onClose: () => void }) {
  const [r, setR] = useState(rule)
  const qc = useQueryClient()
  const refresh = useRefresh()
  const toast = useToast()
  const preview = useQuery({ queryKey: ['rule-preview', r.pattern, r.when_category], queryFn: () => api.post<any>('/rules/preview', r), enabled: !!(r.pattern || r.when_category) })
  const save = async (apply: boolean) => {
    // Applying puts the rule's category back on rows you re-filed by hand.
    if (apply && stat && stat.overridden > 0 && r.set_category && !confirm(`${stat.overridden} matching rows were filed elsewhere by hand. Applying to history moves them back to this rule's category. Continue?`)) return
    try {
      const saved = r.id ? await api.put<Rule>(`/rules/${r.id}`, r) : await api.post<Rule>('/rules', r)
      if (apply) {
        const res = await api.post<any>(`/rules/${saved.id}/apply`)
        toast(`Rule saved · ${res.changed} transactions updated`, 'good')
        refresh()
      } else toast('Rule saved', 'good')
      qc.invalidateQueries({ queryKey: ['rules'] })
      qc.invalidateQueries({ queryKey: ['rule-stats'] })
      onClose()
    } catch (e) {
      toast((e as Error).message, 'bad')
    }
  }
  const del = async () => {
    await api.del(`/rules/${r.id}`)
    qc.invalidateQueries({ queryKey: ['rules'] })
    qc.invalidateQueries({ queryKey: ['rule-stats'] })
    onClose()
  }
  return (
    <Sheet open onClose={onClose} title={r.id ? 'Edit rule' : 'New rule'} wide footer={<>
      {r.id && <button className="btn-danger mr-auto" onClick={del}>Delete</button>}
      <button className="btn-ghost" onClick={() => save(false)}>Save</button>
      <button className="btn-primary" onClick={() => save(true)}>Save & apply to history</button>
    </>}>
      {stat && (() => {
        const issue = issueOf(rule as Rule, stat)
        return (
          <div className="mb-4 rounded-xl border border-line bg-sunken/40 p-3 text-sm">
            <div className="flex flex-wrap gap-x-5 gap-y-1">
              <span><b className="tnum">{stat.matches}</b> <span className="text-muted">matching rows</span></span>
              <span><b className="tnum">{stat.recent}</b> <span className="text-muted">in 90 days</span></span>
              {rule.set_category && <span><b className="tnum">{stat.decides}</b> <span className="text-muted">where it set the category</span></span>}
              {stat.overridden > 0 && <span><b className="tnum text-bad">{stat.overridden}</b> <span className="text-muted">re-filed by hand</span></span>}
              <span className="text-muted">last {stat.last ? shortDate(stat.last) : 'never'}</span>
            </div>
            {issue && issue !== 'off' && <div className={clsx('mt-1.5 text-xs', ISSUE[issue].tone)}>{ISSUE[issue].hint}</div>}
          </div>
        )
      })()}
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
