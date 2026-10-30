import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { eur } from '../../lib/format'
import type { FeeLine, Fees } from '../../lib/types'
import { AskCFO, Card, ErrorBox, Field, Loading, NumberInput, Sheet, useToast } from '../../components/ui'

const SOURCE: Record<FeeLine['source'], string> = { yours: 'your figure', typical: 'typical — check the factsheet', none: 'a share: no yearly charge', missing: 'fee not set' }

/** What investments cost a year, and what that takes over 20 years against
 *  a low-cost index fund. */
export function FeesCard() {
  const { data: f, isLoading, error } = useQuery({ queryKey: ['fees'], queryFn: () => api.get<Fees>('/wealth/fees') })
  const [edit, setEdit] = useState<FeeLine | null>(null)
  if (isLoading) return <Card title="Fees" icon="percent"><Loading /></Card>
  if (error || !f) return <ErrorBox error={error} />
  const dear = f.saving_20y > 500
  return (
    <Card title="What your investments cost" icon="percent" color={dear ? 'var(--hue-warn)' : 'var(--hue-good)'}
      action={<AskCFO q="Look at my investment fees: which holdings are expensive, what would cheaper equivalents be, and is it worth switching after costs and taxes?" />}>
      <div className="grid grid-cols-3 gap-3 text-sm">
        <div><div className="text-xs text-muted">A year</div><div className="tnum text-xl font-semibold">{eur(f.yearly)}</div><div className="text-xs text-muted">{f.weighted.toFixed(2)}% on average</div></div>
        <div><div className="text-xs text-muted">Over 20 years</div><div className="tnum text-xl font-semibold text-bad">−{eur(f.drag_20y)}</div><div className="text-xs text-muted">at {f.return.toFixed(1)}% a year</div></div>
        <div><div className="text-xs text-muted">At {f.low_cost}% instead</div><div className={clsx('tnum text-xl font-semibold', dear ? 'text-good' : '')}>{dear ? `+${eur(f.saving_20y)}` : '—'}</div><div className="text-xs text-muted">{dear ? 'kept over 20 years' : 'already low-cost'}</div></div>
      </div>
      <ul className="mt-3 divide-y divide-line">
        {f.lines.map((l) => (
          <li key={l.id} className="flex items-center gap-3 py-2.5">
            <div className="min-w-0 flex-1">
              <div className="truncate text-sm font-medium">{l.name}{l.name !== l.id && l.kind !== 'account' && <span className="text-muted"> · {l.id}</span>}</div>
              <div className={clsx('text-xs', l.source === 'missing' ? 'text-warn' : 'text-muted')}>
                {eur(l.value)} · {l.source === 'missing' ? SOURCE.missing : `${(l.ter + l.platform).toFixed(2)}% a year · ${SOURCE[l.source]}`}
              </div>
            </div>
            <div className="text-right">
              <div className="tnum text-sm">{l.source === 'missing' ? '—' : `${eur(l.yearly)}/yr`}</div>
              {l.saving_20y > 100 && <div className="tnum text-xs text-warn">{eur(l.saving_20y)} over 20y</div>}
            </div>
            {l.kind !== 'stock' && <button className={clsx('h-8 shrink-0 px-2 text-xs', l.source === 'missing' ? 'btn-outline' : 'btn-ghost')} onClick={() => setEdit(l)} aria-label={`Set the fee of ${l.name}`}>{l.source === 'missing' ? 'Set fee' : 'Edit'}</button>}
          </li>
        ))}
      </ul>
      {f.missing > 0 && <p className="mt-1 text-xs text-warn">{f.missing} fund{f.missing === 1 ? '' : 's'} without a fee yet — the totals leave {f.missing === 1 ? 'it' : 'them'} out. The yearly fee ("ongoing charges" or TER) is on the fund's key information document.</p>}
      {edit && <FeeSheet line={edit} onClose={() => setEdit(null)} />}
    </Card>
  )
}

function FeeSheet({ line, onClose }: { line: FeeLine; onClose: () => void }) {
  const [ter, setTer] = useState<number | undefined>(line.source === 'missing' ? undefined : line.ter)
  const [platform, setPlatform] = useState<number | undefined>(line.platform || undefined)
  const qc = useQueryClient()
  const toast = useToast()
  const save = useMutation({
    mutationFn: () => api.put(`/wealth/fees/${encodeURIComponent(line.id)}`, { ter: ter ?? 0, platform: platform ?? 0 }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['fees'] }); toast('Saved', 'good'); onClose() },
    onError: (e) => toast((e as Error).message, 'bad'),
  })
  return (
    <Sheet open onClose={onClose} title={line.name}
      footer={<div className="flex justify-end gap-2"><button className="btn-ghost" onClick={onClose}>Cancel</button><button className="btn-primary" disabled={save.isPending || ter == null} onClick={() => save.mutate()}>Save</button></div>}>
      <div className="space-y-3">
        <Field label="Yearly fund fee, %" hint="“Ongoing charges” or TER on the key information document — e.g. 0.22 for VWCE, often 1–1.5 for bank and pension funds.">
          <NumberInput value={ter} onChange={setTer} placeholder="0.22" />
        </Field>
        <Field label="Platform or custody fee, % a year" hint="What the bank or broker charges on top, if anything. 0 for most brokers.">
          <NumberInput value={platform} onChange={setPlatform} placeholder="0" />
        </Field>
      </div>
    </Sheet>
  )
}
