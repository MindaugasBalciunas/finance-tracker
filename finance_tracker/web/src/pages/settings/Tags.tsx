import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../../lib/api'
import { useRefresh, useTags } from '../../lib/hooks'
import { Card, Field, Loading, Sheet, useToast } from '../../components/ui'

// ── tags ────────────────────────────────────────────────────────────

export function Tags() {
  const { data, isLoading } = useTags()
  const refresh = useRefresh()
  const toast = useToast()
  const [edit, setEdit] = useState<{ from: string; to: string } | null>(null)
  if (isLoading) return <Loading />
  const run = async () => {
    const r = await api.post<any>('/tags/rename', edit)
    toast(`${r.changed} transactions updated`, 'good')
    refresh()
    setEdit(null)
  }
  return (
    <div className="space-y-3">
      <div className="text-sm text-muted">Rename a tag to merge it into another; rename to nothing to remove it everywhere (rules included).</div>
      <TagSuggestions onPick={(from, to) => setEdit({ from, to })} />
      <div className="flex flex-wrap gap-1.5">
        {[...(data ?? [])].sort((a, b) => b.count - a.count).map((t) => (
          <button key={t.tag} className="chip hover:bg-sunken" onClick={() => setEdit({ from: t.tag, to: t.tag })}>{t.tag} <span className="text-muted">{t.count}</span></button>
        ))}
      </div>
      {edit && (
        <Sheet open onClose={() => setEdit(null)} title={`Tag “${edit.from}”`} footer={<>
          <button className="btn-danger mr-auto" onClick={() => setEdit({ ...edit, to: '' })}>Remove everywhere</button>
          <button className="btn-primary" onClick={run}>{edit.to ? 'Rename' : 'Remove'}</button>
        </>}>
          <Field label="New name"><input className="input" value={edit.to} onChange={(e) => setEdit({ ...edit, to: e.target.value })} /></Field>
          <a className="mt-3 inline-block text-sm text-accent" href={`#/ledger?period=all&tag=${encodeURIComponent(edit.from)}`}>See transactions</a>
        </Sheet>
      )}
    </div>
  )
}

function TagSuggestions({ onPick }: { onPick: (from: string, to: string) => void }) {
  const { data } = useQuery({ queryKey: ['tag-suggestions'], queryFn: () => api.get<{ from: string; to: string; reason: string }[]>('/tags/suggestions') })
  if (!data?.length) return null
  return (
    <Card title="Probably the same tag">
      <div className="space-y-1.5">
        {data.map((s) => (
          <div key={s.from + s.to} className="flex items-center justify-between gap-2 text-sm">
            <span><b>{s.from}</b> → <b>{s.to}</b> <span className="text-xs text-muted">{s.reason}</span></span>
            <button className="btn-outline h-8 text-xs" onClick={() => onPick(s.from, s.to)}>Merge…</button>
          </div>
        ))}
      </div>
    </Card>
  )
}
