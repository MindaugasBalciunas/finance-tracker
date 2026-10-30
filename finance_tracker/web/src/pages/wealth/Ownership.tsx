import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { api } from '../../lib/api'
import { eur } from '../../lib/format'
import type { OwnCost, OwnSettings } from '../../lib/types'
import { AskCFO, Card, Empty, ErrorBox, Field, Loading, NumberInput, Sheet, useToast } from '../../components/ui'
import { Icon } from '../../components/Icon'

/** True cost of owning: what the car or the house really costs a year —
 *  running costs, interest, value lost, and what the money tied up in it
 *  would earn — beside the alternative you name. */
export function Ownership() {
  const { data, isLoading, error } = useQuery({ queryKey: ['ownership'], queryFn: () => api.get<OwnCost[]>('/wealth/ownership') })
  const [edit, setEdit] = useState<OwnCost | null>(null)
  if (isLoading) return <Loading />
  if (error || !data) return <ErrorBox error={error} />
  if (data.length === 0) return <Empty title="No property or vehicle" icon="car">Add a property or vehicle account to see what owning it really costs.</Empty>
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:items-start">
        {data.map((c) => <AssetCard key={c.account_id} c={c} onEdit={() => setEdit(c)} />)}
      </div>
      <p className="text-xs text-muted">Last 12 months. Running costs are everything tagged with the asset; “would earn invested” is an estimate at your plan’s expected return.</p>
      {edit && <EditSheet c={edit} onClose={() => setEdit(null)} />}
    </div>
  )
}

function AssetCard({ c, onEdit }: { c: OwnCost; onEdit: () => void }) {
  const car = c.kind === 'vehicle'
  return (
    <Card title={c.name} icon={car ? 'car' : 'home'} color={car ? 'var(--hue-vehicle)' : 'var(--hue-property)'}
      action={<div className="flex shrink-0 gap-1"><AskCFO q={`Is owning ${c.name} still worth it? Use get_ownership_costs.`} label="" />
        <button className="btn-ghost h-8 px-2 text-xs" aria-label={`Edit figures for ${c.name}`} onClick={onEdit}><Icon name="edit" size={14} />Edit</button></div>}>
      <div className="flex items-end justify-between gap-3">
        <div>
          <div className="text-xs text-ink2">Really costs</div>
          <div className="tnum text-3xl font-semibold">{eur(Math.round(c.month))}<span className="text-base font-normal text-ink2"> a month</span></div>
          <div className="tnum text-xs text-ink2">{eur(Math.round(c.year))} a year{c.per_km ? ` · €${c.per_km.toFixed(2)} a km` : ''}</div>
        </div>
        {c.alt_year != null && c.alt_year > 0 && (
          <div className="text-right">
            <div className="text-xs text-ink2">{car ? 'Without it' : 'Renting'}</div>
            <div className="tnum text-lg font-semibold text-ink2">{eur(c.alt_year / 12)}<span className="text-sm font-normal"> /mo</span></div>
          </div>
        )}
      </div>
      {c.verdict && <p className={clsx('mt-2 text-sm font-medium', c.tone === 'warn' ? 'text-warn' : 'text-good')}>{c.verdict}</p>}
      <ul className="mt-3 divide-y divide-line text-sm">
        {c.lines.map((l) => {
          const row = (
            <div className="flex justify-between gap-2 py-2">
              <span className="text-ink2">{l.label}{l.estimate && <span className="text-xs text-muted"> · estimate</span>}</span>
              {/* Costs are positive; an info line shows the change itself (a gain is +). */}
              <span className={clsx('tnum', l.info ? 'text-muted' : l.amount < 0 && 'text-good')}>
                {l.info ? `${l.amount > 0 ? '−' : '+'}${eur(Math.abs(l.amount))}` : l.amount < 0 ? `−${eur(-l.amount)}` : eur(l.amount)}</span>
            </div>
          )
          return <li key={l.key}>{l.link ? <Link to={l.link} className="block hover:bg-sunken/50">{row}</Link> : row}</li>
        })}
      </ul>
      {c.running.length > 0 && (
        <details className="mt-1 text-sm">
          <summary className="cursor-pointer py-1 text-xs text-ink2">Running costs by category</summary>
          <ul className="mt-1 space-y-1">{c.running.map((r) => <li key={r.key} className="flex justify-between"><Link to={r.link!} className="text-ink2 hover:text-ink">{r.label}</Link><span className="tnum">{eur(r.amount)}</span></li>)}</ul>
        </details>
      )}
      {c.missing.length > 0 && (
        <button className="mt-3 w-full rounded-xl border border-dashed border-line p-2 text-left text-xs text-ink2 hover:bg-sunken/50" onClick={onEdit}>
          For the full picture add: {c.missing.map((m) => MISSING[m] ?? m).join(', ')}.
        </button>
      )}
    </Card>
  )
}

const MISSING: Record<string, string> = {
  tag: 'the tag its spending carries', value: 'its value a year ago (Wealth → History)', km_year: 'km driven a year',
  alt_per_km: 'what a km costs without it', rent: 'what renting the same would cost',
}

function EditSheet({ c, onClose }: { c: OwnCost; onClose: () => void }) {
  const [s, setS] = useState<OwnSettings>(c.settings)
  const qc = useQueryClient()
  const toast = useToast()
  const save = useMutation({
    mutationFn: () => api.put(`/wealth/ownership/${encodeURIComponent(c.account_id)}`, s),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['ownership'] }); toast('Saved', 'good'); onClose() },
    onError: (e) => toast((e as Error).message, 'bad'),
  })
  const set = (p: Partial<OwnSettings>) => setS((x) => ({ ...x, ...p }))
  return (
    <Sheet open onClose={onClose} title={c.name}
      footer={<div className="flex justify-end gap-2"><button className="btn-ghost" onClick={onClose}>Cancel</button><button className="btn-primary" disabled={save.isPending} onClick={() => save.mutate()}>Save</button></div>}>
      <div className="space-y-3">
        <Field label="Tag on its spending" hint="Fuel, insurance, repairs… tagged with this count as running costs.">
          <input className="input" value={s.tag} onChange={(e) => set({ tag: e.target.value })} placeholder={c.kind === 'vehicle' ? 'car' : 'house'} />
        </Field>
        {c.kind === 'vehicle' ? <>
          <Field label="Km driven a year"><NumberInput integer value={s.km_year || undefined} onChange={(v) => set({ km_year: v ?? 0 })} placeholder="14000" /></Field>
          <Field label="€ a km without it" hint="Taxi, car-sharing and the odd rental, averaged — 0.30–0.45 is typical in Vilnius.">
            <NumberInput value={s.alt_per_km || undefined} onChange={(v) => set({ alt_per_km: v ?? 0 })} placeholder="0.35" />
          </Field>
        </> : (
          <Field label="Rent for the same, a month"><NumberInput value={s.rent || undefined} onChange={(v) => set({ rent: v ?? 0 })} placeholder="€" /></Field>
        )}
      </div>
    </Sheet>
  )
}
