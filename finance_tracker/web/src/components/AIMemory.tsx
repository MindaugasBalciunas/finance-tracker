import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../lib/api'
import { Loading, Tabs, useToast } from './ui'

/** The assistant's two kinds of memory: what you wrote for it and what it noted itself. */
export function AIMemory() {
  const [tab, setTab] = useState('brief')
  return (
    <div>
      <Tabs value={tab} onChange={setTab} tabs={[{ value: 'brief', label: 'Your brief' }, { value: 'notes', label: 'Remembered decisions' }]} />
      <div className="pt-3">
        {tab === 'brief'
          ? <TextDoc qk="ai-context" path="/ai/context" rows="h-80" hint="Who you are, your goals and rules, and how you want to be advised. Numbers belong in the data, not here — the assistant reads those itself."
              placeholder={'e.g.\n- Two kids, alimony €1,000/month\n- Goal: mortgage gone by 2035, FI by 55\n- Be blunt; suggest one concrete action'} />
          : <TextDoc qk="ai-notes" path="/ai/notes" rows="h-80" hint="Things you asked the assistant to remember. It adds to this list itself; edit or prune freely." placeholder="Nothing remembered yet." />}
      </div>
    </div>
  )
}

function TextDoc({ qk, path, rows, hint, placeholder }: { qk: string; path: string; rows: string; hint: string; placeholder: string }) {
  const qc = useQueryClient()
  const toast = useToast()
  const { data, isLoading } = useQuery({ queryKey: [qk], queryFn: () => api.get<{ content: string }>(path) })
  const [v, setV] = useState<string | null>(null)
  if (isLoading) return <Loading />
  const text = v ?? data?.content ?? ''
  const save = async () => {
    await api.put(path, { content: v })
    toast('Saved', 'good'); setV(null)
    qc.invalidateQueries({ queryKey: [qk] })
  }
  return (
    <div>
      <div className="mb-2 text-xs text-muted">{hint}</div>
      <textarea className={clsx('input py-2 text-xs leading-relaxed', rows)} value={text} placeholder={placeholder} onChange={(e) => setV(e.target.value)} />
      <div className="mt-2 flex items-center justify-end gap-2">
        <span className="mr-auto text-xs text-muted tnum">{text.length.toLocaleString('en')} characters{v != null && <span className="text-warn"> · unsaved</span>}</span>
        {v != null && <button className="btn-ghost" onClick={() => setV(null)}>Discard</button>}
        <button className="btn-primary" disabled={v == null} onClick={save}>Save</button>
      </div>
    </div>
  )
}
