import { useState } from 'react'
import { useAssignTrip, useCreateBudget, useDeleteBudget, useTrips, useUpdateBudget } from '../../hooks/useBudgets'
import type { BudgetLineStatus, TripSuggestion, TripSummary } from '../../types'
import { formatEuro } from '../../utils/format'
import LoadingSpinner from '../ui/LoadingSpinner'
import CategoryTransactionsModal from '../ui/CategoryTransactionsModal'

function day(d: string, withYear: boolean): string {
  return new Date(`${d}T00:00:00`).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', ...(withYear ? { year: 'numeric' } : {}) })
}

// "20 – 28 Jul 2025", "28 Dec 2025 – 3 Jan 2026", "21 Sep 2026".
function dateSpan(from: string, to: string): string {
  if (!from) return 'planned'
  if (from === to) return day(from, true)
  const sameYear = from.slice(0, 4) === to.slice(0, 4)
  if (sameYear && from.slice(0, 7) === to.slice(0, 7)) return `${Number(from.slice(8))} – ${day(to, true)}`
  return `${day(from, !sameYear)} – ${day(to, true)}`
}

function TripCard({ trip, onView }: { trip: TripSummary; onView: () => void }) {
  const createBudget = useCreateBudget()
  const updateBudget = useUpdateBudget()
  const deleteBudget = useDeleteBudget()
  const [editing, setEditing] = useState(false)
  const [amount, setAmount] = useState(trip.budget ?? Math.ceil(Math.max(trip.total, 100) / 100) * 100)
  const top = trip.by_label.slice(0, 5)
  const max = Math.max(...top.map((x) => x.amount), 1)
  const over = trip.remaining != null && trip.remaining < -0.5

  async function saveBudget() {
    const input = { name: trip.name, kind: 'trip' as const, label: trip.label, amount, amount_from: 'all' }
    if (trip.budget_id) await updateBudget.mutateAsync({ id: trip.budget_id, input })
    else await createBudget.mutateAsync(input)
    setEditing(false)
  }

  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <button onClick={onView} disabled={!trip.from} className="text-sm font-semibold text-gray-900 hover:text-blue-700 text-left truncate max-w-full">
            {trip.name}
          </button>
          <p className="text-xs text-gray-400">
            {dateSpan(trip.from, trip.to)}{trip.days > 0 && <> · {trip.days} day{trip.days === 1 ? '' : 's'} · {trip.count} transactions</>}
          </p>
        </div>
        <div className="text-right flex-shrink-0">
          <p className="text-lg font-bold text-gray-900 tabular-nums">{formatEuro(trip.total)}</p>
          {trip.days > 0 && <p className="text-[11px] text-gray-400">{formatEuro(trip.per_day)}/day</p>}
        </div>
      </div>

      {top.length > 0 && (
        <div className="mt-3 space-y-1">
          {top.map((x) => (
            <div key={x.name} className="flex items-center gap-2 text-xs">
              <span className="w-20 text-gray-500 truncate">{x.name}</span>
              <div className="flex-1 h-1.5 bg-gray-100 rounded-full overflow-hidden">
                <div className="h-full bg-sky-400 rounded-full" style={{ width: `${(x.amount / max) * 100}%` }} />
              </div>
              <span className="w-16 text-right tabular-nums text-gray-700">{formatEuro(x.amount)}</span>
            </div>
          ))}
        </div>
      )}

      <div className="mt-3 pt-3 border-t border-gray-100 text-xs">
        {editing ? (
          <div className="flex flex-wrap items-center gap-2">
            <input
              type="number" inputMode="decimal" value={amount || ''}
              onChange={(e) => setAmount(parseFloat(e.target.value) || 0)}
              className="w-28 border border-gray-300 rounded-lg px-2 py-1.5 text-sm"
              aria-label="Trip budget"
            />
            <button disabled={amount <= 0 || createBudget.isPending || updateBudget.isPending} onClick={saveBudget}
              className="px-3 py-1.5 font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-40">Save</button>
            {trip.budget_id && (
              <button onClick={async () => { await deleteBudget.mutateAsync(trip.budget_id!); setEditing(false) }}
                className="px-2 py-1.5 text-red-600 hover:text-red-800">Remove budget</button>
            )}
            <button onClick={() => setEditing(false)} className="px-2 py-1.5 text-gray-500">Cancel</button>
          </div>
        ) : trip.budget != null ? (
          <div className="flex items-center justify-between gap-2">
            <span className={over ? 'text-red-600 font-medium' : 'text-gray-500'}>
              Budget {formatEuro(trip.budget)} · {over ? `${formatEuro(-(trip.remaining ?? 0))} over` : `${formatEuro(trip.remaining ?? 0)} left`}
            </span>
            <button onClick={() => setEditing(true)} className="text-blue-600 hover:text-blue-800 font-medium">Edit</button>
          </div>
        ) : (
          <button onClick={() => setEditing(true)} className="text-blue-600 hover:text-blue-800 font-medium">+ Set a trip budget</button>
        )}
      </div>
    </div>
  )
}

function SuggestionRow({ s, tripNames }: { s: TripSuggestion; tripNames: string[] }) {
  const assign = useAssignTrip()
  const [name, setName] = useState(s.suggested_label.replace(/^trip:/, ''))
  const [error, setError] = useState<string | null>(null)
  const merging = tripNames.includes(name.trim().toLowerCase().replace(/\s+/g, '-'))

  async function tag() {
    setError(null)
    try {
      await assign.mutateAsync({ name, tx_ids: s.tx_ids })
    } catch (err) {
      setError((err as Error).message)
    }
  }

  return (
    <div className="py-3 border-t border-gray-100 first:border-t-0">
      <div className="flex items-baseline justify-between gap-3">
        <p className="text-sm text-gray-800">
          {dateSpan(s.from, s.to)} <span className="text-gray-400 text-xs">· {s.count} transaction{s.count === 1 ? '' : 's'}</span>
        </p>
        <span className="text-sm font-semibold tabular-nums text-gray-800">{formatEuro(s.total)}</span>
      </div>
      <p className="text-[11px] text-gray-400 truncate">{s.top_comments.join(' · ')}</p>
      <div className="mt-2 flex items-center gap-2">
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          list="trip-names"
          placeholder="Trip name, e.g. zakopane-2025"
          className="flex-1 min-w-0 border border-gray-300 rounded-lg px-2.5 py-1.5 text-sm"
          aria-label="Trip name"
        />
        <button
          disabled={!name.trim() || assign.isPending}
          onClick={tag}
          className="px-3 py-1.5 text-xs font-medium text-white bg-sky-600 rounded-lg hover:bg-sky-700 disabled:opacity-40 whitespace-nowrap"
        >
          {assign.isPending ? 'Tagging…' : merging ? 'Add to trip' : 'Tag as trip'}
        </button>
      </div>
      {merging && <p className="text-[11px] text-sky-700 mt-1">Joins the existing trip — use this for bookings paid weeks ahead.</p>}
      {error && <p className="text-[11px] text-red-600 mt-1">{error}</p>}
    </div>
  )
}

export default function TripsView({ vacationFund }: { vacationFund?: BudgetLineStatus }) {
  const { data, isLoading } = useTrips()
  const createBudget = useCreateBudget()
  const [shown, setShown] = useState(6)
  const [viewing, setViewing] = useState<TripSummary | null>(null)
  const [planning, setPlanning] = useState(false)
  const [planName, setPlanName] = useState('')
  const [planAmount, setPlanAmount] = useState(0)

  if (isLoading || !data) return <LoadingSpinner />
  const trips = data.trips
  const tripNames = trips.map((t) => t.name)
  const taken = trips.filter((t) => t.from)
  const year = String(new Date().getFullYear())
  const thisYear = taken.filter((t) => t.from.startsWith(year))
  const yearTotal = thisYear.reduce((s, t) => s + t.total, 0)

  return (
    <div className="space-y-4 sm:space-y-6">
      <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
          <div>
            <p className="text-xs text-gray-400">Trips in {year}</p>
            <p className="text-2xl font-bold text-gray-900">{thisYear.length}</p>
            <p className="text-xs text-gray-500">{formatEuro(yearTotal)} tagged</p>
          </div>
          <div>
            <p className="text-xs text-gray-400">Vacation fund</p>
            {vacationFund?.fund_state ? (
              <>
                <p className={`text-2xl font-bold ${vacationFund.fund_state.available < 0 ? 'text-red-600' : 'text-teal-700'}`}>{formatEuro(vacationFund.fund_state.available)}</p>
                <p className="text-xs text-gray-500">{formatEuro(vacationFund.monthly_share)}/mo since {vacationFund.fund_state.start_month}</p>
              </>
            ) : (
              <p className="text-xs text-gray-500 mt-1">No Vacation fund yet — create one from "Not covered by any budget" on the Month tab.</p>
            )}
          </div>
          <div className="col-span-2 sm:col-span-1 flex sm:justify-end items-start">
            {planning ? (
              <div className="flex flex-wrap items-center gap-2 w-full sm:justify-end">
                <input value={planName} onChange={(e) => setPlanName(e.target.value)} placeholder="Trip name"
                  className="flex-1 min-w-[120px] border border-gray-300 rounded-lg px-2.5 py-1.5 text-sm" aria-label="Planned trip name" />
                <input type="number" inputMode="decimal" value={planAmount || ''} onChange={(e) => setPlanAmount(parseFloat(e.target.value) || 0)}
                  placeholder="Budget €" className="w-24 border border-gray-300 rounded-lg px-2.5 py-1.5 text-sm" aria-label="Planned trip budget" />
                <button
                  disabled={!planName.trim() || planAmount <= 0 || createBudget.isPending}
                  onClick={async () => {
                    await createBudget.mutateAsync({ name: planName, kind: 'trip', amount: planAmount })
                    setPlanning(false); setPlanName(''); setPlanAmount(0)
                  }}
                  className="px-3 py-1.5 text-xs font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-40"
                >Save</button>
                <button onClick={() => setPlanning(false)} className="text-xs text-gray-500 px-1">Cancel</button>
              </div>
            ) : (
              <button onClick={() => setPlanning(true)} className="px-3 py-1.5 text-xs font-medium text-blue-700 bg-blue-50 rounded-lg hover:bg-blue-100">
                + Plan a trip
              </button>
            )}
          </div>
        </div>
      </div>

      {trips.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {trips.map((t) => (
            <TripCard key={t.label} trip={t} onView={() => setViewing(t)} />
          ))}
        </div>
      )}

      {data.suggestions.length > 0 && (
        <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
          <h3 className="text-base font-semibold text-gray-900">Untagged holiday spending</h3>
          <p className="text-xs text-gray-400 mb-2">
            Vacation transactions grouped by date. Name each group to turn it into a trip — reuse a name to join a
            booking paid weeks ahead to its trip.
          </p>
          <datalist id="trip-names">
            {tripNames.map((n) => <option key={n} value={n} />)}
          </datalist>
          {data.suggestions.slice(0, shown).map((s) => (
            <SuggestionRow key={`${s.from}-${s.tx_ids[0]}`} s={s} tripNames={tripNames} />
          ))}
          {data.suggestions.length > shown && (
            <button onClick={() => setShown(shown + 10)} className="mt-2 text-xs font-medium text-blue-600 hover:text-blue-800">
              Show older ({data.suggestions.length - shown} more)
            </button>
          )}
        </div>
      )}

      {trips.length === 0 && data.suggestions.length === 0 && (
        <p className="text-sm text-gray-400 text-center py-8">No holiday spending yet — Vacation transactions show up here to be grouped into trips.</p>
      )}

      {viewing && (
        <CategoryTransactionsModal
          title={viewing.name}
          label={viewing.label}
          dateRange={{ date_from: viewing.from, date_to: viewing.to }}
          onClose={() => setViewing(null)}
        />
      )}
    </div>
  )
}
