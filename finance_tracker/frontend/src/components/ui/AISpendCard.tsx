import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { aiApi } from '../../api/insights'

const KIND_LABELS: Record<string, string> = {
  chat: 'Chat',
  view_summary: 'View reviews',
  forecast: 'Forecasts',
  analysis: 'Analysis',
  tagging: 'Auto-tagging',
  rule_review: 'Rule review',
  other: 'Other',
}

const usd = (n: number) => `$${n.toFixed(n < 1 ? 4 : 2).replace(/0+$/, '').replace(/\.$/, '')}`

export default function AISpendCard() {
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['ai-spend'], queryFn: aiApi.spend })
  const [adding, setAdding] = useState(false)
  const [amount, setAmount] = useState('')
  const [on, setOn] = useState(() => new Date().toISOString().slice(0, 10))
  const [note, setNote] = useState('')

  const add = useMutation({
    mutationFn: aiApi.addTopUp,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['ai-spend'] })
      setAdding(false)
      setAmount('')
      setNote('')
    },
  })
  const remove = useMutation({
    mutationFn: aiApi.deleteTopUp,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['ai-spend'] }),
  })

  if (isLoading || !data) return null
  const kinds = Object.entries(data.by_kind ?? {})
    .filter(([, v]) => v > 0)
    .sort((a, b) => b[1] - a[1])

  return (
    <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4 space-y-3">
      <div className="flex items-start justify-between gap-2">
        <h3 className="text-sm font-medium text-gray-800">AI spending</h3>
        <button
          onClick={() => setAdding((a) => !a)}
          className="text-xs text-indigo-600 hover:underline shrink-0"
        >
          {adding ? 'Cancel' : '+ Record a top-up'}
        </button>
      </div>

      {data.remaining !== null ? (
        <div>
          <p className="text-2xl font-semibold text-gray-900">≈ {usd(data.remaining)}</p>
          <p className="text-xs text-gray-400">
            left of {usd(data.topped_up)} added since {data.since_date}
          </p>
        </div>
      ) : (
        /* Nothing to count down from yet. Saying "$0 remaining" here would
           read as "you are out of credit", which is a different claim. */
        <p className="text-xs text-gray-500">
          Record what you last added at your provider and this will count down from it.
        </p>
      )}

      <div className="grid grid-cols-3 gap-2 text-center">
        {[
          ['This month', data.this_month],
          ['Last 30 days', data.last_30_days],
          ['All time', data.all_time],
        ].map(([label, value]) => (
          <div key={label as string} className="bg-gray-50 rounded-lg py-2">
            <p className="text-sm font-medium text-gray-800">{usd(value as number)}</p>
            <p className="text-[11px] text-gray-400">{label as string}</p>
          </div>
        ))}
      </div>

      {kinds.length > 0 && (
        <div className="space-y-1">
          {kinds.map(([kind, value]) => (
            <div key={kind} className="flex items-center justify-between text-xs">
              <span className="text-gray-500">{KIND_LABELS[kind] ?? kind}</span>
              <span className="text-gray-700">{usd(value)}</span>
            </div>
          ))}
        </div>
      )}

      {adding && (
        <div className="border-t border-gray-50 pt-3 space-y-2">
          <div className="grid grid-cols-2 gap-2">
            <label className="block text-xs text-gray-500">
              Amount (USD)
              <input
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
                inputMode="decimal"
                placeholder="20"
                className="block w-full mt-1 text-sm border border-gray-200 rounded-lg px-2 py-1.5"
              />
            </label>
            <label className="block text-xs text-gray-500">
              Date
              <input
                type="date"
                value={on}
                onChange={(e) => setOn(e.target.value)}
                className="block w-full mt-1 text-sm border border-gray-200 rounded-lg px-2 py-1.5"
              />
            </label>
          </div>
          <label className="block text-xs text-gray-500">
            Note (optional)
            <input
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder="Console top-up"
              className="block w-full mt-1 text-sm border border-gray-200 rounded-lg px-2 py-1.5"
            />
          </label>
          <button
            onClick={() =>
              add.mutate({ amount_usd: parseFloat(amount), note, occurred_on: on })
            }
            disabled={!(parseFloat(amount) > 0) || add.isPending}
            className="px-3 py-1.5 text-sm rounded-lg bg-indigo-600 text-white hover:bg-indigo-700 disabled:opacity-50"
          >
            {add.isPending ? 'Saving…' : 'Save top-up'}
          </button>
          {add.isError && (
            <p className="text-xs text-red-600">
              {add.error instanceof Error ? add.error.message : 'Could not save'}
            </p>
          )}
        </div>
      )}

      {data.top_ups.length > 0 && (
        <div className="border-t border-gray-50 pt-2 space-y-1">
          {data.top_ups.map((t) => (
            <div key={t.id} className="flex items-center justify-between text-xs">
              <span className="text-gray-500 min-w-0 truncate">
                {t.occurred_on.slice(0, 10)}
                {t.note && ` · ${t.note}`}
              </span>
              <span className="flex items-center gap-2 shrink-0">
                <span className="text-gray-700">{usd(t.amount_usd)}</span>
                <button
                  onClick={() => remove.mutate(t.id)}
                  className="text-gray-300 hover:text-red-500"
                  aria-label="Delete top-up"
                >
                  ✕
                </button>
              </span>
            </div>
          ))}
        </div>
      )}

      {/* Both caveats stated once, where the number is. The provider publishes
          no balance, so every part of this is the app's own bookkeeping. */}
      <p className="text-[11px] text-gray-400 leading-relaxed border-t border-gray-50 pt-2">
        Your provider doesn't publish a balance, so this is the app's own ledger: calls priced as
        they happen, minus what you record adding. Costs on a direct Claude key are estimated from
        list prices, and anything else billed to the same key — another app, another machine — is
        not counted here.
      </p>
    </div>
  )
}
