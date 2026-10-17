import { useState } from 'react'
import { PERIODS, periodRange } from '../lib/periods'
import { shortDate, todayISO } from '../lib/format'
import { Sheet } from './ui'
import { Icon } from './Icon'

/** The Ledger's period chips — This month … All time — plus a custom
 *  From–To range. value is a preset, or "custom:YYYY-MM-DD:YYYY-MM-DD". */
export function PeriodPicker({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const custom = value.startsWith('custom:') ? value.split(':') : null
  const [open, setOpen] = useState(false)
  const [from, setFrom] = useState(custom?.[1] ?? '')
  const [to, setTo] = useState(custom?.[2] ?? todayISO())
  return (
    <>
      <div className="no-scrollbar -mx-4 flex gap-1.5 overflow-x-auto overflow-y-hidden px-4 sm:mx-0 sm:px-0">
        {PERIODS.map((p) => <button key={p.value} className={value === p.value ? 'chip-on' : 'chip'} onClick={() => onChange(p.value)}>{p.label}</button>)}
        <button className={custom ? 'chip-on' : 'chip'} onClick={() => setOpen(true)}>
          <Icon name="calendar" size={12} />{custom ? `${shortDate(custom[1])} – ${shortDate(custom[2])}` : 'Custom'}
        </button>
      </div>
      <Sheet open={open} onClose={() => setOpen(false)} title="Period" footer={
        <button className="btn-primary" disabled={!from || !to || from > to} onClick={() => { onChange(`custom:${from}:${to}`); setOpen(false) }}>Show</button>}>
        <div className="grid grid-cols-2 gap-3">
          <label><span className="label">From</span><input type="date" className="input" value={from} max={to || undefined} onChange={(e) => setFrom(e.target.value)} /></label>
          <label><span className="label">To</span><input type="date" className="input" value={to} min={from || undefined} onChange={(e) => setTo(e.target.value)} /></label>
        </div>
      </Sheet>
    </>
  )
}

/** The from/to a PeriodPicker value stands for. */
export function pickerRange(v: string): { from?: string; to?: string } {
  if (v.startsWith('custom:')) {
    const [, from, to] = v.split(':')
    return { from, to }
  }
  return periodRange(v)
}
