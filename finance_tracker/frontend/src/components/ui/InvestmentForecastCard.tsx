import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer, ReferenceLine,
} from 'recharts'
import { aiApi } from '../../api/insights'
import type { ForecastResponse } from '../../api/insights'
import { useAISettings } from '../../hooks/useInsights'
import { formatEuro } from '../../utils/format'
import Markdown from './Markdown'

// AI investment forecast on the home screen: the AI's saved assumptions
// (blended return scenarios, contribution target, target allocation) drive a
// deterministic 10-year compound projection computed right here — viewing
// the card costs nothing, only "Update forecast" spends tokens.

const SCENARIO_COLORS = ['#ef4444', '#f59e0b', '#10b981', '#6366f1']

// Future value after m months: base compounding at annual rate r plus a
// monthly contribution c (annuity), both at monthly granularity.
function fv(base: number, r: number, c: number, m: number): number {
  const i = Math.pow(1 + r, 1 / 12) - 1
  if (Math.abs(i) < 1e-9) return base + c * m
  const growth = Math.pow(1 + i, m)
  return base * growth + c * ((growth - 1) / i)
}

function horizonLabel(m: number): string {
  if (m === 0) return 'Now'
  return m % 12 === 0 ? `${m / 12}y` : `${(m / 12).toFixed(1)}y`
}

function fmtCost(costUsd?: number, inTok?: number, outTok?: number): string {
  const parts: string[] = []
  if ((costUsd ?? 0) > 0) parts.push(`$${parseFloat((costUsd as number).toFixed(4))}`)
  const n = (inTok ?? 0) + (outTok ?? 0)
  if (n > 0) parts.push(n >= 1000 ? `${parseFloat((n / 1000).toFixed(1))}k tokens` : `${n} tokens`)
  return parts.join(' · ')
}

export default function InvestmentForecastCard() {
  const { data: settings } = useAISettings()
  const configured = !!settings?.has_key && !!settings?.model
  const qc = useQueryClient()

  const { data, isLoading } = useQuery({
    queryKey: ['ai-forecast'],
    queryFn: aiApi.getForecast,
    staleTime: Infinity,
    retry: false,
    enabled: configured,
  })

  const generate = useMutation({
    mutationFn: aiApi.generateForecast,
    onSuccess: (res: ForecastResponse) => qc.setQueryData(['ai-forecast'], res),
  })

  if (!configured || isLoading) return null

  const doc = data?.exists ? data.forecast : undefined

  if (!doc) {
    return (
      <div className="bg-white rounded-xl border border-gray-200 p-6 text-center">
        <h3 className="text-base font-semibold text-gray-900">📈 AI Investment Forecast</h3>
        <p className="text-sm text-gray-500 mt-1 max-w-lg mx-auto">
          Let the AI project your net worth over 1, 5 and 10 years from your current portfolio,
          accounts and savings history — with a clear target allocation to get there.
          The result is saved, so it only costs tokens when you generate it.
        </p>
        {generate.isError && (
          <p className="text-sm text-red-600 mt-3">{(generate.error as Error).message}</p>
        )}
        <button
          onClick={() => generate.mutate()}
          disabled={generate.isPending}
          className="mt-4 px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50"
        >
          {generate.isPending ? 'Forecasting…' : '✦ Generate forecast'}
        </button>
      </div>
    )
  }

  const base = doc.current.total_eur
  const contribution = doc.ai.monthly_contribution
  const scenarios = doc.ai.scenarios

  // 10 years of quarterly points keeps the chart light; milestones read the
  // exact same formula, so the cards always match the lines.
  const points = Array.from({ length: 41 }, (_, k) => {
    const m = k * 3
    const row: Record<string, number> = { m }
    for (const sc of scenarios) row[sc.name] = Math.round(fv(base, sc.annual_return, contribution, m))
    return row
  })
  const milestones = [12, 60, 120].map((m) => ({
    m,
    values: scenarios.map((sc) => fv(base, sc.annual_return, contribution, m)),
  }))
  const mid = Math.min(1, scenarios.length - 1)

  const ForecastTooltip = ({ active, payload, label }: any) => {
    if (!active || !payload?.length) return null
    const d = new Date()
    d.setMonth(d.getMonth() + Number(label))
    return (
      <div className="bg-white border border-gray-200 rounded-lg shadow-lg px-3 py-2 text-xs">
        <p className="font-semibold text-gray-700 mb-1">
          {horizonLabel(Number(label))} · {d.toLocaleDateString('en', { month: 'short', year: 'numeric' })}
        </p>
        {[...payload].reverse().map((p: any) => (
          <div key={p.name} className="flex justify-between gap-4">
            <span style={{ color: p.stroke }} className="capitalize">{p.name}</span>
            <span className="font-medium">{formatEuro(p.value)}</span>
          </div>
        ))}
      </div>
    )
  }

  return (
    <div className="bg-white rounded-xl border border-gray-200 p-4 sm:p-6">
      <div className="flex flex-wrap items-start justify-between gap-2 mb-1">
        <div>
          <h3 className="text-base font-semibold text-gray-900">📈 AI Investment Forecast — 1 / 5 / 10 years</h3>
          <p className="text-xs text-gray-400 mt-0.5">
            From {formatEuro(base)} net worth today + {formatEuro(contribution)}/mo invested · saved{' '}
            {data?.created_at ? new Date(data.created_at).toLocaleDateString('lt-LT') : ''}
            {fmtCost(data?.cost_usd, data?.input_tokens, data?.output_tokens) && (
              <> · {fmtCost(data?.cost_usd, data?.input_tokens, data?.output_tokens)}</>
            )}
          </p>
        </div>
        <button
          onClick={() => generate.mutate()}
          disabled={generate.isPending}
          title="Re-run the forecast with current data (uses tokens)"
          className="px-3 py-1.5 text-xs font-medium text-gray-600 bg-gray-100 rounded-lg hover:bg-gray-200 disabled:opacity-50"
        >
          {generate.isPending ? 'Forecasting…' : '↻ Update forecast'}
        </button>
      </div>
      {generate.isError && (
        <p className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2 my-2">
          {(generate.error as Error).message}
        </p>
      )}

      {/* Milestone cards — the same math as the lines below */}
      <div className="grid grid-cols-3 gap-3 my-4">
        {milestones.map(({ m, values }) => (
          <div key={m} className="rounded-lg border border-gray-100 bg-gray-50/60 px-3 py-2">
            <p className="text-xs font-medium text-gray-500">In {m / 12} year{m > 12 ? 's' : ''}</p>
            <p className="text-lg sm:text-xl font-bold text-gray-900 mt-0.5">{formatEuro(values[mid])}</p>
            {values.length > 1 && (
              <p className="text-[11px] text-gray-400">
                {formatEuro(values[0])} – {formatEuro(values[values.length - 1])}
              </p>
            )}
          </div>
        ))}
      </div>

      <ResponsiveContainer width="100%" height={280}>
        <LineChart data={points} margin={{ top: 5, right: 20, left: 10, bottom: 5 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
          <XAxis
            dataKey="m"
            type="number"
            domain={[0, 120]}
            ticks={[0, 12, 24, 36, 48, 60, 72, 84, 96, 108, 120]}
            tickFormatter={horizonLabel}
            tick={{ fontSize: 11 }}
          />
          <YAxis tickFormatter={(v) => `€${(v / 1000).toFixed(0)}k`} tick={{ fontSize: 11 }} width={56} />
          <Tooltip content={<ForecastTooltip />} />
          <Legend wrapperStyle={{ fontSize: 11, textTransform: 'capitalize' }} />
          <ReferenceLine x={12} stroke="#d1d5db" strokeDasharray="3 3" />
          <ReferenceLine x={60} stroke="#d1d5db" strokeDasharray="3 3" />
          <ReferenceLine y={base} stroke="#9ca3af" strokeDasharray="3 3" />
          {scenarios.map((sc, i) => (
            <Line
              key={sc.name}
              type="monotone"
              dataKey={sc.name}
              stroke={SCENARIO_COLORS[i % SCENARIO_COLORS.length]}
              strokeWidth={i === mid ? 2.5 : 1.5}
              dot={false}
              isAnimationActive={false}
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
      <p className="text-[11px] text-gray-400 mt-1">
        {scenarios.map((sc) => `${sc.name} ${(sc.annual_return * 100).toFixed(1)}%/y`).join(' · ')} — blended
        rates across your whole net worth, chosen by the AI from your actual allocation.
      </p>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-x-8 gap-y-4 mt-5">
        {/* The clear path: narrative + concrete steps */}
        <div>
          <h4 className="text-sm font-semibold text-gray-700 mb-1.5">The path</h4>
          <div className="text-sm text-gray-600 leading-relaxed">
            <Markdown>{doc.ai.narrative}</Markdown>
          </div>
          {doc.ai.actions?.length > 0 && (
            <ul className="mt-2 space-y-1">
              {doc.ai.actions.map((a, i) => (
                <li key={i} className="flex gap-2 text-sm text-gray-600">
                  <span className="text-blue-500 shrink-0">{i + 1}.</span>
                  <span>{a}</span>
                </li>
              ))}
            </ul>
          )}
        </div>

        {/* Target allocation: how the portfolio should look */}
        {doc.ai.target_allocation?.length > 0 && (
          <div>
            <h4 className="text-sm font-semibold text-gray-700 mb-1.5">How the portfolio should look</h4>
            <div className="space-y-2.5">
              {doc.ai.target_allocation.map((a) => {
                const up = a.target_pct > a.current_pct
                return (
                  <div key={a.bucket}>
                    <div className="flex items-baseline justify-between text-sm mb-0.5">
                      <span className="font-medium text-gray-700">{a.bucket}</span>
                      <span className="text-gray-600 tabular-nums">
                        {a.current_pct.toFixed(0)}%
                        <span className={`mx-1 ${up ? 'text-green-600' : 'text-amber-600'}`}>→</span>
                        <span className="font-semibold">{a.target_pct.toFixed(0)}%</span>
                      </span>
                    </div>
                    <div className="relative h-2 bg-gray-100 rounded-full overflow-hidden">
                      <div className="absolute inset-y-0 left-0 bg-gray-300 rounded-full" style={{ width: `${a.current_pct}%` }} />
                      <div
                        className={`absolute inset-y-0 left-0 rounded-full ${up ? 'bg-blue-500/70' : 'bg-blue-500'}`}
                        style={{ width: `${Math.min(a.current_pct, a.target_pct)}%` }}
                      />
                      {/* target marker */}
                      <div className="absolute inset-y-0 w-0.5 bg-blue-700" style={{ left: `${a.target_pct}%` }} />
                    </div>
                    {a.action && <p className="text-[11px] text-gray-400 mt-0.5">{a.action}</p>}
                  </div>
                )
              })}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
