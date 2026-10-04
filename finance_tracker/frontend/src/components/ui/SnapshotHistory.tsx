import { useMemo } from 'react'
import type { AccountGroup, Balance } from '../../types'
import { ACCOUNT_GROUP_LABELS } from '../../types'
import { cryptoEur, freeCash, investments, pensions, GROUP_COLORS } from '../../utils/balanceGroups'
import { useBankSections } from '../../utils/accountLayout'
import { formatDate, formatEuro, formatTime } from '../../utils/format'

const TYPE_TOTALS: { group: AccountGroup; value: (b: Balance) => number }[] = [
  { group: 'cash', value: freeCash },
  { group: 'investments', value: investments },
  { group: 'pensions', value: pensions },
  { group: 'crypto', value: cryptoEur },
]

const Dot = ({ group }: { group: AccountGroup }) => (
  <span className="inline-block w-2 h-2 rounded-full flex-shrink-0" style={{ backgroundColor: GROUP_COLORS[group] }} aria-hidden />
)

const cell = (v: number) => (Math.abs(v) < 0.005 ? <span className="text-gray-300">–</span> : formatEuro(v))

// Snapshot history: accounts grouped under their bank, each with its type's
// color dot — the same layout as the snapshot form. Only accounts that ever
// held money get a column (`all` decides, so paging doesn't move columns).
export default function SnapshotHistory({ rows, all, onEdit, onDelete }: {
  rows: Balance[]
  all: Balance[]
  onEdit: (b: Balance) => void
  onDelete: (id: number) => void
}) {
  const sections = useBankSections()
  const banks = useMemo(
    () => sections
      .map((s) => ({ ...s, fields: s.fields.filter((f) => all.some((b) => Math.abs(f.value(b)) >= 0.005)) }))
      .filter((s) => s.fields.length),
    [sections, all],
  )

  return (
    <>
      {/* Mobile cards */}
      <div className="sm:hidden divide-y divide-gray-100">
        {rows.map((b) => (
          <div key={b.id} className="px-4 py-3">
            <div className="flex items-center justify-between gap-2">
              <div>
                <p className="text-sm font-medium text-gray-700">{formatDate(b.date)}</p>
                <p className="text-xs text-gray-400">{formatTime(b.created_at)}</p>
              </div>
              <div className="flex items-center gap-1">
                <span className="text-base font-bold text-gray-900 mr-1">{formatEuro(b.total)}</span>
                <button onClick={() => onEdit(b)} className="p-2 rounded-lg text-gray-400 hover:text-blue-600 hover:bg-blue-50" aria-label="Edit snapshot">✎</button>
                <button onClick={() => onDelete(b.id)} className="p-2 rounded-lg text-gray-400 hover:text-red-600 hover:bg-red-50" aria-label="Delete snapshot">✕</button>
              </div>
            </div>
            <div className="grid grid-cols-2 gap-x-4 gap-y-1 mt-2 text-xs">
              {TYPE_TOTALS.map((t) => (
                <div key={t.group} className="flex items-center gap-1.5">
                  <Dot group={t.group} />
                  <span className="text-gray-500">{ACCOUNT_GROUP_LABELS[t.group]}</span>
                  <span className="ml-auto font-medium text-gray-800">{formatEuro(t.value(b))}</span>
                </div>
              ))}
            </div>
            <details className="mt-2">
              <summary className="text-xs text-blue-600 cursor-pointer select-none py-1">All accounts</summary>
              <div className="mt-1 space-y-2">
                {banks.map((s) => {
                  const fs = s.fields.filter((f) => Math.abs(f.value(b)) >= 0.005)
                  if (!fs.length) return null
                  return (
                    <div key={s.institution}>
                      <div className="flex justify-between text-[11px] font-semibold uppercase tracking-wide text-gray-400">
                        <span>{s.institution || 'Other'}</span>
                        <span>{formatEuro(fs.reduce((sum, f) => sum + f.value(b), 0))}</span>
                      </div>
                      {fs.map((f) => (
                        <div key={f.key} className="flex items-center gap-1.5 text-xs py-0.5">
                          <Dot group={f.group} />
                          <span className="text-gray-600">{f.label}</span>
                          <span className="ml-auto text-gray-800">{formatEuro(f.value(b))}</span>
                        </div>
                      ))}
                    </div>
                  )
                })}
              </div>
            </details>
          </div>
        ))}
        {rows.length === 0 && <p className="px-4 py-12 text-center text-gray-400 text-sm">No balance snapshots yet. Add one above.</p>}
      </div>

      {/* Desktop table */}
      <div className="overflow-x-auto hidden sm:block">
        <table className="min-w-full text-sm">
          <thead className="bg-gray-50 text-gray-600">
            <tr className="border-b border-gray-200">
              <th rowSpan={2} className="sticky left-0 z-10 bg-gray-50 text-left px-3 py-2 font-semibold align-bottom">Date</th>
              <th rowSpan={2} className="text-right px-3 py-2 font-semibold align-bottom">Total</th>
              {banks.map((s) => (
                <th key={s.institution} colSpan={s.fields.length} className="px-3 pt-2 pb-1 text-center text-xs font-semibold uppercase tracking-wide text-gray-500 border-l border-gray-200">
                  {s.institution || 'Other'}
                </th>
              ))}
              <th rowSpan={2} className="px-3 py-2" />
            </tr>
            <tr className="border-b border-gray-200">
              {banks.flatMap((s) => s.fields.map((f, i) => (
                <th key={f.key} className={`text-right px-3 pb-2 pt-1 font-medium text-xs whitespace-nowrap ${i === 0 ? 'border-l border-gray-200' : ''}`}>
                  <span className="inline-flex items-center gap-1.5"><Dot group={f.group} />{f.label}{f.kind === 'btc' ? ' (€)' : ''}</span>
                </th>
              )))}
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100 tabular-nums">
            {rows.map((b) => (
              <tr key={b.id} className="hover:bg-gray-50 group">
                <td className="sticky left-0 bg-white group-hover:bg-gray-50 px-3 py-2 whitespace-nowrap">
                  <span className="text-gray-700 font-medium">{formatDate(b.date)}</span>
                  <span className="block text-xs text-gray-400">{formatTime(b.created_at)}</span>
                </td>
                <td className="px-3 py-2 text-right font-semibold text-gray-900 whitespace-nowrap">{formatEuro(b.total)}</td>
                {banks.flatMap((s) => s.fields.map((f, i) => (
                  <td key={f.key} className={`px-3 py-2 text-right text-gray-600 whitespace-nowrap ${i === 0 ? 'border-l border-gray-100' : ''}`}>{cell(f.value(b))}</td>
                )))}
                <td className="px-3 py-2 text-right">
                  <div className="flex items-center justify-end gap-1">
                    <button onClick={() => onEdit(b)} className="p-1.5 rounded-lg text-gray-400 hover:text-blue-600 hover:bg-blue-50" aria-label="Edit snapshot">✎</button>
                    <button onClick={() => onDelete(b.id)} className="p-1.5 rounded-lg text-gray-400 hover:text-red-600 hover:bg-red-50" aria-label="Delete snapshot">✕</button>
                  </div>
                </td>
              </tr>
            ))}
            {rows.length === 0 && (
              <tr><td colSpan={3 + banks.reduce((n, s) => n + s.fields.length, 0)} className="px-4 py-12 text-center text-gray-400">No balance snapshots yet. Add one above.</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </>
  )
}
